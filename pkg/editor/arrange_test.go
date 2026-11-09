package editor

import (
	"testing"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// fakeClient satisfies usb.Client without hardware.
type fakeClient struct{ ch chan *usb.SPSIMessage }

func (f *fakeClient) SendSPSI([]byte) error             { return nil }
func (f *fakeClient) RecvChan() <-chan *usb.SPSIMessage { return f.ch }
func (f *fakeClient) Close() error                      { return nil }

// newTestSession: setlists 0 and 1 fully "read"; slot body byte 0 = 100*set+slot.
func newTestSession(t *testing.T) *Session {
	t.Helper()
	s := NewSession(&fakeClient{ch: make(chan *usb.SPSIMessage)}, nil)
	for set := 0; set < 2; set++ {
		for sl := 0; sl < SlotsPerSetlst; sl++ {
			b := make([]byte, podhd500.PresetBodySize)
			b[0] = byte(100*set + sl)
			s.lib.slots[set][sl] = b
		}
	}
	return s
}

// ids returns the id (byte 0) of the editor's version of slots [lo, hi].
func ids(s *Session, set, lo, hi int) []int {
	s.lib.mu.Lock()
	defer s.lib.mu.Unlock()
	eff, _ := s.effectiveLocked(set)
	var out []int
	for i := lo; i <= hi; i++ {
		out = append(out, int(eff[i][0]))
	}
	return out
}

func eq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestArrangeMove(t *testing.T) {
	s := newTestSession(t)
	// move 1 -> 4 (down): 2,3,4 shift up
	if err := s.Arrange(ArrangeRequest{Op: OpMove, From: []SlotPos{{0, 1}}, To: SlotPos{0, 4}}); err != nil {
		t.Fatal(err)
	}
	if got := ids(s, 0, 0, 5); !eq(got, []int{0, 2, 3, 4, 1, 5}) {
		t.Fatalf("move down: %v", got)
	}
	// move 4 -> 1 (up) restores the original order and clears pending edits
	if err := s.Arrange(ArrangeRequest{Op: OpMove, From: []SlotPos{{0, 4}}, To: SlotPos{0, 1}}); err != nil {
		t.Fatal(err)
	}
	if got := ids(s, 0, 0, 5); !eq(got, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatalf("move up: %v", got)
	}
	for i := 0; i < SlotsPerSetlst; i++ {
		if s.lib.edited[0][i] != nil {
			t.Fatalf("slot %d still marked edited after moving back", i)
		}
	}
}

func TestArrangeCopyAndSwap(t *testing.T) {
	s := newTestSession(t)
	// copy 0:2,0:3 onto 1:10..11
	if err := s.Arrange(ArrangeRequest{Op: OpCopy, From: []SlotPos{{0, 2}, {0, 3}}, To: SlotPos{1, 10}}); err != nil {
		t.Fatal(err)
	}
	if got := ids(s, 1, 9, 12); !eq(got, []int{109, 2, 3, 112}) {
		t.Fatalf("copy: %v", got)
	}
	// overlapping copy inside one setlist: 0:5,0:6 onto 0:6..7 must use the originals
	if err := s.Arrange(ArrangeRequest{Op: OpCopy, From: []SlotPos{{0, 5}, {0, 6}}, To: SlotPos{0, 6}}); err != nil {
		t.Fatal(err)
	}
	if got := ids(s, 0, 5, 8); !eq(got, []int{5, 5, 6, 8}) {
		t.Fatalf("overlapping copy: %v", got)
	}
	// swap across setlists
	if err := s.Arrange(ArrangeRequest{Op: OpSwap, From: []SlotPos{{0, 0}}, To: SlotPos{1, 63}}); err != nil {
		t.Fatal(err)
	}
	if a, b := ids(s, 0, 0, 0)[0], ids(s, 1, 63, 63)[0]; a != 163 || b != 0 {
		t.Fatalf("swap: %d %d", a, b)
	}
	// does not fit
	if err := s.Arrange(ArrangeRequest{Op: OpCopy, From: []SlotPos{{0, 1}, {0, 2}}, To: SlotPos{0, 63}}); err == nil {
		t.Fatal("copy past 16D accepted")
	}
}

func TestArrangeProtectsDirtyLoadedPreset(t *testing.T) {
	s := newTestSession(t)
	s.update(func(st *State) { st.Setlist, st.Slot, st.Dirty = 0, 3, true })
	cases := []ArrangeRequest{
		{Op: OpMove, From: []SlotPos{{0, 1}}, To: SlotPos{0, 5}}, // 3 shifts
		{Op: OpCopy, From: []SlotPos{{1, 0}}, To: SlotPos{0, 3}}, // overwrite 3
		{Op: OpCopy, From: []SlotPos{{0, 3}}, To: SlotPos{1, 0}}, // copy dirty 3
		{Op: OpSwap, From: []SlotPos{{0, 3}}, To: SlotPos{1, 0}},
	}
	for _, c := range cases {
		if err := s.Arrange(c); err != ErrLoadedDirty {
			t.Fatalf("%+v: err = %v, want ErrLoadedDirty", c, err)
		}
	}
	// not touching slot 3 is fine
	if err := s.Arrange(ArrangeRequest{Op: OpMove, From: []SlotPos{{0, 10}}, To: SlotPos{0, 12}}); err != nil {
		t.Fatal(err)
	}
}

func TestArrangeNeedsLoadedSetlist(t *testing.T) {
	s := newTestSession(t)
	if err := s.Arrange(ArrangeRequest{Op: OpCopy, From: []SlotPos{{0, 1}}, To: SlotPos{5, 0}}); err == nil {
		t.Fatal("copy into a setlist never read was accepted")
	}
}
