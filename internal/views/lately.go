package views

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// Sentence is one of the router's events as LATELY tells it: its id, which
// the cue on a new row follows; when it happened; its kind, which its mark is
// drawn for, and, of the router's health, whether it's well again, which
// colours that mark apart; and its words, in full, and short, as a phone has
// them, as "work reached its cap: 3 sessions moved to side" and "work at its
// cap: 3 moved to side".
type Sentence struct {
	ID          int
	At          time.Time
	Kind        string
	Well        bool
	Full, Short Said
}

// Said is a sentence of LATELY's in the parts it's drawn in: its subject, as
// "work", "c61b" or "pin"; the words said of it; its object, where it has
// one, as "side" or "3 sessions"; and the words after that. Each names
// accounts apart, by id, as status.Words do, and a session as status.ShownID
// names it.
type Said struct {
	Subject, Words, Object, Rest status.Words
}

// String runs the sentence together, as a page drawn without colour shows it.
func (s Said) String() string {
	return slices.Concat(s.Subject, s.Words, s.Object, s.Rest).String()
}

// Lately are the events of the document m is a moment of, as LATELY tells
// them at its Now, the newest first, n at most: those of the kinds it tells
// of, the sessions a limit, a cap or a refusal moved folded into its line,
// counted, a move showing alone only where the line it's folded into isn't
// among those shown.
func Lately(m status.Moment, n int) []Sentence {
	var told []status.Event
	for _, e := range m.Events {
		if _, ok := latelyTellers[e.Kind]; ok {
			told = append(told, e)
		}
	}
	t := latelyTeller{now: m.Now, events: m.Events, windows: NewTelling(m.NameOf)}
	shown := unfolded(told, n)
	sentences := make([]Sentence, len(shown))
	for i, e := range shown {
		full, short := latelyTellers[e.Kind](t, e)
		well := e.Kind == status.EventHealth && status.Clean(e.Reason) == ""
		sentences[i] = Sentence{ID: e.ID, At: e.At, Kind: e.Kind, Well: well, Full: full, Short: short}
	}
	return sentences
}

// unfolded are as many of events as n allows, in their order, the newest
// first, but a move folded into the line of the event that forced it, as
// foldedInto says, while that line is among them: a move whose line doesn't
// show, as it's too old to, shows itself.
func unfolded(events []status.Event, n int) []status.Event {
	alone := make(map[int]bool)
	for {
		var kept []status.Event
		lines := make(map[int]bool)
		for _, e := range events {
			if len(kept) >= n {
				break
			}
			if into := foldedInto(e); into != 0 && !alone[into] {
				continue
			}
			kept, lines[e.ID] = append(kept, e), true
		}
		more := false
		for _, e := range events {
			if into := foldedInto(e); into != 0 && !alone[into] && !lines[into] {
				alone[into], more = true, true
			}
		}
		if !more {
			return kept
		}
	}
}

// foldedInto is the id of the event whose line a move is folded into, as one
// of the sessions a limit, a cap or a refusal moved: zero for a move none
// counts, and for any other event. A router from before forced_by gives a
// limit's moves their limit's id alone.
func foldedInto(e status.Event) int {
	if e.Kind != status.EventMoved {
		return 0
	}
	return cmp.Or(e.ForcedBy, e.Limit)
}

// latelyTeller tells LATELY's sentences at now, of the document's events,
// which a reason passing an account over under pressure looks back on for
// when it runs out, its windows named as the Log names them.
type latelyTeller struct {
	now     time.Time
	events  []status.Event
	windows *Telling
}

// latelyTellers tell an event of each kind LATELY tells of, in full and
// short.
var latelyTellers = map[string]func(t latelyTeller, e status.Event) (full, short Said){
	status.EventStarted:  latelyTeller.started,
	status.EventMoved:    latelyTeller.moved,
	status.EventPressure: latelyTeller.pressured,
	status.EventCap:      latelyTeller.capped,
	status.EventLimit:    latelyTeller.limited,
	status.EventRefused:  latelyTeller.refused,
	status.EventPrimed:   latelyTeller.primed,
	status.EventRoom:     latelyTeller.opened,
	status.EventPin:      latelyTeller.pinned,
	status.EventAuto:     latelyTeller.unpinned,
	status.EventRestart:  latelyTeller.restarting,
	status.EventHealth:   latelyTeller.turned,
}

