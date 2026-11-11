package editor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// GlobalTempoSync is the pedal-wide STATUS id of Setup > MIDI/Tempo >
// "Tempo Sync": 1 = Global (one tempo for every preset), 0 = Preset (each
// preset's own tempo). Found 2026-11-07 in the debug log while the user
// switched it on the pedal (LCD-verified: PRESET = 0); switching makes the
// pedal send setup 0x17 with the tempo now in use.
const GlobalTempoSync uint32 = 0x0B

// TempoSyncOptions in the LCD's order.
var TempoSyncOptions = []podhd500.Option{{Value: 1, Label: "Global"}, {Value: 0, Label: "Preset"}}

// SetTempoSource writes "Tempo Sync" (1 Global, 0 Preset) and reads it back.
// The pedal then reports the tempo in use (setup 0x17), which updates
// State.Tempo.
func (s *Session) SetTempoSource(value uint32) error {
	if s.cli() == nil {
		return ErrOffline
	}
	if !podhd500.ValidOption(TempoSyncOptions, value) {
		return fmt.Errorf("invalid value %d for Tempo Sync", value)
	}
	if err := s.send(usb.BuildStatusSet(GlobalTempoSync, value)); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	got, ok := s.readGlobal(GlobalTempoSync)
	if !ok {
		return fmt.Errorf("verifying Tempo Sync: no answer")
	}
	s.update(func(st *State) {
		st.TempoSync = got
		st.Tempo = st.tempoInUse(st.presetTempo)
	})
	if got != value {
		return fmt.Errorf("the pedal kept Tempo Sync = %d (requested %d)", got, value)
	}
	return nil
}

// The pedal-wide tempo can be written over USB (setup 0x17 while Tempo Sync
// is Global sets it, LCD-verified) but not read: it is not in the edit
// buffer nor in any STATUS id, and switching Tempo Sync over USB doesn't
// make the pedal report it (switching on the front panel does). So the
// editor remembers the last value it saw (pedal events, its own writes) in
// a small file, and shows no tempo while it doesn't know it.
func globalTempoFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "POD HD500 Edit X", "global-tempo.json")
}

func loadGlobalTempo() float32 {
	f := globalTempoFile()
	if f == "" {
		return 0
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return 0
	}
	var v struct {
		BPM float32 `json:"bpm"`
	}
	if json.Unmarshal(b, &v) != nil || v.BPM < MinTempo || v.BPM > MaxTempo {
		return 0
	}
	return v.BPM
}

// learnGlobalTempo records the pedal-wide tempo (in State and on disk).
func (s *Session) learnGlobalTempo(bpm float32) {
	if bpm < MinTempo || bpm > MaxTempo {
		return
	}
	s.update(func(st *State) {
		st.globalTempo = bpm
		if st.TempoSync == 1 {
			st.Tempo = bpm
		}
	})
	if f := globalTempoFile(); f != "" {
		_ = os.MkdirAll(filepath.Dir(f), 0o755)
		b, _ := json.Marshal(map[string]float32{"bpm": bpm})
		_ = os.WriteFile(f, b, 0o644)
	}
}

// tempoInUse: what the pedal plays at — the preset's tempo, or the
// pedal-wide one with Tempo Sync = Global (0 = not known yet).
func (st *State) tempoInUse(presetTempo float32) float32 {
	if st.TempoSync == 1 {
		return st.globalTempo
	}
	return presetTempo
}
