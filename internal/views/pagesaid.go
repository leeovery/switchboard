package views

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
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
	cost := rewriteSaid(m.Rewrote, m.Cost)
	switch {
	case cost == "":
		return said
	case m.Cost == nil:
		return slices.Concat(said, worth, status.Words{status.Plain(cost)})
	}
	return slices.Concat(said, worth, status.Words{status.Figure(cost)})
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
	return p.where(now, datesSaid)
}

// WhereSaidWidest says where the session runs and why, at now, as Where does,
// but every time it names at its widest, as prose.PastWidest and
// prose.LaterWidest say them: as its page measures what it says, so the
// measure moves only as the session moves or ends, never as the clock moves
// the day of a time it names.
func (p SessionPage) WhereSaidWidest(now time.Time) WhereSaid {
	return p.where(now, datesWidest)
}

// DoingSaidWidest is each thing a session of its kind can say it's doing,
// as its Activity's Said says it, at its widest at now: once it's over, its
// end the day before, "ended yesterday 18:46"; while it runs, asking,
// answering, throttled, refused, and idle to the IdleHour, after which it
// ends. A session's kind changes once, as it ends.
func (p SessionPage) DoingSaidWidest(now time.Time) []string {
	if !p.Runs() {
		return []string{Activity{Doing: Ended, Since: now.AddDate(0, 0, -1)}.Said(now)}
	}
	doings := []Activity{
		{Doing: Asking}, {Doing: Answering}, {Doing: Throttled, Status: 429}, {Doing: Refused, Status: 403},
		{Doing: Idle, Since: now.Add(time.Second - IdleHour)},
	}
	said := make([]string, len(doings))
	for i, a := range doings {
		said[i] = a.Said(now)
	}
	return said
}

// RanSaidWidest says how the session has run, as RanSaid does, at its widest
// for its own kind at now: once over, how long it ran, "ran 23h 59m";
// resumed, when, the day before, "resumed yesterday 22:10"; else how long it
// has run, "23h 59m". "" where its start isn't known.
func (p SessionPage) RanSaidWidest(now time.Time) string {
	if p.Started.IsZero() {
		return ""
	}
	widest := SessionPage{Started: now.Add(time.Minute - 24*time.Hour), Activity: Activity{Doing: Idle}}
	switch {
	case !p.Runs():
		widest.Activity = Activity{Doing: Ended, Since: now}
	case !p.Resumed.IsZero():
		widest.Resumed = now.AddDate(0, 0, -1)
	}
	return widest.RanSaid(now)
}

// dating says the times a session's where and why name: as they're said,
// datesSaid, or at their widest, datesWidest.
type dating struct {
	// past says a time before now, as prose.Past does.
	past func(now, t time.Time) string
	// later says a time after start, as prose.Later does.
	later func(now, start, t time.Time) string
}

var (
	datesSaid   = dating{past: prose.Past, later: prose.Later}
	datesWidest = dating{past: prose.PastWidest, later: prose.LaterWidest}
)

