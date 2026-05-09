package metrics

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHistogram_BucketAndCountAndSum(t *testing.T) {
	h := NewLatencyHistogram("test_latency_seconds", "test")
	for _, d := range []time.Duration{
		2 * time.Millisecond,    // <= 5ms
		8 * time.Millisecond,    // <= 10ms
		20 * time.Millisecond,   // <= 25ms
		300 * time.Millisecond,  // <= 500ms
		3 * time.Second,         // > 2.5s; only +Inf
	} {
		h.Observe(d)
	}
	var buf bytes.Buffer
	h.Write(&buf)
	out := buf.String()

	wantSubstrs := []string{
		`test_latency_seconds_bucket{le="0.005"} 1`,
		`test_latency_seconds_bucket{le="0.01"} 2`,
		`test_latency_seconds_bucket{le="0.025"} 3`,
		`test_latency_seconds_bucket{le="0.5"} 4`,
		`test_latency_seconds_bucket{le="+Inf"} 5`,
		`test_latency_seconds_count 5`,
	}
	for _, s := range wantSubstrs {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in output:\n%s", s, out)
		}
	}
}

func TestHistogram_Time_Records(t *testing.T) {
	h := NewLatencyHistogram("test", "t")
	h.Time(func() {
		time.Sleep(2 * time.Millisecond)
	})
	var buf bytes.Buffer
	h.Write(&buf)
	if !strings.Contains(buf.String(), "test_count 1") {
		t.Fatalf("expected one observation, got:\n%s", buf.String())
	}
}

func TestHistogram_ConcurrentObserves(t *testing.T) {
	h := NewLatencyHistogram("test", "t")
	const writers = 8
	const each = 1000
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				h.Observe(time.Microsecond)
			}
		}()
	}
	wg.Wait()
	var buf bytes.Buffer
	h.Write(&buf)
	got := strings.Contains(buf.String(), "test_count 8000")
	if !got {
		t.Fatalf("expected count=8000 under concurrent load, got:\n%s", buf.String())
	}
}

func TestLabeledCounter(t *testing.T) {
	c := NewLabeledCounter("test_total", "test", "kind", "a", "b", "c")
	c.Inc("a")
	c.Inc("a")
	c.Inc("b")
	c.Inc("d") // not registered — silently dropped

	var buf bytes.Buffer
	c.Write(&buf)
	out := buf.String()

	for _, s := range []string{
		`test_total{kind="a"} 2`,
		`test_total{kind="b"} 1`,
		`test_total{kind="c"} 0`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in output:\n%s", s, out)
		}
	}
	if strings.Contains(out, `test_total{kind="d"}`) {
		t.Errorf("unregistered label leaked into output:\n%s", out)
	}
}
