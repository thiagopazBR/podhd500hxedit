package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/snapshot"
)

// Snapshots: the pedal has none, so they are kept in JSON files (pkg/snapshot)
// and applied as plain block on/off and parameter changes. Everything goes
// through SetBlockActive and SetParam (never raw frames): SetParam routes
// Smart Harmony ids to integer frames, and a float frame to them hangs the pedal.
//
// The editor never applies a snapshot by itself (browsing presets must not
// write to the pedal): applying is a request from the UI or from the live
// player. Applying a snapshot does NOT mark the preset dirty (no asterisk):
// the user decided snapshots are a live layer over the stored preset, and
// "Save to Pedal" only sends presets that are dirty. If the preset was already
// dirty it stays so.

// DefaultSnapshotDir is where the editor keeps the snapshot files and the
// feature switch.
func DefaultSnapshotDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "POD HD500 Edit X", "snapshots")
}

func (s *Session) snapshotDir() string {
	if s.snapDir != "" {
		return s.snapDir
	}
	return DefaultSnapshotDir()
}

// SetSnapshotPlayer makes this session a live snapshot player: snapshots are
// read from dir (default: the editor's folder) and play without the editor's
// switch. Call it once, before using the session.
func (s *Session) SetSnapshotPlayer(dir string) {
	s.snapDir = dir
	s.snapPlayer = true
}

var snapSettingsMu sync.Mutex

type snapSettings struct {
	Enabled bool `json:"enabled"`
}

func (s *Session) snapSettingsPath() string { return filepath.Join(s.snapshotDir(), "settings.json") }

// SnapshotsEnabled reports the feature switch (off by default).
func (s *Session) SnapshotsEnabled() bool {
	if s.snapPlayer {
		return true
	}
	snapSettingsMu.Lock()
	defer snapSettingsMu.Unlock()
	data, err := os.ReadFile(s.snapSettingsPath())
	if err != nil {
		return false
	}
	var st snapSettings
	return json.Unmarshal(data, &st) == nil && st.Enabled
}

