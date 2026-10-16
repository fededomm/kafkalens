package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/twmb/franz-go/pkg/kadm"

	"kafkalens/internal/kafka"
	"kafkalens/internal/store"
)

type API struct {
	store *store.Store
	pool  *kafka.Pool
}

// New returns the REST API router, meant to be mounted under /api.
func New(st *store.Store, pool *kafka.Pool) http.Handler {
	a := &API{store: st, pool: pool}
	r := chi.NewRouter()
	r.Use(logRequests)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { reply(w, nil, httpErr{http.StatusNotFound, "not found"}) })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		reply(w, nil, httpErr{http.StatusMethodNotAllowed, "method not allowed"})
	})

	r.Route("/clusters", func(r chi.Router) {
		r.Get("/", a.listClusters)
		r.Post("/", a.saveCluster)
		r.Post("/test", a.testCluster)
		r.Put("/{id}", a.saveCluster)
		r.Delete("/{id}", a.deleteCluster)
	})

	r.Route("/c/{id}", func(r chi.Router) {
		// long-lived SSE stream with its own client: outside withCluster's 30s timeout
		r.Get("/topics/{topic}/tail", a.tail)

		r.Group(func(r chi.Router) {
			r.Use(a.withCluster)
			r.Get("/overview", handle(a.overview))
			r.Get("/brokers/{broker}/configs", handle(a.brokerConfigs))
			r.Get("/topics", handle(a.listTopics))
			r.Get("/topics/{topic}", handle(a.topicDetail))
			r.Post("/topics/{topic}/browse", handle(a.browse))
			r.Get("/groups", handle(a.listGroups))
			r.Get("/groups/{group}", handle(a.groupDetail))

			r.Group(func(r chi.Router) {
				r.Use(writable)
				r.Post("/topics", handle(a.createTopic))
				r.Delete("/topics/{topic}", handle(a.deleteTopic))
				r.Put("/topics/{topic}/configs", handle(a.alterTopicConfigs))
				r.Post("/topics/{topic}/partitions", handle(a.addPartitions))
				r.Post("/topics/{topic}/produce", handle(a.produce))
				r.Post("/groups/{group}/reset", handle(a.resetGroup))
				r.Delete("/groups/{group}", handle(a.deleteGroup))
			})
		})
	})
	return r
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
		if sw, ok := w.(*statusWriter); ok {
			sw.err = err // for the request log line
		}
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

// cctx is the cluster a /c/{id} request targets, set by withCluster.
type cctx struct {
	cluster store.Cluster
	adm     *kadm.Client
}

type cctxKey struct{}

func clusterOf(r *http.Request) cctx { c, _ := r.Context().Value(cctxKey{}).(cctx); return c }

type handler func(w http.ResponseWriter, r *http.Request, c cctx) (any, error)

// handle adapts a handler that returns its result to http, replying with JSON.
func handle(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := h(w, r, clusterOf(r))
		reply(w, v, err)
	}
}

// withCluster resolves {id} to the stored cluster and its admin client, and bounds the request to 30s.
func (a *API) withCluster(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, cctxKey{}, cctx{cl, adm})))
	})
}

// writable blocks mutating calls on clusters marked read-only (e.g. production).
func writable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if clusterOf(r).cluster.ReadOnly {
			reply(w, nil, httpErr{http.StatusForbidden, "cluster is read-only"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
