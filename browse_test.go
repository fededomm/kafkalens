package main

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Needs a broker on localhost:9092 (`make kafka`); skipped otherwise.
func TestBrowse(t *testing.T) {
	if c, err := net.DialTimeout("tcp", "localhost:9092", time.Second); err != nil {
		t.Skip("no local kafka")
	} else {
		c.Close()
	}
	ctx := context.Background()
	cl := Cluster{ID: "t", Brokers: []string{"localhost:9092"}}
	k, _ := kgo.NewClient(kgo.SeedBrokers(cl.Brokers...))
	defer k.Close()
	adm := kadm.NewClient(k)
	topic := fmt.Sprintf("kafkalens-test-%d", time.Now().UnixNano())
	if _, err := adm.CreateTopic(ctx, 2, 1, nil, topic); err != nil {
		t.Fatal(err)
	}
	defer adm.DeleteTopic(ctx, topic)

	var recs []*kgo.Record
	for i := range 20 {
		recs = append(recs, &kgo.Record{Topic: topic, Partition: int32(i % 2), Value: fmt.Appendf(nil, "msg-%02d", i)})
	}
	k2, _ := kgo.NewClient(kgo.SeedBrokers(cl.Brokers...), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	defer k2.Close()
	if err := k2.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}

	get := func(req BrowseReq) []Record {
		t.Helper()
		r, err := Browse(ctx, cl, adm, topic, req)
		if err != nil {
			t.Fatal(err)
		}
		return r.Records
	}
	if n := len(get(BrowseReq{From: "earliest", Partition: -1, Limit: 5})); n != 5 {
		t.Errorf("earliest limit: got %d, want 5", n)
	}
	if n := len(get(BrowseReq{From: "latest", Partition: -1, Limit: 100})); n != 20 {
		t.Errorf("latest all: got %d, want 20", n)
	}
	if r := get(BrowseReq{From: "latest", Partition: -1, Limit: 3}); len(r) != 3 {
		t.Errorf("latest trim: got %d, want 3", len(r))
	}
	if r := get(BrowseReq{From: "offset", Partition: 1, Offset: 8, Limit: 100}); len(r) != 2 || r[0].Offset != 8 {
		t.Errorf("offset: got %+v", r)
	}
	// filter must reach past the last-N window of "latest"
	if r := get(BrowseReq{From: "latest", Partition: -1, Limit: 2, Filter: "MSG-00"}); len(r) != 1 || *r[0].Value != "msg-00" {
		t.Errorf("filter: got %+v", r)
	}
	if r := get(BrowseReq{From: "timestamp", Partition: -1, Timestamp: time.Now().Add(time.Hour).UnixMilli(), Limit: 10}); len(r) != 0 {
		t.Errorf("future timestamp: got %d records", len(r))
	}
}

func TestBinaryDetection(t *testing.T) {
	r := toRecord(&kgo.Record{Key: []byte("k"), Value: []byte{0xff, 0x00}})
	if !r.Binary || *r.Value != "/wA=" || *r.Key != "aw==" {
		t.Errorf("got %+v", r)
	}
}
