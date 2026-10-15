package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// LogPanel is the Log's panel of what one of its tables shows, as f opens it
// over the page, which the watch builds afresh at each change while it's
// open, so its counts follow the lines: its two sides, Requests' COLUMNS and
// ONLY, or Events' kinds and ONLY, where the cursor is on each and which
// it's on; what it picks, which enter applies; and what that would show.
type LogPanel struct {
	// Of is the table it's of, which its title names.
	Of LogView
	// Laid are the Requests' columns as they showed as it opened, which lay
	// COLUMNS out, so a tick never moves as it's ticked.
	Laid LogTicks
	// Columns, Filter and Shows are what it picks, as enter applies them.
	Columns LogTicks
	Filter  views.LogFilter
	Shows   views.EventFilter
	// Lines count today's lines, its kinds, models and sessions, Shown those
	// Filter shows, as the Requests would count them through it; Events
	// count today's events, Shown those Shows shows.
	Lines  views.LogCount
	Events views.EventCount
	// Accounts are every account, in the config's order, as ONLY ticks
	// them.
	Accounts []status.AccountNow
	// Sessions are today's sessions as views.LogSessions lists them: of
	// Requests' panel, those with lines today; of Events', those with events
	// today; and on either, those TickedSessions has that have none.
	Sessions []views.LogSession
	// Now is when it was built, which its sessions' doing is said at.
	Now time.Time
	// side is the side the cursor's on, 0 the left and 1 the right; on is
	// the tick it's on on each, the first where it's on none there is.
	side int
	on   [2]tickOn
	// openedFilter and openedShows are what it picked as it opened, whose
	// sessions and models it lists though today counts none of them, so one
	// let go stays to be ticked again.
	openedFilter views.LogFilter
	openedShows  views.EventFilter
}

// Panel is the panel f opens of the table p shows: picking what it shows,
// the cursor on its left side's first tick.
func (p LogPage) Panel() LogPanel {
	return LogPanel{
		Of: p.Shown, Laid: p.Columns, Columns: p.Columns, Filter: p.Filter, Shows: p.Shows, openedFilter: p.Filter, openedShows: p.Shows,
	}
}

// TickedSessions are the sessions its table's filter ticks, as it opened or
// as it picks, each by its whole id, which it lists whether today counts
// any of their lines or events or not.
func (p LogPanel) TickedSessions() []string {
	return slices.Concat(sessionsOf(p.Of, p.openedFilter, p.openedShows), p.chosenSessions())
}

// chosenSessions are the sessions its table's filter ticks as it picks.
func (p LogPanel) chosenSessions() []string {
	return sessionsOf(p.Of, p.Filter, p.Shows)
}

// sessionsOf are the sessions the filter of the table given ticks: of the
// Events, shows'; else filter's.
func sessionsOf(of LogView, filter views.LogFilter, shows views.EventFilter) []string {
	if of == LogEvents {
		return shows.Sessions
	}
	return filter.Sessions
}

// Applied is p showing what the panel picks, as enter applies it, the table
// it's of refiltered.
func (p LogPage) Applied(panel LogPanel) LogPage {
	p.Columns, p.Filter, p.Shows = panel.Columns, panel.Filter, panel.Shows
	return p.refiltered(panel.Of)
}

// tickGroup is what a tick of the panel's ticks: a column, a kind of event,
// an account, a model, a kind of line, failed lines alone, or a session.
type tickGroup int

const (
	columnTick tickGroup = iota
	eventKindTick
	accountTick
	modelTick
	rowKindTick
	failedTick
	sessionTick
)

// tickOn is the tick the cursor is on: its group, and of it, by its name or
// id, which one.
type tickOn struct {
	group tickGroup
	id    string
}

// tick is one of the panel's ticks: what it ticks; its name, ticked or not;
// what follows its name, as a count; and how ticking it changes the panel.
type tick struct {
	on     tickOn
	name   string
	ticked bool
	after  line
	toggle func(LogPanel) LogPanel
}

