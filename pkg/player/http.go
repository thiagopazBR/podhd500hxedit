package player

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"podhd500-edit/pkg/editor"
)

// Handler serves the player's HTTP trigger: POST /snapshot/{1..4} applies a
// snapshot, GET /status reports the state. It is meant for testing the
// player without footswitches and for other tools on the same machine: serve
// it on a loopback address only (see Loopback).
func (p *Player) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/snapshot/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		n, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/snapshot/"))
		if err != nil {
			http.Error(w, "snapshot number 1..4 expected", http.StatusBadRequest)
			return
		}
		res, err := p.Trigger(n)
		if err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, editor.ErrOffline) {
				code = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, p.Status())
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// Loopback reports whether addr (host:port) listens only on this machine.
// The HTTP trigger has no authentication, so it must never be reachable from
// the network.
func Loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
