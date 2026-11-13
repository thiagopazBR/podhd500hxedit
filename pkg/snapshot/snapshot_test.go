package snapshot

import (
	"path/filepath"
	"testing"

	"podhd500-edit/pkg/podhd500"
)

func items() []podhd500.PresetItem {
	return []podhd500.PresetItem{
		{ID: 0, Type: 100, Active: true, Params: []podhd500.ItemParam{{ID: 1, Value: 0.5}, {ID: 2, Value: 0.3}}},
		{ID: 4, Type: 200, Active: false, Params: []podhd500.ItemParam{{ID: 1, Value: 0.1}}},
		{ID: 5, Empty: true},
		{ID: 6, Type: 300, DSPOver: true},
	}
}

func TestCaptureSkipsEmptyAndDSPOver(t *testing.T) {
	s := Capture(items())
	if len(s.Blocks) != 2 || s.Blocks[0].ID != 0 || s.Blocks[1].ID != 4 {
		t.Fatalf("blocks = %+v", s.Blocks)
	}
	if !s.Blocks[0].On || s.Blocks[1].On || len(s.Blocks[0].Params) != 2 {
		t.Fatalf("state not captured: %+v", s.Blocks)
	}
}

func TestCaptureSkipsControllerSyncAndCab(t *testing.T) {
	its := []podhd500.PresetItem{
		{ID: 1, Kind: "cab", Type: 5, Active: true, Params: []podhd500.ItemParam{{ID: 0, Value: 0.4}}},
		{ID: 4, Kind: "fx", Type: 7, Active: true, Params: []podhd500.ItemParam{
			{ID: 1, Value: 0.2},               // plain: kept
			{ID: 2, Value: 0.6, Ctl: 2},       // on EXP 2: dropped
			{ID: 3, Value: 0.5, Sync: 4},      // tempo-synced: dropped
			{ID: 4, Value: 0.9, TempoSlot: 1}, // free tempo param: kept
		}},
	}
	s := Capture(its)
	if len(s.Blocks) != 1 || s.Blocks[0].ID != 4 {
		t.Fatalf("cab must not be captured: %+v", s.Blocks)
	}
	got := s.Blocks[0].Params
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 4 {
		t.Fatalf("fx params = %+v, want ids 1 and 4", got)
	}
}

func TestSaveRange(t *testing.T) {
	f := New(Identity{0, 3, "Tone"})
	if err := f.Save(Count, Capture(items())); err == nil {
		t.Fatal("slot out of range accepted")
	}
	if err := f.Save(0, Snapshot{}); err == nil {
		t.Fatal("empty snapshot accepted")
	}
	if err := f.Save(3, Capture(items())); err != nil || f.Snapshots[3].Empty() {
		t.Fatalf("save failed: %v", err)
	}
}

func TestDiffSendsOnlyDifferences(t *testing.T) {
	a := Capture(items())
	b := Capture(items())
	b.Blocks[0].Params[1].Value = 0.9 // one parameter differs
	b.Blocks[1].On = true             // one block toggles
	ops, _ := Diff(a, b, items())
	if len(ops) != 2 {
		t.Fatalf("ops = %+v, want 2", ops)
	}
	got := map[bool]Op{}
	for _, o := range ops {
		got[o.IsParam] = o
	}
	if o := got[true]; o.Block != 0 || o.ParamID != 2 || o.Value != 0.9 {
		t.Fatalf("param op = %+v", o)
	}
	if o := got[false]; o.Block != 4 || !o.On {
		t.Fatalf("on/off op = %+v", o)
	}
	if ops, _ := Diff(a, a, items()); len(ops) != 0 {
		t.Fatalf("same snapshot produced ops: %+v", ops)
	}
}

func TestDiffUnknownCurrentSendsEverything(t *testing.T) {
	ops, _ := Diff(Snapshot{}, Capture(items()), items())
	// 2 blocks on/off + 3 params
	if len(ops) != 5 {
		t.Fatalf("ops = %d, want 5: %+v", len(ops), ops)
	}
}

func TestDiffSkipsChangedModelAndGoneBlock(t *testing.T) {
	next := Capture(items())
	live := items()
	live[0].Type = 999 // amp model changed since the snapshot was saved
	live[1].Empty = true
	ops, skipped := Diff(Snapshot{}, next, live)
	if len(ops) != 0 {
		t.Fatalf("touched changed/removed blocks: %+v", ops)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped = %v, want both blocks reported", skipped)
	}
}

func TestFileRoundTripAndIdentity(t *testing.T) {
	dir := t.TempDir()
	id := Identity{Setlist: 1, Slot: 7, Name: "My Tone"}
	f := New(id)
	if err := f.Save(1, Capture(items())); err != nil {
		t.Fatal(err)
	}
	p := Path(dir, id)
	if filepath.Base(p) != "set2-2D.snapshots.json" {
		t.Fatalf("path = %s", p)
	}
	if err := f.Write(p); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Matches(id) || g.Matches(Identity{1, 7, "Other"}) || g.Matches(Identity{1, 8, "My Tone"}) {
		t.Fatal("identity comparison wrong")
	}
	if g.Snapshots[1].Empty() || !g.Snapshots[0].Empty() || len(g.Snapshots[1].Blocks) != 2 {
		t.Fatalf("round trip lost data: %+v", g.Snapshots)
	}
}

func TestLoadRejectsOtherVersion(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	f := New(Identity{})
	f.Version = 99
	if err := f.Write(p); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("future version accepted")
	}
}

func TestDiffOrdersOffThenParamsThenOn(t *testing.T) {
	cur := Capture(items()) // block 0 on, block 4 off
	next := Capture(items())
	next.Blocks[0].On = false            // 0 turns off
	next.Blocks[0].Params[0].Value = 0.7 // 0 param changes
	next.Blocks[1].On = true             // 4 turns on
	next.Blocks[1].Params[0].Value = 0.8 // 4 param changes
	ops, _ := Diff(cur, next, items())
	if len(ops) != 4 {
		t.Fatalf("ops = %+v", ops)
	}
	if ops[0].IsParam || ops[0].On || ops[0].Block != 0 {
		t.Fatalf("first op must turn block 0 off: %+v", ops[0])
	}
	if !ops[1].IsParam || !ops[2].IsParam {
		t.Fatalf("params must come between: %+v", ops)
	}
	if ops[3].IsParam || !ops[3].On || ops[3].Block != 4 {
		t.Fatalf("last op must turn block 4 on: %+v", ops[3])
	}
}
