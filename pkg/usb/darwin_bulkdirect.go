//go:build darwin

package usb

// Direct user-space access to the POD HD500 control interface (USB interface
// #1, bulk EP 0x81 IN / 0x01 OUT) through IOUSBLib's COM-style plug-in API,
// called via purego (no cgo).
//
// The Line 6 kext normally owns this interface through
// com_line6_driver_bulkpipecomm, but on Apple Silicon that driver starts
// before kl6drvmsg exists and never gets bound (see cmd/l6-rebind). After it
// is terminated (sudo l6-rebind release) the interface is free and can be
// opened here, while audio (interface 0) stays on the kext.

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// IOUSBLib UUIDs (IOUSBLib.h / IOCFPlugIn.h).
const (
	uuidUSBInterfaceUserClientType = "2D9786C6-9EF3-11D4-AD51-000A27052861"
	uuidCFPlugInInterface          = "C244E858-109C-11D4-91D4-0050E4C6426F"
	uuidUSBInterfaceInterface      = "73C97AE8-9EF3-11D4-B1D0-000A27052861" // IOUSBInterfaceStruct100
)

// IOUSBInterfaceStruct100 vtable slots (pointer-sized, IUNKNOWN_C_GUTS first).
const (
	vtQueryInterface    = 1
	vtRelease           = 3
	vtUSBInterfaceOpen  = 8
	vtUSBInterfaceClose = 9
	vtGetNumEndpoints   = 19
	vtGetPipeProperties = 26
	vtAbortPipe         = 28
	vtClearPipeStall    = 30
	vtReadPipe          = 31
	vtWritePipe         = 32
)

const (
	hd500CtrlInterface = 1
	hd500EPCtrlIn      = 0x81
	hd500EPCtrlOut     = 0x01

	kCFStringEncodingUTF8Direct = 0x08000100
	kCFNumberSInt32TypeDirect   = 3
)

var (
	directOnce sync.Once
	directErr  error

	dIOServiceMatching           func(name *byte) uintptr
	dIOServiceGetMatchingService func(mainPort uint32, matching uintptr) uint32
	dIOObjectRelease             func(obj uint32) int32
	dIOCreatePlugInInterface     func(service uint32, pluginType uintptr, interfaceType uintptr, out *uintptr, score *int32) int32
	dCFUUIDCreateFromString      func(alloc uintptr, s uintptr) uintptr
	dCFUUIDGetUUIDBytes          func(uuid uintptr) [16]byte
	dCFStringCreateWithCString   func(alloc uintptr, s *byte, enc uint32) uintptr
	dCFNumberCreate              func(alloc uintptr, typ int, valuePtr unsafe.Pointer) uintptr
	dCFDictionaryCreateMutable   func(alloc uintptr, capacity int, keyCB uintptr, valCB uintptr) uintptr
	dCFDictionarySetValue        func(dict uintptr, key uintptr, value uintptr)
	dCFRelease                   func(obj uintptr)
	dKeyCallBacks, dValCallBacks uintptr
)

func initDirect() error {
	directOnce.Do(func() {
		iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			directErr = err
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			directErr = err
			return
		}
		purego.RegisterLibFunc(&dIOServiceMatching, iokit, "IOServiceMatching")
		purego.RegisterLibFunc(&dIOServiceGetMatchingService, iokit, "IOServiceGetMatchingService")
		purego.RegisterLibFunc(&dIOObjectRelease, iokit, "IOObjectRelease")
		purego.RegisterLibFunc(&dIOCreatePlugInInterface, iokit, "IOCreatePlugInInterfaceForService")
		purego.RegisterLibFunc(&dCFUUIDCreateFromString, cf, "CFUUIDCreateFromString")
		purego.RegisterLibFunc(&dCFStringCreateWithCString, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&dCFNumberCreate, cf, "CFNumberCreate")
		purego.RegisterLibFunc(&dCFDictionaryCreateMutable, cf, "CFDictionaryCreateMutable")
		purego.RegisterLibFunc(&dCFDictionarySetValue, cf, "CFDictionarySetValue")
		purego.RegisterLibFunc(&dCFRelease, cf, "CFRelease")
		if dKeyCallBacks, err = purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks"); err != nil {
			directErr = err
			return
		}
		if dValCallBacks, err = purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks"); err != nil {
			directErr = err
		}
	})
	return directErr
}

func cfStr(s string) uintptr {
	b := append([]byte(s), 0)
	return dCFStringCreateWithCString(0, &b[0], kCFStringEncodingUTF8Direct)
}

// uuidBytes parses a canonical UUID string into its 16 raw bytes.
func uuidBytes(s string) [16]byte {
	var out [16]byte
	j := 0
	for i := 0; i < len(s) && j < 16; i++ {
		if s[i] == '-' {
			continue
		}
		hi, lo := hexNib(s[i]), hexNib(s[i+1])
		out[j] = hi<<4 | lo
		j++
		i++
	}
	return out
}

