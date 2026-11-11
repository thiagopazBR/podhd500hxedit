package editor

import (
	"fmt"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// CmdMIDIAssign: the pedal reports a MIDI Assign edit (MOVE held > MIDI
// ASSIGN) as Cmd 0x34 [0, control, msgType, channel, number, value, b5, 0]
// (found 2026-11-08 in the debug log); the same fields are 6 bytes per control
// in the preset at 0xD84 (see podhd500 MIDI assign).
const CmdMIDIAssign uint8 = 0x34

// CmdDSPOver: the pedal reports "DSP limit reached" (empty payload) when a
// model doesn't fit (found 2026-11-08, user forced it on the pedal).
const CmdDSPOver uint8 = 0x33

// SendMIDIAssignRaw sends one control's six MIDI Assign bytes (debugging).
func (s *Session) SendMIDIAssignRaw(control int, f [6]byte) error {
	if s.cli() == nil {
		return ErrOffline
	}
	if control < 0 || control > 12 {
		return fmt.Errorf("invalid control %d", control)
	}
	return s.send(usb.BuildFrame(CmdMIDIAssign, 0, uint32(control), uint32(f[0]), uint32(f[1]), uint32(f[2]), uint32(f[3]), uint32(f[4]), uint32(f[5])))
}

// MIDIAssignChange: the fields to change (nil = keep).
type MIDIAssignChange struct {
	Message, Channel, Number, Value, Max *uint8
}

// SetMIDIAssign changes one control's MIDI Assign in the edit buffer: sent
// as the pedal reports its own edits (Cmd 0x34 with all six bytes),
// LCD-verified 2026-11-08.
func (s *Session) SetMIDIAssign(control int, c MIDIAssignChange) error {
	if s.cli() == nil {
		return ErrOffline
	}
	if control < 0 || control >= podhd500.MIDIAssignCount {
		return fmt.Errorf("invalid control %d (0..12)", control)
	}
	if s.BufferCopy() == nil {
		if err := s.loadBuffer(); err != nil {
			return err
		}
	}
	p, err := podhd500.ParsePreset(s.BufferCopy())
	if err != nil {
		return err
	}
	a := p.MIDIAssigns()[control]
	set := func(dst *uint8, v *uint8, max uint8) error {
		if v == nil {
			return nil
		}
		if *v > max {
			return fmt.Errorf("value %d out of range (max %d)", *v, max)
		}
		*dst = *v
		return nil
	}
	if c.Channel != nil && *c.Channel > 15 && *c.Channel != podhd500.MIDIChannelNone && *c.Channel != podhd500.MIDIChannelBase {
		return fmt.Errorf("invalid channel %d", *c.Channel)
	}
	for _, f := range []struct {
		dst *uint8
		v   *uint8
		max uint8
	}{{&a.Message, c.Message, 4}, {&a.Channel, c.Channel, 0x11}, {&a.Number, c.Number, 127}, {&a.Value, c.Value, 127}, {&a.Max, c.Max, 127}} {
		if err := set(f.dst, f.v, f.max); err != nil {
			return err
		}
	}
	b := a.Bytes()
	if err := s.SendMIDIAssignRaw(control, b); err != nil {
		return err
	}
	s.editBuffer(func(p *podhd500.Preset) bool { return p.SetMIDIAssign(control, b) })
	s.update(func(st *State) { st.Dirty = true })
	return nil
}

// stuckFX: a block the pedal couldn't run, or left with an invalid type.
func stuckFX(it podhd500.PresetItem) bool {
	return it.Kind == "fx" && (it.DSPOver || it.Type == 0xFFFFFFFF)
}
