//go:build desktop || bindings

// Desktop mode (wails build): the same handler served inside a native window,
// no TCP port. Wails' asset server buffers whole responses, so live tail uses
// Wails events instead of SSE, and exports go through a native save dialog.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Desktop is bound to the UI as window.go.main.Desktop.
type Desktop struct {
	ctx   context.Context
	store *Store
	mu    sync.Mutex
	tails map[string]context.CancelFunc
	seq   int
}

// Tail starts streaming new records of topic as "tail:<session>" events, one
// event per polled batch; an {"error": ...} event reports a failed stream.
func (d *Desktop) Tail(id, topic string) (string, error) {
	cl, ok := d.store.Get(id)
	if !ok {
		return "", errors.New("cluster not found")
	}
	if !acquireTail() {
		return "", errors.New("too many live tails open")
	}
	k, err := openTail(cl, topic)
	if err != nil {
		releaseTail()
		return "", err
	}
	ctx, cancel := context.WithCancel(d.ctx)
	d.mu.Lock()
	d.seq++
	sid := strconv.Itoa(d.seq)
	d.tails[sid] = cancel
	d.mu.Unlock()
	go func() {
		defer releaseTail()
		defer k.Close()
		pumpTail(ctx, k, func(batch []Record) error {
			wr.EventsEmit(d.ctx, "tail:"+sid, batch)
			return nil
		})
	}()
	return sid, nil
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

func main() {
	cfg := defaultConfig()
	store, err := OpenStore(cfg)
	if err != nil {
		log.Fatalf("load %s: %v", cfg, err)
	}
	d := &Desktop{store: store, tails: map[string]context.CancelFunc{}}
	err = wails.Run(&options.App{
		Title:            "KafkaLens",
		Width:            1400,
		Height:           900,
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 11, G: 14, B: 20, A: 255},
		AssetServer:      &assetserver.Options{Handler: newHandler(store)},
		OnStartup:        func(ctx context.Context) { d.ctx = ctx },
		Bind:             []any{d},
	})
	if err != nil {
		log.Fatal(err)
	}
}
