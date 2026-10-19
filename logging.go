package main

import (
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"kafkalens/internal/config"
)

const (
	consoleTime = "15:04:05.000"
	fileTime    = "2006-01-02 15:04:05.000"
)

// until setupLogging runs (e.g. errors while reading the settings), log readable lines to stderr
func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano // default is whole seconds: keep the milliseconds
	log.Logger = zerolog.New(console(os.Stderr, consoleTime)).With().Timestamp().Logger()
}

// setupLogging sends logs to the console and, when file is not nil, also to file, at log.level
// (config.yaml, or KAFKALENS_LOG_LEVEL through config.Load). The standard log package, used by
// net/http for server errors, is routed through it too.
func setupLogging(cfg config.Config, file *os.File) {
	lvl, _ := cfg.LogLevel() // validated by config.Load
	var w io.Writer = console(os.Stderr, consoleTime)
	if file != nil {
		w = zerolog.MultiLevelWriter(w, console(file, fileTime))
	}
	log.Logger = zerolog.New(w).Level(lvl).With().Timestamp().Logger()
	stdlog.SetFlags(0)
	stdlog.SetOutput(stdlogWriter{})
}

// console renders events as aligned, human-readable lines; colors only on a terminal, never in a file.
func console(f *os.File, timeFormat string) zerolog.ConsoleWriter {
	fi, err := f.Stat()
	tty := err == nil && fi.Mode()&os.ModeCharDevice != 0
	return zerolog.ConsoleWriter{Out: f, TimeFormat: timeFormat, NoColor: !tty}
}

// stdlogWriter turns standard log lines into warnings.
type stdlogWriter struct{}

func (stdlogWriter) Write(p []byte) (int, error) {
	log.Warn().Str("source", "log").Msg(strings.TrimSpace(string(p)))
	return len(p), nil
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

// fatal logs msg with key/value pairs (e.g. "file", path, "error", err) at error level and exits.
func fatal(msg string, kv ...any) {
	log.Error().Fields(kv).Msg(msg)
	os.Exit(1)
}
