package editor

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// SEND / RECEIVE, like the Save Controls of the original editor:
//   - SEND writes presets to the pedal's memory (flash). Unlike the original,
//     only presets that actually changed are written: the loaded preset when
//     its edit buffer is dirty, and presets edited in the editor (lib.edited).
//     Rewriting identical presets would take minutes and wear the flash.
//   - RECEIVE re-reads stored presets from the pedal, discarding the editor's
//     pending changes (and the pedal's edit buffer for the loaded preset).
// Every write is verified by reading the slot back (as send-test 6).

// Transfer scopes.
const (
	ScopeSelected = "selected"
	ScopeSetlist  = "setlist"
	ScopeAll      = "all"
)

// TransferRequest selects what to send / receive.
type TransferRequest struct {
	Scope   string `json:"scope"`
	Setlist int    `json:"setlist"` // for selected / setlist
	Slots   []int  `json:"slots"`   // for selected
}

// TransferResult summarizes what happened.
type TransferResult struct {
	Done    int      `json:"done"`    // written (SEND) or read (RECEIVE)
	Nothing bool     `json:"nothing"` // SEND found no changed preset in scope
	Failed  []string `json:"failed,omitempty"`
}

// ErrBusy is returned when another SEND / RECEIVE is running.
var ErrBusy = errors.New("another send/receive operation is in progress")

type slotRef struct{ set, slot int }

func (r slotRef) label() string {
	return fmt.Sprintf("Setlist %d %s", r.set+1, podhd500.SlotLabel(r.slot))
}

func (s *Session) targets(req TransferRequest) ([]slotRef, error) {
	var out []slotRef
	switch req.Scope {
	case ScopeSelected:
		if req.Setlist < 0 || req.Setlist >= NumSetlists {
			return nil, fmt.Errorf("invalid setlist %d", req.Setlist+1)
		}
		if len(req.Slots) == 0 {
			return nil, errors.New("no preset selected")
		}
		seen := map[int]bool{}
		for _, sl := range req.Slots {
			if sl < 0 || sl >= SlotsPerSetlst {
				return nil, fmt.Errorf("invalid slot %d", sl)
			}
			if !seen[sl] {
				seen[sl] = true
				out = append(out, slotRef{req.Setlist, sl})
			}
		}
	case ScopeSetlist:
		if req.Setlist < 0 || req.Setlist >= NumSetlists {
			return nil, fmt.Errorf("invalid setlist %d", req.Setlist+1)
		}
		for sl := 0; sl < SlotsPerSetlst; sl++ {
			out = append(out, slotRef{req.Setlist, sl})
		}
	case ScopeAll:
		for set := 0; set < NumSetlists; set++ {
			for sl := 0; sl < SlotsPerSetlst; sl++ {
				out = append(out, slotRef{set, sl})
			}
		}
	default:
		return nil, fmt.Errorf("invalid scope %q", req.Scope)
	}
	return out, nil
}

func (s *Session) progress(label string, set, done, total int) {
	s.update(func(st *State) { st.Loading = &Progress{Setlist: set, Done: done, Total: total, Label: label} })
}