// started tells of a session starting: "c61b started on side", and why,
// where it wasn't on the router's best: ", as pinned" where a pin sent it,
// else the reason in the views' words, as ": personal came under pressure".
func (t latelyTeller) started(e status.Event) (Said, Said) {
	s := Said{Subject: session(e.Session), Words: clause(" started on "), Object: account(e.Account), Rest: t.startedWhy(e)}
	return s, s
}

// startedWhy says why a session started where it did, as started has it.
func (t latelyTeller) startedWhy(e status.Event) status.Words {
	switch status.Clean(e.Reason) {
	case "", status.ReasonNew:
		return nil
	case status.ReasonPinned, status.ReasonGlobalPin:
		return clause(", as pinned")
	}
	return slices.Concat(clause(": "), status.EventWords(e.Reason, t.facts(e, "", e.Account)))
}

// moved tells of a session's model moving: "db8a moved personal → work", and
// why, in the views' words, its object the account the reason names first,
// as in ": personal came under pressure".
func (t latelyTeller) moved(e status.Event) (Said, Said) {
	words := status.Words{said(" moved "), named(e.From), said(" → "), named(e.To)}
	why := status.EventWords(e.Reason, t.facts(e, e.From, e.To))
	if len(why) > 0 {
		words = append(words, said(": "))
	}
	before, object, after := atAccount(why)
	s := Said{Subject: session(e.Session), Words: slices.Concat(words, before), Object: object, Rest: after}
	return s, s
}

// pressured tells of an account coming under pressure, and when its window
// runs out, where the event says: "personal came under pressure: its 5-hour
// runs out ~16:05", or short, "personal under pressure: out ~16:05".
func (t latelyTeller) pressured(e status.Event) (Said, Said) {
	full := Said{Subject: account(e.Account), Words: clause(" came under pressure")}
	short := Said{Subject: account(e.Account), Words: clause(" under pressure")}
	if e.Until.IsZero() {
		return full, short
	}
	out := clause("~" + status.Dated(t.now, e.Until))
	full.Words = clause(" came under pressure: its " + t.windows.windowOf(first(e.Windows)) + " runs out ")
	short.Words = clause(" under pressure: out ")
	full.Object, short.Object = out, out
	return full, short
}

// capped tells of an account reaching its cap: "work reached its cap", or
// short, "work at its cap", and the sessions it moved.
func (latelyTeller) capped(e status.Event) (Said, Said) {
	return heldBack(e, " reached its cap", false), heldBack(e, " at its cap", true)
}

// limited tells of an account reaching its limit, in the windows it names:
// "personal reached its 5-hour limit", or short, "personal at its limit", and
// the sessions it moved.
func (t latelyTeller) limited(e status.Event) (Said, Said) {
	return heldBack(e, " reached its "+t.limits(e.Windows), false), heldBack(e, " at its limit", true)
}

// refused tells of the upstream refusing an account's requests, and its
// answer: "work refused (401)", or of a model family's alone, "work refused
// (403, opus)"; and the sessions it moved.
func (latelyTeller) refused(e status.Event) (Said, Said) {
	var answer []string
	if e.Status != 0 {
		answer = append(answer, strconv.Itoa(e.Status))
	}
	if family := status.Clean(e.Family); family != "" {
		answer = append(answer, family)
	}
	what := " refused"
	if len(answer) > 0 {
		what += " (" + strings.Join(answer, ", ") + ")"
	}
	return heldBack(e, what, false), heldBack(e, what, true)
}

// primed tells of a prime starting an account's window: "side primed: its
// 5-hour window started", or short, "side primed".
func (t latelyTeller) primed(e status.Event) (Said, Said) {
	full := Said{Subject: account(e.Account), Words: clause(" primed: " + t.windows.its(e.Windows, "its window") + " started")}
	return full, Said{Subject: account(e.Account), Words: clause(" primed")}
}

// opened tells of an account with room again, and the windows that held it
// back resetting, where it names them: "work open again: its 5-hour window
// reset", or short, "work open again".
func (t latelyTeller) opened(e status.Event) (Said, Said) {
	short := Said{Subject: account(e.Account), Words: clause(" open again")}
	if len(e.Windows) == 0 {
		return short, short
	}
	return Said{Subject: account(e.Account), Words: clause(" open again: " + t.windows.its(e.Windows, "") + " reset")}, short
}