// where says where the session runs and why, at now, as Where does, each
// time as dates says it.
func (p SessionPage) where(now time.Time, dates dating) WhereSaid {
	if !p.Runs() {
		return p.ranOn(now, dates)
	}
	if p.Account == "" {
		return WhereSaid{}
	}
	on := status.Words{status.Plain("on "), status.Named(p.Account)}
	if m, ok := p.MovedOnto(); ok {
		return cameBy(on, m, now, dates)
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
		return cameBy(on, SessionMove{At: why.Since, Model: p.Model, From: status.MovedFrom(why.Reason), To: p.Account, Reason: why.Reason}, now, dates)
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
// no room", as it went elsewhere than its pin sends it; when, as dates
// says it.
func cameBy(on status.Words, m SessionMove, now time.Time, dates dating) WhereSaid {
	lead := ", "
	if strings.HasPrefix(m.Reason, status.ReasonPinYields) {
		lead = ": "
	}
	return WhereSaid{
		OnSince: append(on, status.Plain(" since "+dates.past(now, m.At))),
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
// work, then side from 16:02: its cap". A move's day is named only where it
// isn't the day the session started on, as row 6's right names that day:
// "from today 00:40", as dates says it. None where it ran on no account.
func (p SessionPage) ranOn(now time.Time, dates dating) WhereSaid {
	first, moves := p.RanOn()
	if first == "" {
		return WhereSaid{}
	}
	ran := status.Words{status.Plain("ran on "), status.Named(first)}
	why, short := ran, slices.Clone(ran)
	for _, m := range moves {
		then := status.Words{status.Plain(", then "), status.Named(m.To), status.Plain(" from " + dates.later(now, p.Started, m.At) + ": ")}
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
	// cents is its worth to the cent, as Cents says it.
	cents ledger.Picodollars
}

// Cents is its worth to the cent as its row says it, as Worth's Cents says
// it, but in its stead: its rounding reconciled to the session's, as
// toTheCent reconciles it, so the rows add up to TURNS' total, cent for cent.
func (s Stay) Cents() string {
	return s.centsAt(s.cents)
}

// Stays are the accounts the session ran on, in the order it first did,
// each with its time there, at now, as prose.Stint says it: from its first
// request there to its last, or, while it runs, as Runs says, to now on the
// one its latest request went to, never one the router sends it to but no
// request of it has gone to since; each end with its day where the session
// began before today, as an undated time reads as today's, as "yesterday
// 16:40 – 18:05", "today 09:12 – now" and "yesterday 13:20 – 16:02".
func (p SessionPage) Stays(now time.Time) []Stay {
	if len(p.Accounts) == 0 {
		return nil
	}
	first := slices.MinFunc(p.Accounts, func(a, b SessionAccount) int { return a.From.Compare(b.From) }).From
	last := slices.MaxFunc(p.Accounts, func(a, b SessionAccount) int { return a.To.Compare(b.To) })
	dated := !prose.SameDay(now, first, now)
	amounts := make([]ledger.Picodollars, len(p.Accounts))
	for i, a := range p.Accounts {
		amounts[i] = a.amount()
	}
	cents := toTheCent(amounts)
	stays := make([]Stay, len(p.Accounts))
	for i, a := range p.Accounts {
		to := a.To
		if p.Runs() && a.Account == last.Account {
			to = time.Time{}
		}
		stays[i] = Stay{SessionAccount: a, Said: prose.Stint(now, a.From, to, dated), cents: cents[i]}
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

// Live is the page p as it's drawn, the stream s telling what it holds of
// its request answering: what that answer has streamed counting on, as a
// row in flight's does, so a page is built again only as what it's built of
// changes, not as an answer streams on.
func (p SessionPage) Live(s Stream) SessionPage {
	p.Activity = p.Activity.live(s)
	return p
}

// Took says how long the turn took, at now, as its row's took says it: the
// minutes from the one it started in to the one it ended in, both counted,
// as prose.Minutes says them, as "9m"; and of one still going, to the one
// now is in, going saying so, as its row says by "…" after them.
func (t Turn) Took(now time.Time) (minutes string, going bool) {
	end, going := t.end(now)
	return prose.Minutes(t.Started, end), going
}

// end is when the turn ended, or of one still going, now, going saying so.
func (t Turn) end(now time.Time) (end time.Time, going bool) {
	if t.Ended.IsZero() {
		return now, true
	}
	return t.Ended, false
}

// TookSaid says how long the turn took, at now, as its row says it: as Took
// counts it, held to four characters as prose.MinutesWithin says it, "9m",
// "16h", "69d", and of one still going, "…" after, as "3m…".
func (t Turn) TookSaid(now time.Time) string {
	end, going := t.end(now)
	took := prose.MinutesWithin(t.Started, end)
	if going {
		took += prose.Ellipsis
	}
	return took
}

// TookSaidWidest is the widest TookSaid says: 999 minutes and going,
// "999m…".
func TookSaidWidest() string {
	var start time.Time
	return Turn{Started: start}.TookSaid(start.Add(998 * time.Minute))
}

// RequestsSaid says the turn's requests, as its row says them: "1,240".
func (t Turn) RequestsSaid() string {
	return prose.Grouped(t.Requests)
}

// TurnRequestsSaidWidest is the widest RequestsSaid a turn's row holds
// whole: "9,999".
func TurnRequestsSaidWidest() string {
	return Turn{Requests: 9_999}.RequestsSaid()
}

// ToolsNamed is how many of the tools a turn's answers called most its row
// names.
const ToolsNamed = 3

// ToolsSaid says the tools the turn's answers called most, as ToolsEach
// says them, a comma and a blank between: "Bash ×14, Edit ×9, Read ×8".
func (t Turn) ToolsSaid() string {
	return t.ToolsSaidFirst(ToolsNamed)
}

// ToolsSaidFirst says the first n of the tools ToolsSaid says, as it says
// them: "Bash ×14, Edit ×9".
func (t Turn) ToolsSaidFirst(n int) string {
	each := t.ToolsEach()
	return strings.Join(each[:min(n, len(each))], ", ")
}

// ToolsEach says each of the tools the turn's answers called most, three at
// the most, most first, with how many times it was called, as "Bash ×14".
func (t Turn) ToolsEach() []string {
	said := make([]string, 0, ToolsNamed)
	for _, c := range t.Tools[:min(len(t.Tools), ToolsNamed)] {
		said = append(said, prose.Clean(c.Tool)+" ×"+prose.Grouped(c.Times))
	}
	return said
}

// ToolsSaidWidest is ToolsSaid at its widest of Claude Code's own tools:
// ToolsNamed of them, each named as claude.WidestTool and called 999 times.
func ToolsSaidWidest() string {
	tools := make([]ToolCalls, ToolsNamed)
	for i := range tools {
		tools[i] = ToolCalls{Tool: claude.WidestTool, Times: 999}
	}
	return Turn{Tools: tools}.ToolsSaid()
}

// WorthSaidWidest are the worths a turn says at their widest, as Cents
// says them, to the most it's held whole to: $999.99 with part of it
// unpriced, "$999.99+", and none of it priced, "unpriced".
func WorthSaidWidest() []string {
	return worthSaidWidest(99_999)
}

// StayWorthSaidWidest are the worths an account's row says at their widest,
// as Cents says them, to the most it's held whole to, which a session's
// time on an account doesn't pass: $9999.99, with part of it unpriced,
// "$9999.99+", and none of it priced, "unpriced".
func StayWorthSaidWidest() []string {
	return worthSaidWidest(999_999)
}

// worthSaidWidest are the worths said at their widest, as Cents says them,
// to the most given, in cents: with part of it unpriced, and none of it
// priced, "unpriced".
func worthSaidWidest(cents int64) []string {
	most := ledger.Picodollars(cents) * ledger.Picodollars(10_000_000_000)
	unpriced := []string{"unpriced"}
	return []string{Worth{Amount: &most, Unpriced: unpriced}.Cents(), Worth{Unpriced: unpriced}.Cents()}
}

// RequestsSaid says the session's requests on the account, as its row says
// them: "12,345 requests".
func (a SessionAccount) RequestsSaid() string {
	return prose.Counted(a.Requests, "request")
}

// StayRequestsSaidWidest is the widest RequestsSaid an account's row holds
// whole: "99,999 requests".
func StayRequestsSaidWidest() string {
	return SessionAccount{Requests: 99_999}.RequestsSaid()
}

// PointsWordsWidest are the points of the windows with the keys given, as
// PointsWords says them in form, each at its most, a hundred points: the
// widest an account's row says them.
func PointsWordsWidest(keys []string, form PointsForm) []status.Words {
	most := SessionAccount{Points: make(map[string]int, len(keys))}
	for _, key := range keys {
		most.Points[key] = 100
	}
	return most.PointsWords(form)
}

// PointsKeys are the keys of the windows any of the session's accounts took
// points of, as each one's PointsKeys has them: which windows the session
// used, a fact that changes only as one is first read.
func (p SessionPage) PointsKeys() []string {
	var keys []string
	for _, a := range p.Accounts {
		for _, key := range a.PointsKeys() {
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// PointsForm is the form an account's points are said in.
type PointsForm int

const (
	// PointsWide is the wide page's, the zero PointsForm: "≈ 41 points of
	// its 5-hour".
	PointsWide PointsForm = iota
	// PointsShort is a phone's: "≈ 41 of 5-hour".
	PointsShort
)

// PointsWords says the points of the account's windows the session took, in
// form, in the order of care, each named as status names it, and each
// window's words apart, so a row short of room leaves whole windows off its
// end: the first leading with "≈ ", each after it with " · ", the points a
// figure. Wide, "≈ 6 points of its week", " · 41 of its 5-hour"; short, as a
// phone says them, "≈ 6 of week", " · 41 of 5-hour". It leaves out those
// that round to none: none where all do. Each says the window PointsKeys
// gives in its place.
func (a SessionAccount) PointsWords(form PointsForm) []status.Words {
	keys := a.PointsKeys()
	said := make([]status.Words, len(keys))
	for i, key := range keys {
		lead, count := status.Plain(" · "), status.Words{status.Figure(prose.Grouped(a.Points[key]))}
		of := " of " + status.InProse(status.NameOfKey(key))
		if i == 0 {
			lead = status.Plain("≈ ")
		}
		if form == PointsWide {
			of = " of its " + status.InProse(status.NameOfKey(key))
		}
		if i == 0 && form == PointsWide {
			count = CountedWords(a.Points[key], "point")
		}
		said[i] = slices.Concat(status.Words{lead}, count, status.Words{status.Plain(of)})
	}
	return said
}

// PointsKeys are the keys of the account's windows the session took points
// of, as PointsWords says them, in the order the owner reads windows listed,
// as status.CompareWindows has it by Claude's policy, those that round to
// none left out.
func (a SessionAccount) PointsKeys() []string {
	keys := slices.SortedFunc(maps.Keys(a.Points), status.CompareWindows(claude.Policy()))
	return slices.DeleteFunc(keys, func(key string) bool { return a.Points[key] == 0 })
}

// PointsSaid says the points of the account's windows the session took, as
// PointsWords says them wide, run together: "≈ 41 points of its 5-hour · 6
// of its week"; "" where all round to none.
func (a SessionAccount) PointsSaid() string {
	return slices.Concat(a.PointsWords(PointsWide)...).String()
}
