package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

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
	h := New(st, kafka.NewPool(), "1.2.3")
	call := func(method, path string) (int, string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader("{}")))
		return rec.Code, rec.Body.String()
	}

	reads := []string{"GET /overview", "GET /brokers/1/configs", "GET /topics", "GET /topics/t", "POST /topics/t/browse",
		"GET /topics/t/tail", "GET /groups", "GET /groups/g"}
	writes := []string{"POST /topics", "DELETE /topics/t", "DELETE /topics/t/records", "POST /truncate", "PUT /topics/t/configs", "POST /topics/t/partitions",
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
	if code, body := call("GET", "/version"); code != http.StatusOK || !strings.Contains(body, `"version":"1.2.3"`) {
		t.Errorf("version: %d %s", code, body)
	}
	if code, body := call("GET", "/nope"); code != http.StatusNotFound || !strings.Contains(body, `"error"`) {
		t.Errorf("unknown route: %d %s", code, body)
	}
	if code, body := call("PATCH", "/clusters"); code != http.StatusMethodNotAllowed || !strings.Contains(body, `"error"`) {
		t.Errorf("wrong method: %d %s", code, body)
	}
}

// The UI encodes group names with encodeURIComponent; chi hands RawPath params over still encoded.
func TestPathParam(t *testing.T) {
	var got string
	r := chi.NewRouter()
	r.Get("/g/{group}", func(w http.ResponseWriter, r *http.Request) { got = pathParam(r, "group") })
	for path, want := range map[string]string{"/g/a%3Ab": "a:b", "/g/a%2Fb": "a/b", "/g/a%2541": "a%41", "/g/a%20b": "a b"} {
		got = ""
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		if got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}

// Needs a broker on localhost:9092 (`make kafka`); skipped otherwise. Also pins the UI's guess that
// brokers refuse DeleteRecords on a topic whose cleanup.policy lacks delete.
func TestTruncateTopic(t *testing.T) {
	if c, err := net.DialTimeout("tcp", "localhost:9092", time.Second); err != nil {
		t.Skip("no local kafka")
	} else {
		c.Close()
	}
	ctx := context.Background()
	st, _ := store.OpenStore(filepath.Join(t.TempDir(), "clusters.json"))
	cl, _ := st.Save(store.Cluster{Name: "local", Brokers: []string{"localhost:9092"}})
	pool := kafka.NewPool()
	adm, err := pool.Admin(cl)
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, pool, "test")
	k, _ := kgo.NewClient(kgo.SeedBrokers(cl.Brokers...))
	defer k.Close()
	mk := func(policy string) string {
		topic := fmt.Sprintf("kafkalens-test-%d", time.Now().UnixNano())
		if _, err := adm.CreateTopic(ctx, 2, 1, map[string]*string{"cleanup.policy": &policy}, topic); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { adm.DeleteTopic(context.Background(), topic) })
		for i := range 10 {
			if err := k.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: fmt.Appendf(nil, "k%d", i), Value: []byte("v")}).FirstErr(); err != nil {
				t.Fatal(err)
			}
		}
		return topic
	}
	call := func(method, path, body string) (int, string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, "/c/"+cl.ID+path, strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	empty := func(topic string) {
		starts, _ := adm.ListStartOffsets(ctx, topic)
		ends, _ := adm.ListEndOffsets(ctx, topic)
		ends.Each(func(e kadm.ListedOffset) {
			if s, _ := starts.Lookup(topic, e.Partition); s.Offset != e.Offset {
				t.Errorf("%s partition %d: start %d, end %d", topic, e.Partition, s.Offset, e.Offset)
			}
		})
	}

	topic := mk("delete")
	if code, body := call("DELETE", "/topics/"+topic+"/records", ""); code != http.StatusOK {
		t.Fatalf("truncate: %d %s", code, body)
	}
	empty(topic)
	if code, _ := call("DELETE", "/topics/"+mk("compact")+"/records", ""); code == http.StatusOK {
		t.Error("truncate of a compacted topic succeeded: the UI wrongly disables it")
	}

	// bulk: the compacted topic fails alone, the others are still emptied
	a, b, cp := mk("delete"), mk("delete"), mk("compact")
	code, body := call("POST", "/truncate", fmt.Sprintf(`{"topics":[%q,%q,%q]}`, a, b, cp))
	if code != http.StatusOK || strings.Contains(body, a) || strings.Contains(body, b) || !strings.Contains(body, cp) {
		t.Fatalf("bulk truncate: %d %s", code, body)
	}
	empty(a)
	empty(b)
}
