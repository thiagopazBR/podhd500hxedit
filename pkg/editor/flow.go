package editor

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// Signal Flow: the blocks of the preset loaded on the pedal (its edit
// buffer). The session keeps a copy of the buffer. Reading the buffer from
// the pedal makes its LCD show "SYNC IN PROGRESS", so it is read only when
// it must be (connect, new model, heel/toe change); otherwise the copy is
// updated from:
//   - PRESET_LOAD pushed by the pedal after a preset change (the whole body);
//   - ACTIVE / PARAM / SETUP events from the front panel, applied in place
//     (they carry the new value);
//   - changes made from the editor, applied to the copy as they are sent.
// State.BufferRev changes whenever the copy changes; the UI then re-reads
// /api/preset.

// Mixer setup ids (floats; level in dB, pan -1..+1).
const (
	setupMixALevel = 0x2C
	setupMixBLevel = 0x2D
	setupMixAPan   = 0x2E
	setupMixBPan   = 0x2F
)

// Mixer is the A/B mixer between the two paths.
type Mixer struct {
	ALevel float32 `json:"aLevel"`
	BLevel float32 `json:"bLevel"`
	APan   float32 `json:"aPan"`
	BPan   float32 `json:"bPan"`
}

// PresetView is the loaded preset as the Signal Flow shows it.
type PresetView struct {
	Rev    int                   `json:"rev"`
	Name   string                `json:"name"`
	Items  []podhd500.PresetItem `json:"items"` // by block id: 0 Amp A, 1 Cab A, 2 Amp B, 3 Cab B, 4..11 FX 1..8
	Mixer  Mixer                 `json:"mixer"`
	Cabs   [2]CabView            `json:"cabs"`  // E.R. and mic of cab A, B
	Chain  Chain                 `json:"chain"` // FX blocks by lane, in signal order
	Variax VariaxView            `json:"variax"`
	MIDI   []podhd500.MIDIAssign `json:"midi"` // MIDI Assign of the 13 controls (MIDI OUT)
}

// setBuffer replaces the copy of the edit buffer.
func (s *Session) setBuffer(body []byte) {
	if len(body) != podhd500.PresetBodySize {
		return
	}
	s.bufMu.Lock()
	s.buf = append([]byte(nil), body...)
	s.bufMu.Unlock()
	s.update(func(st *State) { st.BufferRev++ })
}

// editBuffer changes the copy in place (f returns false for no change).
func (s *Session) editBuffer(f func(p *podhd500.Preset) bool) {
	s.bufMu.Lock()
	changed := false
	if s.buf != nil {
		p, err := podhd500.ParsePreset(s.buf)
		if err == nil && f(p) {
			copy(s.buf, p.Body[:])
			changed = true
		}
	}
	s.bufMu.Unlock()
	if changed {
		s.update(func(st *State) { st.BufferRev++ })
	}
}

// reloadBufferSoon re-reads the edit buffer after a burst of front-panel
// edits settles (turning a knob sends many events). Never blocks: called
// from the dispatcher, which must not make requests itself.
func (s *Session) reloadBufferSoon() {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	if s.reloadTimer != nil {
		s.reloadTimer.Reset(250 * time.Millisecond)
		return
	}
	s.reloadTimer = time.AfterFunc(250*time.Millisecond, func() {
		s.bufMu.Lock()
		s.reloadTimer = nil
		s.bufMu.Unlock()
		_ = s.loadBuffer()
	})
}

func (s *Session) loadBuffer() error {
	m, err := s.requestRetry(usb.BuildPresetQuery(usb.CurrentPresetAll, usb.CurrentSetAll), usb.CmdPresetLoad, 3*time.Second)
	if err != nil {
		return err
	}
	s.setBuffer(m.Payload)
	return nil
}

// Preset returns the loaded preset's blocks.
func (s *Session) Preset() (PresetView, error) {
	if s.cli() == nil {
		return PresetView{}, ErrOffline
	}
	s.bufMu.Lock()
	have := s.buf != nil
	s.bufMu.Unlock()
	if !have {
		if s.tunerOpen() {
			s.tunerBlocked.Store(true) // read it when the tuner closes
			return PresetView{}, ErrTunerOn
		}
		if err := s.loadBuffer(); err != nil {
			return PresetView{}, err
		}
	}
	s.bufMu.Lock()
	p, err := podhd500.ParsePreset(s.buf)
	s.bufMu.Unlock()
	if err != nil {
		return PresetView{}, err
	}
	f := func(id uint32) float32 {
		v, _ := p.SetupValue(id)
		x := math.Float32frombits(v)
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return 0
		}
		return x
	}
	return PresetView{
		Rev:    s.State().BufferRev,
		Name:   p.Name(),
		Items:  p.Items(),
		Mixer:  Mixer{ALevel: f(setupMixALevel), BLevel: f(setupMixBLevel), APan: f(setupMixAPan), BPan: f(setupMixBPan)},
		Cabs:   cabViews(p),
		Chain:  chainOf(p),
		MIDI:   p.MIDIAssigns(),
		Variax: variaxView(p),
	}, nil
}

