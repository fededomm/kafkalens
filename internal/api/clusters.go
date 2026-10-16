package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"kafkalens/internal/kafka"
	"kafkalens/internal/store"
)

func (a *API) listClusters(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, c := range a.store.List() {
		out = append(out, c.Public())
	}
	reply(w, out, nil)
}

func validCluster(c store.Cluster) error {
	if c.Name == "" || len(c.Brokers) == 0 {
		return bad("name and at least one broker are required")
	}
	if _, err := kafka.ClientOpts(c); err != nil {
		return bad("%v", err)
	}
	return nil
}

// merge restores the stored secrets the browser never sees (see store.WithSecrets).
func (a *API) merge(c store.Cluster) (store.Cluster, error) {
	if c.ID == "" {
		return c, nil
	}
	stored, ok := a.store.Get(c.ID)
	if !ok {
		return c, httpErr{http.StatusNotFound, "cluster not found"}
	}
	c, err := store.WithSecrets(c, stored)
	if errors.Is(err, store.ErrPasswordNeeded) {
		err = bad("%v", err)
	}
	return c, err
}

func (a *API) saveCluster(w http.ResponseWriter, r *http.Request) {
	var c store.Cluster
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
	reply(w, saved.Public(), err)
}

func (a *API) deleteCluster(w http.ResponseWriter, r *http.Request) {
	a.pool.Drop(r.PathValue("id"))
	reply(w, nil, a.store.Delete(r.PathValue("id")))
}

func (a *API) testCluster(w http.ResponseWriter, r *http.Request) {
	var c store.Cluster
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
	opts, _ := kafka.ClientOpts(c)
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
