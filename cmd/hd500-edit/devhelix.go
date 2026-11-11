package main

// Development only (HD500_DEBUG=1): shows Helix Native, running in REAPER, in
// the browser as the visual reference for the UI. The window image comes from
// screencapture; parameters go through tools/reaper/hd500_bridge.lua (files in
// REAPER's resource folder). No synthetic mouse or keyboard input is used.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type devWindow struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// reaperWindows lists REAPER's on-screen windows (CoreGraphics via JXA).
func reaperWindows() ([]devWindow, error) {
	const js = `ObjC.import("CoreGraphics");
const l = ObjC.castRefToObject($.CGWindowListCopyWindowInfo(1|16, 0)), out = [];
for (let i = 0; i < l.count; i++) {
  const w = ObjC.deepUnwrap(l.objectAtIndex(i));
  if (/REAPER/i.test(w.kCGWindowOwnerName || "") && w.kCGWindowLayer === 0)
    out.push({id: w.kCGWindowNumber, name: w.kCGWindowName || "", width: w.kCGWindowBounds.Width, height: w.kCGWindowBounds.Height});
}
JSON.stringify(out)`
	b, err := exec.Command("osascript", "-l", "JavaScript", "-e", js).Output()
	if err != nil {
		return nil, err
	}
	var ws []devWindow
	return ws, json.Unmarshal(b, &ws)
}

// helixWindow picks the window to show: ?id=, else the one named Helix Native,
// else the largest REAPER window that isn't the main one.
func helixWindow(r *http.Request) (int, error) {
	if id, err := strconv.Atoi(r.URL.Query().Get("id")); err == nil {
		return id, nil
	}
	ws, err := reaperWindows()
	if err != nil {
		return 0, err
	}
	best, area := 0, 0.0
	for _, w := range ws {
		if strings.Contains(strings.ToLower(w.Name), "helix") {
			return w.ID, nil
		}
		if !strings.Contains(w.Name, "- REAPER") && w.Width*w.Height > area {
			best, area = w.ID, w.Width*w.Height
		}
	}
	if best == 0 {
		return 0, fmt.Errorf("Helix Native window not open in REAPER")
	}
	return best, nil
}

func registerDevHelix(mux *http.ServeMux) {
	home, _ := os.UserHomeDir()
	bridge := filepath.Join(home, "Library", "Application Support", "REAPER", "hd500-bridge")
	var shotMu sync.Mutex

	mux.HandleFunc("/dev/helix/windows", func(w http.ResponseWriter, r *http.Request) {
		ws, err := reaperWindows()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, ws)
	})
	mux.HandleFunc("/dev/helix/shot.png", func(w http.ResponseWriter, r *http.Request) {
		id, err := helixWindow(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		shotMu.Lock()
		defer shotMu.Unlock()
		f := filepath.Join(os.TempDir(), "hd500-helix-shot.png")
		if err := exec.Command("screencapture", "-x", "-o", "-l"+strconv.Itoa(id), f).Run(); err != nil {
			http.Error(w, "screencapture: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, f)
	})
	mux.HandleFunc("/dev/helix/params", func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join(bridge, "state.json"))
		if err != nil {
			http.Error(w, "bridge not running in REAPER (tools/reaper/hd500_bridge.lua)", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	})
	// POST {"op":"set|show|hide","track":0,"fx":0,"param":3,"value":0.5}
	mux.HandleFunc("/dev/helix/cmd", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var c struct {
			Op    string  `json:"op"`
			Track int     `json:"track"`
			FX    int     `json:"fx"`
			Param int     `json:"param"`
			Value float64 `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var line string
		switch c.Op {
		case "set":
			if c.Value < 0 || c.Value > 1 {
				http.Error(w, "value must be 0..1", http.StatusBadRequest)
				return
			}
			line = fmt.Sprintf("set %d %d %d %.6f\n", c.Track, c.FX, c.Param, c.Value)
		case "show", "hide":
			line = fmt.Sprintf("%s %d %d\n", c.Op, c.Track, c.FX)
		default:
			http.Error(w, "op must be set, show or hide", http.StatusBadRequest)
			return
		}
		f, err := os.OpenFile(filepath.Join(bridge, "cmd.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		if _, err := f.WriteString(line); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
