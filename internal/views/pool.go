package views

import (
	"iter"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// PoolRead is a pool as a verb reads its work from what's recorded: the work
// of the pool with the id ID, as each request, session's start or move, or
// pin recorded its pool at the time, and what befell the accounts in it now,
// whose windows are their own whatever pool they're in. Its zero value reads
// every pool's, as of a config with no pools; NoPool's reads none.
type PoolRead struct {
	ID string
	// First is the id of the first pool, once the move to pools is done, ""
	// till then: work recorded in no pool is the first's.
	First string
	// PoolOf gives the id of the pool the account with the given id is in,
	// as status.AccountPools has it, reporting false where none places it.
	PoolOf func(account string) (string, bool)
	// none is set of a read of no pool's work, as NoPool gives it.
	none bool
}

// NoPool is a read of no pool's work at all: no request, day, window,
// session or event of any pool, the router's own events among them. It's
// the read of a dashboard that shows no pool, as where every pool is fenced
// and none is named, so it never shows every pool's work together.
func NoPool() PoolRead {
	return PoolRead{none: true}
}

// holds reports whether the account with the given id is the pool read's,
// as status.AccountIn has it of PoolOf: none is NoPool's.
func (p PoolRead) holds(account string) bool {
	return !p.none && status.AccountIn(account, p.ID, p.PoolOf)
}

// every reports whether the read is of every pool's work.
func (p PoolRead) every() bool {
	return p.ID == "" && !p.none
}

// keeps reports whether the read keeps a request that recorded the pool with
// the id recorded, "" where it named none, as ledger.RecordedIn has it of
// the pool read and the first: every one where the read is of every pool's,
// none where it's NoPool's.
func (p PoolRead) keeps(recorded string) bool {
	return p.every() || !p.none && ledger.RecordedIn(recorded, p.ID, p.First)
}

// keepsOf reports whether the read keeps any of the requests that recorded
// the pools given, as keeps has it of each, and whether it keeps them all:
// both where it reads every pool's.
func (p PoolRead) keepsOf(pools []string) (some, all bool) {
	if p.every() {
		return true, true
	}
	kept := 0
	for _, pool := range pools {
		if p.keeps(pool) {
			kept++
		}
	}
	return kept > 0, kept == len(pools)
}

// Lines are those of lines routed in the pool read, as each recorded at the
// time, as keeps has it: every one where the read is of every pool's.
func (p PoolRead) Lines(lines iter.Seq[ledger.Held]) iter.Seq[ledger.Held] {
	if p.every() {
		return lines
	}
	return func(yield func(ledger.Held) bool) {
		for h := range lines {
			if p.keeps(h.Pool) && !yield(h) {
				return
			}
		}
	}
}

// Held are those of lines routed in the pool read, in their order, as Lines
// has them: lines themselves where the read is of every pool's.
func (p PoolRead) Held(lines []ledger.Held) []ledger.Held {
	if p.every() {
		return lines
	}
	var held []ledger.Held
	for _, h := range lines {
		if p.keeps(h.Pool) {
			held = append(held, h)
		}
	}
	return held
}

// Days reads the pool's days from those l reads, as LedgerDays reads them,
// each as summary has it: every day whole where the read is of every pool's.
func (p PoolRead) Days(l LedgerDays) LedgerDays {
	if p.every() {
		return l
	}
	return poolDays{read: p, days: l}
}

// Dates reads the pool's days by their dates from those l reads, as
// LedgerDates reads them, each as summary has it: every day whole where the
// read is of every pool's.
func (p PoolRead) Dates(l LedgerDates) LedgerDates {
	if p.every() {
		return l
	}
	return poolDates{read: p, dates: l}
}

// summaries are the days read, each as summary has it, as new summaries,
// leaving those read as they were.
func (p PoolRead) summaries(read []ledger.Summary) []ledger.Summary {
	days := make([]ledger.Summary, len(read))
	for i, s := range read {
		days[i] = p.summary(s)
	}
	return days
}

// summary is the day s in the pool read alone: its work in the pool alone,
// as ledger.Summary's InPool gives it; and of its accounts, those that
// worked in the pool that day, or are the pool's, as holds has it, for their
// windows. NoPool's holds none of its accounts.
func (p PoolRead) summary(s ledger.Summary) ledger.Summary {
	if p.none {
		s.Accounts = nil
		return s
	}
	s = s.InPool(p.ID, p.First)
	s.Accounts = slices.DeleteFunc(s.Accounts, func(a ledger.AccountDay) bool {
		return a.Idle() && !p.holds(a.Account)
	})
	return s
}

// poolDays reads a pool's days, as PoolRead's Days has them.
type poolDays struct {
	read PoolRead
	days LedgerDays
}

func (d poolDays) Days(from time.Time) []ledger.Summary {
	return d.read.summaries(d.days.Days(from))
}

// poolDates reads a pool's days by their dates, as PoolRead's Dates has
// them.
type poolDates struct {
	read  PoolRead
	dates LedgerDates
}

func (d poolDates) DaysOn(dates []string) []ledger.Summary {
	return d.read.summaries(d.dates.DaysOn(dates))
}

// Windows are those of w of the accounts the pool read holds, as holds has
// it, an account's windows being its own whatever pool it's in: every one
// where the read is of every pool's.
func (p PoolRead) Windows(w Windows) Windows {
	if p.every() {
		return w
	}
	return Windows{Windows: slices.DeleteFunc(slices.Clone(w.Windows), func(r WindowReadings) bool { return !p.holds(r.Account) })}
}

// Shows reports whether the event e is of the pool read, as status.Event's
// InPool has it: every event is, where the read is of every pool's, and
// none, where it's NoPool's.
func (p PoolRead) Shows(e status.Event) bool {
	return p.every() || !p.none && e.InPool(p.ID, p.First, p.PoolOf)
}

// ShowsHeld reports whether the event a line of the router's events holds is
// of the pool read, as Shows has it.
func (p PoolRead) ShowsHeld(h events.Held) bool {
	return p.Shows(h.Event)
}

// Events are those of held the pool read shows, as Shows has it.
func (p PoolRead) Events(held iter.Seq[events.Held]) iter.Seq[events.Held] {
	if p.every() {
		return held
	}
	return func(yield func(events.Held) bool) {
		for h := range held {
			if p.ShowsHeld(h) && !yield(h) {
				return
			}
		}
	}
}

// Filed reads the router's events through e as the pool read has them: those
// it shows, as Shows has it, so each read of the files, a first, one back to
// an earlier day, or one again, reads the pool's alone; e itself where the
// read is of every pool's, or e is nil.
func (p PoolRead) Filed(e Events) Events {
	if p.every() || e == nil {
		return e
	}
	return poolEvents{Events: e, read: p}
}

// poolEvents reads a pool's events, as PoolRead's Filed has them.
type poolEvents struct {
	Events
	read PoolRead
}

func (e poolEvents) Between(from, to time.Time) iter.Seq[events.Line] {
	return func(yield func(events.Line) bool) {
		for l := range e.Events.Between(from, to) {
			if e.read.Shows(l.Event) && !yield(l) {
				return
			}
		}
	}
}

// Ledger reads l as the pool read has it, for the sessions' List: each
// session's lines those routed in the pool alone, as keeps has it, so its
// requests and their worth are those it made in the pool, and one with none
// in the pool, resting on no state, none of the pool's; l itself where the
// read is of every pool's.
func (p PoolRead) Ledger(l Ledger) Ledger {
	if p.every() {
		return l
	}
	return poolLedger{Ledger: l, read: p}
}

// poolLedger reads a pool's sessions, as PoolRead's Ledger has them.
type poolLedger struct {
	Ledger
	read PoolRead
}

func (l poolLedger) Sessions(ids []string, rest func(id string) ledger.Rest, take func(date string, line ledger.Line)) map[string]ledger.SessionFrom {
	return l.inPool(l.Ledger.Sessions(ids, rest, l.taking(take)))
}

func (l poolLedger) SessionsOf(ids []string, rest func(id string) ledger.Rest, take func(date string, line ledger.Line)) map[string]ledger.SessionFrom {
	return l.inPool(l.Ledger.SessionsOf(ids, rest, l.taking(take)))
}

// taking is take, handed the lines routed in the pool alone: nil where take
// is.
func (l poolLedger) taking(take func(date string, line ledger.Line)) func(date string, line ledger.Line) {
	if take == nil {
		return nil
	}
	return func(date string, line ledger.Line) {
		if l.read.keeps(line.Pool) {
			take(date, line)
		}
	}
}

// inPool returns the sessions given, each with its lines routed in the pool
// alone, as new slices, leaving those given as they are: none of one with no
// such line that rests on no state, nor of NoPool's any.
func (l poolLedger) inPool(sessions map[string]ledger.SessionFrom) map[string]ledger.SessionFrom {
	in := make(map[string]ledger.SessionFrom, len(sessions))
	for id, s := range sessions {
		var lines []ledger.Line
		for _, line := range s.Lines {
			if l.read.keeps(line.Pool) {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 && (s.Base == nil || l.read.none) {
			continue
		}
		s.Lines = lines
		in[id] = s
	}
	return in
}
