package status

import (
	"slices"
	"strings"
	"time"
)

// Words are a reason the router gives, in the dashboard's words: in parts,
// each account they name, and how long a session idled, a part of its own,
// as the pages draw them apart.
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
// was, as From says; else "from work, " and why, as in "from work, at its
// cap", the account the one From gives. Any reason not in the dashboard's
// words reads as the router gives it, cleaned.
func RoutingWords(reason string, f Facts) Words {
	switch {
	case isNew(Clean(reason)):
		return Words{plain("new session")}
	case f.From == "":
		return nil
	}
	return slices.Concat(FromWords(f.From), Words{plain(", ")}, wordsOf(reason, f).routed)
}

// FromWords are a request's move of its session, the account it left alone,
// as a phone's Log says it: "from work", as RoutingWords starts.
func FromWords(from string) Words {
	return Words{plain("from "), named(from)}
}

// SessionWords are the reason as a session's page reads, as in "since it
// started", or "rescored after 15h idle: personal had the most room". Any
// reason not in the dashboard's words reads as the router gives it, cleaned.
func SessionWords(reason string, f Facts) Words {
	return wordsOf(reason, f).session
}

// RunningWords are the reason as a session running reads, with when it went
// to its account, as Dated says it: "started here 14:41", ending ", the best"
// while that's where new sessions go; "pinned here 14:39", its own pin; "its
// pin to side yields: side has no room"; "moved from work 14:31: work reached
// its cap", the account it left the reason's own where it names one; else
// "here since 14:39: " and its words as an event reads them. Any reason not
// in the dashboard's words reads as the router gives it, cleaned.
func RunningWords(reason string, f Facts) Words {
	reason = Clean(reason)
	at := ""
	if !f.At.IsZero() {
		at = " " + Dated(f.Now, f.At)
	}
	switch {
	case isNew(reason) && f.Best:
		return Words{plain("started here" + at + ", the best")}
	case isNew(reason):
		return Words{plain("started here" + at)}
	case reason == ReasonPinned:
		return Words{plain("pinned here" + at)}
	case strings.HasPrefix(reason, ReasonPinYields):
		return EventWords(reason, f)
	}
	if left, _, ok := cutAccount(reason, ReasonMovedOff); ok {
		f.From = left
	}
	var came Words
	switch {
	case f.From != "":
		came = Words{plain("moved from "), named(f.From), plain(at)}
	case at != "":
		came = Words{plain("here since" + at)}
	}
	switch {
	case reason == "":
		return came
	case came == nil:
		return EventWords(reason, f)
	}
	return slices.Concat(came, Words{plain(": ")}, EventWords(reason, f))
}

// forms are a reason's words in each place the dashboard says it: routed is
// why, as a request's routing says it after the account the session left.
type forms struct {
	event, routed, session Words
}

// alike is words that read the same everywhere.
func alike(w Words) forms {
	return forms{event: w, routed: w, session: w}
}

// plain is words around the accounts and times named.
func plain(text string) Part {
	return Part{Text: text}
}

// named is the account with the given id, cleaned, as it's from elsewhere.
func named(id string) Part {
	return Part{Text: Clean(id), Kind: PartAccount}
}

// readers each read a kind of reason, reporting false for any other, in the
// order they're tried: a choice that passed an account over for pressure says
// so after why it was made, which its words take the place of.
var readers = []func(reason string, f Facts) (forms, bool){pressured, settled, rescored, movedOff, pinYields}

// wordsOf is the reason's words, cleaned, as it's from elsewhere: as the
// router gives it where none of readers reads it.
func wordsOf(reason string, f Facts) forms {
	reason = Clean(reason)
	for _, read := range readers {
		if w, ok := read(reason, f); ok {
			return w
		}
	}
	return alike(Words{plain(reason)})
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
	_, account, _ := passedOver(Clean(reason))
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
// to run out at 15:10", the time as Dated says it: pressure is only ever
// judged in the 5-hour window.
func pressured(reason string, f Facts) (forms, bool) {
	_, account, ok := passedOver(reason)
	if !ok {
		return forms{}, false
	}
	w := Words{named(account), plain(" came under pressure")}
	if !f.Until.IsZero() {
		w = append(w, plain(", its 5-hour to run out at "+Dated(f.Now, f.Until)))
	}
	return alike(w), true
}

// settled reads the reasons that are whole words of their own. Those that
// move nothing are routed as the router gives them, as a request that moved
// its session never carries them.
func settled(reason string, f Facts) (forms, bool) {
	given := Words{plain(reason)}
	switch reason {
	case ReasonNew:
		return forms{event: Words{plain("started")}, session: Words{plain("since it started")}}, true
	case ReasonSticky:
		return forms{event: Words{plain("sticky")}, routed: given, session: Words{plain("sticky")}}, true
	case ReasonBound:
		return forms{event: Words{plain("kept for its thinking")}, routed: given, session: Words{plain("kept for its thinking")}}, true
	case ReasonPinned:
		return alike(Words{plain("pinned here")}), true
	case ReasonGlobalPin, ReasonMovedByPin:
		return alike(pinnedTo(f.To)), true
	case ReasonBack:
		return alike(Words{plain("back where it was")}), true
	}
	return forms{}, false
}

// pinnedTo is a pin's words, to the account with the given id, where it's
// given.
func pinnedTo(account string) Words {
	if account == "" {
		return Words{plain("pinned")}
	}
	return Words{plain("pinned to "), named(account)}
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
	why := Words{plain(ReasonRescored), {Text: idle, Kind: PartDuration}, plain(rescoredIdle)}
	w := alike(why)
	if f.To != "" {
		w.session = append(slices.Clone(why), plain(": "), named(f.To), plain(" had the most room"))
	}
	return w, true
}

// offWords are why a move off an account was forced, as the router says it,
// in the words of an event and of a request's routing.
var offWords = map[string]struct{ event, routed string }{
	WhyReserve: {event: " reached its cap", routed: "at its cap"},
	WhyLimit:   {event: " reached its limit", routed: "at its limit"},
	WhyRefused: {event: " refused the request", routed: "refused"},
	WhyNoRoom:  {event: " had no room", routed: "with no room"},
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
			why = " " + Clean(t.Why)
		}
		w = append(w, plain(lead), named(t.Account), plain(why))
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
		return alike(Words{plain(ReasonMovedOff), named(account), plain(" " + why)}), true
	}
	event := Words{named(account), plain(said.event)}
	return forms{event: event, routed: Words{plain(said.routed)}, session: event}, true
}

// pinYields reads the reason for going elsewhere than the session's own pin
// sends it, "pin yields: side has no room", as "its pin to side yields: side
// has no room".
func pinYields(reason string, _ Facts) (forms, bool) {
	account, why, ok := cutAccount(reason, ReasonPinYields)
	if !ok {
		return forms{}, false
	}
	return alike(Words{plain("its pin to "), named(account), plain(" yields: "), named(account), plain(" " + why)}), true
}

// cutAccount cuts a reason that starts as given and names an account, as
// "moved: work hit its limit" does after ReasonMovedOff, into the account and
// why, reporting false for any other reason.
func cutAccount(reason, start string) (account, why string, ok bool) {
	rest, started := strings.CutPrefix(reason, start)
	account, why, cut := strings.Cut(rest, " ")
	return account, why, started && cut
}