// SetBlockActive turns a block on or off in the edit buffer (like its
// footswitch). The preset becomes dirty; SEND saves it.
func (s *Session) SetBlockActive(id int, on bool) error {
	if id < 0 || id > 11 {
		return fmt.Errorf("invalid block %d", id)
	}
	if s.cli() == nil {
		return ErrOffline
	}
	s.bufMu.Lock()
	var empty bool
	if s.buf != nil {
		p, _ := podhd500.ParsePreset(s.buf)
		empty = p.Item(id).Empty
	}
	s.bufMu.Unlock()
	if empty {
		return errors.New("empty block")
	}
	if err := s.send(usb.BuildActiveChange(uint32(id), on)); err != nil {
		return err
	}
	s.editBuffer(func(p *podhd500.Preset) bool {
		if p.Item(id).Active == on {
			return false
		}
		p.SetItemActive(id, on)
		return true
	})
	s.update(func(st *State) { st.Dirty = true })
	return nil
}

// ModelInfo is one entry of the model browser.
type ModelInfo struct {
	Type    uint32 `json:"type"`
	Name    string `json:"name"`
	BasedOn string `json:"basedOn,omitempty"`
	None    bool   `json:"none,omitempty"` // "No Cab": selected when the block is empty
	// Bass: a bass amp / preamp / cab (the UI's "Bass Amp" / "Bass Cab" lists).
	Bass bool `json:"bass,omitempty"`
	// Style: how the model is drawn (amps and preamps: tolex, grille, panel…).
	Style *podhd500.GearStyle `json:"style,omitempty"`
}

// The HD500's bass models (based on Ampeg, Gallien-Krueger and Line 6 bass
// gear); every other amp and cab is a guitar model. Preamps are matched by
// the amp name without " Preamp".
var (
	bassAmps = map[string]bool{"Flip Top": true, "SVT Bright": true, "SVT Normal": true, "G Cougar 800": true}
	bassCabs = map[string]bool{"115 Flip Top": true, "810 SV Beast": true, "410 Rhino": true}
)

// ModelGroup is one category of the model browser.
type ModelGroup struct {
	Name   string      `json:"name"`
	Kind   string      `json:"kind"` // fx | amp | cab
	Models []ModelInfo `json:"models"`
}

// NoCabType is the cab type of an amp without cabinet.
const NoCabType uint32 = 0x0107FFFF

// NoAmpType is the amp type of a path without amp.
const NoAmpType uint32 = 0x0007FFFF

// fxGroupOrder is the browser order of FX categories (catalog group names).
var fxGroupOrder = []string{"Distortion", "Dynamics", "Preamp+EQ", "Modulation", "Delay", "Reverb", "Pitch", "Filter", "Wah", "Volume/Pan", "FX Loop"}

