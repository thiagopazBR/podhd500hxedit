package podhd500

import (
	"encoding/binary"
	"math"
)

// Bypass and controller assignments, as the original editor's CONTROLLERS
// page offers them:
//   - FS ASSIGN (per block, byte 12): None, FS1–FS8, EXP Toe Switch;
//   - CONTROLLER (per parameter, byte 16 of its entry): Off, EXP-1, EXP-2,
//     Variax Vol, Variax Tone; with a controller, the parameter moves between
//     its Minimum (heel, float at +8) and Maximum (toe, float at +12).
// Codes 0–2 and FS 0–9 seen in factory presets and on the LCD; 3 is used by
// a few factory presets (Variax Vol); 4 (Variax Tone) by none.

// AssignOption is one entry of the bypass / controller lists: the raw code,
// its label and the MIDI CC that drives the same control from the MIDI IN
// port. The HD500's MIDI map is fixed (Advanced Guide, Appendix B: CC 1/2 =
// EXP-1/2, CC 51–58 = FS1–FS8, CC 59 = EXP toe switch); 0 = none.
type AssignOption struct {
	Value uint32 `json:"value"`
	Label string `json:"label"`
	CC    int    `json:"cc,omitempty"`
}

// Option lists for the API.
var (
	BypassOptions = []AssignOption{
		{0, "None", 0}, {1, "FS1", 51}, {2, "FS2", 52}, {3, "FS3", 53}, {4, "FS4", 54},
		{5, "FS5", 55}, {6, "FS6", 56}, {7, "FS7", 57}, {8, "FS8", 58}, {9, "EXP Toe Switch", 59},
	}
	ControllerOptions = []AssignOption{
		{0, "None", 0}, {1, "EXP 1", 1}, {2, "EXP 2", 2}, {3, "Variax Vol", 0}, {4, "Variax Tone", 0},
	}
)

// ValidAssign reports whether v is one of the list's codes.
func ValidAssign(list []AssignOption, v uint32) bool {
	for _, o := range list {
		if o.Value == v {
			return true
		}
	}
	return false
}

const itemFSByte = 12

func paramEntry(p *Preset, id, k int) int {
	return ItemOffset(id) + ParamBase + k*ParamSize
}

// SetItemFS sets the footswitch that turns block id on/off (0 = none).
func (p *Preset) SetItemFS(id int, fs uint8) bool {
	o := ItemOffset(id) + itemFSByte
	if p.Body[o] == fs {
		return false
	}
	p.Body[o] = fs
	return true
}

// SetParamController sets the controller of parameter k (index in the
// block's list) of block id.
func (p *Preset) SetParamController(id, k int, ctl uint8) bool {
	if k < 0 || k >= MaxParams {
		return false
	}
	q := paramEntry(p, id, k) + 16
	if p.Body[q] == ctl {
		return false
	}
	p.Body[q] = ctl
	return true
}

// SetParamRange sets the heel (min) or toe (max) value of parameter k.
func (p *Preset) SetParamRange(id, k int, toe bool, v float32) bool {
	if k < 0 || k >= MaxParams {
		return false
	}
	q := paramEntry(p, id, k) + 8
	if toe {
		q += 4
	}
	bits := math.Float32bits(v)
	if binary.LittleEndian.Uint32(p.Body[q:]) == bits {
		return false
	}
	binary.LittleEndian.PutUint32(p.Body[q:], bits)
	return true
}

// ClearDSPOver turns FX blocks the pedal couldn't run (DSP limit reached:
// model kept, not in use) into empty slots (None). Sending a buffer that
// still holds such a block back to the pedal garbles its LCD (user-reported
// 2026-11-08), so every buffer rewrite clears them first; a new attempt to
// load a model then gets a fresh DSP check. Reports whether anything changed.
func (p *Preset) ClearDSPOver() bool {
	changed := false
	for id := 4; id <= 11; id++ {
		if p.Item(id).DSPOver {
			changed = p.SetFXNone(id) || changed
		}
	}
	return changed
}

// SetFXNone makes FX block id an empty slot exactly as the pedal stores one
// (type 0x020DFFFF, off, not in use).
func (p *Preset) SetFXNone(id int) bool {
	o := ItemOffset(id)
	if binary.LittleEndian.Uint32(p.Body[o:]) == NoFXType && p.Body[o+8] == 0 && p.Body[o+11] == 0 {
		return false
	}
	binary.LittleEndian.PutUint32(p.Body[o:], NoFXType)
	p.Body[o+8], p.Body[o+11] = 0, 0
	return true
}
