package editor

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// The chain of FX blocks. Each of the 8 FX slots (blocks 4..11, empty ones
// included) has a lane (byte 6) and a position 0..7 (u16 at byte 4). The
// position only orders the blocks inside a lane: factory presets show any
// interleaving across lanes. Learned from the pedal's own MOVE (it rewrites
// the whole preset and pushes PRESET_LOAD) and factory files.

// Lanes of FX blocks.
const (
	LanePre   = 0 // before the A/B split
	LaneAPre  = 1 // path A, before amp A
	LaneBPre  = 2 // path B, before amp B (the only B lane without amp B)
	LaneAPost = 3 // path A, after amp A
	LaneBPost = 4 // path B, after amp B
	LanePost  = 5 // after the mixer
	NumLanes  = 6
)

// LaneNames are stable identifiers for the API.
var LaneNames = [NumLanes]string{"pre", "aPre", "bPre", "aPost", "bPost", "post"}

// ChainLane lists the FX blocks of one lane, in signal order.
type ChainLane struct {
	Lane   int    `json:"lane"`
	Name   string `json:"name"`
	Blocks []int  `json:"blocks"` // block ids 4..11, empty slots included
}

// Chain is the signal flow as the API exposes it.
type Chain struct {
	Lanes []ChainLane `json:"lanes"` // always 6, in lane order (not signal order)
	Dual  bool        `json:"dual"`  // amp B in use
	// AmpPos is where the (single) amp sits in the signal flow: "pre" (end of
	// the pre section, before the split), "a" (path A), "b" (path B) or
	// "post" (start of the post section, after the mixer). With two amps it
	// is "a" (amp A on path A, amp B on path B); a lone amp on path B is amp
	// B with amp A disabled, so it is "a" too (dual).
	AmpPos string `json:"ampPos"`
}

// Amp position codes: u16 at byte 4 of amp A (the cab stores code+1, or 3).
// From factory presets and the LCD: 5 = Pre (1C "New Tone", LCD-verified),
// 7 = Post (6A "1 Coil Spank" in Best Of HD, LCD-verified), 0 = path A
// (always with two amps). A single amp on path B is stored as two amps
// with amp A disabled (4B in User 2, LCD-verified: "AMP DISABLED" on path A).
const (
	ampPosPathA = 0
	ampPosPre   = 5
	ampPosPost  = 7
)

func ampPosOf(p *podhd500.Preset) string {
	if !p.Item(2).Empty {
		return "a"
	}
	switch binary.LittleEndian.Uint16(p.Body[podhd500.ItemOffset(0)+4:]) {
	case ampPosPre:
		return "pre"
	case ampPosPost:
		return "post"
	}
	return "a"
}

const firstFX, lastFX = 4, 11

type fxSlot struct {
	id, lane, pos int
}

func fxSlots(p *podhd500.Preset) []fxSlot {
	out := make([]fxSlot, 0, 8)
	for id := firstFX; id <= lastFX; id++ {
		o := podhd500.ItemOffset(id)
		out = append(out, fxSlot{id, int(p.Body[o+6]), int(binary.LittleEndian.Uint16(p.Body[o+4:]))})
	}
	return out
}

// chainOf groups the FX slots by lane, ordered by position.
func chainOf(p *podhd500.Preset) Chain {
	c := Chain{Lanes: make([]ChainLane, NumLanes), Dual: !p.Item(2).Empty, AmpPos: ampPosOf(p)}
	for l := range c.Lanes {
		c.Lanes[l] = ChainLane{Lane: l, Name: LaneNames[l], Blocks: []int{}}
	}
	slots := fxSlots(p)
	sort.SliceStable(slots, func(i, j int) bool { return slots[i].pos < slots[j].pos })
	for _, s := range slots {
		if s.lane >= 0 && s.lane < NumLanes {
			c.Lanes[s.lane].Blocks = append(c.Lanes[s.lane].Blocks, s.id)
		}
	}
	return c
}

// moveFX moves FX block id into lane, before block `before` (which must be
// in that lane), or at the end of the lane when before < 0. The other blocks
// keep their order; positions are renumbered 0..7 lane by lane.
func moveFX(p *podhd500.Preset, id, lane, before int) error {
	if id < firstFX || id > lastFX {
		return fmt.Errorf("block %d is not an effect (4..11)", id)
	}
	if lane < 0 || lane >= NumLanes {
		return fmt.Errorf("invalid lane %d (0..5)", lane)
	}
	if before == id {
		return nil
	}
	c := chainOf(p)
	for l := range c.Lanes { // take the block out
		bs := c.Lanes[l].Blocks[:0:0]
		for _, b := range c.Lanes[l].Blocks {
			if b != id {
				bs = append(bs, b)
			}
		}
		c.Lanes[l].Blocks = bs
	}
	target := c.Lanes[lane].Blocks
	at := len(target)
	if before >= 0 {
		at = -1
		for i, b := range target {
			if b == before {
				at = i
			}
		}
		if at < 0 {
			return fmt.Errorf("block %d is not in lane %s", before, LaneNames[lane])
		}
	}
	target = append(target[:at:at], append([]int{id}, target[at:]...)...)
	c.Lanes[lane].Blocks = target
	pos := 0
	for l := range c.Lanes {
		for _, b := range c.Lanes[l].Blocks {
			o := podhd500.ItemOffset(b)
			binary.LittleEndian.PutUint16(p.Body[o+4:], uint16(pos))
			p.Body[o+6] = byte(l)
			pos++
		}
	}
	return nil
}

