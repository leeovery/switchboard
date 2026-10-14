package views

import (
	"cmp"
	"slices"
	"strconv"
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
	return toldFrom(m, n, overview)
}

// side is a side LATELY tells events from: tells reports whether it tells of
// an event at all, and tell tells one it does, its facts given, in full and
// short, with the kind its mark is drawn for.
type side struct {
	tells func(e status.Event) bool
	tell  func(t latelyTeller, e status.Event, f eventFact) (kind string, full, short Said)
}

// overview is the Overview's side: every event of the kinds LATELY tells
// of, each as its kind's teller tells it.
var overview = side{
	tells: func(e status.Event) bool {
		_, ok := latelyTellers[e.Kind]
		return ok
	},
	tell: func(t latelyTeller, e status.Event, f eventFact) (string, Said, Said) {
		full, short := latelyTellers[e.Kind](t, e, f)
		return e.Kind, full, short
	},
}

// toldFrom are the events of the document m is a moment of that from tells
// of, as it tells them at m's Now, the newest first, n at most, the sessions
// a limit, a cap or a refusal moved folded into its line, as Lately has
// them.
func toldFrom(m status.Moment, n int, from side) []Sentence {
	var told []status.Event
	for _, e := range m.Events {
		if from.tells(e) {
			told = append(told, e)
		}
	}
	t, facts := latelyTeller{now: m.Now}, latelyFacts(m)
	shown := unfolded(told, n)
	sentences := make([]Sentence, len(shown))
	for i, e := range shown {
		kind, full, short := from.tell(t, e, facts[e.ID])
		sentences[i] = Sentence{ID: e.ID, At: e.At, Kind: kind, Well: wellAgain(e), Full: full, Short: short}
	}
	return sentences
}

