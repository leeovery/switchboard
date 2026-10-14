package status

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

// Words are a reason the router gives, in the dashboard's words, or a phrase
// History says: in parts, each account they name, how long a session idled
// and each figure, a part of its own, as the pages draw them apart.
type Words []Part

// Part is a run of Words, of the kind it is.
type Part struct {
	Text string
	Kind PartKind
}

// PartKind is what a Part of Words is, which says how it's drawn.
type PartKind int

// The kinds of Part.
const (
	// PartPlain is the words around the accounts and times named.
	PartPlain PartKind = iota
	// PartAccount is an account's id.
	PartAccount
	// PartDuration is how long a session idled.
	PartDuration
	// PartFigure is a figure among words, as a count of weeks.
	PartFigure
	// PartUse is how much of a window is used, as "95%", drawn in its
	// level's colour.
	PartUse
)

// String runs the words together, as a page drawn without colour shows them.
func (w Words) String() string {
	var b strings.Builder
	for _, p := range w {
		b.WriteString(p.Text)
	}
	return b.String()
}

// Facts are what a reason's words take beyond the reason itself, from the
// line, assignment or event that gives it. Words never name an account the
// facts don't give: the clause that would is left out.
type Facts struct {
	// From is the account the session moved from: "" where it didn't move,
	// as the ledger's line says.
	From string
	// To is the account the session went to, or is on: the pin's, for a move
	// by pin or a global pin, and the one the choice found had the most room,
	// for a session rescored.
	To string
	// Until is when the account a choice passed over under pressure runs out
	// at its rate, as its pressure event's until gives it: zero where that
	// isn't known, which leaves it unsaid.
	Until time.Time
	// At is when the session went to the account it's on, as a session
	// running says it: zero where that isn't known, which leaves it unsaid.
	At time.Time
	// Best is set while the account the session is on is where new sessions
	// go.
	Best bool
	// Now gives the time zone Until and At are shown in.
	Now time.Time
}

// EventWords are the reason as an event or a session's assignment reads, as
// in "work reached its cap". Any reason not in the dashboard's words reads as
// the router gives it, cleaned.
func EventWords(reason string, f Facts) Words {
	return wordsOf(reason, f).event
}

// RoutingWords are the reason as a request's routing reads: "new session" for
// a session's first request; none for a request that stayed where its session
// was, as From says; else "from " and its LeftWords, as in "from work, at its
// cap". Any reason not in the dashboard's words reads as the router gives it,
// cleaned.
func RoutingWords(reason string, f Facts) Words {
	if isNew(prose.Clean(reason)) {
		return Words{Plain("new session")}
	}
	if left := LeftWords(reason, f); left != nil {
		return slices.Concat(Words{Plain("from ")}, left)
	}
	return nil
}

// FromWords are a request's move of its session, the account it left alone,
// as a phone's Log says it: "from work", as RoutingWords starts.
func FromWords(from string) Words {
	return Words{Plain("from "), Named(from)}
}

// LeftWords are a request's move of its session as a request's card says it
// beside from: the account it left, the one From gives, and why, as in
// "work, at its cap", as RoutingWords says them after "from ". None for a
// session's first request, which left none, or one that stayed where its
// session was. Any reason not in the dashboard's words reads as the router
// gives it, cleaned.
func LeftWords(reason string, f Facts) Words {
	if f.From == "" || isNew(prose.Clean(reason)) {
		return nil
	}
	return slices.Concat(Words{Named(f.From), Plain(", ")}, wordsOf(reason, f).routed)
}

// SessionWords are the reason as a session's page reads, as in "since it
// started", or "rescored after 15h idle: personal had the most room". Any
// reason not in the dashboard's words reads as the router gives it, cleaned.
func SessionWords(reason string, f Facts) Words {
	return wordsOf(reason, f).session
}

// ShortWords are the reason as a phone's session page reads it, in its few
// cells: why a move off an account was forced, the account it left said
// apart, "its cap"; else as EventWords reads it. Any reason not in the
// dashboard's words reads as the router gives it, cleaned.
func ShortWords(reason string, f Facts) Words {
	w := wordsOf(reason, f)
	if w.short != nil {
		return w.short
	}
	return w.event
}

