package views

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// sidesRead is a ledger as the find reads it through DaySessionsOn, noting
// the dates of the days whose sides DaySide gave, standing, and of those
// whose lines Day read.
type sidesRead struct {
	Ledger
	sides, raw map[string]bool
}

// readingSides is l read as sidesRead notes it, nothing read yet.
func readingSides(l Ledger) *sidesRead {
	return &sidesRead{Ledger: l, sides: make(map[string]bool), raw: make(map[string]bool)}
}

func (r *sidesRead) DaySide(date string, body int) (json.RawMessage, bool) {
	side, ok := r.Ledger.DaySide(date, body)
	if ok {
		r.sides[date] = true
	}
	return side, ok
}

func (r *sidesRead) Day(date string) []ledger.Held {
	r.raw[date] = true
	return r.Ledger.Day(date)
}

// noSides is a ledger read as one that holds no day's head: the find reads
// every day's lines.
type noSides struct {
	Ledger
}

func (noSides) DaySide(string, int) (json.RawMessage, bool) {
	return nil, false
}

// findTyped is what's typed on the find's line as its reads are compared:
// nothing, finding every session; the start of an id; and a directory's
// name.
var findTyped = []string{"", "s", "other"}

// findWalked are the sources of the find at now, walked back through l as
// the watch walks it while it's open, each past day it names read as
// DaySessionsOn reads it, until it's done: of none of today's sessions, as
// the past days' alone are read.
func findWalked(l Ledger, now time.Time) FindSources {
	src := FindSources{Read: make(map[string][]DaySession), Now: now}
	first, err := l.FirstLinesDay(now)
	if err != nil {
		return src
	}
	src.Oldest = OldestKept(first, 0, now)
	src.Summaries = l.Days(src.Oldest.Day.Start)
	for found := FindSessions(src); !found.Done(); found = FindSessions(src) {
		src.Read[found.Next] = DaySessionsOn(l, found.Next)
	}
	return src
}

// foundTyped is what the find of src finds of each of findTyped, in turn.
func foundTyped(src FindSources) []Found {
	found := make([]Found, len(findTyped))
	for i, typed := range findTyped {
		src.Typed = typed
		found[i] = FindSessions(src)
	}
	return found
}

// sidesRaw notes, of a read of the find through r, a ledger whose days'
// sides it read as sidesRead notes them, the dates of the days it read raw
// though their sides stand; none where it read none so.
func sidesRaw(r *sidesRead) []string {
	var both []string
	for date := range r.raw {
		if r.sides[date] {
			both = append(both, date)
		}
	}
	slices.Sort(both)
	return both
}

// The side of a day, as the views' Digests build it of a real ledger's lines
// day by day, holds the sessions DaySessions gives of the day's lines read
// raw, as Day gives them, each restored exactly, a session's directory as
// its lines last named it, quota checks counted toward none: and stands of
// every day the generated worlds file lines on, once its head is built. The
// find walked back over the sides, at the end, reads no day's lines, and
// finds what it finds over the days read raw, by id and by directory.
func TestADaysSideIsItsSessionsReadRaw(t *testing.T) {
	var stood int
	for _, w := range chainWorlds {
		for seed := range uint64(chainSeeds) {
			c := newDigestChain(t, fmt.Sprintf("%s %d", w.name, seed), seed, w.lines(seed))
			c.digestDayByDay(func(LedgerDay) {})
			f := ledger.NewFollower(c.state, func() time.Time { return c.at }, ledger.CapsOf(nil, nil), DigestVersion, quiet)
			for _, date := range filedDates(c.lines) {
				side, ok := f.DaySide(date, RowsVersion)
				if !ok {
					t.Errorf("%s: the side of %s doesn't stand at %v, its head built", c.name, date, c.at)
					continue
				}
				stood++
				got, err := daySessionsIn(side)
				if want := DaySessions(f.Day(date)); err != nil || !reflect.DeepEqual(got, want) {
					t.Errorf("%s: the side of %s holds\n%+v (%v)\nwant, as its lines read raw give\n%+v", c.name, date, got, err, want)
				}
			}
			sides := readingSides(f)
			if got, want := foundTyped(findWalked(sides, c.at)), foundTyped(findWalked(noSides{f}, c.at)); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: the find over the sides finds\n%+v\nwant, as over the days read raw\n%+v", c.name, got, want)
			}
			if len(sides.raw) > 0 {
				t.Errorf("%s: the find over the sides read the lines of %v, want none, every side standing", c.name, slices.Sorted(maps.Keys(sides.raw)))
			}
		}
	}
	if stood == 0 {
		t.Error("no day's side stood, want every day's")
	}
}

// filedDates are the dates of the local days lines arrived on, oldest first.
func filedDates(lines []ledger.Line) []string {
	var dates []string
	for _, l := range lines {
		dates = append(dates, dayfile.DateOf(l.At))
	}
	slices.Sort(dates)
	return slices.Compact(dates)
}

// A day's side read where it doesn't stand, or won't read, is never what the
// find reads: the day's lines are read instead, as Day gives them. Where it
// stands, the lines are never read.
func TestTheFindReadsADaysSideOnlyWhereItStands(t *testing.T) {
	const date = "2026-10-08"
	lines := []ledger.Held{{At: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC), Session: "s", Kind: ledger.KindMessage, Dir: "/code/app"}}
	raw := DaySessions(lines)
	stale := []DaySession{{ID: "s", Dir: "/code/old", Lines: 7, Last: time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)}}
	staleSide, err := daySideOf(stale)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		side   json.RawMessage
		stands bool
		want   []DaySession
		read   bool
	}{
		{name: "a side standing", side: staleSide, stands: true, want: stale},
		{name: "a side not standing", side: staleSide, want: raw, read: true},
		{name: "a side standing that won't read", side: json.RawMessage(`{"id":"s"}`), stands: true, want: raw, read: true},
		{name: "no side", want: raw, read: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := readingSides(sided{Ledger: dayHeld{date: date, lines: lines}, side: tt.side, stands: tt.stands})
			if got := DaySessionsOn(l, date); !reflect.DeepEqual(got, tt.want) || l.raw[date] != tt.read {
				t.Errorf("DaySessionsOn(%s) = %+v, its lines read %t; want %+v, read %t", date, got, l.raw[date], tt.want, tt.read)
			}
		})
	}
}

// sided is a ledger whose every day's side is side, standing as stands
// says.
type sided struct {
	Ledger
	side   json.RawMessage
	stands bool
}

func (s sided) DaySide(string, int) (json.RawMessage, bool) {
	return s.side, s.stands
}

// dayHeld is a ledger holding lines, the day with the given date's alone.
type dayHeld struct {
	Ledger
	date  string
	lines []ledger.Held
}

func (d dayHeld) Day(date string) []ledger.Held {
	if date != d.date {
		return nil
	}
	return d.lines
}
