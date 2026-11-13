package views

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
)

// SessionDays are the sessions of each day from the first read to today, as
// Projects reads them: each day before today's as its head's side holds its
// sessions, else as its lines fold them, as DaySessionsOn reads a day, read
// once; and today's folded from its lines as the ledger's tail gives them,
// what each session spent by the hour too. begun are the first requests of
// the sessions that ran into the first day read, by their ids, as
// ReadSessionDays walks back for them. It's a value: each change gives
// another, sharing nothing it changes with it.
type SessionDays struct {
	days  []sessionDay
	today LedgerDay
	held  daySessions
	begun map[string]time.Time
}

// sessionDay is a day gone's sessions, by its date; owed where they're what
// the tail gave of it before the day turned, its lines after the tail's last
// read unread, till they're read again whole, as Owed has it.
type sessionDay struct {
	date     string
	sessions []DaySession
	owed     bool
}

// todaysSessions is a fold of today's sessions of no line yet, what each
// spent kept by the hour, no story told of its requests, as the List tells
// today's, keeping the keys of the lines it takes, as it takes a read of
// the tail at a time.
func todaysSessions() daySessions {
	d := newDaySessions(false)
	d.hourly, d.keys = true, make(map[turnKey]int)
	return d
}

// startedBack is how many days before the first it reads ReadSessionDays
// walks back, at most, for when a session running into it began.
const startedBack = 31

// ReadSessionDays reads the sessions of each day from from's to the day
// before now's, oldest first, as DaySessionsOn reads each: of today's none
// yet, for the ledger's tail to give, as Tailed takes them. It walks back
// from the day before from's, as begunBefore has it, for when the sessions
// running into the first day began, so a session's start is its first
// request wherever it fell.
func ReadSessionDays(l Ledger, from, now time.Time) SessionDays {
	d := SessionDays{today: Today(now), held: todaysSessions()}
	for _, date := range dayfile.Span(from, dayfile.DayStart(now.Local(), -1)) {
		d.days = append(d.days, sessionDay{date: date, sessions: DaySessionsOn(l, date)})
	}
	var first []DaySession
	if len(d.days) > 0 {
		first = d.days[0].sessions
	}
	d.begun = begunBefore(l, dayfile.DayStart(score.Earlier(from, now).Local(), -1), first, len(d.days) > 0)
	return d
}

// begunBefore are the first requests of the sessions that ran into the day
// after the one day starts, by their ids, as DaySessionsOn reads each day
// from day's back, while any of them ran on the day after it, startedBack
// days at most: those of the sessions given where some are, as of a day
// read, else of every session of day's, as of today, whose sessions the tail
// gives.
func begunBefore(l Ledger, day time.Time, after []DaySession, read bool) map[string]time.Time {
	var running map[string]bool
	if read {
		running = make(map[string]bool, len(after))
		for _, s := range after {
			running[s.ID] = true
		}
	}
	begun := make(map[string]time.Time)
	for range startedBack {
		ran := make(map[string]bool)
		for _, s := range DaySessionsOn(l, dayfile.DateOf(day)) {
			if running == nil || running[s.ID] {
				ran[s.ID] = true
				if !s.FirstRequest.IsZero() {
					begun[s.ID] = s.FirstRequest
				}
			}
		}
		if len(ran) == 0 {
			break
		}
		running, day = ran, dayfile.DayStart(day, -1)
	}
	return begun
}

// Owed are the dates of the days before now's d needs read again whole,
// oldest first: each it holds owed, as sessionDay's owed has it; and where
// the day has turned since d's today, the day it followed then, its lines
// after the tail's last read unread, and each after it before now's, none
// of whose lines it read. The caller reads each, and hands Tailed every
// date it read, as read, with the lines of each, as late, a day of no lines
// read none, so a day read stops being owed whatever it held. A watch doing
// so for each read of the tail loses no line of a day that turns.
func (d SessionDays) Owed(now time.Time) []string {
	var owed []string
	for _, day := range d.days {
		if day.owed {
			owed = append(owed, day.date)
		}
	}
	if d.turned(now) {
		owed = append(owed, dayfile.Span(d.today.Start, dayfile.DayStart(Today(now).Start, -1))...)
	}
	slices.Sort(owed)
	return slices.Compact(owed)
}

// Tailed is d at now, given what the ledger's tail read: gained, today's
// lines gained since its mark, every one of today's where afresh says, as
// Ledger's Today gives them; read, the dates of the days before today it
// read again whole, as Owed names the days d needs; and late, their lines,
// by their dates, none of a day of none. Where the day has turned since d's
// today, what d folded of it is kept as that day's, owed, so none of its
// lines is lost or taken twice, and each day after it before today's is
// owed too; and each day read, or late gives, is folded afresh of its
// lines, whole, in place of what d held of it, owed no more, whatever it
// held. Lines gained that arrived on another day than today are the late
// read's of their day, and are passed over.
func (d SessionDays) Tailed(gained []ledger.Held, afresh bool, read []string, late map[string][]ledger.Held, now time.Time) SessionDays {
	today := Today(now)
	switch {
	case d.turned(now):
		d.days = withDay(d.days, sessionDay{date: d.today.Date, sessions: d.held.sessions(), owed: true})
		for _, date := range dayfile.Span(dayfile.DayStart(d.today.Start, 1), dayfile.DayStart(today.Start, -1)) {
			d.days = withDay(d.days, sessionDay{date: date, owed: true})
		}
		d.today, d.held = today, todaysSessions()
	case d.today.Date != today.Date, afresh:
		d.today, d.held = today, todaysSessions()
	}
	dates := slices.Concat(read, slices.Collect(maps.Keys(late)))
	slices.Sort(dates)
	for _, date := range slices.Compact(dates) {
		if date < today.Date {
			d.days = withDay(d.days, sessionDay{date: date, sessions: DaySessions(RecordsOf(late[date]))})
		}
	}
	records := slices.DeleteFunc(RecordsOf(gained).records, func(r lineRecord) bool { return dayfile.DateOf(r.arrived()) != today.Date })
	d.held = d.held.with(records)
	return d
}

// turned reports whether the day has turned at now since d's today, the
// day its fold of the tail is of: never of d read of no day, nor where the
// clock was set back, where Tailed folds today afresh.
func (d SessionDays) turned(now time.Time) bool {
	return d.today.Date != "" && d.today.Date < Today(now).Date
}

// withDay is days, oldest first, with day in place of the one of its date,
// or among them where they hold none: afresh, so days, and any copy of
// them, never see it.
func withDay(days []sessionDay, day sessionDay) []sessionDay {
	i, found := slices.BinarySearchFunc(days, day.date, func(d sessionDay, date string) int { return strings.Compare(d.date, date) })
	days = slices.Clone(days)
	if found {
		days[i] = day
		return days
	}
	return slices.Insert(days, i, day)
}
