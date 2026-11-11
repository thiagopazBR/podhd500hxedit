package podhd500

import "testing"

func TestMIDIAssign(t *testing.T) {
	blank, err := EmbeddedData.ReadFile("data/blank_preset.bin")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ParsePreset(blank)
	// FS1 as the user set it on the pedal: CC Toggle, channel 3, CC 20, value 127
	if !p.SetMIDIAssign(0, [6]byte{0, 2, 20, 127, 0, 0}) {
		t.Fatal("no change")
	}
	a := p.MIDIAssigns()
	if a[0].Channel != 2 || a[0].Number != 20 || a[0].Value != 127 || a[0].Name != "FS1" || a[0].Pedal {
		t.Fatalf("%+v", a[0])
	}
	if !a[11].Pedal || a[11].Name != "EXP 1" || a[10].Pedal {
		t.Fatal("pedals")
	}
	// storage order = the LCD's MIDI ASSIGN screen read row by row
	want := []string{"FS1", "FS2", "FS3", "FS4", "Looper", "EXP Toe", "FS5", "FS6", "FS7", "FS8", "Tap", "EXP 1", "EXP 2"}
	for i, n := range want {
		if a[i].Name != n || a[i].Control != i {
			t.Fatalf("control %d: got %q, want %q", i, a[i].Name, n)
		}
	}
	if MIDIChannelOptions[2].Label != "01" || MIDIChannelOptions[17].Label != "16" || len(MIDIChannelOptions) != 18 {
		t.Fatal("channel options")
	}
}
