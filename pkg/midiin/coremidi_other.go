//go:build !darwin && !linux

package midiin

import "errors"

var errNoBackend = errors.New("no MIDI input backend for this OS yet (macOS and Linux only)")

// List returns the names of the MIDI inputs.
func List() ([]string, error) { return nil, errNoBackend }

// Open connects to the first MIDI input whose name contains match and calls
// onMsg for every channel message. The returned func closes it.
func Open(match string, onMsg func(Message)) (name string, closeFn func(), err error) {
	return "", nil, errNoBackend
}
