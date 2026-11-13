package views

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// SessionDays are the sessions of each day from the first read to today, as
// Projects reads them: each day before today's as its head's side holds its
// sessions, else as its lines fold them, as DaySessionsOn reads a day, read
// once; and today's folded from its lines as the ledger's tail gives them,
// what each session spent by the hour too. It's a value: each change gives
// another, sharing nothing it changes with it.
type SessionDays struct {
	days  []sessionDay
	today LedgerDay
	held  daySessions
}

// sessionDay is a day gone's sessions, by its date.
type sessionDay struct {
	date     string
	sessions []DaySession
}

// todaysSessions is a fold of today's sessions of no line yet, what each
// spent kept by the hour, no story told of its requests, as the List tells
// today's.
func todaysSessions() daySessions {
	d := newDaySessions(false)
	d.hourly = true
	return d
}

// ReadSessionDays reads the sessions of each day from from's to the day
// before now's, oldest first, as DaySessionsOn reads each: of today's none
// yet, for the ledger's tail to give, as Tailed takes them.
func ReadSessionDays(l Ledger, from, now time.Time) SessionDays {
	d := SessionDays{today: Today(now), held: todaysSessions()}
	for _, date := range dayfile.Span(from, dayfile.DayStart(now.Local(), -1)) {
		d.days = append(d.days, sessionDay{date: date, sessions: DaySessionsOn(l, date)})
	}
	return d
}

// Tailed is d at now, given what the ledger's tail read: gained, today's
// lines gained since its mark, every one of today's where afresh says, as
// Ledger's Today gives them; and late, the lines of days before today read
// again whole, by their dates, as the tail gives the days it followed no
// further. Where the day has turned since d's today, what d folded of it is
// kept as that day's, so none of its lines is lost or taken twice; and each
// day late gives is folded afresh of its lines, whole, in place of what d
// held of it. Lines gained that arrived on another day than today are its
// tail's to give whole, and are passed over.
func (d SessionDays) Tailed(gained []ledger.Held, afresh bool, late map[string][]ledger.Held, now time.Time) SessionDays {
	today := Today(now)
	switch {
	case d.today.Date != today.Date:
		if d.today.Date != "" {
			d.days = withDay(d.days, sessionDay{date: d.today.Date, sessions: d.held.sessions()})
		}
		d.today, d.held = today, todaysSessions()
	case afresh:
		d.held = todaysSessions()
	}
	for _, date := range slices.Sorted(maps.Keys(late)) {
		if date < today.Date {
			d.days = withDay(d.days, sessionDay{date: date, sessions: DaySessions(RecordsOf(late[date]))})
		}
	}
	records := slices.DeleteFunc(RecordsOf(gained).records, func(r lineRecord) bool { return dayfile.DateOf(r.arrived()) != today.Date })
	d.held = d.held.with(records)
	return d
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
