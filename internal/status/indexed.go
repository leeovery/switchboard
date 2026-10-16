package status

import (
	"math"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
)

// Readings are readings of the readings history as Then judges how an
// account stood by them, indexed once, by Index: each account's, a series
// each window, oldest first, so a judgement finds a window's last reading by
// a moment, and the run of them that shows a hold, with a search rather
// than a scan of them all.
type Readings struct {
	of map[string][]series
}

// Index is the readings given, oldest first, as the history reads them,
// indexed as Readings has them, by their places among them: they're held,
// not copied, so must not change after.
func Index(read []readings.Reading) Readings {
	type window struct{ account, key string }
	found := make(map[window]int)
	var windows []window
	var places [][]int
	for i, r := range read {
		w := window{r.Account, r.Key}
		j, ok := found[w]
		if !ok {
			j = len(windows)
			found[w] = j
			windows, places = append(windows, w), append(places, nil)
		}
		places[j] = append(places[j], i)
	}
	indexed := Readings{of: make(map[string][]series)}
	for j, w := range windows {
		indexed.of[w.account] = append(indexed.of[w.account], seriesOf(w.key, read, places[j]))
	}
	return indexed
}

// window is the series of the account with the given id's readings of the
// window with the given key, empty where there are none.
func (r Readings) window(id, key string) series {
	i := slices.IndexFunc(r.of[id], func(s series) bool { return s.key == key })
	if i < 0 {
		return series{}
	}
	return r.of[id][i]
}

// windowBy is the window with the given key of the account with the given
// id as its last reading by t has it, reporting false where none was read by
// then.
func (r Readings) windowBy(id, key string, t time.Time) (quota.Window, bool) {
	s := r.window(id, key)
	k := s.lastBy(t)
	if k < 0 {
		return quota.Window{}, false
	}
	return s.reading(k).Window(), true
}

// Resets are the resets of the window with the given key that the readings
// of the account with the given id give: each as read, once for a run of
// readings reading the one; and each between two readings, as
// score.ResetBetween tells it, as a day's summary keeps it: a reset made by
// hand among them, at the reading that shows it, as a hold on the window
// lifts then, as Then has it.
func (r Readings) Resets(id, key string) []time.Time {
	s := r.window(id, key)
	var resets []time.Time
	var was quota.Window
	for k := range s.places {
		read := s.reading(k)
		w := read.Window()
		if !w.ResetsAt.IsZero() && !w.ResetsAt.Equal(was.ResetsAt) {
			resets = append(resets, w.ResetsAt)
		}
		if at, ok := score.ResetBetween(was, w, read.At); ok {
			resets = append(resets, at)
		}
		was = w
	}
	return resets
}

// series is an account's readings of a window, oldest first, indexed: by
// their places among the readings indexed; and how they hold the account
// back, as holding has it, worked out once, the first time a run of them
// that shows a hold is asked of.
type series struct {
	key     string
	read    []readings.Reading
	places  []int
	holding func() holding
}

// holding is how a series' readings hold their account back, indexed so the
// runs of them that show a hold, and where they lift, are found with a
// search: for each, the first of the chain of them ending with it, each read
// before the reset of the one before it passed, as a run runs on only so;
// and how far each holds the account back, as lows over them: held, its use,
// but of one spent, beyond any use; and capping, its use, but of one spent,
// short of any, as being spent isn't being at a cap.
type holding struct {
	chain         []int
	held, capping lows
}

// seriesOf is the readings of the window with the given key at the places
// given among those given, oldest first, indexed as series has them.
// Readings given out of order are put in order, those read at the one moment
// kept as given.
func seriesOf(key string, read []readings.Reading, places []int) series {
	byTime := func(i, j int) int { return read[i].At.Compare(read[j].At) }
	if !slices.IsSortedFunc(places, byTime) {
		slices.SortStableFunc(places, byTime)
	}
	s := series{key: key, read: read, places: places}
	s.holding = sync.OnceValue(s.holdingOf)
	return s
}

// holdingOf is how the series' readings hold their account back, as holding
// has it.
func (s series) holdingOf() holding {
	h := holding{chain: make([]int, len(s.places))}
	for k := 1; k < len(s.places); k++ {
		h.chain[k] = k
		if !s.reading(k - 1).Window().ResetBy(s.reading(k).At) {
			h.chain[k] = h.chain[k-1]
		}
	}
	h.held = lowsOf(len(s.places), func(k int) float64 { return s.use(k, math.Inf(1)) })
	h.capping = lowsOf(len(s.places), func(k int) float64 { return s.use(k, math.Inf(-1)) })
	return h
}

// reading is the series' k-th reading.
func (s series) reading(k int) *readings.Reading {
	return &s.read[s.places[k]]
}

// use is the k-th reading's use, but spent where it's spent, as score.Spent
// says.
func (s series) use(k int, spent float64) float64 {
	if r := s.reading(k); !score.Spent(r.Window()) {
		return r.Utilization
	}
	return spent
}

