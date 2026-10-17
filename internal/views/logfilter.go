package views

import (
	"cmp"
	"maps"
	"slices"
	"strings"

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
	return ticked(f.Accounts, r.Account) && ticked(f.Models, r.Model) && ticked(f.Sessions, r.CountedSession) && (!f.Failed || r.Failed)
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
	switch hid {
	case notHidden:
		c.Shown++
	case hiddenKind:
		c.byKind[r.Kind]++
	case hiddenChosen:
		c.chosen++
	}
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
