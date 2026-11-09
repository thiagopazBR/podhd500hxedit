package podhd500

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// Files of the original POD HD500 Edit:
//
//	.h5e preset   = 40-byte header ("H5EP") + 4096-byte body              =   4136 B
//	.h5s setlist  = 40-byte header ("H5ES") + 16-byte name + 64 × .h5e    = 264760 B
//	.h5b bundle   = 40-byte header ("H5EB") + 8 × .h5s                    = 2118120 B
//
// Header words are big-endian. The body in a file is the pedal's body with
// some 32-bit fields byte-swapped (big-endian in the file, little-endian on
// USB): see swappedWords. Layout confirmed on factory files; the field map
// follows the conversion routine of POD HD500 Edit as documented by the
// hd500-editor project (MIT).
//
// The POD HD500X uses the same layout (.5xe/.5xs/.5xb, model byte 0x2D).

const (
	FileHeaderSize  = 40
	SetNameSize     = 16
	H5ESize         = FileHeaderSize + PresetBodySize           // 4136
	H5SSize         = FileHeaderSize + SetNameSize + 64*H5ESize // 264760
	H5BSize         = FileHeaderSize + 8*H5SSize                // 2118120
	FilePresetSlots = 64
	FileSetlists    = 8
)

// Model bytes at header offset 0x0B.
const (
	FileModelHD500  = 0x27
	FileModelHD500X = 0x2D
)

var (
	magicPreset  = []byte("H5EP")
	magicSetlist = []byte("H5ES")
	magicBundle  = []byte("H5EB")
)

// FileKind tells what a file holds.
type FileKind string

const (
	KindPreset  FileKind = "preset"
	KindSetlist FileKind = "setlist"
	KindBundle  FileKind = "bundle"
)

// Ext returns the HD500 file extension for a kind.
func (k FileKind) Ext() string {
	switch k {
	case KindPreset:
		return ".h5e"
	case KindSetlist:
		return ".h5s"
	case KindBundle:
		return ".h5b"
	}
	return ""
}

// File is a decoded .h5e / .h5s / .h5b. Bodies are in the pedal's (USB)
// byte order, ready for PRESET_SET.
type File struct {
	Kind     FileKind
	Model    byte       // FileModelHD500 / FileModelHD500X / other
	SetNames []string   // setlist / bundle: one per setlist
	Bodies   [][][]byte // [setlist][slot]body; a preset file has one setlist with one slot
}

var swapped = swappedWords()

func swappedWords() []int {
	seen := map[int]bool{}
	var w []int
	add := func(o int) {
		if !seen[o] {
			seen[o] = true
			w = append(w, o)
		}
	}
	add(0x20)
	add(0x24)
	for o := 0xD48; o < 0xD84; o += 4 { // tempo and its neighbours
		add(o)
	}
	for it := 0; it < 12; it++ {
		o := ItemBaseOffset + it*ItemSize
		add(o)     // model type
		add(o + 4) // position / lane
		for p := 0; p < MaxParams; p++ {
			for k := 0; k < 4; k++ { // id, value, heel, toe
				add(o + ParamBase + p*ParamSize + 4*k)
			}
		}
	}
	return w
}

// swapBody converts a body between file and pedal byte order (its own inverse).
func swapBody(body []byte) []byte {
	out := append([]byte(nil), body...)
	for _, o := range swapped {
		v := binary.LittleEndian.Uint32(out[o:])
		binary.BigEndian.PutUint32(out[o:], v)
	}
	return out
}

// header builds the 40-byte header. Field values copied from files written
// by POD HD500 Edit 2.27 for firmware 2.62. The preset header stores its
// words in a different order than the setlist/bundle one.
func header(magic []byte) []byte {
	h := make([]byte, FileHeaderSize)
	copy(h, magic)
	be := binary.BigEndian
	be.PutUint32(h[4:], 1)
	if bytes.Equal(magic, magicPreset) {
		be.PutUint32(h[8:], 0x7D010000|FileModelHD500)
		be.PutUint32(h[12:], 0x02270000)
		be.PutUint32(h[16:], 0x00140002)
		be.PutUint32(h[20:], 0x02620000)
	} else {
		be.PutUint32(h[8:], 0x00140002)
		be.PutUint32(h[12:], 0x02620000)
		be.PutUint32(h[16:], 0x02270000)
		be.PutUint32(h[20:], 0x7D010000|FileModelHD500)
	}
	return h
}

// EncodePreset builds a .h5e from a pedal body.
func EncodePreset(body []byte) ([]byte, error) {
	if len(body) != PresetBodySize {
		return nil, fmt.Errorf("preset has %d bytes (expected %d)", len(body), PresetBodySize)
	}
	return append(header(magicPreset), swapBody(body)...), nil
}

func encodeSetlist(name string, bodies [][]byte) ([]byte, error) {
	if len(bodies) != FilePresetSlots {
		return nil, fmt.Errorf("setlist has %d presets (expected %d)", len(bodies), FilePresetSlots)
	}
	out := make([]byte, 0, H5SSize)
	out = append(out, header(magicSetlist)...)
	nm := make([]byte, SetNameSize) // zero padded
	copy(nm, name)
	out = append(out, nm...)
	for _, b := range bodies {
		e, err := EncodePreset(b)
		if err != nil {
			return nil, err
		}
		out = append(out, e...)
	}
	return out, nil
}

