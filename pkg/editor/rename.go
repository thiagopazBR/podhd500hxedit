package editor

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// MaxNameLen is the size of the name field at the start of a preset body.
const MaxNameLen = 16

// CleanName validates a preset name: trimmed, 1..16 chars of printable ASCII
// (the pedal's LCD has no accents or other characters).
func CleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("the name can't be empty")
	}
	if len(name) > MaxNameLen {
		return "", fmt.Errorf("the name has %d characters (maximum %d)", len(name), MaxNameLen)
	}
	for _, r := range name {
		if r < 0x20 || r > 0x7E {
			return "", fmt.Errorf("character %q not supported by the pedal (use unaccented letters, digits and simple symbols)", r)
		}
	}
	return name, nil
}

// Rename changes the name of a preset.
//   - The loaded preset: applied live to the pedal's edit buffer (its LCD
//     shows it at once); the preset becomes dirty, SEND saves it.
//   - Any other slot: changed in the editor only (lib.edited); SEND writes it.
//     Renaming back to the stored name drops the pending change.
func (s *Session) Rename(set, slot int, name string) error {
	if set < 0 || set >= NumSetlists || slot < 0 || slot >= SlotsPerSetlst {
		return fmt.Errorf("invalid position: setlist %d slot %d", set+1, slot)
	}
	name, err := CleanName(name)
	if err != nil {
		return err
	}
	if s.client == nil {
		return ErrOffline
	}
	s.lib.mu.Lock()
	stored := s.lib.slots[set][slot]
	base := s.lib.edited[set][slot]
	s.lib.mu.Unlock()

	// The loaded preset is renamed live on the pedal, unless the editor
	// already holds a pending copy/move for that slot (then that one).
	st := s.State()
	if set == st.Setlist && slot == st.Slot && base == nil {
		return s.renameLoaded(name)
	}
	if stored == nil {
		return errors.New("this setlist hasn't been read from the pedal yet")
	}
	if base == nil {
		base = stored
	}
	p, err := podhd500.ParsePreset(base)
	if err != nil {
		return err
	}
	p.SetName(name)
	s.lib.mu.Lock()
	if bytes.Equal(p.Body[:], stored) {
		s.lib.edited[set][slot] = nil // back to what the pedal has
	} else {
		s.lib.edited[set][slot] = append([]byte(nil), p.Body[:]...)
	}
	s.lib.mu.Unlock()
	s.update(func(st *State) { st.LibraryRev++ })
	return nil
}

// renameLoaded rewrites the name in the pedal's edit buffer: read the buffer,
// change the 16-byte name, apply it back (PRESET_SET to 0xFFFFFFFF), which
// keeps every other unsaved edit.
func (s *Session) renameLoaded(name string) error {
	m, err := s.requestRetry(usb.BuildPresetQuery(usb.CurrentPresetAll, usb.CurrentSetAll), usb.CmdPresetLoad, 3*time.Second)
	if err != nil {
		return fmt.Errorf("reading the loaded preset: %w", err)
	}
	p, err := podhd500.ParsePreset(m.Payload)
	if err != nil {
		return err
	}
	if p.Name() == name {
		return nil
	}
	p.SetName(name)
	frame, err := usb.BuildPresetSet(usb.CurrentPresetAll, usb.CurrentSetAll, p.Body[:])
	if err != nil {
		return err
	}
	ack, err := s.request(frame, usb.CmdPresetSetAck, 5*time.Second)
	if err != nil {
		return err
	}
	if ack.Word(0) != 0 {
		return fmt.Errorf("the pedal rejected the name (status %d)", ack.Word(0))
	}
	s.setBuffer(p.Body[:])
	s.update(func(st *State) { st.PresetName = name; st.Dirty = true })
	return nil
}
