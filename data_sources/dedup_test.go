package data_sources

import (
	"testing"
	"time"
)

func TestDedup(t *testing.T) {
	d := newDedup()
	t0 := time.Unix(1_000_000, 0)
	cases := []struct {
		mac, data string
		dt        time.Duration
		want      bool
	}{
		{"A", "x1", 0, false},                       // first copy
		{"A", "x1", 50 * time.Millisecond, true},    // channel copy
		{"B", "x1", 60 * time.Millisecond, false},   // other sensor, same bytes
		{"A", "x1", 900 * time.Millisecond, true},   // re-advertised, same measurement
		{"A", "x2", time.Second, false},             // new measurement
		{"A", "x1", 1100 * time.Millisecond, false}, // changed back: new reference
		{"A", "x1", 7 * time.Second, false},         // stable reading after window
	}
	for i, c := range cases {
		if got := d.duplicate(c.mac, c.data, t0.Add(c.dt)); got != c.want {
			t.Errorf("case %d: duplicate(%s, %s, +%v) = %v, want %v", i, c.mac, c.data, c.dt, got, c.want)
		}
	}
}
