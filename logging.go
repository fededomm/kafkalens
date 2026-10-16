package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"kafkalens/internal/config"
)

// setupLogging sends slog (and the log package, which slog takes over) to w at log.level
// (config.yaml, or KAFKALENS_LOG_LEVEL through config.Load).
func setupLogging(w io.Writer, cfg config.Config) {
	lvl, _ := cfg.LogLevel() // validated by config.Load
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})))
}

// openLogFile opens dir/kafkalens.log for appending. Past 10 MiB the old file becomes kafkalens.log.1.
// ponytail: rotates only at startup, a session that logs > 10 MiB keeps growing; rotate on write if that happens.
func openLogFile(dir string) (*os.File, string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", err
	}
	p := filepath.Join(dir, "kafkalens.log")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 10<<20 {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	return f, p, err
}

// fatal logs at error level and exits; log.Fatal would log at info once slog is the default.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}