// pinned tells of routing set by hand: the global pin, "pin set by hand: new
// sessions go to personal", or with its move, "new and running sessions";
// or a session's own, "pin set by hand: c61b goes to side". Short, it's "pin
// set: …".
func (latelyTeller) pinned(e status.Event) (Said, Said) {
	who, accounts := "new sessions go to ", pinnedTo(e)
	switch {
	case e.Session != "":
		who, accounts = status.ShownID(e.Session)+" goes to ", account(e.Account)
	case e.Move:
		who = "new and running sessions go to "
	}
	full := Said{Subject: clause("pin"), Words: clause(" set by hand: " + who), Object: accounts}
	short := Said{Subject: clause("pin"), Words: clause(" set: " + who), Object: accounts}
	return full, short
}

// unpinned tells of routing given back to the router: "new sessions back to
// auto", or of a session's own pin cleared, "c61b back to auto".
func (latelyTeller) unpinned(e status.Event) (Said, Said) {
	who := clause("new sessions")
	if e.Session != "" {
		who = session(e.Session)
	}
	s := Said{Subject: who, Words: clause(" back to auto")}
	return s, s
}

// restarting tells of a restart falling due, and why, in the Log's words:
// "restart due: the config changed".
func (latelyTeller) restarting(e status.Event) (Said, Said) {
	reason := status.Clean(e.Reason)
	if words, ok := restartWords[reason]; ok {
		reason = words
	}
	s := Said{Subject: clause("restart"), Words: clause(" due")}
	if reason != "" {
		s.Words = clause(" due: " + reason)
	}
	return s, s
}

// turned tells of the router's health turning: "the router turned
// unhealthy: <its reason>", or "the router is healthy again".
func (latelyTeller) turned(e status.Event) (Said, Said) {
	s := Said{Subject: clause("the router"), Words: clause(" is healthy again")}
	if reason := status.Clean(e.Reason); reason != "" {
		s.Words = clause(" turned unhealthy: " + reason)
	}
	return s, s
}

// facts are what a reason's words take of the event e, from and to the
// accounts given: where it passed an account over under pressure, when that
// account's latest pressure event before e said it runs out, where e came
// before then.
func (t latelyTeller) facts(e status.Event, from, to string) status.Facts {
	facts := status.Facts{From: from, To: to, Now: t.now}
	passed := status.PassedOver(e.Reason)
	if passed == "" {
		return facts
	}
	for _, p := range t.events {
		if p.ID < e.ID && p.Kind == status.EventPressure && status.Clean(p.Account) == passed && !p.Until.IsZero() {
			if e.At.Before(p.Until) {
				facts.Until = p.Until
			}
			return facts
		}
	}
	return facts
}

// limits names the limits an event says were reached, as in "5-hour limit"
// or "5-hour and week limits", or "limit" where it names none.
func (t latelyTeller) limits(keys []string) string {
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = t.windows.windowOf(key)
	}
	switch len(names) {
	case 0:
		return "limit"
	case 1:
		return names[0] + " limit"
	default:
		return prose.List(names) + " limits"
	}
}

// heldBack is the line of an account a cap, a limit or a refusal held back,
// its name, then what befell it, as in " reached its cap", and after that the
// sessions the event moved, counted, where it moved any: in full, as in ": 3
// sessions moved to side", the count its object; short, as in ": 3 moved to
// side"; and where they went to several accounts, "moved to other accounts".
func heldBack(e status.Event, what string, short bool) Said {
	s := Said{Subject: account(e.Account), Words: clause(what)}
	if e.Count == 0 {
		return s
	}
	count := status.SessionCount(e.Count)
	if short {
		count = strconv.Itoa(e.Count)
	}
	to := status.Words{said("other accounts")}
	if e.To != "" {
		to = status.Words{named(e.To)}
	}
	s.Words = clause(what + ": ")
	s.Object = clause(count)
	s.Rest = slices.Concat(clause(" moved to "), to)
	return s
}

// atAccount parts words at the first account they name: the words before it,
// the account, and the words after; or the words whole, where they name
// none.
func atAccount(words status.Words) (before, account, after status.Words) {
	i := slices.IndexFunc(words, func(p status.Part) bool { return p.Kind == status.PartAccount })
	if i < 0 {
		return words, nil, nil
	}
	return words[:i], words[i : i+1], words[i+1:]
}

// pinnedTo are the accounts the global pin names, as English runs them
// together, "personal and side": its first, as Account gives it, where it
// lists none.
func pinnedTo(e status.Event) status.Words {
	names := cleaned(e.Accounts)
	if len(names) == 0 {
		return account(e.Account)
	}
	return joined(names, " and ")
}

// session is the session with the given id, named as status.ShownID names
// it: none for "".
func session(id string) status.Words {
	if status.Clean(id) == "" {
		return nil
	}
	return clause(status.ShownID(id))
}
