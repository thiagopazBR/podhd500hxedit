package podhd500

import "encoding/binary"

// Setup ids (SETUP_CHANGE 0x16) for the preset's input section.
const (
	SetupInput1Source uint32 = 0x36
	SetupInput2Source uint32 = 0x37
	SetupGuitarInZ    uint32 = 0x55
)

// GlobalInputsSetup is the pedal-wide STATUS id (0x20 set / 0x21 query) of
// Setup > Inputs > "Inputs Setup". Found 2026-11-03 by toggling it on the
// pedal and diffing all STATUS ids: Global = 1, Preset = 0.
const GlobalInputsSetup uint32 = 0x0E

// Pedal-wide input settings used instead of the preset's when Inputs Setup is
// Global. Confirmed on 2026-11-04 by watching the pedal's LCD (webcam):
// switching to Global showed Variax/Guitar = the values of 0x0F/0x10, and
// writing 3 to 0x11 made Guitar In-Z show 70K. Same value tables as the
// preset settings.
const (
	GlobalInput1Source uint32 = 0x0F
	GlobalInput2Source uint32 = 0x10
	GlobalGuitarInZ    uint32 = 0x11
)

// InputsSetupOptions in the original editor's order.
var InputsSetupOptions = []Option{{1, "Global"}, {0, "Preset"}}

// Option is one entry of a setting's list: the raw value stored in the
// preset / sent to the pedal, and its label as shown by the original editor.
type Option struct {
	Value uint32 `json:"value"`
	Label string `json:"label"`
}

// Raw values from refs/hd500-editor/docs/preset-format.md (value 4 unused).
var (
	Input1Options = []Option{
		{1, "Guitar"}, {2, "Mic"}, {3, "Aux"}, {5, "Guitar+Aux"},
		{6, "Guitar+Variax"}, {7, "Guitar+Aux+Variax"}, {8, "Variax"}, {9, "Variax Mags"},
	}
	Input2Options = append([]Option{{0, "Same"}}, Input1Options...)
	InZOptions    = func() []Option {
		o := make([]Option, len(InZNames))
		for i, n := range InZNames {
			o[i] = Option{uint32(i), n}
		}
		return o
	}()
)

// ValidOption reports whether v is one of opts' values.
func ValidOption(opts []Option, v uint32) bool {
	for _, o := range opts {
		if o.Value == v {
			return true
		}
	}
	return false
}

// SetupValue reads the raw value of a setup id from the preset body, using
// the id -> offset table extracted from the official editor.
func (p *Preset) SetupValue(id uint32) (uint32, bool) {
	s, ok := LookupSetup(id)
	if !ok || s.Offset < 0 || s.Offset+s.Size > PresetBodySize {
		return 0, false
	}
	switch s.Size {
	case 1:
		return uint32(p.Body[s.Offset]), true
	case 4:
		return binary.LittleEndian.Uint32(p.Body[s.Offset : s.Offset+4]), true
	default:
		return 0, false
	}
}

// SetSetupValue writes the raw value of a setup id into the preset body.
func (p *Preset) SetSetupValue(id, v uint32) bool {
	s, ok := LookupSetup(id)
	if !ok || s.Offset < 0 || s.Offset+s.Size > PresetBodySize {
		return false
	}
	switch s.Size {
	case 1:
		p.Body[s.Offset] = byte(v)
	case 4:
		binary.LittleEndian.PutUint32(p.Body[s.Offset:s.Offset+4], v)
	default:
		return false
	}
	return true
}
