package main

import (
	"bytes"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestOpenLogFileRotates(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "kafkalens.log")
	os.WriteFile(p, make([]byte, 11<<20), 0o600)
	f, got, err := openLogFile(dir)
	if err != nil || got != p {
		t.Fatal(got, err)
	}
	f.WriteString("new\n")
	f.Close()
	if fi, _ := os.Stat(p + ".1"); fi == nil || fi.Size() != 11<<20 {
		t.Error("big log not moved to .1")
	}
	if b, _ := os.ReadFile(p); string(b) != "new\n" {
		t.Errorf("fresh log: %q", b)
	}
}

func TestVersionFromWailsJSON(t *testing.T) {
	if v := version(); v == "dev" || v == "" {
		t.Errorf("productVersion not read from wails.json: %q", v)
	}
}

func TestStdlogRoutedAsWarning(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	defer func() { log.Logger = prev }()
	stdlog.New(stdlogWriter{}, "", 0).Print("http: TLS handshake error\n")
	if got := buf.String(); !strings.Contains(got, `"level":"warn","source":"log","message":"http: TLS handshake error"`) {
		t.Errorf("got %s", got)
	}
}
