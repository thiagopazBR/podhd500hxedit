// Package snapshot holds the snapshots of a POD HD500 preset: the pedal has
// none, so they live in a JSON file next to the editor's data (never inside
// the preset or the .h5e files). A snapshot stores, for the blocks the preset
// already has, the on/off state and the parameter values. It never changes a
// model (that takes ~400 ms and can hit the DSP limit).
//
// The package is pure Go with no pedal or OS dependency: the editor builds
// snapshots with Capture, the live player (cmd/hd500-snap) turns Diff into
// frames. See the design in the project notes (hd500-snapshots-plan).
package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"podhd500-edit/pkg/podhd500"
)

// Count is the number of snapshots per preset.
const Count = 4

// Version is the file format version.
const Version = 1

// Identity names the preset a file belongs to. Slot and name change when the
// user reorders a setlist, so all three are compared (see Matches).
type Identity struct {
	Setlist int    `json:"setlist"`
	Slot    int    `json:"slot"`
	Name    string `json:"name"`
}

// Param is one parameter value of a block (normalized 0..1 like the API).
type Param struct {
	ID    uint32  `json:"id"`
	Value float32 `json:"value"`
}

// Block is the saved state of one block (ids as the API: 0 Amp A, 1 Cab A,
// 2 Amp B, 3 Cab B, 4..11 FX 1..8). Type is the model's type word at capture
// time: a snapshot only touches a block whose current type is unchanged.
type Block struct {
	ID     int     `json:"id"`
	Type   uint32  `json:"type"`
	On     bool    `json:"on"`
	Params []Param `json:"params,omitempty"`
}

// Snapshot is one saved state. Empty means the slot was never saved.
type Snapshot struct {
	Name   string  `json:"name,omitempty"`
	Blocks []Block `json:"blocks,omitempty"`
}

// Empty reports whether the snapshot was never saved.
func (s Snapshot) Empty() bool { return len(s.Blocks) == 0 }

// File is the content of one preset's snapshot file.
type File struct {
	Version   int             `json:"version"`
	Preset    Identity        `json:"preset"`
	Snapshots [Count]Snapshot `json:"snapshots"`
	Variax    json.RawMessage `json:"variax,omitempty"` // reserved for a later phase
}

// New returns an empty file for a preset.
func New(id Identity) *File { return &File{Version: Version, Preset: id} }

// Capture reads the current state of a preset's blocks. Empty blocks and
// blocks over the DSP limit and cabs are skipped.
// Parameters driven by a controller or synced to the tempo are not captured.
// Values are in the API scale (the one Session.SetParam takes), so Diff's
// output can be applied as is.
func Capture(items []podhd500.PresetItem) Snapshot {
	var s Snapshot
	for _, it := range items {
		if it.Empty || it.DSPOver {
			continue
		}
		if it.Kind == "cab" {
			// A cab is "turned off" on the HD500 by choosing the model "No Cab":
			// a model change, which snapshots never do. Its values go through
			// setup ids, not PARAM frames. So cabs are left out.
			continue
		}
		b := Block{ID: it.ID, Type: it.Type, On: it.Active}
		for _, p := range it.Params {
			// A controller (EXP 1/2, Variax knob) owns its parameter: the pedal
			// rewrites the value with the pedal position (as on tuner close).
			// A tempo-synced value is recomputed by the pedal from the tap tempo.
			if p.Ctl != 0 || p.Sync != 0 {
				continue
			}
			b.Params = append(b.Params, Param{ID: p.ID, Value: p.Value})
		}
		s.Blocks = append(s.Blocks, b)
	}
	return s
}

// Save stores a snapshot in slot n (0-based).
func (f *File) Save(n int, s Snapshot) error {
	if n < 0 || n >= Count {
		return fmt.Errorf("snapshot %d out of range 1..%d", n+1, Count)
	}
	if s.Empty() {
		return errors.New("refusing to save an empty snapshot")
	}
	f.Snapshots[n] = s
	return nil
}

// Matches reports whether the file belongs to the preset found at that
// setlist/slot. A false result means the setlist was reordered or the preset
// renamed: the caller warns instead of applying snapshots to a wrong preset.
func (f *File) Matches(id Identity) bool { return f.Preset == id }

// Op is one change the player sends to the pedal: a block on/off, or a
// parameter value.
type Op struct {
	Block   int
	IsParam bool
	On      bool    // when !IsParam
	ParamID uint32  // when IsParam
	Value   float32 // when IsParam
}

// Diff returns the operations that turn the pedal from state cur into next,
// for the blocks present in the live preset (items), and the ids of the blocks
// it left alone because the block is gone or its model changed since the
// snapshot was saved (snapshots never change models). It sends only
// differences, ordered so nothing is heard half-way: blocks turning OFF first,
// then parameter values, then blocks turning ON (a block never switches on
// with its old parameters). cur may be the zero Snapshot (unknown state): then
// every block of next is sent.
func Diff(cur, next Snapshot, items []podhd500.PresetItem) (ops []Op, skipped []int) {
	live := map[int]podhd500.PresetItem{}
	for _, it := range items {
		if !it.Empty && !it.DSPOver {
			live[it.ID] = it
		}
	}
	curBlocks := map[int]Block{}
	for _, b := range cur.Blocks {
		curBlocks[b.ID] = b
	}
	var offs, params, ons []Op
	for _, nb := range next.Blocks {
		it, ok := live[nb.ID]
		if !ok || it.Type != nb.Type {
			skipped = append(skipped, nb.ID)
			continue
		}
		cb, had := curBlocks[nb.ID]
		if !had || cb.On != nb.On {
			if nb.On {
				ons = append(ons, Op{Block: nb.ID, On: true})
			} else {
				offs = append(offs, Op{Block: nb.ID, On: false})
			}
		}
		curVals := map[uint32]float32{}
		if had {
			for _, p := range cb.Params {
				curVals[p.ID] = p.Value
			}
		}
		for _, p := range nb.Params {
			if v, seen := curVals[p.ID]; had && seen && v == p.Value {
				continue
			}
			params = append(params, Op{Block: nb.ID, IsParam: true, ParamID: p.ID, Value: p.Value})
		}
	}
	ops = append(append(offs, params...), ons...)
	return ops, skipped
}

// Path returns the file name for a preset inside dir.
func Path(dir string, id Identity) string {
	return filepath.Join(dir, fmt.Sprintf("set%d-%s.snapshots.json", id.Setlist+1, podhd500.SlotLabel(id.Slot)))
}

// Load reads a snapshot file.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := &File{}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Version != Version {
		return nil, fmt.Errorf("%s: unsupported snapshot file version %d", path, f.Version)
	}
	return f, nil
}

// Write stores the file atomically (temp file + rename).
func (f *File) Write(path string) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