func hexNib(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

func cfUUID(s string) uintptr {
	str := cfStr(s)
	defer dCFRelease(str)
	return dCFUUIDCreateFromString(0, str)
}

// comCall invokes vtable slot `slot` of COM object `obj` (an **vtable).
func comCall(obj uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := purego.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

// PipeInfo describes one endpoint of the opened interface.
type PipeInfo struct {
	Ref           uint8
	Direction     uint8 // 0 out, 1 in
	Number        uint8
	TransferType  uint8 // 2 = bulk, 3 = interrupt
	MaxPacketSize uint16
	Interval      uint8
}

// Address returns the USB endpoint address (0x81 style).
func (p PipeInfo) Address() uint8 {
	if p.Direction == 1 {
		return 0x80 | p.Number
	}
	return p.Number
}

// ControlInterface is an open user-space handle on HD500 USB interface #1.
//
// Concurrency: ReadRaw, WriteRaw and ClearInStall may run concurrently (one
// reader, one writer). Every operation holds a read lock on rw for its whole
// duration; Close sets closing, keeps aborting both pipes until it can take
// the write lock (i.e. no operation is in flight), and only then releases the
// interface, so the COM object is never used after release.
type ControlInterface struct {
	rw       sync.RWMutex
	closeMu  sync.Mutex // serializes Close calls (aborts must never race the release)
	closing  atomic.Bool
	released bool    // guarded by closeMu
	intf     uintptr // IOUSBInterfaceInterface**
	Pipes    []PipeInfo
	inPipe   uint8
	outPipe  uint8
}

// OwnAbortCalls counts every AbortPipe/ClearPipeStall issued by this package
// (timeouts, Close, stall recovery), so aborted transfers can be told apart
// from aborts coming from elsewhere.
var OwnAbortCalls atomic.Int64

// ErrInterfaceClosed is returned by operations attempted during/after Close.
var ErrInterfaceClosed = errors.New("POD HD500 control interface closed")

// DefaultWriteTimeout bounds a bulk OUT write. A write can block forever when
// the pedal stops accepting OUT data (e.g. its IN queue is not being drained).
const DefaultWriteTimeout = 2 * time.Second

func (c *ControlInterface) begin() error {
	if !c.rw.TryRLock() {
		return ErrInterfaceClosed
	}
	if c.closing.Load() {
		c.rw.RUnlock()
		return ErrInterfaceClosed
	}
	return nil
}

func findHD500Interface(ifaceNum int32) uint32 {
	match := dIOServiceMatching(cstrPtr("IOUSBHostInterface"))
	props := dCFDictionaryCreateMutable(0, 0, dKeyCallBacks, dValCallBacks)
	set := func(k string, v int32) {
		ks := cfStr(k)
		n := dCFNumberCreate(0, kCFNumberSInt32TypeDirect, unsafe.Pointer(&v))
		dCFDictionarySetValue(props, ks, n)
		dCFRelease(ks)
		dCFRelease(n)
	}
	set("idVendor", VendorLine6)
	set("idProduct", ProductPODHD500)
	set("bInterfaceNumber", ifaceNum)
	pk := cfStr("IOPropertyMatch")
	dCFDictionarySetValue(match, pk, props)
	dCFRelease(pk)
	dCFRelease(props)
	return dIOServiceGetMatchingService(0, match)
}

func cstrPtr(s string) *byte { b := append([]byte(s), 0); return &b[0] }

// OpenControlInterface opens HD500 interface #1 for exclusive bulk I/O.
// It fails with kIOReturnExclusiveAccess (0xe00002c5) while the Line 6
// bulkpipecomm driver still owns the interface.
func OpenControlInterface() (*ControlInterface, error) {
	if err := initDirect(); err != nil {
		return nil, err
	}
	svc := findHD500Interface(hd500CtrlInterface)
	if svc == 0 {
		return nil, fmt.Errorf("POD HD500 USB interface #%d not found", hd500CtrlInterface)
	}
	defer dIOObjectRelease(svc)

	pluginType := cfUUID(uuidUSBInterfaceUserClientType)
	plugInIface := cfUUID(uuidCFPlugInInterface)
	defer dCFRelease(pluginType)
	defer dCFRelease(plugInIface)

	var plugin uintptr // IOCFPlugInInterface**
	var score int32
	if kr := dIOCreatePlugInInterface(svc, pluginType, plugInIface, &plugin, &score); kr != 0 || plugin == 0 {
		return nil, fmt.Errorf("IOCreatePlugInInterfaceForService: 0x%08x", uint32(kr))
	}
	defer comCall(plugin, vtRelease)

	// QueryInterface(self, CFUUIDBytes iid /* 16 bytes by value: 2 regs */, void **ppv)
	iid := uuidBytes(uuidUSBInterfaceInterface)
	lo := *(*uintptr)(unsafe.Pointer(&iid[0]))
	hi := *(*uintptr)(unsafe.Pointer(&iid[8]))
	var intf uintptr
	if hr := int32(comCall(plugin, vtQueryInterface, lo, hi, uintptr(unsafe.Pointer(&intf)))); hr != 0 || intf == 0 {
		return nil, fmt.Errorf("QueryInterface(IOUSBInterfaceInterface): 0x%08x", uint32(hr))
	}

	if kr := int32(comCall(intf, vtUSBInterfaceOpen)); kr != 0 {
		comCall(intf, vtRelease)
		return nil, fmt.Errorf("USBInterfaceOpen: 0x%08x (0xe00002c5 = interface still owned by the Line 6 kext; run 'sudo l6-rebind release')", uint32(kr))
	}

	ci := &ControlInterface{intf: intf}
	var n uint8
	comCall(intf, vtGetNumEndpoints, uintptr(unsafe.Pointer(&n)))
	for ref := uint8(1); ref <= n; ref++ {
		var p PipeInfo
		p.Ref = ref
		comCall(intf, vtGetPipeProperties, uintptr(ref),
			uintptr(unsafe.Pointer(&p.Direction)), uintptr(unsafe.Pointer(&p.Number)),
			uintptr(unsafe.Pointer(&p.TransferType)), uintptr(unsafe.Pointer(&p.MaxPacketSize)),
			uintptr(unsafe.Pointer(&p.Interval)))
		ci.Pipes = append(ci.Pipes, p)
		switch p.Address() {
		case hd500EPCtrlIn:
			ci.inPipe = ref
		case hd500EPCtrlOut:
			ci.outPipe = ref
		}
	}
	if ci.inPipe == 0 || ci.outPipe == 0 {
		ci.Close()
		return nil, fmt.Errorf("control endpoints 0x81/0x01 not found on interface #1 (pipes: %+v)", ci.Pipes)
	}
	return ci, nil
}

// WriteRaw sends bytes on bulk OUT 0x01 as-is (caller does the chunk framing),
// giving up after DefaultWriteTimeout.
func (c *ControlInterface) WriteRaw(b []byte) error {
	return c.WriteRawTimeout(b, DefaultWriteTimeout)
}

// WriteRawTimeout is WriteRaw with an explicit timeout. On timeout the OUT
// pipe is aborted so the blocked WritePipe returns.
func (c *ControlInterface) WriteRawTimeout(b []byte, timeout time.Duration) error {
	if len(b) == 0 {
		return nil
	}
	if err := c.begin(); err != nil {
		return err
	}
	defer c.rw.RUnlock()

	var timedOut atomic.Bool
	fired := make(chan struct{})
	t := time.AfterFunc(timeout, func() {
		defer close(fired)
		timedOut.Store(true)
		OwnAbortCalls.Add(1)
		comCall(c.intf, vtAbortPipe, uintptr(c.outPipe)) // intf valid: we hold the read lock
	})
	kr := int32(comCall(c.intf, vtWritePipe, uintptr(c.outPipe), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b))))
	if !t.Stop() {
		<-fired // never return (and drop the read lock) while the abort may still run
	}
	switch {
	case timedOut.Load():
		return fmt.Errorf("WritePipe(0x01): timeout after %v (the pedal didn't accept data; kr=0x%08x)", timeout, uint32(kr))
	case kr != 0:
		return fmt.Errorf("WritePipe(0x01): 0x%08x", uint32(kr))
	}
	return nil
}

