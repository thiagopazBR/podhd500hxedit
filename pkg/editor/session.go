// Package editor holds the editor's live view of the POD HD500: one Session
// owns the USB client, routes replies to the request that asked for them,
// turns unsolicited pedal messages into state updates, and fans the state out
// to subscribers (the UI).
package editor

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// State is what the UI shows. Indices are 0-based; labels are pre-formatted.
type State struct {
	Connected   bool    `json:"connected"`
	Error       string  `json:"error,omitempty"`
	Setlist     int     `json:"setlist"`
	SetlistName string  `json:"setlistName"`
	Slot        int     `json:"slot"`
	SlotLabel   string  `json:"slotLabel"`
	PresetName  string  `json:"presetName"`
	Tempo       float32 `json:"tempo"`
	// Input1/Input2/InZ are the values the pedal is USING: the preset's when
	// InputsSetup is Preset (0), the pedal-wide ones when it is Global (1).
	Input1        uint32 `json:"input1"`
	Input2        uint32 `json:"input2"`
	InZ           uint32 `json:"inz"`
	InputsSetup   uint32 `json:"inputsSetup"`   // global: 1 = Global, 0 = Preset
	VariaxControl uint32 `json:"variaxControl"` // global: 1 = Global, 0 = Preset
	// TempoSync: global "Tempo Sync": 1 = Global (Tempo is the pedal-wide
	// tempo, not the preset's), 0 = Preset.
	TempoSync uint32 `json:"tempoSync"`
	// Stalled: the pedal stopped accepting data (USB writes time out, e.g.
	// after the editor was stopped mid-message): power-cycle the pedal. Cleared
	// by the next write that gets through.
	Stalled bool `json:"stalled,omitempty"`
	// DSPAlert counts the pedal's "DSP limit reached" messages (Cmd 0x33):
	// the UI shows the warning whenever it changes.
	DSPAlert int `json:"dspAlert,omitempty"`
	// MIDIChannel: global MIDI channel 1..16 (MIDI IN), 0 = not read yet.
	MIDIChannel uint32 `json:"midiChannel"`
	// Tuner: the pedal's tuner, on/off and its live reading (tuner.go).
	Tuner TunerState `json:"tuner"`
	// WaitingTuner: the first read found the tuner open, so preset, setlist
	// and tempo are not known yet; they are read when it closes.
	WaitingTuner bool `json:"waitingTuner,omitempty"`

	// Dirty: the edit buffer was changed after the preset was loaded (the
	// original editor shows it in amber italics with an asterisk).
	Dirty bool `json:"dirty"`
	// Loading: a setlist is being read from the pedal's memory.
	Loading *Progress `json:"loading,omitempty"`
	// LibraryRev changes whenever cached setlist content changes.
	LibraryRev int `json:"libraryRev"`
	// BufferRev changes whenever the copy of the edit buffer changes (flow.go).
	BufferRev int `json:"bufferRev"`

	presetIn [3]uint32 // from the edit buffer (setup 0x36, 0x37, 0x55)
	// presetTempo: the loaded preset's tempo; globalTempo: the pedal-wide
	// one as last seen (0 = unknown; see tempo_source.go).
	presetTempo, globalTempo float32
	globalIn                 [3]uint32 // pedal-wide (STATUS 0x0F, 0x10, 0x11)
}

// inputIdx maps the UI's (preset) setup ids to slots of presetIn/globalIn.
var inputIdx = map[uint32]int{
	podhd500.SetupInput1Source: 0,
	podhd500.SetupInput2Source: 1,
	podhd500.SetupGuitarInZ:    2,
}

var globalInputIDs = [3]uint32{podhd500.GlobalInput1Source, podhd500.GlobalInput2Source, podhd500.GlobalGuitarInZ}

// effective recomputes the visible input values from the active set.
func (st *State) effective() {
	in := st.presetIn
	if st.InputsSetup == 1 {
		in = st.globalIn
	}
	st.Input1, st.Input2, st.InZ = in[0], in[1], in[2]
}

// ErrOffline is returned by operations that need the pedal.
var ErrOffline = errors.New("POD HD500 not connected")

type waiter struct {
	cmd uint8
	ch  chan *usb.SPSIMessage
}

