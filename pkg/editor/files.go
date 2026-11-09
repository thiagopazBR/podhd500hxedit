package editor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// Export / import of the original editor's files (.h5e preset, .h5s setlist,
// .h5b bundle) and full backups of the pedal.
//   - Export writes what the editor shows: pending changes included, and the
//     loaded preset's edit buffer when it has unsaved edits.
//   - Backup re-reads every stored preset from the pedal (ignoring pending
//     changes) and saves a .h5b in BackupDir.
//   - Import only changes the editor's copy (asterisk); SEND writes it.

// ExportRequest selects what to export.
type ExportRequest struct {
	Kind    podhd500.FileKind `json:"kind"`
	Setlist int               `json:"setlist"`
	Slot    int               `json:"slot"`
}

// ensureLoaded reads the given setlists from the pedal if they were not yet
// read, waiting for them.
func (s *Session) ensureLoaded(sets ...int) error {
	for _, set := range sets {
		if _, err := s.Setlist(set); err != nil { // starts reading if needed
			return err
		}
		for {
			s.lib.mu.Lock()
			done, ch := s.lib.loaded[set], s.lib.loading[set]
			s.lib.mu.Unlock()
			if done {
				break
			}
			if ch == nil {
				return fmt.Errorf("setlist %d wasn't fully read from the pedal (try again)", set+1)
			}
			<-ch
		}
	}
	return nil
}

// editorBody returns the editor's version of a slot; the loaded preset with
// unsaved edits comes from the pedal's edit buffer.
func (s *Session) editorBody(set, slot int) ([]byte, error) {
	st := s.State()
	if st.Dirty && set == st.Setlist && slot == st.Slot {
		m, err := s.requestRetry(usb.BuildPresetQuery(usb.CurrentPresetAll, usb.CurrentSetAll), usb.CmdPresetLoad, 3*time.Second)
		if err != nil {
			return nil, fmt.Errorf("reading the loaded preset: %w", err)
		}
		return append([]byte(nil), m.Payload...), nil
	}
	s.lib.mu.Lock()
	defer s.lib.mu.Unlock()
	b := s.lib.edited[set][slot]
	if b == nil {
		b = s.lib.slots[set][slot]
	}
	if b == nil {
		return nil, fmt.Errorf("%s hasn't been read from the pedal yet", slotRef{set, slot}.label())
	}
	return b, nil
}

func (s *Session) editorSetlist(set int) ([][]byte, error) {
	if err := s.ensureLoaded(set); err != nil {
		return nil, err
	}
	out := make([][]byte, SlotsPerSetlst)
	for i := range out {
		b, err := s.editorBody(set, i)
		if err != nil {
			return nil, err
		}
		out[i] = b
	}
	return out, nil
}

// setlistNames returns the editor's names (pending renames included), or
// the pedal's when stored is set.
func (s *Session) setlistNames(stored bool) ([]string, error) {
	l, err := s.Setlists()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(l))
	for i, x := range l {
		names[i] = x.Name
		if stored {
			names[i] = x.Stored
		}
	}
	return names, nil
}

// Export builds a file; it returns the content and a suggested file name.
func (s *Session) Export(req ExportRequest) ([]byte, string, error) {
	if s.client == nil {
		return nil, "", ErrOffline
	}
	switch req.Kind {
	case podhd500.KindPreset:
		if !validPos(SlotPos{req.Setlist, req.Slot}) {
			return nil, "", errors.New("invalid position")
		}
		if err := s.ensureLoaded(req.Setlist); err != nil {
			return nil, "", err
		}
		b, err := s.editorBody(req.Setlist, req.Slot)
		if err != nil {
			return nil, "", err
		}
		p, err := podhd500.ParsePreset(b)
		if err != nil {
			return nil, "", err
		}
		f, err := podhd500.EncodePreset(b)
		return f, fileName(p.Name(), "Preset") + ".h5e", err

	case podhd500.KindSetlist:
		if req.Setlist < 0 || req.Setlist >= NumSetlists {
			return nil, "", errors.New("invalid setlist")
		}
		names, err := s.setlistNames(false)
		if err != nil {
			return nil, "", err
		}
		bodies, err := s.editorSetlist(req.Setlist)
		if err != nil {
			return nil, "", err
		}
		f, err := podhd500.EncodeSetlist(names[req.Setlist], bodies)
		return f, fileName(names[req.Setlist], fmt.Sprintf("Setlist %d", req.Setlist+1)) + ".h5s", err

	case podhd500.KindBundle:
		names, err := s.setlistNames(false)
		if err != nil {
			return nil, "", err
		}
		sets := make([][][]byte, NumSetlists)
		for i := range sets {
			if sets[i], err = s.editorSetlist(i); err != nil {
				return nil, "", err
			}
		}
		f, err := podhd500.EncodeBundle(names, sets)
		return f, "POD HD500 Bundle.h5b", err
	}
	return nil, "", fmt.Errorf("invalid type %q", req.Kind)
}

