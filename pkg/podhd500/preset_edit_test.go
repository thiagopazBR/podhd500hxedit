package podhd500

import (
	"os"
	"testing"
)

func loadBlank(t *testing.T) *Preset {
	t.Helper()
	b, err := os.ReadFile("data/blank_preset.bin")
	if err != nil {
		t.Skip("data/blank_preset.bin not available:", err)
	}
	if len(b) < PresetBodySize {
		t.Skipf("blank preset has %d bytes", len(b))
	}
	p, err := ParsePreset(b[len(b)-PresetBodySize:])
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSettersRoundTripThroughReader(t *testing.T) {
	p := loadBlank(t)
	p.SetName("CLAUDE TEST")
	if p.Name() != "CLAUDE TEST" {
		t.Fatalf("name = %q", p.Name())
	}
	for id := 0; id < 12; id++ {
		orig := p.Item(id).Active
		p.SetItemActive(id, !orig)
		if p.Item(id).Active == orig {
			t.Fatalf("item %d: Active not toggled", id)
		}
		p.SetItemActive(id, orig)
		seen := map[uint32]int{}
		for _, prm := range p.Item(id).Params {
			seen[prm.ID]++
		}
		for _, prm := range p.Item(id).Params {
			if seen[prm.ID] > 1 { // blank cab: ids all 0, ambiguous by id
				continue
			}
			if !p.SetParamValue(id, prm.ID, 0.25) {
				t.Fatalf("item %d: SetParamValue(0x%08X) = false", id, prm.ID)
			}
			for _, q := range p.Item(id).Params {
				if q.Index == prm.Index && q.Value != 0.25 {
					t.Fatalf("item %d param 0x%08X = %v", id, prm.ID, q.Value)
				}
			}
		}
	}
	if p.SetParamValue(0, 0xDEADBEEF, 1) {
		t.Fatal("SetParamValue accepted an unknown parameter")
	}
}
