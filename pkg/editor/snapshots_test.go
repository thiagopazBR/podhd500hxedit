package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// recClient records the frames "sent to the pedal".
type recClient struct {
	ch     chan *usb.SPSIMessage
	frames [][]byte
}

func (r *recClient) SendSPSI(b []byte) error {
	r.frames = append(r.frames, append([]byte(nil), b...))
	return nil
}
func (r *recClient) RecvChan() <-chan *usb.SPSIMessage { return r.ch }
func (r *recClient) Close() error                      { return nil }

// snapSession: a session whose edit buffer holds a real preset body (a fixture
// recorded from the pedal), with the config dir redirected to a temp folder.
func snapSession(t *testing.T) (*Session, *recClient, []byte) {
	t.Helper()
	return snapSessionWith(t, "amp_on_b.bin")
}

func snapSessionWith(t *testing.T, fixture string) (*Session, *recClient, []byte) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	body, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	rc := &recClient{ch: make(chan *usb.SPSIMessage)}
	s := NewSession(rc, nil)
	s.setBuffer(body)
	s.update(func(st *State) { st.Setlist, st.Slot, st.PresetName = 1, 5, "Test Tone" })
	if err := s.SetSnapshotsEnabled(true); err != nil {
		t.Fatal(err)
	}
	return s, rc, body
}

func TestSnapshotsOffByDefaultAndRefuseWhenOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := NewSession(&recClient{ch: make(chan *usb.SPSIMessage)}, nil)
	if s.SnapshotsEnabled() {
		t.Fatal("snapshots must be off by default")
	}
	if err := s.SaveSnapshot(1, "", false); err == nil {
		t.Fatal("save accepted while off")
	}
	if _, err := s.ApplySnapshot(1); err == nil {
		t.Fatal("apply accepted while off")
	}
}

// Capture -> change the buffer -> apply must bring the buffer back byte for
// byte: capture and apply use the same value scale and the same writers.
func TestSnapshotRoundTripRestoresBuffer(t *testing.T) {
	// amp_on_b.bin: a dual-amp preset; fx_chain.bin: a real preset with 8 FX
	// (compressors, delays, reverb, drives; one tempo-synced) dumped from the pedal
	for _, fx := range []string{"amp_on_b.bin", "fx_chain.bin"} {
		t.Run(fx, func(t *testing.T) { roundTrip(t, fx) })
	}
}

func roundTrip(t *testing.T, fixture string) {
	s, rc, orig := snapSessionWith(t, fixture)
	if err := s.SaveSnapshot(1, "Clean", false); err != nil {
		t.Fatal(err)
	}
	// perturb: toggle every non-empty block and move every plain param
	items, _ := s.liveItems()
	changed := 0
	for _, it := range items {
		if it.Empty || it.DSPOver {
			continue
		}
		if it.Kind == "cab" {
			continue // cabs are not part of snapshots
		}
		if err := s.SetBlockActive(it.ID, !it.Active); err != nil {
			t.Fatal(err)
		}
		changed++
		for _, p := range it.Params {
			if p.Ctl != 0 || p.Sync != 0 || len(p.Options) > 0 {
				continue
			}
			v := p.Value
			if v < 0.5 {
				v += 0.25
			} else {
				v -= 0.25
			}
			if err := s.SetParam(it.ID, p.ID, v); err != nil {
				t.Fatal(err)
			}
			changed++
		}
	}
	if changed == 0 || bytes.Equal(s.BufferCopy(), orig) {
		t.Fatal("perturbation changed nothing")
	}
	if !s.State().Dirty {
		t.Fatal("perturbing through SetParam should have made the preset dirty")
	}
	s.update(func(st *State) { st.Dirty = false }) // the snapshot is applied on a clean preset
	before := len(rc.frames)
	res, err := s.ApplySnapshot(1)
	n := res.Changes
	if err != nil || n == 0 {
		t.Fatalf("apply: n=%d err=%v", n, err)
	}
	if len(rc.frames)-before != n {
		t.Fatalf("sent %d frames for %d changes", len(rc.frames)-before, n)
	}
	if !bytes.Equal(s.BufferCopy(), orig) {
		t.Fatal("buffer differs from the original after apply")
	}
	if s.State().Dirty {
		t.Fatal("applying a snapshot must not mark the preset dirty")
	}
	// applying again sends nothing: the diff is empty
	if r, _ := s.ApplySnapshot(1); r.Changes != 0 {
		t.Fatalf("second apply sent %d changes", r.Changes)
	}
}

