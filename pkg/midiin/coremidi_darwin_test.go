//go:build darwin

package midiin

import (
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The read callback runs on a CoreMIDI thread: send a message from a virtual
// source of this process and check that Open delivers it, without the pedal.
func TestOpenReceivesVirtualSource(t *testing.T) {
	if err := initCoreMIDI(); err != nil {
		t.Skip(err)
	}
	cm, err := purego.Dlopen("/System/Library/Frameworks/CoreMIDI.framework/CoreMIDI", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip(err)
	}
	var sourceCreate func(client uint32, name uintptr, out *uint32) int32
	var received func(src uint32, list unsafe.Pointer) int32
	purego.RegisterLibFunc(&sourceCreate, cm, "MIDISourceCreate")
	purego.RegisterLibFunc(&received, cm, "MIDIReceived")

	var client, src uint32
	cn, sn := cfStr("hd500-test-client"), cfStr("hd500-test-source")
	if st := midiClientCreate(cn, 0, 0, &client); st != 0 {
		t.Skipf("MIDIClientCreate: %d", st)
	}
	defer midiClientDispose(client)
	if st := sourceCreate(client, sn, &src); st != 0 {
		t.Skipf("MIDISourceCreate: %d", st)
	}

	got := make(chan Message, 4)
	name, closeFn, err := Open("hd500-test-source", func(m Message) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	if name != "hd500-test-source" {
		t.Logf("source name %q", name)
	}

	// MIDIPacketList: UInt32 count; packet {UInt64 timestamp, UInt16 length, data}
	buf := make([]byte, 64)
	*(*uint32)(unsafe.Pointer(&buf[0])) = 1
	*(*uint16)(unsafe.Pointer(&buf[12])) = 3
	copy(buf[14:], []byte{0xB0, 80, 127})
	time.Sleep(100 * time.Millisecond)
	if st := received(src, unsafe.Pointer(&buf[0])); st != 0 {
		t.Fatalf("MIDIReceived: %d", st)
	}
	select {
	case m := <-got:
		if m != (Message{Kind: Control, Channel: 1, Data1: 80, Data2: 127}) {
			t.Fatalf("got %v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message delivered by the CoreMIDI callback")
	}
}
