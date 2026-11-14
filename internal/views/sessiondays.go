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
// what each session spent by the hour too, and how many times the fold took
// lines in or began afresh. first is the date of the first day
// ReadSessionDays read, and begun are the first requests of the sessions
// that ran into it, by their ids, as it walks back for them. It's a value:
// each change gives another, sharing nothing it changes with it.
type SessionDays struct {
	days  []sessionDay
	today LedgerDay
	held  daySessions
	folds int
	first string
	begun map[string]time.Time
}

// sessionDay is a day gone's sessions, by its date, and when they were read
// whole; owed where they're what the tail gave of it before the day turned,
// its lines after the tail's last read unread, till they're read again
// whole, as Owed has it, read whole at no time.
type sessionDay struct {
	date     string
	sessions []DaySession
	at       time.Time
	owed     bool
}

// todaysSessions is a fold of today's sessions of no line yet, what each
// spent kept by the hour, and what requests of no session spent, no story
// told of its requests, as the List tells today's, keeping the keys of the
// lines it takes, as it takes a read of the tail at a time.
func todaysSessions() daySessions {
	d := newDaySessions(false)
	d.hourly, d.loose, d.keys = true, accountHours{}, make(map[turnKey]int)
	return d
}

// startedBack is how many days before the first it reads ReadSessionDays
// walks back, at most, for when a session running into it began.
const startedBack = 31

// ReadSessionDays reads the sessions of each day from from's to the day
// before now's, oldest first, as DaySessionsOn reads each: of today's none
// yet, for the ledger's tail to give, as Tailed takes them, or as Joined
// joins the days read to those the tail fed. It walks back
// from the day before from's, as begunBefore has it, for when the sessions
// running into the first day began, so a session's start is its first
// request wherever it fell. Each day is read whole at now.
func ReadSessionDays(l Ledger, from, now time.Time) SessionDays {
	d := SessionDays{today: Today(now), held: todaysSessions(), first: dayfile.DateOf(from)}
	for _, date := range dayfile.Span(from, dayfile.DayStart(now.Local(), -1)) {
		d.days = append(d.days, sessionDay{date: date, sessions: DaySessionsOn(l, date), at: now})
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
// lines, whole, read at now, in place of what d held of it, owed no more,
// whatever it held. Lines gained that arrived on another day than today are
// the late read's of their day, and are passed over.
func (d SessionDays) Tailed(gained []ledger.Held, afresh bool, read []string, late map[string][]ledger.Held, now time.Time) SessionDays {
	today := Today(now)
	switch {
	case d.turned(now):
		d.days = withDay(d.days, sessionDay{date: d.today.Date, sessions: d.held.sessions(), owed: true})
		for _, date := range dayfile.Span(dayfile.DayStart(d.today.Start, 1), dayfile.DayStart(today.Start, -1)) {
			d.days = withDay(d.days, sessionDay{date: date, owed: true})
		}
		d.today, d.held, d.folds = today, todaysSessions(), d.folds+1
	case d.today.Date != today.Date, afresh:
		d.today, d.held, d.folds = today, todaysSessions(), d.folds+1
	}
	dates := slices.Concat(read, slices.Collect(maps.Keys(late)))
	slices.Sort(dates)
	for _, date := range slices.Compact(dates) {
		if date < today.Date {
			d.days = withDay(d.days, sessionDay{date: date, sessions: DaySessions(RecordsOf(late[date])), at: now})
		}
	}
	records := slices.DeleteFunc(RecordsOf(gained).records, func(r lineRecord) bool { return dayfile.DateOf(r.arrived()) != today.Date })
	if len(records) > 0 {
		d.held, d.folds = d.held.with(records), d.folds+1
	}
	return d
}

// Told reports whether d tells otherwise of the sessions than was, the days
// it was fed from: today another day, or its fold having taken lines in or
// begun afresh since, or a day gone held of another read, or another day
// held, or one let go of.
func (d SessionDays) Told(was SessionDays) bool {
	return d.today.Date != was.today.Date || d.folds != was.folds || !slices.EqualFunc(d.days, was.days, sameRead)
}

// sameRead reports whether a and b are the same read of the same day.
func sameRead(a, b sessionDay) bool {
	return a.date == b.date && a.at.Equal(b.at) && a.owed == b.owed
}

// Joined is the days read, as ReadSessionDays reads them, joined to d, the
// days the ledger's tail fed, as Tailed takes its reads: of each day either
// holds, the copy read later, d's where they were read alike, an owed day
// read whole at no time; today's sessions d's, never read's; and the first
// requests of the sessions running into the first day read read's, as it
// walked back for them. So a read landing after a read of the tail never
// stands for what the tail gave meanwhile, nor the tail's copy of a day
// for a read of it since.
func (d SessionDays) Joined(read SessionDays) SessionDays {
	joined := read
	joined.today, joined.held = d.today, d.held
	for _, day := range d.days {
		if !read.readLater(day) {
			joined.days = withDay(joined.days, day)
		}
	}
	return joined
}

// Without is d, the days the ledger's tail fed, without those read stands
// for, as Joined joins them: each day before the first read reads, and each
// read holds of a later read.
func (d SessionDays) Without(read SessionDays) SessionDays {
	d.days = slices.DeleteFunc(slices.Clone(d.days), func(day sessionDay) bool { return day.date < read.first || read.readLater(day) })
	return d
}

// readLater reports whether d holds the day of day's date read whole later
// than day was.
func (d SessionDays) readLater(day sessionDay) bool {
	i, found := dayAt(d.days, day.date)
	return found && d.days[i].at.After(day.at)
}

// TodayAlone is d's fold of today's sessions alone, and how many times it
// took lines in or began afresh, so Told sees each; the days gone it holds
// let go of, so it owes none of them a read.
func (d SessionDays) TodayAlone() SessionDays {
	return SessionDays{today: d.today, held: d.held, folds: d.folds}
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
	i, found := dayAt(days, day.date)
	days = slices.Clone(days)
	if found {
		days[i] = day
		return days
	}
	return slices.Insert(days, i, day)
}

// dayAt is where the day with the given date is among days, oldest first,
// or would be, reporting whether it is.
func dayAt(days []sessionDay, date string) (int, bool) {
	return slices.BinarySearchFunc(days, date, func(d sessionDay, date string) int { return strings.Compare(d.date, date) })
}