// Session is safe for concurrent use.
type Session struct {
	clientp atomic.Pointer[clientHolder] // nil when offline; Attach sets it later
	onLost  atomic.Pointer[func()]       // called after the pedal is lost

	reqMu sync.Mutex // one request/response exchange at a time

	// tunerBlocked: a read found the tuner open; read again when it closes.
	tunerBlocked atomic.Bool
	// leavingTuner: the tuner was just closed and afterTuner hasn't re-read
	// the pedal yet; the setup events the pedal sends meanwhile are ignored.
	leavingTuner atomic.Bool
	missed       atomic.Int32 // requests in a row without a reply (noteReply)

	waitMu  sync.Mutex
	waiters []*waiter

	stMu sync.RWMutex
	st   State

	subMu sync.Mutex
	subs  map[chan State]struct{}

	lib library

	opMu sync.Mutex // one SEND / RECEIVE at a time

	// Copy of the pedal's edit buffer for the Signal Flow (flow.go).
	bufMu       sync.Mutex
	buf         []byte
	reloadTimer *time.Timer

	// selectedAt: UnixNano of the last preset/setlist selection (event or
	// our own STATUS_SET): a PRESET_LOAD right after it is the stored version.
	selectedAt atomic.Int64

	// Snapshots (snapshots.go): snapDir overrides the folder of the snapshot
	// files; snapPlayer (the live player, cmd/hd500-snap) plays them without
	// the editor's on/off switch. Both are set once at start-up.
	snapDir    string
	snapPlayer bool
}

type clientHolder struct{ c usb.Client }

// cli returns the current client, or nil while offline.
func (s *Session) cli() usb.Client {
	if h := s.clientp.Load(); h != nil {
		return h.c
	}
	return nil
}

// Attach connects an offline session to a client that opened later (the pedal
// was powered on or plugged in after the editor started). The caller then
// runs Refresh, as at startup.
func (s *Session) Attach(c usb.Client) {
	s.clientp.Store(&clientHolder{c})
	s.update(func(st *State) {
		st.Connected = true
		st.Error = ""
	})
	go s.dispatch()
}

// NewSession wraps an open client (or nil for offline mode, with offlineErr
// explaining why).
func NewSession(c usb.Client, offlineErr error) *Session {
	s := &Session{subs: map[chan State]struct{}{}}
	if c != nil {
		s.clientp.Store(&clientHolder{c})
	}
	s.st.globalTempo = loadGlobalTempo()
	if c == nil {
		s.st.Error = fmt.Sprint(offlineErr)
		return s
	}
	s.st.Connected = true
	go s.dispatch()
	return s
}

// State returns a snapshot.
func (s *Session) State() State {
	s.stMu.RLock()
	defer s.stMu.RUnlock()
	return s.st
}

// Subscribe returns a channel receiving every new state (latest wins if the
// reader is slow) and a function to unsubscribe.
func (s *Session) Subscribe() (<-chan State, func()) {
	ch := make(chan State, 1)
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()
	return ch, func() {
		s.subMu.Lock()
		delete(s.subs, ch)
		s.subMu.Unlock()
	}
}

func (s *Session) update(f func(*State)) {
	s.stMu.Lock()
	f(&s.st)
	st := s.st
	s.stMu.Unlock()
	s.subMu.Lock()
	for ch := range s.subs {
		select { // keep only the newest state for slow subscribers
		case <-ch:
		default:
		}
		ch <- st
	}
	s.subMu.Unlock()
}

// dispatch is the only reader of the client's channel. It must never block on
// a request (replies arrive through it), so event follow-ups run in goroutines.
func (s *Session) dispatch() {
	c := s.cli()
	for m := range c.RecvChan() {
		if !s.deliver(m) {
			s.onEvent(m)
		}
	}
	// The client closed its channel: the pedal went away (power-off, cable, USB reset).
	s.lost(c)
}

// SetOnLost registers what to do after the pedal is lost (main starts the reconnect loop).
func (s *Session) SetOnLost(f func()) { s.onLost.Store(&f) }

// lost marks the session offline, releases the dead client and calls OnLost.
func (s *Session) lost(c usb.Client) {
	if s.cli() != c {
		return
	}
	s.clientp.Store(nil)
	s.update(func(st *State) {
		st.Connected = false
		st.Error = "POD HD500 disconnected"
	})
	go c.Close()
	if f := s.onLost.Load(); f != nil && *f != nil {
		go (*f)()
	}
}

