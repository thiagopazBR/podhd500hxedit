package editor

import (
	"bytes"
	"errors"
	"fmt"
)

// Copy / move / swap presets between slots, like dragging in the original
// editor's Set Lists view. All changes stay in the editor (lib.edited, shown
// with an asterisk) until SEND; RECEIVE discards them.

// Arrange operations.
const (
	OpMove = "move" // insert: the preset goes to the target slot, the ones in between shift by one
	OpCopy = "copy" // overwrite target slot(s) with the source preset(s)
	OpSwap = "swap" // exchange two presets
)

// SlotPos addresses a slot.
type SlotPos struct {
	Setlist int `json:"setlist"`
	Slot    int `json:"slot"`
}

// ArrangeRequest describes one operation. Copy may take several sources (from
// one setlist), pasted on consecutive slots starting at To.
type ArrangeRequest struct {
	Op   string    `json:"op"`
	From []SlotPos `json:"from"`
	To   SlotPos   `json:"to"`
}

// ErrLoadedDirty is returned when an operation would move or overwrite the
// preset loaded on the pedal while it has unsaved edits.
var ErrLoadedDirty = errors.New("the loaded preset has unsaved changes: save (SEND) or discard (RECEIVE) them before moving or overwriting it")

func validPos(p SlotPos) bool {
	return p.Setlist >= 0 && p.Setlist < NumSetlists && p.Slot >= 0 && p.Slot < SlotsPerSetlst
}

// effective returns the editor's version of every slot of a setlist (pending
// edit if any, else what the pedal has). Locked by the caller.
func (s *Session) effectiveLocked(set int) ([SlotsPerSetlst][]byte, error) {
	var out [SlotsPerSetlst][]byte
	for i := range out {
		b := s.lib.edited[set][i]
		if b == nil {
			b = s.lib.slots[set][i]
		}
		if b == nil {
			return out, fmt.Errorf("setlist %d hasn't been fully read from the pedal yet", set+1)
		}
		out[i] = b
	}
	return out, nil
}

// setEffectiveLocked stores the editor's version of one slot: a pending edit,
// or none if it equals what the pedal has.
func (s *Session) setEffectiveLocked(set, slot int, body []byte) {
	if bytes.Equal(body, s.lib.slots[set][slot]) {
		s.lib.edited[set][slot] = nil
	} else {
		s.lib.edited[set][slot] = append([]byte(nil), body...)
	}
}

// Arrange applies a move / copy / swap to the editor's copy of the presets.
func (s *Session) Arrange(req ArrangeRequest) error {
	if len(req.From) == 0 || !validPos(req.To) {
		return errors.New("invalid source/destination")
	}
	for _, f := range req.From {
		if !validPos(f) {
			return errors.New("invalid source")
		}
	}
	if s.client == nil {
		return ErrOffline
	}

	st := s.State()
	touchesLoaded := func(set, lo, hi int) bool { // [lo, hi] of set includes the loaded preset?
		return st.Dirty && set == st.Setlist && st.Slot >= lo && st.Slot <= hi
	}

	s.lib.mu.Lock()
	defer func() {
		s.lib.mu.Unlock()
		s.update(func(st *State) { st.LibraryRev++ })
	}()

	switch req.Op {
	case OpMove:
		if len(req.From) != 1 {
			return errors.New("move takes one preset at a time")
		}
		from, to := req.From[0], req.To
		if from.Setlist != to.Setlist {
			return errors.New("move only works within the same setlist (use copy for another setlist)")
		}
		if from.Slot == to.Slot {
			return nil
		}
		lo, hi := min(from.Slot, to.Slot), max(from.Slot, to.Slot)
		if touchesLoaded(from.Setlist, lo, hi) {
			return ErrLoadedDirty
		}
		eff, err := s.effectiveLocked(from.Setlist)
		if err != nil {
			return err
		}
		moved := eff[from.Slot]
		if from.Slot < to.Slot { // those after it shift up
			copy(eff[from.Slot:to.Slot], eff[from.Slot+1:to.Slot+1])
		} else { // those before it shift down
			copy(eff[to.Slot+1:from.Slot+1], eff[to.Slot:from.Slot])
		}
		eff[to.Slot] = moved
		for i := lo; i <= hi; i++ {
			s.setEffectiveLocked(from.Setlist, i, eff[i])
		}

	case OpSwap:
		if len(req.From) != 1 {
			return errors.New("swap takes one preset at a time")
		}
		a, b := req.From[0], req.To
		if a == b {
			return nil
		}
		if touchesLoaded(a.Setlist, a.Slot, a.Slot) || touchesLoaded(b.Setlist, b.Slot, b.Slot) {
			return ErrLoadedDirty
		}
		ea, err := s.effectiveLocked(a.Setlist)
		if err != nil {
			return err
		}
		eb, err := s.effectiveLocked(b.Setlist)
		if err != nil {
			return err
		}
		pa, pb := ea[a.Slot], eb[b.Slot]
		s.setEffectiveLocked(a.Setlist, a.Slot, pb)
		s.setEffectiveLocked(b.Setlist, b.Slot, pa)

	case OpCopy:
		src := req.From[0].Setlist
		for _, f := range req.From {
			if f.Setlist != src {
				return errors.New("copy: the sources must be in the same setlist")
			}
		}
		last := req.To.Slot + len(req.From) - 1
		if last >= SlotsPerSetlst {
			return fmt.Errorf("doesn't fit: %d preset(s) starting at %s go past slot 16D", len(req.From), slotName(req.To.Slot))
		}
		for _, f := range req.From {
			if touchesLoaded(f.Setlist, f.Slot, f.Slot) {
				return ErrLoadedDirty // its pending edits live only in the pedal's buffer
			}
		}
		if touchesLoaded(req.To.Setlist, req.To.Slot, last) {
			return ErrLoadedDirty
		}
		es, err := s.effectiveLocked(src)
		if err != nil {
			return err
		}
		if _, err := s.effectiveLocked(req.To.Setlist); err != nil {
			return err
		}
		bodies := make([][]byte, len(req.From)) // read all first: source and target may overlap
		for i, f := range req.From {
			bodies[i] = es[f.Slot]
		}
		for i, b := range bodies {
			s.setEffectiveLocked(req.To.Setlist, req.To.Slot+i, b)
		}

	default:
		return fmt.Errorf("invalid operation %q", req.Op)
	}
	return nil
}

func slotName(slot int) string {
	return fmt.Sprintf("%02d%c", slot/4+1, "ABCD"[slot%4])
}
