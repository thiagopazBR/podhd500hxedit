//go:build !darwin && !linux

package main

import (
	"errors"

	"podhd500-edit/pkg/usb"
)

// openPedal: no USB backend on this OS.
func openPedal() (usb.Client, error) {
	return nil, errors.New("no USB backend for this OS (macOS and Linux only)")
}