// Models lists the HD500's models by category: FX groups, then amps,
// preamps and cabs. "None" (empty FX slot) and "Amp Disabled" are left out
// of the lists; the UI offers them as "Nenhum".
func Models() []ModelGroup {
	cat := podhd500.GetCatalog()
	byGroup := map[string][]ModelInfo{}
	for _, m := range cat.Pedals {
		if m.Type&0xFFFF == 0xFFFF {
			continue
		}
		byGroup[m.Group] = append(byGroup[m.Group], ModelInfo{Type: m.Type, Name: m.Name, BasedOn: m.BasedOn})
	}
	var out []ModelGroup
	for _, g := range fxGroupOrder {
		if len(byGroup[g]) > 0 {
			out = append(out, ModelGroup{Name: g, Kind: "fx", Models: byGroup[g]})
		}
	}
	// "No Cab" as factory presets store it (amp in use, cab 0x0107FFFF); the
	// catalog's 0x0007FFFF is the type of an unused amp/cab pair.
	var amps, pre, cabs []ModelInfo
	for _, m := range cat.Amps {
		if m.Type&0xFFFF == 0xFFFF {
			continue
		}
		if strings.HasSuffix(m.Name, " Preamp") {
			pre = append(pre, ModelInfo{Type: m.Type, Name: m.Name, BasedOn: m.BasedOn, Bass: bassAmps[strings.TrimSuffix(m.Name, " Preamp")], Style: podhd500.StyleOf("amp", m.Name)})
		} else {
			amps = append(amps, ModelInfo{Type: m.Type, Name: m.Name, BasedOn: m.BasedOn, Bass: bassAmps[m.Name], Style: podhd500.StyleOf("amp", m.Name)})
		}
	}
	for _, m := range cat.Cabs {
		if m.Type&0xFFFF != 0xFFFF {
			cabs = append(cabs, ModelInfo{Type: m.Type, Name: m.Name, BasedOn: m.BasedOn, Bass: bassCabs[m.Name], Style: podhd500.StyleOf("cab", m.Name)})
		}
	}
	cabs = append(cabs, ModelInfo{Type: NoCabType, Name: "No Cab", None: true}) // last, as the user asked
	// "Amp Disabled" (factory presets: 0x0007FFFF, e.g. "Stereo Dobro"), last too.
	amps = append(amps, ModelInfo{Type: NoAmpType, Name: "Amp Disabled", None: true})
	pre = append(pre, ModelInfo{Type: NoAmpType, Name: "Amp Disabled", None: true})
	return append(out,
		ModelGroup{Name: "Amps", Kind: "amp", Models: amps},
		ModelGroup{Name: "Cabs", Kind: "cab", Models: cabs},
		ModelGroup{Name: "Preamps", Kind: "amp", Models: pre})
}

// validModel tells whether t is a model of the block kind (empty included).
func validModel(kind string, t uint32) bool {
	if t&0xFFFF == 0xFFFF {
		return true // None / Amp Disabled / No Cab
	}
	cat := podhd500.GetCatalog()
	list := cat.Pedals
	switch kind {
	case "amp":
		list = cat.Amps
	case "cab":
		list = cat.Cabs
	}
	for _, m := range list {
		if m.Type == t {
			return true
		}
	}
	return false
}

// SetBlockModel changes the model of a block in the edit buffer (TYPE 0x11).
// The pedal loads the model's default parameters; the buffer is re-read.
func (s *Session) SetBlockModel(id int, modelType uint32) error {
	if id < 0 || id > 11 {
		return fmt.Errorf("invalid block %d", id)
	}
	if s.cli() == nil {
		return ErrOffline
	}
	kind := podhd500.ItemKind(id)
	if !validModel(kind, modelType) {
		return fmt.Errorf("model 0x%08x doesn't fit this block", modelType)
	}
	if kind == "fx" && modelType&0xFFFF == 0xFFFF {
		// None: a TYPE None from the host leaves the block as 0xFFFFFFFF
		// (LCD garbled, "AMP x"); written as the pedal stores an empty slot,
		// in a buffer rewrite (blocks the pedal couldn't run are cleared too)
		return s.rewriteBuffer("block removal", func(p *podhd500.Preset) (bool, error) {
			return p.SetFXNone(id), nil
		})
	}
	if p, err := podhd500.ParsePreset(s.BufferCopy()); err == nil {
		for i := 4; i <= 11; i++ {
			if stuckFX(p.Item(i)) {
				if err := s.rewriteBuffer("cleanup", func(*podhd500.Preset) (bool, error) { return false, nil }); err != nil {
					return err
				}
				break
			}
		}
	}
	if kind == "amp" && modelType == NoAmpType {
		return s.disableAmp(id)
	}
	if kind == "amp" {
		return s.setAmp(id, modelType)
	}
	// FX, as the front panel does it (debug log 2026-11-08): a model is
	// ACTIVE on + TYPE; None is TYPE None + ACTIVE off. A bare TYPE None on
	// a block the pedal couldn't run (DSP over) left it "on" with type
	// 0xFFFFFFFF and garbled the LCD.
	var frames [][]byte
	if modelType&0xFFFF == 0xFFFF {
		frames = [][]byte{usb.BuildTypeChange(uint32(id), modelType)} // the pedal turns it off itself
	} else {
		frames = [][]byte{usb.BuildActiveChange(uint32(id), true), usb.BuildTypeChange(uint32(id), modelType)}
	}
	for _, f := range frames {
		if err := s.send(f); err != nil {
			return err
		}
	}
	s.update(func(st *State) { st.Dirty = true })
	time.Sleep(300 * time.Millisecond) // let the pedal set up the model
	return s.loadBuffer()
}

