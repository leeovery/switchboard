package views

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// LogFilter is what the Log's Requests show, as its f panel picks it. A
// choice ticked keeps those ticked; none ticked is every one: so the zero
// LogFilter shows every line, and the Log opens on LogOpening's.
type LogFilter struct {
	// Accounts, Models and Sessions keep the lines of the accounts, the
	// models and the sessions ticked, each by its whole id, a line's session
	// the one it counts toward: a line of none, as a request the router
	// answered itself is on no account, and a quota check counts toward no
	// session, is kept by none ticked.
	Accounts, Models, Sessions []string
	// Kinds keeps the lines of what the kinds ticked asked.
	Kinds []RowKind
	// Failed keeps only those that failed.
	Failed bool
}

// LogOpening is the filter the Log opens on, as the filters aren't
// remembered: its requests alone, Claude Code's quota checks and its counts
// of tokens hidden until they're shown.
func LogOpening() LogFilter {
	return LogFilter{Kinds: []RowKind{RowRequest}}
}

// rowKinds are the kinds of RowKind, in the panel's order.
var rowKinds = [...]RowKind{RowRequest, RowCheck, RowCount}

// Equal reports whether f and o tick the same, each choice in the same
// order.
func (f LogFilter) Equal(o LogFilter) bool {
	return slices.Equal(f.Accounts, o.Accounts) && slices.Equal(f.Models, o.Models) && slices.Equal(f.Sessions, o.Sessions) &&
		slices.Equal(f.Kinds, o.Kinds) && f.Failed == o.Failed
}

// Reach is the filter of the ledger's lines whose reach, as KeptFrom reads
// it, bounds how far back the Requests read through f, and whether one does:
// f itself, of every filter but one keeping the failed of no session. No
// summary counts a day's failures, so the failed reach as far back as every
// line of their accounts, models and kinds, likely every day kept, where a
// session's reach its days alone.
func (f LogFilter) Reach() (LogFilter, bool) {
	return f, !f.Failed || len(f.Sessions) > 0
}

// KeptFrom is the date of the earliest of the days summaries are of whose
// summary may hold a line f keeps: an account's day of a model's, each
// ticked, of a kind ticked, and naming a session ticked, or not naming every
// session it counts; "" where none may. Of a session alone, it's the
// session's first day. No summary counts a day's failures, so what f keeps
// of the failed alone it bounds as it does all of them.
func (f LogFilter) KeptFrom(summaries []ledger.Summary) string {
	from := ""
	for _, s := range summaries {
		if f.mayKeep(s) && (from == "" || s.Day < from) {
			from = s.Day
		}
	}
	return from
}

// mayKeep reports whether the day s summarises may hold a line f keeps, as
// KeptFrom reads it.
func (f LogFilter) mayKeep(s ledger.Summary) bool {
	for _, a := range s.Accounts {
		if !ticked(f.Accounts, a.Account) {
			continue
		}
		for _, m := range a.Models {
			if ticked(f.Models, m.Model) && f.keepsKindOf(m) && (tickedAny(f.Sessions, m.SessionIDs) || m.Unnamed() > 0) {
				return true
			}
		}
	}
	return false
}

// keepsKindOf reports whether f keeps lines of a kind the model's day m
// counts any of.
func (f LogFilter) keepsKindOf(m ledger.ModelDay) bool {
	return m.Requests() > 0 && ticked(f.Kinds, RowRequest) || m.Checks > 0 && ticked(f.Kinds, RowCheck) || m.Counts > 0 && ticked(f.Kinds, RowCount)
}

// ThroughOn is the filter the Log reads the local day with the given date
// back through, as f shows the Requests, s telling what's under way: f with
// every kind, as through has it; but whole, the zero LogFilter, where s
// tells of a request whose line may be filed on the day, as tellsOfLineOn
// has it, so its line is read whatever f keeps.
func (f LogFilter) ThroughOn(s Stream, date string) LogFilter {
	if tellsOfLineOn(s, date) {
		return LogFilter{}
	}
	return f.through()
}

// tellsOfLineOn reports whether s tells of a request whose line may be
// filed on the local day with the given date, as Told's linedOn has the
// days of one.
func tellsOfLineOn(s Stream, date string) bool {
	for _, t := range s.Requests {
		if slices.ContainsFunc(t.linedOn(), func(day LedgerDay) bool { return day.Date == date }) {
			return true
		}
	}
	return false
}

// through is the filter a past day the Log reads back through f is held
// through: f with every kind, as kinds choose little, and a kind changed
// never asks a read; whole, the zero LogFilter, where it keeps every line,
// as whole has it.
func (f LogFilter) through() LogFilter {
	if f.whole() {
		return LogFilter{}
	}
	f.Kinds = nil
	return f
}

