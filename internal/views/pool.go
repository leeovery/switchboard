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
// every pool's, as of a config with no pools.
type PoolRead struct {
	ID string
	// First is the id of the first pool, once the move to pools is done, ""
	// till then: work recorded in no pool is the first's.
	First string
	// PoolOf gives the id of the pool the account with the given id is in,
	// as status.AccountPools has it, reporting false where none places it.
	PoolOf func(account string) (string, bool)
}

// holds reports whether the account with the given id is the pool read's,
// as status.AccountIn has it of PoolOf.
func (p PoolRead) holds(account string) bool {
	return status.AccountIn(account, p.ID, p.PoolOf)
}

// every reports whether the read is of every pool's work.
func (p PoolRead) every() bool {
	return p.ID == ""
}

// Lines are those of lines routed in the pool read, as each recorded at the
// time, ledger.Line's InPool has it: every one where the read is of every
// pool's.
func (p PoolRead) Lines(lines iter.Seq[ledger.Held]) iter.Seq[ledger.Held] {
	if p.every() {
		return lines
	}
	return func(yield func(ledger.Held) bool) {
		for h := range lines {
			if h.InPool(p.ID, p.First) && !yield(h) {
				return
			}
		}
	}
}

// Days reads the pool's days from those l reads, as LedgerDays reads them:
// each day's work in the pool alone, as ledger.Summary's InPool gives it,
// and of its accounts, those that worked in the pool that day, or are the
// pool's, as holds has it, for their windows: every day whole where the read
// is of every pool's.
func (p PoolRead) Days(l LedgerDays) LedgerDays {
	if p.every() {
		return l
	}
	return poolDays{read: p, days: l}
}

// poolDays reads a pool's days, as PoolRead's Days has them.
type poolDays struct {
	read PoolRead
	days LedgerDays
}

func (d poolDays) Days(from time.Time) []ledger.Summary {
	read := d.days.Days(from)
	days := make([]ledger.Summary, len(read))
	for i, s := range read {
		s = s.InPool(d.read.ID, d.read.First)
		s.Accounts = slices.DeleteFunc(s.Accounts, func(a ledger.AccountDay) bool {
			return a.Idle() && !d.read.holds(a.Account)
		})
		days[i] = s
	}
	return days
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
// InPool has it: every event is, where the read is of every pool's.
func (p PoolRead) Shows(e status.Event) bool {
	return p.every() || e.InPool(p.ID, p.First, p.PoolOf)
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
// session's lines those routed in the pool alone, as ledger.Line's InPool
// has it, so its requests and their worth are those it made in the pool,
// and one with none in the pool, resting on no state, none of the pool's; l
// itself where the read is of every pool's.
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
		if line.InPool(l.read.ID, l.read.First) {
			take(date, line)
		}
	}
}

// inPool returns the sessions given, each with its lines routed in the pool
// alone, as new slices, leaving those given as they are: none of one with no
// such line that rests on no state.
func (l poolLedger) inPool(sessions map[string]ledger.SessionFrom) map[string]ledger.SessionFrom {
	in := make(map[string]ledger.SessionFrom, len(sessions))
	for id, s := range sessions {
		var lines []ledger.Line
		for _, line := range s.Lines {
			if line.InPool(l.read.ID, l.read.First) {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 && s.Base == nil {
			continue
		}
		s.Lines = lines
		in[id] = s
	}
	return in
}
