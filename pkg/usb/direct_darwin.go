//go:build darwin

package usb

import (
	"strings"
	"time"
)

var (
	_ Client = (*DriverClient)(nil)
)

// DefaultDirectOptions keeps Mac playback alive the same way it was verified
// on hardware (level 1.0, resent every 2s and after preset changes).
var DefaultDirectOptions = DirectOptions{MonitorLevel: 1.0, Keepalive: 2 * time.Second}

// isAborted: kIOReturnAborted, seen intermittently under heavy traffic.
func isAborted(err error) bool { return strings.HasSuffix(err.Error(), "0xe00002eb") }

// isGone: kIOReturnNotAttached / kIOReturnNoDevice.
func isGone(err error) bool {
	s := err.Error()
	return strings.HasSuffix(s, "0xe00002ed") || strings.HasSuffix(s, "0xe00002c0")
}
