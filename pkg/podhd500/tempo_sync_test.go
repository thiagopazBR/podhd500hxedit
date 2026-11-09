package podhd500

import "testing"

func TestTempoSync(t *testing.T) {
	p := &Preset{}
	id := 4
	if !p.SetTempoSync(id, 1, 7) || p.TempoSync(id, 1) != 7 || p.TempoSync(id, 0) != 0 {
		t.Fatal("slot 2 not stored on its own")
	}
	if p.Body[ItemOffset(id)+10] != 7 {
		t.Fatal("2nd tempo slot must be header byte 10")
	}
	if p.SetTempoSync(id, 1, 7) {
		t.Fatal("unchanged value reported as a change")
	}
	if p.SetTempoSync(id, 0, 1) || p.SetTempoSync(id, 0, len(SyncNotes)) || p.SetTempoSync(id, 2, 3) {
		t.Fatal("invalid note or slot accepted")
	}
	if len(SyncNotes) != len(SyncNoteBeats) || SyncNotes[2] != "Whole" || SyncNotes[7] != "1/4" {
		t.Fatal("note tables out of step")
	}
}
