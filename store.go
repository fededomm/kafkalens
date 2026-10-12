package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

type Cluster struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Color         string   `json:"color"`
	Brokers       []string `json:"brokers"`
	SASLMechanism string   `json:"saslMechanism"` // "", PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
	SASLUser      string   `json:"saslUser"`
	// ponytail: plaintext in a 0600 file, move to the OS keyring if shared machines matter.
	SASLPassword  string `json:"saslPassword,omitempty"`
	TLS           bool   `json:"tls"`
	TLSSkipVerify bool   `json:"tlsSkipVerify"`
	TLSCA         string `json:"tlsCa"`            // PEM, empty = system roots
	TLSCert       string `json:"tlsCert"`          // PEM client certificate for mTLS
	TLSKey        string `json:"tlsKey,omitempty"` // PEM client key, secret like the password
	ReadOnly      bool   `json:"readOnly"`
}

// public strips the secret before sending a cluster to the browser.
func (c Cluster) public() map[string]any {
	b, _ := json.Marshal(c)
	var m map[string]any
	json.Unmarshal(b, &m)
	delete(m, "saslPassword")
	delete(m, "tlsKey")
	m["hasPassword"] = c.SASLPassword != ""
	m["hasTlsKey"] = c.TLSKey != ""
	return m
}

// withSecrets fills empty secrets in c from the stored copy. The password is
// reused only when it would go to the same brokers the same way, so an edited
// request can't replay it to another host or downgrade it to PLAIN/plaintext.
func withSecrets(c, stored Cluster) (Cluster, error) {
	if c.TLSKey == "" {
		c.TLSKey = stored.TLSKey
	}
	if c.SASLPassword != "" || c.SASLMechanism == "" || stored.SASLPassword == "" {
		return c, nil
	}
	same := slices.Equal(c.Brokers, stored.Brokers) && c.SASLMechanism == stored.SASLMechanism &&
		c.SASLUser == stored.SASLUser && c.TLS == stored.TLS && c.TLSSkipVerify == stored.TLSSkipVerify && c.TLSCA == stored.TLSCA
	if !same {
		return c, bad("brokers, authentication or TLS changed: re-enter the password")
	}
	c.SASLPassword = stored.SASLPassword
	return c, nil
}

type Store struct {
	mu       sync.Mutex
	path     string
	clusters []Cluster
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	os.Chmod(path, 0o600) // tighten a hand-made file; it holds credentials
	return s, json.Unmarshal(b, &s.clusters)
}

func (s *Store) List() []Cluster {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.clusters)
}

func (s *Store) Get(id string) (Cluster, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.clusters, func(c Cluster) bool { return c.ID == id })
	if i < 0 {
		return Cluster{}, false
	}
	return s.clusters[i], true
}

// Save inserts or replaces c. Callers merge secrets first with withSecrets.
func (s *Store) Save(c Cluster) (Cluster, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == "" {
		b := make([]byte, 6)
		rand.Read(b)
		c.ID = hex.EncodeToString(b)
		s.clusters = append(s.clusters, c)
	} else {
		i := slices.IndexFunc(s.clusters, func(x Cluster) bool { return x.ID == c.ID })
		if i < 0 {
			return c, errors.New("cluster not found")
		}
		s.clusters[i] = c
	}
	return c, s.flush()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clusters = slices.DeleteFunc(s.clusters, func(c Cluster) bool { return c.ID == id })
	return s.flush()
}

func (s *Store) flush() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s.clusters, "", "  ")
	// temp file (created 0600) + rename: a crash never leaves a half-written file
	f, err := os.CreateTemp(filepath.Dir(s.path), ".clusters-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