// whole reports whether f keeps every line, whatever its kind, as chose
// keeps them: it ticks no account, model or session, and keeps more than
// those that failed.
func (f LogFilter) whole() bool {
	return len(f.Accounts) == 0 && len(f.Models) == 0 && len(f.Sessions) == 0 && !f.Failed
}

// serves reports whether a day held through f, as through has it, holds
// every line o keeps, as chose keeps them, whatever its kind: each of o's
// choices within f's, as choiceWithin has it, and o keeping only those
// that failed where f does.
func (f LogFilter) serves(o LogFilter) bool {
	return choiceWithin(o.Accounts, f.Accounts) && choiceWithin(o.Models, f.Models) && choiceWithin(o.Sessions, f.Sessions) && (!f.Failed || o.Failed)
}

// choiceWithin reports whether a filter's choice keeps none another's
// doesn't, as ticked keeps them: where the other ticks none, which keeps
// every one; else where the choice ticks some, each among the other's.
func choiceWithin[T comparable](choice, other []T) bool {
	return len(other) == 0 || len(choice) > 0 && !slices.ContainsFunc(choice, func(one T) bool { return !slices.Contains(other, one) })
}

// joined is the filter a day held through f, as through has it, is read
// again through to serve o as well, as serves has it: each choice the two
// tick together, as choicesJoined joins them, and only those that failed
// where both keep only those, with every kind.
func (f LogFilter) joined(o LogFilter) LogFilter {
	return LogFilter{
		Accounts: choicesJoined(f.Accounts, o.Accounts), Models: choicesJoined(f.Models, o.Models), Sessions: choicesJoined(f.Sessions, o.Sessions),
		Failed: f.Failed && o.Failed,
	}.through()
}

// choicesJoined is the choice keeping every one either of two keeps, as
// ticked keeps them: none, which keeps every one, where either ticks none;
// else those choice ticks, then those other ticks that it doesn't.
func choicesJoined[T comparable](choice, other []T) []T {
	if len(choice) == 0 || len(other) == 0 {
		return nil
	}
	joined := slices.Clone(choice)
	for _, one := range other {
		if !slices.Contains(joined, one) {
			joined = append(joined, one)
		}
	}
	return joined
}

// mayKeepOn reports whether the local day with the given date may hold a
// line f keeps, whatever its kind, as the days' summaries l gives say: any
// day where f keeps every line, as whole has it, reading none; else where
// the summary of any day whose files may hold its lines, as dayfile's
// FileDates has them, the files Day reads them of, its own and those either
// side of it, may hold one, as mayKeep has it of f with every kind. No
// summary counts a day's failures, so of those that failed it says what it
// says of every line. A date that isn't one holds none.
func (f LogFilter) mayKeepOn(l LedgerDates, date string) bool {
	if f.whole() {
		return true
	}
	day, ok := dayfile.DayBounds(date)
	return ok && slices.ContainsFunc(l.DaysOn(day.FileDates()), f.through().mayKeep)
}

// hiding is what of the filter f hides the row r: none; its kind alone; or
// a choice of another, its kind kept or not.
func (f LogFilter) hiding(r *RequestRow) hidden {
	switch {
	case !f.chose(r):
		return hiddenChosen
	case !ticked(f.Kinds, r.Kind):
		return hiddenKind
	}
	return notHidden
}

// chose reports whether the filter's choices but its kinds keep the row r.
func (f LogFilter) chose(r *RequestRow) bool {
	return f.chosen(rowChoice{account: r.Account, model: r.Model, session: r.CountedSession, failed: r.Failed})
}

// keeps reports whether the filter's choices but its kinds keep the row of
// the record rec, as chose keeps the row lineFacts makes of it, judging its
// choice alone, as choice has it: every one where f keeps every line, as
// whole has it.
func (f LogFilter) keeps(rec *lineRecord) bool {
	return f.whole() || f.chosen(rec.choice())
}

// chosen reports whether the filter's choices but its kinds keep a row of
// the choice c.
func (f LogFilter) chosen(c rowChoice) bool {
	return ticked(f.Accounts, c.account) && ticked(f.Models, c.model) && ticked(f.Sessions, c.session) && (!f.Failed || c.failed)
}

// rowChoice is what of a row a filter's choices judge: the account it's on,
// the model it asked for, the session it counts toward, and whether it
// failed.
type rowChoice struct {
	account, model, session string
	failed                  bool
}

