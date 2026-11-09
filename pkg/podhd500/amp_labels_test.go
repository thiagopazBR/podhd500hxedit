package podhd500

import "testing"

func TestAmpKnobLabels(t *testing.T) {
	want := map[string][]string{ // first four tone knobs: Drive, Bass, Mid, Treble ids
		"Super O":            {"Drive", "Bass", "Tone", "Treble"},
		"Divide 9/15":        {"Drive 1", "Drive 2", "Tone", "Cut"},
		"Divide 9/15 Preamp": {"Drive 1", "Drive 2", "Tone", "Cut"},
		"Class A-15":         {"Drive", "Bass", "Cut", "Treble"},
		"Class A-30 TB":      {"Drive", "Bass", "Cut", "Treble"},
		"Brit J-800":         {"Drive", "Bass", "Mid", "Treble"},
	}
	ids := []uint32{0x3F100003, 0x3F100000, 0x3F100001, 0x3F100002}
	found := map[string]bool{}
	for _, m := range GetCatalog().Amps {
		w, ok := want[m.Name]
		if !ok {
			continue
		}
		found[m.Name] = true
		names := map[uint32]string{}
		for _, p := range m.Params {
			names[p.ID] = p.Name
		}
		for i, id := range ids {
			if names[id] != w[i] {
				t.Errorf("%s: param %#x = %q, want %q", m.Name, id, names[id], w[i])
			}
		}
	}
	for name := range want {
		if !found[name] {
			t.Errorf("%s not in the catalog", name)
		}
	}
}

func TestAmpVolumeIsChVol(t *testing.T) {
	n := 0
	for _, m := range GetCatalog().Amps {
		for _, p := range m.Params {
			if p.Name == "Volume" {
				t.Fatalf("%s still has a Volume knob", m.Name)
			}
			if p.Name == "Ch Vol" {
				n++
			}
		}
	}
	if n == 0 {
		t.Fatal("no Ch Vol knob found")
	}
}

func TestAmpTailOrder(t *testing.T) {
	ps := []ItemParam{{Index: 0, Name: "Drive"}, {Index: 6, Name: "Master"}, {Index: 7, Name: "Bias X"}, {Index: 8, Name: "Sag"}, {Index: 9, Name: "Bias"}, {Index: 10, Name: "Hum"}}
	orderAmpTail(ps)
	got := ""
	for _, p := range ps {
		got += p.Name + ","
	}
	if got != "Drive,Master,Sag,Hum,Bias,Bias X," || ps[5].Index != 7 {
		t.Fatalf("got %s (last index %d)", got, ps[5].Index)
	}
}