// The inks of a tick: ticked, ■ bold accent.key and its name text.primary;
// else □ text.subtle and its name text.muted.
var tickedNameInk = ink{token: theme.TextPrimary}

// tickBox are the cells a tick's box takes, the blank after it included,
// before its name.
const tickBox = 2

// words are the tick as the panel draws it: its box, ticked or not, its name
// and what follows it.
func (t tick) words() line {
	if t.ticked {
		return slices.Concat(line{{"■ ", keyInk}, {t.name, tickedNameInk}}, t.after)
	}
	return slices.Concat(line{{"□ ", dimInk}, {t.name, mutedInk}}, t.after)
}

// Moved is the panel, its cursor moved by places through the ticks of the
// side it's on, as ↑ and ↓ move it, in their reading order: no further than
// the first or the last.
func (p LogPanel) Moved(by int) LogPanel {
	ticks := p.sideTicks(p.side)
	if len(ticks) == 0 {
		return p
	}
	p.on[p.side] = ticks[min(max(p.cursor(ticks)+by, 0), len(ticks)-1)].on
	return p
}

// Sided is the panel, its cursor on the side by places on from the one it's
// on, ← -1 and → 1, as ←→ move it, where it stood there: no further than the
// left or the right.
func (p LogPanel) Sided(by int) LogPanel {
	p.side = min(max(p.side+by, 0), 1)
	return p
}

// Ticked is the panel, the tick the cursor's on ticked where it isn't, and
// not where it is, as space ticks it.
func (p LogPanel) Ticked() LogPanel {
	ticks := p.sideTicks(p.side)
	if len(ticks) == 0 {
		return p
	}
	return ticks[p.cursor(ticks)].toggle(p)
}

// cursor is the place among ticks of the one the cursor's on: the first,
// where it's on none of them.
func (p LogPanel) cursor(ticks []tick) int {
	return max(slices.IndexFunc(ticks, func(t tick) bool { return t.on == p.on[p.side] }), 0)
}

// sideTicks are the ticks of the side given, in their reading order: the
// left's, Requests' columns or Events' kinds; the right's, ONLY's rows'
// ticks, then today's sessions'.
func (p LogPanel) sideTicks(side int) []tick {
	if side == 0 {
		if p.Of == LogEvents {
			return p.eventKindTicks()
		}
		return p.columnTicks()
	}
	var ticks []tick
	for _, row := range p.choiceRows() {
		ticks = append(ticks, row.ticks...)
	}
	return append(ticks, p.sessionTicks()...)
}

// laidColumns are COLUMNS' columns, in its order: those shown as it opened,
// in the table's order, then those hidden, at aside, as it always shows.
func (p LogPanel) laidColumns() (shown, hidden []LogColumn) {
	for _, c := range EveryLogColumn()[1:] {
		if p.Laid.Shows(c) {
			shown = append(shown, c)
			continue
		}
		hidden = append(hidden, c)
	}
	return shown, hidden
}

// columnTicks are COLUMNS' ticks, in its order, each ticked where it shows.
func (p LogPanel) columnTicks() []tick {
	shown, hidden := p.laidColumns()
	var ticks []tick
	for _, c := range slices.Concat(shown, hidden) {
		ticks = append(ticks, tick{
			on: tickOn{columnTick, c.Name()}, name: c.Name(), ticked: p.Columns.Shows(c),
			toggle: func(p LogPanel) LogPanel { p.Columns = p.Columns.Toggled(c); return p },
		})
	}
	return ticks
}

// eventKindTicks are Events' kinds' ticks, in the design's order, each named
// as the Events name it, ticked where Shows shows it.
func (p LogPanel) eventKindTicks() []tick {
	every := make([]string, len(eventKinds))
	for i, k := range eventKinds {
		every[i] = k.kind
	}
	ticks := make([]tick, len(eventKinds))
	for i, k := range every {
		ticks[i] = tick{
			on: tickOn{eventKindTick, k}, name: views.KindWord(k), ticked: views.Ticks(p.Shows.Kinds, k),
			toggle: func(p LogPanel) LogPanel { p.Shows.Kinds = views.Toggled(p.Shows.Kinds, every, k); return p },
		}
	}
	return ticks
}

