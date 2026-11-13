//go:build darwin

package midiin

// CoreMIDI input through purego (no cgo): the same approach as the USB code.

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const kCFStringEncodingUTF8 = 0x08000100

var (
	initOnce sync.Once
	initErr  error

	midiGetNumberOfSources func() uint
	midiGetSource          func(i uint) uint32
	midiObjectGetString    func(obj uint32, prop uintptr, out *uintptr) int32
	midiClientCreate       func(name uintptr, notify uintptr, refcon uintptr, out *uint32) int32
	midiInputPortCreate    func(client uint32, name uintptr, readProc uintptr, refcon uintptr, out *uint32) int32
	midiPortConnectSource  func(port uint32, src uint32, refcon uintptr) int32
	midiPortDispose        func(port uint32) int32
	midiClientDispose      func(client uint32) int32
	cfStringCreate         func(alloc uintptr, s *byte, enc uint32) uintptr
	cfStringGetCString     func(s uintptr, buf *byte, size int, enc uint32) bool
	cfRelease              func(obj uintptr)
	propDisplayName        uintptr // CFString "displayName" (kMIDIPropertyDisplayName)
)

func initCoreMIDI() error {
	initOnce.Do(func() {
		cm, err := purego.Dlopen("/System/Library/Frameworks/CoreMIDI.framework/CoreMIDI", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			initErr = err
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			initErr = err
			return
		}
		purego.RegisterLibFunc(&midiGetNumberOfSources, cm, "MIDIGetNumberOfSources")
		purego.RegisterLibFunc(&midiGetSource, cm, "MIDIGetSource")
		purego.RegisterLibFunc(&midiObjectGetString, cm, "MIDIObjectGetStringProperty")
		purego.RegisterLibFunc(&midiClientCreate, cm, "MIDIClientCreate")
		purego.RegisterLibFunc(&midiInputPortCreate, cm, "MIDIInputPortCreate")
		purego.RegisterLibFunc(&midiPortConnectSource, cm, "MIDIPortConnectSource")
		purego.RegisterLibFunc(&midiPortDispose, cm, "MIDIPortDispose")
		purego.RegisterLibFunc(&midiClientDispose, cm, "MIDIClientDispose")
		purego.RegisterLibFunc(&cfStringCreate, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cfStringGetCString, cf, "CFStringGetCString")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		// kMIDIPropertyDisplayName is CFSTR("displayName"); reading the exported
		// variable gave a null pointer, so the property name is built here.
		propDisplayName = cfStr("displayName")
	})
	return initErr
}

func cfStr(s string) uintptr {
	b := append([]byte(s), 0)
	return cfStringCreate(0, &b[0], kCFStringEncodingUTF8)
}

func sourceName(src uint32) string {
	var s uintptr
	if midiObjectGetString(src, propDisplayName, &s) != 0 || s == 0 {
		return ""
	}
	defer cfRelease(s)
	buf := make([]byte, 256)
	if !cfStringGetCString(s, &buf[0], len(buf), kCFStringEncodingUTF8) {
		return ""
	}
	for i, c := range buf {
		if c == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// List returns the names of the MIDI inputs.
func List() ([]string, error) {
	if err := initCoreMIDI(); err != nil {
		return nil, err
	}
	var out []string
	for i := uint(0); i < midiGetNumberOfSources(); i++ {
		out = append(out, sourceName(midiGetSource(i)))
	}
	return out, nil
}

// handlers maps a port to its message handler (the read callback is shared:
// purego allows a limited number of callbacks, so there is exactly one).
var (
	cbOnce   sync.Once
	cbPtr    uintptr
	handlers sync.Map // refcon (uintptr) -> func(Message)
)

// readProc is CoreMIDI's MIDIReadProc: (const MIDIPacketList*, refCon, srcRefCon).
// A packet list is a UInt32 count followed by packed packets: UInt64 timestamp,
// UInt16 length, data[length]; the next packet starts 4-byte aligned.
func readProc(list, refcon, _ uintptr) uintptr {
	h, ok := handlers.Load(refcon)
	if !ok || list == 0 {
		return 0
	}
	// copy the start of the list at once: CoreMIDI owns the memory only during this call
	var head [48]byte
	copy(head[:], unsafe.Slice((*byte)(unsafe.Pointer(list)), len(head)))
	n := *(*uint32)(unsafe.Pointer(&head[0]))
	if Debug {
		fmt.Fprintf(os.Stderr, "[MIDI raw] list=%#x packets=%d bytes=% x\n", list, n, head[:])
	}
	p := list + 4
	for i := uint32(0); i < n; i++ {
		length := uintptr(*(*uint16)(unsafe.Pointer(p + 8)))
		if length > 0 && length <= 256 {
			data := unsafe.Slice((*byte)(unsafe.Pointer(p+10)), length)
			for _, m := range Parse(data) {
				h.(func(Message))(m)
			}
		}
		p = (p + 10 + length + 3) &^ 3
	}
	return 0
}

var nextRef uintptr = 1

// Open connects to the first MIDI input whose name contains match (case
// insensitive; "" = the first one) and calls onMsg for every channel message,
// on a CoreMIDI thread: it must return quickly. The returned func closes it.
func Open(match string, onMsg func(Message)) (string, func(), error) {
	if err := initCoreMIDI(); err != nil {
		return "", nil, err
	}
	var src uint32
	var name string
	found := false
	for i := uint(0); i < midiGetNumberOfSources(); i++ {
		s := midiGetSource(i)
		n := sourceName(s)
		if strings.Contains(strings.ToLower(n), strings.ToLower(match)) {
			src, name, found = s, n, true
			break
		}
	}
	if !found {
		return "", nil, fmt.Errorf("no MIDI input matching %q (list them with -midi-list)", match)
	}
	cbOnce.Do(func() { cbPtr = purego.NewCallback(readProc) })
	var client, port uint32
	cname, pname := cfStr("hd500-snap"), cfStr("input")
	defer cfRelease(cname)
	defer cfRelease(pname)
	if st := midiClientCreate(cname, 0, 0, &client); st != 0 {
		return "", nil, fmt.Errorf("MIDIClientCreate: OSStatus %d", st)
	}
	ref := nextRef
	nextRef++
	handlers.Store(ref, onMsg)
	if st := midiInputPortCreate(client, pname, cbPtr, ref, &port); st != 0 {
		midiClientDispose(client)
		handlers.Delete(ref)
		return "", nil, fmt.Errorf("MIDIInputPortCreate: OSStatus %d", st)
	}
	if st := midiPortConnectSource(port, src, 0); st != 0 {
		midiPortDispose(port)
		midiClientDispose(client)
		handlers.Delete(ref)
		return "", nil, errors.New("MIDIPortConnectSource failed: OSStatus " + fmt.Sprint(st))
	}
	return name, func() {
		midiPortDispose(port)
		midiClientDispose(client)
		handlers.Delete(ref)
	}, nil
}
