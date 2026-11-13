//go:build darwin

package usb

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	// IOKit UserClient selectors for com_line6_driver_kl6drvmsg_UserClient
	selOpen             uint32 = 0
	selClose            uint32 = 1
	selOnSendMessage    uint32 = 2
	selOnSendGetMessage uint32 = 3
	selOnSendLargeMsg   uint32 = 6
	selOnOpenPipe10_5   uint32 = 8
	selOnClosePipe10_5  uint32 = 9

	// Line 6 eL6DrvMsg IDs
	drvMsgRegisterCallback uint32 = 0x00000000
	drvMsgRecvSPSIFromHW   uint32 = 0x00000003
	drvMsgDriverNotify     uint32 = 0x00000005
	drvMsgSendSPSIToHW     uint32 = 0x00090000
	drvMsgGetUniqueID      uint32 = 0x000A0001
	drvMsgAckCallback      uint32 = 0x000A0003
	drvMsgVersionCheck     uint32 = 0x000A0004

	// struct_DrvMsg size expected by selector 2 and 3
	drvMsgStructSize = 0x1000
	drvMsgMaxInline  = drvMsgStructSize - 12 // 0xFF4 (4084 bytes)
)

var (
	initOnce sync.Once
	initErr  error

	machTaskSelf func() uint32

	ioServiceMatching            func(name *byte) uintptr
	ioServiceGetMatchingService  func(mainPort uint32, matching uintptr) uint32
	ioServiceOpen                func(service uint32, owningTask uint32, connType uint32, connect *uint32) int32
	ioServiceClose               func(connect uint32) int32
	ioObjectRelease              func(object uint32) int32
	ioNotificationPortCreate     func(mainPort uint32) uintptr
	ioNotificationPortDestroy    func(notify uintptr)
	ioNotificationPortGetRunLoop func(notify uintptr) uintptr
	ioNotificationPortGetMach    func(notify uintptr) uint32

	ioConnectCallMethod func(
		connection uint32,
		selector uint32,
		input *uint64,
		inputCnt uint32,
		inputStruct unsafe.Pointer,
		inputStructCnt uintptr,
		output *uint64,
		outputCnt *uint32,
		outputStruct unsafe.Pointer,
		outputStructCnt *uintptr,
	) int32

	ioConnectCallAsyncScalarMethod func(
		connection uint32,
		selector uint32,
		wakePort uint32,
		reference *uint64,
		referenceCnt uint32,
		input *uint64,
		inputCnt uint32,
		output *uint64,
		outputCnt *uint32,
	) int32

	cfRunLoopGetCurrent   func() uintptr
	cfRunLoopAddSource    func(rl uintptr, source uintptr, mode uintptr)
	cfRunLoopRunInMode    func(mode uintptr, seconds float64, returnAfterSourceHandled bool) int32
	cfRunLoopStop         func(rl uintptr)
	kCFRunLoopDefaultMode uintptr

	// Global callback pointer registered with purego
	globalAsyncCallback uintptr
	activePipesMu       sync.RWMutex
	activePipes                 = map[uintptr]*driverPipe{}
	nextPipeID          uintptr = 1
)