// deliver hands m to the request waiting for it. Replies to our queries are
// addressed to our source address (0x08); messages the pedal pushes on its
// own (e.g. the PRESET_LOAD after a front-panel preset change) go to 0x05,
// so they are never mistaken for a reply.
func (s *Session) deliver(m *usb.SPSIMessage) bool {
	if m.DestAddr != usb.DefaultSourceAddr {
		return false
	}
	s.waitMu.Lock()
	defer s.waitMu.Unlock()
	for i, w := range s.waiters {
		if w.cmd == m.Cmd {
			s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
			w.ch <- m
			return true
		}
	}
	return false
}

// request sends spsi and waits for the first message with command want.
func (s *Session) request(spsi []byte, want uint8, timeout time.Duration) (*usb.SPSIMessage, error) {
	if s.cli() == nil {
		return nil, ErrOffline
	}
	s.reqMu.Lock()
	defer s.reqMu.Unlock()
	w := &waiter{cmd: want, ch: make(chan *usb.SPSIMessage, 1)}
	s.waitMu.Lock()
	s.waiters = append(s.waiters, w)
	s.waitMu.Unlock()
	remove := func() {
		s.waitMu.Lock()
		for i, x := range s.waiters {
			if x == w {
				s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
				break
			}
		}
		s.waitMu.Unlock()
	}
	if err := s.cli().SendSPSI(spsi); err != nil {
		remove()
		s.noteWrite(err)
		return nil, err
	}
	s.noteWrite(nil)
	select {
	case m := <-w.ch:
		s.noteReply(true)
		return m, nil
	case <-time.After(timeout):
		remove()
		s.noteReply(false)
		return nil, fmt.Errorf("no response %s within %v", usb.FormatCmd(want), timeout)
	}
}

func (s *Session) send(spsi []byte) error {
	if s.cli() == nil {
		return ErrOffline
	}
	s.reqMu.Lock()
	defer s.reqMu.Unlock()
	err := s.cli().SendSPSI(spsi)
	s.noteWrite(err)
	return err
}

// noteReply: the pedal can also hang while still taking data (writes go
// through, nothing answers — seen after Smart Harmony edits, 2026-11-08):
// two requests in a row without a reply mark the link Stalled too.
func (s *Session) noteReply(ok bool) {
	if ok {
		s.missed.Store(0)
		if s.State().Stalled {
			s.update(func(st *State) { st.Stalled = false })
		}
		return
	}
	if s.missed.Add(1) >= 2 && !s.State().Stalled {
		s.update(func(st *State) { st.Stalled = true })
	}
}

// noteWrite tracks whether the pedal still takes data (State.Stalled).
func (s *Session) noteWrite(err error) {
	// only marks it; a reply clears it (noteReply): a write that gets
	// through doesn't prove the pedal is alive
	if err != nil && strings.Contains(err.Error(), "timeout") && !s.State().Stalled {
		s.update(func(st *State) { st.Stalled = true })
	}
}