// choice is what of the row of the record r a filter's choices judge, as
// rowChoice has it: the one making of those facts, which lineFacts makes
// the row of, and LogFilter's keeps judges a record by.
func (r *rowRecord) choice() rowChoice {
	return rowChoice{account: r.account, model: r.model, session: r.countedSession, failed: ledger.Failing(r.status)}
}

// hidden is what of a filter hides a row.
type hidden int

const (
	// notHidden is a row the filter shows.
	notHidden hidden = iota
	// hiddenKind is a row only its kind hides.
	hiddenKind
	// hiddenChosen is a row a choice of another hides.
	hiddenChosen
)

// LogCount is what a day's lines count, each once: those of each kind, in
// flight included, together each of the day's lines; those that failed; by
// their models, accounts and sessions; and those the table shows of them.
// The counts but Shown are the day's whatever the filter, as the f panel
// counts them, so with Shown, the heading's hidden make the day's lines.
type LogCount struct {
	// Requests, Checks and Counts are the day's lines of what each asked:
	// those the views count among the requests, Claude Code's quota checks,
	// and its counts of tokens.
	Requests, Checks, Counts int
	// Failed are those that failed, as RequestRow's Failed has it: 400 and
	// over, cut off or given up, and lost.
	Failed int
	// Models, Accounts and Sessions count the lines by the model each asked
	// for, the account it's on, as RequestRow's Account has it, and the
	// session it counts toward, as ledger.Line.CountedSession has it, each by
	// its whole id: one of none, as a quota check of a session, is counted by
	// none.
	Models, Accounts, Sessions map[string]int
	// Shown are those the table shows.
	Shown int
	// byKind are those its kind alone hides, by the kind; and chosen, those
	// a choice of another hides.
	byKind [len(rowKinds)]int
	chosen int
}

// Lines is how many of the day's lines there are: each kind's together.
func (c LogCount) Lines() int {
	return c.Requests + c.Checks + c.Counts
}

// ModelsListed are the models the f panel lists, by their whole ids: those
// the day's lines asked for, the most asked first, then by id; then those
// ticked that none asked for, as one ticked before midnight, by id, so a
// model the filter keeps always has a tick.
func (c LogCount) ModelsListed(ticked []string) []string {
	asked := slices.SortedFunc(maps.Keys(c.Models), func(a, b string) int {
		return cmp.Or(c.Models[b]-c.Models[a], strings.Compare(a, b))
	})
	return append(asked, notAmong(ticked, asked)...)
}

// RowKinds are the kinds of RowKind, in the panel's order.
func RowKinds() []RowKind {
	return slices.Clone(rowKinds[:])
}

// Hidden is how many of the day's lines the table hides.
func (c LogCount) Hidden() int {
	return c.Lines() - c.Shown
}

// HiddenBy are the kinds that hide the day's lines hidden, in the panel's
// order, where their kinds alone hide them, as the heading names them, as
// ": checks and counts": none where a choice of another hides any, as the
// heading then says how many alone, or where none is hidden.
func (c LogCount) HiddenBy() []RowKind {
	if c.chosen > 0 {
		return nil
	}
	var by []RowKind
	for _, k := range rowKinds {
		if c.byKind[k] > 0 {
			by = append(by, k)
		}
	}
	return by
}

// add counts the row r, the filter hiding it as given.
func (c *LogCount) add(r *RequestRow, hid hidden) {
	c.count(r)
	switch hid {
	case notHidden:
		c.Shown++
	case hiddenKind:
		c.byKind[r.Kind]++
	case hiddenChosen:
		c.chosen++
	}
}

// count counts the row r as its day's lines count whatever the filter: by
// what it asked, whether it failed, and by its model, account and session.
func (c *LogCount) count(r *RequestRow) {
	switch r.Kind {
	case RowCheck:
		c.Checks++
	case RowCount:
		c.Counts++
	default:
		c.Requests++
	}
	if r.Failed {
		c.Failed++
	}
	c.Models, c.Accounts, c.Sessions = tally(c.Models, r.Model), tally(c.Accounts, r.Account), tally(c.Sessions, r.CountedSession)
}

// cloned is c, counting as it does, for what counts on to change, sharing
// nothing it changes with c.
func (c LogCount) cloned() LogCount {
	c.Models, c.Accounts, c.Sessions = maps.Clone(c.Models), maps.Clone(c.Accounts), maps.Clone(c.Sessions)
	return c
}

// tally is counted with one more of the id given, made where it's nil: as
// it was for none.
func tally(counted map[string]int, id string) map[string]int {
	if id == "" {
		return counted
	}
	if counted == nil {
		counted = make(map[string]int)
	}
	counted[id]++
	return counted
}