// fileName makes a name safe for macOS file names.
func fileName(name, fallback string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '/' || r == ':' || r < 0x20 {
			return '-'
		}
		return r
	}, name))
	name = strings.TrimLeft(name, ".")
	if name == "" {
		return fallback
	}
	return name
}

// BackupDir is where full backups are saved.
func BackupDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "Documents", "POD HD500 Edit X", "Backups")
}

// BackupResult tells where the backup went.
type BackupResult struct {
	Path    string `json:"path"`
	Presets int    `json:"presets"`
}

// Backup re-reads all 512 stored presets and the setlist names from the pedal
// (about 35 s) and saves them as a .h5b in BackupDir. Pending changes in the
// editor are kept but not included: the backup is what the pedal holds.
func (s *Session) Backup() (BackupResult, error) {
	var res BackupResult
	if s.client == nil {
		return res, ErrOffline
	}
	if !s.opMu.TryLock() {
		return res, ErrBusy
	}
	defer s.opMu.Unlock()
	defer s.update(func(st *State) { st.Loading = nil; st.LibraryRev++ })

	s.lib.mu.Lock()
	s.lib.namesLoaded = false
	s.lib.mu.Unlock()
	names, err := s.setlistNames(true)
	if err != nil {
		return res, err
	}
	sets := make([][][]byte, NumSetlists)
	total := NumSetlists * SlotsPerSetlst
	for set := range sets {
		sets[set] = make([][]byte, SlotsPerSetlst)
		for slot := range sets[set] {
			s.progress("Backing up the pedal", set, set*SlotsPerSetlst+slot, total)
			m, err := s.requestRetry(usb.BuildPresetQuery(uint16(slot), uint16(set)), usb.CmdPresetLoad, 3*time.Second)
			if err != nil || len(m.Payload) != podhd500.PresetBodySize {
				return res, fmt.Errorf("backup stopped: %s didn't respond", slotRef{set, slot}.label())
			}
			body := append([]byte(nil), m.Payload...)
			sets[set][slot] = body
			s.cacheSlot(set, slot, body)
		}
		s.lib.mu.Lock()
		s.lib.loaded[set] = true
		s.lib.mu.Unlock()
	}
	data, err := podhd500.EncodeBundle(names, sets)
	if err != nil {
		return res, err
	}
	dir := BackupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	res.Path = filepath.Join(dir, "Backup "+time.Now().Format("2006-01-02 15-04-05")+".h5b")
	if err := os.WriteFile(res.Path, data, 0o644); err != nil {
		return res, err
	}
	res.Presets = total
	return res, nil
}

// ImportResult summarizes an import.
type ImportResult struct {
	Kind     podhd500.FileKind `json:"kind"`
	Presets  int               `json:"presets"`  // slots now pending (asterisk)
	Same     int               `json:"same"`     // slots identical to the pedal (nothing to send)
	SetNames []string          `json:"setNames"` // setlist names in the file
	Renamed  int               `json:"renamed"`  // setlist names now pending (differ from the pedal)
	HD500X   bool              `json:"hd500x,omitempty"`
}

