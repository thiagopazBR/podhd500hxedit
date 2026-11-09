package editor

import "testing"

func TestTunerReading(t *testing.T) {
	// Readings checked against the pedal's LCD with the user's guitar (2026-11-06).
	cases := []struct {
		note   int32
		cents  float32
		name   string
		octave int
		signal bool
	}{
		{41, 0, "E", 4, true},    // high E string
		{36, -1, "B", 3, true},   // B string
		{32, 6, "G", 3, true},    // G string
		{-3, -104, "", 0, false}, // no signal
		{-2, 97, "", 0, false},   // starting
	}
	for _, c := range cases {
		r := tunerReading(c.note, c.cents)
		if !r.On || r.Signal != c.signal || r.Note != c.name || r.Octave != c.octave {
			t.Errorf("tunerReading(%d, %v) = %+v", c.note, c.cents, r)
		}
	}
}
