package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// passedOver ends the reason the router gives for a choice that passed over
// an account under pressure, as in "new, work under pressure".
const passedOver = " under pressure"

// counted reports whether an event is a move a limit forced, which the
// limit's own line counts.
func counted(e status.Event) bool {
	return e.Kind == status.EventMoved && e.Limit != 0
}

// told is what RECENT says of an event: its mark, a glyph in the ink of what
// it tells of, and its words, its times in now's time zone, accounts by their
// names; of what befell an account, its name, then what aside says of it;
// reporting false for a kind it doesn't tell of, as one from a later router.
func told(e status.Event, doc status.Document, now time.Time) (span, line, bool) {
	e.At = e.At.In(now.Location())
	switch e.Kind {
	case status.EventStarted:
		return span{"▲", accentInk}, slices.Concat(line{{status.ShownID(e.Session), titleInk}, {" started on ", mutedInk}, {docNamed(doc, e.Account), strongInk}}, why(doc, e.Reason)), true
	case status.EventMoved:
		return movedMark, moveSaid(e, doc, why(doc, e.Reason)), true
	case status.EventRestart:
		return span{"!", warningInk}, line{{"restart due (" + prose.Clean(e.Reason) + ")", mutedInk}}, true
	case status.EventHealth:
		if e.Reason != "" {
			return span{"●", errorInk}, line{{because("the router turned unhealthy", e.Reason), mutedInk}}, true
		}
		return span{"●", positiveInk}, line{{"the router is healthy again", mutedInk}}, true
	}
	if mark, gist, tail, ok := aside(e, doc); ok {
		return mark, slices.Concat(line{{docNamed(doc, e.Account), strongInk}, {" " + gist, mutedInk}}, tail), true
	}
	return span{}, nil, false
}

// aside is what's told of an event that befell an account, from the
// account's side, which RECENT tells after its name: its mark, a glyph in the
// ink of what it tells of; its gist, as in "reached its session limit"; and
// what follows it, as in "; 3 sessions moved to side", in the ink of their
// own. It reports false for an event of any other kind.
func aside(e status.Event, doc status.Document) (mark span, gist string, tail line, ok bool) {
	switch e.Kind {
	case status.EventPressure:
		return span{"●", warningInk}, "came under pressure", runsOut(e, doc), true
	case status.EventLimit:
		return span{"■", errorInk}, "reached its " + limits(e, doc), moves(e, doc), true
	case status.EventRefused:
		mark, words, until := refusal(e)
		return mark, words, line{{until, mutedInk}}, true
	case status.EventPrimed:
		return span{"◇", primedInk}, "primed: its " + spanOf(first(e.Windows)) + " started", line{{resetting(e), mutedInk}}, true
	case status.EventRoom:
		return span{"●", positiveInk}, "has room again", nil, true
	}
	return span{}, "", nil, false
}

// movedMark marks a session's move, as RECENT and LOG tell of it.
var movedMark = span{"▸", dimInk}

// moveSaid says a session's model moved, from where to where, as in "d28c
// moved work → side", then why, as said gives it.
func moveSaid(e status.Event, doc status.Document, said line) line {
	return slices.Concat(line{{status.ShownID(e.Session), titleInk}, {" moved ", mutedInk}, {docNamed(doc, e.From), strongInk}, {" → ", mutedInk}, {docNamed(doc, e.To), strongInk}}, said)
}

// why says why a session went where it did, as the router's reason gives it,
// accounts by their names in doc: ", the best" where it was chosen afresh,
// " (pin)" where a pin sent it, else the reason itself, as namedIn has it;
// and the account under pressure the choice passed over, where it passed
// one.
func why(doc status.Document, reason string) line {
	said, passed := passedOverIn(reason)
	var l line
	switch said {
	case "":
	case status.ReasonNew:
		l = line{{", the best", mutedInk}}
	case status.ReasonPinned, status.ReasonGlobalPin, status.ReasonMovedByPin:
		l = line{{" (pin)", mutedInk}}
	default:
		l = line{{": " + namedIn(doc, said), mutedInk}}
	}
	return slices.Concat(l, passing(doc, passed))
}

// namedIn is a reason the router gives, the account it leads with named as
// doc names it: one moved off, as in "work has no room", without the reason's
// "moved: "; or one a session's own pin yielded at, as in "pin yields: work
// has no room". Any other reason is as it is.
func namedIn(doc status.Document, said string) string {
	if rest, ok := strings.CutPrefix(said, status.ReasonMovedOff); ok {
		return namedFirst(doc, rest)
	}
	if rest, ok := strings.CutPrefix(said, status.ReasonPinYields); ok {
		return status.ReasonPinYields + namedFirst(doc, rest)
	}
	return said
}

// namedFirst is words led by an account's id, the id named as doc names it.
func namedFirst(doc status.Document, words string) string {
	id, _, _ := strings.Cut(words, " ")
	return docNamed(doc, id) + words[len(id):]
}

// passedOverIn parts the reason the router gives for where a session went,
// cleaned, into what it says of the choice, and the account under pressure
// the choice passed over, "" where it passed none.
func passedOverIn(reason string) (said, passed string) {
	reason = prose.Clean(reason)
	if i := strings.LastIndex(reason, ", "); i >= 0 && strings.HasSuffix(reason, passedOver) {
		return reason[:i], reason[i+2:]
	}
	return reason, ""
}