// ReadRaw blocks until one bulk IN transfer arrives on 0x81 and returns it.
// Close unblocks it (returns ErrInterfaceClosed or an abort error).
func (c *ControlInterface) ReadRaw(buf []byte) (int, error) {
	if err := c.begin(); err != nil {
		return 0, err
	}
	defer c.rw.RUnlock()
	size := uint32(len(buf))
	kr := int32(comCall(c.intf, vtReadPipe, uintptr(c.inPipe), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))))
	if kr != 0 {
		return 0, fmt.Errorf("ReadPipe(0x81): 0x%08x", uint32(kr))
	}
	return int(size), nil
}

// ClearInStall clears a halt/stall condition on bulk IN 0x81 (both host and
// device side) so reading can resume after an error.
func (c *ControlInterface) ClearInStall() error {
	if err := c.begin(); err != nil {
		return err
	}
	defer c.rw.RUnlock()
	OwnAbortCalls.Add(1)
	if kr := int32(comCall(c.intf, vtClearPipeStall, uintptr(c.inPipe))); kr != 0 {
		return fmt.Errorf("ClearPipeStall(0x81): 0x%08x", uint32(kr))
	}
	return nil
}

// Close aborts pending transfers, waits until none is in flight, then closes
// and releases the interface. Safe to call more than once and concurrently
// with ReadRaw/WriteRaw.
func (c *ControlInterface) Close() {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.released {
		return
	}
	c.closing.Store(true)
	// Keep aborting until every in-flight operation has returned: a ReadPipe
	// posted just after one abort is caught by the next.
	for !c.rw.TryLock() {
		if c.inPipe != 0 {
			comCall(c.intf, vtAbortPipe, uintptr(c.inPipe))
		}
		if c.outPipe != 0 {
			comCall(c.intf, vtAbortPipe, uintptr(c.outPipe))
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer c.rw.Unlock() // later operations still fail: begin() checks closing
	c.released = true
	comCall(c.intf, vtUSBInterfaceClose)
	comCall(c.intf, vtRelease)
}
