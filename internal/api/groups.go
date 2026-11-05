package api

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/twmb/franz-go/pkg/kadm"
)

func (a *API) listGroups(r *http.Request, c cctx) (any, error) {
	ctx := r.Context()
	lg, err := c.adm.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	if r.URL.Query().Has("names") { // command palette: no lag
		names := append([]string{}, lg.Groups()...)
		slices.Sort(names)
		return names, nil
	}
	out := []map[string]any{}
	if len(lg) == 0 {
		return out, nil
	}
	lags, err := groupLags(ctx, c.adm, lg.Groups()...)
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
			"group": g.Group, "state": g.State, "members": len(g.Members),
			"coordinator": g.Coordinator.NodeID, "lag": g.Lag.Total(), "topics": topics,
		}
		if e := g.Error(); e != nil {
			row["error"] = e.Error()
		}
		out = append(out, row)
	}
	return out, nil
}

// groupLags is kadm's Lag without its all-or-nothing auth errors: there one topic we may not describe
// (or one group we may not read) fails every group, here it only leaves that topic without lag.
func groupLags(ctx context.Context, adm *kadm.Client, groups ...string) (kadm.DescribedGroupLags, error) {
	described, derr := adm.DescribeGroups(ctx, groups...)
	if len(described) == 0 && derr != nil {
		return nil, derr
	}
	fetched := adm.FetchManyOffsets(ctx, groups...)
	ps := described.AssignedPartitions()
	ps.MergeTopics(described.JoinTopics())
	ps.Merge(fetched.CommittedPartitions())
	// naming a topic we may not describe fails the whole offsets listing: keep only the visible ones
	// (all-topics metadata leaves the others out, uncached like listTopics)
	visible, err := adm.ListTopicsWithInternal(kadm.WithAuthorizedOps(ctx))
	if err != nil {
		return nil, err
	}
	topics := slices.DeleteFunc(ps.Topics(), func(t string) bool { return !visible.Has(t) })
	var starts, ends kadm.ListedOffsets
	if len(topics) > 0 { // no topics would list them all
		starts, _ = adm.ListStartOffsets(ctx, topics...) // failures show up as partitions without lag
		ends, _ = adm.ListEndOffsets(ctx, topics...)
	}
	lags := kadm.DescribedGroupLags{}
	for _, name := range groups {
		g, ok := described[name]
		if !ok { // its describe shard failed
			g = kadm.DescribedGroup{Group: name, Err: cmp.Or(derr, errors.New("not described"))}
		}
		f := fetched[name]
		lags[name] = kadm.DescribedGroupLag{
			Group: name, Coordinator: g.Coordinator, State: g.State, ProtocolType: g.ProtocolType, Protocol: g.Protocol,
			Members: g.Members, DescribeErr: g.Err, FetchErr: f.Err,
			Lag: kadm.CalculateGroupLagWithStartOffsets(g, f.Fetched, starts, ends),
		}
	}
	return lags, nil
}

func (a *API) groupDetail(r *http.Request, c cctx) (any, error) {
	name := pathParam(r, "group")
	lags, err := groupLags(r.Context(), c.adm, name)
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
		"group": g.Group, "state": g.State, "protocol": g.Protocol,
		"coordinator": g.Coordinator.NodeID, "members": members, "partitions": parts, "lag": g.Lag.Total(),
	}, nil
}

func (a *API) resetGroup(r *http.Request, c cctx) (any, error) {
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
	if (req.To == "timestamp" && req.Timestamp <= 0) || (req.To == "offset" && req.Offset < 0) {
		return nil, bad("invalid %s to reset to", req.To)
	}
	ctx, group := r.Context(), pathParam(r, "group")
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
	var starts kadm.ListedOffsets // an explicit offset is clamped to [log start, end], like Browse
	if req.To == "offset" {
		if starts, err = c.adm.ListStartOffsets(ctx, req.Topic); err != nil {
			return nil, err
		}
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
			at = min(max(req.Offset, starts[req.Topic][p].Offset), lo.Offset)
		}
		offs[req.Topic][p] = kadm.Offset{Topic: req.Topic, Partition: p, At: at, LeaderEpoch: -1}
	}
	if len(offs[req.Topic]) == 0 {
		return nil, bad("no partitions matched")
	}
	return nil, c.adm.CommitAllOffsets(ctx, group, offs)
}

func (a *API) deleteGroup(r *http.Request, c cctx) (any, error) {
	_, err := c.adm.DeleteGroup(r.Context(), pathParam(r, "group"))
	return nil, err
}
