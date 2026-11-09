package usb

import (
	"bytes"
	"testing"
)

// Transfers captured from a real POD HD500 on 2026-11-03 (bulk IN 0x81).
func TestReassemblerSplitHeader(t *testing.T) {
	var r Reassembler
	// ALERT (0x23) whose 8-byte SPSI header is split across two transfers.
	if got := r.Feed([]byte{0x04, 0x00, 0x01, 0x00, 0x01, 0x00, 0x0a, 0x05}); len(got) != 0 {
		t.Fatalf("expected no message from half a header, got %d", len(got))
	}
	got := r.Feed([]byte{0x04, 0x00, 0x04, 0x00, 0x00, 0x40, 0x00, 0x23})
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	msg, err := ParseSPSI(got[0])
	if err != nil {
		t.Fatal(err)
	}
	if msg.Cmd != CmdAlert || msg.SrcAddr != AddrPedal || msg.DestAddr != AddrHostX4 {
		t.Fatalf("unexpected message %+v", msg)
	}
}

func TestReassemblerSingleChunk(t *testing.T) {
	var r Reassembler
	// STATUS_RESP (0x22): status 9 (current setlist) = 6
	got := r.Feed([]byte{
		0x14, 0x00, 0x01, 0x00,
		0x04, 0x00, 0x0a, 0x08, 0x01, 0x40, 0x00, 0x22,
		0x00, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	msg, _ := ParseSPSI(got[0])
	if msg.Cmd != CmdStatusResp || msg.Word(1) != StatusIDCurrentSet || msg.Word(2) != 6 {
		t.Fatalf("unexpected message %+v", msg)
	}
}

func TestReassemblerMultiChunkPreset(t *testing.T) {
	// Build a 4104-byte PRESET_LOAD and deliver it the way the pedal does:
	// 60-byte chunks, one per 64-byte transfer.
	body := make([]byte, PresetBodySize)
	copy(body, "New Tone")
	spsi := BuildFrameWithSource(AddrHostX4, CmdPresetLoad, []uint32{0}, body)

	var r Reassembler
	var got [][]byte
	for off := 0; off < len(spsi); off += 60 {
		end := min(off+60, len(spsi))
		flag := byte(chunkContinuation)
		if off == 0 {
			flag = chunkFirst
		}
		tr := append([]byte{byte(end - off), 0, flag, 0}, spsi[off:end]...)
		got = append(got, r.Feed(tr)...)
	}
	if len(got) != 1 || !bytes.Equal(got[0], spsi) {
		t.Fatalf("reassembly mismatch: %d messages", len(got))
	}
}

func TestFrameSPSIRoundTrip(t *testing.T) {
	for _, size := range []int{12, 60, 64, 4108} {
		spsi := BuildFrameWithSource(AddrHostHD, CmdPresetSet, nil, make([]byte, size-8))
		pkts := FrameSPSI(spsi)
		if want := (size + ChunkPayload - 1) / ChunkPayload; len(pkts) != want {
			t.Fatalf("size %d: %d packets, want %d", size, len(pkts), want)
		}
		var r Reassembler
		var got [][]byte
		for i, p := range pkts {
			if len(p) > 64 {
				t.Fatalf("size %d: packet %d is %d bytes (> 64)", size, i, len(p))
			}
			got = append(got, r.Feed(p)...)
		}
		if len(got) != 1 || !bytes.Equal(got[0], spsi) {
			t.Fatalf("size %d: round trip failed (%d messages)", size, len(got))
		}
	}
	// Single packet must match what the pedal accepted live.
	f := FrameSPSI(BuildFrameWithSource(AddrHostHD, CmdStatusQuery, []uint32{9}, nil))
	want := []byte{0x0c, 0x00, 0x01, 0x00, 0x02, 0x00, 0x0a, 0x40, 0x01, 0x08, 0x00, 0x21, 0x09, 0x00, 0x00, 0x00}
	if len(f) != 1 || !bytes.Equal(f[0], want) {
		t.Fatalf("frame = %x, want %x", f, want)
	}
}
