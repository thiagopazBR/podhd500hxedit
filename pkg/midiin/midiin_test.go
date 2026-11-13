package midiin

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	got := Parse([]byte{0xB0, 80, 127, 0xC1, 5, 0x90, 60, 100})
	want := []Message{
		{Control, 1, 80, 127},
		{Program, 2, 5, 0},
		{NoteOn, 1, 60, 100},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseRunningStatusRealtimeAndGarbage(t *testing.T) {
	// running status (second CC without a status byte), a clock byte inside, leading data bytes
	got := Parse([]byte{12, 34, 0xB2, 81, 127, 0xF8, 82, 127})
	want := []Message{{Control, 3, 81, 127}, {Control, 3, 82, 127}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// sysex is skipped, a truncated message is dropped
	if got := Parse([]byte{0xF0, 1, 2, 3, 0xF7, 0xB0, 80}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestSnapshotFor(t *testing.T) {
	cases := []struct {
		m      Message
		cc, ch int
		want   int
	}{
		{Message{Control, 1, 80, 127}, 80, 0, 1},
		{Message{Control, 5, 83, 127}, 80, 0, 4},
		{Message{Control, 1, 84, 127}, 80, 0, 0}, // outside 80..83
		{Message{Control, 1, 79, 127}, 80, 0, 0},
		{Message{Control, 1, 80, 0}, 80, 0, 0},   // release: ignored
		{Message{Control, 2, 80, 127}, 80, 1, 0}, // other channel
		{Message{Control, 1, 80, 127}, 80, 1, 1},
		{Message{Program, 1, 80, 0}, 80, 0, 0},
	}
	for _, c := range cases {
		if got := SnapshotFor(c.m, c.cc, c.ch); got != c.want {
			t.Errorf("SnapshotFor(%v, %d, %d) = %d, want %d", c.m, c.cc, c.ch, got, c.want)
		}
	}
}
