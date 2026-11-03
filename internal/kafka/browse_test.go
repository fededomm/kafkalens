package kafka

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"kafkalens/internal/store"
)

// localTopic creates a throwaway topic on a broker at localhost:9092 (`make kafka`); skips the test without one.
func localTopic(t *testing.T, partitions int32) (store.Cluster, *kadm.Client, string) {
	if c, err := net.DialTimeout("tcp", "localhost:9092", time.Second); err != nil {
		t.Skip("no local kafka")
	} else {
		c.Close()
	}
	cl := store.Cluster{ID: "t", Brokers: []string{"localhost:9092"}}
	k, _ := kgo.NewClient(kgo.SeedBrokers(cl.Brokers...))
	t.Cleanup(k.Close)
	adm := kadm.NewClient(k)
	topic := fmt.Sprintf("kafkalens-test-%d", time.Now().UnixNano())
	if _, err := adm.CreateTopic(context.Background(), partitions, 1, nil, topic); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { adm.DeleteTopic(context.Background(), topic) })
	return cl, adm, topic
}

func TestBrowse(t *testing.T) {
	cl, adm, topic := localTopic(t, 2)
	ctx := context.Background()
	var recs []*kgo.Record
	for i := range 20 {
		recs = append(recs, &kgo.Record{Topic: topic, Partition: int32(i % 2), Value: fmt.Appendf(nil, "msg-%02d", i)})
	}
	k2, _ := kgo.NewClient(kgo.SeedBrokers(cl.Brokers...), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	defer k2.Close()
	if err := k2.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}

	// a missing topic is an error, not an empty page (callers pass free-form names: MCP)
	if _, err := Browse(ctx, cl, adm, topic+"-missing", BrowseReq{From: "latest", Partition: -1}); err == nil {
		t.Error("browse of a missing topic succeeded")
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

// A transactional producer ends the log with a commit marker (a control record): Browse must
// take it as the partition's end instead of waiting for the timeout and answering partial.
func TestBrowseTransactional(t *testing.T) {
	cl, adm, topic := localTopic(t, 1)
	ctx := context.Background()
	k, err := kgo.NewClient(kgo.SeedBrokers(cl.Brokers...), kgo.TransactionalID(topic))
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if err := k.BeginTransaction(); err != nil {
		t.Fatal(err)
	}
	if err := k.ProduceSync(ctx, &kgo.Record{Topic: topic, Value: []byte("a")}, &kgo.Record{Topic: topic, Value: []byte("b")}).FirstErr(); err != nil {
		t.Fatal(err)
	}
	if err := k.EndTransaction(ctx, kgo.TryCommit); err != nil {
		t.Fatal(err)
	}
	for i := 0; ; i++ { // the coordinator writes the commit marker (offset 2) after EndTransaction returns
		if ends, _ := adm.ListEndOffsets(ctx, topic); ends[topic][0].Offset == 3 {
			break
		} else if i == 50 {
			t.Fatal("no commit marker")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if r, err := Browse(ctx, cl, adm, topic, BrowseReq{From: "latest", Partition: -1, Limit: 10}); err != nil || len(r.Records) != 2 || r.Partial {
		t.Errorf("got %+v %v", r, err)
	}
}

func TestBinaryDetection(t *testing.T) {
	r := toRecord(&kgo.Record{Key: []byte("k"), Value: []byte{0xff, 0x00},
		Headers: []kgo.RecordHeader{{Key: "t", Value: []byte("x")}, {Key: "b", Value: []byte{0xff}}}})
	if !r.Binary || *r.Value != "/wA=" || *r.Key != "aw==" {
		t.Errorf("got %+v", r)
	}
	if h := r.Headers; h[0] != (Header{"t", "x", false}) || h[1] != (Header{"b", "/w==", true}) {
		t.Errorf("headers: got %+v", h)
	}
}