// choiceRow is a row of ONLY: its label, and its ticks, or words in their
// place.
type choiceRow struct {
	label string
	ticks []tick
	words line
}

// choiceRows are ONLY's rows: of Requests', accounts, models, kinds, failed
// and sessions; of Events', accounts and sessions. Sessions' says what none
// ticked keeps, today's sessions listed under it.
func (p LogPanel) choiceRows() []choiceRow {
	rows := []choiceRow{{label: "accounts", ticks: p.accountTicks()}}
	if p.Of == LogRequests {
		rows = append(rows,
			choiceRow{label: "models", ticks: p.modelTicks()},
			choiceRow{label: "kinds", ticks: p.rowKindTicks()},
			choiceRow{label: "failed", ticks: []tick{p.failedTick()}},
		)
	}
	return append(rows, choiceRow{label: "sessions", words: line{{"every one, unless some are ticked", mutedInk}}})
}

// accountTicks are every account's tick, in the config's order, ticked where
// the table's filter keeps its lines, or its events.
func (p LogPanel) accountTicks() []tick {
	every := make([]string, len(p.Accounts))
	for i, a := range p.Accounts {
		every[i] = a.ID
	}
	chosen := p.Filter.Accounts
	if p.Of == LogEvents {
		chosen = p.Shows.Accounts
	}
	ticks := make([]tick, len(p.Accounts))
	for i, a := range p.Accounts {
		ticks[i] = tick{
			on: tickOn{accountTick, a.ID}, name: name(a), ticked: views.Ticks(chosen, a.ID),
			toggle: func(p LogPanel) LogPanel {
				if p.Of == LogEvents {
					p.Shows.Accounts = views.Toggled(p.Shows.Accounts, every, a.ID)
					return p
				}
				p.Filter.Accounts = views.Toggled(p.Filter.Accounts, every, a.ID)
				return p
			},
		}
	}
	return ticks
}

// modelTicks are the ticks of the models today's lines asked for, and of
// those its filter ticks, as it opened or as it picks, that none asked for,
// in the order LogCount's ModelsListed gives, each named by its id less
// claude-, ticked where the filter keeps its lines.
func (p LogPanel) modelTicks() []tick {
	every := p.Lines.ModelsListed(slices.Concat(p.openedFilter.Models, p.Filter.Models))
	ticks := make([]tick, len(every))
	for i, m := range every {
		ticks[i] = tick{
			on: tickOn{modelTick, m}, name: modelNamed(m), ticked: views.Ticks(p.Filter.Models, m),
			toggle: func(p LogPanel) LogPanel { p.Filter.Models = views.Toggled(p.Filter.Models, every, m); return p },
		}
	}
	return ticks
}

// rowKindTicks are the kinds' ticks, each with today's lines of it, ticked
// where the filter keeps them.
func (p LogPanel) rowKindTicks() []tick {
	counts := map[views.RowKind]int{views.RowRequest: p.Lines.Requests, views.RowCheck: p.Lines.Checks, views.RowCount: p.Lines.Counts}
	every := views.RowKinds()
	ticks := make([]tick, len(every))
	for i, k := range every {
		ticks[i] = tick{
			on: tickOn{rowKindTick, rowKindNames[k]}, name: rowKindNames[k], ticked: views.Ticks(p.Filter.Kinds, k),
			after:  line{{" " + prose.Grouped(counts[k]), mutedInk}},
			toggle: func(p LogPanel) LogPanel { p.Filter.Kinds = views.Toggled(p.Filter.Kinds, every, k); return p },
		}
	}
	return ticks
}

// failedTick is the tick keeping only the lines that failed, with today's
// lines that did.
func (p LogPanel) failedTick() tick {
	return tick{
		on: tickOn{group: failedTick}, name: "only those that failed", ticked: p.Filter.Failed,
		after:  line{{" " + prose.Grouped(p.Lines.Failed), mutedInk}},
		toggle: func(p LogPanel) LogPanel { p.Filter.Failed = !p.Filter.Failed; return p },
	}
}