// Send writes the changed presets in scope to the pedal's memory.
func (s *Session) Send(req TransferRequest) (TransferResult, error) {
	var res TransferResult
	if s.cli() == nil {
		return res, ErrOffline
	}
	refs, err := s.targets(req)
	if err != nil {
		return res, err
	}
	if !s.opMu.TryLock() {
		return res, ErrBusy
	}
	defer s.opMu.Unlock()
	defer s.update(func(st *State) { st.Loading = nil; st.LibraryRev++ })

	st := s.State()
	type job struct {
		ref        slotRef
		body       []byte
		current    bool // the slot loaded on the pedal
		fromBuffer bool // body is the pedal's edit buffer (else an editor change)
	}
	var jobs []job
	for _, r := range refs {
		s.lib.mu.Lock()
		e := s.lib.edited[r.set][r.slot]
		s.lib.mu.Unlock()
		isCur := r.set == st.Setlist && r.slot == st.Slot
		switch {
		case isCur && st.Dirty:
			// The pedal's edit buffer holds the changes: save it (it already
			// includes anything applied to it from the editor).
			m, err := s.requestRetry(usb.BuildPresetQuery(usb.CurrentPresetAll, usb.CurrentSetAll), usb.CmdPresetLoad, 3*time.Second)
			if err != nil {
				return res, fmt.Errorf("reading the loaded preset: %w", err)
			}
			jobs = append(jobs, job{r, append([]byte(nil), m.Payload...), true, true})
		case e != nil:
			jobs = append(jobs, job{r, e, isCur, false})
		}
	}
	// Pending setlist renames go with SEND › Set List / All.
	type nameJob struct {
		set  int
		name string
	}
	var names []nameJob
	if req.Scope != ScopeSelected {
		s.lib.mu.Lock()
		for set := 0; set < NumSetlists; set++ {
			if n := s.lib.editedName[set]; n != "" && (req.Scope == ScopeAll || set == req.Setlist) {
				names = append(names, nameJob{set, n})
			}
		}
		s.lib.mu.Unlock()
	}
	if len(jobs) == 0 && len(names) == 0 {
		res.Nothing = true
		return res, nil
	}
	total := len(jobs) + len(names)
	for i, n := range names {
		s.progress("Saving setlist name", n.set, i, total)
		if err := s.writeSetlistName(n.set, n.name); err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("setlist %d name: %v", n.set+1, err))
			continue
		}
		res.Done++
	}
	for i, j := range jobs {
		s.progress("Saving to the pedal", j.ref.set, len(names)+i, total)
		if err := s.writeSlotVerified(j.ref.set, j.ref.slot, j.body); err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", j.ref.label(), err))
			continue
		}
		res.Done++
		s.lib.mu.Lock()
		s.lib.edited[j.ref.set][j.ref.slot] = nil
		s.lib.mu.Unlock()
		switch {
		case j.current && j.fromBuffer:
			s.update(func(st *State) { st.Dirty = false })
		case j.current:
			// A copy/move from the editor replaced the loaded slot: reload it
			// so the pedal plays what is now stored there.
			if err := s.reloadCurrent(j.ref.set, j.ref.slot); err != nil {
				res.Failed = append(res.Failed, j.ref.label()+": reloading on the pedal: "+err.Error())
			}
		}
	}
	return res, nil
}

// writeSlotVerified stores body in (set, slot), waits for the pedal to
// finish ("SYNC IN PROGRESS") and reads it back byte for byte.
func (s *Session) writeSlotVerified(set, slot int, body []byte) error {
	frame, err := usb.BuildPresetSet(uint16(slot), uint16(set), body)
	if err != nil {
		return err
	}
	ack, err := s.request(frame, usb.CmdPresetSetAck, 10*time.Second)
	if err != nil {
		return err
	}
	if ack.Word(0) != 0 {
		return fmt.Errorf("the pedal rejected the write (status %d)", ack.Word(0))
	}
	time.Sleep(usb.PresetSyncSettle)
	m, err := s.requestRetry(usb.BuildPresetQuery(uint16(slot), uint16(set)), usb.CmdPresetLoad, 3*time.Second)
	if err != nil {
		return fmt.Errorf("verification: %w", err)
	}
	if !bytes.Equal(m.Payload, body) {
		return errors.New("verification: the slot read back differs from what was written")
	}
	s.cacheSlot(set, slot, body)
	return nil
}