// setAmp chooses an amp. It always starts enabled with its cab (unless "No
// Cab"), as the front panel does: ACTIVE on goes first (the pedal ignores a
// TYPE for an amp that is not in use), then TYPE (the pedal also picks the
// amp's matching cab), and the cab is turned on only after the pedal has
// settled: sent right away, the pedal turns it off again.
func (s *Session) setAmp(id int, modelType uint32) error {
	cab := id + 1
	for _, f := range [][]byte{usb.BuildActiveChange(uint32(id), true), usb.BuildTypeChange(uint32(id), modelType)} {
		if err := s.send(f); err != nil {
			return err
		}
	}
	s.update(func(st *State) { st.Dirty = true })
	time.Sleep(400 * time.Millisecond)
	if err := s.loadBuffer(); err != nil {
		return err
	}
	s.bufMu.Lock()
	p, _ := podhd500.ParsePreset(s.buf)
	s.bufMu.Unlock()
	if c := p.Item(cab); c.Type&0xFFFF != 0xFFFF && !c.Active {
		if err := s.send(usb.BuildActiveChange(uint32(cab), true)); err != nil {
			return err
		}
		s.editBuffer(func(p *podhd500.Preset) bool { p.SetItemActive(cab, true); return true })
	}
	return nil
}

// SetParam changes one parameter of a block in the edit buffer (PARAM 0x2D).
// Values are normalized 0..1, as stored in the preset (lists included).
func (s *Session) SetParam(id int, paramID uint32, value float32) error {
	if id < 0 || id > 11 {
		return fmt.Errorf("invalid block %d", id)
	}
	if s.cli() == nil {
		return ErrOffline
	}
	if math.IsNaN(float64(value)) {
		return errors.New("invalid value")
	}
	value = float32(math.Max(0, math.Min(1, float64(value))))
	// a list stored as index/ListDiv (Studio EQ freqs): the API's index/(n-1)
	// is sent as the pedal stores it
	if b := s.BufferCopy(); b != nil {
		if pr, err := podhd500.ParsePreset(b); err == nil {
			for _, q := range pr.Item(id).Params {
				if q.ID == paramID && q.ListDiv > 0 && len(q.Options) > 1 {
					idx := math.Round(float64(value) * float64(len(q.Options)-1))
					value = float32(idx / float64(q.ListDiv))
				}
			}
		}
	}
	frame := usb.BuildParamChangeFloat(uint32(id), paramID, value)
	intIdx, intRaw := -1, uint32(0)
	if podhd500.IsIntParam(paramID) {
		// Smart Harmony Key / Shift / Scale (the only 0x3F00xxxx ids) are
		// integer parameters: the pedal reports them as PARAM [0, block, 0
		// (int), id, option index] (front-panel log 2026-11-08) and a float
		// write hangs it. The preset keeps index/(n-1) as a float.
		n, base := 0, 0
		if b := s.BufferCopy(); b != nil {
			if pr, err := podhd500.ParsePreset(b); err == nil {
				for _, q := range pr.Item(id).Params {
					if q.ID == paramID {
						n, base = len(q.Options), q.IntMin
					}
				}
			}
		}
		if n < 2 {
			return errors.New("unknown option list for this parameter")
		}
		idx := int(math.Round(float64(value) * float64(n-1)))
		value = float32(idx) / float32(n-1)
		raw := uint32(int32(idx + base)) // Shift: -8..+8
		frame = usb.BuildParamChangeInt(uint32(id), paramID, raw)
		intRaw, intIdx = raw, idx
	}
	if err := s.send(frame); err != nil {
		return err
	}
	s.editBuffer(func(p *podhd500.Preset) bool {
		if intIdx >= 0 {
			return p.SetParamInt(id, paramID, intRaw)
		}
		return p.SetParamValue(id, paramID, value)
	})
	s.update(func(st *State) { st.Dirty = true })
	return nil
}

