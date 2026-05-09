// Package metrics carries a small, dependency-free Prometheus
// exposition for the gateway.
//
// Mirrors services/hsm-broker/internal/metrics/metrics.go in shape
// — the duplication is intentional given how few metrics each
// service emits today. If the surface grows, both copies move
// behind a shared module.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"sync/atomic"
)

// LabeledCounter is a counter with one variable label dimension.
// Used for per-path / per-status response counters on the gateway.
type LabeledCounter struct {
	name   string
	help   string
	label  string
	values map[string]*atomic.Uint64
}

// NewLabeledCounter pre-registers a fixed set of label values so
// the Inc() hot path is lock-free. Unknown values passed to Inc()
// are silently dropped.
func NewLabeledCounter(name, help, labelName string, labelValues ...string) *LabeledCounter {
	c := &LabeledCounter{
		name:   name,
		help:   help,
		label:  labelName,
		values: make(map[string]*atomic.Uint64, len(labelValues)),
	}
	for _, v := range labelValues {
		c.values[v] = new(atomic.Uint64)
	}
	return c
}

// Inc increments the counter for the given label value.
func (c *LabeledCounter) Inc(labelValue string) {
	if v, ok := c.values[labelValue]; ok {
		v.Add(1)
	}
}

// Write emits the counter in Prometheus text format.
func (c *LabeledCounter) Write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n", c.name, c.help)
	fmt.Fprintf(w, "# TYPE %s counter\n", c.name)
	keys := make([]string, 0, len(c.values))
	for k := range c.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s{%s=%q} %d\n", c.name, c.label, k, c.values[k].Load())
	}
}
