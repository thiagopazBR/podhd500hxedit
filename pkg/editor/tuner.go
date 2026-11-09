package editor

import (
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"podhd500-edit/pkg/usb"
)

// The pedal's tuner (found 2026-11-06 with HD500_DEBUG=1 while the user
// opened and closed the tuner on the pedal):
//   - pedal-wide STATUS id 0x00: 1 = tuner on, 0 = off (set 0x20 / query 0x21,
//     like Inputs Setup); leaving the tuner is followed by EXP positions and a
//     STATUS 0x2A event;
//   - while it is on, ~5 readings per second, command 0x24, [note, cents]:
//     note = MIDI note - 23 (checked against the pedal's LCD: high E string
//     41 = E4, B string 36 = B3, G string 32 = G3); cents = float32 deviation; negative note words mean
//     no reading (-3 no signal, -2 starting, -4 seen while the note fades).
//
// Tuner settings (pedal-wide, the two knobs of the pedal's tuner screen):
// STATUS 0x01 = output while tuning (1 Mute, 0 Bypass) and 0x02 = reference
// in Hz (425..455, 440 default), found in /api/globals on 2026-11-06.
const (
	GlobalTuner     uint32 = 0x00
	GlobalTunerMute uint32 = 0x01
	GlobalTunerRef  uint32 = 0x02
	MinTunerRef            = 425
	MaxTunerRef            = 455
	cmdTunerReading uint8  = 0x24
	tunerMIDIOffset        = 23
)

// Flats, as HX Edit shows them ("Gb", not "F#").
var noteNames = [12]string{"C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"}

// TunerState is the tuner as shown by the pedal.
type TunerState struct {
	On bool `json:"on"`
	// Signal: a note is being read; Note ("E"), Octave (2) and Cents
	// (-50..+50, 0 = in tune) are only meaningful when Signal is true.
	Signal bool    `json:"signal"`
	Note   string  `json:"note,omitempty"`
	Octave int     `json:"octave,omitempty"`
	Cents  float32 `json:"cents"`
	// Settings: Reference in Hz (425..455) and Mute (output muted while
	// tuning; false = bypass). 0 Reference = not read yet.
	Reference int  `json:"reference"`
	Mute      bool `json:"mute"`
}

// setOn opens/closes the tuner, clearing the reading and keeping the settings.
func (t *TunerState) setOn(on bool) {
	*t = TunerState{On: on, Reference: t.Reference, Mute: t.Mute}
}

// setReading copies a reading, keeping the settings.
func (t *TunerState) setReading(r TunerState) {
	r.Reference, r.Mute = t.Reference, t.Mute
	*t = r
}

// readTunerSettings reads Reference and Mute (status queries work even with
// the tuner open).
func (s *Session) readTunerSettings() {
	ref, okR := s.readGlobal(GlobalTunerRef)
	mute, okM := s.readGlobal(GlobalTunerMute)
	s.update(func(st *State) {
		if okR {
			st.Tuner.Reference = int(ref)
		}
		if okM {
			st.Tuner.Mute = mute != 0
		}
	})
}

// SetTunerReference sets the tuner reference (Hz) and reads it back.
func (s *Session) SetTunerReference(hz int) error {
	if s.client == nil {
		return ErrOffline
	}
	if hz < MinTunerRef || hz > MaxTunerRef {
		return fmt.Errorf("reference %d Hz outside %d..%d", hz, MinTunerRef, MaxTunerRef)
	}
	return s.setTunerGlobal(GlobalTunerRef, uint32(hz))
}

// SetTunerMute sets the tuner output: true Mute, false Bypass.
func (s *Session) SetTunerMute(mute bool) error {
	if s.client == nil {
		return ErrOffline
	}
	v := uint32(0)
	if mute {
		v = 1
	}
	return s.setTunerGlobal(GlobalTunerMute, v)
}

func (s *Session) setTunerGlobal(id, v uint32) error {
	if err := s.send(usb.BuildStatusSet(id, v)); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	got, ok := s.readGlobal(id)
	if !ok {
		return fmt.Errorf("verifying tuner setting 0x%02x: no response", id)
	}
	s.update(func(st *State) { st.applyTunerGlobal(id, got) })
	if got != v {
		return fmt.Errorf("the pedal kept %d (requested %d)", got, v)
	}
	return nil
}

// applyTunerGlobal applies a tuner STATUS value (from the pedal or a read back).
func (st *State) applyTunerGlobal(id, v uint32) {
	switch id {
	case GlobalTuner:
		st.Tuner.setOn(v != 0)
	case GlobalTunerMute:
		st.Tuner.Mute = v != 0
	case GlobalTunerRef:
		st.Tuner.Reference = int(v)
	}
}

// SetTuner turns the pedal's tuner on or off.
func (s *Session) SetTuner(on bool) error {
	if s.client == nil {
		return ErrOffline
	}
	v := uint32(0)
	if on {
		v = 1
	}
	if !on {
		s.leavingTuner.Store(true) // before the pedal's setup 0x68 can arrive
	}
	if err := s.send(usb.BuildStatusSet(GlobalTuner, v)); err != nil {
		s.leavingTuner.Store(false)
		return err
	}
	s.update(func(st *State) { st.Tuner.setOn(on) })
	if !on {
		go s.afterTuner()
	}
	return nil
}

// tunerOpen: the pedal's tuner is open (preset and setlist reads time out).
func (s *Session) tunerOpen() bool { return s.State().Tuner.On }

// ErrTunerOn: the pedal's tuner is open, so presets can't be read.
var ErrTunerOn = errors.New("the pedal's tuner is open: close it to read the presets")

// afterTuner runs when the tuner is closed. Preset reads made while it was
// open failed: re-read the pedal if the first read was blocked by it, and
// let the UI fetch the setlist again (LibraryRev) to fill the missing slots.
func (s *Session) afterTuner() {
	time.Sleep(400 * time.Millisecond) // the pedal leaves the tuner screen first
	// Always re-read, not only when the first read was blocked: leaving the
	// tuner the pedal sends setup 0x68 = 0x7f without changing its buffer
	// and writes the EXP position into EXP-assigned values. Re-reading the
	// buffer and comparing it with the stored slot (Refresh) gives the real
	// dirty state instead of trusting those events.
	s.tunerBlocked.Store(false)
	if err := s.Refresh(); err != nil {
		log.Printf("leitura depois do afinador falhou: %v", err)
	}
	s.leavingTuner.Store(false)
	s.update(func(st *State) { st.Error = ""; st.WaitingTuner = false; st.LibraryRev++ })
}

// tunerReading decodes a 0x24 event: [note, cents].
func tunerReading(note int32, cents float32) TunerState {
	t := TunerState{On: true}
	if note < 0 || math.IsNaN(float64(cents)) {
		return t
	}
	midi := int(note) + tunerMIDIOffset
	t.Signal = true
	t.Note = noteNames[midi%12]
	t.Octave = midi/12 - 1
	t.Cents = cents
	return t
}