// SetTempoSync makes a block's tempo parameter (slot 1 or 2) follow a note
// value of the tap tempo, or free time/speed with note 0. Sent as the same
// Cmd 0x14 / 0x31 the pedal reports: [0, block, note].
func (s *Session) SetTempoSync(id, slot, note int) error {
	if id < 4 || id > 11 {
		return fmt.Errorf("invalid FX block %d", id)
	}
	if slot < 1 || slot > podhd500.MaxTempoSlots {
		return fmt.Errorf("invalid tempo slot %d", slot)
	}
	if !podhd500.ValidSyncNote(note) {
		return fmt.Errorf("invalid note %d", note)
	}
	if s.cli() == nil {
		return ErrOffline
	}
	cmd := usb.CmdTempoParam1
	if slot == 2 {
		cmd = usb.CmdTempoParam2
	}
	if err := s.send(usb.BuildFrame(cmd, 0, uint32(id), uint32(note))); err != nil {
		return err
	}
	s.editBuffer(func(p *podhd500.Preset) bool { return p.SetTempoSync(id, slot-1, note) })
	s.update(func(st *State) { st.Dirty = true })
	return nil
}

// Cab settings, A and B, as setup ids (SETUP 0x16). Low cut, res level,
// thump and decay point into the cab block's parameters 0..3; E.R. is a
// float 0..1 and the mic an index into podhd500.MicNames.
var cabSetup = map[string][2]uint32{
	"lowcut": {0x57, 0x58},
	"res":    {0x59, 0x5A},
	"thump":  {0x5B, 0x5C},
	"decay":  {0x5D, 0x5E},
	"er":     {0x32, 0x33},
	"mic":    {0x34, 0x35},
}

// CabView holds the cab settings that live outside the cab block.
type CabView struct {
	ER  float32 `json:"er"`
	Mic int     `json:"mic"`
}

func cabViews(p *podhd500.Preset) [2]CabView {
	var out [2]CabView
	for i := 0; i < 2; i++ {
		er, _ := p.SetupValue(cabSetup["er"][i])
		mic, _ := p.SetupValue(cabSetup["mic"][i])
		x := math.Float32frombits(er)
		if math.IsNaN(float64(x)) {
			x = 0
		}
		out[i] = CabView{ER: x, Mic: int(mic)}
	}
	return out
}

// SetCab changes a cab setting of path A (cab 0) or B (cab 1) in the edit
// buffer. value is normalized 0..1, except "mic" (0..7).
func (s *Session) SetCab(cab int, param string, value float32) error {
	ids, ok := cabSetup[param]
	if !ok || cab < 0 || cab > 1 {
		return fmt.Errorf("invalid cab parameter %q", param)
	}
	if s.cli() == nil {
		return ErrOffline
	}
	id := ids[cab]
	var frame []byte
	var raw uint32
	if param == "mic" {
		n := int(value)
		if n < 0 || n >= len(podhd500.MicNames) {
			return fmt.Errorf("invalid mic %d", n)
		}
		raw = uint32(n)
		frame = usb.BuildSetupChangeInt(id, raw)
	} else {
		if math.IsNaN(float64(value)) {
			return errors.New("invalid value")
		}
		value = float32(math.Max(0, math.Min(1, float64(value))))
		raw = math.Float32bits(value)
		frame = usb.BuildSetupChangeFloat(id, value)
	}
	if err := s.send(frame); err != nil {
		return err
	}
	s.editBuffer(func(p *podhd500.Preset) bool { return p.SetSetupValue(id, raw) })
	s.update(func(st *State) { st.Dirty = true })
	return nil
}

// applyParamEvent applies a PARAM event from the pedal ([0, block, vtype,
// param id, value]) to the copy of the edit buffer. Returns false when it
// can't (then the caller re-reads).
func (s *Session) applyParamEvent(m *usb.SPSIMessage) bool {
	id, vtype, pid := int(m.Word(1)), m.Word(2), m.Word(3)
	if id >= 0 && id <= 11 && vtype == 0 && podhd500.IsIntParam(pid) {
		// integer parameter turned on the pedal: [0, block, 0, id, index]
		idx := m.Word(4)
		s.editBuffer(func(p *podhd500.Preset) bool { return p.SetParamInt(id, pid, idx) })
		return true
	}
	if id < 0 || id > 11 || vtype != 1 {
		return false
	}
	v := m.FloatWord(4)
	ok := false
	s.editBuffer(func(p *podhd500.Preset) bool {
		for _, q := range p.Item(id).Params {
			if q.ID == pid {
				ok = true
				if q.Value == v {
					return false
				}
				return p.SetParamValue(id, pid, v)
			}
		}
		return false
	})
	return ok
}