// lastBy is the place in the series of its last reading by t, -1 where none
// was read by then.
func (s series) lastBy(t time.Time) int {
	return sort.Search(len(s.places), func(k int) bool { return s.reading(k).At.After(t) }) - 1
}

// readBy is the window as its readings by t have it, as windowRead has it,
// its account's reserve given, the readings reaching back as reach says,
// reporting false where none was read by then: the run of them that reads
// it spent, where the last does, and lifts as the first after it that
// doesn't; and the run that reads it at its cap, where the last does, as
// score.HeldByReserve has it, and lifts as the first after it held back at
// neither, as a cap turned to a limit hasn't lifted.
func (s series) readBy(t time.Time, reach Reach, reserve float64) (windowRead, bool) {
	k := s.lastBy(t)
	if k < 0 {
		return windowRead{}, false
	}
	w := windowRead{last: s.reading(k).Window()}
	if score.Spent(w.last) {
		h := s.holding()
		w.spent = s.run(k, h.chain[k], h.held.lastBelow(k, math.Inf(1)), h.held.firstBelow(k, math.Inf(1)), reach)
	}
	if score.HeldByReserve(w.last, reserve) {
		h, from := s.holding(), score.ReserveFrom(reserve)
		w.capped = s.run(k, h.chain[k], h.capping.lastBelow(k, from), h.held.firstBelow(k, from), reach)
	}
	return w, true
}

// run is the run of readings ending with the k-th that shows a hold, its
// chain beginning with the chained-th, the last before it that doesn't show
// the hold the before-th, and the first after it that doesn't the after-th,
// -1 where none does: begun as the first of it in the chain, as begun has
// it; and lifted at the after-th where that's before the k-th's reset, as a
// reading of a window reset by hand doesn't show its hold, the window
// started again; else as it was, its hold lifting at the reset at the
// latest.
func (s series) run(k, chained, before, after int, reach Reach) run {
	r := run{shown: true, from: s.begun(max(before+1, chained), reach)}
	if after >= 0 && !s.reading(k).Window().ResetBy(s.reading(after).At) {
		r.lifted = s.reading(after).At
	}
	return r
}

// begun is when a run begun by the k-th reading began: as it was read, but
// zero where that's the window's first from an edge of the reach on, and
// the window began before that edge, or when isn't known, so the run may
// have begun before anything read tells.
func (s series) begun(k int, reach Reach) time.Time {
	at := s.reading(k).At
	for _, edge := range reach.edges() {
		if edge.IsZero() || at.Before(edge) || k > 0 && !s.reading(k-1).At.Before(edge) {
			continue
		}
		if began, _, spanned := s.reading(k).Window().Span(); !spanned || began.Before(edge) {
			return time.Time{}
		}
	}
	return at
}

// lows is the lowest of a list of values over each span of it a tree halves
// it into: the root over the list, at 1; and the children of node i over
// the halves of its span at 2i and 2i+1; down to the leaves, a value each,
// from size on, a power of two, those past the list's end beyond any bound.
// So the last value before a place, or the first after one, below a bound
// is found by a search down the tree, not a scan of the list.
type lows struct {
	size int
	low  []float64
}

// lowsOf is the list of n values given, by place, as lows has it.
func lowsOf(n int, value func(int) float64) lows {
	size := 1
	for size < n {
		size *= 2
	}
	low := make([]float64, 2*size)
	for i := range size {
		low[size+i] = math.Inf(1)
		if i < n {
			low[size+i] = value(i)
		}
	}
	for i := size - 1; i > 0; i-- {
		low[i] = min(low[2*i], low[2*i+1])
	}
	return lows{size: size, low: low}
}

// lastBelow is the place of the last value before k below bound, -1 where
// there's none.
func (l lows) lastBelow(k int, bound float64) int {
	return l.last(1, 0, l.size, k, bound)
}

// last is the place of the last value before k below bound in node's span,
// from lo up to hi, -1 where there's none.
func (l lows) last(node, lo, hi, k int, bound float64) int {
	if lo >= k || l.low[node] >= bound {
		return -1
	}
	if hi-lo == 1 {
		return lo
	}
	mid := (lo + hi) / 2
	if i := l.last(2*node+1, mid, hi, k, bound); i >= 0 {
		return i
	}
	return l.last(2*node, lo, mid, k, bound)
}

// firstBelow is the place of the first value after k below bound, -1 where
// there's none.
func (l lows) firstBelow(k int, bound float64) int {
	return l.first(1, 0, l.size, k, bound)
}

// first is the place of the first value after k below bound in node's span,
// from lo up to hi, -1 where there's none.
func (l lows) first(node, lo, hi, k int, bound float64) int {
	if hi <= k+1 || l.low[node] >= bound {
		return -1
	}
	if hi-lo == 1 {
		return lo
	}
	mid := (lo + hi) / 2
	if i := l.first(2*node, lo, mid, k, bound); i >= 0 {
		return i
	}
	return l.first(2*node+1, mid, hi, k, bound)
}
