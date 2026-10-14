package readings

import (
	"cmp"
	"iter"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// Day is the readings history of a local day as a summary of it reads it,
// read as it grows: the last reading of each window from from up to the
// day's start, and the day's own, until its end, of the history's files that
// Between reads of the two, whatever day's file each is in; or, as Runway's
// tail reads it, each window's run from from up to the day's start in place
// of its last. Each file is read as a dayfile.Tail reads it, so a read after
// the first reads only what they gained, and holds none of the readings
// before the day but what it carries in of each window. It isn't safe for
// concurrent use.
type Day struct {
	span  span
	carry func() carry
	files []*dayReadings
}

// span is the times a Day's readings are of: from from up to start, before
// the day, and from start up to end, of it.
type span struct {
	from, start, end time.Time
}

// part is where a reading falls of a span.
type part int

const (
	// outside is outside the span.
	outside part = iota
	// before is before the day, from from up to start.
	before
	// during is of the day, from start up to end.
	during
)

// of returns where r falls of the span.
func (s span) of(r Reading) part {
	switch {
	case r.At.Before(s.from) || !r.At.Before(s.end):
		return outside
	case r.At.Before(s.start):
		return before
	}
	return during
}

// dayReadings is one of the history's files of a Day, read as it grows, and
// what it holds of the Day's span: what it carries in of each window before
// the day, and those of the day, in the order they were read.
type dayReadings struct {
	tail   *dayfile.Tail[Reading]
	before carry
	day    []Reading
}

// carry is what's carried in of each window's readings from before a day, as
// they're read: each kept as it's read, after those kept; those it holds,
// each window's oldest first, which a read keeps again, after a file's
// before it, to join them; and those it carries in.
type carry interface {
	keep(r Reading)
	held() []Reading
	carried() []Reading
}

// lastRead is the last reading of each window of those read, by the window
// it reads, as a summary reads the use before its day.
type lastRead map[window]Reading

// newLastRead is a lastRead of none read yet.
func newLastRead() carry {
	return make(lastRead)
}

// window names a window of an account.
type window struct {
	account, key string
}

// keep keeps r, read after those kept, as its window's last, unless one
// kept was read later.
func (l lastRead) keep(r Reading) {
	w := window{account: r.Account, key: r.Key}
	if last, ok := l[w]; !ok || !r.At.Before(last.At) {
		l[w] = r
	}
}

// held are the readings kept, one of each window.
func (l lastRead) held() []Reading {
	return slices.Collect(maps.Values(l))
}

// carried are the readings kept, one of each window.
func (l lastRead) carried() []Reading {
	return l.held()
}

// NewDay returns the readings history in files of the local day that starts
// at start and ends at end, and the last reading of each window from from up
// to its start, none read yet.
func NewDay(files *dayfile.Files, from, start, end time.Time) *Day {
	return newDay(files, span{from: from, start: start, end: end}, newLastRead)
}

// newDay returns the readings history in files of the span's day, none read
// yet, what's carried in of each window from before the day kept by a carry
// that carry makes.
func newDay(files *dayfile.Files, s span, carry func() carry) *Day {
	d := &Day{span: s, carry: carry}
	for _, date := range dayfile.Dates(s.from, s.end) {
		d.files = append(d.files, &dayReadings{tail: dayfile.NewTail(files, date, In), before: carry()})
	}
	return d
}

// Read reads what the history's files gained since the last read, and
// returns what it carries in of each window from from up to the day's start,
// in the order they were read, then the day's, in the order they were read:
// of a Day NewDay made, those a read of the history from from up to the
// day's end gives that a summary of the day reads, as each window's use
// before the day is the last read of it. A file that can't be read is warned
// of, as dayfile.Tail says, and the readings of those that can be returned.
func (d *Day) Read() []Reading {
	kept, ofDay := d.carry(), 0
	for _, file := range d.files {
		file.read(d.span, d.carry)
		for _, r := range file.before.held() {
			kept.keep(r)
		}
		if cut, ok := file.tail.Cut(); ok && d.span.of(cut) == before {
			kept.keep(cut)
		}
		ofDay += len(file.day) + 1
	}
	day := make([]Reading, 0, ofDay)
	for _, file := range d.files {
		day = append(day, file.day...)
		if cut, ok := file.tail.Cut(); ok && d.span.of(cut) == during {
			day = append(day, cut)
		}
	}
	return then(kept.carried(), day)
}

// Carried returns what a Day from from, starting at start and ending at end,
// reads of the readings given, oldest first, as its Read returns them: the
// last reading of each window from from up to start, then those from start
// up to end. It's for a reader that holds the history's readings itself, as
// a Day reads them from the files.
func Carried(read iter.Seq[Reading], from, start, end time.Time) []Reading {
	return carriedBy(read, span{from: from, start: start, end: end}, newLastRead)
}

// carriedBy returns what's read of the readings given, oldest first, of the
// span s: what a carry that carry makes carries in of each window before its
// day, then the day's, as a Day's Read returns them.
func carriedBy(read iter.Seq[Reading], s span, carry func() carry) []Reading {
	kept := carry()
	var day []Reading
	for r := range read {
		switch s.of(r) {
		case before:
			kept.keep(r)
		case during:
			day = append(day, r)
		}
	}
	return then(kept.carried(), day)
}

// then are the readings carried in, ordered by when they were read, then by
// their accounts' ids and their windows' keys, a window's read at one time
// in the order given, then the day's, by when each was read, those read at
// one time in the order given.
func then(carried, day []Reading) []Reading {
	read := append(slices.Grow(carried, len(day)), day...)
	slices.SortStableFunc(read[:len(carried)], byWindowRead)
	slices.SortStableFunc(read[len(carried):], func(a, b Reading) int { return a.At.Compare(b.At) })
	return read
}

// read reads what the file gained since the last read, keeping what's of
// span, what's carried in kept by a carry that carry makes as the file is
// read afresh.
func (f *dayReadings) read(s span, carry func() carry) {
	f.tail.Read(func() { f.before, f.day = carry(), nil }, func(r Reading) {
		switch s.of(r) {
		case before:
			f.before.keep(r)
		case during:
			f.day = append(f.day, r)
		}
	})
}

// byWindowRead orders readings by when they were read, those read at one
// time by their accounts' ids and their windows' keys.
func byWindowRead(a, b Reading) int {
	return cmp.Or(a.At.Compare(b.At), strings.Compare(a.Account, b.Account), strings.Compare(a.Key, b.Key))
}
