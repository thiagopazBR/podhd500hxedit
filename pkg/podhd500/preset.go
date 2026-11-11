package podhd500

import (
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

//go:embed data/*
var EmbeddedData embed.FS

const (
	PresetBodySize = 4096
	ItemBaseOffset = 0x28
	ItemSize       = 256
	ParamBase      = 16
	ParamSize      = 20
	MaxParams      = 12

	NoAmpType uint32 = 0x0007ffff
	NoCabType uint32 = 0x0007ffff
	NoFXType  uint32 = 0x020dffff

	TempoOffset = 0x0d58
)

var (
	ItemOrder = []int{0, 2, 1, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	FSNames   = []string{"—", "FS1", "FS2", "FS3", "FS4", "FS5", "FS6", "FS7", "FS8", "EXP toe"}
	CtlNames  = []string{"—", "EXP 1", "EXP 2"}
	MicNames  = []string{"57 On XS", "57 Off XS", "409 Dyn", "421 Dyn", "4038 Rbn", "121 Rbn", "67 Cond", "87 Cond"}
	InZNames  = []string{"Auto", "22K", "32K", "70K", "90K", "136K", "230K", "1M", "3.5M"}
	LaneNames = []string{"Pre", "Path A Pre", "Path B Pre", "Path A Post", "Path B Post", "Post"}
)

type CatalogParam struct {
	ID      uint32   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Min     float32  `json:"min,omitempty"`
	Max     float32  `json:"max,omitempty"`
	Unit    string   `json:"unit,omitempty"`    // display unit of a range ("dB", "ms")
	Whole   bool     `json:"whole,omitempty"`   // shown without decimals (LCD "-12 dB", not "-12.0")
	IntMin  int      `json:"intMin,omitempty"`  // integer params: raw value of the first option (Smart Harmony Shift: -8)
	Step    float32  `json:"step,omitempty"`    // display resolution of a range (Hard Gate thresholds: 1 dB)
	ListDiv int      `json:"listDiv,omitempty"` // list stored as index/ListDiv (Studio EQ freqs: 8) instead of index/(n-1)
	Options []string `json:"options,omitempty"`
}

type CatalogModel struct {
	Type    uint32         `json:"type"`
	Name    string         `json:"name"`
	Group   string         `json:"group,omitempty"`
	BasedOn string         `json:"basedOn,omitempty"`
	Params  []CatalogParam `json:"params"`
}

type Catalog struct {
	Amps   []CatalogModel `json:"amps"`
	Cabs   []CatalogModel `json:"cabs"`
	Pedals []CatalogModel `json:"pedals"`
}

type SetupEntry struct {
	ID     uint32
	Offset int
	Size   int
	Name   string
}

var (
	catalogOnce sync.Once
	globalCat   Catalog
	modelsByID  = map[uint32]*CatalogModel{}
	ampsByID    = map[uint32]*CatalogModel{}
	cabsByID    = map[uint32]*CatalogModel{}
	fxByID      = map[uint32]*CatalogModel{}
	setupByID   = map[uint32]SetupEntry{}
)

func initCatalog() {
	catalogOnce.Do(func() {
		rawCat, err := EmbeddedData.ReadFile("data/catalog.json")
		if err == nil {
			_ = json.Unmarshal(rawCat, &globalCat)
		}
		var ampOrigins map[string]string
		if b, err := EmbeddedData.ReadFile("data/amp-origins.json"); err == nil {
			_ = json.Unmarshal(b, &ampOrigins)
		}
		var fxOrigins map[string]string
		if b, err := EmbeddedData.ReadFile("data/fx-origins.json"); err == nil {
			_ = json.Unmarshal(b, &fxOrigins)
		}

		for i := range globalCat.Amps {
			m := &globalCat.Amps[i]
			relabelAmpKnobs(m)
			if orig, ok := ampOrigins[m.Name]; ok {
				m.BasedOn = orig
			}
			modelsByID[m.Type] = m
			ampsByID[m.Type] = m
		}
		for i := range globalCat.Cabs {
			m := &globalCat.Cabs[i]
			modelsByID[m.Type] = m
			cabsByID[m.Type] = m
		}
		for i := range globalCat.Pedals {
			m := &globalCat.Pedals[i]
			if orig, ok := fxOrigins[m.Name]; ok {
				m.BasedOn = orig
			}
			modelsByID[m.Type] = m
			fxByID[m.Type] = m
		}

		var rawSetup [][3]int
		if b, err := EmbeddedData.ReadFile("data/setup_map.json"); err == nil {
			_ = json.Unmarshal(b, &rawSetup)
		}
		var setupNames map[string]string
		if b, err := EmbeddedData.ReadFile("data/setting_names.json"); err == nil {
			_ = json.Unmarshal(b, &setupNames)
		}
		for _, row := range rawSetup {
			id := uint32(row[0])
			name := setupNames[fmt.Sprintf("%d", id)]
			setupByID[id] = SetupEntry{
				ID:     id,
				Offset: row[1],
				Size:   row[2],
				Name:   name,
			}
		}
	})
}

func GetCatalog() *Catalog {
	initCatalog()
	return &globalCat
}

func LookupModel(modelType uint32) *CatalogModel {
	initCatalog()
	return modelsByID[modelType]
}

func LookupParamName(modelType, paramID uint32) string {
	initCatalog()
	if m, ok := modelsByID[modelType]; ok {
		for _, p := range m.Params {
			if p.ID == paramID {
				return p.Name
			}
		}
	}
	for _, m := range modelsByID {
		for _, p := range m.Params {
			if p.ID == paramID {
				return p.Name
			}
		}
	}
	return fmt.Sprintf("0x%08X", paramID)
}

func LookupSetup(setupID uint32) (SetupEntry, bool) {
	initCatalog()
	s, ok := setupByID[setupID]
	return s, ok
}

func ItemKind(id int) string {
	if id == 0 || id == 2 {
		return "amp"
	}
	if id == 1 || id == 3 {
		return "cab"
	}
	return "fx"
}

func ItemOffset(id int) int {
	for idx, v := range ItemOrder {
		if v == id {
			return ItemBaseOffset + idx*ItemSize
		}
	}
	return ItemBaseOffset
}

type ItemParam struct {
	Index   int      `json:"index"`
	ID      uint32   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Value   float32  `json:"value"`
	Lo      float32  `json:"lo"`
	Hi      float32  `json:"hi"`
	Ctl     uint8    `json:"ctl"`
	Ext     uint32   `json:"ext"`           // raw last 4 bytes of the entry (byte 0 = controller)
	Min     float32  `json:"min,omitempty"` // display range of the normalized value (catalog)
	Max     float32  `json:"max,omitempty"`
	Unit    string   `json:"unit,omitempty"`    // display unit of a range ("dB", "ms")
	Whole   bool     `json:"whole,omitempty"`   // shown without decimals (LCD "-12 dB", not "-12.0")
	IntMin  int      `json:"intMin,omitempty"`  // integer params: raw value of the first option (Smart Harmony Shift: -8)
	Step    float32  `json:"step,omitempty"`    // display resolution of a range (Hard Gate thresholds: 1 dB)
	ListDiv int      `json:"listDiv,omitempty"` // list stored as index/ListDiv (Studio EQ freqs: 8) instead of index/(n-1)
	Options []string `json:"options,omitempty"`
	// Tempo parameters only: TempoSlot 1 or 2 (the block's 1st or 2nd tempo
	// parameter, the slot /api/param/sync addresses) and Sync, the note code
	// it follows (0 = free time / speed; see SyncNotes).
	TempoSlot int `json:"tempoSlot,omitempty"`
	Sync      int `json:"sync,omitempty"`
}

type PresetItem struct {
	ID      int    `json:"id"`
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Type    uint32 `json:"type"`
	Model   string `json:"model"`
	BasedOn string `json:"basedOn,omitempty"`
	Group   string `json:"group,omitempty"`
	Short   string `json:"short,omitempty"` // FX: category label under the block (ShortCategory)
	Empty   bool   `json:"empty"`
	// DSPOver: an FX block with a model the pedal couldn't run (DSP limit
	// reached): model kept, "in use" byte 11 = 0; the LCD shows DSP OVER.
	DSPOver bool        `json:"dspOver,omitempty"`
	Active  bool        `json:"active"`
	Pos     uint16      `json:"pos"`
	Lane    uint8       `json:"lane"`
	FS      uint8       `json:"fs"`
	Assign  string      `json:"assign,omitempty"` // footswitch / EXP label under the block (AssignLabel)
	Style   *GearStyle  `json:"style,omitempty"`
	Params  []ItemParam `json:"params"`
}

type Preset struct {
	Body [PresetBodySize]byte
}

func ParsePreset(body []byte) (*Preset, error) {
	if len(body) != PresetBodySize {
		return nil, fmt.Errorf("expected %d-byte preset body, got %d", PresetBodySize, len(body))
	}
	var p Preset
	copy(p.Body[:], body)
	return &p, nil
}

func (p *Preset) Name() string {
	raw := p.Body[0:16]
	if idx := strings.IndexByte(string(raw), 0); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(string(raw))
}

func (p *Preset) SetName(name string) {
	for i := 0; i < 16; i++ {
		p.Body[i] = ' '
	}
	if len(name) > 16 {
		name = name[:16]
	}
	copy(p.Body[0:16], []byte(name))
}

// SetItemActive sets a block's on/off byte (the field Item().Active reads).
func (p *Preset) SetItemActive(id int, active bool) {
	var v byte
	if active {
		v = 1
	}
	p.Body[ItemOffset(id)+8] = v
}

// SetParamValue sets the current value of parameter paramID in block id (the
// field Item().Params[k].Value reads). Returns false if the block has no such
// parameter.
// IsIntParam: parameters the pedal handles as integers (option index): the
// 0x3F00xxxx ids, used only by Smart Harmony Key / Shift / Scale.
func IsIntParam(paramID uint32) bool { return paramID>>16 == 0x3F00 }

// SetParamInt stores an integer parameter's option index (raw, as the pedal does).
func (p *Preset) SetParamInt(id int, paramID uint32, idx uint32) bool {
	return p.SetParamValue(id, paramID, math.Float32frombits(idx))
}

func (p *Preset) SetParamValue(id int, paramID uint32, value float32) bool {
	off := ItemOffset(id)
	for k := 0; k < MaxParams; k++ {
		q := off + ParamBase + k*ParamSize
		pid := binary.LittleEndian.Uint32(p.Body[q : q+4])
		if pid == 0 && !(ItemKind(id) == "cab" && k < len(cabParamDefs)) { // cab ids are 0..3
			return false
		}
		if pid == paramID {
			binary.LittleEndian.PutUint32(p.Body[q+4:q+8], math.Float32bits(value))
			return true
		}
	}
	return false
}

func (p *Preset) Tempo() float32 {
	bits := binary.LittleEndian.Uint32(p.Body[TempoOffset : TempoOffset+4])
	return math.Float32frombits(bits)
}

func (p *Preset) GuitarInZ() string {
	initCatalog()
	if s, ok := setupByID[0x55]; ok && s.Offset < PresetBodySize {
		idx := int(p.Body[s.Offset])
		if idx >= 0 && idx < len(InZNames) {
			return InZNames[idx]
		}
	}
	return "Auto"
}

var cabParamDefs = []struct {
	Name string
	Kind string
}{
	{"Low Cut", "freq"},
	{"Res Level", "percent"},
	{"Thump", "percent"},
	{"Decay", "percent"},
}

func (p *Preset) Item(id int) PresetItem {
	initCatalog()
	off := ItemOffset(id)
	b := p.Body[:]
	modelType := binary.LittleEndian.Uint32(b[off : off+4])
	kind := ItemKind(id)
	catModel := lookupKind(kind, modelType)

	modelName := fmt.Sprintf("0x%08X", modelType)
	var basedOn, group string
	if catModel != nil {
		modelName = catModel.Name
		basedOn = catModel.BasedOn
		group = catModel.Group
	}

	// Empty: no model (type xxxxFFFF), or the block is not in use (byte 11 = 0:
	// an FX slot set to None, Amp B / Cab B of a single-amp preset).
	empty := modelType&0xFFFF == 0xFFFF || b[off+11] == 0
	pos := binary.LittleEndian.Uint16(b[off+4 : off+6])
	lane := b[off+6]
	active := b[off+8] == 1
	fs := b[off+12]

	label := fmt.Sprintf("FX %d", id-3)
	switch id {
	case 0:
		label = "Amp A"
	case 1:
		label = "Cab A"
	case 2:
		label = "Amp B"
	case 3:
		label = "Cab B"
	}

	var params []ItemParam
	tempoN := 0 // tempo parameters seen so far (their sync bytes are 9, 10)
	for k := 0; k < MaxParams; k++ {
		q := off + ParamBase + k*ParamSize
		pid := binary.LittleEndian.Uint32(b[q : q+4])
		// A zero id ends the list, except in cabs: their parameters 0..3 (Low
		// Cut, Res Level, Thump, Decay) have ids 0..3.
		if pid == 0 && !(kind == "cab" && k < len(cabParamDefs)) {
			break
		}
		pName := fmt.Sprintf("0x%08X", pid)
		pKind := "percent"
		var opts []string
		var pmin, pmax float32
		var punit string
		var pwhole bool
		var pintMin int
		var pstep float32
		var pdiv int
		if kind == "cab" && k < len(cabParamDefs) {
			pName = cabParamDefs[k].Name
			pKind = cabParamDefs[k].Kind
		} else if catModel != nil {
			for _, cp := range catModel.Params {
				if cp.ID == pid {
					pName = cp.Name
					pKind = cp.Kind
					opts = cp.Options
					pmin, pmax = cp.Min, cp.Max
					punit = cp.Unit
					pwhole = cp.Whole
					pintMin = cp.IntMin
					pstep = cp.Step
					pdiv = cp.ListDiv
					break
				}
			}
		}
		val := math.Float32frombits(binary.LittleEndian.Uint32(b[q+4 : q+8]))
		// lists stored as index/ListDiv (Studio EQ Low/Hi Freq: the pedal steps
		// 0, 0.125 … 0.875 and shows round(v·8): 0.714 = 1000 Hz, LCD 2026-11-08)
		// (most lists are index/(n-1), e.g. Tron Up Type LP/BP/HP; the ones
		// stored as index/n carry "listDiv" in the catalog — LCD-checked)
		if pdiv > 0 && len(opts) > 1 {
			idx := int(math.Round(float64(val) * float64(pdiv)))
			if idx > len(opts)-1 {
				idx = len(opts) - 1
			}
			if idx < 0 {
				idx = 0
			}
			val = float32(idx) / float32(len(opts)-1)
		}
		// integer parameters (Smart Harmony Key/Shift/Scale, ids 0x3F00xxxx):
		// the pedal stores the option index as a raw integer once edited
		// (factory presets may hold index/(n-1) as a float); the API always
		// gives the normalized value, like every list
		// (signed: Smart Harmony Shift is -8..+8 around None = 0; intMin = -8)
		if r := int32(binary.LittleEndian.Uint32(b[q+4 : q+8])); IsIntParam(pid) && len(opts) > 1 &&
			int(r) >= pintMin && int(r) <= pintMin+len(opts)-1 {
			val = float32(int(r)-pintMin) / float32(len(opts)-1)
		}
		lo := math.Float32frombits(binary.LittleEndian.Uint32(b[q+8 : q+12]))
		hi := math.Float32frombits(binary.LittleEndian.Uint32(b[q+12 : q+16]))
		ctl := b[q+16]
		ext := binary.LittleEndian.Uint32(b[q+16 : q+20])

		params = append(params, ItemParam{
			Index:   k,
			ID:      pid,
			Name:    FullParamName(pName),
			Kind:    pKind,
			Value:   val,
			Lo:      lo,
			Hi:      hi,
			Ctl:     ctl,
			Ext:     ext,
			Options: opts,
			Min:     pmin,
			Max:     pmax,
			Unit:    punit,
			Whole:   pwhole,
			IntMin:  pintMin,
			Step:    pstep,
			ListDiv: pdiv,
		})
		if pKind == "tempo" && tempoN < MaxTempoSlots {
			last := &params[len(params)-1]
			last.TempoSlot = tempoN + 1
			last.Sync = int(b[off+tempoSyncByte+tempoN])
			tempoN++
		}
	}

	if kind == "amp" {
		orderAmpTail(params)
	}
	return PresetItem{
		ID:      id,
		Label:   label,
		Kind:    kind,
		Type:    modelType,
		Model:   modelName,
		BasedOn: basedOn,
		Group:   group,
		Short:   shortOf(kind, group, modelName, empty),
		Style:   lookupStyle(kind, modelName),
		Empty:   empty,
		// (or left with type 0xFFFFFFFF after a failed delete: also stuck)
		DSPOver: kind == "fx" && (modelType&0xFFFF != 0xFFFF && b[off+11] == 0 || modelType == 0xFFFFFFFF),
		Active:  active,
		Pos:     pos,
		Lane:    lane,
		FS:      fs,
		Assign:  AssignLabel(fs, params),
		Params:  params,
	}
}

func (p *Preset) Items() []PresetItem {
	items := make([]PresetItem, 12)
	for id := 0; id < 12; id++ {
		items[id] = p.Item(id)
	}
	return items
}

// SlotLabel formats a preset index (0..63) as Bank+Channel ("1A".."16D").
func SlotLabel(presetIdx int) string {
	if presetIdx < 0 || presetIdx >= 64 {
		return fmt.Sprintf("%d", presetIdx)
	}
	bank := (presetIdx / 4) + 1
	ch := 'A' + rune(presetIdx%4)
	return fmt.Sprintf("%d%c", bank, ch)
}

// lookupKind finds a model in the table of its block kind (amp and cab tables
// share ids such as 0x7FFFF, "Amp Disabled" / "No Cab").
func lookupKind(kind string, modelType uint32) *CatalogModel {
	initCatalog()
	switch kind {
	case "amp":
		return ampsByID[modelType]
	case "cab":
		return cabsByID[modelType]
	}
	return fxByID[modelType]
}

// GearStyle is how a model is drawn: cabinet colour and archetype
// (data/gear-styles.json).
type GearStyle struct {
	Color  string `json:"color,omitempty"`  // pedals: enclosure
	Body   string `json:"body,omitempty"`   // pedals: boss | mxr | muff | round | tall | treadle | echo | spring | rack | loop
	Knob   string `json:"knob,omitempty"`   // white | black | chrome | cream
	Tolex  string `json:"tolex,omitempty"`  // amps
	Grille string `json:"grille,omitempty"` // amps
	Panel  string `json:"panel,omitempty"`  // amps
	Trim   string `json:"trim,omitempty"`   // amps
	Layout string `json:"layout,omitempty"` // amps: combo | head; cabs: 1x8 | 6x9 | 1x12 | 2x12 | 4x10 | 4x12 | 1x15 | 8x10
	// amps: tweed | silver | diamond | diamond-plate | basket | none (heads)
	GrilleType string `json:"grilleType,omitempty"`
	// amps: speakers showing through the cloth (1, 2 or 4), as the HX thumbnails
	Speakers int `json:"speakers,omitempty"`
	// amps: where the controls are: front (default) or top (on top of the
	// cabinet, not seen from the front: Vox, tweed Fenders, Supro, Gibson)
	Controls string `json:"controls,omitempty"`
	// amps, cabs: the picture's file name (no extension), from the family key:
	// "amp-tweed-b-man", "cab-412-uber". The UI shows that image when it has
	// one and its own drawing otherwise.
	Art string `json:"art,omitempty"`
}

var (
	stylesOnce sync.Once
	styles     struct {
		Pedals map[string]*GearStyle `json:"pedals"`
		Amps   map[string]*GearStyle `json:"amps"`
		Cabs   map[string]*GearStyle `json:"cabs"`
	}
)

// artSlug("amp", "Blackface ‘Lux") = "amp-blackface-lux": lower case, runs of
// anything but letters and digits become one "-".
func artSlug(kind, key string) string {
	var b strings.Builder
	b.WriteString(kind)
	dash := true
	for _, r := range strings.ToLower(key) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	return b.String()
}

func lookupStyle(kind, model string) *GearStyle {
	stylesOnce.Do(func() {
		if b, err := EmbeddedData.ReadFile("data/gear-styles.json"); err == nil {
			_ = json.Unmarshal(b, &styles)
		}
		for k, st := range styles.Amps {
			st.Art = artSlug("amp", k)
		}
		for k, st := range styles.Cabs {
			st.Art = artSlug("cab", k)
		}
	})
	switch kind {
	case "fx":
		return styles.Pedals[model]
	case "cab": // layout = the speaker arrangement (1x8, 6x9, 1x12, 2x12, 4x10, 4x12, 1x15, 8x10)
		return styles.Cabs[model]
	case "amp": // keys are amp families: "Tweed B-Man" for "Tweed B-Man Normal"
		if s, ok := styles.Amps[model]; ok {
			return s
		}
		var best string
		for k := range styles.Amps {
			if strings.HasPrefix(model, k+" ") && len(k) > len(best) {
				best = k
			}
		}
		if best != "" {
			return styles.Amps[best]
		}
	}
	return nil
}

// SameIgnoringExp reports whether two preset bodies are equal apart from the
// values of parameters assigned to EXP 1 / EXP 2. The pedal writes the
// expression pedal's live position into those values (e.g. a Volume block
// when the tuner closes), which the HD500 doesn't count as an edit.
func SameIgnoringExp(a, b []byte) bool {
	if len(a) != PresetBodySize || len(b) != PresetBodySize {
		return false
	}
	skip := make([]bool, PresetBodySize)
	for _, body := range [][]byte{a, b} {
		for idx := range ItemOrder {
			off := ItemBaseOffset + idx*ItemSize
			for k := 0; k < MaxParams; k++ {
				q := off + ParamBase + k*ParamSize
				if q+ParamSize > PresetBodySize {
					break
				}
				if c := body[q+16]; c == 1 || c == 2 { // controller byte: 1 = EXP 1, 2 = EXP 2
					for i := q + 4; i < q+8; i++ { // the value (float32)
						skip[i] = true
					}
				}
			}
		}
	}
	for i := range a {
		if a[i] != b[i] && !skip[i] {
			return false
		}
	}
	return true
}

// Amp tone knobs with model-specific labels (POD HD500 manual, "Amp Control
// Labels"): the parameter ids are the same for every amp, only the names the
// pedal's Amp:Edit screen shows change. Applies to the Preamp versions too.
var ampKnobLabels = map[string]map[uint32]string{
	// The Supro only has Drive and Tone: Tone is mapped to the MID knob.
	"Super O": {0x3F100001: "Tone"},
	// Two channels: DRIVE = clean channel, BASS = drive of the dirty one;
	// MID and TREBLE act as the amp's Tone and Cut.
	"Divide 9/15": {0x3F100003: "Drive 1", 0x3F100000: "Drive 2", 0x3F100001: "Tone", 0x3F100002: "Cut"},
	// Vox design: MID is "Cut" (counter-clockwise reduces the highs).
	"Class A-15":    {0x3F100001: "Cut"},
	"Class A-30 TB": {0x3F100001: "Cut"},
}

func relabelAmpKnobs(m *CatalogModel) {
	labels := ampKnobLabels[strings.TrimSuffix(m.Name, " Preamp")]
	for i := range m.Params {
		if l, ok := labels[m.Params[i].ID]; ok {
			m.Params[i].Name = l
		} else if m.Params[i].Name == "Volume" {
			// the channel volume knob, as HX Edit names it (user, 2026-11-08)
			m.Params[i].Name = "Ch Vol"
		}
	}
}

// StyleOf returns how a model is drawn (kind "amp" — preamps included — or
// "fx"); nil when gear-styles.json has nothing for it.
func StyleOf(kind, model string) *GearStyle { return lookupStyle(kind, model) }

// ampTailOrder: the amp's power-amp knobs in the order HX Edit lists them
// (user, 2026-11-08): Master, Sag, Hum, Bias, Bias X. They keep the slots
// they already take in the list; only their order among themselves changes.
// Each ItemParam keeps its storage Index (the API's parameter index).
var ampTailOrder = map[string]int{"Master": 0, "Sag": 1, "Hum": 2, "Bias": 3, "Bias X": 4}

func orderAmpTail(ps []ItemParam) {
	var slots []int
	var tail []ItemParam
	for i, p := range ps {
		if _, ok := ampTailOrder[p.Name]; ok {
			slots = append(slots, i)
			tail = append(tail, p)
		}
	}
	sort.SliceStable(tail, func(a, b int) bool { return ampTailOrder[tail[a].Name] < ampTailOrder[tail[b].Name] })
	for i, s := range slots {
		ps[s] = tail[i]
	}
}
