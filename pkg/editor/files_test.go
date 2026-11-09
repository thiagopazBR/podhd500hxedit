package editor

import (
	"encoding/binary"
	"math"
	"testing"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// newFileSession: setlists 0 and 1 read, every slot a valid preset named
// "<set>-<slot>".
func newFileSession(t *testing.T) *Session {
	t.Helper()
	blank, err := podhd500.EmbeddedData.ReadFile("data/blank_preset.bin")
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession(&fakeClient{ch: make(chan *usb.SPSIMessage)}, nil)
	for set := 0; set < 2; set++ {
		for sl := 0; sl < SlotsPerSetlst; sl++ {
			p, _ := podhd500.ParsePreset(blank)
			p.SetName(string(rune('0'+set)) + "-" + slotName(sl))
			s.lib.slots[set][sl] = append([]byte(nil), p.Body[:]...)
		}
		s.lib.loaded[set] = true
	}
	s.lib.names[0], s.lib.names[1] = "First", "Second"
	s.lib.namesLoaded = true
	return s
}

func name(s *Session, set, slot int) string {
	b, err := s.editorBody(set, slot)
	if err != nil {
		return err.Error()
	}
	p, _ := podhd500.ParsePreset(b)
	return p.Name()
}

func TestExportImportPreset(t *testing.T) {
	s := newFileSession(t)
	data, fn, err := s.Export(ExportRequest{Kind: podhd500.KindPreset, Setlist: 0, Slot: 5})
	if err != nil {
		t.Fatal(err)
	}
	if fn != "0-02B.h5e" || len(data) != podhd500.H5ESize {
		t.Fatalf("file %q %d bytes", fn, len(data))
	}
	// two copies into setlist 1 at 10, 11
	res, err := s.Import([][]byte{data, data}, SlotPos{1, 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Presets != 2 || res.Same != 0 {
		t.Fatalf("result %+v", res)
	}
	if name(s, 1, 10) != "0-02B" || name(s, 1, 11) != "0-02B" || name(s, 1, 12) != "1-04A" {
		t.Fatal("wrong slots", name(s, 1, 10), name(s, 1, 11), name(s, 1, 12))
	}
	// importing onto itself changes nothing
	res, err = s.Import([][]byte{data}, SlotPos{0, 5})
	if err != nil || res.Same != 1 || s.lib.edited[0][5] != nil {
		t.Fatalf("self import %+v %v", res, err)
	}
	if _, err := s.Import([][]byte{data, data}, SlotPos{0, 63}); err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestExportImportSetlist(t *testing.T) {
	s := newFileSession(t)
	data, fn, err := s.Export(ExportRequest{Kind: podhd500.KindSetlist, Setlist: 0})
	if err != nil {
		t.Fatal(err)
	}
	if fn != "First.h5s" || len(data) != podhd500.H5SSize {
		t.Fatalf("file %q %d bytes", fn, len(data))
	}
	res, err := s.Import([][]byte{data}, SlotPos{Setlist: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Presets != 64 || len(res.SetNames) != 1 || res.SetNames[0] != "First" || res.Renamed != 1 {
		t.Fatalf("result %+v", res)
	}
	for i := 0; i < SlotsPerSetlst; i++ {
		if name(s, 1, i) != name(s, 0, i) {
			t.Fatalf("slot %d: %q", i, name(s, 1, i))
		}
	}
	l, _ := s.Setlists()
	if l[1].Name != "First" || l[1].Stored != "Second" || !l[1].Edited {
		t.Fatalf("setlist name not pending: %+v", l[1])
	}
	if _, err := s.Import([][]byte{data, data}, SlotPos{Setlist: 1}); err == nil {
		t.Fatal("two setlists at once must fail")
	}
}

func TestImportLoadedDirty(t *testing.T) {
	s := newFileSession(t)
	data, _, _ := s.Export(ExportRequest{Kind: podhd500.KindPreset, Setlist: 0, Slot: 1})
	s.update(func(st *State) { st.Setlist, st.Slot, st.Dirty = 1, 3, true })
	if _, err := s.Import([][]byte{data}, SlotPos{1, 3}); err != ErrLoadedDirty {
		t.Fatalf("got %v", err)
	}
}

func TestImportRejectsGarbage(t *testing.T) {
	s := newFileSession(t)
	if _, err := s.Import([][]byte{[]byte("not a preset at all")}, SlotPos{0, 0}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRenameSetlist(t *testing.T) {
	s := newFileSession(t)
	if err := s.RenameSetlist(0, "  Rock Gig "); err != nil {
		t.Fatal(err)
	}
	l, _ := s.Setlists()
	if l[0].Name != "Rock Gig" || !l[0].Edited || l[0].Stored != "First" {
		t.Fatalf("%+v", l[0])
	}
	// the export carries the editor's name, the backup names stay the pedal's
	if n, _ := s.setlistNames(false); n[0] != "Rock Gig" {
		t.Fatal(n[0])
	}
	if n, _ := s.setlistNames(true); n[0] != "First" {
		t.Fatal(n[0])
	}
	if err := s.RenameSetlist(0, "First"); err != nil { // back to the pedal's name
		t.Fatal(err)
	}
	if l, _ := s.Setlists(); l[0].Edited {
		t.Fatal("still pending")
	}
	if err := s.RenameSetlist(0, "Ação"); err == nil {
		t.Fatal("accent accepted")
	}
	if err := s.RenameSetlist(0, "12345678901234567"); err == nil {
		t.Fatal("17 chars accepted")
	}
}

func TestModels(t *testing.T) {
	g := Models()
	names := map[string]int{}
	for _, x := range g {
		names[x.Name] = len(x.Models)
		for _, m := range x.Models {
			if !validModel(x.Kind, m.Type) {
				t.Fatalf("%s %s not valid for %s", x.Name, m.Name, x.Kind)
			}
		}
	}
	if names["Distortion"] == 0 || names["Amps"] == 0 || names["Preamps"] == 0 || names["Cabs"] == 0 {
		t.Fatalf("groups %v", names)
	}
	if validModel("cab", 0x00070000) { // an amp is not a cab
		t.Fatal("amp accepted as cab")
	}
	t.Logf("%v", names)
}

// Front-panel edits are applied to the copy of the edit buffer from the
// event itself, without re-reading the buffer (which shows "SYNC IN
// PROGRESS" on the pedal).
func TestFrontPanelEventsApplied(t *testing.T) {
	s := newFileSession(t)
	p, _ := podhd500.ParsePreset(s.lib.slots[0][0])
	// give FX 1 a known parameter
	o := podhd500.ItemOffset(4) + podhd500.ParamBase
	binary.LittleEndian.PutUint32(p.Body[o:], 0x3F100003)
	s.setBuffer(p.Body[:])
	rev := s.State().BufferRev

	ev := func(cmd uint8, words ...uint32) *usb.SPSIMessage {
		b := make([]byte, 4*len(words))
		for i, w := range words {
			binary.LittleEndian.PutUint32(b[4*i:], w)
		}
		return &usb.SPSIMessage{Cmd: cmd, Payload: b}
	}
	s.onEvent(ev(usb.CmdParamChange, 0, 4, 1, 0x3F100003, math.Float32bits(0.75)))
	s.onEvent(ev(usb.CmdSetupChange, 0, 1, 0x32, math.Float32bits(0.3))) // cab A E.R.
	s.bufMu.Lock()
	got, _ := podhd500.ParsePreset(s.buf)
	timer := s.reloadTimer
	s.bufMu.Unlock()
	if v := got.Item(4).Params[0].Value; v != 0.75 {
		t.Fatalf("param not applied: %v", v)
	}
	if er := cabViews(got)[0].ER; er < 0.299 || er > 0.301 {
		t.Fatalf("E.R. not applied: %v", er)
	}
	if timer != nil {
		t.Fatal("buffer re-read scheduled")
	}
	if s.State().BufferRev <= rev || !s.State().Dirty {
		t.Fatal("state not updated")
	}
}

// Variax model list of the 300/600 500/700: 111 entries, value = index;
// verified on the LCD: 10 = F-T-Model-1.
func TestVariaxModels(t *testing.T) {
	l := VariaxModels()["300/600 500/700"]
	if len(l) != 111 || l[0] != "(Don't Force)" || l[10] != "F-T-Model-1" || l[110] != "Reso-1" {
		t.Fatalf("variax list: %d entries, %q %q %q", len(l), l[0], l[10], l[len(l)-1])
	}
}

func TestVariaxModelsTyler(t *testing.T) {
	l := VariaxModels()["Tyler Variax"]
	if len(l) != 61 || l[6] != "U:T-Model-5" || l[10] != "U:T-Model-1" || l[55] != "U:Reso-1" || l[60] != "Custom 2-1" {
		t.Fatalf("tyler list: %d entries %q %q %q %q", len(l), l[6], l[10], l[55], l[60])
	}
}

func TestVariaxModelsAcoustic(t *testing.T) {
	l := VariaxModels()["Acoustic 700"]
	if len(l) != 33 || l[1] != "F-Parlor" || l[5] != "F-C & W" || l[16] != "F-Sitar" || l[17] != "U-Parlor" || l[32] != "U-Sitar" {
		t.Fatalf("acoustic list: %d entries %q", len(l), l)
	}
}
