package views

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// A story's rates are counted of its columns' time, each a mean over the
// minutes it spans, as counting every request into its minute counts them:
// over stories of many idle gaps, their oldest merged at narrow widths so a
// fold hides requests, sealed and open alike, some requests sent at the very
// start of a minute, among them some a fold ends on.
func TestAStorysRatesAreItsRequestsCountedIntoTheirMinutes(t *testing.T) {
	onEdge, foldsOnEdge := 0, 0
	for seed := range uint64(40) {
		s := storyOfGaps(rand.New(rand.NewPCG(seed, 11)))
		for at := range s.Sent.All() {
			if at.Equal(at.Truncate(time.Minute)) {
				onEdge++
			}
		}
		for _, f := range s.Folds {
			if f.To.Equal(f.To.Truncate(time.Minute)) {
				foldsOnEdge++
			}
		}
		for _, width := range []int{12, 40, 73, 160, 216} {
			sc, ok := s.scaled(width)
			if !ok {
				continue
			}
			got, want := sc.rates(s.Sent), ratesCounted(sc, s.Sent)
			for i := range want {
				if math.Abs(got[i]-want[i]) > 1e-9*max(1, want[i]) {
					t.Fatalf("story %d at %d: column %d's rate is %v, want %v", seed, width, i, got[i], want[i])
				}
			}
		}
	}
	if onEdge == 0 || foldsOnEdge == 0 {
		t.Fatalf("%d requests sent at a minute's start, and %d folds ending on one; want some of each", onEdge, foldsOnEdge)
	}
}

// storyOfGaps is a story of bursts of requests drawn from r, some an idle
// hour or more apart, folded where they are, each fold ending on the first
// request after it, its oldest sealed and the rest open; now and then a
// request is sent at the start of a minute, as onMinute has it.
func storyOfGaps(r *rand.Rand) Story {
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	var times []time.Time
	var folds []Fold
	for range 1 + r.IntN(30) {
		if len(times) > 0 {
			gap := time.Duration(r.IntN(int(3 * time.Hour)))
			at = onMinute(r, at.Add(gap))
			if gap >= IdleHour {
				folds = append(folds, Fold{From: times[len(times)-1], To: at})
			}
		}
		for range 1 + r.IntN(200) {
			times = append(times, at)
			at = onMinute(r, at.Add(time.Duration(r.IntN(int(2*time.Minute)))))
		}
	}
	sealed := r.IntN(len(times) + 1)
	return Story{
		Start: times[0], End: times[len(times)-1], Folds: folds,
		Sent: SentTimes{sealed: upstreamTimes{}.with(times[:sealed]), open: times[sealed:]},
	}
}

// onMinute is t, or one time in four, as r draws it, the start of the
// minute after the one t falls in.
func onMinute(r *rand.Rand, t time.Time) time.Time {
	if r.IntN(4) > 0 {
		return t
	}
	return t.Truncate(time.Minute).Add(time.Minute)
}

// ratesCounted are the rates of the columns sc lays, of the times given, as
// counting each into the minute it falls in, but those a fold hides, and
// each column's mean over the minutes it spans, counts them.
func ratesCounted(sc storyScale, sent SentTimes) []float64 {
	perMinute := make([]int, sc.total+1)
	for t := range sent.All() {
		if u := sc.unfolded(t); !sc.hides(t) && u >= 0 && u <= sc.total {
			perMinute[u]++
		}
	}
	n := len(sc.edges) - 1
	rates := make([]float64, sc.drawn(n-1)+1)
	for b := range n {
		lo, hi := sc.edges[b], sc.edges[b+1]
		var sum float64
		for m := int(lo); float64(m) < hi; m++ {
			sum += float64(perMinute[m]) * (min(float64(m+1), hi) - max(float64(m), lo))
		}
		rates[sc.drawn(b)] = sum / (hi - lo)
	}
	return rates
}
