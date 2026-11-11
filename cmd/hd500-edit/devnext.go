package main

import (
	"net/http"
	"os"
	"path/filepath"
)

// registerDevNext serves the new UI (/next/, built by frontend/ with Vite)
// from disk instead of the copy embedded in the binary, so `npm run dev` in
// frontend/ (vite build --watch) shows changes on reload without rebuilding
// the Go binary. Development only (HD500_DEBUG=1), when run from the repo.
func registerDevNext(mux *http.ServeMux) {
	dir := filepath.Join("cmd", "hd500-edit", "web", "next")
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return
	}
	files := http.StripPrefix("/next/", http.FileServer(http.Dir(dir)))
	mux.Handle("/next/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
}
