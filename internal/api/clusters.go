package api

import (
	"context"
	"net/http"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"kafkalens/internal/kafka"
	"kafkalens/internal/store"
)

func (a *API) listClusters(r *http.Request, _ cctx) (any, error) {
	out := []map[string]any{}
	for _, c := range a.store.List() {
		out = append(out, c.Public())
	}
	return out, nil
}

// prepare restores the stored secrets the browser never sees (see store.WithSecrets) and validates the cluster.
func (a *API) prepare(c store.Cluster) (store.Cluster, error) {
	if c.ID != "" {
		stored, ok := a.store.Get(c.ID)
		if !ok {
			return c, httpErr{http.StatusNotFound, "cluster not found"}
		}
		var err error
		if c, err = store.WithSecrets(c, stored); err != nil {
			return c, bad("%v", err)
		}
	}
	if c.Name == "" || len(c.Brokers) == 0 {
		return c, bad("name and at least one broker are required")
	}
	if _, err := kafka.ClientOpts(c); err != nil {
		return c, bad("%v", err)
	}
	return c, nil
}

func (a *API) saveCluster(r *http.Request, _ cctx) (any, error) {
	var c store.Cluster
	if err := decode(r, &c); err != nil {
		return nil, err
	}
	c.ID = r.PathValue("id")
	c, err := a.prepare(c)
	if err != nil {
		return nil, err
	}
	saved, err := a.store.Save(c)
	a.pool.Drop(saved.ID)
	return saved.Public(), err
}

func (a *API) deleteCluster(r *http.Request, _ cctx) (any, error) {
	a.pool.Drop(r.PathValue("id"))
	return nil, a.store.Delete(r.PathValue("id"))
}

func (a *API) testCluster(r *http.Request, _ cctx) (any, error) {
	var c store.Cluster
	if err := decode(r, &c); err != nil {
		return nil, err
	}
	c, err := a.prepare(c)
	if err != nil {
		return nil, err
	}
	opts, _ := kafka.ClientOpts(c)
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, bad("%v", err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	start := time.Now()
	m, err := kadm.NewClient(cl).BrokerMetadata(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"clusterId": m.Cluster, "brokers": len(m.Brokers), "latencyMs": time.Since(start).Milliseconds()}, nil
}