// passing says the choice passed over the account under pressure with the
// given id, named as doc names it, as in ", passing over work under
// pressure": nothing where it passed none.
func passing(doc status.Document, passed string) line {
	if passed == "" {
		return nil
	}
	name, pressed, _ := strings.Cut(passed, " ")
	return line{{", passing over " + docNamed(doc, name) + " " + pressed, mutedInk}}
}

// runsOut says when, at the rate an account came under pressure at, its
// window runs out, and the span that rate is measured over, as in ": its
// session runs out ~16:05 at its last-30-min rate"; or reaches its reserve,
// where its reserve held it back as it came under pressure, as reserveHeld
// says. It's nothing where the event doesn't say when.
func runsOut(e status.Event, doc status.Document) line {
	if e.Until.IsZero() {
		return nil
	}
	rate := "at its pace"
	if !e.Since.IsZero() {
		rate = "at its " + lately(e.Since, e.At) + " rate"
	}
	out := " runs out ~"
	if reserveHeld(e, doc) {
		out = " reaches its reserve ~"
	}
	window := windowName(doc, e.Account, first(e.Windows))
	return line{{": its " + window + out + prose.When(e.At, e.Until) + " " + rate, mutedInk}}
}

// reserveHeld reports whether doc's account the event e befell had its
// reserve hold it back when e came, as the router judged its pressure: it
// keeps one, and no pin spent it then, as the global pin naming it now does
// only where it was set before e.
func reserveHeld(e status.Event, doc status.Document) bool {
	a, ok := doc.Account(e.Account)
	if !ok || a.Reserve <= 0 {
		return false
	}
	return !doc.Pin.Has(a.ID) || doc.Pin.Since.After(e.At)
}

// limits names the limits an event says were reached, as in "session limit"
// or "session and week limits", or "limit" where it names none.
func limits(e status.Event, doc status.Document) string {
	windows := make([]string, len(e.Windows))
	for i, key := range e.Windows {
		windows[i] = windowName(doc, e.Account, key)
	}
	switch len(windows) {
	case 0:
		return "limit"
	case 1:
		return windows[0] + " limit"
	default:
		return prose.List(windows) + " limits"
	}
}

// moves says how many sessions a limit moved, and where, where they all went
// to one account, as in "; 3 sessions moved to side"; nothing where it moved
// none.
func moves(e status.Event, doc status.Document) line {
	if e.Count == 0 {
		return nil
	}
	l := line{{"; ", mutedInk}, {prose.SessionCount(e.Count), secondaryInk}, {" moved", mutedInk}}
	if e.To != "" {
		l = append(l, span{" to ", mutedInk}, span{docNamed(doc, e.To), strongInk})
	}
	return l
}

// movedTo says where the sessions a limit moved went, where they all went to
// one account, as in " to side": nothing where they didn't.
func movedTo(doc status.Document, e status.Event) string {
	if e.To == "" {
		return ""
	}
	return " to " + docNamed(doc, e.To)
}

// refusal is what's said of the upstream refusing requests on an account,
// after its name: its mark, a refusal of its token, which holds back every
// request, in state.destructive, or of a model family's requests alone,
// which hold back less, in accent.attention; what was refused, as
// status.RefusalAnswer says it, as in "was refused (403, opus)"; and until
// when, as in " until 21:40", where the event says.
func refusal(e status.Event) (mark span, words, until string) {
	mark = span{"■", errorInk}
	if e.Family != "" {
		mark = span{"■", warningInk}
	}
	if !e.Until.IsZero() {
		until = " until " + prose.When(e.At, e.Until)
	}
	return mark, "was " + status.RefusalAnswer(e.Status, e.Family), until
}

// resetting says when the window a prime started resets, as in ",
// resetting 18:50", where the event says.
func resetting(e status.Event) string {
	if e.Until.IsZero() {
		return ""
	}
	return ", resetting " + prose.When(e.At, e.Until)
}

// windowName names an account's window in a sentence, as prosed has its
// label, or by its key, cleaned, where doc lacks it.
func windowName(doc status.Document, account, key string) string {
	if a, ok := doc.Account(account); ok {
		if w, ok := a.Window(key); ok {
			return status.InProse(w.Label)
		}
	}
	return prose.Clean(key)
}

// first is the first of keys, or "" for none.
func first(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// telling is an event as RECENT tells of it: its id, what leads its line,
// its time and its mark, and its words.
type telling struct {
	id    int
	lead  line
	words line
}

// tellingsOf are the events given, as tell tells of each at now: each led by
// its time, as Dated shows it, the first's standing out, in a column as wide
// as the widest, timeGap after it, then its mark.
func (f Frame) tellingsOf(events []status.Event, doc status.Document, now time.Time, tell func(status.Event, status.Document, time.Time) (span, line, bool)) []telling {
	column := 0
	for _, e := range events {
		column = max(column, len(prose.Dated(now, e.At)))
	}
	tellings := make([]telling, len(events))
	for i, e := range events {
		mark, words, _ := tell(e, doc, now)
		at := span{fmt.Sprintf("%-*s", column, prose.Dated(now, e.At)) + f.timeGap(), dimInk}
		if i == 0 {
			at.ink = secondaryInk
		}
		tellings[i] = telling{id: e.ID, lead: line{at, mark, spaces(1)}, words: words}
	}
	return tellings
}

// timeGap is the blanks after a column of times, as LOG has them: two, but
// on a phone, one.
func (f Frame) timeGap() string {
	if f.phone() {
		return " "
	}
	return "  "
}