// Refresh reads position, setlist name and the edit buffer from the pedal.
func (s *Session) Refresh() error {
	// With the tuner open the pedal doesn't answer preset reads: show the
	// tuner and read everything when it is closed (tuner.go).
	s.readTunerSettings()
	if on, ok := s.readGlobal(GlobalTuner); ok && on != 0 {
		s.tunerBlocked.Store(true)
		s.update(func(st *State) { st.Tuner.setOn(true); st.WaitingTuner = true })
		return ErrTunerOn
	}
	sm, err := s.request(usb.BuildStatusQuery(usb.StatusIDCurrentSet), usb.CmdStatusResp, 2*time.Second)
	if err != nil {
		return err
	}
	pm, err := s.request(usb.BuildStatusQuery(usb.StatusIDCurrentPreset), usb.CmdStatusResp, 2*time.Second)
	if err != nil {
		return err
	}
	set, slot := int(sm.Word(2)), int(pm.Word(2))
	name := ""
	if set >= 0 && set < 8 {
		if nm, err := s.request(usb.BuildSetQuery(uint32(set)), usb.CmdSetLoad, 2*time.Second); err == nil {
			name = usb.ParseSetName(nm)
		}
	}
	lm, err := s.request(usb.BuildPresetQuery(usb.CurrentPresetAll, usb.CurrentSetAll), usb.CmdPresetLoad, 3*time.Second)
	if err != nil {
		return err
	}
	p, err := podhd500.ParsePreset(lm.Payload)
	if err != nil {
		return err
	}
	inputsSetup, haveIS := s.readGlobal(podhd500.GlobalInputsSetup)
	variaxCtl, haveVC := s.readGlobal(GlobalVariaxControl)
	tempoSync, haveTS := s.readGlobal(GlobalTempoSync)
	midiCh, haveMC := s.readGlobal(GlobalMIDIChannel)
	var gin [3]uint32
	haveG := [3]bool{}
	for i, id := range globalInputIDs {
		gin[i], haveG[i] = s.readGlobal(id)
	}
	// Edited? Compare the edit buffer with the stored version of the slot,
	// ignoring EXP-driven values: the pedal writes the expression pedal's
	// position into them (e.g. when the tuner closes), which isn't an edit.
	dirty, haveDirty := false, false
	if set >= 0 && set < NumSetlists && slot >= 0 && slot < SlotsPerSetlst {
		if sm, err := s.requestRetry(usb.BuildPresetQuery(uint16(slot), uint16(set)), usb.CmdPresetLoad, 3*time.Second); err == nil {
			s.cacheSlot(set, slot, sm.Payload)
			dirty, haveDirty = !podhd500.SameIgnoringExp(sm.Payload, lm.Payload), true
		}
	}
	s.update(func(st *State) {
		if haveTS {
			st.TempoSync = tempoSync // before applyPreset: it picks the tempo in use
		}
		if haveMC {
			st.MIDIChannel = midiCh
		}
		if haveDirty {
			st.Dirty = dirty
		}
		st.LibraryRev++
		if haveIS {
			st.InputsSetup = inputsSetup
		}
		if haveVC {
			st.VariaxControl = variaxCtl
		}
		for i := range gin {
			if haveG[i] {
				st.globalIn[i] = gin[i]
			}
		}
		st.Setlist, st.SetlistName = set, name
		st.Slot, st.SlotLabel = slot, podhd500.SlotLabel(slot)
		applyPreset(st, p)
	})
	s.setBuffer(lm.Payload)
	return nil
}

// readGlobal reads one pedal-wide STATUS id.
func (s *Session) readGlobal(id uint32) (uint32, bool) {
	g, err := s.request(usb.BuildStatusQuery(id), usb.CmdStatusResp, time.Second)
	if err != nil || g.Word(1) != id {
		return 0, false
	}
	return g.Word(2), true
}

func applyPreset(st *State, p *podhd500.Preset) {
	st.PresetName = p.Name()
	st.presetTempo = p.Tempo()
	st.Tempo = st.tempoInUse(st.presetTempo)
	st.presetIn[0], _ = p.SetupValue(podhd500.SetupInput1Source)
	st.presetIn[1], _ = p.SetupValue(podhd500.SetupInput2Source)
	st.presetIn[2], _ = p.SetupValue(podhd500.SetupGuitarInZ)
	st.effective()
}

// onEvent handles messages the pedal pushes on its own (front-panel changes).
// debugEvents logs every message the pedal pushes (HD500_DEBUG=1), to learn
// what the front panel does.
var debugEvents = os.Getenv("HD500_DEBUG") == "1"

