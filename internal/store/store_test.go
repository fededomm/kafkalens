package store

import "testing"

func TestWithSecrets(t *testing.T) {
	stored := Cluster{ID: "x", Brokers: []string{"k:9093"}, SASLMechanism: "SCRAM-SHA-512", SASLUser: "u", SASLPassword: "pw", TLS: true, TLSKey: "key"}

	same := stored
	same.SASLPassword, same.TLSKey, same.Name = "", "", "renamed"
	if c, err := WithSecrets(same, stored); err != nil || c.SASLPassword != "pw" || c.TLSKey != "key" {
		t.Fatalf("unchanged target must keep secrets: %+v %v", c, err)
	}

	for name, mut := range map[string]func(*Cluster){
		"brokers":   func(c *Cluster) { c.Brokers = []string{"evil:9092"} },
		"mechanism": func(c *Cluster) { c.SASLMechanism = "PLAIN" },
		"user":      func(c *Cluster) { c.SASLUser = "other" },
		"tls off":   func(c *Cluster) { c.TLS = false },
		"skip":      func(c *Cluster) { c.TLSSkipVerify = true },
		"ca":        func(c *Cluster) { c.TLSCA = "pem" },
	} {
		c := same
		mut(&c)
		if _, err := WithSecrets(c, stored); err == nil {
			t.Errorf("%s changed: stored password must not be reused", name)
		}
	}

	typed := same
	typed.Brokers, typed.SASLPassword = []string{"new:9093"}, "new"
	if c, err := WithSecrets(typed, stored); err != nil || c.SASLPassword != "new" {
		t.Fatalf("explicit password must win: %+v %v", c, err)
	}
}
