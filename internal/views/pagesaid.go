package views

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// MovedOnto is the move that brought the session onto the account its page
// names it on: its named model's last move, where that was onto it; false
// where it started there, or that model's last move took it elsewhere. A
// session's models are routed apart, so another's move onto the same account
// tells nothing of how it came there.
func (p SessionPage) MovedOnto() (SessionMove, bool) {
	moves := p.modelMoves()
	if len(moves) == 0 || moves[len(moves)-1].To != p.Account {
		return SessionMove{}, false
	}
	return moves[len(moves)-1], true
}

// RanOn is where the session ran, its named model's account history: the
// account it ran on first, and each of that model's moves after, in turn. Its
// first is its first move's From, where it moved, else the account it's
// named on: "" where it's named on none.
func (p SessionPage) RanOn() (first string, moves []SessionMove) {
	moves = p.modelMoves()
	if len(moves) > 0 {
		return moves[0].From, moves
	}
	return p.Account, nil
}

// modelMoves are the moves of the model the page names the session by, in
// the order they came.
func (p SessionPage) modelMoves() []SessionMove {
	return slices.DeleteFunc(slices.Clone(p.Moves), func(m SessionMove) bool { return m.Model != p.Model })
}

// MoveForm is the form a move's label on the story is said in.
type MoveForm int

const (
	// MoveWide is the wide page's label, the zero MoveForm.
	MoveWide MoveForm = iota
	// MoveShort is a phone's.
	MoveShort
)

// Said says the move as the story's label at its marker says it, in form,
// accounts named apart and figures as figures: wide, what moved it, as an
// event says it, and where to, "work reached its cap → side"; and where it
// wrote its context again, the tokens that wrote and their worth, " · 182k
// cached again, $1.82", "unpriced" where the price table can't price it.
// Short, as a phone says it, why it was forced short, and the worth alone:
// "its cap → side, $1.82". Words that name where it went already, as
// "pinned to side", have no arrow after them.
func (m SessionMove) Said(form MoveForm) status.Words {
	words, worth := status.EventWords, status.Words{status.Plain(" · "), status.Figure(prose.Tokens(m.Written)), status.Plain(" cached again, ")}
	if form == MoveShort {
		words, worth = status.ShortWords, status.Words{status.Plain(", ")}
	}
	said := words(m.Reason, status.Facts{From: m.From, To: m.To})
	if last := len(said) - 1; last < 0 || said[last] != status.Named(m.To) {
		said = slices.Concat(said, status.Words{status.Plain(" → "), status.Named(m.To)})
	}
	if !m.Rewrote {
		return said
	}
	cost := status.Plain(unpriced)
	if m.Cost != nil {
		cost = status.Figure(Worth{Amount: m.Cost}.Cents())
	}
	return slices.Concat(said, worth, status.Words{cost})
}

// WhereSaid is where a session runs and why, at a moment, as its page says
// it, in its parts: row 6 says OnSince and Why together, as Words runs them;
// a phone's row 7 says OnSince, and its row 8 Short.
type WhereSaid struct {
	// OnSince is the account it's named on, which its marks follow, and
	// since when it's been there: "on side since 14:31", "on work since it
	// started". None where it's named on none, or once it's over.
	OnSince status.Words
	// Why is how it came there, or why it's there, as row 6 says it after
	// OnSince, with its lead: ", moved from work: work reached its cap", ":
	// sticky"; none where it started there for no other reason. Once it's
	// over, where it ran: "ran on work, then side from 16:02: work reached
	// its cap".
	Why status.Words
	// Short is Why as a phone's row 8 says it, without its lead, why a move
	// off an account was forced said short: "moved from work: its cap", "ran
	// on work, then side from 16:02: its cap".
	Short status.Words
}

// Words runs its parts together, as row 6 and sessions <id>'s text say
// them.
func (w WhereSaid) Words() status.Words {
	return slices.Concat(w.OnSince, w.Why)
}

