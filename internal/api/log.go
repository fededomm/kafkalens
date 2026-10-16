package api

import (
	"log/slog"
	"net/http"
	"time"
)

// statusWriter records the status and, via reply, the error of a response.
type statusWriter struct {
	http.ResponseWriter
	status int
	err    error
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer (Flush for the SSE tail).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logRequests logs one line per API call: failures always, successful reads only at debug
// level (the UI polls), successful writes at info. Bodies are never logged: they can hold credentials.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)

		lvl := slog.LevelInfo
		switch {
		case sw.status >= 500:
			lvl = slog.LevelError
		case sw.status >= 400:
			lvl = slog.LevelWarn
		case r.Method == http.MethodGet:
			lvl = slog.LevelDebug
		}
		attrs := []any{"method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds()}
		if sw.err != nil {
			attrs = append(attrs, "error", sw.err.Error())
		}
		slog.Log(r.Context(), lvl, "api", attrs...)
	})
}
