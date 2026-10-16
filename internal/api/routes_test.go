package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"kafkalens/internal/kafka"
	"kafkalens/internal/store"
)

// Route table and guards, without a broker: every cluster route must resolve
// (unknown cluster → 404 "cluster not found", not a router 404/405) and every
// mutating route must be refused on a read-only cluster before touching Kafka.
func TestRoutes(t *testing.T) {
	st, _ := store.OpenStore(filepath.Join(t.TempDir(), "clusters.json"))
	ro, err := st.Save(store.Cluster{Name: "prod", Brokers: []string{"127.0.0.1:1"}, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, kafka.NewPool())
	call := func(method, path string) (int, string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader("{}")))
		return rec.Code, rec.Body.String()
	}

	reads := []string{"GET /overview", "GET /brokers/1/configs", "GET /topics", "GET /topics/t", "POST /topics/t/browse",
		"GET /topics/t/tail", "GET /groups", "GET /groups/g"}
	writes := []string{"POST /topics", "DELETE /topics/t", "PUT /topics/t/configs", "POST /topics/t/partitions",
		"POST /topics/t/produce", "POST /groups/g/reset", "DELETE /groups/g"}
	for _, rt := range append(reads, writes...) {
		m, p, _ := strings.Cut(rt, " ")
		if code, body := call(m, "/c/nope"+p); code != http.StatusNotFound || !strings.Contains(body, "cluster not found") {
			t.Errorf("%s on unknown cluster: %d %s", rt, code, body)
		}
	}
	for _, rt := range writes {
		m, p, _ := strings.Cut(rt, " ")
		if code, body := call(m, "/c/"+ro.ID+p); code != http.StatusForbidden || !strings.Contains(body, "read-only") {
			t.Errorf("%s on read-only cluster: %d %s", rt, code, body)
		}
	}

	if code, body := call("GET", "/clusters"); code != http.StatusOK || !strings.Contains(body, `"prod"`) {
		t.Errorf("list clusters: %d %s", code, body)
	}
	if code, body := call("GET", "/nope"); code != http.StatusNotFound || !strings.Contains(body, `"error"`) {
		t.Errorf("unknown route: %d %s", code, body)
	}
	if code, body := call("PATCH", "/clusters"); code != http.StatusMethodNotAllowed || !strings.Contains(body, `"error"`) {
		t.Errorf("wrong method: %d %s", code, body)
	}
}
