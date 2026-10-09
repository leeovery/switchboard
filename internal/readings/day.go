package readings

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// Day is the readings history of a local day as a summary of it reads it,
// read as it grows: the last reading of each window from from up to the
// day's start, and the day's own, until its end, of the history's files that
// Between reads of the two, whatever day's file each is in. Each file is read
// as a dayfile.Tail reads it, so a read after the first reads only what they
// gained, and holds none of the readings before the day but each window's
// last. It isn't safe for concurrent use.
type Day struct {
	span  span
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
// what it holds of the Day's span: the last of each window before the day,
// and those of the day, in the order they were read.
type dayReadings struct {
	tail *dayfile.Tail[Reading]
	last lastRead
	day  []Reading
}

// lastRead is the last reading of each window of those read, by the window
// it reads.
type lastRead map[window]Reading

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

// NewDay returns the readings history in files of the local day that starts
// at start and ends at end, and the last reading of each window from from up
// to its start, none read yet.
func NewDay(files *dayfile.Files, from, start, end time.Time) *Day {
	d := &Day{span: span{from: from, start: start, end: end}}
	for _, date := range dayfile.Dates(from, end) {
		d.files = append(d.files, &dayReadings{tail: dayfile.NewTail(files, date, In), last: make(lastRead)})
	}
	return d
}

// Read reads what the history's files gained since the last read, and
// returns the last reading of each window from from up to the day's start,
// in the order they were read, then the day's, in the order they were read:
// those a read of the history from from up to the day's end gives that a
// summary of the day reads, as each window's use before the day is the last
// read of it. A file that can't be read is warned of, as dayfile.Tail says,
// and the readings of those that can be returned.
func (d *Day) Read() []Reading {
	last, ofDay := make(lastRead), 0
	for _, file := range d.files {
		file.read(d.span)
		for _, r := range file.last {
			last.keep(r)
		}
		if cut, ok := file.tail.Cut(); ok && d.span.of(cut) == before {
			last.keep(cut)
		}
		ofDay += len(file.day) + 1
	}
	read := slices.AppendSeq(make([]Reading, 0, len(last)+ofDay), maps.Values(last))
	slices.SortFunc(read, byWindowRead)
	for _, file := range d.files {
		read = append(read, file.day...)
		if cut, ok := file.tail.Cut(); ok && d.span.of(cut) == during {
			read = append(read, cut)
		}
	}
	slices.SortStableFunc(read[len(last):], func(a, b Reading) int { return a.At.Compare(b.At) })
	return read
}

// read reads what the file gained since the last read, keeping what's of
// span.
func (f *dayReadings) read(s span) {
	f.tail.Read(func() { f.last, f.day = make(lastRead), nil }, func(r Reading) {
		switch s.of(r) {
		case before:
			f.last.keep(r)
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
