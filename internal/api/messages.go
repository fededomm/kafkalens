package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/twmb/franz-go/pkg/kgo"

	"kafkalens/internal/kafka"
)

func (a *API) browse(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	req := kafka.BrowseReq{Partition: -1}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return kafka.Browse(r.Context(), c.cluster, c.adm, r.PathValue("topic"), req)
}

// tail streams new records as Server-Sent Events until the browser disconnects.
func (a *API) tail(w http.ResponseWriter, r *http.Request) {
	cl, ok := a.store.Get(r.PathValue("id"))
	if !ok {
		reply(w, nil, httpErr{http.StatusNotFound, "cluster not found"})
		return
	}
	if !kafka.AcquireTail() {
		reply(w, nil, httpErr{http.StatusTooManyRequests, "too many live tails open"})
		return
	}
	defer kafka.ReleaseTail()
	k, err := kafka.OpenTail(cl, r.PathValue("topic"))
	if err != nil {
		reply(w, nil, err)
		return
	}
	defer k.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	fmt.Fprint(w, ": connected\n\n")
	if rc.Flush() != nil {
		return
	}
	kafka.PumpTail(r.Context(), k, func(batch []kafka.Record) error {
		for _, rec := range batch {
			b, _ := json.Marshal(rec)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		return rc.Flush() // client gone or writer can't stream
	})
}

func (a *API) produce(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	var req struct {
		Partition int32          `json:"partition"` // -1 = partitioner decides
		Key       *string        `json:"key"`
		Value     *string        `json:"value"`
		Base64    bool           `json:"base64"`
		Template  bool           `json:"template"` // render key/value/header values per record with text/template
		Headers   []kafka.Header `json:"headers"`
		Count     int            `json:"count"`
	}
	req.Partition = -1
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	req.Count = min(max(req.Count, 1), 10_000)
	if req.Template && req.Base64 {
		return nil, bad("template and base64 are mutually exclusive")
	}

	// each field becomes a per-record renderer; without templating it returns the same bytes every time
	tm := newMsgTemplates()
	field := func(name string, s *string) (func() ([]byte, error), error) {
		if req.Template {
			f, err := tm.parse(s)
			if err != nil {
				return nil, bad("%s: %v", name, err)
			}
			return f, nil
		}
		var b []byte
		if s != nil {
			b = []byte(*s)
			if req.Base64 {
				var err error
				if b, err = base64.StdEncoding.DecodeString(*s); err != nil {
					return nil, bad("%s: %v", name, err)
				}
			}
		}
		return func() ([]byte, error) { return b, nil }, nil
	}
	key, err := field("key", req.Key)
	if err != nil {
		return nil, err
	}
	val, err := field("value", req.Value)
	if err != nil {
		return nil, err
	}
	type hdr struct {
		key string
		val func() ([]byte, error)
	}
	var hs []hdr
	for _, h := range req.Headers {
		if h.Key == "" {
			continue
		}
		v := h.Value
		var f func() ([]byte, error)
		if req.Template {
			if f, err = tm.parse(&v); err != nil {
				return nil, bad("header %s: %v", h.Key, err)
			}
		} else {
			f = func() ([]byte, error) { return []byte(v), nil }
		}
		hs = append(hs, hdr{h.Key, f})
	}

	recs := make([]*kgo.Record, req.Count)
	for i := range recs {
		tm.i = i
		rec := &kgo.Record{Topic: r.PathValue("topic"), Partition: req.Partition}
		if rec.Key, err = key(); err != nil {
			return nil, bad("key: %v", err)
		}
		if rec.Value, err = val(); err != nil {
			return nil, bad("value: %v", err)
		}
		for _, h := range hs {
			b, err := h.val()
			if err != nil {
				return nil, bad("header %s: %v", h.key, err)
			}
			rec.Headers = append(rec.Headers, kgo.RecordHeader{Key: h.key, Value: b})
		}
		recs[i] = rec
	}

	opts, _ := kafka.ClientOpts(c.cluster)
	if req.Partition >= 0 {
		opts = append(opts, kgo.RecordPartitioner(kgo.ManualPartitioner()))
	}
	k, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	res := k.ProduceSync(r.Context(), recs...)
	if err := res.FirstErr(); err != nil {
		return nil, err
	}
	last := res[len(res)-1].Record
	return map[string]any{"count": len(res), "partition": last.Partition, "offset": last.Offset}, nil
}
