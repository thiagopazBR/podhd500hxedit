package player

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"podhd500-edit/pkg/editor"
	"podhd500-edit/pkg/midiin"
)

type fakeEngine struct {
	mu      sync.Mutex
	st      editor.State
	ch      chan editor.State
	applied []int
	err     error
}

func newFake() *fakeEngine {
	return &fakeEngine{ch: make(chan editor.State, 16), st: editor.State{Connected: true, Slot: 1, PresetName: "A"}}
}
func (f *fakeEngine) ApplySnapshot(n int) (editor.SnapshotResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, n)
	if f.err != nil {
		return editor.SnapshotResult{}, f.err
	}
	return editor.SnapshotResult{Changes: 3}, nil
}
func (f *fakeEngine) State() editor.State                      { return f.st }
func (f *fakeEngine) Subscribe() (<-chan editor.State, func()) { return f.ch, func() {} }
func (f *fakeEngine) calls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.applied...)
}

func quiet(p *Player) *Player { p.logf = func(string, ...any) {}; return p }

func TestTriggerTracksActive(t *testing.T) {
	f := newFake()
	p := quiet(New(f, 0))
	if _, err := p.Trigger(3); err != nil {
		t.Fatal(err)
	}
	if st := p.Status(); st.Active != 3 || st.Preset != "A" {
		t.Fatalf("status = %+v", st)
	}
	f.err = errors.New("boom")
	if _, err := p.Trigger(2); err == nil {
		t.Fatal("error swallowed")
	}
	if st := p.Status(); st.Active != 3 || st.LastError != "boom" {
		t.Fatalf("active must stay on failure: %+v", st)
	}
}

func waitCalls(f *fakeEngine, want int) []int {
	for i := 0; i < 100; i++ {
		if c := f.calls(); len(c) >= want {
			return c
		}
		time.Sleep(10 * time.Millisecond)
	}
	return f.calls()
}

func TestRunAppliesSnapshot1AfterPresetChange(t *testing.T) {
	f := newFake()
	p := quiet(New(f, 40*time.Millisecond))
	stop := make(chan struct{})
	defer close(stop)
	go p.Run(stop)
	f.ch <- f.st // first state: connected -> snapshot 1
	if c := waitCalls(f, 1); len(c) != 1 || c[0] != 1 {
		t.Fatalf("calls = %v, want [1]", c)
	}
	p.Trigger(4)
	// another preset: active resets and snapshot 1 is applied again
	st := f.st
	st.Slot, st.PresetName = 2, "B"
	f.ch <- st
	c := waitCalls(f, 3)
	if len(c) != 3 || c[2] != 1 {
		t.Fatalf("calls = %v, want [1 4 1]", c)
	}
}

func TestRunWaitsForQuietAndIgnoresSameKey(t *testing.T) {
	f := newFake()
	p := quiet(New(f, 80*time.Millisecond))
	stop := make(chan struct{})
	defer close(stop)
	go p.Run(stop)
	f.ch <- f.st
	for i := 0; i < 4; i++ { // the state keeps changing: not settled yet
		time.Sleep(30 * time.Millisecond)
		f.ch <- f.st
	}
	if c := f.calls(); len(c) != 0 {
		t.Fatalf("applied before the state settled: %v", c)
	}
	if c := waitCalls(f, 1); len(c) != 1 {
		t.Fatalf("calls = %v", c)
	}
	time.Sleep(150 * time.Millisecond)
	f.ch <- f.st // same preset again: no new auto-apply
	time.Sleep(150 * time.Millisecond)
	if c := f.calls(); len(c) != 1 {
		t.Fatalf("same preset re-applied: %v", c)
	}
}

func TestAutoApplyIgnoresEmptyAndMismatch(t *testing.T) {
	f := newFake()
	f.err = fmt.Errorf("%w (snapshot 1)", editor.ErrSnapshotEmpty)
	p := quiet(New(f, 20*time.Millisecond))
	stop := make(chan struct{})
	defer close(stop)
	go p.Run(stop)
	f.ch <- f.st
	waitCalls(f, 1)
	time.Sleep(40 * time.Millisecond)
	if st := p.Status(); st.LastError != "" || st.Active != 0 {
		t.Fatalf("a preset without snapshots must stay quiet: %+v", st)
	}
}

func TestDisconnectCancelsPendingApply(t *testing.T) {
	f := newFake()
	p := quiet(New(f, 60*time.Millisecond))
	stop := make(chan struct{})
	defer close(stop)
	go p.Run(stop)
	f.ch <- f.st
	off := f.st
	off.Connected = false
	f.ch <- off
	time.Sleep(150 * time.Millisecond)
	if c := f.calls(); len(c) != 0 {
		t.Fatalf("applied while offline: %v", c)
	}
}

func TestHTTPHandler(t *testing.T) {
	f := newFake()
	h := quiet(New(f, 0)).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/snapshot/2", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"changes":3`) {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/snapshot/2", 405},
		{http.MethodPost, "/snapshot/x", 400},
		{http.MethodPost, "/status", 405},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.want {
			t.Fatalf("%s %s = %d, want %d", c.method, c.path, rec.Code, c.want)
		}
	}
	f.err = editor.ErrOffline
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/snapshot/1", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("offline = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"active":2`) {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8600": true, "localhost:8600": true, "[::1]:8600": true,
		"0.0.0.0:8600": false, ":8600": false, "192.168.1.5:8600": false, "bad": false,
	} {
		if got := Loopback(addr); got != want {
			t.Errorf("Loopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestHandleMIDIAppliesSnapshotsInOrder(t *testing.T) {
	f := newFake()
	p := quiet(New(f, time.Hour)) // no auto-apply during the test
	stop := make(chan struct{})
	defer close(stop)
	go p.Run(stop)
	for _, cc := range []int{81, 80, 83} {
		p.HandleMIDI(midiin.Message{Kind: midiin.Control, Channel: 1, Data1: cc, Data2: 127}, 80, 0)
	}
	p.HandleMIDI(midiin.Message{Kind: midiin.Control, Channel: 1, Data1: 82, Data2: 0}, 80, 0)   // release: ignored
	p.HandleMIDI(midiin.Message{Kind: midiin.Program, Channel: 1, Data1: 80}, 80, 0)             // not a CC
	p.HandleMIDI(midiin.Message{Kind: midiin.Control, Channel: 9, Data1: 80, Data2: 127}, 80, 1) // other channel
	c := waitCalls(f, 3)
	if len(c) != 3 || c[0] != 2 || c[1] != 1 || c[2] != 4 {
		t.Fatalf("calls = %v, want [2 1 4]", c)
	}
	time.Sleep(50 * time.Millisecond)
	if c := f.calls(); len(c) != 3 {
		t.Fatalf("ignored messages triggered snapshots: %v", c)
	}
}
