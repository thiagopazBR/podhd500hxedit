package podhd500

import (
	"encoding/binary"
	"testing"
)

func TestControllerSetters(t *testing.T) {
	blank, err := EmbeddedData.ReadFile("data/blank_preset.bin")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ParsePreset(blank)
	if !p.SetItemFS(4, 3) || p.SetItemFS(4, 3) {
		t.Fatal("SetItemFS change detection")
	}
	if got := p.Item(4).FS; got != 3 {
		t.Fatalf("fs %d", got)
	}
	if !p.SetParamController(0, 1, 2) {
		t.Fatal("SetParamController")
	}
	if !p.SetParamRange(0, 1, false, 0.25) || !p.SetParamRange(0, 1, true, 0.75) {
		t.Fatal("SetParamRange")
	}
	it := p.Item(0)
	if len(it.Params) < 2 {
		t.Skip("blank amp has fewer than 2 params")
	}
	q := it.Params[1]
	if q.Ctl != 2 || q.Lo != 0.25 || q.Hi != 0.75 {
		t.Fatalf("param %+v", q)
	}
}

func TestAssignMIDI(t *testing.T) {
	if BypassOptions[3].CC != 53 || BypassOptions[9].CC != 59 || ControllerOptions[2].CC != 2 || ControllerOptions[3].CC != 0 {
		t.Fatal("MIDI CC map")
	}
}

func TestClearDSPOver(t *testing.T) {
	blank, _ := EmbeddedData.ReadFile("data/blank_preset.bin")
	p, _ := ParsePreset(blank)
	o := ItemOffset(4)
	binary.LittleEndian.PutUint32(p.Body[o:], 0x02040021) // Hall
	p.Body[o+11] = 0                                      // not in use: DSP over
	if !p.Item(4).DSPOver || !p.ClearDSPOver() || p.Item(4).DSPOver || !p.Item(4).Empty {
		t.Fatal("DSP over block not cleared")
	}
}