// RunningWords are the reason as a session running reads, with when it went
// to its account, as prose.Dated says it: "started here 14:41", ending ",
// the best" while that's where new sessions go; "pinned here 14:39", its own
// pin; "its pin to side yields: side has no room"; "moved from work 14:31:
// work reached its cap", the account it left the reason's own where it names
// one; else "here since 14:39: " and its words as an event reads them. Any
// reason not in the dashboard's words reads as the router gives it, cleaned.
func RunningWords(reason string, f Facts) Words {
	reason = prose.Clean(reason)
	at := ""
	if !f.At.IsZero() {
		at = " " + prose.Dated(f.Now, f.At)
	}
	switch {
	case isNew(reason) && f.Best:
		return Words{Plain("started here" + at + ", the best")}
	case isNew(reason):
		return Words{Plain("started here" + at)}
	case reason == ReasonPinned:
		return Words{Plain("pinned here" + at)}
	case strings.HasPrefix(reason, ReasonPinYields):
		return EventWords(reason, f)
	}
	if left := MovedFrom(reason); left != "" {
		f.From = left
	}
	var came Words
	switch {
	case f.From != "":
		came = Words{Plain("moved from "), Named(f.From), Plain(at)}
	case at != "":
		came = Words{Plain("here since" + at)}
	}
	switch {
	case reason == "":
		return came
	case came == nil:
		return EventWords(reason, f)
	}
	return slices.Concat(came, Words{Plain(": ")}, EventWords(reason, f))
}

// forms are a reason's words in each place the dashboard says it: routed is
// why, as a request's routing says it after the account the session left;
// and short, where it's given, why as a phone says it, as ShortWords has it.
type forms struct {
	event, routed, session, short Words
}

// alike is words that read the same everywhere.
func alike(w Words) forms {
	return forms{event: w, routed: w, session: w}
}

// then is each of w's forms followed by a further clause, after ", ": none
// where a form is none, as ShortWords reads the event's in its place.
func (w forms) then(clause Words) forms {
	further := func(form Words) Words {
		if form == nil {
			return nil
		}
		return slices.Concat(form, Words{Plain(", ")}, clause)
	}
	return forms{event: further(w.event), routed: further(w.routed), session: further(w.session), short: further(w.short)}
}

// Plain is words around the accounts and times named.
func Plain(text string) Part {
	return Part{Text: text}
}

// Named is the account with the given id, cleaned, as it's from elsewhere.
func Named(id string) Part {
	return Part{Text: prose.Clean(id), Kind: PartAccount}
}

// Figure is a figure among words, as a count.
func Figure(text string) Part {
	return Part{Text: text, Kind: PartFigure}
}

// Use is how much of a window is used among words, as "95%".
func Use(text string) Part {
	return Part{Text: text, Kind: PartUse}
}

// readers each read a kind of reason, reporting false for any other, in the
// order they're tried: a choice that passed an account over for pressure says
// so after why it was made, as pressured reads it.
var readers = []func(reason string, f Facts) (forms, bool){pressured, settled, rescored, movedOff, pinYields}

// wordsOf is the reason's words, cleaned, as it's from elsewhere: as the
// router gives it where none of readers reads it.
func wordsOf(reason string, f Facts) forms {
	reason = prose.Clean(reason)
	for _, read := range readers {
		if w, ok := read(reason, f); ok {
			return w
		}
	}
	return alike(Words{Plain(reason)})
}

// MovedFrom is the account a reason for a move off an account names, as
// "moved: work hit its limit" names work, cleaned, as it's from elsewhere: ""
// for any other reason.
func MovedFrom(reason string) string {
	account, _, ok := cutAccount(prose.Clean(reason), ReasonMovedOff)
	if !ok {
		return ""
	}
	return account
}

// MovedNothing reports whether the reason is one that moves no session,
// cleaned, as it's from elsewhere: a new session's, whether or not the
// choice passed an account over for pressure; sticky; or kept for its
// thinking.
func MovedNothing(reason string) bool {
	return isNew(prose.Clean(reason)) || Kept(reason)
}

