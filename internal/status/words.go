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
	// Now gives the time zone Until is shown in.
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
	return append(Words{plain("from "), named(f.From), plain(", ")}, wordsOf(reason, f).routed...)
}

// SessionWords are the reason as a session's page reads, as in "since it
// started", or "rescored after 15h idle: personal had the most room". Any
// reason not in the dashboard's words reads as the router gives it, cleaned.
func SessionWords(reason string, f Facts) Words {
	return wordsOf(reason, f).session
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
	rest, ok := strings.CutSuffix(reason, " under pressure")
	base, account, found := strings.CutLast(rest, ", ")
	if !ok || !found || strings.Contains(account, " ") {
		return "", "", false
	}
	return base, account, true
}

// pressured reads a reason ending ", personal under pressure", which names the
// account the choice passed over, as "personal came under pressure, its 5-hour
// to run out at 15:10": pressure is only ever judged in the 5-hour window.
func pressured(reason string, f Facts) (forms, bool) {
	_, account, ok := passedOver(reason)
	if !ok {
		return forms{}, false
	}
	w := Words{named(account), plain(" came under pressure")}
	if !f.Until.IsZero() {
		w = append(w, plain(", its 5-hour to run out at "+TimeOfDay(f.Now, f.Until)))
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
	idle, idled := strings.CutSuffix(rest, rescoredIdle)
	if !ok || !idled || idle == "" {
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

// movedOff reads the reason for a move off an account that can't take the
// request, "moved: work hit its limit", which names the account it left. A
// why the dashboard has no words for reads as the router gives it, the
// account named apart all the same.
func movedOff(reason string, _ Facts) (forms, bool) {
	rest, ok := strings.CutPrefix(reason, ReasonMovedOff)
	account, why, cut := strings.Cut(rest, " ")
	if !ok || !cut {
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
	rest, ok := strings.CutPrefix(reason, ReasonPinYields)
	account, why, cut := strings.Cut(rest, " ")
	if !ok || !cut {
		return forms{}, false
	}
	return alike(Words{plain("its pin to "), named(account), plain(" yields: "), named(account), plain(" " + why)}), true
}