func initFrameworks() error {
	initOnce.Do(func() {
		libSys, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			initErr = fmt.Errorf("dlopen libSystem: %w", err)
			return
		}
		iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			initErr = fmt.Errorf("dlopen IOKit: %w", err)
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			initErr = fmt.Errorf("dlopen CoreFoundation: %w", err)
			return
		}

		purego.RegisterLibFunc(&machTaskSelf, libSys, "mach_task_self")

		purego.RegisterLibFunc(&ioServiceMatching, iokit, "IOServiceMatching")
		purego.RegisterLibFunc(&ioServiceGetMatchingService, iokit, "IOServiceGetMatchingService")
		purego.RegisterLibFunc(&ioServiceOpen, iokit, "IOServiceOpen")
		purego.RegisterLibFunc(&ioServiceClose, iokit, "IOServiceClose")
		purego.RegisterLibFunc(&ioObjectRelease, iokit, "IOObjectRelease")
		purego.RegisterLibFunc(&ioNotificationPortCreate, iokit, "IONotificationPortCreate")
		purego.RegisterLibFunc(&ioNotificationPortDestroy, iokit, "IONotificationPortDestroy")
		purego.RegisterLibFunc(&ioNotificationPortGetRunLoop, iokit, "IONotificationPortGetRunLoopSource")
		purego.RegisterLibFunc(&ioNotificationPortGetMach, iokit, "IONotificationPortGetMachPort")
		purego.RegisterLibFunc(&ioConnectCallMethod, iokit, "IOConnectCallMethod")
		purego.RegisterLibFunc(&ioConnectCallAsyncScalarMethod, iokit, "IOConnectCallAsyncScalarMethod")

		purego.RegisterLibFunc(&cfRunLoopGetCurrent, cf, "CFRunLoopGetCurrent")
		purego.RegisterLibFunc(&cfRunLoopAddSource, cf, "CFRunLoopAddSource")
		purego.RegisterLibFunc(&cfRunLoopRunInMode, cf, "CFRunLoopRunInMode")
		purego.RegisterLibFunc(&cfRunLoopStop, cf, "CFRunLoopStop")

		modeSym, err := purego.Dlsym(cf, "kCFRunLoopDefaultMode")
		if err != nil {
			initErr = fmt.Errorf("dlsym kCFRunLoopDefaultMode: %w", err)
			return
		}
		kCFRunLoopDefaultMode = *(*uintptr)(unsafe.Pointer(modeSym))

		globalAsyncCallback = purego.NewCallback(drvMsgAsyncCallback)
	})
	return initErr
}

// drvMsgAsyncCallback matches IOKit IOAsyncCallback:
// void DrvMsgCallback(void *refcon, IOReturn result, void **args, uint32_t numArgs)
func drvMsgAsyncCallback(refcon uintptr, result int32, args uintptr, numArgs uint32) {
	activePipesMu.RLock()
	pipe := activePipes[refcon]
	activePipesMu.RUnlock()
	if pipe == nil {
		return
	}
	pipe.handleAsyncCallback(result, args, numArgs)
}

type driverPipe struct {
	mu         sync.Mutex
	refID      uintptr
	appID      uint64
	connect    uint32
	uniqueID   uint32
	notifyPort uintptr
	runLoop    uintptr
	closed     bool
	stopCh     chan struct{}
	doneCh     chan struct{}
	recvCh     chan *SPSIMessage
}

func openDriverPipe(appID uint64, recvCh chan *SPSIMessage) (*driverPipe, error) {
	cName := append([]byte("com_line6_driver_kl6drvmsg"), 0)
	matching := ioServiceMatching(&cName[0])
	if matching == 0 {
		return nil, fmt.Errorf("IOServiceMatching(com_line6_driver_kl6drvmsg) returned nil")
	}

	service := ioServiceGetMatchingService(0, matching)
	if service == 0 {
		return nil, fmt.Errorf("com_line6_driver_kl6drvmsg service not found (is POD HD500 connected and powered on?)")
	}
	defer ioObjectRelease(service)

	var connect uint32
	kr := ioServiceOpen(service, machTaskSelf(), 0, &connect)
	if kr != 0 {
		return nil, fmt.Errorf("IOServiceOpen failed with IOReturn 0x%08x", uint32(kr))
	}

	// Selector 0: sOpen
	kr = ioConnectCallMethod(connect, selOpen, nil, 0, nil, 0, nil, nil, nil, nil)
	if kr != 0 {
		ioServiceClose(connect)
		return nil, fmt.Errorf("IOConnectCallMethod(sOpen) failed: 0x%08x", uint32(kr))
	}

	activePipesMu.Lock()
	refID := nextPipeID
	nextPipeID++
	p := &driverPipe{
		refID:   refID,
		appID:   appID,
		connect: connect,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		recvCh:  recvCh,
	}
	activePipes[refID] = p
	activePipesMu.Unlock()

	readyCh := make(chan error, 1)
	go p.runLoopThread(readyCh)
	if err := <-readyCh; err != nil {
		p.Close()
		return nil, err
	}

	uniqueID, err := p.queryUniqueID()
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("queryUniqueID: %w", err)
	}
	p.uniqueID = uniqueID

	if err := p.sendVersionCheck(); err != nil {
		p.Close()
		return nil, fmt.Errorf("sendVersionCheck: %w", err)
	}

	if err := p.registerCallback(3); err != nil {
		p.Close()
		return nil, fmt.Errorf("registerCallback(3): %w", err)
	}
	_ = p.registerCallback(5)

	return p, nil
}

