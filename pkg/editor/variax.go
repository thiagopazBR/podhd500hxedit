package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// Variax (VDI input). Learned from the pedal's own events and the original
// editor's Mixer view (2026-11-06):
//   - Variax Control: pedal-wide STATUS 0x12, 1 = Global, 0 = Preset;
//   - Model: preset SETUP id 0 (one byte), the index into the model list of
//     the instrument type (0 = "Don't Force"); verified on the LCD with
//     value 10 = "F-T-Model-1" on a Variax 300/600;
//   - Tone knob: preset SETUP id 1, 0..127 (127 shown as 100%).
// The VDI Input (instrument type) exists only in the software, to pick the
// model list; only the 300/600 500/700 list is known so far.

// GlobalVariaxControl is the pedal-wide STATUS id of Variax Control.
const GlobalVariaxControl uint32 = 0x12

const (
	setupVariaxModel = 0
	setupVariaxTone  = 1
	VariaxToneMax    = 127
)

// VariaxControlOptions in the original editor's order.
var VariaxControlOptions = []podhd500.Option{{Value: 1, Label: "Global"}, {Value: 0, Label: "Preset"}}

// VariaxView is the preset's Variax settings.
type VariaxView struct {
	Model int `json:"model"` // index into the instrument's model list
	Tone  int `json:"tone"`  // 0..127
}

func variaxView(p *podhd500.Preset) VariaxView {
	m, _ := p.SetupValue(setupVariaxModel)
	t, _ := p.SetupValue(setupVariaxTone)
	return VariaxView{Model: int(m), Tone: int(t)}
}

var (
	variaxOnce   sync.Once
	variaxModels map[string][]string
)

// VariaxModels returns the model lists by VDI Input type
// (data/variax-models.json).
func VariaxModels() map[string][]string {
	variaxOnce.Do(func() {
		variaxModels = map[string][]string{}
		b, err := podhd500.EmbeddedData.ReadFile("data/variax-models.json")
		if err != nil {
			return
		}
		var raw map[string]json.RawMessage
		if json.Unmarshal(b, &raw) != nil {
			return
		}
		for k, v := range raw {
			var list []string
			if k != "_doc" && json.Unmarshal(v, &list) == nil {
				variaxModels[k] = list
			}
		}
	})
	return variaxModels
}

// SetVariax changes a Variax setting of the edit buffer: "model" (index
// into the model list) or "tone" (0..127).
func (s *Session) SetVariax(param string, value int) error {
	if s.client == nil {
		return ErrOffline
	}
	var id uint32
	switch param {
	case "model":
		id = setupVariaxModel
		max := 0
		for _, l := range VariaxModels() {
			if len(l) > max {
				max = len(l)
			}
		}
		if value < 0 || value >= max {
			return fmt.Errorf("invalid Variax model %d", value)
		}
	case "tone":
		id = setupVariaxTone
		if value < 0 || value > VariaxToneMax {
			return fmt.Errorf("invalid tone %d (0..%d)", value, VariaxToneMax)
		}
	default:
		return fmt.Errorf("invalid Variax parameter %q", param)
	}
	if err := s.send(usb.BuildSetupChangeInt(id, uint32(value))); err != nil {
		return err
	}
	s.editBuffer(func(p *podhd500.Preset) bool { return p.SetSetupValue(id, uint32(value)) })
	s.update(func(st *State) { st.Dirty = true })
	return nil
}

// SetVariaxControl writes the pedal-wide Variax Control (1 Global, 0 Preset)
// and reads it back.
func (s *Session) SetVariaxControl(value uint32) error {
	if !podhd500.ValidOption(VariaxControlOptions, value) {
		return fmt.Errorf("invalid value %d for Variax Control", value)
	}
	if s.client == nil {
		return ErrOffline
	}
	if err := s.send(usb.BuildStatusSet(GlobalVariaxControl, value)); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	got, ok := s.readGlobal(GlobalVariaxControl)
	if !ok {
		return errors.New("verifying Variax Control: no response")
	}
	s.update(func(st *State) { st.VariaxControl = got })
	if got != value {
		return fmt.Errorf("the pedal kept Variax Control = %d (requested %d)", got, value)
	}
	return nil
}
