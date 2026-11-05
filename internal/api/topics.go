package api

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
)

type topicRow struct {
	Name              string `json:"name"`
	Partitions        int    `json:"partitions"`
	ReplicationFactor int    `json:"replicationFactor"`
	Internal          bool   `json:"internal"`
	Messages          int64  `json:"messages"` // end - start, approximate with compaction/transactions
	UnderReplicated   int    `json:"underReplicated"`
	Error             string `json:"error,omitempty"`
}

func (a *API) listTopics(r *http.Request, c cctx) (any, error) {
	ctx := r.Context()
	// Bypass the client's metadata cache: a cached TOPIC_AUTHORIZATION_FAILED (an earlier request
	// naming a topic we may not describe) fails the whole list.
	ts, err := c.adm.ListTopicsWithInternal(kadm.WithAuthorizedOps(ctx))
	if err != nil {
		return nil, err
	}
	if r.URL.Query().Has("names") { // command palette: no offsets
		return ts.Names(), nil
	}
	starts, _ := c.adm.ListStartOffsets(ctx, ts.Names()...)
	ends, _ := c.adm.ListEndOffsets(ctx, ts.Names()...)
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

func (a *API) topicDetail(r *http.Request, c cctx) (any, error) {
	ctx, topic := r.Context(), r.PathValue("topic")
	ts, err := c.adm.ListTopicsWithInternal(ctx, topic)
	if err != nil {
		return nil, err
	}
	t, ok := ts[topic]
	if !ok || errors.Is(t.Err, kerr.UnknownTopicOrPartition) {
		return nil, httpErr{http.StatusNotFound, "topic not found"}
	}
	if t.Err != nil {
		return nil, t.Err
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
	// consumer groups reading this topic, lag from this topic's offsets only (a cluster-wide Lag
	// would list the offsets of every topic every group reads)
	groups := []map[string]any{}
	lg, gerr := c.adm.ListGroups(ctx) // a partial failure still returns the groups it listed
	if len(lg) > 0 {
		dg, err := c.adm.DescribeGroups(ctx, lg.Groups()...)
		fetched := c.adm.FetchManyOffsets(ctx, lg.Groups()...)
		gerr = cmp.Or(gerr, err)
		for _, g := range dg.Sorted() { // g.Err unchecked: KIP-848 groups fail the classic describe, their commits count
			f := fetched[g.Group]
			gerr = cmp.Or(gerr, f.Err)
			if tl, ok := kadm.CalculateGroupLagWithStartOffsets(g, f.Fetched, starts, ends).TotalByTopic()[topic]; ok {
				groups = append(groups, map[string]any{"group": g.Group, "state": g.State, "lag": tl.Lag})
			}
		}
	}
	out := map[string]any{"name": topic, "internal": t.IsInternal, "partitions": parts, "configs": configs, "groups": groups}
	if gerr != nil {
		out["groupsError"] = gerr.Error()
	}
	return out, nil
}

func (a *API) createTopic(r *http.Request, c cctx) (any, error) {
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
	_, err := c.adm.CreateTopic(r.Context(), req.Partitions, req.ReplicationFactor, req.Configs, req.Name)
	return nil, err
}

func (a *API) deleteTopic(r *http.Request, c cctx) (any, error) {
	_, err := c.adm.DeleteTopic(r.Context(), r.PathValue("topic"))
	return nil, err
}

// truncateTopic deletes every record up to the current end of each partition; the topic and its config stay.
func (a *API) truncateTopic(r *http.Request, c cctx) (any, error) {
	ends, err := c.adm.ListEndOffsets(r.Context(), r.PathValue("topic"))
	if err == nil {
		err = ends.Error()
	}
	if err != nil {
		return nil, err
	}
	res, err := c.adm.DeleteRecords(r.Context(), ends.Offsets())
	if err == nil {
		err = res.Error()
	}
	return nil, err
}

// truncateTopics empties many topics (a selection, a folder) in one pass and replies with the error of each topic it failed on.
func (a *API) truncateTopics(r *http.Request, c cctx) (any, error) {
	var req struct {
		Topics []string `json:"topics"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if len(req.Topics) == 0 {
		return nil, bad("no topics given")
	}
	ends, err := c.adm.ListEndOffsets(r.Context(), req.Topics...)
	if err != nil {
		return nil, err
	}
	failed := map[string]string{}
	ends.Each(func(o kadm.ListedOffset) {
		if o.Err != nil {
			failed[o.Topic] = o.Err.Error()
		}
	})
	for t := range failed {
		delete(ends, t) // a topic with an unlisted partition is left whole, not emptied halfway
	}
	res, err := c.adm.DeleteRecords(r.Context(), ends.Offsets())
	if err != nil {
		return nil, err
	}
	res.Each(func(d kadm.DeleteRecordsResponse) {
		if d.Err != nil {
			failed[d.Topic] = d.Err.Error()
		}
	})
	return failed, nil
}

func (a *API) alterTopicConfigs(r *http.Request, c cctx) (any, error) {
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

func (a *API) addPartitions(r *http.Request, c cctx) (any, error) {
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
