package views

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// DaySession is a session of a past day, as the Log's find holds it for the
// run once the day's lines are read, of its lines that day but Claude Code's
// quota checks: its whole id; the directory it ran in, as they last named
// it, "" where none did; how many they are; and when the last arrived.
type DaySession struct {
	ID, Dir string
	Lines   int
	Last    time.Time
}

// DaySessions are the sessions of a day's lines, as Ledger's Day gives them,
// oldest first: those its summary names, each line counting toward the one
// Line.CountedSession says, the last to arrive first.
func DaySessions(day []ledger.Held) []DaySession {
	sessions := make(daySessions)
	for i := range day {
		sessions.take(&day[i].Line)
	}
	return sessions.sessions()
}

// DaySessionsOn are the sessions of the past day with the given date, as
// DaySessions gives them of its lines: read from its side, as l's DaySide
// gives it, where its head stands; else of its lines, as l's Day gives them,
// as of a day with no head standing, today's among them, and yesterday's
// until its head is built, or one whose side won't read.
func DaySessionsOn(l Ledger, date string) []DaySession {
	if side, ok := l.DaySide(date, RowsVersion); ok {
		if sessions, err := daySessionsIn(side); err == nil {
			return sessions
		}
	}
	return DaySessions(l.Day(date))
}

// daySessions are the sessions of a day's lines taken in so far, by their
// ids: the one fold of a day's sessions, whether of the lines Day gives, as
// DaySessions folds them, or as the day's side is built of them.
type daySessions map[string]*DaySession

// take takes in l, the day's next line in Day's order, toward the session it
// counts toward, as Line.CountedSession says, where it counts toward one:
// when it arrived in UTC, as the ledger writes it, and a day's side restores
// it, whatever zone a line edited by hand names.
func (d daySessions) take(l *ledger.Line) {
	id := l.CountedSession()
	if id == "" {
		return
	}
	s, ok := d[id]
	if !ok {
		s = &DaySession{ID: id}
		d[id] = s
	}
	s.Lines++
	s.Last = score.Later(s.Last, l.At.UTC())
	if l.Dir != "" {
		s.Dir = l.Dir
	}
}

// sessions are the sessions taken in, the last to arrive first, as
// DaySessions orders them.
func (d daySessions) sessions() []DaySession {
	sessions := make([]DaySession, 0, len(d))
	for _, s := range d {
		sessions = append(sessions, *s)
	}
	slices.SortFunc(sessions, func(a, b DaySession) int { return cmp.Or(b.Last.Compare(a.Last), strings.Compare(a.ID, b.ID)) })
	return sessions
}

// storedDaySession is a DaySession as a day's side stores it, the side a
// list of them in DaySessions' order: when its last line arrived as
// nanoseconds since the Unix epoch, restored in UTC, as the ledger writes a
// line's time; zeros left out. A day's head holds its bounds so too, and
// stands only where they're the day's, so no time a side holds is beyond the
// years nanoseconds since the epoch hold.
type storedDaySession struct {
	ID    string `json:"id"`
	Dir   string `json:"dir,omitzero"`
	Lines int    `json:"lines,omitzero"`
	Last  int64  `json:"last,omitzero"`
}

// stored is s as a day's side stores it.
func (s DaySession) stored() storedDaySession {
	stored := storedDaySession{ID: s.ID, Dir: s.Dir, Lines: s.Lines}
	if !s.Last.IsZero() {
		stored.Last = s.Last.UnixNano()
	}
	return stored
}

// daySession is the DaySession s stores.
func (s storedDaySession) daySession() DaySession {
	d := DaySession{ID: s.ID, Dir: s.Dir, Lines: s.Lines}
	if s.Last != 0 {
		d.Last = time.Unix(0, s.Last).UTC()
	}
	return d
}

// daySideOf is the side of a day of the given sessions, as DaySessions gives
// them, as the day's head holds it. Its error is the ledger's to wrap, with
// the day's date.
func daySideOf(sessions []DaySession) (json.RawMessage, error) {
	stored := make([]storedDaySession, len(sessions))
	for i, s := range sessions {
		stored[i] = s.stored()
	}
	return json.Marshal(stored)
}

// daySessionsIn are the sessions side holds, as daySideOf gave it, failing
// where it doesn't read as a day's side.
func daySessionsIn(side json.RawMessage) ([]DaySession, error) {
	var stored []storedDaySession
	if err := json.Unmarshal(side, &stored); err != nil {
		return nil, fmt.Errorf("a day's side: %w", err)
	}
	sessions := make([]DaySession, len(stored))
	for i, s := range stored {
		sessions[i] = s.daySession()
	}
	return sessions, nil
}

// FindSources are what the Log's find is built from.
type FindSources struct {
	// Typed is what's typed on its line.
	Typed string
	// Today are today's sessions, as LogSessions gives them, in its order.
	Today []LogSession
	// Read are the sessions of the past days whose lines are read, by each
	// day's date, as DaySessions gives them: held for the run.
	Read map[string][]DaySession
	// Summaries are the summaries of the days the ledger holds, oldest
	// first, as Ledger's Days gives them: the days the find goes back
	// through, and the sessions each names by its id, before its lines are
	// read.
	Summaries []ledger.Summary
	// Oldest is the oldest day the ledger keeps the lines of, as OldestKept
	// gives it: the find goes back no further, though summaries are kept for
	// good; nor back at all where it's none known.
	Oldest OldestDay
	Now    time.Time
}

