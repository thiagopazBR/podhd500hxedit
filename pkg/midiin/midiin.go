// Package midiin reads MIDI messages from a MIDI input of this computer. The
// POD HD500's footswitches send them on its MIDI OUT (the MIDI Assign of the
// preset), through a USB-MIDI interface, and the snapshot player turns them
// into snapshot changes. macOS (CoreMIDI) works; the Linux (ALSA) backend is
// still to be written.
package midiin

import "fmt"

// Debug logs the raw packet bytes of the input (stderr).
var Debug bool

// Kind of a channel message.
type Kind uint8

const (
	NoteOff Kind = 0x80
	NoteOn  Kind = 0x90
	Control Kind = 0xB0 // control change (CC)
	Program Kind = 0xC0 // program change
)

// Message is a MIDI channel message.
type Message struct {
	Kind    Kind
	Channel int // 1..16
	Data1   int // note, CC number or program
	Data2   int // velocity or CC value (0 for a program change)
}

func (m Message) String() string {
	switch m.Kind {
	case Control:
		return fmt.Sprintf("CC %d = %d (ch %d)", m.Data1, m.Data2, m.Channel)
	case Program:
		return fmt.Sprintf("PC %d (ch %d)", m.Data1, m.Channel)
	case NoteOn:
		return fmt.Sprintf("Note On %d vel %d (ch %d)", m.Data1, m.Data2, m.Channel)
	case NoteOff:
		return fmt.Sprintf("Note Off %d (ch %d)", m.Data1, m.Channel)
	}
	return fmt.Sprintf("0x%02X %d %d (ch %d)", uint8(m.Kind), m.Data1, m.Data2, m.Channel)
}

// Parse extracts the channel messages of a raw MIDI byte stream (one packet).
// It handles running status and skips system messages (clock, sysex, ...).
func Parse(b []byte) []Message {
	var out []Message
	var status byte
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c >= 0xF8: // real-time: single byte, may sit between data bytes
			i++
			continue
		case c >= 0xF0: // system common / sysex: skip to the next status byte
			status = 0
			i++
			for i < len(b) && b[i] < 0x80 {
				i++
			}
			continue
		case c >= 0x80:
			status = c
			i++
			continue
		}
		// data byte: needs a running status
		if status == 0 {
			i++
			continue
		}
		kind := Kind(status & 0xF0)
		need := 2
		if kind == Program || kind == 0xD0 { // program change, channel pressure
			need = 1
		}
		if i+need > len(b) {
			break
		}
		m := Message{Kind: kind, Channel: int(status&0x0F) + 1, Data1: int(b[i])}
		if need == 2 {
			m.Data2 = int(b[i+1])
		}
		out = append(out, m)
		i += need
	}
	return out
}

// SnapshotFor maps a message to a snapshot number (1..4), or 0 when it is not
// a snapshot trigger: a control change with value > 0, number cc..cc+3, on
// channel ch (0 = any channel).
func SnapshotFor(m Message, cc, ch int) int {
	if m.Kind != Control || m.Data2 == 0 {
		return 0
	}
	if ch != 0 && m.Channel != ch {
		return 0
	}
	if n := m.Data1 - cc + 1; n >= 1 && n <= 4 {
		return n
	}
	return 0
}