// sessionTicks are today's sessions' ticks, as Sessions lists them, each
// ticked where the table's filter ticks it: none at first, as none ticked
// keeps every one.
func (p LogPanel) sessionTicks() []tick {
	chosen := p.chosenSessions()
	ticks := make([]tick, len(p.Sessions))
	for i, s := range p.Sessions {
		ticks[i] = tick{
			on: tickOn{sessionTick, s.ID}, ticked: slices.Contains(chosen, s.ID), after: p.sessionWords(s),
			toggle: func(p LogPanel) LogPanel {
				if p.Of == LogEvents {
					p.Shows.Sessions = toggledIn(p.Shows.Sessions, s.ID)
					return p
				}
				p.Filter.Sessions = toggledIn(p.Filter.Sessions, s.ID)
				return p
			},
		}
	}
	return ticks
}

// toggledIn is choice with one taken out where it's in it, else added.
func toggledIn(choice []string, one string) []string {
	if i := slices.Index(choice, one); i >= 0 {
		return slices.Delete(slices.Clone(choice), i, i+1)
	}
	return append(slices.Clone(choice), one)
}

// The columns of a session's row, from its tick: its directory, the account
// it's on, its count ending, and what it's doing; and the cells it takes at
// its widest, as started 14:41 says. Its id follows its box.
const (
	sessionDirAt     = 8
	sessionAccountAt = 22
	sessionCountEnd  = 35
	sessionDoingAt   = 44
	sessionCells     = sessionDoingAt + 13
)

// sessionWords are a session's row after its tick: its id in
// text.secondary; its directory's name, padded, in text.muted; the account
// it's on, padded, in text.primary; its lines today, or of Events', its
// events, the figure ending at its column, in text.secondary and the rest
// in text.muted; and what it's doing.
func (p LogPanel) sessionWords(s views.LogSession) line {
	count := line{{prose.Grouped(s.Lines), secondaryInk}, {" today", mutedInk}}
	if p.Of == LogEvents {
		count = countedSaid(s.Events, "event", secondaryInk, mutedInk)
	}
	words := line{{status.ShownID(s.ID), secondaryInk}}
	words = padTo(words, sessionDirAt-tickBox, line{{status.DirName(s.Dir, views.SessionDirCells), mutedInk}})
	words = padTo(words, sessionAccountAt-tickBox, line{{prose.Cut(p.named(s.Account), logAccountCells), accountInk}})
	words = padTo(words, sessionCountEnd-tickBox-count[:1].width(), count)
	if doing := sessionDoing(s, p.Now); doing.text != "" {
		words = padTo(words, sessionDoingAt-tickBox, line{doing})
	}
	return words
}

// logAccountCells are the most cells an account's name takes in the panel's
// sessions' rows, cut with an ellipsis past them.
const logAccountCells = 8

// named is what the panel calls the account with the given id: its name, of
// those it ticks, else its id, cleaned; "" for none.
func (p LogPanel) named(id string) string {
	if i := slices.IndexFunc(p.Accounts, func(a status.AccountNow) bool { return a.ID == id }); i >= 0 {
		return name(p.Accounts[i])
	}
	return prose.Clean(id)
}

// sessionDoing is what a session's row says it's doing at now, as its Doing
// has it: asking in accent.key, or as last known as asKnown inks it;
// started, idle or ended, and when, in text.muted; nothing while it's busy.
func sessionDoing(s views.LogSession, now time.Time) span {
	switch doing, since := s.Doing(now); doing {
	case views.LogAsking:
		return span{"asking", asKnown(s.Activity, doingInk)}
	case views.LogStarted:
		return span{"started " + prose.TimeOfDay(now, since), mutedInk}
	case views.LogIdle:
		if since.IsZero() {
			return span{"idle", mutedInk}
		}
		return span{"idle " + prose.SeenAgo(now, since), mutedInk}
	case views.LogEnded:
		return span{"ended " + prose.TimeOfDay(now, since), mutedInk}
	}
	return span{}
}
