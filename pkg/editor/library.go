package editor

import (
	"fmt"
	"sync"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// Pedal memory layout.
const (
	NumSetlists    = 8
	SlotsPerSetlst = 64
)

// Progress describes a running operation: reading a setlist, or a SEND /
// RECEIVE. Label is the UI text ("Recebendo presets", "Saving to the pedal").
type Progress struct {
	Setlist int    `json:"setlist"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Label   string `json:"label,omitempty"`
}

// library caches what was read from the pedal's memory: setlist names and
// the stored body of every slot (kept for SEND/RECEIVE and the edited mark).
type library struct {
	mu          sync.Mutex
	names       [NumSetlists]string
	namesLoaded bool
	slots       [NumSetlists][SlotsPerSetlst][]byte // stored on the pedal; nil = not read yet
	loaded      [NumSetlists]bool
	loading     [NumSetlists]chan struct{} // closed when that setlist finishes
	// edited holds presets changed in the editor and not yet sent (renames,
	// moves, imported files…); nil = no pending change for that slot.
	edited [NumSetlists][SlotsPerSetlst][]byte
	// editedName holds setlist renames not yet sent ("" = none).
	editedName [NumSetlists]string
}

// SetlistInfo is one entry of the setlist selector.
type SetlistInfo struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`             // the editor's name (pending rename included)
	Stored string `json:"stored"`           // the name on the pedal
	Edited bool   `json:"edited,omitempty"` // renamed in the editor, not sent yet
}

// Setlists returns the 8 setlist names, reading them from the pedal once.
func (s *Session) Setlists() ([]SetlistInfo, error) {
	s.lib.mu.Lock()
	loaded := s.lib.namesLoaded
	s.lib.mu.Unlock()
	if !loaded && s.tunerOpen() {
		return nil, ErrTunerOn // the UI asks again when the tuner closes (LibraryRev)
	}
	if !loaded {
		var names [NumSetlists]string
		for i := 0; i < NumSetlists; i++ {
			m, err := s.requestRetry(usb.BuildSetQuery(uint32(i)), usb.CmdSetLoad, 2*time.Second)
			if err != nil {
				return nil, fmt.Errorf("setlist %d name: %w", i+1, err)
			}
			names[i] = usb.ParseSetName(m)
		}
		s.lib.mu.Lock()
		s.lib.names, s.lib.namesLoaded = names, true
		s.lib.mu.Unlock()
	}
	s.lib.mu.Lock()
	defer s.lib.mu.Unlock()
	out := make([]SetlistInfo, NumSetlists)
	for i := range out {
		out[i] = SetlistInfo{Index: i, Name: s.lib.names[i], Stored: s.lib.names[i]}
		if e := s.lib.editedName[i]; e != "" {
			out[i].Name, out[i].Edited = e, true
		}
	}
	return out, nil
}

// SetlistView is the content of one setlist as far as it is known.
type SetlistView struct {
	Setlist int      `json:"setlist"`
	Loaded  bool     `json:"loaded"`
	Names   []string `json:"names"`  // "" for slots not read yet
	Edited  []bool   `json:"edited"` // changed in the editor, not sent yet
}

// Setlist returns what is cached for setlist set and, if it was never read,
// starts reading it in the background (progress via State.Loading).
func (s *Session) Setlist(set int) (SetlistView, error) {
	if set < 0 || set >= NumSetlists {
		return SetlistView{}, fmt.Errorf("invalid setlist %d", set+1)
	}
	if s.cli() == nil {
		return SetlistView{}, ErrOffline
	}
	s.lib.mu.Lock()
	v := SetlistView{Setlist: set, Loaded: s.lib.loaded[set], Names: make([]string, SlotsPerSetlst), Edited: make([]bool, SlotsPerSetlst)}
	for i, b := range s.lib.slots[set] {
		if e := s.lib.edited[set][i]; e != nil {
			b = e // show the editor's version
			v.Edited[i] = true
		}
		if b != nil {
			if p, err := podhd500.ParsePreset(b); err == nil {
				v.Names[i] = p.Name()
			}
		}
	}
	// Not with the tuner open (the pedal ignores preset reads): afterTuner
	// bumps LibraryRev, the UI asks again and the load starts then.
	start := !v.Loaded && s.lib.loading[set] == nil && !s.tunerOpen()
	if start {
		s.lib.loading[set] = make(chan struct{})
	}
	s.lib.mu.Unlock()
	if start {
		go s.loadSetlist(set)
	}
	return v, nil
}

// loadSetlist reads the 64 stored presets of a setlist, one PRESET_QUERY
// each (the pedal has no names-only query), publishing progress.
func (s *Session) loadSetlist(set int) {
	defer func() {
		s.lib.mu.Lock()
		close(s.lib.loading[set])
		s.lib.loading[set] = nil
		s.lib.mu.Unlock()
		s.update(func(st *State) { st.Loading = nil; st.LibraryRev++ })
	}()
	failed := 0
	for slot := 0; slot < SlotsPerSetlst; slot++ {
		if s.tunerOpen() {
			// Opened mid-load: stop (slots read so far are kept) and resume
			// when it closes, instead of timing out on every slot.
			return
		}
		s.lib.mu.Lock()
		cached := s.lib.slots[set][slot] != nil // read before the tuner interrupted
		s.lib.mu.Unlock()
		if cached {
			continue
		}
		s.update(func(st *State) {
			st.Loading = &Progress{Setlist: set, Done: slot, Total: SlotsPerSetlst, Label: "Receiving presets"}
		})
		m, err := s.requestRetry(usb.BuildPresetQuery(uint16(slot), uint16(set)), usb.CmdPresetLoad, 3*time.Second)
		if err != nil || len(m.Payload) != podhd500.PresetBodySize {
			failed++
			continue
		}
		body := append([]byte(nil), m.Payload...)
		s.lib.mu.Lock()
		s.lib.slots[set][slot] = body
		s.lib.mu.Unlock()
		if slot%8 == 7 { // let the UI fill in names as they arrive
			s.update(func(st *State) { st.LibraryRev++ })
		}
	}
	s.lib.mu.Lock()
	s.lib.loaded[set] = failed == 0
	s.lib.mu.Unlock()
	if failed > 0 {
		s.update(func(st *State) { st.Error = fmt.Sprintf("%d presets in setlist %d didn't respond", failed, set+1) })
	}
}

// requestRetry is request with one retry on timeout (the reference editor
// does the same: the pedal occasionally drops a query while busy).
func (s *Session) requestRetry(spsi []byte, want uint8, timeout time.Duration) (*usb.SPSIMessage, error) {
	m, err := s.request(spsi, want, timeout)
	if err != nil && err != ErrOffline {
		m, err = s.request(spsi, want, timeout)
	}
	return m, err
}

// cacheSlot stores a body read for (set, slot), e.g. the PRESET_LOAD the
// pedal pushes right after a preset is selected (= the stored version).
func (s *Session) cacheSlot(set, slot int, body []byte) {
	if set < 0 || set >= NumSetlists || slot < 0 || slot >= SlotsPerSetlst {
		return
	}
	s.lib.mu.Lock()
	s.lib.slots[set][slot] = append([]byte(nil), body...)
	s.lib.mu.Unlock()
}

// cachedName returns the stored name of (set, slot) if that slot was read.
func (s *Session) cachedName(set, slot int) (string, bool) {
	if set < 0 || set >= NumSetlists || slot < 0 || slot >= SlotsPerSetlst {
		return "", false
	}
	s.lib.mu.Lock()
	b := s.lib.slots[set][slot]
	s.lib.mu.Unlock()
	if b == nil {
		return "", false
	}
	p, err := podhd500.ParsePreset(b)
	if err != nil {
		return "", false
	}
	return p.Name(), true
}

// Select loads a stored preset on the pedal (like a double-click in the
// original editor): switches setlist if needed, then the slot. The pedal
// answers with SET/PRESET_SELECTED and pushes the preset (PRESET_LOAD),
// which update the state through onEvent.
func (s *Session) Select(set, slot int) error {
	if set < 0 || set >= NumSetlists || slot < 0 || slot >= SlotsPerSetlst {
		return fmt.Errorf("invalid position: setlist %d slot %d", set+1, slot)
	}
	if s.cli() == nil {
		return ErrOffline
	}
	if s.State().Setlist != set {
		if err := s.send(usb.BuildStatusSet(usb.StatusIDCurrentSet, uint32(set))); err != nil {
			return err
		}
		time.Sleep(250 * time.Millisecond) // as refs/hd500-editor: let the pedal switch setlist first
	}
	s.markSelected()
	return s.send(usb.BuildStatusSet(usb.StatusIDCurrentPreset, uint32(slot)))
}
