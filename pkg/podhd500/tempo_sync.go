package podhd500

// Tempo sync (verified on the pedal, 2026-11-07). A block's "tempo"
// parameters (delay Time in ms, modulation/filter Speed in Hz) can follow the
// tap tempo as a note value instead. The note is not in the parameter: it is
// a byte of the block header — byte 9 for the block's 1st tempo parameter,
// byte 10 for the 2nd (only Stereo Delay has two). 0 = free (ms / Hz);
// 2.. = SyncNotes index. Turning the value knob past its maximum on the pedal
// enters the note list; the pedal reports the change with Cmd 0x14 (1st) or
// 0x31 (2nd): [0, block, note]. The parameter value itself is left as is.
const tempoSyncByte = 9

// MaxTempoSlots is the number of tempo parameters a block can sync.
const MaxTempoSlots = 2

// SyncNotes names the note values by their stored code (codes 2..20, as the
// pedal steps through them after the maximum time: semibreve, dotted minim,
// minim…; 2–7 checked on the LCD, the rest follow the same pattern).
var SyncNotes = []string{
	"", "",
	"Whole", "1/2 Dotted", "1/2", "1/2 Triplet",
	"1/4 Dotted", "1/4", "1/4 Triplet",
	"1/8 Dotted", "1/8", "1/8 Triplet",
	"1/16 Dotted", "1/16", "1/16 Triplet",
	"1/32 Dotted", "1/32", "1/32 Triplet",
	"1/64 Dotted", "1/64", "1/64 Triplet",
}

// SyncNoteBeats is each note's length in quarter-note beats (for converting
// between a time and the nearest note at the current tempo).
var SyncNoteBeats = []float64{
	0, 0,
	4, 3, 2, 4.0 / 3,
	1.5, 1, 2.0 / 3,
	0.75, 0.5, 1.0 / 3,
	0.375, 0.25, 1.0 / 6,
	0.1875, 0.125, 1.0 / 12,
	0.09375, 0.0625, 1.0 / 24,
}

// ValidSyncNote reports whether v is 0 (free) or a known note code.
func ValidSyncNote(v int) bool { return v == 0 || (v >= 2 && v < len(SyncNotes)) }

// TempoSync returns the note code of the block's tempo slot (0 or 1).
func (p *Preset) TempoSync(id, slot int) int {
	if slot < 0 || slot >= MaxTempoSlots {
		return 0
	}
	return int(p.Body[ItemOffset(id)+tempoSyncByte+slot])
}

// SetTempoSync stores the note code of a block's tempo slot; false if unchanged.
func (p *Preset) SetTempoSync(id, slot, note int) bool {
	if slot < 0 || slot >= MaxTempoSlots || !ValidSyncNote(note) {
		return false
	}
	off := ItemOffset(id) + tempoSyncByte + slot
	if int(p.Body[off]) == note {
		return false
	}
	p.Body[off] = byte(note)
	return true
}