func TestSnapshotRefusesAnotherPreset(t *testing.T) {
	s, _, _ := snapSession(t)
	if err := s.SaveSnapshot(2, "", false); err != nil {
		t.Fatal(err)
	}
	s.update(func(st *State) { st.PresetName = "Renamed" })
	if _, err := s.ApplySnapshot(2); err == nil || !strings.Contains(err.Error(), "not to this preset") {
		t.Fatalf("apply on another preset: %v", err)
	}
	v, err := s.Snapshots()
	if err != nil || v.Mismatch == "" {
		t.Fatalf("mismatch not reported: %+v %v", v, err)
	}
	// saving on the renamed preset is refused: it would delete the old preset's snapshots
	if err := s.SaveSnapshot(1, "", false); err == nil || !strings.Contains(err.Error(), "would replace") {
		t.Fatalf("save over another preset's file: %v", err)
	}
	if v, _ := s.Snapshots(); v.Mismatch == "" {
		t.Fatal("file was replaced without replace:true")
	}
	if err := s.SaveSnapshot(1, "", true); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Snapshots(); v.Mismatch != "" || !v.Slots[0].Saved || v.Slots[1].Saved {
		t.Fatalf("after re-save: %+v", v)
	}
}

func TestApplyEmptySlotAndRange(t *testing.T) {
	s, _, _ := snapSession(t)
	if _, err := s.ApplySnapshot(3); err == nil {
		t.Fatal("empty snapshot applied")
	}
	if _, err := s.ApplySnapshot(0); err == nil {
		t.Fatal("slot 0 accepted")
	}
	if err := s.SaveSnapshot(5, "", false); err == nil {
		t.Fatal("slot 5 accepted")
	}
}

func TestPrepareSnapshotMIDIWritesCCs(t *testing.T) {
	s, rc, _ := snapSession(t)
	if err := s.PrepareSnapshotMIDI(nil); err != nil {
		t.Fatal(err)
	}
	if len(rc.frames) != len(DefaultSnapshotMIDI) {
		t.Fatalf("frames = %d, want %d", len(rc.frames), len(DefaultSnapshotMIDI))
	}
	p, _ := podhd500.ParsePreset(s.BufferCopy())
	for _, d := range DefaultSnapshotMIDI {
		a := p.MIDIAssigns()[d.Control]
		if a.Message != 1 || a.Channel != podhd500.MIDIChannelBase || a.Number != d.Number || a.Value != 127 {
			t.Fatalf("control %d = %+v", d.Control, a)
		}
	}
	if err := s.PrepareSnapshotMIDI([]SnapshotMIDI{{6, 200}}); err == nil {
		t.Fatal("CC 200 accepted")
	}
}

func TestApplyKeepsDirtyWhenAlreadyDirty(t *testing.T) {
	s, _, _ := snapSession(t)
	if err := s.SaveSnapshot(1, "", false); err != nil {
		t.Fatal(err)
	}
	items, _ := s.liveItems()
	for _, it := range items {
		if !it.Empty && it.Kind == "amp" {
			s.SetBlockActive(it.ID, !it.Active) // user edit: dirty
			break
		}
	}
	if _, err := s.ApplySnapshot(1); err != nil {
		t.Fatal(err)
	}
	if !s.State().Dirty {
		t.Fatal("an already dirty preset must stay dirty")
	}
}
