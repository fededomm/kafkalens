package api

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

		lvl := zerolog.InfoLevel
		switch {
		case sw.status >= 500:
			lvl = zerolog.ErrorLevel
		case sw.status >= 400:
			lvl = zerolog.WarnLevel
		case r.Method == http.MethodGet:
			lvl = zerolog.DebugLevel
		}
		// reads as: 18:05:01.375 WRN DELETE /api/c/ro/topics/x error="cluster is read-only" ms=0 status=403
		log.WithLevel(lvl).Int("status", sw.status).Int64("ms", time.Since(start).Milliseconds()).
			Err(sw.err).Msg(r.Method + " " + r.URL.Path)
	})
}
