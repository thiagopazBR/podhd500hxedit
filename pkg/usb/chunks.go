package usb

import "encoding/binary"

// Bulk packet framing used on the POD HD500 control interface (EP 0x01/0x81).
//
// Every SPSI message travels as one or more USB packets of at most 64 bytes,
// each with a 4-byte header:
//
//	[0..1] uint16LE payload length of this packet (<= 60)
//	[2]    0x01 = first packet of a message, 0x04 = continuation
//	[3]    0x00
//
// Each packet must be its own bulk transfer. The pedal sends this way (60-byte
// payloads, even splitting the 8-byte SPSI header), refs/hd500-editor writes
// this way, and on 2026-11-03 a 4108-byte PRESET_SET sent as 252-byte chunks
// in one transfer (the layout in the Line 6 kext's
// BuildOutputFromL6SPSIAndSend) got no PRESET_SET_ACK.
const (
	chunkFirst        = 0x01
	chunkContinuation = 0x04
	ChunkPayload      = 60
)

// FrameSPSI splits a complete SPSI message into USB packets for EP 0x01.
// Write each returned packet as a separate bulk transfer, in order.
func FrameSPSI(spsi []byte) [][]byte {
	var pkts [][]byte
	for off := 0; off < len(spsi); off += ChunkPayload {
		end := min(off+ChunkPayload, len(spsi))
		flag := byte(chunkContinuation)
		if off == 0 {
			flag = chunkFirst
		}
		pkt := make([]byte, 4, 4+end-off)
		binary.LittleEndian.PutUint16(pkt, uint16(end-off))
		pkt[2] = flag
		pkts = append(pkts, append(pkt, spsi[off:end]...))
	}
	return pkts
}

// Reassembler turns bulk IN transfers from EP 0x81 back into complete SPSI
// messages. It is not safe for concurrent use.
type Reassembler struct {
	buf []byte
}

// Feed consumes one bulk IN transfer and returns every SPSI message it
// completed (each a fresh slice).
func (r *Reassembler) Feed(transfer []byte) [][]byte {
	var msgs [][]byte
	for off := 0; off+4 <= len(transfer); {
		n := int(binary.LittleEndian.Uint16(transfer[off:]))
		flag := transfer[off+2]
		start := off + 4
		end := min(start+n, len(transfer))
		if flag == chunkFirst && len(r.buf) > 0 {
			// a new message started before the previous one completed: drop the stale bytes
			r.buf = r.buf[:0]
		}
		r.buf = append(r.buf, transfer[start:end]...)
		off = start + n

		for len(r.buf) >= 2 {
			want := (int(binary.LittleEndian.Uint16(r.buf)) + 1) * 4
			if len(r.buf) < want {
				break
			}
			msg := make([]byte, want)
			copy(msg, r.buf[:want])
			msgs = append(msgs, msg)
			r.buf = append(r.buf[:0], r.buf[want:]...)
		}
	}
	return msgs
}
