package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"kafkalens/internal/kafka"
	"kafkalens/internal/store"
)

func TestRequestLog(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf).Level(zerolog.InfoLevel)
	defer func() { log.Logger = prev }()

	st, _ := store.OpenStore(filepath.Join(t.TempDir(), "clusters.json"))
	h := New(st, kafka.NewPool(), "1.2.3")
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/c/nope/topics", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/clusters", nil))

	out := buf.String()
	if want := `{"level":"warn","status":404,`; !strings.Contains(out, want) ||
		!strings.Contains(out, `"error":"cluster not found","message":"GET /c/nope/topics"`) {
		t.Errorf("failed call not logged as %s… with its error:\n%s", want, out)
	}
	if strings.Contains(out, "/clusters\"") {
		t.Errorf("successful GET should only log at debug:\n%s", out)
	}

	// the SSE tail flushes through the wrapper
	rec := httptest.NewRecorder()
	if err := http.NewResponseController(&statusWriter{ResponseWriter: rec}).Flush(); err != nil || !rec.Flushed {
		t.Errorf("flush through statusWriter: err=%v flushed=%v", err, rec.Flushed)
	}
}
