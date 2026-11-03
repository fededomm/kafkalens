package main

import (
	"cmp"
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"kafkalens/internal/api"
	"kafkalens/internal/config"
	"kafkalens/internal/kafka"
	"kafkalens/internal/mcpserver"
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

// clustersFile is the clusters file from the settings, or <user config dir>/kafkalens/clusters.json.
func clustersFile(cfg config.Config) string {
	if cfg.Clusters != "" {
		return cfg.Clusters
	}
	home, err := os.UserConfigDir()
	if err != nil {
		fatal("cannot find the user config dir", "error", err)
	}
	return filepath.Join(home, "kafkalens", "clusters.json")
}

// newGate is the MCP switch: locked off by mcp.enabled: false, else on, with the token kept next to the clusters file.
func newGate(cfg config.Config, clusters string) *mcpserver.Gate {
	g := &mcpserver.Gate{Locked: !cfg.MCP.Enabled}
	if g.Locked {
		return g
	}
	t, err := mcpserver.LoadToken(filepath.Join(filepath.Dir(clusters), "mcp-token"))
	if err != nil {
		g.Err = "cannot keep the MCP token: " + err.Error()
		log.Warn().Err(err).Msg("MCP unavailable")
	}
	g.Token = t
	return g
}

// newHandler serves the API, the embedded UI and, unless the settings turn it off, MCP at /mcp;
// shared by the server and desktop builds. mcp is the MCP handler alone (nil when off), for the desktop's listener.
func newHandler(st *store.Store, gate *mcpserver.Gate) (h, mcp http.Handler) {
	r := chi.NewRouter()
	r.Use(harden)
	a := api.New(st, kafka.NewPool(), version(), gate)
	r.Mount("/api", a)
	if !gate.Locked {
		mcp = mcpserver.Handler(a, version(), gate)
		r.Handle("/mcp", mcp)
	}

	ui, _ := fs.Sub(dist, "web/dist/web/browser")
	files := http.FileServerFS(ui)
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// MCP clients probe /.well-known/oauth-* and take a 200 (index.html) for an OAuth server, then drop
		// the configured token: well-known paths are not app routes
		if strings.HasPrefix(r.URL.Path, "/.well-known/") {
			http.NotFound(w, r)
			return
		}
		// SPA fallback: unknown non-asset paths serve index.html
		if _, err := fs.Stat(ui, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
	return r, mcp
}

// listenURL is the http URL of a listener: a wildcard address (":8088", "0.0.0.0:8088") is reached as
// localhost, since http://[::]:8088 would be refused by localOnly.
func listenURL(addr net.Addr) string {
	a := addr.(*net.TCPAddr)
	host := a.IP.String()
	if a.IP.IsUnspecified() {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(a.Port))
}

// localOnly guards the unauthenticated API against other web pages: DNS rebinding (a page on
// evil.example resolved to 127.0.0.1 still sends Host: evil.example) and cross-site POSTs.
// The desktop app needs it only for its MCP listener.
func localOnly(h http.Handler) http.Handler {
	h = http.NewCrossOriginProtection().Handler(h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = strings.Trim(r.Host, "[]") // no port
		}
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
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
