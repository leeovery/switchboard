package views

import (
	"iter"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
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
// the pools given, one at least, as ledger.RecordedPools restores a head's,
// or a digest's entry holds them, as keeps has it of each, and whether it
// keeps them all: both where it reads every pool's.
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

// mayHoldOn reports whether the local day with the given date may hold a
// line the read keeps, as the summaries l holds of the days whose files may
// hold its lines, as dayfile's FileDates has them, the files Day reads them
// of, tell of the pool's work, as summary has it of each and the zero
// LogFilter's mayKeep reads it: every day of every pool's read; none of
// NoPool's, nor of a date that isn't one; and every day where one of those
// days has lines but no summary that stands, as today's, as l's DaysHeldOn
// tells, so it summarises none from its lines to tell.
func (p PoolRead) mayHoldOn(l LedgerHeld, date string) bool {
	switch {
	case p.every():
		return true
	case p.none:
		return false
	}
	day, ok := dayfile.DayBounds(date)
	if !ok {
		return false
	}
	held, told := l.DaysHeldOn(day.FileDates())
	return !told || slices.ContainsFunc(p.summaries(held), LogFilter{}.through().mayKeep)
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
// has them: lines themselves where the read keeps every one, as every
// pool's does, and a pool's of a ledger of its work alone, none copied.
func (p PoolRead) Held(lines []ledger.Held) []ledger.Held {
	return keptIn(p, lines, func(h *ledger.Held) string { return h.Pool })
}

// keptIn are those of lines routed in the pool read p, in their order, as
// Held has them, each line's pool as pool reads it.
func keptIn[L any](p PoolRead, lines []L, pool func(*L) string) []L {
	if p.every() {
		return lines
	}
	for i := range lines {
		if !p.keeps(pool(&lines[i])) {
			return keptPast(p, lines, i, pool)
		}
	}
	return lines
}

// keptPast are those of lines routed in the pool read p, as keptIn has
// them, the read dropping the one at dropped and keeping each before it:
// those before it, and those after it the read keeps, copied.
func keptPast[L any](p PoolRead, lines []L, dropped int, pool func(*L) string) []L {
	held := slices.Clone(lines[:dropped])
	for i := dropped + 1; i < len(lines); i++ {
		if p.keeps(pool(&lines[i])) {
			held = append(held, lines[i])
		}
	}
	return held
}

// Took decodes a read of the ledger's tail for the store, as the package's
// Took does, the requests of the pool read alone taken, as Held has them:
// every line kept for a page's points all the same, as owned hands each.
func (p PoolRead) Took(gained []ledger.Held, days map[string][]ledger.Held) Taken {
	t := Taken{gained: takenOf(p.Held(gained)), days: make(map[string]takenLines, len(days)), read: tailRead{gained: p.ownedAll(gained), days: make(map[string][]ledger.Held, len(days))}}
	for date, lines := range days {
		t.days[date], t.read.days[date] = takenOf(p.Held(lines)), p.ownedAll(lines)
	}
	return t
}

// owned is the line l as the pool read hands it to a page's points, which
// count every request on an account, of every pool, as its windows do: of
// no session where it was routed in another pool, so a session's own are
// its requests in the pool alone.
func (p PoolRead) owned(l ledger.Line) ledger.Line {
	if !p.keeps(l.Pool) {
		l.Session = ""
	}
	return l
}

// ownedAll are lines, each as owned has it: lines themselves where the
// read keeps every one, as every pool's does.
func (p PoolRead) ownedAll(lines []ledger.Held) []ledger.Held {
	i := slices.IndexFunc(lines, func(h ledger.Held) bool { return !p.keeps(h.Pool) })
	if i < 0 {
		return lines
	}
	owned := slices.Clone(lines)
	for ; i < len(owned); i++ {
		owned[i].Line = p.owned(owned[i].Line)
	}
	return owned
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

// Start tells where the pool read's work begins, as l tells where the
// ledger begins, for the days History, Accounts, Runway and Graphs read to
// begin there: l itself where the read is of every pool's; else the first
// day the ledger holds whose summary in the pool holds work, so a pool added
// to a ledger months old never counts the days before it as days of none.
// Nil where l can't tell where it begins.
func (p PoolRead) Start(l Ledger) LedgerStart {
	start, ok := l.(LedgerStart)
	if !ok || p.every() {
		return start
	}
	return poolStart{start: start, dates: p.Dates(l)}
}

// poolStart tells where a pool's work begins, as PoolRead's Start has it.
type poolStart struct {
	start LedgerStart
	dates LedgerDates
}

// poolStartRead is how many days poolStart reads at a time, from the first
// the ledger holds, till it finds the pool's first.
const poolStartRead = 31

// FirstDay gives the start of the first local day the ledger holds whose
// summary in the pool holds work, at now, read from the first it holds,
// failing as it fails to say; today's where no day before today's does. A
// pool yet to work so begins with today as a ledger yet to hold a day does,
// but isn't asked again at each read, as that would read every day's
// summary each time: one that first works while the dashboard is open
// begins where it was opened, till it's opened again.
func (s poolStart) FirstDay(now time.Time) (time.Time, error) {
	first, err := s.start.FirstDay(now)
	if err != nil {
		return first, err
	}
	today := dayfile.DayStart(now.Local(), 0)
	for dates := range slices.Chunk(dayfile.Span(first, dayfile.DayStart(today, -1)), poolStartRead) {
		for _, day := range s.dates.DaysOn(dates) {
			if workedOn(day) {
				start, _, _ := dayfile.Day(day.Day)
				return start, nil
			}
		}
	}
	return today, nil
}

// workedOn reports whether any account's day the summary s holds has work.
func workedOn(s ledger.Summary) bool {
	return slices.ContainsFunc(s.Accounts, func(a ledger.AccountDay) bool { return !a.Idle() })
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

// Ledger reads l as the pool read has it, for the sessions the store holds,
// the List, a session's page and the Log's telling of its rows among them:
// each session as a read of its lines routed in the pool alone, as keeps
// has it, would hold it, so its story, its requests and their worth, and
// its page are those of its work in the pool, and one with none there none
// of the pool's. A state a digest holds is rested on only where the lines
// it was folded from were all the pool's, as its entry's pools tell; one
// none of whose lines were is passed over, the session's lines after it
// being all it holds of the pool; and one the pool shares with another is
// read whole instead, as inPool has it. A reader taking every line read, as
// a page's points take every account's answers, is handed every pool's, as
// owned hands each. l itself where the read is of every pool's, or l is
// nil.
func (p PoolRead) Ledger(l Ledger) Ledger {
	if p.every() || l == nil {
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
	return l.inPoolRead(l.Ledger.Sessions, ids, rest, take)
}

func (l poolLedger) SessionsOf(ids []string, rest func(id string) ledger.Rest, take func(date string, line ledger.Line)) map[string]ledger.SessionFrom {
	return l.inPoolRead(l.Ledger.SessionsOf, ids, rest, take)
}

// sessionsReading reads sessions, as LedgerSessions' Sessions and
// SessionsOf do.
type sessionsReading func(ids []string, rest func(id string) ledger.Rest, take func(date string, line ledger.Line)) map[string]ledger.SessionFrom

// inPoolRead returns the sessions read gives of the ids given, each resting
// as rest says of it and take handed what it hands, as owned has each, each
// in the pool alone, as inPool has it: those whose state the pool shares
// with another read again whole, through the ledger read, take handed then
// the lines of the days read hadn't handed it, so it's handed each day's
// once.
func (l poolLedger) inPoolRead(read sessionsReading, ids []string, rest func(id string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	take, handed := l.owning(take), make(map[string]bool)
	in, shared := l.inPool(read(ids, rest, handing(take, handed)))
	if len(shared) > 0 {
		whole, _ := l.inPool(l.Ledger.SessionsOf(shared, eachWhole, unhanded(take, handed)))
		maps.Copy(in, whole)
	}
	return in
}

// owning is take, handed each line as owned has it: nil where take is.
func (l poolLedger) owning(take func(string, ledger.Line)) func(string, ledger.Line) {
	if take == nil {
		return nil
	}
	return func(date string, line ledger.Line) {
		take(date, l.read.owned(line))
	}
}

// handing is take, noting in handed the date of each day it's handed a
// line of: nil where take is, so a read that takes nothing decodes no more
// than it keeps.
func handing(take func(string, ledger.Line), handed map[string]bool) func(string, ledger.Line) {
	if take == nil {
		return nil
	}
	return func(date string, line ledger.Line) {
		handed[date] = true
		take(date, line)
	}
}

// unhanded is take, handed the lines of the days handed doesn't note alone:
// nil where take is.
func unhanded(take func(string, ledger.Line), handed map[string]bool) func(string, ledger.Line) {
	if take == nil {
		return nil
	}
	return func(date string, line ledger.Line) {
		if !handed[date] {
			take(date, line)
		}
	}
}

// inPool returns the sessions given, each in the pool alone, leaving those
// given as they are: resting on its state where the lines it was folded
// from, as its entry's pools tell them, were all the pool's, as keepsOf has
// it; on none, its parts with it, where none of them were, as all it holds
// of the pool is in its lines after it; and its lines those routed in the
// pool, as keeps has it, the slice given where they all are. None of one
// with no such line that rests on none, nor of NoPool's any. It returns too
// the ids of those whose state the pool shares with another, in order,
// none of them held: what's folded in a state, its story and its page, is
// one fold of every line it rests on, which can't be told apart by pool,
// so their lines are read instead.
func (l poolLedger) inPool(sessions map[string]ledger.SessionFrom) (map[string]ledger.SessionFrom, []string) {
	in := make(map[string]ledger.SessionFrom, len(sessions))
	var shared []string
	for id, s := range sessions {
		if s.Base != nil {
			switch some, all := l.read.keepsOf(s.Base.Pools); {
			case !some:
				s.Base, s.Parts = nil, nil
			case !all:
				shared = append(shared, id)
				continue
			}
		}
		if s.Lines = keptIn(l.read, s.Lines, func(line *ledger.Line) string { return line.Pool }); len(s.Lines) == 0 {
			s.Lines = nil
		}
		if s.Lines != nil || s.Base != nil {
			in[id] = s
		}
	}
	slices.Sort(shared)
	return in, shared
}
