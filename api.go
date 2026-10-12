package main

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

type API struct {
	store *Store
	pool  *Pool
}

func (a *API) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/clusters", a.listClusters)
	mux.HandleFunc("POST /api/clusters", a.saveCluster)
	mux.HandleFunc("PUT /api/clusters/{id}", a.saveCluster)
	mux.HandleFunc("DELETE /api/clusters/{id}", a.deleteCluster)
	mux.HandleFunc("POST /api/clusters/test", a.testCluster)

	mux.HandleFunc("GET /api/c/{id}/overview", a.withAdmin(a.overview))
	mux.HandleFunc("GET /api/c/{id}/brokers/{broker}/configs", a.withAdmin(a.brokerConfigs))
	mux.HandleFunc("GET /api/c/{id}/topics", a.withAdmin(a.listTopics))
	mux.HandleFunc("POST /api/c/{id}/topics", a.withAdmin(a.write(a.createTopic)))
	mux.HandleFunc("GET /api/c/{id}/topics/{topic}", a.withAdmin(a.topicDetail))
	mux.HandleFunc("DELETE /api/c/{id}/topics/{topic}", a.withAdmin(a.write(a.deleteTopic)))
	mux.HandleFunc("PUT /api/c/{id}/topics/{topic}/configs", a.withAdmin(a.write(a.alterTopicConfigs)))
	mux.HandleFunc("POST /api/c/{id}/topics/{topic}/partitions", a.withAdmin(a.write(a.addPartitions)))
	mux.HandleFunc("POST /api/c/{id}/topics/{topic}/browse", a.withAdmin(a.browse))
	mux.HandleFunc("GET /api/c/{id}/topics/{topic}/tail", a.tail)
	mux.HandleFunc("POST /api/c/{id}/topics/{topic}/produce", a.withAdmin(a.write(a.produce)))
	mux.HandleFunc("GET /api/c/{id}/groups", a.withAdmin(a.listGroups))
	mux.HandleFunc("GET /api/c/{id}/groups/{group}", a.withAdmin(a.groupDetail))
	mux.HandleFunc("POST /api/c/{id}/groups/{group}/reset", a.withAdmin(a.write(a.resetGroup)))
	mux.HandleFunc("DELETE /api/c/{id}/groups/{group}", a.withAdmin(a.write(a.deleteGroup)))
}

// ---- helpers ----

type httpErr struct {
	code int
	msg  string
}

func (e httpErr) Error() string { return e.msg }

func reply(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		code := http.StatusBadGateway // most failures come from the Kafka cluster
		if he, ok := errors.AsType[httpErr](err); ok {
			code = he.code
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if v == nil {
		v = map[string]bool{"ok": true}
	}
	json.NewEncoder(w).Encode(v)
}

func bad(format string, args ...any) error {
	return httpErr{http.StatusBadRequest, fmt.Sprintf(format, args...)}
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return bad("invalid JSON body: %v", err)
	}
	return nil
}

type cctx struct {
	cluster Cluster
	adm     *kadm.Client
}

type handler func(w http.ResponseWriter, r *http.Request, c cctx) (any, error)