// Where says where the session runs and why, at now, as its page says it.
// While it runs, as Runs says, it's on the account it's named on: since the
// move of its named model that brought it there, as MovedOnto has it, as
// cameBy says it, "on side since 14:31, moved from work: work reached its
// cap"; else, for why that model goes there, as Goes gives it, a reason that
// moved nothing, since it started, "on work since it started: sticky", and
// for none, "on work since it started"; else for any other, as a move to it
// when the router put it there, "on side since 14:39, pinned here", or where
// that isn't known, "on side: pinned here". Once it's over, where it ran, as
// RanOn has it, as ranOn says it. None where it's named on no account.
func (p SessionPage) Where(now time.Time) WhereSaid {
	if !p.Runs() {
		return p.ranOn(now)
	}
	if p.Account == "" {
		return WhereSaid{}
	}
	on := status.Words{status.Plain("on "), status.Named(p.Account)}
	if m, ok := p.MovedOnto(); ok {
		return cameBy(on, m, now)
	}
	why := p.whyOn()
	facts := status.Facts{To: p.Account, Now: now}
	switch {
	case why.Reason == "" || why.Reason == status.ReasonNew:
		return WhereSaid{OnSince: append(on, status.Plain(" since it started"))}
	case status.MovedNothing(why.Reason):
		return WhereSaid{OnSince: append(on, status.Plain(" since it started")), Why: slices.Concat(status.Words{status.Plain(": ")},
			status.SessionWords(why.Reason, facts)), Short: status.ShortWords(why.Reason, facts)}
	case !why.Since.IsZero():
		return cameBy(on, SessionMove{At: why.Since, Model: p.Model, From: status.MovedFrom(why.Reason), To: p.Account, Reason: why.Reason}, now)
	}
	return WhereSaid{OnSince: on, Why: slices.Concat(status.Words{status.Plain(": ")}, status.SessionWords(why.Reason, facts)),
		Short: status.ShortWords(why.Reason, facts)}
}

// whyOn is why the session's named model goes to the account it's named on,
// and since when, as Goes gives it: none where it gives none.
func (p SessionPage) whyOn() SessionModel {
	i := slices.IndexFunc(p.Goes, func(m SessionModel) bool { return m.Model == p.Model && m.Account == p.Account })
	if i < 0 {
		return SessionModel{}
	}
	return p.Goes[i]
}

// cameBy says where a session runs, on as its OnSince starts, as the move m
// brought it there, at now: since when, then why, after ", ", as "on side
// since 14:31, moved from work: work reached its cap", or after ": " for a
// pin that yields, as "on work since 14:31: its pin to side yields: side has
// no room", as it went elsewhere than its pin sends it.
func cameBy(on status.Words, m SessionMove, now time.Time) WhereSaid {
	lead := ", "
	if strings.HasPrefix(m.Reason, status.ReasonPinYields) {
		lead = ": "
	}
	return WhereSaid{
		OnSince: append(on, status.Plain(" since "+prose.Past(now, m.At))),
		Why:     slices.Concat(status.Words{status.Plain(lead)}, movedWords(m, status.SessionWords, now)),
		Short:   movedWords(m, status.ShortWords, now),
	}
}

// movedWords say why m moved its session, as a session's page says it in
// words's form: "moved from work: " before them, for a move off an account
// that couldn't take it, as "moved from work: work reached its cap"; else
// as words says it, as "rescored after 15h idle: personal had the most
// room".
func movedWords(m SessionMove, words func(reason string, f status.Facts) status.Words, now time.Time) status.Words {
	said := words(m.Reason, status.Facts{From: m.From, To: m.To, Now: now})
	if m.From == "" || !strings.HasPrefix(m.Reason, status.ReasonMovedOff) {
		return said
	}
	return slices.Concat(status.Words{status.Plain("moved from "), status.Named(m.From), status.Plain(": ")}, said)
}

// ranOn says where the session ran, as RanOn has it, at now: on each
// account, in turn, with when its named model's moves came and why, as "ran
// on work, then side from 16:02: work reached its cap", and short, "ran on
// work, then side from 16:02: its cap". None where it ran on no account.
func (p SessionPage) ranOn(now time.Time) WhereSaid {
	first, moves := p.RanOn()
	if first == "" {
		return WhereSaid{}
	}
	ran := status.Words{status.Plain("ran on "), status.Named(first)}
	why, short := ran, slices.Clone(ran)
	for _, m := range moves {
		then := status.Words{status.Plain(", then "), status.Named(m.To), status.Plain(" from " + prose.Past(now, m.At) + ": ")}
		facts := status.Facts{From: m.From, To: m.To, Now: now}
		why = slices.Concat(why, then, status.EventWords(m.Reason, facts))
		short = slices.Concat(short, then, status.ShortWords(m.Reason, facts))
	}
	return WhereSaid{Why: why, Short: short}
}

// StartedSaid says when the session started, at now, as row 6 says it after
// "started": "12:02" today, "yesterday 16:40" the day before, as prose.Past
// says it; "" where its start isn't known.
func (p SessionPage) StartedSaid(now time.Time) string {
	if p.Started.IsZero() {
		return ""
	}
	return prose.Past(now, p.Started)
}

