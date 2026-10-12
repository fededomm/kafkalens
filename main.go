package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:web/dist/web/browser
var dist embed.FS

func defaultConfig() string {
	home, _ := os.UserConfigDir()
	return filepath.Join(home, "kafkalens", "clusters.json")
}

// newHandler serves the API and the embedded UI; shared by the server and desktop builds.
func newHandler(store *Store) http.Handler {
	mux := http.NewServeMux()
	(&API{store: store, pool: NewPool()}).Routes(mux)

	ui, _ := fs.Sub(dist, "web/dist/web/browser")
	files := http.FileServerFS(ui)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: unknown non-asset paths serve index.html
		if _, err := fs.Stat(ui, strings.TrimPrefix(r.URL.Path, "/")); err != nil && !strings.HasPrefix(r.URL.Path, "/api/") {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
	return harden(mux)
}

// harden adds browser security headers and caps request bodies.
func harden(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")
		// inline styles: Angular injects component <style> tags
		hd.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; "+
			"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		h.ServeHTTP(w, r)
	})
}