// SetSnapshotsEnabled turns the feature on or off. Nothing is deleted when
// it goes off.
func (s *Session) SetSnapshotsEnabled(on bool) error {
	snapSettingsMu.Lock()
	defer snapSettingsMu.Unlock()
	if err := os.MkdirAll(s.snapshotDir(), 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(snapSettings{Enabled: on})
	return os.WriteFile(s.snapSettingsPath(), append(data, '\n'), 0o644)
}

// SnapshotSlot is one slot as the UI shows it.
type SnapshotSlot struct {
	Saved bool   `json:"saved"`
	Name  string `json:"name,omitempty"`
}

// SnapshotsView is the snapshot state of the loaded preset.
type SnapshotsView struct {
	Enabled  bool                         `json:"enabled"`
	Preset   snapshot.Identity            `json:"preset"`
	Slots    [snapshot.Count]SnapshotSlot `json:"slots"`
	Mismatch string                       `json:"mismatch,omitempty"` // a file exists for this slot but names another preset
}

// otherPreset describes the preset a snapshot file names, for messages.
func otherPreset(id snapshot.Identity) string {
	return fmt.Sprintf("%q (setlist %d, slot %s)", id.Name, id.Setlist+1, podhd500.SlotLabel(id.Slot))
}

func (s *Session) snapshotIdentity() snapshot.Identity {
	st := s.State()
	return snapshot.Identity{Setlist: st.Setlist, Slot: st.Slot, Name: st.PresetName}
}

func (s *Session) loadSnapshotFile(id snapshot.Identity) (*snapshot.File, error) {
	f, err := snapshot.Load(snapshot.Path(s.snapshotDir(), id))
	if errors.Is(err, os.ErrNotExist) {
		return snapshot.New(id), nil
	}
	return f, err
}

// Snapshots describes the snapshots of the loaded preset.
func (s *Session) Snapshots() (SnapshotsView, error) {
	v := SnapshotsView{Enabled: s.SnapshotsEnabled(), Preset: s.snapshotIdentity()}
	if !v.Enabled {
		return v, nil
	}
	f, err := s.loadSnapshotFile(v.Preset)
	if err != nil {
		return v, err
	}
	if !f.Matches(v.Preset) {
		v.Mismatch = "these snapshots belong to " + otherPreset(f.Preset) + ", not to this preset"
		return v, nil
	}
	for i, sn := range f.Snapshots {
		v.Slots[i] = SnapshotSlot{Saved: !sn.Empty(), Name: sn.Name}
	}
	return v, nil
}

func (s *Session) liveItems() ([]podhd500.PresetItem, error) {
	b := s.BufferCopy()
	if b == nil {
		return nil, errors.New("no preset loaded")
	}
	p, err := podhd500.ParsePreset(b)
	if err != nil {
		return nil, err
	}
	return p.Items(), nil
}

func checkSnapshotSlot(n int) error {
	if n < 1 || n > snapshot.Count {
		return fmt.Errorf("snapshot %d out of range 1..%d", n, snapshot.Count)
	}
	return nil
}

// SaveSnapshot stores the current state of the loaded preset in slot n (1..4).
// It reads the edit buffer copy, so nothing is sent to the pedal. If the file
// of this slot names another preset (renamed or moved), the save is refused
// unless replace is true: replacing deletes that file's other snapshots.
func (s *Session) SaveSnapshot(n int, name string, replace bool) error {
	if err := checkSnapshotSlot(n); err != nil {
		return err
	}
	if !s.SnapshotsEnabled() {
		return errors.New("snapshots are turned off")
	}
	items, err := s.liveItems()
	if err != nil {
		return err
	}
	id := s.snapshotIdentity()
	f, err := s.loadSnapshotFile(id)
	if err != nil {
		return err
	}
	if !f.Matches(id) {
		if !replace {
			return fmt.Errorf("the snapshots in this slot belong to %s; saving would replace them", otherPreset(f.Preset))
		}
		f = snapshot.New(id)
	}
	sn := snapshot.Capture(items)
	sn.Name = name
	if err := f.Save(n-1, sn); err != nil {
		return err
	}
	return f.Write(snapshot.Path(s.snapshotDir(), id))
}

// Errors the live player tells apart from real failures.
var (
	ErrSnapshotEmpty    = errors.New("snapshot is empty")
	ErrSnapshotMismatch = errors.New("snapshots belong to another preset")
)

// SnapshotResult is what ApplySnapshot did.
type SnapshotResult struct {
	Changes int   `json:"changes"`           // frames sent (also on a partial failure)
	Skipped []int `json:"skipped,omitempty"` // blocks left alone: gone or another model since the save
}

// ApplySnapshot sends the differences between the pedal's current state and
// snapshot n (1..4). A snapshot saved for another preset is refused. On a
// failure half-way, the result still says how many changes were sent.
func (s *Session) ApplySnapshot(n int) (SnapshotResult, error) {
	var res SnapshotResult
	if err := checkSnapshotSlot(n); err != nil {
		return res, err
	}
	if !s.SnapshotsEnabled() {
		return res, errors.New("snapshots are turned off")
	}
	if s.cli() == nil {
		return res, ErrOffline
	}
	id := s.snapshotIdentity()
	f, err := s.loadSnapshotFile(id)
	if err != nil {
		return res, err
	}
	if !f.Matches(id) {
		return res, fmt.Errorf("%w: these snapshots belong to %s, not to this preset", ErrSnapshotMismatch, otherPreset(f.Preset))
	}
	target := f.Snapshots[n-1]
	if target.Empty() {
		return res, fmt.Errorf("%w (snapshot %d)", ErrSnapshotEmpty, n)
	}
	items, err := s.liveItems()
	if err != nil {
		return res, err
	}
	ops, skipped := snapshot.Diff(snapshot.Capture(items), target, items)
	res.Skipped = skipped
	// SetParam / SetBlockActive mark the preset dirty; a snapshot must not
	wasDirty := s.State().Dirty
	defer func() {
		if !wasDirty {
			s.update(func(st *State) { st.Dirty = false })
		}
	}()
	for _, op := range ops {
		if op.IsParam {
			err = s.SetParam(op.Block, op.ParamID, op.Value)
		} else {
			err = s.SetBlockActive(op.Block, op.On)
		}
		if err != nil {
			return res, err
		}
		res.Changes++
	}
	return res, nil
}

// SnapshotMIDI sets what one footswitch sends to trigger a snapshot.
type SnapshotMIDI struct {
	Control int   `json:"control"` // MIDI Assign control id (6..9 = FS5..FS8)
	Number  uint8 `json:"number"`  // CC number
}

// DefaultSnapshotMIDI: FS5..FS8 send CC 80..83 (value 127) on the base channel.
var DefaultSnapshotMIDI = []SnapshotMIDI{{6, 80}, {7, 81}, {8, 82}, {9, 83}}

// PrepareSnapshotMIDI writes the MIDI Assign of the footswitches that trigger
// snapshots into the loaded preset's edit buffer (message CC, base channel,
// value 127). Controls and numbers are inputs: whether the bottom row sends
// MIDI depends on the pedal's footswitch mode, to be checked on the LCD.
func (s *Session) PrepareSnapshotMIDI(sw []SnapshotMIDI) error {
	if !s.SnapshotsEnabled() {
		return errors.New("snapshots are turned off")
	}
	if len(sw) == 0 {
		sw = DefaultSnapshotMIDI
	}
	if len(sw) > snapshot.Count {
		return fmt.Errorf("at most %d footswitches", snapshot.Count)
	}
	msg, ch, val := uint8(1), uint8(podhd500.MIDIChannelBase), uint8(127)
	for _, x := range sw {
		num := x.Number
		if num > 127 {
			return fmt.Errorf("invalid CC number %d", num)
		}
		if err := s.SetMIDIAssign(x.Control, MIDIAssignChange{Message: &msg, Channel: &ch, Number: &num, Value: &val}); err != nil {
			return err
		}
	}
	return nil
}
