package main

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

func clientOpts(c Cluster) ([]kgo.Opt, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(c.Brokers...),
		kgo.ClientID("kafkalens"),
		kgo.DialTimeout(5 * time.Second),
		kgo.RequestRetries(2),
	}
	if c.TLS {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.TLSSkipVerify}
		if c.TLSCA != "" {
			cfg.RootCAs = x509.NewCertPool()
			if !cfg.RootCAs.AppendCertsFromPEM([]byte(c.TLSCA)) {
				return nil, errors.New("TLS CA: no valid PEM certificate")
			}
		}
		if c.TLSCert != "" || c.TLSKey != "" {
			cert, err := tls.X509KeyPair([]byte(c.TLSCert), []byte(c.TLSKey))
			if err != nil {
				return nil, fmt.Errorf("TLS client certificate: %w", err)
			}
			cfg.Certificates = []tls.Certificate{cert}
		}
		opts = append(opts, kgo.DialTLSConfig(cfg))
	}
	switch c.SASLMechanism {
	case "":
	case "PLAIN":
		opts = append(opts, kgo.SASL(plain.Auth{User: c.SASLUser, Pass: c.SASLPassword}.AsMechanism()))
	case "SCRAM-SHA-256":
		opts = append(opts, kgo.SASL(scram.Auth{User: c.SASLUser, Pass: c.SASLPassword}.AsSha256Mechanism()))
	case "SCRAM-SHA-512":
		opts = append(opts, kgo.SASL(scram.Auth{User: c.SASLUser, Pass: c.SASLPassword}.AsSha512Mechanism()))
	default:
		return nil, fmt.Errorf("unsupported SASL mechanism %q", c.SASLMechanism)
	}
	return opts, nil
}

// Pool keeps one long-lived admin client per cluster.
type Pool struct {
	mu      sync.Mutex
	clients map[string]*kgo.Client
}

func NewPool() *Pool { return &Pool{clients: map[string]*kgo.Client{}} }

