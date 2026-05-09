package metrics

import (
	"bytes"
	"strings"
	"testing"
)

func TestLabeledCounter(t *testing.T) {
	c := NewLabeledCounter("test_total", "test", "kind", "a", "b")
	c.Inc("a")
	c.Inc("a")
	c.Inc("b")
	c.Inc("unregistered") // dropped

	var buf bytes.Buffer
	c.Write(&buf)
	out := buf.String()

	for _, s := range []string{
		`test_total{kind="a"} 2`,
		`test_total{kind="b"} 1`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in output:\n%s", s, out)
		}
	}
	if strings.Contains(out, `kind="unregistered"`) {
		t.Errorf("unregistered label leaked into output:\n%s", out)
	}
}