func (p *driverPipe) runLoopThread(readyCh chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(p.doneCh)

	notifyPort := ioNotificationPortCreate(0)
	if notifyPort == 0 {
		readyCh <- fmt.Errorf("IONotificationPortCreate failed")
		return
	}
	p.notifyPort = notifyPort
	defer ioNotificationPortDestroy(notifyPort)

	rlSource := ioNotificationPortGetRunLoop(notifyPort)
	wakePort := ioNotificationPortGetMach(notifyPort)
	p.runLoop = cfRunLoopGetCurrent()

	cfRunLoopAddSource(p.runLoop, rlSource, kCFRunLoopDefaultMode)

	var asyncRef [8]uint64
	asyncRef[0] = uint64(wakePort)
	asyncRef[1] = uint64(globalAsyncCallback)
	asyncRef[2] = uint64(p.refID)

	inputScalar := [4]uint64{
		0,
		0,
		p.appID,
		1,
	}
	var outCnt uint32
	kr := ioConnectCallAsyncScalarMethod(
		p.connect,
		selOnOpenPipe10_5,
		wakePort,
		&asyncRef[0],
		8,
		&inputScalar[0],
		4,
		nil,
		&outCnt,
	)
	if kr != 0 {
		readyCh <- fmt.Errorf("sOnOpenPipe_10_5(appID=0x%X) failed: 0x%08x", p.appID, uint32(kr))
		return
	}

	readyCh <- nil

	for {
		select {
		case <-p.stopCh:
			return
		default:
			cfRunLoopRunInMode(kCFRunLoopDefaultMode, 0.05, true)
		}
	}
}

func (p *driverPipe) handleAsyncCallback(result int32, args uintptr, numArgs uint32) {
	debugf("callback appID=0x%X result=0x%08x args=0x%x numArgs=%d", p.appID, uint32(result), args, numArgs)
	if result != 0 || args == 0 || numArgs < 2 || numArgs >= 5 {
		debugf("  -> descartado: result/args/numArgs fora do esperado")
		return
	}

	argSlice := unsafe.Slice((*uint64)(unsafe.Pointer(args)), numArgs)
	for i, a := range argSlice {
		debugf("  arg[%d]=0x%016x", i, a)
	}
	sharedBufAddr := uintptr(argSlice[0])
	totalLen := uint32(argSlice[1])

	if sharedBufAddr == 0 || totalLen < 28 || totalLen > 0x20000 {
		debugf("  -> descartado: sharedBuf=0x%x totalLen=%d", sharedBufAddr, totalLen)
		return
	}

	rawShared := unsafe.Slice((*byte)(unsafe.Pointer(sharedBufAddr)), int(totalLen))
	buf := make([]byte, len(rawShared))
	copy(buf, rawShared)

	proxyID := binary.LittleEndian.Uint32(buf[0:4])
	marshaled := buf[20:]
	if Debug {
		dump := buf
		if len(dump) > 256 {
			dump = dump[:256]
		}
		debugf("  proxyID=%d totalLen=%d raw (primeiros %d bytes):\n%s", proxyID, totalLen, len(dump), hex.Dump(dump))
	}

	// Send ACK (0x000A0003) with proxyID immediately
	if err := p.sendAck(proxyID); err != nil {
		debugf("  ACK(proxyID=%d) falhou: %v", proxyID, err)
	}

	if len(marshaled) < 8 {
		debugf("  -> descartado: marshaled curto (%d bytes)", len(marshaled))
		return
	}
	msgID := binary.LittleEndian.Uint32(marshaled[0:4])
	numParams := binary.LittleEndian.Uint32(marshaled[4:8])
	debugf("  msgID=0x%08X numParams=%d", msgID, numParams)

	if msgID != drvMsgRecvSPSIFromHW || numParams < 1 || len(marshaled) < 12 {
		debugf("  -> nao e SPSI do hardware, ignorado")
		return
	}
	paramLen := binary.LittleEndian.Uint32(marshaled[8:12])
	if uint32(len(marshaled)-12) < paramLen {
		debugf("  -> descartado: paramLen=%d > disponivel=%d", paramLen, len(marshaled)-12)
		return
	}
	spsiRaw := marshaled[12 : 12+paramLen]
	msg, err := ParseSPSI(spsiRaw)
	if err != nil {
		debugf("  -> ParseSPSI falhou: %v (raw=%x)", err, spsiRaw)
		return
	}
	select {
	case p.recvCh <- msg:
	default:
		debugf("  -> recvCh cheio, descartando a mensagem mais antiga")
		select {
		case <-p.recvCh:
		default:
		}
		p.recvCh <- msg
	}
}

