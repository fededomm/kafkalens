package api

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"

	"github.com/twmb/franz-go/pkg/kadm"
)

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
