package mcpserver

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kafkalens/internal/kafka"
)

// LLM context is small: lists are filtered and capped, message texts clipped.
const (
	listDefault, listMax     = 100, 1000
	browseDefault, browseMax = 20, 200
	maxText                  = 4 << 10 // bytes kept of each key, value and header key and value
	maxHeaders               = 50      // headers kept per record
)

// tools call the REST API in-process, as the UI does: same clusters, connection pool, read-only guard,
// 30s timeout, errors and request log (paths without /api tell MCP calls apart).
type tools struct{ api http.Handler }

func (t tools) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	// drop the routing state of the MCP request: chi would route this one as POST /mcp's method
	ctx = context.WithValue(ctx, chi.RouteCtxKey, nil)
	r, err := http.NewRequestWithContext(ctx, method, path, rd)
	if err != nil {
		return err
	}
	w := httptest.NewRecorder()
	t.api.ServeHTTP(w, r)
	if w.Code >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(w.Body.Bytes(), &e)
		return errors.New(cmp.Or(e.Error, http.StatusText(w.Code)))
	}
	return json.Unmarshal(w.Body.Bytes(), out)
}

// cluster is the API path under a cluster; each part is escaped so it stays one path segment.
func cluster(id string, parts ...string) string {
	p := "/c/" + url.PathEscape(id)
	for _, s := range parts {
		p += "/" + url.PathEscape(s)
	}
	return p
}

type clusterIn struct {
	Cluster string `json:"cluster" jsonschema:"cluster id, from list_clusters"`
}

type listIn struct {
	Cluster string `json:"cluster" jsonschema:"cluster id, from list_clusters"`
	Filter  string `json:"filter,omitempty" jsonschema:"only names containing this text, ignoring case"`
	Limit   int    `json:"limit,omitempty" jsonschema:"how many to return, default 100, at most 1000"`
}

// page keeps the items whose name contains filter (ignoring case), at most limit of them.
func page[T any](items []T, name func(T) string, filter string, limit int) (shown []T, matched int) {
	limit = min(cmp.Or(max(limit, 0), listDefault), listMax)
	filter = strings.ToLower(filter)
	shown = []T{}
	for _, it := range items {
		if strings.Contains(strings.ToLower(name(it)), filter) {
			if matched++; len(shown) < limit {
				shown = append(shown, it)
			}
		}
	}
	return shown, matched
}

type config struct {
	Key       string  `json:"key"`
	Value     *string `json:"value"`
	Sensitive bool    `json:"sensitive,omitempty"`
	Source    string  `json:"source"`
	IsDefault bool    `json:"isDefault"`
}

// configs drops sensitive values and, unless all, the configs left at their default.
func configs(cs []config, all bool) []config {
	out := []config{}
	for _, c := range cs {
		if c.Sensitive {
			c.Value = nil
		}
		if all || !c.IsDefault {
			out = append(out, c)
		}
	}
	return out
}

type topicRow struct {
	Name              string `json:"name"`
	Partitions        int    `json:"partitions"`
	ReplicationFactor int    `json:"replicationFactor"`
	Internal          bool   `json:"internal,omitempty"`
	Messages          int64  `json:"messages"`
	UnderReplicated   int    `json:"underReplicated,omitempty"`
	Error             string `json:"error,omitempty"`
}

type groupRow struct {
	Group       string   `json:"group"`
	State       string   `json:"state"`
	Members     int      `json:"members"`
	Coordinator int32    `json:"coordinator"`
	Lag         int64    `json:"lag"`
	Topics      []string `json:"topics"`
	Error       string   `json:"error,omitempty"`
}

type browseIn struct {
	Cluster   string `json:"cluster" jsonschema:"cluster id, from list_clusters"`
	Topic     string `json:"topic"`
	From      string `json:"from,omitempty" jsonschema:"latest (default): the newest messages; earliest: the oldest; offset: from offset on; timestamp: from time on"`
	Partition *int32 `json:"partition,omitempty" jsonschema:"only this partition; every partition when left out"`
	Offset    int64  `json:"offset,omitempty" jsonschema:"with from=offset: the first offset to read, in each partition read"`
	Time      string `json:"time,omitempty" jsonschema:"with from=timestamp: RFC 3339 time to start from, e.g. 2026-11-03T09:00:00Z"`
	Filter    string `json:"filter,omitempty" jsonschema:"only messages whose key, value or a header (written key=value) contains this text, ignoring case"`
	Limit     int    `json:"limit,omitempty" jsonschema:"how many messages, default 20, at most 200"`
}

