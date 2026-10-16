package main

import (
	"bytes"
	"math/rand/v2"
	"text/template"
	"time"

	"github.com/google/uuid"
)

// msgTemplates renders per-record key/value/header templates. The funcs close
// over the current record index, so call render(i) sequentially (not concurrently).
type msgTemplates struct {
	i     int
	funcs template.FuncMap
}

func newMsgTemplates() *msgTemplates {
	m := &msgTemplates{}
	m.funcs = template.FuncMap{
		"i":       func() int { return m.i },
		"uuid":    uuid.NewString,
		"now":     func() string { return time.Now().UTC().Format(time.RFC3339Nano) },
		"unixMs":  func() int64 { return time.Now().UnixMilli() },
		"randInt": func(lo, hi int) int { return lo + rand.IntN(max(hi-lo+1, 1)) },
		"pick":    func(xs ...any) any { return xs[rand.IntN(len(xs))] },
		"seq":     func(xs ...any) any { return xs[m.i%len(xs)] },
	}
	return m
}

// parse returns a renderer for s; nil input stays nil (e.g. null key).
func (m *msgTemplates) parse(s *string) (func() ([]byte, error), error) {
	if s == nil {
		return func() ([]byte, error) { return nil, nil }, nil
	}
	t, err := template.New("").Funcs(m.funcs).Option("missingkey=error").Parse(*s)
	if err != nil {
		return nil, err
	}
	return func() ([]byte, error) {
		var b bytes.Buffer
		err := t.Execute(&b, nil)
		return b.Bytes(), err
	}, nil
}
