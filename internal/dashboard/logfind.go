package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// LogFinding is the Log's find, as / opens it in the state line's row:
// whether it's open; what's typed; what it finds, as far back as it has
// read, as the watch built it; and the session picked among them, by its
// whole id, the first where none is, or the one picked is no longer found.
type LogFinding struct {
	Open   bool
	Typed  string
	Found  views.Found
	Picked string
}

// picked is the place of the session picked among those found: false where
// none is found.
func (f LogFinding) picked() (int, bool) {
	found := f.Found.Sessions
	if len(found) == 0 {
		return 0, false
	}
	return max(slices.IndexFunc(found, func(s views.FoundSession) bool { return s.ID == f.Picked }), 0), true
}

// Pick is the session picked: false where none is found.
func (f LogFinding) Pick() (views.FoundSession, bool) {
	at, ok := f.picked()
	if !ok {
		return views.FoundSession{}, false
	}
	return f.Found.Sessions[at], true
}

// Stepped is the find with the session by places on from the one picked
// picked, ↑ -1 and ↓ 1, as the keys step, no further than the first found
// nor the last.
func (f LogFinding) Stepped(by int) LogFinding {
	at, ok := f.picked()
	if !ok {
		return f
	}
	f.Picked = f.Found.Sessions[min(max(at+by, 0), len(f.Found.Sessions)-1)].ID
	return f
}

// FoundOnly is the Log filtered to the session given alone, as the find's
// enter has it: both tables showing its lines alone, as the panel ticks a
// session, each following again, refiltered; and the find closed.
func (p LogPage) FoundOnly(id string) LogPage {
	p.Filter.Sessions, p.Shows.Sessions = []string{id}, []string{id}
	p.Finding = LogFinding{}
	for v, s := range p.Stands {
		p.Stands[v] = LogStand{filledAt: s.filledAt}
	}
	return p.refiltered(LogRequests, LogEvents)
}

// refiltered is p with the tables given unfilled, as each stood as it first
// opened, whose filter has changed what it shows, where bounded says how far
// back it would read is bounded: short of its view, each reads back to fill
// it, as it did on opening, paused or following as it stands. Another
// stands filled as it was, reading back as WantsDayBefore has a filled
// table read back.
func (p LogPage) refiltered(tables ...LogView) LogPage {
	for _, v := range tables {
		if p.bounded(v) {
			p.Stands[v].filledAt = 0
		}
	}
	return p
}

// bounded reports whether how far back the table given reads to fill its
// view through its filter is bounded: the Requests' by how far back the days'
// summaries say the lines it keeps reach, as the watch reads them, but of
// the failed, as no summary counts a day's failures; the Events' where their
// filter keeps kinds alone, as on opening, never some sessions or accounts,
// which no summary of the ledger's lines tells the days of.
func (p LogPage) bounded(v LogView) bool {
	if v == LogEvents {
		return len(p.Shows.Sessions)+len(p.Shows.Accounts) == 0
	}
	return !p.Filter.Failed
}

// Filtered reports whether the Log's filter keeps some lines alone, as the
// heading names them after "only": some sessions, accounts or models, or
// those that failed, of the Requests, or some sessions or accounts of the
// Events. What kinds they show is no such choice.
func (p LogPage) Filtered() bool {
	f, s := p.Filter, p.Shows
	return len(f.Sessions)+len(f.Accounts)+len(f.Models)+len(s.Sessions)+len(s.Accounts) > 0 || f.Failed
}

// Unfiltered is the Log with what Filtered says its filter keeps alone let
// go, as esc lets it go: the kinds each table shows kept, and each table
// refiltered.
func (p LogPage) Unfiltered() LogPage {
	p.Filter = views.LogFilter{Kinds: p.Filter.Kinds}
	p.Shows = views.EventFilter{Kinds: p.Shows.Kinds}
	return p.refiltered(LogRequests, LogEvents)
}

// The find's line's spacing: the cells after what's typed, before the match
// picked; and after the match, before its day.
const (
	findMatchGap = 3
	findDayGap   = 2
)

// The find's line's inks: what's typed, in text.primary, and the cursor
// after it, in accent.key.
var (
	typedInk  = ink{token: theme.TextPrimary}
	cursorInk = ink{token: theme.AccentKey}
)

// findLine is the find's line, width cells wide, which the state line's row
// shows in its place while the find is open, reporting false while it
// isn't: at its left, / and what's typed, the cursor after it, as "/ api▏";
// three cells on, the session picked, named as the table names a session,
// "7f3a api"; and two cells on, the day it's found on, "today", or as the
// day rule names a day, "Tue 29 Sep". At its right, ending at the last
// column, how many sessions it finds so far, and while it finds any, the
// keys that pick one: "3 sessions · ↑↓ enter". Narrowing, the keys go
// first, then the day, then the count, then the session picked; what's
// left is cut.
func (p LogPage) findLine(width int) (line, bool) {
	f := p.Finding
	if !f.Open {
		return nil, false
	}
	typed := line{{"/", keyInk}, {" " + f.Typed, typedInk}, {"▏", cursorInk}}
	named, day := typed, typed
	if s, ok := f.Pick(); ok {
		named = slices.Concat(typed, line{spaces(findMatchGap)}, sessionCell(s.ID, s.Dir, false))
		day = slices.Concat(named, line{spaces(findDayGap), {foundOn(s, p.Today), mutedInk}})
	}
	count, keyed := f.counted()
	for _, form := range [][2]line{{day, keyed}, {day, count}, {named, count}, {named, nil}} {
		if form[0].width()+headingGap+form[1].width() <= width {
			return spread(form[0], form[1], width), true
		}
	}
	return typed.fit(width), true
}

// counted is how many sessions the find finds so far, "3 sessions", the
// figure in text.secondary, or "no sessions" in text.muted; and keyed, the
// same with the keys that pick one after it, where it finds any: "3
// sessions · ↑↓ enter".
func (f LogFinding) counted() (count, keyed line) {
	n := len(f.Found.Sessions)
	if n == 0 {
		count = line{{prose.SessionCount(n), mutedInk}}
		return count, count
	}
	count = logCounted(n, "session", secondaryInk)
	return count, slices.Concat(count, line{{" · ", mutedInk}, {"↑↓", keyInk}, {" ", mutedInk}, {"enter", keyInk}})
}

// foundOn names the day the session s is found on: "today", where that's
// today, else as the day rule names a day, "Tue 29 Sep".
func foundOn(s views.FoundSession, today views.LedgerDay) string {
	if s.Day.Date == today.Date {
		return "today"
	}
	return views.DateNamed(s.Day.Date)
}
