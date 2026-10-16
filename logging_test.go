package main

import (
	"os"
	"path/filepath"
	"testing"
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
