package editor

import (
	"errors"
	"fmt"
	"time"

	"podhd500-edit/pkg/usb"
)

// Setlist names. Like preset changes, a rename (typed, or from an imported
// .h5s / .h5b) stays pending in the editor (lib.editedName, asterisk) until
// SEND › Set List / All writes it with SET_NAME (0x2A → ack 0x2B) and reads
// it back (SET_QUERY 0x28 → SET_LOAD 0x29). RECEIVE › Set List / All drops it.

// RenameSetlist changes the editor's name for a setlist (pending until SEND).
// Renaming back to the pedal's name drops the pending change.
func (s *Session) RenameSetlist(set int, name string) error {
	if set < 0 || set >= NumSetlists {
		return fmt.Errorf("invalid setlist %d", set+1)
	}
	name, err := CleanName(name)
	if err != nil {
		return err
	}
	if s.client == nil {
		return ErrOffline
	}
	if _, err := s.Setlists(); err != nil { // the pedal's names
		return err
	}
	s.setPendingName(set, name)
	s.update(func(st *State) { st.LibraryRev++ })
	return nil
}

func (s *Session) setPendingName(set int, name string) {
	s.lib.mu.Lock()
	defer s.lib.mu.Unlock()
	if name == s.lib.names[set] {
		s.lib.editedName[set] = ""
	} else {
		s.lib.editedName[set] = name
	}
}

// writeSetlistName stores a setlist name on the pedal and verifies it.
func (s *Session) writeSetlistName(set int, name string) error {
	ack, err := s.request(usb.BuildSetName(uint32(set), name), usb.CmdSetNameAck, 5*time.Second)
	if err != nil {
		return err
	}
	if ack.Word(0) != 0 {
		return fmt.Errorf("the pedal rejected the name (status %d)", ack.Word(0))
	}
	var got string
	for try := 0; try < 3; try++ { // the pedal may still be syncing
		time.Sleep(300 * time.Millisecond)
		m, err := s.requestRetry(usb.BuildSetQuery(uint32(set)), usb.CmdSetLoad, 2*time.Second)
		if err != nil {
			return fmt.Errorf("verification: %w", err)
		}
		if got = usb.ParseSetName(m); got == name {
			s.lib.mu.Lock()
			s.lib.names[set] = name
			s.lib.editedName[set] = ""
			s.lib.mu.Unlock()
			if s.State().Setlist == set {
				s.update(func(st *State) { st.SetlistName = name })
			}
			return nil
		}
	}
	return errors.New("verification: the pedal has the name " + fmt.Sprintf("%q", got))
}
