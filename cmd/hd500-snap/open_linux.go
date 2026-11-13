//go:build linux

package main

import "podhd500-edit/pkg/usb"

// openPedal opens the POD HD500's control interface through usbfs (Raspberry
// Pi). The kernel driver is detached from interface 1; the user needs write
// access to the device node (scripts/udev/99-hd500.rules).
func openPedal() (usb.Client, error) {
	c, err := usb.OpenDirectClient(usb.DefaultDirectOptions)
	if err != nil {
		return nil, err
	}
	return c, nil
}