// MoveBlock moves an FX block in the chain of the loaded preset: into lane
// (LanePre..LanePost), before block `before`, or at the end if before < 0.
// Like the front panel's MOVE, the whole edit buffer is rewritten
// (PRESET_SET to the buffer); the LCD shows "SYNC IN PROGRESS" once.
func (s *Session) MoveBlock(id, lane, before int) error {
	if s.client == nil {
		return ErrOffline
	}
	if err := s.loadBuffer(); err != nil {
		return err
	}
	s.bufMu.Lock()
	p, err := podhd500.ParsePreset(s.buf)
	s.bufMu.Unlock()
	if err != nil {
		return err
	}
	old := p.Body
	if err := moveFX(p, id, lane, before); err != nil {
		return err
	}
	if p.Body == old {
		return nil // already there
	}
	p.ClearDSPOver() // never send back a block the pedal couldn't run
	frame, err := usb.BuildPresetSet(usb.CurrentPresetAll, usb.CurrentSetAll, p.Body[:])
	if err != nil {
		return err
	}
	ack, err := s.request(frame, usb.CmdPresetSetAck, 5*time.Second)
	if err != nil {
		return err
	}
	if ack.Word(0) != 0 {
		return errors.New("the pedal rejected the new block order")
	}
	s.update(func(st *State) { st.Dirty = true })
	s.setBuffer(p.Body[:])
	return nil
}

// Amp moves, as the front panel does them (learned by recording the edit
// buffer while moving the amp on the pedal):
//   - only amp A moves (amp B never does), even when it is Amp Disabled;
//   - "pre" (5) and "post" (7) hold amp A alone: amp B is cleared (model
//     xxxxFFFF, off, not in use) and cab B is set off / not in use;
//   - back to the paths (0) only the position changes: amp B is there again
//     as Amp Disabled.
//
// The cab follows its amp: position code+1, or 3 on the paths.
const ampBSelByte = 0xFF6

var ampPosCode = map[string]uint16{"pre": ampPosPre, "a": ampPosPathA, "post": ampPosPost}

// setAmpPos moves amp A (with its cab) to pos: "pre", "a" (the paths) or
// "post". Reports whether anything changed.
func setAmpPos(p *podhd500.Preset, pos string) (bool, error) {
	code, ok := ampPosCode[pos]
	if !ok {
		return false, fmt.Errorf("invalid amp position %q (pre, a, post)", pos)
	}
	le := binary.LittleEndian
	oa, oc := podhd500.ItemOffset(0), podhd500.ItemOffset(1)
	cab := uint16(3)
	if code != ampPosPathA {
		cab = code + 1
	}
	changed := le.Uint16(p.Body[oa+4:]) != code || le.Uint16(p.Body[oc+4:]) != cab
	le.PutUint16(p.Body[oa+4:], code)
	le.PutUint16(p.Body[oc+4:], cab)
	if code != ampPosPathA {
		ob, ocb := podhd500.ItemOffset(2), podhd500.ItemOffset(3)
		if le.Uint16(p.Body[ob:]) != 0xFFFF || p.Body[ob+8] != 0 || p.Body[ob+11] != 0 || p.Body[ocb+8] != 0 || p.Body[ocb+11] != 0 {
			changed = true
		}
		le.PutUint16(p.Body[ob:], 0xFFFF) // low word of the model type: no model
		p.Body[ob+8], p.Body[ob+11] = 0, 0
		p.Body[ocb+8], p.Body[ocb+11] = 0, 0
		// 0xFF6 goes 1 → 0 too when the pedal clears amp B (it looks like
		// "amp B is the selected amp"; it is 0 whenever amp B is gone).
		if p.Body[ampBSelByte] != 0 {
			p.Body[ampBSelByte] = 0
			changed = true
		}
	}
	return changed, nil
}

// MoveAmp moves amp A (and its cab) of the loaded preset to "pre" (end of
// the pre section), "a" (the paths: amp A on path A, amp B slot on path B)
// or "post" (start of the post section). The whole edit buffer is rewritten,
// like MoveBlock.
func (s *Session) MoveAmp(pos string) error {
	if s.client == nil {
		return ErrOffline
	}
	if err := s.loadBuffer(); err != nil {
		return err
	}
	s.bufMu.Lock()
	p, err := podhd500.ParsePreset(s.buf)
	s.bufMu.Unlock()
	if err != nil {
		return err
	}
	changed, err := setAmpPos(p, pos)
	if err != nil || !changed {
		return err
	}
	p.ClearDSPOver() // never send back a block the pedal couldn't run
	frame, err := usb.BuildPresetSet(usb.CurrentPresetAll, usb.CurrentSetAll, p.Body[:])
	if err != nil {
		return err
	}
	ack, err := s.request(frame, usb.CmdPresetSetAck, 5*time.Second)
	if err != nil {
		return err
	}
	if ack.Word(0) != 0 {
		return errors.New("the pedal rejected the amp move")
	}
	s.update(func(st *State) { st.Dirty = true })
	s.setBuffer(p.Body[:])
	return nil
}
