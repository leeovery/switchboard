package capture

import (
	"iter"
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// history is a readings history as a readings.Reader reads it: a line for each
// reading that changed how a window of an account reads, in the order they
// were read.
type history []readings.Reading

// Between gives the readings of times from from up to to, in the order they
// were read.
func (h history) Between(from, to time.Time) iter.Seq[readings.Reading] {
	return func(yield func(readings.Reading) bool) {
		i, _ := slices.BinarySearchFunc(h, from, func(r readings.Reading, t time.Time) int { return r.At.Compare(t) })
		for _, r := range h[i:] {
			if !r.At.Before(to) || !yield(r) {
				return
			}
		}
	}
}

// of is the history of the accounts with the given ids alone.
func (h history) of(ids []string) history {
	return slices.DeleteFunc(slices.Clone(h), func(r readings.Reading) bool { return !slices.Contains(ids, r.Account) })
}

// before is the history of the readings taken before t.
func (h history) before(t time.Time) history {
	i, _ := slices.BinarySearchFunc(h, t, func(r readings.Reading, t time.Time) int { return r.At.Compare(t) })
	return h[:i]
}

// windows are the account with the given id's windows as last read before
// t, in quota.Sort's order, labelled as Claude labels them.
func (h history) windows(account string, t time.Time, label func(key string) string) []quota.Window {
	last := make(map[string]quota.Window)
	for _, r := range h.before(t) {
		if r.Account == account {
			w := r.Window()
			w.Label = label(r.Key)
			last[r.Key] = w
		}
	}
	windows := make([]quota.Window, 0, len(last))
	for _, w := range last {
		windows = append(windows, w)
	}
	quota.Sort(windows)
	return windows
}

// readingsOf collects the readings of a world's accounts as the router takes
// them in, then gives them as its history.
type readingsOf struct {
	read history
}

// window is a window of an account as the router reads it, through the
// stretch from one reset of it to the next.
type window struct {
	account, key string
	resets       time.Time
}

// at takes in a reading of w at a time, its use the one given, read off an
// answer, its status allowed, or rejected once it's used up.
func (r *readingsOf) at(w window, t time.Time, use float64) {
	r.from(w, t, use, readings.FromAnswer)
}

// from takes in a reading of w at a time, from the source given.
func (r *readingsOf) from(w window, t time.Time, use float64, source readings.Source) {
	status := quota.StatusAllowed
	if use >= 1 {
		status = quota.StatusRejected
	}
	r.read = append(r.read, readings.Reading{
		At: t.UTC(), Account: w.account, Key: w.key, Utilization: use, ResetsAt: w.resets.UTC(), Status: status, Source: source,
	})
}

// rise takes in w's use rising evenly from one use to another, from one time
// to another, as the router reads it off the answers while the account is
// busy: a reading each step of use it moves, the last at to. A use is read in
// hundredths, as Claude gives it.
func (r *readingsOf) rise(w window, from, to time.Time, start, end, step float64) {
	steps := int(math.Ceil((end-start)/step - 1e-9))
	for i := 1; i <= steps; i++ {
		use := hundredths(min(start+float64(i)*step, end))
		r.at(w, from.Add(time.Duration(float64(to.Sub(from))*float64(i)/float64(steps))), use)
	}
}

// history is the readings taken in, in the order they were read.
func (r *readingsOf) history() history {
	slices.SortStableFunc(r.read, func(a, b readings.Reading) int { return a.At.Compare(b.At) })
	return r.read
}

// hundredths is a use in whole hundredths of its window, as Claude gives it.
func hundredths(use float64) float64 {
	return math.Round(use*100) / 100
}
