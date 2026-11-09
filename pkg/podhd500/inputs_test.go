package podhd500

import "testing"

func TestInputSetupOffsets(t *testing.T) {
	// Offsets documented in refs/hd500-editor/docs/preset-format.md.
	want := map[uint32]int{SetupInput1Source: 0xFFE, SetupInput2Source: 0xFFF, SetupGuitarInZ: 0xDD2}
	for id, off := range want {
		s, ok := LookupSetup(id)
		if !ok || s.Offset != off || s.Size != 1 {
			t.Fatalf("setup 0x%X: got %+v ok=%v, want offset 0x%X size 1", id, s, ok, off)
		}
	}
}

func TestSetupValueRoundTrip(t *testing.T) {
	var p Preset
	for _, c := range []struct{ id, v uint32 }{{SetupInput1Source, 5}, {SetupInput2Source, 0}, {SetupGuitarInZ, 7}} {
		if !p.SetSetupValue(c.id, c.v) {
			t.Fatalf("SetSetupValue(0x%X) failed", c.id)
		}
		if got, ok := p.SetupValue(c.id); !ok || got != c.v {
			t.Fatalf("SetupValue(0x%X) = %d,%v want %d", c.id, got, ok, c.v)
		}
	}
	if p.GuitarInZ() != "1M" {
		t.Fatalf("GuitarInZ() = %q, want 1M", p.GuitarInZ())
	}
	if !ValidOption(Input2Options, 0) || ValidOption(Input1Options, 0) || ValidOption(Input1Options, 4) {
		t.Fatal("option validation wrong")
	}
}
