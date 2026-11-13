//go:build linux

package midiin

// Linux (Raspberry Pi) MIDI input: the ALSA raw MIDI device nodes
// /dev/snd/midiC<card>D<dev>, read as a plain byte stream (pure Go, no
// libasound). The user needs the "audio" group. Not verified on hardware yet
// (written 2026-11-13).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var nodeRe = regexp.MustCompile(`midiC(\d+)D(\d+)$`)

// cardNames maps a card number to its name from /proc/asound/cards, whose
// first line per card looks like " 1 [MIDI           ]: USB-Audio - USB2.0-MIDI".
func cardNames() map[string]string {
	out := map[string]string{}
	b, _ := os.ReadFile("/proc/asound/cards")
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || !strings.Contains(line, "[") {
			continue
		}
		name := strings.TrimSpace(line[strings.Index(line, "]:")+2:])
		if i := strings.Index(name, " - "); i >= 0 {
			name = name[i+3:]
		}
		out[f[0]] = name
	}
	return out
}

type input struct{ node, name string }

func inputs() []input {
	nodes, _ := filepath.Glob("/dev/snd/midiC*D*")
	names := cardNames()
	var out []input
	for _, n := range nodes {
		m := nodeRe.FindStringSubmatch(n)
		if m == nil {
			continue
		}
		out = append(out, input{n, fmt.Sprintf("%s (midiC%sD%s)", names[m[1]], m[1], m[2])})
	}
	return out
}

// List returns the names of the MIDI inputs.
func List() ([]string, error) {
	var out []string
	for _, in := range inputs() {
		out = append(out, in.name)
	}
	return out, nil
}

// Open connects to the first MIDI input whose name contains match (case
// insensitive) and calls onMsg for every channel message. The returned func
// closes it.
func Open(match string, onMsg func(Message)) (name string, closeFn func(), err error) {
	for _, in := range inputs() {
		if !strings.Contains(strings.ToLower(in.name), strings.ToLower(match)) {
			continue
		}
		fd, err := syscall.Open(in.node, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != nil {
			return "", nil, fmt.Errorf("open %s: %w (is the user in the audio group?)", in.node, err)
		}
		var stop atomic.Bool
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer syscall.Close(fd)
			buf := make([]byte, 256)
			for !stop.Load() {
				n, err := syscall.Read(fd, buf)
				if n > 0 {
					for _, m := range Parse(buf[:n]) {
						onMsg(m)
					}
					continue
				}
				if err != nil && err != syscall.EAGAIN && err != syscall.EINTR {
					return // device gone
				}
				time.Sleep(3 * time.Millisecond)
			}
		}()
		return in.name, func() { stop.Store(true); <-done }, nil
	}
	return "", nil, errors.New("no MIDI input matches " + match)
}
