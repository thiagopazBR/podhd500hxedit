//go:build darwin

package main

import "podhd500-edit/pkg/usb"

// openPedal opens the POD HD500's control interface (macOS: IOKit; the
// l6-rebind daemon must have freed it, see the README).
func openPedal() (usb.Client, error) {
	c, err := usb.OpenDirectClient(usb.DefaultDirectOptions)
	if err != nil {
		return nil, err
	}
	return c, nil
}
