//go:build linux

package usb

// Linux (Raspberry Pi) backend: the POD HD500 control interface (USB interface
// #1, bulk EP 0x81 IN / 0x01 OUT) through usbfs (/dev/bus/usb/BBB/DDD), pure
// Go with raw ioctls (no cgo, no libusb).
//
// The kernel's snd-usb-podhd driver normally owns the device; opening detaches
// it from interface 1 (USBDEVFS_DISCONNECT) before claiming. The user running
// the program needs write access to the device node (see scripts/udev/).
//
// Not verified on hardware yet (written 2026-11-13, pedal not at hand).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	hd500CtrlInterface = 1
	hd500EPCtrlIn      = 0x81
	hd500EPCtrlOut     = 0x01

	// readSlice is how long one bulk IN ioctl waits before giving control back,
	// so Close can stop the reader without aborting a transfer.
	readSlice = 250 * time.Millisecond

	// DefaultWriteTimeout bounds one bulk OUT transfer.
	DefaultWriteTimeout = 2 * time.Second
)

// DefaultDirectOptions: no USB playback handling on the Pi (the audio goes
// through the pedal's analog outputs), so no monitor writes.
var DefaultDirectOptions = DirectOptions{MonitorLevel: -1}

// ErrInterfaceClosed is returned after Close.
var ErrInterfaceClosed = errors.New("POD HD500 control interface closed")

// usbfs ioctl request numbers (linux/usbdevice_fs.h). The _IOC layout is the
// same on arm, arm64 and amd64.
func ioc(dir, typ, nr, size uintptr) uintptr {
	return dir<<30 | size<<16 | typ<<8 | nr
}

const (
	iocWrite = 1 // _IOC_WRITE (userspace -> kernel)
	iocRead  = 2
)

// usbdevfs_disconnect_claim: flags 0 = detach whatever driver is bound.
type disconnectClaim struct {
	iface  uint32
	flags  uint32
	driver [256]byte
}

type bulkTransfer struct {
	ep      uint32
	length  uint32
	timeout uint32 // ms
	data    unsafe.Pointer
}

var (
	reqBulk       = ioc(iocRead|iocWrite, 'U', 2, unsafe.Sizeof(bulkTransfer{}))
	reqClaimIface = ioc(iocRead, 'U', 15, 4)
	reqRelIface   = ioc(iocRead, 'U', 16, 4)
	reqClearHalt  = ioc(iocRead, 'U', 21, 4)
	// Detach any kernel driver from the interface and claim it in one call.
	reqDisconnectClaim = ioc(iocRead, 'U', 27, unsafe.Sizeof(disconnectClaim{}))
)

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

// ControlInterface is the claimed HD500 interface #1.
type ControlInterface struct {
	fd      int
	rw      sync.RWMutex // read-locked by transfers, write-locked by Close
	closeMu sync.Mutex
	closed  bool
}

// findHD500 returns the usbfs path of the first POD HD500 on the bus.
func findHD500() (string, error) {
	devs, _ := filepath.Glob("/sys/bus/usb/devices/*/idVendor")
	rd := func(dir, name string) string {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		return strings.TrimSpace(string(b))
	}
	for _, v := range devs {
		dir := filepath.Dir(v)
		if rd(dir, "idVendor") != fmt.Sprintf("%04x", VendorLine6) || rd(dir, "idProduct") != fmt.Sprintf("%04x", ProductPODHD500) {
			continue
		}
		bus, e1 := strconv.Atoi(rd(dir, "busnum"))
		dev, e2 := strconv.Atoi(rd(dir, "devnum"))
		if e1 != nil || e2 != nil {
			continue
		}
		return fmt.Sprintf("/dev/bus/usb/%03d/%03d", bus, dev), nil
	}
	return "", errors.New("POD HD500 not connected")
}

