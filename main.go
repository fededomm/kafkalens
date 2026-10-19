package main

import (
	"cmp"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"kafkalens/internal/api"
	"kafkalens/internal/config"
	"kafkalens/internal/kafka"
	"kafkalens/internal/store"
)

//go:embed all:web/dist/web/browser
var dist embed.FS

//go:embed wails.json
var wailsJSON []byte

// version is info.productVersion from wails.json: the one place a release bumps it.
func version() string {
	var w struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	json.Unmarshal(wailsJSON, &w)
	return cmp.Or(w.Info.ProductVersion, "dev")
}

// defaultSettings is config.yaml: the browser mode's defaults and the desktop app's settings.
//
//go:embed config.yaml
var defaultSettings []byte

// clustersFile is the clusters file from the settings, or the default location.
func clustersFile(cfg config.Config) string {
	if cfg.Clusters != "" {
		return cfg.Clusters
	}
	return defaultConfig()
}

func defaultConfig() string {
	home, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("cannot get user config dir: %v", err)
	}

	return filepath.Join(home, "kafkalens", "clusters.json")
}

// newHandler serves the API and the embedded UI; shared by the server and desktop builds.
func newHandler(st *store.Store) http.Handler {
	r := chi.NewRouter()
	r.Use(harden)
	r.Mount("/api", api.New(st, kafka.NewPool(), version()))

	ui, _ := fs.Sub(dist, "web/dist/web/browser")
	files := http.FileServerFS(ui)
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: unknown non-asset paths serve index.html
		if _, err := fs.Stat(ui, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
	return r
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
