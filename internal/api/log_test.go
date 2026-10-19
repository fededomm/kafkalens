package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"kafkalens/internal/kafka"
	"kafkalens/internal/store"
)

func TestRequestLog(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(prev)

	st, _ := store.OpenStore(filepath.Join(t.TempDir(), "clusters.json"))
	h := New(st, kafka.NewPool(), "1.2.3")
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/c/nope/topics", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/clusters", nil))

	out := buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "path=/c/nope/topics") || !strings.Contains(out, `error="cluster not found"`) {
		t.Errorf("failed call not logged with its error:\n%s", out)
	}
	if strings.Contains(out, "path=/clusters") {
		t.Errorf("successful GET should only log at debug:\n%s", out)
	}

	// the SSE tail flushes through the wrapper
	rec := httptest.NewRecorder()
	if err := http.NewResponseController(&statusWriter{ResponseWriter: rec}).Flush(); err != nil || !rec.Flushed {
		t.Errorf("flush through statusWriter: err=%v flushed=%v", err, rec.Flushed)
	}
}