// OpenControlInterface opens HD500 interface #1 for exclusive bulk I/O,
// detaching the kernel driver from it first.
func OpenControlInterface() (*ControlInterface, error) {
	path, err := findHD500()
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w (install scripts/udev/99-hd500.rules or run as root)", path, err)
	}
	dc := disconnectClaim{iface: hd500CtrlInterface}
	if err := ioctl(fd, reqDisconnectClaim, unsafe.Pointer(&dc)); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("claim interface %d: %w (is another program, e.g. the editor, using the pedal?)", hd500CtrlInterface, err)
	}
	// The kernel driver may have left the endpoints' data toggles out of sync
	// with the pedal; a clear-halt on each resets them (host and device side).
	for _, ep := range []uint32{hd500EPCtrlOut, hd500EPCtrlIn} {
		e := ep
		if err := ioctl(fd, reqClearHalt, unsafe.Pointer(&e)); err != nil {
			warnf("clear halt 0x%02x: %v", ep, err)
		}
	}
	return &ControlInterface{fd: fd}, nil
}

func (c *ControlInterface) begin() error {
	c.rw.RLock()
	if c.closed {
		c.rw.RUnlock()
		return ErrInterfaceClosed
	}
	return nil
}

func (c *ControlInterface) bulk(ep uint8, b []byte, timeout time.Duration) (int, error) {
	t := bulkTransfer{ep: uint32(ep), length: uint32(len(b)), timeout: uint32(timeout / time.Millisecond), data: unsafe.Pointer(&b[0])}
	for {
		n, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(c.fd), reqBulk, uintptr(unsafe.Pointer(&t)))
		if e == syscall.EINTR {
			continue
		}
		if e != 0 {
			return 0, e
		}
		return int(n), nil
	}
}

// WriteRaw sends bytes on bulk OUT 0x01 as-is (the caller does the framing).
func (c *ControlInterface) WriteRaw(b []byte) error { return c.WriteRawTimeout(b, DefaultWriteTimeout) }

// WriteRawTimeout is WriteRaw with an explicit timeout.
func (c *ControlInterface) WriteRawTimeout(b []byte, timeout time.Duration) error {
	if len(b) == 0 {
		return nil
	}
	if err := c.begin(); err != nil {
		return err
	}
	defer c.rw.RUnlock()
	n, err := c.bulk(hd500EPCtrlOut, b, timeout)
	if err != nil {
		if err == syscall.ETIMEDOUT {
			return fmt.Errorf("bulk OUT 0x01: timeout after %v (the pedal didn't accept data)", timeout)
		}
		return fmt.Errorf("bulk OUT 0x01: %w", err)
	}
	if n != len(b) {
		return fmt.Errorf("bulk OUT 0x01: short write %d of %d", n, len(b))
	}
	return nil
}

// ReadRaw blocks until one bulk IN transfer arrives on 0x81 and returns it.
// It polls in readSlice steps so Close takes effect quickly.
func (c *ControlInterface) ReadRaw(buf []byte) (int, error) {
	for {
		if err := c.begin(); err != nil {
			return 0, err
		}
		n, err := c.bulk(hd500EPCtrlIn, buf, readSlice)
		c.rw.RUnlock()
		if err == syscall.ETIMEDOUT {
			continue // nothing yet; re-check closed
		}
		if err != nil {
			return 0, fmt.Errorf("bulk IN 0x81: %w", err)
		}
		return n, nil
	}
}

// ClearInStall clears a halt condition on bulk IN 0x81.
func (c *ControlInterface) ClearInStall() error {
	if err := c.begin(); err != nil {
		return err
	}
	defer c.rw.RUnlock()
	ep := uint32(hd500EPCtrlIn)
	if err := ioctl(c.fd, reqClearHalt, unsafe.Pointer(&ep)); err != nil {
		return fmt.Errorf("clear halt 0x81: %w", err)
	}
	return nil
}

// Close waits for in-flight transfers (at most readSlice / write timeout),
// releases the interface and closes the device. Safe to call more than once.
func (c *ControlInterface) Close() {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return
	}
	c.closed = true // new transfers fail; reads return within readSlice
	c.rw.Lock()
	defer c.rw.Unlock()
	n := uint32(hd500CtrlInterface)
	_ = ioctl(c.fd, reqRelIface, unsafe.Pointer(&n))
	syscall.Close(c.fd)
}

// isAborted: transfer cancelled but the pedal still holds the data (retry).
func isAborted(err error) bool {
	return errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EINTR)
}

// isGone: device unplugged or powered off.
func isGone(err error) bool {
	return errors.Is(err, syscall.ENODEV) || errors.Is(err, syscall.ESHUTDOWN)
}
