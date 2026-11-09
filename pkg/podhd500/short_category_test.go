package podhd500

import "testing"

func TestShortCategory(t *testing.T) {
	for _, c := range []struct{ group, model, want string }{
		{"Distortion", "Screamer", "Dist"},
		{"Dynamics", "Red Comp", "Dyn"},
		{"Modulation", "U-Vibe", "Mod"},
		{"Reverb", "Spring", "Verb"},
		{"Delay", "Ping Pong", "Delay"},
		{"Preamp+EQ", "Mid Focus EQ", "EQ"},
		{"Preamp+EQ", "Vintage Pre", "Preamp"},
		{"Volume/Pan", "Volume", "Vol"},
		{"Volume/Pan", "Pan", "Pan"},
		{"FX Loop", "FX Loop", "FX Loop"},
		{"None", "None", ""},
	} {
		if got := ShortCategory(c.group, c.model); got != c.want {
			t.Errorf("%s / %s: got %q, want %q", c.group, c.model, got, c.want)
		}
	}
}

func TestAssignLabel(t *testing.T) {
	p := func(ctls ...uint8) []ItemParam {
		var out []ItemParam
		for _, c := range ctls {
			out = append(out, ItemParam{Ctl: c})
		}
		return out
	}
	for _, c := range []struct {
		fs     uint8
		params []ItemParam
		want   string
	}{
		{0, nil, ""},
		{3, p(0, 0), "FS3"},
		{0, p(2), "EXP 2"},
		{9, nil, "EXP Toe"},
		{9, p(1), "EXP 1 · Toe"},
		{9, p(0, 2), "EXP 2 · Toe"},
		{1, p(1, 2), "FS1 · EXP 1+2"},
	} {
		if got := AssignLabel(c.fs, c.params); got != c.want {
			t.Errorf("fs %d: got %q, want %q", c.fs, got, c.want)
		}
	}
}
