//go:build !desktop && !bindings

package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kafkalens/internal/mcpserver"
	"kafkalens/internal/store"
)

func TestLocalOnly(t *testing.T) {
	st, _ := store.OpenStore(filepath.Join(t.TempDir(), "clusters.json"))
	app, _ := newHandler(st, &mcpserver.Gate{Token: "tok"})
	h := localOnly(app)
	call := func(method, host, site string) int {
		return serve(h, method, "/api/clusters", host, site)
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
	// MCP sits behind the same guard, and its own token
	if c := serve(h, "POST", "/mcp", "evil.example:8088", ""); c != http.StatusForbidden {
		t.Errorf("MCP via DNS rebinding: %d, want 403", c)
	}
	if c := serve(h, "POST", "/mcp", "127.0.0.1:8088", ""); c != http.StatusUnauthorized {
		t.Errorf("MCP without token: %d, want 401", c)
	}
	// no OAuth here: the SPA fallback must not answer the discovery probes with index.html
	if c := serve(h, "GET", "/.well-known/oauth-protected-resource/mcp", "127.0.0.1:8088", ""); c != http.StatusNotFound {
		t.Errorf("OAuth discovery probe: %d, want 404", c)
	}
	if c := serve(h, "GET", "/topics", "127.0.0.1:8088", ""); c != http.StatusOK {
		t.Errorf("SPA route: %d, want 200", c)
	}
	// turned off in the settings: no endpoint, the SPA fallback is GET only
	off, _ := newHandler(st, &mcpserver.Gate{Locked: true})
	if c := serve(localOnly(off), "POST", "/mcp", "127.0.0.1:8088", ""); c != http.StatusMethodNotAllowed && c != http.StatusNotFound {
		t.Errorf("MCP locked off: %d", c)
	}
}

// Tools reach the real API through the whole handler: a GET tool must not be routed with the
// method of the MCP request (POST /clusters would save a cluster).
func TestMCPThroughHandler(t *testing.T) {
	st, _ := store.OpenStore(filepath.Join(t.TempDir(), "clusters.json"))
	st.Save(store.Cluster{Name: "local", Brokers: []string{"127.0.0.1:1"}})
	h, _ := newHandler(st, &mcpserver.Gate{Token: "tok"})
	srv := httptest.NewServer(h)
	defer srv.Close()
	hc := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer tok")
		return http.DefaultTransport.RoundTrip(r)
	})}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: hc, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_clusters"})
	if err != nil || res.IsError || len(st.List()) != 1 {
		t.Fatalf("list_clusters: %v %+v, %d clusters", err, res, len(st.List()))
	}
	if txt := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(txt, `"name":"local"`) {
		t.Errorf("list_clusters: %s", txt)
	}
}

func TestListenURL(t *testing.T) {
	for addr, want := range map[string]string{"127.0.0.1:0": "http://127.0.0.1:", "[::1]:0": "http://[::1]:", ":0": "http://localhost:", "0.0.0.0:0": "http://localhost:"} {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			continue // no IPv6 here
		}
		if got := listenURL(ln.Addr()); !strings.HasPrefix(got, want) {
			t.Errorf("%s: %s, want %s…", addr, got, want)
		}
		ln.Close()
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func serve(h http.Handler, method, path, host, site string) int {
	r := httptest.NewRequest(method, path, strings.NewReader(`{"name":"x","brokers":["evil:9092"]}`))
	r.Host = host
	if site != "" {
		r.Header.Set("Sec-Fetch-Site", site)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}
