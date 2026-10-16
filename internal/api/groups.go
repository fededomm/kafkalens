package api

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/twmb/franz-go/pkg/kadm"
)

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
