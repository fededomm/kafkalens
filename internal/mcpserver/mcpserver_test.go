package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeAPI answers the REST routes the tools call with canned JSON and keeps the last request body.
type fakeAPI struct{ body string }

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.body = string(b)
	big := strings.Repeat("é", maxText) // 2 bytes per rune: clipping must not split one
	var hs []string
	for range maxHeaders + 1 {
		hs = append(hs, `{"key":"`+strings.Repeat("k", maxText+1)+`","value":"v"}`)
	}
	routes := map[string]string{
		"GET /clusters": `[{"id":"c1","name":"prod","brokers":["k:9092"],"readOnly":true,"saslUser":"admin","tlsCert":"PEM"}]`,
		"GET /c/c1/topics": `[{"name":"__consumer_offsets","internal":true,"partitions":50},
			{"name":"orders","partitions":3,"messages":10},{"name":"Orders.dlq","partitions":1},{"name":"payments","partitions":1}]`,
		"GET /c/c1/topics/orders": `{"name":"orders","partitions":[],"groups":[],"configs":[
			{"key":"retention.ms","value":"1000","source":"DYNAMIC_TOPIC_CONFIG","isDefault":false},
			{"key":"segment.bytes","value":"1","source":"DEFAULT_CONFIG","isDefault":true},
			{"key":"sasl.jaas.config","value":"secret","sensitive":true,"source":"DYNAMIC_TOPIC_CONFIG","isDefault":false}]}`,
		"POST /c/c1/topics/orders/browse": `{"scanned":1,"partial":true,"records":[{"partition":0,"offset":7,"key":"k","value":"` + big +
			`","size":9000,"headers":[` + strings.Join(hs, ",") + `]}]}`,
		"GET /c/c1/groups/a%2Fb": `{"group":"a/b","partitions":[{"topic":"orders","partition":0},{"topic":"payments","partition":0}]}`,
	}
	key := r.Method + " " + r.URL.EscapedPath()
	if out, ok := routes[key]; ok {
		w.Write([]byte(out))
		return
	}
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"error":"cluster not found"}`))
}

func connect(t *testing.T, api http.Handler) *mcp.ClientSession {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := newServer(api, "test").Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// call runs a tool and decodes its structured result; errText is the error a failing tool reports.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (out map[string]any, errText string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		return nil, res.Content[0].(*mcp.TextContent).Text
	}
	b, _ := json.Marshal(res.StructuredContent)
	json.Unmarshal(b, &out)
	return out, ""
}

func TestTools(t *testing.T) {
	api := &fakeAPI{}
	cs := connect(t, api)

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
			t.Errorf("%s is not marked read-only", tl.Name)
		}
	}

	// clusters: no credentials or certificates
	out, _ := call(t, cs, "list_clusters", nil)
	if b, _ := json.Marshal(out); strings.Contains(string(b), "admin") || strings.Contains(string(b), "PEM") || !strings.Contains(string(b), `"readOnly":true`) {
		t.Errorf("list_clusters: %s", b)
	}

	// topics: internal hidden, filter ignores case, limit caps, counts stay
	out, _ = call(t, cs, "list_topics", map[string]any{"cluster": "c1", "filter": "ORDERS", "limit": 1})
	if ts := out["topics"].([]any); out["total"] != 4.0 || out["matched"] != 2.0 || len(ts) != 1 || ts[0].(map[string]any)["name"] != "orders" {
		t.Errorf("list_topics: %v", out)
	}
	if out, _ = call(t, cs, "list_topics", map[string]any{"cluster": "c1", "limit": -5}); len(out["topics"].([]any)) != 3 {
		t.Errorf("list_topics with limit -5: %v", out)
	}

	// configs: defaults dropped, sensitive values never sent
	out, _ = call(t, cs, "describe_topic", map[string]any{"cluster": "c1", "topic": "orders"})
	if b, _ := json.Marshal(out["configs"]); strings.Contains(string(b), "segment.bytes") || strings.Contains(string(b), "secret") || !strings.Contains(string(b), "retention.ms") {
		t.Errorf("describe_topic configs: %s", b)
	}

	// browse: newest of all partitions by default (not partition 0), 20 records, texts clipped on a rune boundary
	out, _ = call(t, cs, "browse_messages", map[string]any{"cluster": "c1", "topic": "orders"})
	var req map[string]any
	json.Unmarshal([]byte(api.body), &req)
	if req["from"] != "latest" || req["partition"] != -1.0 || req["limit"] != 20.0 {
		t.Errorf("browse request: %s", api.body)
	}
	r := out["records"].([]any)[0].(map[string]any)
	if v := r["value"].(string); len(v) > maxText || !strings.HasPrefix(v, "éé") || strings.ContainsRune(v, '�') || r["clipped"] != true || out["partial"] != true {
		t.Errorf("browse record: %d bytes, clipped=%v partial=%v", len(v), r["clipped"], out["partial"])
	}
	if hs := r["headers"].([]any); len(hs) != maxHeaders || len(hs[0].(map[string]any)["key"].(string)) != maxText {
		t.Errorf("headers: %d kept, first key %d bytes; want %d and %d", len(hs), len(hs[0].(map[string]any)["key"].(string)), maxHeaders, maxText)
	}
	// a negative limit is the default, not Browse's 500
	call(t, cs, "browse_messages", map[string]any{"cluster": "c1", "topic": "orders", "limit": -5})
	json.Unmarshal([]byte(api.body), &req)
	if req["limit"] != float64(browseDefault) {
		t.Errorf("browse with limit -5: %s", api.body)
	}
	call(t, cs, "browse_messages", map[string]any{"cluster": "c1", "topic": "orders", "partition": 0, "limit": 5000, "from": "timestamp", "time": "2026-11-03T09:00:00Z"})
	json.Unmarshal([]byte(api.body), &req)
	if req["partition"] != 0.0 || req["limit"] != float64(browseMax) || req["timestamp"] != 1793696400000.0 {
		t.Errorf("browse request with partition, time and a big limit: %s", api.body)
	}
	if _, e := call(t, cs, "browse_messages", map[string]any{"cluster": "c1", "topic": "orders", "from": "timestamp", "time": "yesterday"}); !strings.Contains(e, "RFC 3339") {
		t.Errorf("bad time: %q", e)
	}

	// group names travel escaped, as one path segment; topic narrows the partitions
	out, _ = call(t, cs, "describe_consumer_group", map[string]any{"cluster": "c1", "group": "a/b", "topic": "orders"})
	if out["partitionsTotal"] != 1.0 {
		t.Errorf("describe_consumer_group: %v", out)
	}

	// API errors become tool errors the model can read
	if _, e := call(t, cs, "cluster_overview", map[string]any{"cluster": "nope"}); e != "cluster not found" {
		t.Errorf("error: %q", e)
	}
}

func TestPrompts(t *testing.T) {
	cs := connect(t, &fakeAPI{})
	res, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "diagnose_lag", Arguments: map[string]string{"cluster": "c1", "group": "billing"}})
	if err != nil {
		t.Fatal(err)
	}
	if txt := res.Messages[0].Content.(*mcp.TextContent).Text; !strings.Contains(txt, "billing") || !strings.Contains(txt, "c1") || strings.Contains(txt, "{") {
		t.Errorf("prompt text: %s", txt)
	}
	if _, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "inspect_topic", Arguments: map[string]string{"cluster": "c1"}}); !errors.Is(err, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams}) && !strings.Contains(fmt.Sprint(err), "-32602") {
		t.Errorf("missing argument: %v, want invalid params (-32602)", err)
	}
}

// Over HTTP: the switch and the token guard the endpoint; a real client gets through with the token.
func TestHandler(t *testing.T) {
	g := &Gate{Token: "s3cret"}
	srv := httptest.NewServer(Handler(&fakeAPI{}, "test", g))
	defer srv.Close()
	connectHTTP := func(token string) error {
		hc := &http.Client{Transport: bearer{token}}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: hc, MaxRetries: -1}, nil)
		if err == nil {
			_, err = cs.ListTools(context.Background(), nil)
			cs.Close()
		}
		return err
	}
	if err := connectHTTP("s3cret"); err != nil {
		t.Fatalf("with the token: %v", err)
	}
	if err := connectHTTP("wrong"); err == nil {
		t.Error("wrong token accepted")
	}
	g.Set(false)
	resp, _ := http.Post(srv.URL, "application/json", strings.NewReader("{}"))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("switched off: %d", resp.StatusCode)
	}
	if err := connectHTTP("s3cret"); err == nil {
		t.Error("switched off, still served")
	}
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestLoadToken(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "mcp-token")
	a, err := LoadToken(p)
	if err != nil || len(a) < 20 {
		t.Fatalf("new token %q: %v", a, err)
	}
	if b, err := LoadToken(p); b != a || err != nil {
		t.Errorf("token changed on reload: %q -> %q (%v)", a, b, err)
	}
	if (&Gate{}).On() || (&Gate{Token: "x", Locked: true}).On() || (&Gate{Token: "x", Err: "port taken"}).On() || !(&Gate{Token: "x"}).On() {
		t.Error("gate: only an unlocked gate with a token and no error starts on")
	}
}