// Found is what the Log's find finds, as far as it has gone back.
type Found struct {
	// Sessions are the sessions it matches, in order.
	Sessions []FoundSession
	// Next is the date of the day whose lines are to be read next, for its
	// sessions' directories: the newest past day not read yet whose summary
	// names a session, or might; "" once there's none, the find done.
	Next string
}

// Done reports whether the find has gone back through every day it reads.
func (f Found) Done() bool {
	return f.Next == ""
}

// FoundSession is a session the find matches, on the newest day it's of.
type FoundSession struct {
	// ID is its whole id, and Dir the directory it ran in on Day, as today's
	// sessions and a day's lines name it: "" of a day not read yet.
	ID, Dir string
	// Day is the day it matches on: today, or the newest past day it's of.
	Day LedgerDay
	// Ended is set where it has ended: one of today's that has, and every
	// one of a past day, none of whose requests came today.
	Ended bool
	// Last is when its last request on Day came, where it has ended and
	// that's known: zero of a day not read yet.
	Last time.Time
}

// FindSessions finds the sessions what's typed matches, as far back as the
// find has read: each whose id starts with it, or whose directory's name, its
// last part, holds it, whatever the case. Each session is judged once, on
// the newest day it's of: today's first, in the panel's order; then each
// past day's the summaries give, newest first, back to the oldest day the
// ledger keeps: of a day read, its sessions as its lines name them, the last
// to arrive first, then those its summary names that its lines don't, as
// where its lines are damaged; of a day not read yet, those its summary
// names. A session its summary alone names is found by its id alone, as its
// directory isn't known till its lines are read. A day whose summary names
// no session, and might name none, is never read.
func FindSessions(src FindSources) Found {
	today := Today(src.Now)
	f := finder{typed: strings.ToLower(src.Typed), judged: make(map[string]bool)}
	for _, s := range src.Today {
		f.judge(foundToday(s, today))
	}
	var next string
	for _, summary := range slices.Backward(src.Summaries) {
		if summary.Day >= today.Date {
			continue
		}
		day := dayOf(summary.Day)
		if !src.Oldest.keeps(day) {
			break
		}
		if toRead := f.judgeDay(summary, day, src.Read); toRead && next == "" {
			next = summary.Day
		}
	}
	return Found{Sessions: f.found, Next: next}
}

// foundToday is s, one of today's sessions, as the find finds it: ended,
// its last request come when it ended, where it has.
func foundToday(s LogSession, today LedgerDay) FoundSession {
	found := FoundSession{ID: s.ID, Dir: s.Dir, Day: today}
	if s.Activity.Doing == Ended {
		found.Ended, found.Last = true, s.Activity.Since
	}
	return found
}

// finder judges sessions for the find: what's typed, lowered; the ids of
// those judged, each once; and those found, in the order they were.
type finder struct {
	typed  string
	judged map[string]bool
	found  []FoundSession
}

// judgeDay judges the sessions of the past day the summary is of, as
// FindSessions says: those read of it where its lines are read, then those
// its summary names. It reports whether its lines are still to be read: not
// read yet, its summary naming a session, or might.
func (f *finder) judgeDay(summary ledger.Summary, day LedgerDay, read map[string][]DaySession) (toRead bool) {
	sessions, isRead := read[summary.Day]
	for _, s := range sessions {
		f.judge(FoundSession{ID: s.ID, Dir: s.Dir, Day: day, Ended: true, Last: s.Last})
	}
	ids, might := sessionsNamed(summary)
	for _, id := range ids {
		f.judge(FoundSession{ID: id, Day: day, Ended: true})
	}
	return !isRead && (len(ids) > 0 || might)
}

// judge finds s where it matches what's typed, as FindSessions says, and it
// wasn't judged before, on a newer day.
func (f *finder) judge(s FoundSession) {
	if f.judged[s.ID] {
		return
	}
	f.judged[s.ID] = true
	name := strings.ToLower(status.DirName(s.Dir, math.MaxInt))
	if strings.HasPrefix(strings.ToLower(s.ID), f.typed) || strings.Contains(name, f.typed) {
		f.found = append(f.found, s)
	}
}

// sessionsNamed are the ids of the sessions a day's summary names, each
// once, in their order, and whether it might be of more, as one that counts
// sessions it doesn't name, as Summary.Unnamed says, is.
func sessionsNamed(s ledger.Summary) (ids []string, might bool) {
	for _, a := range s.Accounts {
		ids = append(ids, a.SessionIDs...)
	}
	slices.Sort(ids)
	return slices.Compact(ids), s.Unnamed() > 0
}

// dayOf is the local day with the given date.
func dayOf(date string) LedgerDay {
	start, _, _ := dayfile.Day(date)
	return Today(start)
}
