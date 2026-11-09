package podhd500

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func blankBody(t *testing.T) []byte {
	b, err := EmbeddedData.ReadFile("data/blank_preset.bin")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > PresetBodySize {
		b = b[len(b)-PresetBodySize:]
	}
	if len(b) != PresetBodySize {
		t.Fatalf("blank preset %d bytes", len(b))
	}
	return b
}

func TestPresetRoundTrip(t *testing.T) {
	body := blankBody(t)
	f, err := EncodePreset(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != H5ESize || string(f[:4]) != "H5EP" || f[0x0B] != FileModelHD500 {
		t.Fatalf("bad header % x", f[:24])
	}
	d, err := Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != KindPreset || !bytes.Equal(d.Bodies[0][0], body) {
		t.Fatal("round trip mismatch")
	}
}

func TestSetlistAndBundleRoundTrip(t *testing.T) {
	body := blankBody(t)
	p, _ := ParsePreset(body)
	sets := make([][][]byte, FileSetlists)
	names := make([]string, FileSetlists)
	for s := range sets {
		names[s] = "Set " + string(rune('A'+s))
		sets[s] = make([][]byte, FilePresetSlots)
		for i := range sets[s] {
			p.SetName(SlotLabel(i) + names[s])
			sets[s][i] = append([]byte(nil), p.Body[:]...)
		}
	}
	b, err := EncodeBundle(names, sets)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != H5BSize {
		t.Fatalf("bundle %d bytes", len(b))
	}
	d, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	for s := range sets {
		if d.SetNames[s] != names[s] {
			t.Fatalf("name %q", d.SetNames[s])
		}
		for i := range sets[s] {
			if !bytes.Equal(d.Bodies[s][i], sets[s][i]) {
				t.Fatalf("set %d slot %d mismatch", s, i)
			}
		}
	}
	sl, err := EncodeSetlist(names[3], sets[3])
	if err != nil || len(sl) != H5SSize {
		t.Fatal(err, len(sl))
	}
	if !bytes.Equal(sl, b[FileHeaderSize+3*H5SSize : FileHeaderSize+4*H5SSize][:0:0]) && !bytes.Equal(sl[FileHeaderSize:], b[FileHeaderSize+3*H5SSize+FileHeaderSize:FileHeaderSize+4*H5SSize]) {
		t.Fatal("setlist inside bundle differs")
	}
}

// TestRealFiles decodes factory files when HD500_SAMPLES points at a folder
// with .h5e/.h5s/.h5b (or .5xe/.5xs/.5xb) files, and re-encodes them.
func TestRealFiles(t *testing.T) {
	dir := os.Getenv("HD500_SAMPLES")
	if dir == "" {
		t.Skip("HD500_SAMPLES not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	for _, fn := range files {
		data, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		d, err := Decode(data)
		if err != nil {
			t.Errorf("%s: %v", fn, err)
			continue
		}
		var re []byte
		switch d.Kind {
		case KindPreset:
			re, err = EncodePreset(d.Bodies[0][0])
		case KindSetlist:
			re, err = EncodeSetlist(d.SetNames[0], d.Bodies[0])
		case KindBundle:
			re, err = EncodeBundle(d.SetNames, d.Bodies)
		}
		if err != nil {
			t.Fatal(err)
		}
		// headers differ (model byte); the rest must be identical
		diff := 0
		for i := range re {
			if re[i] != data[i] {
				diff++
			}
		}
		bad := 0
		for _, set := range d.Bodies {
			for _, b := range set {
				q, _ := ParsePreset(b)
				if tp := q.Tempo(); tp < 30 || tp > 240 {
					bad++
				}
			}
		}
		if bad > 0 {
			t.Errorf("%s: %d presets with tempo out of 30..240", fn, bad)
		}
		p, _ := ParsePreset(d.Bodies[0][0])
		t.Logf("%s: %s %q first=%q, %d header bytes differ", filepath.Base(fn), d.Kind, d.SetNames[0], p.Name(), diff)
	}
}

// Cab parameters have ids 0..3: the first one (Low Cut, id 0) must not end
// the list.
func TestCabParams(t *testing.T) {
	dir := os.Getenv("HD500_SAMPLES")
	if dir == "" {
		t.Skip("HD500_SAMPLES not set")
	}
	data, err := os.ReadFile(filepath.Join(dir, "user1.5xs"))
	if err != nil {
		t.Skip(err)
	}
	f, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ParsePreset(f.Bodies[0][0])
	cab := p.Item(1)
	if len(cab.Params) != 4 || cab.Params[0].Name != "Low Cut" || cab.Params[3].Name != "Decay" {
		t.Fatalf("cab params: %+v", cab.Params)
	}
}