// clip cuts a text to maxText bytes, on a rune boundary; it reports whether it cut.
func clip(s *string) bool {
	if s == nil || len(*s) <= maxText {
		return false
	}
	*s = strings.ToValidUTF8((*s)[:maxText], "")
	return true
}

// record is a Kafka record with texts clipped to maxText.
type record struct {
	kafka.Record
	Clipped bool `json:"clipped,omitempty"` // key, value or headers were cut: size is the full record
}

func addTools(s *mcp.Server, t tools) {
	read := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "list_clusters", Annotations: read,
		Description: "The Kafka clusters saved in KafkaLens: id (the cluster argument of the other tools), name, bootstrap brokers, and readOnly (clusters guarded against writes, usually production)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			var cs []struct {
				ID       string   `json:"id"`
				Name     string   `json:"name"`
				Brokers  []string `json:"brokers"`
				ReadOnly bool     `json:"readOnly"`
			}
			if err := t.call(ctx, "GET", "/clusters", nil, &cs); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"clusters": cs}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "cluster_overview", Annotations: read,
		Description: "Brokers (id, host, rack, controller, partition leaders) and the counts of topics, partitions, under-replicated and offline partitions and consumer groups."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in clusterIn) (*mcp.CallToolResult, any, error) {
			var out json.RawMessage
			if err := t.call(ctx, "GET", cluster(in.Cluster, "overview"), nil, &out); err != nil {
				return nil, nil, err
			}
			return nil, out, nil
		})

	type brokerIn struct {
		Cluster string `json:"cluster" jsonschema:"cluster id, from list_clusters"`
		Broker  int32  `json:"broker" jsonschema:"broker id, from cluster_overview"`
		All     bool   `json:"all,omitempty" jsonschema:"also the configs left at their default (about 200)"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "broker_configs", Annotations: read,
		Description: "Configuration of a broker: the values that differ from the default, or every one with all=true. Sensitive values are never returned."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in brokerIn) (*mcp.CallToolResult, any, error) {
			var cs []config
			if err := t.call(ctx, "GET", cluster(in.Cluster, "brokers", fmt.Sprint(in.Broker), "configs"), nil, &cs); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"configs": configs(cs, in.All)}, nil
		})

	type topicsIn struct {
		listIn
		Internal bool `json:"internal,omitempty" jsonschema:"also internal topics such as __consumer_offsets"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "list_topics", Annotations: read,
		Description: "Topics sorted by name, with partitions, replication factor, approximate message count (end minus start offsets) and under-replicated partitions. total counts every topic, matched the ones passing the filter."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in topicsIn) (*mcp.CallToolResult, any, error) {
			var ts []topicRow
			if err := t.call(ctx, "GET", cluster(in.Cluster, "topics"), nil, &ts); err != nil {
				return nil, nil, err
			}
			total := len(ts)
			if !in.Internal {
				ts = slices.DeleteFunc(ts, func(r topicRow) bool { return r.Internal })
			}
			shown, matched := page(ts, func(r topicRow) string { return r.Name }, in.Filter, in.Limit)
			return nil, map[string]any{"total": total, "matched": matched, "topics": shown}, nil
		})

	type topicIn struct {
		Cluster    string `json:"cluster" jsonschema:"cluster id, from list_clusters"`
		Topic      string `json:"topic"`
		AllConfigs bool   `json:"allConfigs,omitempty" jsonschema:"also the configs left at their default"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "describe_topic", Annotations: read,
		Description: "A topic's partitions (leader, replicas, in-sync replicas, start and end offsets), its configs that differ from the default (all with allConfigs=true), and the consumer groups reading it with their lag on it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in topicIn) (*mcp.CallToolResult, any, error) {
			var d struct {
				Name        string          `json:"name"`
				Internal    bool            `json:"internal"`
				Partitions  json.RawMessage `json:"partitions"`
				Configs     []config        `json:"configs"`
				Groups      json.RawMessage `json:"groups"`
				GroupsError string          `json:"groupsError,omitempty"`
			}
			if err := t.call(ctx, "GET", cluster(in.Cluster, "topics", in.Topic), nil, &d); err != nil {
				return nil, nil, err
			}
			d.Configs = configs(d.Configs, in.AllConfigs)
			return nil, d, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "browse_messages", Annotations: read,
		Description: "Reads messages of a topic without consuming them (no consumer group, no commits): the newest by default. " +
			"Keys and values are text, or base64 when binary is true; texts longer than 4 KB and headers past 50 are cut (clipped). " +
			"partial means the scan stopped early (time or scan limit): narrow it with partition, from=offset or from=timestamp. " +
			"To page on, read from=offset with the last offset + 1 of a partition.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in browseIn) (*mcp.CallToolResult, any, error) {
		req := kafka.BrowseReq{From: cmp.Or(in.From, "latest"), Partition: -1, Offset: in.Offset, Filter: in.Filter,
			Limit: min(cmp.Or(max(in.Limit, 0), browseDefault), browseMax)}
		if in.Partition != nil {
			req.Partition = *in.Partition
		}
		switch req.From {
		case "latest", "earliest", "offset":
		case "timestamp":
			ts, err := time.Parse(time.RFC3339, in.Time)
			if err != nil {
				return nil, nil, fmt.Errorf("from=timestamp needs time in RFC 3339, e.g. 2026-11-03T09:00:00Z: %v", err)
			}
			req.Timestamp = ts.UnixMilli()
		default:
			return nil, nil, fmt.Errorf("from must be latest, earliest, offset or timestamp, not %q", req.From)
		}
		var res kafka.BrowseResp
		if err := t.call(ctx, "POST", cluster(in.Cluster, "topics", in.Topic, "browse"), req, &res); err != nil {
			return nil, nil, err
		}
		recs := make([]record, len(res.Records))
		for i, r := range res.Records {
			c := clip(r.Key)
			c = clip(r.Value) || c
			if len(r.Headers) > maxHeaders {
				r.Headers, c = r.Headers[:maxHeaders], true
			}
			for j := range r.Headers {
				c = clip(&r.Headers[j].Key) || c
				c = clip(&r.Headers[j].Value) || c
			}
			recs[i] = record{r, c}
		}
		return nil, map[string]any{"records": recs, "scanned": res.Scanned, "partial": res.Partial}, nil
	})

	mcp.AddTool(s, &mcp.Tool{Name: "list_consumer_groups", Annotations: read,
		Description: "Consumer groups sorted by name, with state, members, coordinator broker, total lag and the topics they commit offsets on. total counts every group, matched the ones passing the filter."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, any, error) {
			var gs []groupRow
			if err := t.call(ctx, "GET", cluster(in.Cluster, "groups"), nil, &gs); err != nil {
				return nil, nil, err
			}
			shown, matched := page(gs, func(g groupRow) string { return g.Group }, in.Filter, in.Limit)
			return nil, map[string]any{"total": len(gs), "matched": matched, "groups": shown}, nil
		})

	type groupIn struct {
		Cluster string `json:"cluster" jsonschema:"cluster id, from list_clusters"`
		Group   string `json:"group"`
		Topic   string `json:"topic,omitempty" jsonschema:"only the partitions of this topic"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "describe_consumer_group", Annotations: read,
		Description: "A consumer group's state, protocol, members (client id, host, assigned partitions) and, per partition, the committed offset, end offset, lag and owning member. At most 1000 partitions: use topic to narrow."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in groupIn) (*mcp.CallToolResult, any, error) {
			var g map[string]any
			if err := t.call(ctx, "GET", cluster(in.Cluster, "groups", in.Group), nil, &g); err != nil {
				return nil, nil, err
			}
			ps, _ := g["partitions"].([]any)
			if in.Topic != "" {
				ps = slices.DeleteFunc(ps, func(p any) bool { m, _ := p.(map[string]any); return m["topic"] != in.Topic })
			}
			g["partitionsTotal"] = len(ps)
			g["partitions"] = ps[:min(len(ps), listMax)]
			return nil, g, nil
		})
}
