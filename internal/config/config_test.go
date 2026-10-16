package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	defaults, err := os.ReadFile("../../config.yaml") // the file embedded in the binaries
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "config.yaml")
		os.WriteFile(p, []byte(body), 0o600)
		return p
	}

	c, src, err := Load(defaults, "", false)
	if err != nil || src != "built-in defaults" || c.Server.Addr != "127.0.0.1:8088" || !c.Server.OpenBrowser || c.Clusters != "" || c.Log.Level != "info" {
		t.Fatalf("defaults: %+v %q %v", c, src, err)
	}

	// partial file: only the given keys change (key case doesn't matter)
	p := write("server:\n  openBrowser: false\nlog:\n  level: DEBUG\n")
	c, src, err = Load(defaults, p, false)
	if lvl, _ := c.LogLevel(); err != nil || src != p || c.Server.OpenBrowser || c.Server.Addr != "127.0.0.1:8088" || lvl != slog.LevelDebug {
		t.Fatalf("overlay: %+v %q %v", c, src, err)
	}
	if _, _, err := Load(defaults, write("# only comments\n"), false); err != nil {
		t.Errorf("comments-only file: %v", err)
	}

	// a missing file is fine only when optional
	missing := filepath.Join(dir, "nope.yaml")
	if _, src, err := Load(defaults, missing, true); err != nil || src != "built-in defaults" {
		t.Errorf("optional missing file: %q %v", src, err)
	}
	if _, _, err := Load(defaults, missing, false); err == nil {
		t.Error("explicit missing file must fail")
	}

	// environment beats the file
	t.Setenv("KAFKALENS_LOG_LEVEL", "warn")
	t.Setenv("KAFKALENS_SERVER_ADDR", "127.0.0.1:9999")
	c, _, err = Load(defaults, write("log:\n  level: debug\n"), false)
	if err != nil || c.Log.Level != "warn" || c.Server.Addr != "127.0.0.1:9999" {
		t.Errorf("env override: %+v %v", c, err)
	}
	os.Unsetenv("KAFKALENS_LOG_LEVEL")
	os.Unsetenv("KAFKALENS_SERVER_ADDR")

	for in, want := range map[string]string{
		"server:\n  adr: x\n":     "adr",
		"logs:\n  level: info\n":  "logs",
		"log:\n  level: loud\n":   "log.level",
		"server:\n  addr: \"\"\n": "server.addr is empty",
		"server: [1, 2]\n":        "server",
	} {
		if _, _, err := Load(defaults, write(in), false); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want error containing %q", in, err, want)
		}
	}
}