// Kept reports whether the reason, cleaned, as it's from elsewhere, is one
// a request gives that the router kept where its session was: sticky, or
// kept for its thinking. The router keeps the reason its session's
// assignment came by as it keeps it.
func Kept(reason string) bool {
	reason = prose.Clean(reason)
	return reason == ReasonSticky || reason == ReasonBound
}

// isNew reports whether the reason, cleaned, is a new session's, whether or
// not the choice passed an account over for pressure.
func isNew(reason string) bool {
	if base, _, ok := passedOver(reason); ok {
		reason = base
	}
	return reason == ReasonNew
}

// How a reason ends that says the choice passed an account over under
// pressure, as in ", personal under pressure": before the account and after.
const (
	passingOver   = ", "
	underPressure = " under pressure"
)

// PassingOver is the reason given, ending as one does whose choice passed the
// account given over under pressure: "new" passing over personal is "new,
// personal under pressure", which PassedOver reads.
func PassingOver(reason, account string) string {
	return reason + passingOver + account + underPressure
}

// PassedOver is the account a reason ending ", personal under pressure" says
// the choice passed over, cleaned: "" for a reason without it.
func PassedOver(reason string) string {
	_, account, _ := passedOver(prose.Clean(reason))
	return account
}

// passedOver cuts the ending ", personal under pressure" from a reason,
// giving why the choice was made and the account it passed over, and reports
// false for a reason without it.
func passedOver(reason string) (base, account string, ok bool) {
	rest, ok := strings.CutSuffix(reason, underPressure)
	base, account, found := strings.CutLast(rest, passingOver)
	if !ok || !found || strings.Contains(account, " ") {
		return "", "", false
	}
	return base, account, true
}

// pressured reads a reason ending ", personal under pressure", which names the
// account the choice passed over, as "personal came under pressure, its 5-hour
// to run out at 15:10", the time as prose.Dated says it: pressure is only
// ever judged in the 5-hour window. Its words take the place of why the
// choice was made, but where the session was forced off where it was, as
// forced reads it, which they follow as a further clause: "work reached its
// cap, personal came under pressure".
func pressured(reason string, f Facts) (forms, bool) {
	base, account, ok := passedOver(reason)
	if !ok {
		return forms{}, false
	}
	w := Words{Named(account), Plain(" came under pressure")}
	if !f.Until.IsZero() {
		w = append(w, Plain(", its 5-hour to run out at "+prose.Dated(f.Now, f.Until)))
	}
	if why, ok := forced(base, f); ok {
		return why.then(w), true
	}
	return alike(w), true
}

// forced reads the reasons for going elsewhere than a session would, as the
// account it's on, or its own pin's, can't take the request: a move off its
// account, as movedOff reads it, and its pin yielding, as pinYields does.
func forced(reason string, f Facts) (forms, bool) {
	if w, ok := movedOff(reason, f); ok {
		return w, true
	}
	return pinYields(reason, f)
}

// settled reads the reasons that are whole words of their own. Those that
// move nothing are routed as the router gives them, as a request that moved
// its session never carries them.
func settled(reason string, f Facts) (forms, bool) {
	given := Words{Plain(reason)}
	switch reason {
	case ReasonNew:
		return forms{event: Words{Plain("started")}, session: Words{Plain("since it started")}}, true
	case ReasonSticky:
		return forms{event: Words{Plain("sticky")}, routed: given, session: Words{Plain("sticky")}}, true
	case ReasonBound:
		return forms{event: Words{Plain("kept for its thinking")}, routed: given, session: Words{Plain("kept for its thinking")}}, true
	case ReasonPinned:
		return alike(Words{Plain("pinned here")}), true
	case ReasonGlobalPin, ReasonMovedByPin:
		return alike(pinnedTo(f.To)), true
	case ReasonBack:
		return alike(Words{Plain("back where it was")}), true
	}
	return forms{}, false
}