// latelyFacts are the facts of the events of the document m is a moment of,
// by their ids, worked out over them in the order the router told them, by
// their ids, each window named as m names it.
func latelyFacts(m status.Moment) map[int]eventFact {
	told := slices.SortedFunc(slices.Values(m.Events), func(a, b status.Event) int { return cmp.Compare(a.ID, b.ID) })
	facts := eventFacts{name: m.NameOf}
	byID := make(map[int]eventFact, len(told))
	for _, e := range told {
		byID[e.ID] = facts.take(e)
	}
	return byID
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

// latelyTeller tells LATELY's sentences at now, each event's facts given as
// latelyFacts works them out.
type latelyTeller struct {
	now time.Time
}

// latelyTellers tell an event of each kind LATELY tells of, its facts given,
// in full and short.
var latelyTellers = map[string]func(t latelyTeller, e status.Event, f eventFact) (full, short Said){
	status.EventStarted:  latelyTeller.started,
	status.EventMoved:    latelyTeller.moved,
	status.EventPressure: latelyTeller.pressured,
	status.EventCap:      latelyTeller.held,
	status.EventLimit:    latelyTeller.held,
	status.EventRefused:  latelyTeller.held,
	status.EventPrimed:   latelyTeller.befallen,
	status.EventRoom:     latelyTeller.befallen,
	status.EventPin:      latelyTeller.pinned,
	status.EventAuto:     latelyTeller.unpinned,
	status.EventRestart:  latelyTeller.restarting,
	status.EventHealth:   latelyTeller.turned,
}

// started tells of a session starting: "c61b started on side", and why,
// where it wasn't on the router's best: ", as pinned" where a pin sent it,
// else the reason in the views' words, as ": personal came under pressure".
func (t latelyTeller) started(e status.Event, f eventFact) (Said, Said) {
	s := Said{Subject: session(e.Session), Words: clause(" started on "), Object: account(e.Account), Rest: t.startedWhy(e, f, false)}
	return s, s
}

// startedWhy says why a session started where it did, as started has it:
// on the router's best, as a new session's reason says it did, nothing, or
// ", the best" where saysBest says to, the event's own fact whatever the
// best is since.
func (t latelyTeller) startedWhy(e status.Event, f eventFact, saysBest bool) status.Words {
	switch prose.Clean(e.Reason) {
	case "", status.ReasonNew:
		if saysBest {
			return clause(", the best")
		}
		return nil
	case status.ReasonPinned, status.ReasonGlobalPin:
		return clause(", as pinned")
	}
	return slices.Concat(clause(": "), status.EventWords(e.Reason, t.facts(f, "", e.Account)))
}

// moved tells of a session's model moving: "db8a moved personal → work", and
// why, in the views' words, its object the account the reason names first,
// as in ": personal came under pressure".
func (t latelyTeller) moved(e status.Event, f eventFact) (Said, Said) {
	words := status.Words{status.Plain(" moved "), status.Named(e.From), status.Plain(" → "), status.Named(e.To)}
	why := status.EventWords(e.Reason, t.facts(f, e.From, e.To))
	if len(why) > 0 {
		words = append(words, status.Plain(": "))
	}
	before, object, after := atAccount(why)
	s := Said{Subject: session(e.Session), Words: slices.Concat(words, before), Object: object, Rest: after}
	return s, s
}

// pressured tells of an account coming under pressure, and when its window
// runs out, where the event says: "personal came under pressure: its 5-hour
// runs out ~16:05", or short, "personal under pressure: out ~16:05".
func (t latelyTeller) pressured(e status.Event, f eventFact) (Said, Said) {
	full := Said{Subject: account(e.Account), Words: clause(" came under pressure")}
	short := Said{Subject: account(e.Account), Words: clause(" under pressure")}
	if e.Until.IsZero() {
		return full, short
	}
	out := clause("~" + prose.Dated(t.now, e.Until))
	full.Words = clause(" came under pressure: its " + firstOr(f.windows, "window") + " runs out ")
	short.Words = clause(" under pressure: out ")
	full.Object, short.Object = out, out
	return full, short
}

// held tells of a cap, a limit or a refusal holding an account back, its
// name first, then what befell it, as befell says: "work reached its cap",
// or short, "work at its cap"; "personal reached its 5-hour limit", or
// "personal at its limit"; "work refused (401)"; and the sessions it moved,
// as heldBack has them.
func (latelyTeller) held(e status.Event, f eventFact) (Said, Said) {
	full, short := befell(e, f)
	return heldBack(account(e.Account), e, f, " "+full, false), heldBack(account(e.Account), e, f, " "+short, true)
}

// befallen tells of a prime or room again befalling an account, its name
// first, then what befell it, as befell says: "side primed: its 5-hour
// window started", or short, "side primed"; "work open again: its 5-hour
// window reset", or short, "work open again".
func (latelyTeller) befallen(e status.Event, f eventFact) (Said, Said) {
	full, short := befell(e, f)
	return Said{Subject: account(e.Account), Words: clause(" " + full)}, Said{Subject: account(e.Account), Words: clause(" " + short)}
}

// befell says what befell an account, of an event of a kind that befalls
// one, in full and short: "reached its cap" and "at its cap"; "reached its
// 5-hour limit", in the windows it names, and "at its limit"; the upstream's
// answer, as status.RefusalAnswer says it, "refused (401)", or of a model
// family's alone, "refused (403, opus)"; "primed: its 5-hour window
// started", the windows the prime started, and "primed"; and "open again:
// its 5-hour window reset", where it names the windows that held it back,
// and "open again". It's nothing of any other kind.
func befell(e status.Event, f eventFact) (full, short string) {
	switch e.Kind {
	case status.EventCap:
		return "reached its cap", "at its cap"
	case status.EventLimit:
		return "reached its " + prose.ListOf(f.windows, "limit"), "at its limit"
	case status.EventRefused:
		answer := status.RefusalAnswer(e.Status, e.Family)
		return answer, answer
	case status.EventPrimed:
		return "primed: " + its(f.windows, "its window") + " started", "primed"
	case status.EventRoom:
		if len(f.windows) == 0 {
			return "open again", "open again"
		}
		return "open again: " + its(f.windows, "") + " reset", "open again"
	}
	return "", ""
}

// pinned tells of routing set by hand: the global pin, "pin set by hand: new
// sessions go to personal", or with its move, "new and running sessions";
// or a session's own, "pin set by hand: c61b goes to side". Short, it's "pin
// set: …".
func (latelyTeller) pinned(e status.Event, _ eventFact) (Said, Said) {
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
func (latelyTeller) unpinned(e status.Event, _ eventFact) (Said, Said) {
	who := clause("new sessions")
	if e.Session != "" {
		who = session(e.Session)
	}
	s := Said{Subject: who, Words: clause(" back to auto")}
	return s, s
}

// restarting tells of a restart falling due, and why, in the Log's words:
// "restart due: the config changed".
func (latelyTeller) restarting(e status.Event, _ eventFact) (Said, Said) {
	reason := restartWhy(e.Reason)
	s := Said{Subject: clause("restart"), Words: clause(" due")}
	if reason != "" {
		s.Words = clause(" due: " + reason)
	}
	return s, s
}

// turned tells of the router's health turning: "the router turned
// unhealthy: <its reason>", or "the router is healthy again".
func (latelyTeller) turned(e status.Event, _ eventFact) (Said, Said) {
	s := Said{Subject: clause("the router"), Words: clause(" is healthy again")}
	if reason := prose.Clean(e.Reason); reason != "" {
		s.Words = clause(" turned unhealthy: " + reason)
	}
	return s, s
}

// facts are what a reason's words take of an event, its facts f, from and
// to the accounts given, its times shown beside now.
func (t latelyTeller) facts(f eventFact, from, to string) status.Facts {
	return status.Facts{From: from, To: to, Until: f.runsOut, Now: t.now}
}

// heldBack is the line of an account a cap, a limit or a refusal held back,
// subject naming it, then what befell it, as in " reached its cap", and after
// that the sessions the event moved, as its facts f count them, where it
// moved any: in full, as in ": 3 sessions moved to side", the count its
// object; short, as in ": 3 moved to side"; and where they went to several
// accounts, "moved to other accounts".
func heldBack(subject status.Words, e status.Event, f eventFact, what string, short bool) Said {
	s := Said{Subject: subject, Words: clause(what)}
	if f.moved == 0 {
		return s
	}
	count := prose.SessionCount(f.moved)
	if short {
		count = strconv.Itoa(f.moved)
	}
	to := status.Words{status.Plain("other accounts")}
	if f.to != "" {
		to = status.Words{status.Named(f.to)}
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

// pinnedTo are the accounts the global pin names, as pinnedNames has them,
// as English runs them together, "personal and side".
func pinnedTo(e status.Event) status.Words {
	return joined(pinnedNames(e), prose.And)
}

// pinnedNames are the accounts the global pin names, cleaned: its first, as
// Account gives it, where it lists none.
func pinnedNames(e status.Event) []string {
	if names := cleaned(e.Accounts); len(names) > 0 {
		return names
	}
	return cleaned([]string{e.Account})
}

// session is the session with the given id, named as status.ShownID names
// it: none for "".
func session(id string) status.Words {
	if prose.Clean(id) == "" {
		return nil
	}
	return clause(status.ShownID(id))
}