func (p *Pool) Admin(c Cluster) (*kadm.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cl, ok := p.clients[c.ID]; ok {
		return kadm.NewClient(cl), nil
	}
	opts, err := clientOpts(c)
	if err != nil {
		return nil, err
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	p.clients[c.ID] = cl
	return kadm.NewClient(cl), nil
}

func (p *Pool) Raw(c Cluster) (*kgo.Client, error) {
	if _, err := p.Admin(c); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clients[c.ID], nil
}

func (p *Pool) Drop(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cl, ok := p.clients[id]; ok {
		cl.Close()
		delete(p.clients, id)
	}
}

type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Record struct {
	Partition int32    `json:"partition"`
	Offset    int64    `json:"offset"`
	Timestamp int64    `json:"timestamp"`
	Key       *string  `json:"key"`
	Value     *string  `json:"value"`
	Binary    bool     `json:"binary"` // key/value are base64 when true
	Size      int      `json:"size"`
	Headers   []Header `json:"headers"`
}

func encode(b []byte, binary bool) *string {
	if b == nil {
		return nil
	}
	s := string(b)
	if binary {
		s = base64.StdEncoding.EncodeToString(b)
	}
	return &s
}

func toRecord(r *kgo.Record) Record {
	bin := !utf8.Valid(r.Key) || !utf8.Valid(r.Value)
	out := Record{
		Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp.UnixMilli(),
		Key: encode(r.Key, bin), Value: encode(r.Value, bin), Binary: bin,
		Size: len(r.Key) + len(r.Value), Headers: []Header{},
	}
	for _, h := range r.Headers {
		out.Headers = append(out.Headers, Header{h.Key, string(h.Value)})
	}
	return out
}

func (r Record) matches(q string) bool {
	if q == "" {
		return true
	}
	has := func(s *string) bool { return s != nil && strings.Contains(strings.ToLower(*s), q) }
	if has(r.Key) || has(r.Value) {
		return true
	}
	for _, h := range r.Headers {
		if strings.Contains(strings.ToLower(h.Key+"="+h.Value), q) {
			return true
		}
	}
	return false
}

type BrowseReq struct {
	From      string `json:"from"`      // earliest | latest | offset | timestamp
	Partition int32  `json:"partition"` // -1 = all
	Offset    int64  `json:"offset"`
	Timestamp int64  `json:"timestamp"` // unix millis
	Limit     int    `json:"limit"`
	Filter    string `json:"filter"`
}

type BrowseResp struct {
	Records []Record `json:"records"`
	Scanned int      `json:"scanned"`
	Partial bool     `json:"partial"` // stopped by the scan cap or timeout before reaching the end
}

const scanCap = 200_000

// Browse reads a bounded window of records without joining a consumer group.
func Browse(ctx context.Context, c Cluster, adm *kadm.Client, topic string, req BrowseReq) (BrowseResp, error) {
	if req.Limit <= 0 || req.Limit > 5000 {
		req.Limit = 500
	}
	starts, err := adm.ListStartOffsets(ctx, topic)
	if err != nil {
		return BrowseResp{}, err
	}
	ends, err := adm.ListEndOffsets(ctx, topic)
	if err != nil {
		return BrowseResp{}, err
	}
	var byTime kadm.ListedOffsets
	if req.From == "timestamp" {
		if byTime, err = adm.ListOffsetsAfterMilli(ctx, req.Timestamp, topic); err != nil {
			return BrowseResp{}, err
		}
	}

	// with a filter, "latest" searches a deeper window and keeps the newest matches
	window := int64(req.Limit)
	if req.Filter != "" {
		window = int64(scanCap / max(len(ends[topic]), 1))
	}
	from := map[int32]kgo.Offset{}
	stopAt := map[int32]int64{} // exclusive end per partition, snapshotted now
	for p, end := range ends[topic] {
		if req.Partition >= 0 && p != req.Partition {
			continue
		}
		start := starts[topic][p].Offset
		at := start
		switch req.From {
		case "latest":
			// last window per partition; the merged result is trimmed to Limit below
			at = max(start, end.Offset-window)
		case "offset":
			at = min(max(start, req.Offset), end.Offset)
		case "timestamp":
			if o, ok := byTime.Lookup(topic, p); ok && o.Offset >= 0 {
				at = o.Offset
			} else {
				at = end.Offset
			}
		}
		if at < end.Offset {
			from[p] = kgo.NewOffset().At(at)
			stopAt[p] = end.Offset
		}
	}
	resp := BrowseResp{Records: []Record{}}
	if len(from) == 0 {
		return resp, nil
	}

	opts, _ := clientOpts(c)
	cl, err := kgo.NewClient(append(opts, kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{topic: from}))...)
	if err != nil {
		return resp, err
	}
	defer cl.Close()

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	q := strings.ToLower(req.Filter)
	latest := req.From == "latest"
	for len(stopAt) > 0 && resp.Scanned < scanCap {
		fs := cl.PollFetches(ctx)
		if ctx.Err() != nil {
			resp.Partial = true
			break
		}
		fs.EachRecord(func(r *kgo.Record) {
			end, live := stopAt[r.Partition]
			if !live || r.Offset >= end {
				return
			}
			resp.Scanned++
			if r.Offset+1 >= end {
				delete(stopAt, r.Partition)
			}
			if rec := toRecord(r); rec.matches(q) && (latest || len(resp.Records) < req.Limit) {
				resp.Records = append(resp.Records, rec)
			}
		})
		if !latest && len(resp.Records) >= req.Limit {
			break
		}
	}
	if resp.Scanned >= scanCap {
		resp.Partial = true
	}
	if latest && len(resp.Records) > req.Limit {
		// keep the newest Limit records across partitions
		slices.SortStableFunc(resp.Records, func(a, b Record) int { return cmp.Compare(a.Timestamp, b.Timestamp) })
		resp.Records = resp.Records[len(resp.Records)-req.Limit:]
	}
	return resp, nil
}
