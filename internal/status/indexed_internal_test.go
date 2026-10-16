package status

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/readings"
)

// The lows find the last value before a place, and the first after one,
// below a bound, as a scan of the list does: at every place, for bounds
// below, among and above its values, over lists of every length to one past
// a power of two, those spent beyond any use or short of any among them.
func TestLowsFindTheLastAndFirstBelowABoundAsAScanDoes(t *testing.T) {
	values := []float64{0.4, math.Inf(1), 0.9, 0.2, math.Inf(-1), 0.95, 0.9, 1, 0.3}
	bounds := []float64{0.1, 0.25, 0.9, 0.95, math.Inf(1)}
	for n := range len(values) + 1 {
		l := lowsOf(n, func(k int) float64 { return values[k] })
		for k := range n + 1 {
			for _, bound := range bounds {
				last, first := scanBelow(values[:n], k, bound)
				if got := l.lastBelow(k, bound); got != last {
					t.Errorf("of %v, the last before %d below %v is at %d, want %d", values[:n], k, bound, got, last)
				}
				if got := l.firstBelow(k, bound); got != first {
					t.Errorf("of %v, the first after %d below %v is at %d, want %d", values[:n], k, bound, got, first)
				}
			}
		}
	}
}

// scanBelow is the place of the last value before k below bound, and of the
// first after it, each -1 where there's none, as a scan finds them.
func scanBelow(values []float64, k int, bound float64) (last, first int) {
	last, first = -1, -1
	for i, v := range values {
		switch {
		case v >= bound:
		case i < k:
			last = i
		case i > k && first < 0:
			first = i
		}
	}
	return last, first
}

// Readings given out of order are indexed in order, those read at the one
// moment as given, so each window's last reading by a moment is the one a
// scan of them in order finds.
func TestReadingsGivenOutOfOrderAreIndexedInOrder(t *testing.T) {
	at := time.Date(2026, 10, 14, 13, 0, 0, 0, time.UTC)
	read := []readings.Reading{
		{At: at.Add(2 * time.Hour), Account: "work", Key: "5h", Utilization: 0.3},
		{At: at, Account: "work", Key: "5h", Utilization: 0.1},
		{At: at.Add(time.Hour), Account: "work", Key: "5h", Utilization: 0.2},
		{At: at.Add(time.Hour), Account: "work", Key: "5h", Utilization: 0.25},
		{At: at.Add(time.Hour), Account: "side", Key: "5h", Utilization: 0.9},
	}
	indexed := Index(slices.Clone(read))
	for _, tt := range []struct {
		t    time.Time
		want float64
	}{{at, 0.1}, {at.Add(time.Hour), 0.25}, {at.Add(90 * time.Minute), 0.25}, {at.Add(2 * time.Hour), 0.3}} {
		if w, ok := indexed.windowBy("work", "5h", tt.t); !ok || w.Utilization != tt.want {
			t.Errorf("work's session by %s reads %v, %t, want %v", tt.t.Format("15:04"), w.Utilization, ok, tt.want)
		}
	}
	if _, ok := indexed.windowBy("work", "5h", at.Add(-time.Minute)); ok {
		t.Errorf("work's session reads before its first reading, want it not read by then")
	}
}
