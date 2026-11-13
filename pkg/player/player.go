// Package player is the live side of the snapshots: it applies a snapshot when
// a trigger asks for one (HTTP today, MIDI from the pedal's footswitches
// next) and applies snapshot 1 of every preset that gets loaded. It never
// saves or edits snapshots (that is the editor's job) and it reaches the pedal
// only through Engine.ApplySnapshot, which uses the editor's own writers.
package player

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"podhd500-edit/pkg/editor"
	"podhd500-edit/pkg/midiin"
)

// Engine is what the player needs from the editor session.
type Engine interface {
	ApplySnapshot(n int) (editor.SnapshotResult, error)
	State() editor.State
	Subscribe() (<-chan editor.State, func())
}

// Status is what GET /status reports.
type Status struct {
	Connected bool   `json:"connected"`
	Preset    string `json:"preset"`
	Slot      string `json:"slot"`
	Active    int    `json:"active"` // last snapshot applied to this preset (1..4), 0 = none
	LastError string `json:"lastError,omitempty"`
}

// Player applies snapshots.
type Player struct {
	eng    Engine
	settle time.Duration // quiet time after a preset change before snapshot 1 is applied
	logf   func(string, ...any)

	queue chan int // snapshot numbers asked by MIDI, applied in order by Run's worker

	mu        sync.Mutex // one apply at a time (triggers and auto-apply)
	active    int
	lastError string
}

// New returns a player. settle is how long the state must stay quiet after a
// preset change before snapshot 1 is applied (the pedal pushes the new preset
// in several steps); 0 uses 300 ms.
func New(eng Engine, settle time.Duration) *Player {
	if settle <= 0 {
		settle = 300 * time.Millisecond
	}
	return &Player{eng: eng, settle: settle, logf: log.Printf, queue: make(chan int, 8)}
}

// Trigger applies snapshot n (1..4).
func (p *Player) Trigger(n int) (editor.SnapshotResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	res, err := p.eng.ApplySnapshot(n)
	if err != nil {
		p.lastError = err.Error()
		return res, err
	}
	p.active, p.lastError = n, ""
	p.logf("snapshot %d: %d change(s)%s", n, res.Changes, skippedNote(res.Skipped))
	return res, nil
}

func skippedNote(skipped []int) string {
	if len(skipped) == 0 {
		return ""
	}
	return ", skipped blocks (model changed): " + intsString(skipped)
}

func intsString(v []int) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ",")
}

// HandleMIDI is the MIDI input callback: a control change with value > 0 on
// numbers cc..cc+3 (channel ch, 0 = any) asks for snapshot 1..4. It returns at
// once (CoreMIDI calls it on its own thread); the pedal is written by a worker.
// If presses pile up faster than the pedal takes them, the oldest are dropped.
func (p *Player) HandleMIDI(m midiin.Message, cc, ch int) {
	p.logf("MIDI in: %s", m)
	n := midiin.SnapshotFor(m, cc, ch)
	if n == 0 {
		return
	}
	for {
		select {
		case p.queue <- n:
			return
		default:
			select {
			case <-p.queue: // full: drop the oldest
			default:
			}
		}
	}
}

func (p *Player) worker(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case n := <-p.queue:
			if _, err := p.Trigger(n); err != nil {
				p.logf("snapshot %d from MIDI failed: %v", n, err)
			}
		}
	}
}

// Status reports the player's state.
func (p *Player) Status() Status {
	st := p.eng.State()
	p.mu.Lock()
	defer p.mu.Unlock()
	return Status{Connected: st.Connected, Preset: st.PresetName, Slot: st.SlotLabel, Active: p.active, LastError: p.lastError}
}

type presetKey struct {
	connected bool
	setlist   int
	slot      int
	name      string
}

func keyOf(st editor.State) presetKey {
	return presetKey{st.Connected, st.Setlist, st.Slot, st.PresetName}
}

// Run watches the pedal's state until stop is closed: when another preset is
// loaded (or the pedal connects), it waits for the state to settle and applies
// snapshot 1 if that preset has one. A preset without snapshots, or whose file
// belongs to another preset, is left as it is.
func (p *Player) Run(stop <-chan struct{}) {
	go p.worker(stop)
	ch, unsub := p.eng.Subscribe()
	defer unsub()
	var last presetKey
	var timer *time.Timer
	var fire <-chan time.Time
	arm := func() {
		if timer == nil {
			timer = time.NewTimer(p.settle)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(p.settle)
		}
		fire = timer.C
	}
	for {
		select {
		case <-stop:
			return
		case st := <-ch:
			k := keyOf(st)
			if k != last {
				last = k
				p.mu.Lock()
				p.active = 0
				p.mu.Unlock()
				if k.connected {
					arm()
				} else {
					fire = nil
				}
			} else if fire != nil && st.Connected {
				arm() // still changing (preset loading): wait for quiet
			}
		case <-fire:
			fire = nil
			p.applyFirst()
		}
	}
}

func (p *Player) applyFirst() {
	if _, err := p.Trigger(1); err != nil {
		if errors.Is(err, editor.ErrSnapshotEmpty) || errors.Is(err, editor.ErrSnapshotMismatch) {
			p.mu.Lock()
			p.lastError = ""
			p.mu.Unlock()
			return // no snapshots for this preset: leave it as stored
		}
		p.logf("auto-apply of snapshot 1 failed: %v", err)
	}
}