func (p *driverPipe) queryUniqueID() (uint32, error) {
	marshaled := marshalDrvMsg(drvMsgGetUniqueID)
	inStruct, err := packDrvMsgStruct(marshaled)
	if err != nil {
		return 0, err
	}
	var outStruct [drvMsgStructSize]byte
	binary.LittleEndian.PutUint32(outStruct[0:4], drvMsgStructSize)
	outSize := uintptr(drvMsgStructSize)

	kr := ioConnectCallMethod(
		p.connect,
		selOnSendGetMessage,
		nil,
		0,
		unsafe.Pointer(&inStruct[0]),
		drvMsgStructSize,
		nil,
		nil,
		unsafe.Pointer(&outStruct[0]),
		&outSize,
	)
	if kr != 0 {
		return 0, fmt.Errorf("sOnSendGetMessage(0x000A0001) failed: 0x%08x", uint32(kr))
	}
	return binary.LittleEndian.Uint32(outStruct[12:16]), nil
}

func (p *driverPipe) sendDrvMsg(marshaled []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("driver pipe closed")
	}

	if len(marshaled) <= drvMsgMaxInline {
		inStruct, err := packDrvMsgStruct(marshaled)
		if err != nil {
			return err
		}
		scalarIn := uint64(p.uniqueID)
		kr := ioConnectCallMethod(
			p.connect,
			selOnSendMessage,
			&scalarIn,
			1,
			unsafe.Pointer(&inStruct[0]),
			drvMsgStructSize,
			nil,
			nil,
			nil,
			nil,
		)
		if kr != 0 {
			return fmt.Errorf("sOnSendMessage failed: 0x%08x", uint32(kr))
		}
		return nil
	}

	scalarIn := [3]uint64{
		uint64(p.uniqueID),
		uint64(uintptr(unsafe.Pointer(&marshaled[0]))),
		uint64(len(marshaled)),
	}
	kr := ioConnectCallMethod(
		p.connect,
		selOnSendLargeMsg,
		&scalarIn[0],
		3,
		nil,
		0,
		nil,
		nil,
		nil,
		nil,
	)
	runtime.KeepAlive(marshaled)
	if kr != 0 {
		return fmt.Errorf("sOnSendLargeMessage failed: 0x%08x", uint32(kr))
	}
	return nil
}

func (p *driverPipe) sendVersionCheck() error {
	var zero [4]byte
	marshaled := marshalDrvMsg(drvMsgVersionCheck, zero[:])
	return p.sendDrvMsg(marshaled)
}

func (p *driverPipe) registerCallback(cbType byte) error {
	var fnPtr [8]byte
	var ctxPtr [8]byte
	binary.LittleEndian.PutUint64(fnPtr[:], 0x100000000|uint64(cbType))
	binary.LittleEndian.PutUint64(ctxPtr[:], uint64(p.refID))
	marshaled := marshalDrvMsg(drvMsgRegisterCallback, fnPtr[:], ctxPtr[:], []byte{cbType})
	return p.sendDrvMsg(marshaled)
}

func (p *driverPipe) sendAck(proxyID uint32) error {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], proxyID)
	marshaled := marshalDrvMsg(drvMsgAckCallback, b[:])
	return p.sendDrvMsg(marshaled)
}