// RanSaid says how it has run, at now, as row 6 says it after when it
// started, and a phone's row 8 alone: how long since it started, as
// prose.Span says it, "2h 40m"; "resumed 09:12" once it came back after an
// IdleHour or more without a request, when, as prose.Past says it; or once
// it's over, as its Activity says, how long it ran, from its first request
// to its last, "ran 5h 27m". "" where its start isn't known.
func (p SessionPage) RanSaid(now time.Time) string {
	switch {
	case p.Started.IsZero():
		return ""
	case !p.Runs():
		return "ran " + prose.Span(p.Activity.Since.Sub(p.Started))
	case !p.Resumed.IsZero():
		return "resumed " + prose.Past(now, p.Resumed)
	}
	return prose.Span(now.Sub(p.Started))
}

// Stay is the session's time on an account it ran on, as its page's row of
// the account has it: what it made there, and its time there, as Stays says
// it.
type Stay struct {
	SessionAccount
	// Said is its time there, as prose.Stint says it: "12:02 – 14:31",
	// "today 09:12 – now".
	Said string
}

// Stays are the accounts the session ran on, in the order it first did,
// each with its time there, at now, as prose.Stint says it: from its first
// request there to its last, or, while it runs, as Runs says, to now on the
// one its latest request went to, never one the router sends it to but no
// request of it has gone to since; each end with its day where its time,
// from its first request to its last or to now, spans more than one, as
// "yesterday 16:40 – 18:05" and "today 09:12 – now".
func (p SessionPage) Stays(now time.Time) []Stay {
	if len(p.Accounts) == 0 {
		return nil
	}
	first := slices.MinFunc(p.Accounts, func(a, b SessionAccount) int { return a.From.Compare(b.From) }).From
	last := slices.MaxFunc(p.Accounts, func(a, b SessionAccount) int { return a.To.Compare(b.To) })
	end := last.To
	if p.Runs() {
		end = now
	}
	dated := !prose.SameDay(now, first, end)
	stays := make([]Stay, len(p.Accounts))
	for i, a := range p.Accounts {
		to := a.To
		if p.Runs() && a.Account == last.Account {
			to = time.Time{}
		}
		stays[i] = Stay{SessionAccount: a, Said: prose.Stint(now, a.From, to, dated)}
	}
	return stays
}

// Runs reports whether the session runs, as its page draws it: its Activity
// not ended, whether the router lists it or not, as a session the stream
// tells of before the router lists it, or one whose router has stopped
// answering, runs all the same.
func (p SessionPage) Runs() bool {
	return p.Activity.Doing != Ended
}

// Took says how long the turn took, at now, as its row's took says it: the
// minutes from the one it started in to the one it ended in, both counted,
// as prose.Minutes says them, as "9m"; and of one still going, to the one
// now is in, going saying so, as its row says by "…" after them.
func (t Turn) Took(now time.Time) (minutes string, going bool) {
	if t.Ended.IsZero() {
		return prose.Minutes(t.Started, now), true
	}
	return prose.Minutes(t.Started, t.Ended), false
}

// toolsSaid is how many of the tools a turn's answers called most its row
// names.
const toolsSaid = 3

// ToolsSaid says the tools the turn's answers called most, three at the
// most, most first, with how many times each, as "Bash ×14, Edit ×9, Read
// ×8".
func (t Turn) ToolsSaid() string {
	said := make([]string, 0, toolsSaid)
	for _, c := range t.Tools[:min(len(t.Tools), toolsSaid)] {
		said = append(said, prose.Clean(c.Tool)+" ×"+prose.Grouped(c.Times))
	}
	return strings.Join(said, ", ")
}

// PointsSaid says the points of the account's windows the session took, the
// shortest window first, each named as status names it, as "≈ 41 points of
// its 5-hour · 6 of its week", leaving out those that round to none: "" where
// all do.
func (a SessionAccount) PointsSaid() string {
	keys := slices.SortedFunc(maps.Keys(a.Points), quota.CompareKeys)
	keys = slices.DeleteFunc(keys, func(key string) bool { return a.Points[key] == 0 })
	said := make([]string, len(keys))
	for i, key := range keys {
		of := " of its " + status.InProse(status.NameOfKey(key))
		if i > 0 {
			said[i] = prose.Grouped(a.Points[key]) + of
			continue
		}
		said[i] = "≈ " + prose.Counted(a.Points[key], "point") + of
	}
	return strings.Join(said, " · ")
}
