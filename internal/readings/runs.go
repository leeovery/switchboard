package readings

import (
	"iter"
	"slices"
	"sort"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// CarriedRuns returns what Runway's tail reads of the readings given, oldest
// first, from from, starting at start and ending at end: each window's run
// from from up to start, as runRead keeps it, then those from start up to
// end, each part ordered as Carried orders it. It's for a reader that holds
// the history's readings itself, as Reader's Tail reads them from the files.
func CarriedRuns(read iter.Seq[Reading], from, start, end time.Time) []Reading {
	return carriedBy(read, span{from: from, start: start, end: end}, newRunRead)
}

// TailOf returns a read of the readings between gives, as Reader's Tail
// reads them from the files: from start up to end, each window's run from
// from up to start carried in, as CarriedRuns has them. Each call reads them
// whole: it's for a reader that holds the history's readings itself, whose
// Between is between.
func TailOf(between func(from, to time.Time) iter.Seq[Reading], from, start, end time.Time) func() []Reading {
	return func() []Reading { return CarriedRuns(between(from, end), from, start, end) }
}

// newRunsDay returns the readings history in files of the local day that
// starts at start and ends at end, and each window's run from from up to its
// start, as runRead keeps it, none read yet.
func newRunsDay(files *dayfile.Files, from, start, end time.Time) *Day {
	return newDay(files, span{from: from, start: start, end: end}, newRunRead)
}

// runRead is each window's run of those read, by the window it reads, so
// whoever reads it can tell when a hold that run shows began: its readings
// back to the first of the unbroken run that ends with its last, no reset
// any of them gives passed before the next was read, as one window's period
// runs, oldest first; held led by the reading before them, where one was
// read, whose reset had passed, so a later file's run, kept after it, joins
// it or not as the reset says.
type runRead map[window][]Reading

// newRunRead is a runRead of none read yet.
func newRunRead() carry {
	return make(runRead)
}

// keep keeps r, read after those kept, in its window's run, in its place by
// when it was read, after any read at the same time, as the history can hold
// one read a moment before another after it.
func (rr runRead) keep(r Reading) {
	w := window{account: r.Account, key: r.Key}
	run := rr[w]
	if n := len(run); n == 0 || !r.At.Before(run[n-1].At) {
		if n > 0 && run[n-1].Window().ResetBy(r.At) {
			run = run[n-1:]
		}
		rr[w] = append(run, r)
		return
	}
	run = slices.Insert(run, sort.Search(len(run), func(i int) bool { return run[i].At.After(r.At) }), r)
	rr[w] = run[brokenAt(run):]
}

// brokenAt is where the run given, oldest first, last broke: the index of
// the last reading whose reset had passed by the next's, 0 where none had.
func brokenAt(run []Reading) int {
	for i := len(run) - 1; i > 0; i-- {
		if run[i-1].Window().ResetBy(run[i].At) {
			return i - 1
		}
	}
	return 0
}

// held are the readings kept, each window's run led by the reading before
// it, where one was read, oldest first.
func (rr runRead) held() []Reading {
	var read []Reading
	for _, run := range rr {
		read = append(read, run...)
	}
	return read
}

// carried are each window's run, oldest first, without the reading before
// it.
func (rr runRead) carried() []Reading {
	var read []Reading
	for _, run := range rr {
		if len(run) > 1 && run[0].Window().ResetBy(run[1].At) {
			run = run[1:]
		}
		read = append(read, run...)
	}
	return read
}
