//go:build !desktop && !bindings

package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"kafkalens/internal/store"
)

func TestLocalOnly(t *testing.T) {
	st, _ := store.OpenStore(filepath.Join(t.TempDir(), "clusters.json"))
	h := localOnly(newHandler(st))
	call := func(method, host, site string) int {
		r := httptest.NewRequest(method, "/api/clusters", strings.NewReader(`{"name":"x","brokers":["evil:9092"]}`))
		r.Host = host
		if site != "" {
			r.Header.Set("Sec-Fetch-Site", site)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for _, host := range []string{"localhost:8088", "127.0.0.1:8088", "[::1]:8088", "[::1]"} {
		if c := call("GET", host, ""); c != http.StatusOK {
			t.Errorf("GET via %s: %d", host, c)
		}
	}
	if c := call("GET", "evil.example:8088", ""); c != http.StatusForbidden {
		t.Errorf("DNS rebinding: %d, want 403", c)
	}
	if c := call("POST", "127.0.0.1:8088", "cross-site"); c != http.StatusForbidden || len(st.List()) != 0 {
		t.Errorf("cross-site POST: %d, %d clusters, want 403 and none", c, len(st.List()))
	}
}