func (p *driverPipe) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.stopCh)
	if p.runLoop != 0 {
		cfRunLoopStop(p.runLoop)
	}
	p.mu.Unlock()

	<-p.doneCh

	activePipesMu.Lock()
	delete(activePipes, p.refID)
	activePipesMu.Unlock()

	if p.connect != 0 {
		scalarIn := uint64(p.uniqueID)
		_ = ioConnectCallMethod(p.connect, selOnClosePipe10_5, &scalarIn, 1, nil, 0, nil, nil, nil, nil)
		_ = ioConnectCallMethod(p.connect, selClose, nil, 0, nil, 0, nil, nil, nil, nil)
		_ = ioServiceClose(p.connect)
		p.connect = 0
	}
	return nil
}

func marshalDrvMsg(msgID uint32, params ...[]byte) []byte {
	total := 8
	for _, p := range params {
		total += 4 + len(p)
	}
	out := make([]byte, total)
	binary.LittleEndian.PutUint32(out[0:4], msgID)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(params)))
	offset := 8
	for _, p := range params {
		binary.LittleEndian.PutUint32(out[offset:offset+4], uint32(len(p)))
		offset += 4
		copy(out[offset:offset+len(p)], p)
		offset += len(p)
	}
	return out
}

func packDrvMsgStruct(marshaled []byte) ([drvMsgStructSize]byte, error) {
	var st [drvMsgStructSize]byte
	if len(marshaled) > drvMsgMaxInline {
		return st, fmt.Errorf("marshaled message (%d bytes) exceeds inline struct limit (%d bytes)", len(marshaled), drvMsgMaxInline)
	}
	binary.LittleEndian.PutUint32(st[0:4], drvMsgStructSize)
	binary.LittleEndian.PutUint32(st[4:8], uint32(len(marshaled)))
	binary.LittleEndian.PutUint32(st[8:12], 0)
	copy(st[12:], marshaled)
	return st, nil
}

// DriverClient manages communication with the Line 6 macOS kernel driver
// (com_line6_driver_kl6drvmsg) for the POD HD500.
// It opens pipes for both 0x7D01002D (spsi[3]==0x08) and 0x7D010027 (spsi[3]==0x05)
// so all incoming hardware messages are captured regardless of target address byte.
type DriverClient struct {
	primary   *driverPipe
	secondary *driverPipe
	recvCh    chan *SPSIMessage
}

// OpenDriverClient connects to com_line6_driver_kl6drvmsg and starts listening
// for SPSI messages from the POD HD500.
func OpenDriverClient() (*DriverClient, error) {
	if err := initFrameworks(); err != nil {
		return nil, err
	}

	recvCh := make(chan *SPSIMessage, 256)

	// Pipe 1: AppIDPODHD500XEdit (0x7D01002D -> spsi[3] == 0x08)
	p1, err := openDriverPipe(AppIDPODHD500XEdit, recvCh)
	if err != nil {
		return nil, err
	}

	// Pipe 2: AppIDPODHD500Edit (0x7D010027 -> spsi[3] == 0x05)
	p2, err := openDriverPipe(AppIDPODHD500Edit, recvCh)
	if err != nil {
		debugf("pipe secundario (0x7D010027) nao abriu: %v", err)
		p2 = nil
	}

	return &DriverClient{
		primary:   p1,
		secondary: p2,
		recvCh:    recvCh,
	}, nil
}

// SendSPSI sends a complete L6SPSI message to the POD HD500 via eL6DrvMsg 0x00090000.
func (c *DriverClient) SendSPSI(spsi []byte) error {
	marshaled := marshalDrvMsg(drvMsgSendSPSIToHW, spsi)
	return c.primary.sendDrvMsg(marshaled)
}

// RecvChan returns the channel of incoming SPSI messages from the POD HD500.
func (c *DriverClient) RecvChan() <-chan *SPSIMessage {
	return c.recvCh
}

// UniqueID returns the unique client ID assigned by the Line 6 kernel driver.
func (c *DriverClient) UniqueID() uint32 {
	return c.primary.uniqueID
}

// SecondaryUniqueID returns the unique client ID of the secondary pipe (0x7D010027), if open.
func (c *DriverClient) SecondaryUniqueID() uint32 {
	if c.secondary != nil {
		return c.secondary.uniqueID
	}
	return 0
}

// Close cleanly unregisters all pipes and closes the IOKit UserClient connections.
func (c *DriverClient) Close() error {
	if c.secondary != nil {
		_ = c.secondary.Close()
	}
	if c.primary != nil {
		return c.primary.Close()
	}
	return nil
}