// Receive re-reads the stored presets in scope, discarding the editor's
// pending changes. If the loaded preset is in scope and edited on the pedal,
// it is reloaded too (the caller confirmed discarding those edits).
func (s *Session) Receive(req TransferRequest) (TransferResult, error) {
	var res TransferResult
	if s.cli() == nil {
		return res, ErrOffline
	}
	refs, err := s.targets(req)
	if err != nil {
		return res, err
	}
	if !s.opMu.TryLock() {
		return res, ErrBusy
	}
	defer s.opMu.Unlock()
	defer s.update(func(st *State) { st.Loading = nil; st.LibraryRev++ })

	switch req.Scope { // setlist names too
	case ScopeAll:
		s.lib.mu.Lock()
		s.lib.namesLoaded = false
		s.lib.editedName = [NumSetlists]string{}
		s.lib.mu.Unlock()
		if _, err := s.Setlists(); err != nil {
			res.Failed = append(res.Failed, err.Error())
		}
	case ScopeSetlist:
		s.lib.mu.Lock()
		s.lib.editedName[req.Setlist] = ""
		s.lib.mu.Unlock()
		if m, err := s.requestRetry(usb.BuildSetQuery(uint32(req.Setlist)), usb.CmdSetLoad, 2*time.Second); err == nil {
			s.lib.mu.Lock()
			s.lib.names[req.Setlist] = usb.ParseSetName(m)
			s.lib.mu.Unlock()
		} else {
			res.Failed = append(res.Failed, fmt.Sprintf("setlist %d name", req.Setlist+1))
		}
	}
	st := s.State()
	reloadCurrent := false
	failedSets := map[int]bool{}
	for i, r := range refs {
		s.progress("Receiving from the pedal", r.set, i, len(refs))
		m, err := s.requestRetry(usb.BuildPresetQuery(uint16(r.slot), uint16(r.set)), usb.CmdPresetLoad, 3*time.Second)
		if err != nil || len(m.Payload) != podhd500.PresetBodySize {
			res.Failed = append(res.Failed, r.label())
			failedSets[r.set] = true
			continue
		}
		s.cacheSlot(r.set, r.slot, m.Payload)
		s.lib.mu.Lock()
		s.lib.edited[r.set][r.slot] = nil
		s.lib.mu.Unlock()
		res.Done++
		if r.set == st.Setlist && r.slot == st.Slot && st.Dirty {
			reloadCurrent = true
		}
	}
	if req.Scope != ScopeSelected {
		s.lib.mu.Lock()
		for _, r := range refs {
			s.lib.loaded[r.set] = !failedSets[r.set]
		}
		s.lib.mu.Unlock()
	}
	if reloadCurrent {
		if err := s.reloadCurrent(st.Setlist, st.Slot); err != nil {
			res.Failed = append(res.Failed, "reloading the current preset: "+err.Error())
		}
	}
	return res, nil
}

// reloadCurrent discards the pedal's edit buffer by selecting the slot
// again. If the pedal ignores re-selecting the same slot, it goes to the
// neighbouring slot and back (as send-test -reload).
func (s *Session) reloadCurrent(set, slot int) error {
	s.markSelected()
	if err := s.send(usb.BuildStatusSet(usb.StatusIDCurrentPreset, uint32(slot))); err != nil {
		return err
	}
	for i := 0; i < 10; i++ { // PRESET_LOAD event clears Dirty
		time.Sleep(100 * time.Millisecond)
		if !s.State().Dirty {
			return nil
		}
	}
	s.markSelected()
	if err := s.send(usb.BuildStatusSet(usb.StatusIDCurrentPreset, uint32(slot^1))); err != nil {
		return err
	}
	time.Sleep(400 * time.Millisecond)
	if err := s.send(usb.BuildStatusSet(usb.StatusIDCurrentPreset, uint32(slot))); err != nil {
		return err
	}
	for i := 0; i < 15; i++ {
		time.Sleep(100 * time.Millisecond)
		if st := s.State(); !st.Dirty && st.Slot == slot && st.Setlist == set {
			return nil
		}
	}
	return errors.New("the pedal didn't confirm the reload")
}