func (s *Session) onEvent(m *usb.SPSIMessage) {
	if debugEvents && m.Cmd != usb.CmdPresetLoad {
		ws := make([]string, 0, len(m.Payload)/4)
		for i := 0; i < len(m.Payload)/4 && i < 8; i++ {
			ws = append(ws, fmt.Sprintf("%08x", m.Word(i)))
		}
		log.Printf("evento %s dest=%02x %v", usb.FormatCmd(m.Cmd), m.DestAddr, ws)
	} else if debugEvents {
		log.Printf("evento PRESET_LOAD (%d bytes)", len(m.Payload))
	}
	switch m.Cmd {
	case usb.CmdPresetLoad:
		// The pedal pushes the whole preset (a) right after a preset change
		// (PRESET_SELECTED comes first): the stored version, clean; and (b)
		// after a front-panel edit that rewrites the preset, e.g. MOVE of a
		// block: the edited buffer, dirty.
		if p, err := podhd500.ParsePreset(m.Payload); err == nil {
			if s.justSelected() {
				cur := s.State()
				s.cacheSlot(cur.Setlist, cur.Slot, m.Payload)
				s.update(func(st *State) { applyPreset(st, p); st.Dirty = false; st.LibraryRev++ })
			} else {
				s.update(func(st *State) { applyPreset(st, p); st.Dirty = true })
			}
			s.setBuffer(m.Payload)
		}
	case usb.CmdActiveChange, usb.CmdTypeChange, usb.CmdParamChange, usb.CmdParamMin, usb.CmdParamMax:
		s.update(func(st *State) { st.Dirty = true }) // edited on the pedal's front panel
		if m.Cmd == usb.CmdActiveChange {             // footswitch: [0, block, on] — show it at once
			id, on := int(m.Word(1)), m.Word(2) == 1
			if id >= 0 && id < 12 {
				s.editBuffer(func(p *podhd500.Preset) bool {
					if p.Item(id).Active == on {
						return false
					}
					p.SetItemActive(id, on)
					return true
				})
			}
		}
		if m.Cmd == usb.CmdParamChange && s.applyParamEvent(m) {
			break
		}
		if m.Cmd == usb.CmdTypeChange || m.Cmd == usb.CmdParamMin || m.Cmd == usb.CmdParamMax {
			// A new model (defaults for every parameter) or a heel/toe change:
			// re-read. Rare; each read shows "SYNC IN PROGRESS" on the LCD.
			s.reloadBufferSoon()
		}
	case CmdDSPOver:
		// DSP limit reached: the pedal keeps the new model but doesn't run it
		// (block not in use; LCD "DSP OVER"). Re-read to show which block.
		s.update(func(st *State) { st.DSPAlert++ })
		s.reloadBufferSoon()
	case CmdMIDIAssign:
		// MIDI Assign edited on the pedal: [0, control, six bytes]
		if c := int(m.Word(1)); c >= 0 && c < podhd500.MIDIAssignCount && len(m.Payload) >= 32 {
			var b [podhd500.MIDIAssignSize]byte
			for i := range b {
				b[i] = byte(m.Word(2 + i))
			}
			s.editBuffer(func(p *podhd500.Preset) bool { return p.SetMIDIAssign(c, b) })
			s.update(func(st *State) { st.Dirty = true })
		}
	case usb.CmdTempoParam1, usb.CmdTempoParam2:
		// A tempo parameter turned past its maximum into the note list (or
		// back): [0, block, note]; 0x14 = the block's 1st tempo slot, 0x31 = 2nd.
		id, note := int(m.Word(1)), int(m.Word(2))
		slot := 0
		if m.Cmd == usb.CmdTempoParam2 {
			slot = 1
		}
		if id >= 0 && id < 12 {
			s.editBuffer(func(p *podhd500.Preset) bool { return p.SetTempoSync(id, slot, note) })
			s.update(func(st *State) { st.Dirty = true }) // edited on the pedal's front panel
		}
	case usb.CmdPresetSelected:
		s.markSelected()
		// The full preset (PRESET_LOAD) only arrives ~50 ms later: show the
		// slot's stored name right away instead of the previous preset's
		// (was visible as a flash of the old name on the new row).
		slot := int(m.Word(0))
		name, _ := s.cachedName(s.State().Setlist, slot) // "" (shown as …) if never read
		s.update(func(st *State) {
			st.Slot, st.SlotLabel = slot, podhd500.SlotLabel(slot)
			st.PresetName = name
			st.Dirty = false // a freshly selected preset has no edits
		})
	case usb.CmdSetSelected:
		s.markSelected()
		set := int(m.Word(0))
		s.update(func(st *State) { st.Setlist = set })
		go func() { // setlist name needs a request: never from dispatch itself
			if nm, err := s.request(usb.BuildSetQuery(uint32(set)), usb.CmdSetLoad, 2*time.Second); err == nil {
				name := usb.ParseSetName(nm)
				s.update(func(st *State) { st.SetlistName = name })
			}
		}()
	case usb.CmdStatusSet:
		// Pedal-wide settings changed on the front panel: [0, id, value].
		id, v := m.Word(1), m.Word(2)
		switch id {
		case podhd500.GlobalInputsSetup:
			s.update(func(st *State) { st.InputsSetup = v; st.effective() })
		case GlobalVariaxControl:
			s.update(func(st *State) { st.VariaxControl = v })
		case GlobalMIDIChannel:
			s.update(func(st *State) { st.MIDIChannel = v })
		case GlobalTempoSync:
			s.update(func(st *State) { st.TempoSync = v; st.Tempo = st.tempoInUse(st.presetTempo) })
		case GlobalTuner, GlobalTunerMute, GlobalTunerRef:
			s.update(func(st *State) { st.applyTunerGlobal(id, v) })
			if id == GlobalTuner && v == 0 {
				s.leavingTuner.Store(true) // before the setup 0x68 that follows
				go s.afterTuner()
			}
		}
	case cmdTunerReading:
		r := tunerReading(int32(m.Word(0)), m.FloatWord(1))
		s.update(func(st *State) {
			if st.Tuner.On { // late readings after the tuner was turned off are dropped
				st.Tuner.setReading(r)
			}
		})
	case usb.CmdSetupChange:
		if s.leavingTuner.Load() {
			// Leaving the tuner the pedal sends setup 0x68 = 0x7f without
			// changing its buffer: applying it would flash "*" until
			// afterTuner's re-read, which brings the real values anyway.
			break
		}
		vtype, id := m.Word(1), m.Word(2)
		// Mixer, cab, mic… turned on the pedal: [0, vtype, id, value] carries
		// the new raw value, applied to the copy (no re-read: see flow.go).
		// Leaving the tuner re-sends setup 0x68 unchanged: only a real change
		// makes the preset dirty.
		// Without a loaded copy (e.g. the tuner was open at startup) there is
		// nothing to compare with: not dirty, the read after the tuner brings
		// the pedal's current values anyway.
		raw := m.Word(3)
		if vtype == 1 && id == usb.SetupIDTempo && s.State().TempoSync == 1 {
			// Tempo Sync = Global: this is the pedal-wide tempo (also sent when
			// switching to Global); the preset's own tempo is untouched.
			s.learnGlobalTempo(m.FloatWord(3))
			break
		}
		changed := false
		s.editBuffer(func(p *podhd500.Preset) bool {
			old, ok := p.SetupValue(id)
			if ok && old == raw {
				return false
			}
			changed = true
			return p.SetSetupValue(id, raw)
		})
		s.update(func(st *State) {
			if changed {
				st.Dirty = true // preset settings (tempo, inputs, mixer…) live in the preset
			}
			switch {
			case vtype == 1 && id == usb.SetupIDTempo:
				st.presetTempo = m.FloatWord(3)
				st.Tempo = st.presetTempo
			default:
				if i, ok := inputIdx[id]; ok {
					st.presetIn[i] = m.Word(3)
					st.effective()
				}
			}
		})
	}
}

