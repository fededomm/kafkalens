//go:build desktop || bindings

// Desktop mode (wails build): the same handler served inside a native window,
// no TCP port but the MCP one (serveMCP). Wails' asset server buffers whole responses, so live tail uses
// Wails events instead of SSE, and exports go through a native save dialog.
package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"kafkalens/internal/config"
	"kafkalens/internal/kafka"
	"kafkalens/internal/mcpserver"
	"kafkalens/internal/store"
)

// Desktop is bound to the UI as window.go.main.Desktop.
type Desktop struct {
	ctx   context.Context
	store *store.Store
	mu    sync.Mutex
	tails map[string]context.CancelFunc
}

// Tail starts streaming new records of topic as "tail:<sid>" events, one event per polled
// batch; a Kafka error ends the stream with a "tail:<sid>:error" event (the message). The UI
// picks sid and subscribes first: Wails drops events emitted before a listener exists.
func (d *Desktop) Tail(id, topic, sid string) error {
	cl, ok := d.store.Get(id)
	if !ok {
		return errors.New("cluster not found")
	}
	k, stop, err := kafka.OpenTail(cl, topic)
	if err != nil {
		log.Warn().Str("cluster", id).Str("topic", topic).Err(err).Msg("live tail")
		return err
	}
	ctx, cancel := context.WithCancel(d.ctx)
	d.mu.Lock()
	d.tails[sid] = cancel
	d.mu.Unlock()
	go func() {
		defer stop()
		err := kafka.PumpTail(ctx, k, func(batch []kafka.Record) error {
			wr.EventsEmit(d.ctx, "tail:"+sid, batch)
			return nil
		})
		if err != nil {
			log.Warn().Str("cluster", id).Str("topic", topic).Err(err).Msg("live tail")
			wr.EventsEmit(d.ctx, "tail:"+sid+":error", err.Error())
		}
	}()
	return nil
}

func (d *Desktop) StopTail(sid string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cancel, ok := d.tails[sid]; ok {
		cancel()
		delete(d.tails, sid)
	}
}

// SaveFile asks where to save an export; returns "" when the user cancels.
func (d *Desktop) SaveFile(name, content string) (string, error) {
	path, err := wr.SaveFileDialog(d.ctx, wr.SaveDialogOptions{DefaultFilename: name, CanCreateDirectories: true})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, []byte(content), 0o644)
}

// serveMCP opens the desktop app's only TCP port, on loopback, for MCP clients: the window itself
// talks to the backend in-process. A taken port is not fatal: the sidebar tells why MCP is off.
// It returns the function closing the listener.
func serveMCP(addr string, mcp http.Handler, gate *mcpserver.Gate) (stop func()) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		gate.Err = "cannot listen on " + addr + ": " + err.Error()
		log.Warn().Err(err).Str("addr", addr).Msg("MCP unavailable")
		return func() {}
	}
	gate.URL = listenURL(ln.Addr()) + "/mcp"
	log.Info().Str("url", gate.URL).Msg("MCP listening")
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp)
	srv := &http.Server{Handler: localOnly(harden(mux)), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	go srv.Serve(ln)
	return func() { srv.Close() }
}

func main() {
	settings, _, err := config.Load(defaultSettings, "", false) // embedded config.yaml + KAFKALENS_* env
	if err != nil {
		fatal("invalid built-in settings", "error", err)
	}
	cfg := clustersFile(settings)
	// installed app, often started without a terminal (no console at all on Windows): log to a file
	logPath := "stderr"
	if f, p, err := openLogFile(filepath.Dir(cfg)); err == nil {
		defer f.Close()
		setupLogging(settings, f) // file + stderr
		logPath = p
	} else {
		setupLogging(settings, nil)
		log.Warn().Err(err).Msg("cannot open log file, logging to stderr")
	}
	log.Info().Str("version", version()).Str("mode", "desktop").Str("clusters", cfg).Str("log", logPath).Msg("KafkaLens started")

	st, err := store.OpenStore(cfg)
	if err != nil {
		fatal("load clusters", "file", cfg, "error", err)
	}
	gate := newGate(settings, cfg)
	h, mcp := newHandler(st, gate)
	if mcp != nil && gate.Err == "" {
		defer serveMCP(settings.MCP.Addr, mcp, gate)()
	}
	d := &Desktop{store: st, tails: map[string]context.CancelFunc{}}
	err = wails.Run(&options.App{
		Title:            "KafkaLens",
		Width:            1400,
		Height:           900,
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 11, G: 14, B: 20, A: 255},
		AssetServer:      &assetserver.Options{Handler: h},
		OnStartup:        func(ctx context.Context) { d.ctx = ctx },
		Bind:             []any{d},
	})
	if err != nil {
		fatal("desktop window", "error", err)
	}
}