// disableAmp removes the amp of a path ("Amp Disabled"). The pedal ignores a
// TYPE 0x0007FFFF sent over USB (only the panel can do it: it then reports
// TYPE amp 0x7FFFF + ACTIVE off for amp and cab), so the edit buffer is
// rewritten whole (PRESET_SET to the buffer, as a live rename), with the amp
// and its cab as the panel leaves them: amp type 0x7FFFF, both off and not
// in use (byte 11 = 0).
func (s *Session) disableAmp(id int) error {
	// First as the panel reports it, amp off first: if the pedal takes the
	// TYPE that way, the LCD cursor stays on the amp (a whole-buffer write
	// moves it).
	for _, f := range [][]byte{
		usb.BuildActiveChange(uint32(id), false),
		usb.BuildTypeChange(uint32(id), NoAmpType),
		usb.BuildActiveChange(uint32(id+1), false),
	} {
		if err := s.send(f); err != nil {
			return err
		}
	}
	s.update(func(st *State) { st.Dirty = true })
	time.Sleep(400 * time.Millisecond)
	if err := s.loadBuffer(); err != nil {
		return err
	}
	s.bufMu.Lock()
	cur, _ := podhd500.ParsePreset(s.buf)
	s.bufMu.Unlock()
	if cur.Item(id).Type == NoAmpType {
		if debugEvents {
			log.Printf("amp %d desabilitado por TYPE (sem reescrever o buffer)", id)
		}
		return nil
	}
	if debugEvents {
		log.Printf("amp %d: TYPE 0x7FFFF ignorado, reescrevendo o buffer", id)
	}
	s.bufMu.Lock()
	p, err := podhd500.ParsePreset(s.buf)
	s.bufMu.Unlock()
	if err != nil {
		return err
	}
	ao, co := podhd500.ItemOffset(id), podhd500.ItemOffset(id+1)
	binary.LittleEndian.PutUint32(p.Body[ao:], NoAmpType)
	p.Body[ao+8], p.Body[ao+11] = 0, 0 // amp off, not in use
	p.Body[co+8], p.Body[co+11] = 0, 0 // cab off, not in use
	p.ClearDSPOver()                   // never send back a block the pedal couldn't run
	frame, err := usb.BuildPresetSet(usb.CurrentPresetAll, usb.CurrentSetAll, p.Body[:])
	if err != nil {
		return err
	}
	ack, err := s.request(frame, usb.CmdPresetSetAck, 5*time.Second)
	if err != nil {
		return err
	}
	if ack.Word(0) != 0 {
		return fmt.Errorf("the pedal rejected it (status %d)", ack.Word(0))
	}
	s.update(func(st *State) { st.Dirty = true })
	time.Sleep(300 * time.Millisecond)
	// The write moves the LCD cursor to another block. Known ways to bring it
	// back are worse (user chose to stay on the chain view): re-sending the
	// cab's type opens the AMP:EDIT page; a TYPE to the amp stores 0xFFFFFFFF.
	return s.loadBuffer() // what the pedal made of it
}

// Mixer ranges (floats stored in dB and -1..+1, as in factory presets).
const (
	MixerLevelMin = -60
	MixerLevelMax = 12
)

var mixerSetup = map[string]uint32{
	"aLevel": setupMixALevel, "bLevel": setupMixBLevel,
	"aPan": setupMixAPan, "bPan": setupMixBPan,
}

// SetMixer changes a mixer setting of the edit buffer (SETUP 0x16, float):
// level of path A/B in dB, or pan of path A/B from -1 (left) to +1 (right).
func (s *Session) SetMixer(param string, value float32) error {
	id, ok := mixerSetup[param]
	if !ok {
		return fmt.Errorf("invalid mixer parameter %q", param)
	}
	if s.cli() == nil {
		return ErrOffline
	}
	if math.IsNaN(float64(value)) {
		return errors.New("invalid value")
	}
	lo, hi := float64(MixerLevelMin), float64(MixerLevelMax)
	if id == setupMixAPan || id == setupMixBPan {
		lo, hi = -1, 1
	}
	value = float32(math.Max(lo, math.Min(hi, float64(value))))
	if err := s.send(usb.BuildSetupChangeFloat(id, value)); err != nil {
		return err
	}
	s.editBuffer(func(p *podhd500.Preset) bool { return p.SetSetupValue(id, math.Float32bits(value)) })
	s.update(func(st *State) { st.Dirty = true })
	return nil
}

// BufferCopy returns a copy of the edit buffer as last read (nil if none).
// For debugging (GET /api/debug/buffer with HD500_DEBUG=1).
func (s *Session) BufferCopy() []byte {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	if s.buf == nil {
		return nil
	}
	return append([]byte(nil), s.buf...)
}