// SetInputsSetup writes the pedal-wide "Inputs Setup" (1 Global, 0 Preset)
// and reads it back to confirm.
func (s *Session) SetInputsSetup(value uint32) error {
	if !podhd500.ValidOption(podhd500.InputsSetupOptions, value) {
		return fmt.Errorf("invalid value %d for Inputs Setup", value)
	}
	if err := s.send(usb.BuildStatusSet(podhd500.GlobalInputsSetup, value)); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	g, err := s.request(usb.BuildStatusQuery(podhd500.GlobalInputsSetup), usb.CmdStatusResp, time.Second)
	if err != nil {
		return fmt.Errorf("verifying Inputs Setup: %w", err)
	}
	got := g.Word(2)
	s.update(func(st *State) { st.InputsSetup = got; st.effective() })
	if got != value {
		return fmt.Errorf("the pedal kept Inputs Setup = %d (requested %d)", got, value)
	}
	return nil
}

// Tempo range accepted by the editor. 240 is the maximum the pedal reported
// while tapping on its TAP switch (2026-11-03).
const (
	MinTempo float32 = 30
	MaxTempo float32 = 240
)

// SetTempo sets the preset tempo (setup 0x17, float BPM) in the edit buffer.
func (s *Session) SetTempo(bpm float32) error {
	if bpm != bpm || bpm < MinTempo || bpm > MaxTempo { // bpm != bpm: NaN
		return fmt.Errorf("tempo %.1f outside the range %.0f–%.0f BPM", bpm, MinTempo, MaxTempo)
	}
	if err := s.send(usb.BuildTempoChange(bpm)); err != nil {
		return err
	}
	// With Tempo Sync = Global it is the pedal-wide tempo: not a preset edit.
	if s.State().TempoSync == 1 {
		s.learnGlobalTempo(bpm)
		return nil
	}
	s.update(func(st *State) { st.presetTempo, st.Tempo = bpm, bpm; st.Dirty = true })
	return nil
}

