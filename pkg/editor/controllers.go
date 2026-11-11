package editor

import (
	"errors"
	"fmt"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

// rewriteBuffer applies f to the edit buffer and, if it changed anything,
// sends the whole buffer to the pedal (PRESET_SET), like the front panel's
// own edits that rewrite the preset. The LCD shows "SYNC IN PROGRESS" once.
func (s *Session) rewriteBuffer(what string, f func(p *podhd500.Preset) (bool, error)) error {
	if s.cli() == nil {
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
	changed, err := f(p)
	if err != nil {
		return err
	}
	// blocks the pedal couldn't run go out as clean empty slots (a TYPE
	// None from the host leaves them as 0xFFFFFFFF and garbles the LCD)
	if p.ClearDSPOver() {
		changed = true
	}
	if !changed {
		return nil
	}
	frame, err := usb.BuildPresetSet(usb.CurrentPresetAll, usb.CurrentSetAll, p.Body[:])
	if err != nil {
		return err
	}
	ack, err := s.request(frame, usb.CmdPresetSetAck, 5*time.Second)
	if err != nil {
		return err
	}
	if ack.Word(0) != 0 {
		return fmt.Errorf("the pedal rejected the %s", what)
	}
	s.update(func(st *State) { st.Dirty = true })
	s.setBuffer(p.Body[:])
	return nil
}

func checkBlock(id int) error {
	if id < 0 || id > 11 {
		return fmt.Errorf("invalid block %d (0..11)", id)
	}
	if id == 1 || id == 3 {
		return errors.New("a cab has no bypass or controllers of its own")
	}
	return nil
}

// SetBypassAssign sets the footswitch that turns block id on/off:
// 0 none, 1–8 FS1–FS8, 9 the EXP pedal's toe switch.
func (s *Session) SetBypassAssign(id int, fs uint32) error {
	if err := checkBlock(id); err != nil {
		return err
	}
	if !podhd500.ValidAssign(podhd500.BypassOptions, fs) {
		return fmt.Errorf("invalid footswitch %d", fs)
	}
	return s.rewriteBuffer("footswitch assignment", func(p *podhd500.Preset) (bool, error) {
		if p.Item(id).Empty {
			return false, errors.New("the block is empty")
		}
		return p.SetItemFS(id, uint8(fs)), nil
	})
}

// SetController sets the controller of parameter k (index in the block's
// parameter list) of block id: 0 none, 1 EXP 1, 2 EXP 2, 3 Variax Vol,
// 4 Variax Tone. A newly assigned parameter starts with Minimum = Maximum =
// its current value (as the original editor showed it), unless it already
// has a range.
func (s *Session) SetController(id, k int, ctl uint32) error {
	if err := checkBlock(id); err != nil {
		return err
	}
	if !podhd500.ValidAssign(podhd500.ControllerOptions, ctl) {
		return fmt.Errorf("invalid controller %d", ctl)
	}
	return s.rewriteBuffer("controller assignment", func(p *podhd500.Preset) (bool, error) {
		it := p.Item(id)
		if it.Empty || k < 0 || k >= len(it.Params) {
			return false, fmt.Errorf("block %d has no parameter %d", id, k)
		}
		return p.SetParamController(id, k, uint8(ctl)), nil
	})
}

// SetParamRange sets the Minimum (heel) or Maximum (toe) of parameter k of
// block id, 0..1 normalized like the value. Sent as the pedal reports its
// own heel/toe edits (Cmd 0x2E / 0x2F), no buffer rewrite.
func (s *Session) SetParamRange(id, k int, toe bool, v float32) error {
	if s.cli() == nil {
		return ErrOffline
	}
	if err := checkBlock(id); err != nil {
		return err
	}
	if v != v || v < 0 || v > 1 {
		return fmt.Errorf("value %v outside 0..1", v)
	}
	if s.BufferCopy() == nil {
		if err := s.loadBuffer(); err != nil {
			return err
		}
	}
	s.bufMu.Lock()
	p, err := podhd500.ParsePreset(s.buf)
	s.bufMu.Unlock()
	if err != nil {
		return err
	}
	it := p.Item(id)
	if it.Empty || k < 0 || k >= len(it.Params) {
		return fmt.Errorf("block %d has no parameter %d", id, k)
	}
	pid := it.Params[k].ID
	frame := usb.BuildParamMin(uint32(id), pid, v)
	if toe {
		frame = usb.BuildParamMax(uint32(id), pid, v)
	}
	if err := s.send(frame); err != nil {
		return err
	}
	s.editBuffer(func(p *podhd500.Preset) bool { return p.SetParamRange(id, k, toe, v) })
	s.update(func(st *State) { st.Dirty = true })
	return nil
}

// ClearAssignments removes bypass/controller assignments in one buffer
// rewrite: all of the preset's (all), or one: block id's bypass footswitch
// (k < 0) or the controller of its parameter k.
func (s *Session) ClearAssignments(all bool, id, k int) error {
	if !all {
		if err := checkBlock(id); err != nil {
			return err
		}
	}
	return s.rewriteBuffer("cleared assignments", func(p *podhd500.Preset) (bool, error) {
		changed := false
		clear := func(id, k int) {
			if k < 0 {
				changed = p.SetItemFS(id, 0) || changed
			} else {
				changed = p.SetParamController(id, k, 0) || changed
			}
		}
		if !all {
			clear(id, k)
			return changed, nil
		}
		for id := 0; id < 12; id++ {
			if id == 1 || id == 3 {
				continue
			}
			clear(id, -1)
			for k := range p.Item(id).Params {
				clear(id, k)
			}
		}
		return changed, nil
	})
}
