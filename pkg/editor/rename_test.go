package editor

import "testing"

func TestCleanName(t *testing.T) {
	ok := map[string]string{
		"New Tone":             "New Tone",
		"  Lead  ":             "Lead",
		"1234567890123456":     "1234567890123456",
		"Crunch/Rhythm #2 (B)": "", // 20 chars: too long
	}
	for in, want := range ok {
		got, err := CleanName(in)
		if want == "" {
			if err == nil {
				t.Errorf("CleanName(%q) = %q, want error", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("CleanName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "Ação", "Tab\tName", "12345678901234567"} {
		if _, err := CleanName(bad); err == nil {
			t.Errorf("CleanName(%q) accepted", bad)
		}
	}
}
