// Package metrics carries a small, dependency-free Prometheus
// exposition for the HSM broker.
//
// We deliberately don't pull in github.com/prometheus/client_golang
// for the few metrics we currently emit — adding a transitive
// dependency tree the size of client_golang for two gauges and one
// histogram is not the right cost-benefit. Rolled by hand: the
// concurrency model is per-bucket atomics; the exposition format is
// vanilla text/plain version=0.0.4.
//
// When Aether grows enough metrics that the surface area justifies
// it (custom collectors, exemplars, native histograms), this package
// is the obvious place to swap in client_golang behind a stable
// internal API.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"sync/atomic"
	"time"
)

// LatencyHistogram is a Prometheus-compatible histogram with a fixed
// bucket layout suitable for HSM-call latencies. Buckets are
// inclusive upper bounds in seconds; the +Inf bucket is implicit.
type LatencyHistogram struct {
	name string
	help string

	bounds  []float64
	buckets []atomic.Uint64
	count   atomic.Uint64
	sumNs   atomic.Uint64 // wall-clock nanoseconds, summed
}

// NewLatencyHistogram constructs a histogram with the standard HSM
// latency buckets: 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s.
// The 250ms boundary corresponds to the AetherHSMSignLatencyP99 SLO
// in deployments/observability/.
func NewLatencyHistogram(name, help string) *LatencyHistogram {
	bounds := []float64{0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.000, 2.500}
	return &LatencyHistogram{
		name:    name,
		help:    help,
		bounds:  bounds,
		buckets: make([]atomic.Uint64, len(bounds)),
	}
}

// Observe records one sample.
func (h *LatencyHistogram) Observe(d time.Duration) {
	secs := d.Seconds()
	for i, b := range h.bounds {
		if secs <= b {
			h.buckets[i].Add(1)
		}
	}
	h.count.Add(1)
	h.sumNs.Add(uint64(d.Nanoseconds()))
}

// Time is a convenience that times f and observes its duration.
func (h *LatencyHistogram) Time(f func()) {
	t := time.Now()
	f()
	h.Observe(time.Since(t))
}

// Write emits the histogram in Prometheus text format.
func (h *LatencyHistogram) Write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n", h.name, h.help)
	fmt.Fprintf(w, "# TYPE %s histogram\n", h.name)
	for i, b := range h.bounds {
		fmt.Fprintf(w, "%s_bucket{le=\"%g\"} %d\n", h.name, b, h.buckets[i].Load())
	}
	// +Inf bucket is the total count.
	count := h.count.Load()
	fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", h.name, count)
	sumSecs := float64(h.sumNs.Load()) / 1e9
	fmt.Fprintf(w, "%s_sum %g\n", h.name, sumSecs)
	fmt.Fprintf(w, "%s_count %d\n", h.name, count)
}

// LabeledCounter is a counter with one variable label dimension.
// Used for the gateway's per-path / per-status 401 counter.
type LabeledCounter struct {
	name   string
	help   string
	label  string
	values map[string]*atomic.Uint64

	// values is read-only after construction by callers that pre-
	// register label values; the gateway's use case has a fixed,
	// small label set so a sync.Map is overkill.
}

// NewLabeledCounter constructs a counter. Callers must call With()
// for every label value they intend to emit, before observations
// start. This keeps the hot path lock-free.
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

// Inc increments the counter for the given label value. Unknown
// values are silently dropped (the design assumes all values were
// pre-registered).
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