// Global is one pedal-wide setting read with STATUS_QUERY (0x21).
type Global struct {
	ID    uint32 `json:"id"`
	Value uint32 `json:"value"`
}

// Globals reads the given STATUS ids; ids the pedal doesn't answer are
// skipped. refs/hd500-editor reports that ids 0..0x25 respond.
func (s *Session) Globals(ids []uint32) ([]Global, error) {
	if s.cli() == nil {
		return nil, ErrOffline
	}
	var out []Global
	for _, id := range ids {
		m, err := s.request(usb.BuildStatusQuery(id), usb.CmdStatusResp, 400*time.Millisecond)
		if err != nil {
			continue
		}
		if m.Word(1) != id {
			continue // late reply to an earlier query
		}
		out = append(out, Global{ID: id, Value: m.Word(2)})
	}
	return out, nil
}

// SetInput changes one of the input settings of the current preset (edit
// buffer only, like turning the pedal's own controls).
func (s *Session) SetInput(id, value uint32) error {
	var opts []podhd500.Option
	switch id {
	case podhd500.SetupInput1Source:
		opts = podhd500.Input1Options
	case podhd500.SetupInput2Source:
		opts = podhd500.Input2Options
	case podhd500.SetupGuitarInZ:
		opts = podhd500.InZOptions
	default:
		return fmt.Errorf("setting 0x%X is not an input", id)
	}
	if !podhd500.ValidOption(opts, value) {
		return fmt.Errorf("invalid value %d for 0x%X", value, id)
	}
	i := inputIdx[id]
	if s.State().InputsSetup == 1 {
		// Global mode: the pedal uses its pedal-wide inputs, not the preset's.
		gid := globalInputIDs[i]
		if err := s.send(usb.BuildStatusSet(gid, value)); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
		got, ok := s.readGlobal(gid)
		if !ok {
			return fmt.Errorf("verifying global 0x%X: no response", gid)
		}
		s.update(func(st *State) { st.globalIn[i] = got; st.effective() })
		if got != value {
			return fmt.Errorf("the pedal kept global 0x%X = %d (requested %d)", gid, got, value)
		}
		return nil
	}
	if err := s.send(usb.BuildSetupChangeInt(id, value)); err != nil {
		return err
	}
	s.update(func(st *State) { st.presetIn[i] = value; st.effective(); st.Dirty = true })
	return nil
}

// RefreshGlobals re-reads only the pedal-wide settings (Inputs Setup and the
// global inputs), which the pedal doesn't announce when changed on its front
// panel. Unlike Refresh it doesn't read the edit buffer, which would make
// the LCD show "SYNC IN PROGRESS".
func (s *Session) RefreshGlobals() error {
	if s.cli() == nil {
		return ErrOffline
	}
	is, haveIS := s.readGlobal(podhd500.GlobalInputsSetup)
	vc, haveVC := s.readGlobal(GlobalVariaxControl)
	s.readTunerSettings()
	var gin [3]uint32
	var have [3]bool
	for i, id := range globalInputIDs {
		gin[i], have[i] = s.readGlobal(id)
	}
	s.update(func(st *State) {
		if haveIS {
			st.InputsSetup = is
		}
		if haveVC {
			st.VariaxControl = vc
		}
		for i := range gin {
			if have[i] {
				st.globalIn[i] = gin[i]
			}
		}
		st.effective()
	})
	return nil
}

func (s *Session) markSelected() { s.selectedAt.Store(time.Now().UnixNano()) }

func (s *Session) justSelected() bool {
	return time.Since(time.Unix(0, s.selectedAt.Load())) < 2*time.Second
}

// ReadStatus reads one pedal-wide STATUS id (debugging: GET /api/debug/status).
func (s *Session) ReadStatus(id uint32) (uint32, bool) {
	if s.cli() == nil {
		return 0, false
	}
	return s.readGlobal(id)
}
