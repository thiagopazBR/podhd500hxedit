package editor

import (
	"fmt"
	"time"

	"podhd500-edit/pkg/usb"
)

// Setup > MIDI/Tempo > MIDI Channel (pedal-wide; the channel MIDI IN listens
// on, so the same for every assignment's CC). Found 2026-11-08 in the debug
// log while the user changed it on the pedal: STATUS 0x0D = channel 1..16;
// the pedal also reports 0x0C = channel − 1.
const (
	GlobalMIDIChannel     uint32 = 0x0D
	globalMIDIChannelZero uint32 = 0x0C
	MinMIDIChannel               = 1
	MaxMIDIChannel               = 16
)

// SetMIDIChannel writes the pedal's MIDI channel and reads it back.
func (s *Session) SetMIDIChannel(ch uint32) error {
	if s.client == nil {
		return ErrOffline
	}
	if ch < MinMIDIChannel || ch > MaxMIDIChannel {
		return fmt.Errorf("MIDI channel %d outside %d..%d", ch, MinMIDIChannel, MaxMIDIChannel)
	}
	// The pedal reports both ids; writing 0x0D alone is read back but the
	// LCD keeps the old channel: 0x0C (channel − 1) is written first.
	if err := s.send(usb.BuildStatusSet(globalMIDIChannelZero, ch-1)); err != nil {
		return err
	}
	time.Sleep(20 * time.Millisecond)
	if err := s.send(usb.BuildStatusSet(GlobalMIDIChannel, ch)); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	got, ok := s.readGlobal(GlobalMIDIChannel)
	if !ok {
		return fmt.Errorf("verifying the MIDI channel: no answer")
	}
	s.update(func(st *State) { st.MIDIChannel = got })
	if got != ch {
		return fmt.Errorf("the pedal kept MIDI channel %d (requested %d)", got, ch)
	}
	return nil
}
