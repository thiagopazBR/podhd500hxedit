//go:build darwin

// hd500-edit is the POD HD500 editor: a local web server (127.0.0.1 only)
// that talks to the pedal over USB and serves the UI embedded in the binary.
// The UI is plain HTML/CSS/JS so it can later be wrapped in a native shell
// (e.g. Wails) without changes.
//
// Requires the control interface to be free: 'sudo ./bin/l6-rebind release'.
// Without the pedal it starts in offline mode.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"podhd500-edit/pkg/editor"
	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

//go:embed web
var webFS embed.FS

func main() {
	// HD500_USB_DEBUG=1: also log every raw USB packet the pedal sends (reverse engineering)
	usb.Debug = os.Getenv("HD500_USB_DEBUG") == "1"
	addr := flag.String("addr", "127.0.0.1:8500", "Listen address (keep it on localhost)")
	// The browser is NOT opened by default (it opened the default browser —
	// Safari — on every restart). -open opens the page in the given app,
	// e.g. -open "Google Chrome for Testing".
	openApp := flag.String("open", "Google Chrome for Testing", "browser app that opens the editor page at start (never the system default); -open= to open none")
	flag.Parse()

	var sess *editor.Session
	var connecting atomic.Bool
	// connectLoop keeps trying until the pedal opens: it may be powered on or plugged in after
	// the editor started (or came back after a power cycle); the l6-rebind daemon frees its
	// interface by itself.
	connectLoop := func() {
		if !connecting.CompareAndSwap(false, true) {
			return
		}
		defer connecting.Store(false)
		for {
			time.Sleep(2 * time.Second)
			c, cerr := usb.OpenDirectClient(usb.DefaultDirectOptions)
			if cerr != nil {
				continue
			}
			log.Printf("POD HD500 conectada")
			sess.Attach(c)
			if rerr := sess.Refresh(); rerr != nil {
				log.Printf("leitura inicial do pedal falhou: %v", rerr)
			}
			return
		}
	}
	dc, err := usb.OpenDirectClient(usb.DefaultDirectOptions)
	if err != nil {
		log.Printf("modo offline: %v (se for 0xe00002c5: sudo ./bin/l6-rebind release, ou instale o daemon uma vez: sudo ./bin/l6-rebind install)", err)
		sess = editor.NewSession(nil, err)
		sess.SetOnLost(func() { connectLoop() })
		go connectLoop()
	} else {
		defer dc.Close()
		sess = editor.NewSession(dc, nil)
		sess.SetOnLost(func() { log.Printf("POD HD500 desconectada: tentando reconectar"); connectLoop() })
		// Read the pedal in the background: the UI is served at once (it
		// follows the state over SSE) even if the first read is slow.
		go func() {
			if err := sess.Refresh(); err != nil {
				log.Printf("leitura inicial do pedal falhou: %v", err)
			}
		}()
	}

	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	// The UI is embedded in the binary and changes with every build: make the
	// browser revalidate it instead of reusing a stale style.css / app.js.
	files := http.FileServer(http.FS(static))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
	// server: changes on every start, so an open page can tell that the
	// server was restarted (possibly with a new UI) and reload itself.
	serverID := strconv.FormatInt(time.Now().UnixNano(), 36)
	mux.HandleFunc("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"server":        serverID,
			"syncNotes":     podhd500.SyncNotes,     // tempo sync note names by code (2..)
			"syncBeats":     podhd500.SyncNoteBeats, // their length in quarter notes
			"input1":        podhd500.Input1Options,
			"input2":        podhd500.Input2Options,
			"inz":           podhd500.InZOptions,
			"inputsSetup":   podhd500.InputsSetupOptions,
			"tempoSync":     editor.TempoSyncOptions,
			"bypassAssign":  podhd500.BypassOptions,
			"midiMessages":  podhd500.MIDIMessageOptions,
			"midiChannels":  podhd500.MIDIChannelOptions,
			"controllers":   podhd500.ControllerOptions,
			"tempo":         map[string]float32{"min": editor.MinTempo, "max": editor.MaxTempo},
			"mics":          podhd500.MicNames,
			"variaxControl": editor.VariaxControlOptions,
			"variaxModels":  editor.VariaxModels(), // by VDI Input type; index = value
			"variaxToneMax": editor.VariaxToneMax,
			"setupInput":    map[string]uint32{"input1": podhd500.SetupInput1Source, "input2": podhd500.SetupInput2Source, "inz": podhd500.SetupGuitarInZ},
		})
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, sess.State()) })
	mux.HandleFunc("/api/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		refresh := sess.Refresh
		if r.URL.Query().Get("globals") == "1" {
			refresh = sess.RefreshGlobals
		}
		if err := refresh(); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/input", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID    uint32 `json:"id"`
			Value uint32 `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetInput(req.ID, req.Value); err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/setlists", func(w http.ResponseWriter, r *http.Request) {
		l, err := sess.Setlists()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, l)
	})
	mux.HandleFunc("/api/setlist", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.URL.Query().Get("n"))
		if err != nil {
			http.Error(w, "parameter n (0..7) is required", http.StatusBadRequest)
			return
		}
		v, err := sess.Setlist(n)
		if err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, v)
	})
	transfer := func(do func(editor.TransferRequest) (editor.TransferResult, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "POST", http.StatusMethodNotAllowed)
				return
			}
			var req editor.TransferRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			res, err := do(req)
			if err != nil {
				code := http.StatusBadRequest
				switch err {
				case editor.ErrOffline:
					code = http.StatusServiceUnavailable
				case editor.ErrBusy:
					code = http.StatusConflict
				}
				http.Error(w, err.Error(), code)
				return
			}
			writeJSON(w, res)
		}
	}
	mux.HandleFunc("/api/send", transfer(sess.Send))
	mux.HandleFunc("/api/receive", transfer(sess.Receive))
	mux.HandleFunc("/api/arrange", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req editor.ArrangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.Arrange(req); err != nil {
			code := http.StatusBadRequest
			switch err {
			case editor.ErrOffline:
				code = http.StatusServiceUnavailable
			case editor.ErrLoadedDirty:
				code = http.StatusConflict
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/rename", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Setlist int    `json:"setlist"`
			Slot    int    `json:"slot"`
			Name    string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.Rename(req.Setlist, req.Slot, req.Name); err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, sess.State())
	})
	if os.Getenv("HD500_DEBUG") == "1" {
		registerDevHelix(mux) // Helix Native in REAPER as the UI reference (devhelix.go)
		registerDevNext(mux)  // the new UI from disk (devnext.go)
		// raw edit buffer (4096 bytes, pedal byte order), for reverse engineering
		mux.HandleFunc("/api/debug/buffer", func(w http.ResponseWriter, r *http.Request) {
			b := sess.BufferCopy()
			if b == nil {
				http.Error(w, "no edit buffer yet", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(b)
		})
		// raw MIDI Assign write: {control, bytes:[6]}
		mux.HandleFunc("/api/debug/midi-assign", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Control int     `json:"control"`
				Bytes   [6]byte `json:"bytes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := sess.SendMIDIAssignRaw(req.Control, req.Bytes); err != nil {
				httpError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		// one pedal-wide STATUS id: ?id=11 → {"id":11,"value":1}
		mux.HandleFunc("/api/debug/status", func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.ParseUint(r.URL.Query().Get("id"), 0, 32)
			v, ok := sess.ReadStatus(uint32(id))
			if !ok {
				http.Error(w, "no answer", http.StatusServiceUnavailable)
				return
			}
			writeJSON(w, map[string]uint32{"id": uint32(id), "value": v})
		})
	}
	mux.HandleFunc("/api/export", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		set, _ := strconv.Atoi(q.Get("setlist"))
		slot, _ := strconv.Atoi(q.Get("slot"))
		data, name, err := sess.Export(editor.ExportRequest{Kind: podhd500.FileKind(q.Get("kind")), Setlist: set, Slot: slot})
		if err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/api/backup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		res, err := sess.Backup()
		if err != nil {
			code := http.StatusBadRequest
			switch err {
			case editor.ErrOffline:
				code = http.StatusServiceUnavailable
			case editor.ErrBusy:
				code = http.StatusConflict
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/api/reveal", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dir := editor.BackupDir()
		p := filepath.Clean(req.Path)
		if req.Path == "" {
			p = dir
		}
		if p != dir && !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			http.Error(w, "outside the backups folder", http.StatusBadRequest)
			return
		}
		_ = os.MkdirAll(dir, 0o755)
		args := []string{p}
		if p != dir {
			args = []string{"-R", p}
		}
		if err := exec.Command("open", args...).Run(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		set, _ := strconv.Atoi(r.FormValue("setlist"))
		slot, _ := strconv.Atoi(r.FormValue("slot"))
		var files [][]byte
		for _, fh := range r.MultipartForm.File["file"] {
			f, err := fh.Open()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			b, err := io.ReadAll(io.LimitReader(f, podhd500.H5BSize+1))
			f.Close()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			files = append(files, b)
		}
		res, err := sess.Import(files, editor.SlotPos{Setlist: set, Slot: slot})
		if err != nil {
			code := http.StatusBadRequest
			switch err {
			case editor.ErrOffline:
				code = http.StatusServiceUnavailable
			case editor.ErrLoadedDirty:
				code = http.StatusConflict
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/api/setlist-name", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Setlist int    `json:"setlist"`
			Name    string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.RenameSetlist(req.Setlist, req.Name); err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		l, _ := sess.Setlists()
		writeJSON(w, l)
	})
	mux.HandleFunc("/api/preset", func(w http.ResponseWriter, r *http.Request) {
		v, err := sess.Preset()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, v)
	})
	mux.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, editor.Models()) })
	mux.HandleFunc("/api/block/model", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID   int    `json:"id"`
			Type uint32 `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetBlockModel(req.ID, req.Type); err != nil {
			httpError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/cab", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Cab   int     `json:"cab"`
			Param string  `json:"param"`
			Value float32 `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetCab(req.Cab, req.Param, req.Value); err != nil {
			httpError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/mixer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Param string  `json:"param"`
			Value float32 `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetMixer(req.Param, req.Value); err != nil {
			httpError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/chain/move", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Block  int    `json:"block"`
			Lane   string `json:"lane"`
			Before *int   `json:"before"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		lane := -1
		for i, n := range editor.LaneNames {
			if n == req.Lane {
				lane = i
			}
		}
		if lane < 0 {
			http.Error(w, "invalid lane: use pre, aPre, bPre, aPost, bPost or post", http.StatusBadRequest)
			return
		}
		before := -1
		if req.Before != nil {
			before = *req.Before
		}
		if err := sess.MoveBlock(req.Block, lane, before); err != nil {
			httpError(w, err)
			return
		}
		v, err := sess.Preset()
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, v.Chain)
	})
	// Bypass / controller assignments (BYPASS/CONTROLLER ASSIGN page).
	mux.HandleFunc("/api/assign", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		// {id, fs}: the block's bypass footswitch.
		// {id, param, ctl}: parameter index param's controller.
		// {id, param, min} / {id, param, max}: its heel / toe value (0..1).
		var req struct {
			ID    int      `json:"id"`
			FS    *uint32  `json:"fs"`
			Param *int     `json:"param"`
			Ctl   *uint32  `json:"ctl"`
			Min   *float32 `json:"min"`
			Max   *float32 `json:"max"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var err error
		switch {
		case req.FS != nil:
			err = sess.SetBypassAssign(req.ID, *req.FS)
		case req.Param != nil && req.Ctl != nil:
			err = sess.SetController(req.ID, *req.Param, *req.Ctl)
		case req.Param != nil && req.Min != nil:
			err = sess.SetParamRange(req.ID, *req.Param, false, *req.Min)
		case req.Param != nil && req.Max != nil:
			err = sess.SetParamRange(req.ID, *req.Param, true, *req.Max)
		default:
			http.Error(w, "send fs, or param with ctl, min or max", http.StatusBadRequest)
			return
		}
		if err != nil {
			httpError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/assign/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		// {all: true} or {id, param} (param -1 = the block's bypass footswitch)
		var req struct {
			All   bool `json:"all"`
			ID    int  `json:"id"`
			Param int  `json:"param"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.ClearAssignments(req.All, req.ID, req.Param); err != nil {
			httpError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/amp/move", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Pos string `json:"pos"` // pre | a | post
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.MoveAmp(req.Pos); err != nil {
			httpError(w, err)
			return
		}
		v, err := sess.Preset()
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, v.Chain)
	})
	mux.HandleFunc("/api/variax", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Param string `json:"param"` // model | tone | control
			Value int    `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var err error
		if req.Param == "control" {
			err = sess.SetVariaxControl(uint32(req.Value))
		} else {
			err = sess.SetVariax(req.Param, req.Value)
		}
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/param", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID    int     `json:"id"`
			Param uint32  `json:"param"`
			Value float32 `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetParam(req.ID, req.Param, req.Value); err != nil {
			httpError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Tempo parameter follows a note value of the tap tempo (or free with 0):
	// {id: block, slot: 1|2 (the param's tempoSlot), sync: note code (meta.syncNotes)}.
	mux.HandleFunc("/api/param/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID   int `json:"id"`
			Slot int `json:"slot"`
			Sync int `json:"sync"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetTempoSync(req.ID, req.Slot, req.Sync); err != nil {
			httpError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/block", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     int  `json:"id"`
			Active bool `json:"active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetBlockActive(req.ID, req.Active); err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/select", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Setlist int `json:"setlist"`
			Slot    int `json:"slot"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.Select(req.Setlist, req.Slot); err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/tempo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			BPM float32 `json:"bpm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetTempo(req.BPM); err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/midi-assign", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		// {control, message?, channel?, number?, value?, max?}
		var req struct {
			Control int    `json:"control"`
			Message *uint8 `json:"message"`
			Channel *uint8 `json:"channel"`
			Number  *uint8 `json:"number"`
			Value   *uint8 `json:"value"`
			Max     *uint8 `json:"max"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetMIDIAssign(req.Control, editor.MIDIAssignChange{Message: req.Message, Channel: req.Channel, Number: req.Number, Value: req.Value, Max: req.Max}); err != nil {
			httpError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/midi-channel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Channel uint32 `json:"channel"` // 1..16
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetMIDIChannel(req.Channel); err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/tempo-sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Value uint32 `json:"value"` // 1 Global, 0 Preset
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetTempoSource(req.Value); err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/inputs-setup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Value uint32 `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sess.SetInputsSetup(req.Value); err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/tuner", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		// Any of: on (open/close the tuner), reference (Hz), mute.
		var req struct {
			On        *bool `json:"on"`
			Reference *int  `json:"reference"`
			Mute      *bool `json:"mute"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var err error
		if req.Reference != nil {
			err = sess.SetTunerReference(*req.Reference)
		}
		if err == nil && req.Mute != nil {
			err = sess.SetTunerMute(*req.Mute)
		}
		if err == nil && req.On != nil {
			err = sess.SetTuner(*req.On)
		}
		if err != nil {
			code := http.StatusBadRequest
			if err == editor.ErrOffline {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, sess.State())
	})
	mux.HandleFunc("/api/globals", func(w http.ResponseWriter, r *http.Request) {
		ids := make([]uint32, 0, 0x30)
		for id := uint32(0); id < 0x30; id++ {
			ids = append(ids, id)
		}
		g, err := sess.Globals(ids)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, g)
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) { serveEvents(w, r, sess) })

	appURL := "http://" + *addr + "/"
	fmt.Println("POD HD500 Edit em", appURL, "(Ctrl+C para sair)")
	if *openApp != "" {
		go func() { time.Sleep(300 * time.Millisecond); _ = exec.Command("open", "-a", *openApp, appURL).Run() }()
	}
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// serveEvents streams every state change as Server-Sent Events.
func serveEvents(w http.ResponseWriter, r *http.Request, sess *editor.Session) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, cancel := sess.Subscribe()
	defer cancel()
	emit := func(st editor.State) {
		b, _ := json.Marshal(st)
		fmt.Fprintf(w, "data: %s\n\n", b)
		fl.Flush()
	}
	emit(sess.State())
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case st := <-ch:
			emit(st)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// httpError maps session errors to status codes.
func httpError(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if err == editor.ErrOffline {
		code = http.StatusServiceUnavailable
	}
	http.Error(w, err.Error(), code)
}
