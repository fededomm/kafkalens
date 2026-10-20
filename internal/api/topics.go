package api

import (
	"fmt"
	"net/http"

	"github.com/twmb/franz-go/pkg/kadm"
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

func (a *API) listTopics(w http.ResponseWriter, r *http.Request, c cctx) (any, error) {
	ctx := r.Context()
	// Bypass the client's metadata cache: Lag (topic detail, groups) caches topics of other
	// groups we may not be authorized on, and a cached TOPIC_AUTHORIZATION_FAILED fails the whole list.
	ts, err := c.adm.ListTopicsWithInternal(kadm.WithAuthorizedOps(ctx))
	if err != nil {
		return nil, err
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
