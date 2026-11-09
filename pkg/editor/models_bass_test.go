package editor

import "testing"

// Every bass name must exist in the catalog (a typo would silently make a
// bass model show up as a guitar one).
func TestBassModels(t *testing.T) {
	found := map[string]bool{}
	nBass := map[string]int{}
	for _, g := range Models() {
		for _, m := range g.Models {
			found[m.Name] = true
			if m.Bass {
				nBass[g.Name]++
			}
		}
	}
	for name := range bassAmps {
		if !found[name] || !found[name+" Preamp"] {
			t.Errorf("bass amp %q (or its preamp) not in the catalog", name)
		}
	}
	for name := range bassCabs {
		if !found[name] {
			t.Errorf("bass cab %q not in the catalog", name)
		}
	}
	if nBass["Amps"] != 4 || nBass["Preamps"] != 4 || nBass["Cabs"] != 3 {
		t.Errorf("bass models per group = %v, want Amps 4, Preamps 4, Cabs 3", nBass)
	}
}

// Every amp and preamp has a drawing style (list icons).
func TestAmpStyles(t *testing.T) {
	for _, g := range Models() {
		if g.Kind != "amp" {
			continue
		}
		for _, m := range g.Models {
			if !m.None && m.Style == nil {
				t.Errorf("%s: no style in gear-styles.json", m.Name)
			}
		}
	}
}