func (a *API) withAdmin(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := a.store.Get(r.PathValue("id"))
		if !ok {
			reply(w, nil, httpErr{http.StatusNotFound, "cluster not found"})
			return
		}
		adm, err := a.pool.Admin(cl)
		if err != nil {
			reply(w, nil, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		v, err := h(w, r.WithContext(ctx), cctx{cl, adm})
		reply(w, v, err)
	}
}

// write blocks mutating calls on clusters marked read-only (e.g. production).
func (a *API) write(h handler) handler {
	return func(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
		if c.cluster.ReadOnly {
			return nil, httpErr{http.StatusForbidden, "cluster is read-only"}
		}
		return h(w, r, c)
	}
}

// ---- clusters ----

func (a *API) listClusters(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, c := range a.store.List() {
		out = append(out, c.public())
	}
	reply(w, out, nil)
}

func validCluster(c Cluster) error {
	if c.Name == "" || len(c.Brokers) == 0 {
		return bad("name and at least one broker are required")
	}
	if _, err := clientOpts(c); err != nil {
		return bad("%v", err)
	}
	return nil
}

// merge restores the stored secrets the browser never sees (see withSecrets).
func (a *API) merge(c Cluster) (Cluster, error) {
	if c.ID == "" {
		return c, nil
	}
	stored, ok := a.store.Get(c.ID)
	if !ok {
		return c, httpErr{http.StatusNotFound, "cluster not found"}
	}
	return withSecrets(c, stored)
}

func (a *API) saveCluster(w http.ResponseWriter, r *http.Request) {
	var c Cluster
	if err := decode(r, &c); err != nil {
		reply(w, nil, err)
		return
	}
	c.ID = r.PathValue("id")
	c, err := a.merge(c)
	if err == nil {
		err = validCluster(c)
	}
	if err != nil {
		reply(w, nil, err)
		return
	}
	saved, err := a.store.Save(c)
	a.pool.Drop(saved.ID)
	reply(w, saved.public(), err)
}

func (a *API) deleteCluster(w http.ResponseWriter, r *http.Request) {
	a.pool.Drop(r.PathValue("id"))
	reply(w, nil, a.store.Delete(r.PathValue("id")))
}

func (a *API) testCluster(w http.ResponseWriter, r *http.Request) {
	var c Cluster
	if err := decode(r, &c); err != nil {
		reply(w, nil, err)
		return
	}
	c, err := a.merge(c)
	if err == nil {
		err = validCluster(c)
	}
	if err != nil {
		reply(w, nil, err)
		return
	}
	opts, _ := clientOpts(c)
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		reply(w, nil, bad("%v", err))
		return
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	start := time.Now()
	m, err := kadm.NewClient(cl).BrokerMetadata(ctx)
	if err != nil {
		reply(w, nil, err)
		return
	}
	reply(w, map[string]any{"clusterId": m.Cluster, "brokers": len(m.Brokers), "latencyMs": time.Since(start).Milliseconds()}, nil)
}

// ---- overview / brokers ----

func (a *API) overview(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	m, err := c.adm.Metadata(r.Context())
	if err != nil {
		return nil, err
	}
	groups, _ := c.adm.ListGroups(r.Context())
	partitions, underReplicated, offline := 0, 0, 0
	topics := 0
	for _, t := range m.Topics {
		if !t.IsInternal {
			topics++
		}
		for _, p := range t.Partitions {
			partitions++
			if len(p.ISR) < len(p.Replicas) {
				underReplicated++
			}
			if p.Leader < 0 {
				offline++
			}
		}
	}
	leaders := map[int32]int{}
	for _, t := range m.Topics {
		for _, p := range t.Partitions {
			leaders[p.Leader]++
		}
	}
	type broker struct {
		ID         int32   `json:"id"`
		Host       string  `json:"host"`
		Port       int32   `json:"port"`
		Rack       *string `json:"rack"`
		Controller bool    `json:"controller"`
		Leaders    int     `json:"leaders"`
	}
	brokers := []broker{}
	for _, b := range m.Brokers {
		brokers = append(brokers, broker{b.NodeID, b.Host, b.Port, b.Rack, b.NodeID == m.Controller, leaders[b.NodeID]})
	}
	return map[string]any{
		"clusterId": m.Cluster, "controller": m.Controller, "brokers": brokers,
		"topics": topics, "partitions": partitions, "underReplicated": underReplicated,
		"offlinePartitions": offline, "groups": len(groups),
	}, nil
}

type configEntry struct {
	Key       string  `json:"key"`
	Value     *string `json:"value"`
	Sensitive bool    `json:"sensitive"`
	Source    string  `json:"source"`
	IsDefault bool    `json:"isDefault"`
}

func configsOut(rc kadm.ResourceConfigs, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	if len(rc) == 0 {
		return []configEntry{}, nil
	}
	if rc[0].Err != nil {
		return nil, rc[0].Err
	}
	out := []configEntry{}
	for _, cf := range rc[0].Configs {
		src := cf.Source.String()
		out = append(out, configEntry{cf.Key, cf.Value, cf.Sensitive, src, src == "DEFAULT_CONFIG"})
	}
	slices.SortFunc(out, func(a, b configEntry) int { return cmp.Compare(a.Key, b.Key) })
	return out, nil
}

func (a *API) brokerConfigs(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	id, err := strconv.Atoi(r.PathValue("broker"))
	if err != nil {
		return nil, bad("invalid broker id")
	}
	return configsOut(c.adm.DescribeBrokerConfigs(r.Context(), int32(id)))
}

// ---- topics ----

type topicRow struct {
	Name              string `json:"name"`
	Partitions        int    `json:"partitions"`
	ReplicationFactor int    `json:"replicationFactor"`
	Internal          bool   `json:"internal"`
	Messages          int64  `json:"messages"` // end - start, approximate with compaction/transactions
	UnderReplicated   int    `json:"underReplicated"`
	Error             string `json:"error,omitempty"`
}

func (a *API) listTopics(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	ctx := r.Context()
	ts, err := c.adm.ListTopicsWithInternal(ctx)
	if err != nil {
		return nil, err
	}
	starts, _ := c.adm.ListStartOffsets(ctx)
	ends, _ := c.adm.ListEndOffsets(ctx)
	out := []topicRow{}
	for _, t := range ts.Sorted() {
		row := topicRow{Name: t.Topic, Partitions: len(t.Partitions), ReplicationFactor: t.Partitions.NumReplicas(), Internal: t.IsInternal}
		if t.Err != nil {
			row.Error = t.Err.Error()
		}
		for _, p := range t.Partitions {
			if len(p.ISR) < len(p.Replicas) {
				row.UnderReplicated++
			}
			s, ok1 := starts.Lookup(t.Topic, p.Partition)
			e, ok2 := ends.Lookup(t.Topic, p.Partition)
			if ok1 && ok2 && s.Err == nil && e.Err == nil {
				row.Messages += e.Offset - s.Offset
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (a *API) topicDetail(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	ctx, topic := r.Context(), r.PathValue("topic")
	ts, err := c.adm.ListTopicsWithInternal(ctx, topic)
	if err != nil {
		return nil, err
	}
	t, ok := ts[topic]
	if !ok || t.Err != nil {
		return nil, httpErr{http.StatusNotFound, "topic not found"}
	}
	starts, _ := c.adm.ListStartOffsets(ctx, topic)
	ends, _ := c.adm.ListEndOffsets(ctx, topic)
	type part struct {
		ID       int32   `json:"id"`
		Leader   int32   `json:"leader"`
		Replicas []int32 `json:"replicas"`
		ISR      []int32 `json:"isr"`
		Start    int64   `json:"start"`
		End      int64   `json:"end"`
	}
	parts := []part{}
	for _, p := range t.Partitions.Sorted() {
		s, _ := starts.Lookup(topic, p.Partition)
		e, _ := ends.Lookup(topic, p.Partition)
		parts = append(parts, part{p.Partition, p.Leader, p.Replicas, p.ISR, s.Offset, e.Offset})
	}
	configs, err := configsOut(c.adm.DescribeTopicConfigs(ctx, topic))
	if err != nil {
		return nil, err
	}
	// consumer groups reading this topic
	groups := []map[string]any{}
	if lg, err := c.adm.ListGroups(ctx); err == nil {
		if lags, err := c.adm.Lag(ctx, lg.Groups()...); err == nil {
			for _, g := range lags.Sorted() {
				if tl, ok := g.Lag.TotalByTopic()[topic]; ok {
					groups = append(groups, map[string]any{"group": g.Group, "state": g.State, "lag": tl.Lag})
				}
			}
		}
	}
	return map[string]any{"name": topic, "internal": t.IsInternal, "id": t.ID.String(), "partitions": parts, "configs": configs, "groups": groups}, nil
}

func (a *API) createTopic(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	var req struct {
		Name              string             `json:"name"`
		Partitions        int32              `json:"partitions"`
		ReplicationFactor int16              `json:"replicationFactor"`
		Configs           map[string]*string `json:"configs"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, bad("topic name is required")
	}
	if req.Partitions == 0 {
		req.Partitions = -1 // broker default
	}
	if req.ReplicationFactor == 0 {
		req.ReplicationFactor = -1
	}
	res, err := c.adm.CreateTopic(r.Context(), req.Partitions, req.ReplicationFactor, req.Configs, req.Name)
	if err == nil {
		err = res.Err
	}
	return nil, err
}

func (a *API) deleteTopic(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	res, err := c.adm.DeleteTopic(r.Context(), r.PathValue("topic"))
	if err == nil {
		err = res.Err
	}
	return nil, err
}

func (a *API) alterTopicConfigs(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	var req map[string]*string // nil value = reset to default
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	var alters []kadm.AlterConfig
	for k, v := range req {
		op := kadm.SetConfig
		if v == nil {
			op = kadm.DeleteConfig
		}
		alters = append(alters, kadm.AlterConfig{Op: op, Name: k, Value: v})
	}
	res, err := c.adm.AlterTopicConfigs(r.Context(), alters, r.PathValue("topic"))
	if err != nil {
		return nil, err
	}
	for _, x := range res {
		if x.Err != nil {
			return nil, fmt.Errorf("%v %s", x.Err, x.ErrMessage)
		}
	}
	return nil, nil
}

func (a *API) addPartitions(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	var req struct {
		Total int `json:"total"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	res, err := c.adm.UpdatePartitions(r.Context(), req.Total, r.PathValue("topic"))
	if err == nil {
		err = res.Error()
	}
	return nil, err
}

// ---- messages ----

func (a *API) browse(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	req := BrowseReq{Partition: -1}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return Browse(r.Context(), c.cluster, c.adm, r.PathValue("topic"), req)
}

// tailSlots caps concurrent live tails: each one opens its own Kafka client.
var tailSlots = make(chan struct{}, 16)

func acquireTail() bool {
	select {
	case tailSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseTail() { <-tailSlots }

// openTail creates a client positioned at the end of topic; no group is joined.
func openTail(cl Cluster, topic string) (*kgo.Client, error) {
	opts, err := clientOpts(cl)
	if err != nil {
		return nil, err
	}
	return kgo.NewClient(append(opts, kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))...)
}

// pumpTail hands each polled batch of new records to emit until ctx ends or emit fails.
func pumpTail(ctx context.Context, k *kgo.Client, emit func([]Record) error) error {
	for {
		fs := k.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil
		}
		var batch []Record
		fs.EachRecord(func(rec *kgo.Record) { batch = append(batch, toRecord(rec)) })
		if len(batch) == 0 {
			continue
		}
		if err := emit(batch); err != nil {
			return err
		}
	}
}

// tail streams new records as Server-Sent Events until the browser disconnects.
func (a *API) tail(w http.ResponseWriter, r *http.Request) {
	cl, ok := a.store.Get(r.PathValue("id"))
	if !ok {
		reply(w, nil, httpErr{http.StatusNotFound, "cluster not found"})
		return
	}
	if !acquireTail() {
		reply(w, nil, httpErr{http.StatusTooManyRequests, "too many live tails open"})
		return
	}
	defer releaseTail()
	k, err := openTail(cl, r.PathValue("topic"))
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
	pumpTail(r.Context(), k, func(batch []Record) error {
		for _, rec := range batch {
			b, _ := json.Marshal(rec)
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		return rc.Flush() // client gone or writer can't stream
	})
}

func (a *API) produce(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	var req struct {
		Partition int32    `json:"partition"` // -1 = partitioner decides
		Key       *string  `json:"key"`
		Value     *string  `json:"value"`
		Base64    bool     `json:"base64"`
		Headers   []Header `json:"headers"`
		Count     int      `json:"count"`
	}
	req.Partition = -1
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	raw := func(s *string) ([]byte, error) {
		if s == nil {
			return nil, nil
		}
		if req.Base64 {
			return base64.StdEncoding.DecodeString(*s)
		}
		return []byte(*s), nil
	}
	key, err := raw(req.Key)
	if err != nil {
		return nil, bad("key: %v", err)
	}
	val, err := raw(req.Value)
	if err != nil {
		return nil, bad("value: %v", err)
	}
	req.Count = min(max(req.Count, 1), 10_000)

	opts, _ := clientOpts(c.cluster)
	if req.Partition >= 0 {
		opts = append(opts, kgo.RecordPartitioner(kgo.ManualPartitioner()))
	}
	k, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	var hs []kgo.RecordHeader
	for _, h := range req.Headers {
		if h.Key != "" {
			hs = append(hs, kgo.RecordHeader{Key: h.Key, Value: []byte(h.Value)})
		}
	}
	recs := make([]*kgo.Record, req.Count)
	for i := range recs {
		recs[i] = &kgo.Record{Topic: r.PathValue("topic"), Partition: req.Partition, Key: key, Value: val, Headers: hs}
	}
	res := k.ProduceSync(r.Context(), recs...)
	if err := res.FirstErr(); err != nil {
		return nil, err
	}
	last := res[len(res)-1].Record
	return map[string]any{"count": len(res), "partition": last.Partition, "offset": last.Offset}, nil
}

// ---- consumer groups ----

func (a *API) listGroups(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	ctx := r.Context()
	lg, err := c.adm.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	if len(lg) == 0 {
		return out, nil
	}
	lags, err := c.adm.Lag(ctx, lg.Groups()...)
	if err != nil {
		return nil, err
	}
	for _, g := range lags.Sorted() {
		topics := []string{}
		for t := range g.Lag {
			topics = append(topics, t)
		}
		slices.Sort(topics)
		row := map[string]any{
			"group": g.Group, "state": g.State, "protocolType": lg[g.Group].ProtocolType,
			"members": len(g.Members), "coordinator": g.Coordinator.NodeID, "lag": g.Lag.Total(), "topics": topics,
		}
		if e := g.Error(); e != nil {
			row["error"] = e.Error()
		}
		out = append(out, row)
	}
	return out, nil
}

func (a *API) groupDetail(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	name := r.PathValue("group")
	lags, err := c.adm.Lag(r.Context(), name)
	if err != nil {
		return nil, err
	}
	g, ok := lags[name]
	if !ok {
		return nil, httpErr{http.StatusNotFound, "group not found"}
	}
	if e := g.Error(); e != nil {
		return nil, e
	}
	type member struct {
		ID         string   `json:"id"`
		InstanceID *string  `json:"instanceId"`
		ClientID   string   `json:"clientId"`
		Host       string   `json:"host"`
		Assigned   []string `json:"assigned"`
	}
	members := []member{}
	for _, m := range g.Members {
		mb := member{m.MemberID, m.InstanceID, m.ClientID, m.ClientHost, []string{}}
		if as, ok := m.Assigned.AsConsumer(); ok {
			for _, t := range as.Topics {
				for _, p := range t.Partitions {
					mb.Assigned = append(mb.Assigned, fmt.Sprintf("%s-%d", t.Topic, p))
				}
			}
		}
		members = append(members, mb)
	}
	type plag struct {
		Topic     string  `json:"topic"`
		Partition int32   `json:"partition"`
		Committed int64   `json:"committed"`
		End       int64   `json:"end"`
		Lag       int64   `json:"lag"`
		Member    *string `json:"member"`
	}
	parts := []plag{}
	for _, l := range g.Lag.Sorted() {
		p := plag{Topic: l.Topic, Partition: l.Partition, Committed: l.Commit.At, End: l.End.Offset, Lag: l.Lag}
		if l.Member != nil {
			p.Member = &l.Member.ClientID
		}
		parts = append(parts, p)
	}
	return map[string]any{
		"group": g.Group, "state": g.State, "protocol": g.Protocol, "protocolType": g.ProtocolType,
		"coordinator": g.Coordinator.NodeID, "members": members, "partitions": parts, "lag": g.Lag.Total(),
	}, nil
}

func (a *API) resetGroup(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	var req struct {
		Topic     string `json:"topic"`
		To        string `json:"to"` // earliest | latest | timestamp | offset
		Timestamp int64  `json:"timestamp"`
		Offset    int64  `json:"offset"`
		Partition *int32 `json:"partition"` // nil = all partitions
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	ctx, group := r.Context(), r.PathValue("group")
	dg, err := c.adm.DescribeGroups(ctx, group)
	if err != nil {
		return nil, err
	}
	if st := dg[group].State; st != "Empty" && st != "Dead" && st != "" {
		return nil, httpErr{http.StatusConflict, "stop all consumers first: group is " + st}
	}
	var listed kadm.ListedOffsets
	switch req.To {
	case "earliest":
		listed, err = c.adm.ListStartOffsets(ctx, req.Topic)
	case "latest", "offset":
		listed, err = c.adm.ListEndOffsets(ctx, req.Topic)
	case "timestamp":
		listed, err = c.adm.ListOffsetsAfterMilli(ctx, req.Timestamp, req.Topic)
	default:
		return nil, bad("unknown reset target %q", req.To)
	}
	if err != nil {
		return nil, err
	}
	offs := kadm.Offsets{req.Topic: {}}
	for p, lo := range listed[req.Topic] {
		if req.Partition != nil && *req.Partition != p {
			continue
		}
		if lo.Err != nil {
			return nil, lo.Err
		}
		at := lo.Offset
		if req.To == "offset" {
			at = min(req.Offset, lo.Offset)
		}
		offs[req.Topic][p] = kadm.Offset{Topic: req.Topic, Partition: p, At: at, LeaderEpoch: -1}
	}
	if len(offs[req.Topic]) == 0 {
		return nil, bad("no partitions matched")
	}
	res, err := c.adm.CommitOffsets(ctx, group, offs)
	if err == nil {
		err = res.Error()
	}
	return nil, err
}

func (a *API) deleteGroup(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	res, err := c.adm.DeleteGroup(r.Context(), r.PathValue("group"))
	if err == nil {
		err = res.Err
	}
	return nil, err
}
