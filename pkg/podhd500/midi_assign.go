package podhd500

// MIDI Assign (MOVE held > MIDI ASSIGN on the pedal; the original editor's
// MIDI ASSIGN pane): what each footswitch / pedal SENDS on MIDI OUT, stored
// per preset. 13 controls × 6 bytes from 0xD84. The storage order is the order
// of the LCD's MIDI ASSIGN screen read row by row: FS1–FS4, LOOPER, EXP TOE,
// FS5–FS8, TAP, EXP 1, EXP 2 (CORRECTED 2026-11-10: it was FS1–FS8, LOOPER,
// TAP, EXP TOE, ...). Verified on the pedal by correlating the 0x34 events with
// webcam photos of the LCD (FS2/FS3 note+velocity, LPR CC 99, EXP TOE channel),
// and by the defaults (EXP TOE = CC 59 at index 5, TAP = CC 64 at index 10).
// Byte layout learned 2026-11-08 by diffing the edit buffer while the user
// edited it on the pedal:
//   byte 0  message (switches): 0 CC Toggle, 1 CC, 2 Prog Change, 3 Bank Change, 4 Note On
//   byte 1  channel: 0x10 None (control sends nothing), 0x11 Base, 0..15 = 1..16
//   byte 2  CC # / program / bank / note
//   byte 3  value / velocity; pedals: Min Value
//   byte 4  pedals: Max Value
//   byte 5  unknown (kept)

const (
	MIDIAssignOffset = 0xD84
	MIDIAssignSize   = 6
	MIDIAssignCount  = 13
	MIDIChannelNone  = 0x10
	MIDIChannelBase  = 0x11
)

// MIDIControlNames, in storage order (= the LCD screen, row by row).
var MIDIControlNames = [MIDIAssignCount]string{"FS1", "FS2", "FS3", "FS4", "Looper", "EXP Toe", "FS5", "FS6", "FS7", "FS8", "Tap", "EXP 1", "EXP 2"}

// MIDIMessageOptions (switches) and MIDIChannelOptions in the LCD's order.
var (
	MIDIMessageOptions = []Option{{0, "CC Toggle"}, {1, "CC"}, {2, "Prog Change"}, {3, "Bank Change"}, {4, "Note On"}}
	MIDIChannelOptions = func() []Option {
		o := []Option{{MIDIChannelNone, "None"}, {MIDIChannelBase, "Base"}}
		for c := 0; c < 16; c++ {
			o = append(o, Option{uint32(c), itoa2(c + 1)})
		}
		return o
	}()
)

func itoa2(n int) string { return string([]byte{byte('0' + n/10), byte('0' + n%10)}) }

// MIDIAssign is one control's MIDI Assign.
type MIDIAssign struct {
	Control int    `json:"control"`
	Name    string `json:"name"`
	Pedal   bool   `json:"pedal"` // EXP 1/2: CC with Min/Max instead of a message
	Message uint8  `json:"message"`
	Channel uint8  `json:"channel"`
	Number  uint8  `json:"number"`
	Value   uint8  `json:"value"` // pedals: Min Value
	Max     uint8  `json:"max"`   // pedals only
	Extra   uint8  `json:"extra"` // byte 5, unknown
}

// Bytes returns the six stored bytes.
func (a MIDIAssign) Bytes() [MIDIAssignSize]byte {
	return [MIDIAssignSize]byte{a.Message, a.Channel, a.Number, a.Value, a.Max, a.Extra}
}

// MIDIAssigns reads the 13 controls.
func (p *Preset) MIDIAssigns() []MIDIAssign {
	out := make([]MIDIAssign, MIDIAssignCount)
	for i := range out {
		o := MIDIAssignOffset + i*MIDIAssignSize
		b := p.Body[o : o+MIDIAssignSize]
		out[i] = MIDIAssign{Control: i, Name: MIDIControlNames[i], Pedal: i >= 11,
			Message: b[0], Channel: b[1], Number: b[2], Value: b[3], Max: b[4], Extra: b[5]}
	}
	return out
}

// SetMIDIAssign stores one control's six bytes; reports a change.
func (p *Preset) SetMIDIAssign(control int, b [MIDIAssignSize]byte) bool {
	if control < 0 || control >= MIDIAssignCount {
		return false
	}
	o := MIDIAssignOffset + control*MIDIAssignSize
	changed := false
	for i, v := range b {
		if p.Body[o+i] != v {
			p.Body[o+i] = v
			changed = true
		}
	}
	return changed
}