// pinnedTo is a pin's words, to the account with the given id, where it's
// given.
func pinnedTo(account string) Words {
	if account == "" {
		return Words{Plain("pinned")}
	}
	return Words{Plain("pinned to "), Named(account)}
}

// rescored reads the reason for a session that idled being chosen afresh,
// "rescored after 15h idle", as Rescored gives it, which a session's page
// follows with what the choice found, where it's given.
func rescored(reason string, f Facts) (forms, bool) {
	rest, ok := strings.CutPrefix(reason, ReasonRescored)
	idle, ends := strings.CutSuffix(rest, rescoredIdle)
	if !ok || !ends || idle == "" {
		return forms{}, false
	}
	why := Words{Plain(ReasonRescored), {Text: idle, Kind: PartDuration}, Plain(rescoredIdle)}
	w := alike(why)
	if f.To != "" {
		w.session = append(slices.Clone(why), Plain(": "), Named(f.To), Plain(" had the most room"))
	}
	return w, true
}

// offWords are why a move off an account was forced, as the router says it,
// in the words of an event, of a request's routing, and of a phone.
var offWords = map[string]struct{ event, routed, short string }{
	WhyReserve: {event: " reached its cap", routed: "at its cap", short: "its cap"},
	WhyLimit:   {event: " reached its limit", routed: "at its limit", short: "its limit"},
	WhyRefused: {event: " refused the request", routed: "refused", short: "refused"},
	WhyNoRoom:  {event: " had no room", routed: "with no room", short: "no room"},
}

// Tried is an account a request went out on that couldn't serve it, and
// why, as the router says it, as "hit its limit": as a ledger line's tried
// gives it.
type Tried struct {
	Account, Why string
}

// triedWords are why a request went out again elsewhere, as the router says
// it, in the words of a request's routing, after the account it left.
var triedWords = map[string]string{
	WhyLimit:   "'s limit",
	WhyRefused: " refused it",
}

// TriedWords are the accounts a request went out on that couldn't serve it,
// in the order tried, as its routing reads them in place of its reason's
// words: "after work's limit", "after work refused it", and several as
// "after work's limit, personal's limit". A why the dashboard has no words
// for reads as the router gives it, cleaned, the account named apart all the
// same, as "after work has a new token". None where none was tried.
func TriedWords(tried ...Tried) Words {
	var w Words
	for i, t := range tried {
		lead := ", "
		if i == 0 {
			lead = "after "
		}
		why, known := triedWords[t.Why]
		if !known {
			why = " " + prose.Clean(t.Why)
		}
		w = append(w, Plain(lead), Named(t.Account), Plain(why))
	}
	return w
}

// movedOff reads the reason for a move off an account that can't take the
// request, "moved: work hit its limit", which names the account it left. A
// why the dashboard has no words for reads as the router gives it, the
// account named apart all the same.
func movedOff(reason string, _ Facts) (forms, bool) {
	account, why, ok := cutAccount(reason, ReasonMovedOff)
	if !ok {
		return forms{}, false
	}
	said, known := offWords[why]
	if !known {
		return alike(Words{Plain(ReasonMovedOff), Named(account), Plain(" " + why)}), true
	}
	event := Words{Named(account), Plain(said.event)}
	return forms{event: event, routed: Words{Plain(said.routed)}, session: event, short: Words{Plain(said.short)}}, true
}

// pinYields reads the reason for going elsewhere than the session's own pin
// sends it, "pin yields: side has no room", as "its pin to side yields: side
// has no room".
func pinYields(reason string, _ Facts) (forms, bool) {
	account, why, ok := cutAccount(reason, ReasonPinYields)
	if !ok {
		return forms{}, false
	}
	return alike(Words{Plain("its pin to "), Named(account), Plain(" yields: "), Named(account), Plain(" " + why)}), true
}

// cutAccount cuts a reason that starts as given and names an account, as
// "moved: work hit its limit" does after ReasonMovedOff, into the account and
// why, reporting false for any other reason.
func cutAccount(reason, start string) (account, why string, ok bool) {
	rest, started := strings.CutPrefix(reason, start)
	account, why, cut := strings.Cut(rest, " ")
	return account, why, started && cut
}