// EncodeSetlist builds a .h5s.
func EncodeSetlist(name string, bodies [][]byte) ([]byte, error) {
	return encodeSetlist(name, bodies)
}

// EncodeBundle builds a .h5b (the whole pedal: 8 setlists × 64 presets).
func EncodeBundle(names []string, bodies [][][]byte) ([]byte, error) {
	if len(names) != FileSetlists || len(bodies) != FileSetlists {
		return nil, fmt.Errorf("a bundle needs %d setlists", FileSetlists)
	}
	out := make([]byte, 0, H5BSize)
	out = append(out, header(magicBundle)...)
	for i := range bodies {
		s, err := encodeSetlist(names[i], bodies[i])
		if err != nil {
			return nil, fmt.Errorf("setlist %d: %w", i+1, err)
		}
		out = append(out, s...)
	}
	return out, nil
}

// Decode reads a .h5e / .h5s / .h5b (or the HD500X .5xe / .5xs / .5xb).
// It checks sizes, magics and that every block holds a known model.
func Decode(data []byte) (*File, error) {
	if len(data) < 4 {
		return nil, errors.New("empty or truncated file")
	}
	f := &File{}
	var err error
	switch m := data[:4]; {
	case bytes.Equal(m, magicPreset):
		f.Kind = KindPreset
		if len(data) != H5ESize {
			return nil, fmt.Errorf(".h5e preset has %d bytes (expected %d)", len(data), H5ESize)
		}
		f.Model = data[0x0B]
		var b []byte
		if b, err = decodePreset(data); err == nil {
			f.SetNames = []string{""}
			f.Bodies = [][][]byte{{b}}
		}
	case bytes.Equal(m, magicSetlist):
		f.Kind = KindSetlist
		if len(data) != H5SSize {
			return nil, fmt.Errorf(".h5s setlist has %d bytes (expected %d)", len(data), H5SSize)
		}
		f.Model = data[0x17]
		var name string
		var bodies [][]byte
		if name, bodies, err = decodeSetlist(data); err == nil {
			f.SetNames = []string{name}
			f.Bodies = [][][]byte{bodies}
		}
	case bytes.Equal(m, magicBundle):
		f.Kind = KindBundle
		if len(data) != H5BSize {
			return nil, fmt.Errorf(".h5b bundle has %d bytes (expected %d)", len(data), H5BSize)
		}
		f.Model = data[0x17]
		for i := 0; i < FileSetlists && err == nil; i++ {
			o := FileHeaderSize + i*H5SSize
			var name string
			var bodies [][]byte
			name, bodies, err = decodeSetlist(data[o : o+H5SSize])
			if err != nil {
				err = fmt.Errorf("setlist %d: %w", i+1, err)
				break
			}
			f.SetNames = append(f.SetNames, name)
			f.Bodies = append(f.Bodies, bodies)
		}
	default:
		return nil, errors.New("not a POD HD500 Edit file (.h5e/.h5s/.h5b)")
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

func decodePreset(rec []byte) ([]byte, error) {
	if len(rec) != H5ESize || !bytes.Equal(rec[:4], magicPreset) {
		return nil, errors.New("invalid preset record")
	}
	body := swapBody(rec[FileHeaderSize:])
	if err := CheckBody(body); err != nil {
		return nil, err
	}
	return body, nil
}

func decodeSetlist(data []byte) (string, [][]byte, error) {
	if !bytes.Equal(data[:4], magicSetlist) {
		return "", nil, errors.New("invalid setlist header")
	}
	raw := data[FileHeaderSize : FileHeaderSize+SetNameSize]
	if i := bytes.IndexByte(raw, 0); i >= 0 {
		raw = raw[:i]
	}
	name := strings.TrimSpace(string(raw))
	bodies := make([][]byte, FilePresetSlots)
	for i := range bodies {
		o := FileHeaderSize + SetNameSize + i*H5ESize
		b, err := decodePreset(data[o : o+H5ESize])
		if err != nil {
			return "", nil, fmt.Errorf("preset %s: %w", SlotLabel(i), err)
		}
		bodies[i] = b
	}
	return name, bodies, nil
}

// CheckBody sanity-checks a body in pedal byte order: every amp/cab/FX block
// is empty or a model the HD500 knows. A wrong byte order fails here.
func CheckBody(body []byte) error {
	if len(body) != PresetBodySize {
		return fmt.Errorf("preset has %d bytes", len(body))
	}
	for it := 0; it < 12; it++ {
		o := ItemOffset(it)
		t := binary.LittleEndian.Uint32(body[o:])
		if t&0xFFFF == 0xFFFF { // empty block (0x0007FFFF, 0x0107FFFF, 0x020DFFFF…)
			continue
		}
		if LookupModel(t) == nil {
			return fmt.Errorf("block %d has unknown model 0x%08x", it, t)
		}
	}
	return nil
}
