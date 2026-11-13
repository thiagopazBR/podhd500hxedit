//go:build linux

package usb

import (
	"testing"
	"unsafe"
)

// The request numbers must match linux/usbdevice_fs.h (checked by hand for
// 64-bit; the bulk one depends on the pointer size).
func TestUsbfsRequestNumbers(t *testing.T) {
	bulk := uintptr(0xC0185502) // struct usbdevfs_bulktransfer = 24 bytes
	if unsafe.Sizeof(uintptr(0)) == 4 {
		bulk = 0xC0105502 // 16 bytes
	}
	for name, c := range map[string][2]uintptr{
		"BULK":             {reqBulk, bulk},
		"CLAIMINTERFACE":   {reqClaimIface, 0x8004550F},
		"RELEASEINTERFACE": {reqRelIface, 0x80045510},
		"CLEAR_HALT":       {reqClearHalt, 0x80045515},
		"DISCONNECT_CLAIM": {reqDisconnectClaim, 0x8108551B},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %#x, want %#x", name, c[0], c[1])
		}
	}
}