// Import puts files into the editor's copy of the presets:
//   - presets (.h5e, one or more): consecutive slots starting at To;
//   - a setlist (.h5s): the 64 slots of To.Setlist;
//   - a bundle (.h5b): all 8 setlists (To is ignored).
func (s *Session) Import(files [][]byte, to SlotPos) (ImportResult, error) {
	var res ImportResult
	if s.client == nil {
		return res, ErrOffline
	}
	if len(files) == 0 {
		return res, errors.New("no file")
	}
	var decoded []*podhd500.File
	for i, data := range files {
		f, err := podhd500.Decode(data)
		if err != nil {
			return res, fmt.Errorf("file %d: %w", i+1, err)
		}
		if f.Model == podhd500.FileModelHD500X {
			res.HD500X = true
		}
		decoded = append(decoded, f)
	}
	res.Kind = decoded[0].Kind
	if len(decoded) > 1 && res.Kind != podhd500.KindPreset {
		return res, errors.New("setlists (.h5s) and bundles (.h5b) are imported one at a time")
	}

	type put struct {
		pos  SlotPos
		body []byte
	}
	var puts []put
	switch res.Kind {
	case podhd500.KindPreset:
		if !validPos(to) {
			return res, errors.New("select the destination slot")
		}
		for _, f := range decoded {
			if f.Kind != podhd500.KindPreset {
				return res, errors.New("can't mix presets with a setlist/bundle")
			}
		}
		if to.Slot+len(decoded) > SlotsPerSetlst {
			return res, fmt.Errorf("doesn't fit: %d preset(s) starting at %s go past slot 16D", len(decoded), slotName(to.Slot))
		}
		for i, f := range decoded {
			puts = append(puts, put{SlotPos{to.Setlist, to.Slot + i}, f.Bodies[0][0]})
		}
		res.SetNames = []string{}
	case podhd500.KindSetlist:
		if to.Setlist < 0 || to.Setlist >= NumSetlists {
			return res, errors.New("invalid destination setlist")
		}
		for i, b := range decoded[0].Bodies[0] {
			puts = append(puts, put{SlotPos{to.Setlist, i}, b})
		}
		res.SetNames = decoded[0].SetNames
	case podhd500.KindBundle:
		for set, bodies := range decoded[0].Bodies {
			for i, b := range bodies {
				puts = append(puts, put{SlotPos{set, i}, b})
			}
		}
		res.SetNames = decoded[0].SetNames
	}

	st := s.State()
	sets := map[int]bool{}
	for _, p := range puts {
		if st.Dirty && p.pos.Setlist == st.Setlist && p.pos.Slot == st.Slot {
			return res, ErrLoadedDirty
		}
		sets[p.pos.Setlist] = true
	}
	for set := 0; set < NumSetlists; set++ { // the pedal's version is needed to tell what changed
		if sets[set] {
			if err := s.ensureLoaded(set); err != nil {
				return res, err
			}
		}
	}

	// Setlist names from the file (pending too). Names the pedal can't show
	// (empty, too long, accents) are skipped.
	type rename struct {
		set  int
		name string
	}
	var renames []rename
	if res.Kind != podhd500.KindPreset {
		if _, err := s.Setlists(); err != nil {
			return res, err
		}
		for i, n := range res.SetNames {
			set := i
			if res.Kind == podhd500.KindSetlist {
				set = to.Setlist
			}
			if n, err := CleanName(n); err == nil {
				renames = append(renames, rename{set, n})
			}
		}
	}

	s.lib.mu.Lock()
	for _, p := range puts {
		s.setEffectiveLocked(p.pos.Setlist, p.pos.Slot, p.body)
		if s.lib.edited[p.pos.Setlist][p.pos.Slot] == nil {
			res.Same++
		} else {
			res.Presets++
		}
	}
	s.lib.mu.Unlock()
	for _, r := range renames {
		s.setPendingName(r.set, r.name)
		s.lib.mu.Lock()
		if s.lib.editedName[r.set] != "" {
			res.Renamed++
		}
		s.lib.mu.Unlock()
	}
	s.update(func(st *State) { st.LibraryRev++ })
	return res, nil
}
