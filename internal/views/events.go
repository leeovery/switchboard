package views

import (
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// EventLine is an event as the Log's Events and LATELY tell it, and events
// prints it: the kind's word, the account it befell and what happened, the
// accounts each names apart, as status.Words part them.
type EventLine struct {
	// Kind is the kind's word, as "open" is the router's room: an unknown
	// kind's, as from a later router, as it's given, cleaned.
	Kind string
	// Account is the account it befell: a move's "work → side", a pin's
	// accounts joined with ", ", and none for routing given back to the
	// router, a restart or the router's health.
	Account status.Words
	// What is what happened, in the Log's words, as "its 5-hour window,
	// till 23:58: its session moves to side".
	What status.Words
}

// kindWords are the Log's words for the kinds the router names otherwise.
var kindWords = map[string]string{status.EventRoom: "open"}

// restartWords are why a restart fell due, in the Log's words.
var restartWords = map[string]string{
	status.RestartForConfig:  "the config changed",
	status.RestartForUpgrade: "switchboard upgraded",
	status.RestartForZone:    "the time zone changed",
}

// by says who set or cleared a pin, as an event's By gives it, in the Log's
// words: nothing for anyone else, or no one.
var by = map[string]string{"cli": ", set from the command line", "dashboard": ", set from the dashboard"}

// forced says a global pin set or cleared with force cleared every
// session's own pin too: nothing of one without.
func forced(e status.Event) string {
	if !e.Force {
		return ""
	}
	return ", every session's own pin cleared too"
}

// Telling tells the router's events as their lines, oldest first, as they're
// read: it remembers when each account's latest pressure event said it runs
// out, for a move whose choice later passed the account over. Its zero value
// names a window by its key, cleaned. A Telling isn't safe for concurrent
// use.
type Telling struct {
	window   func(key string) string
	pressure map[string]time.Time
}

// NewTelling returns a Telling that names a window by its key as window
// does, as "5-hour" or "week", cleaned, with no event told yet.
func NewTelling(window func(key string) string) *Telling {
	return &Telling{window: window, pressure: make(map[string]time.Time)}
}

// Line is the event e's line, its times in now's time zone, as the Log's
// Events has it.
func (t *Telling) Line(e status.Event, now time.Time) EventLine {
	at := e.At.In(now.Location())
	if e.Kind == status.EventPressure && !e.Until.IsZero() {
		if t.pressure == nil {
			t.pressure = make(map[string]time.Time)
		}
		t.pressure[status.Clean(e.Account)] = e.Until
	}
	kind, ok := kindWords[e.Kind]
	if !ok {
		kind = status.Clean(e.Kind)
	}
	line := EventLine{Kind: kind}
	if tell, ok := tellers[e.Kind]; ok {
		line.Account, line.What = tell(t, e, at)
	} else {
		line.Account, line.What = account(e.Account), clause(status.Clean(e.Reason))
	}
	return line
}

// tellers tell an event of each kind the Log knows: its account, and what
// happened, its times shown beside at, when it happened.
var tellers = map[string]func(t *Telling, e status.Event, at time.Time) (account, what status.Words){
	status.EventStarted:  (*Telling).started,
	status.EventMoved:    (*Telling).moved,
	status.EventPressure: (*Telling).pressured,
	status.EventCap:      (*Telling).capped,
	status.EventLimit:    (*Telling).limited,
	status.EventRefused:  (*Telling).refused,
	status.EventPrimed:   (*Telling).primed,
	status.EventRoom:     (*Telling).opened,
	status.EventPin:      (*Telling).pinned,
	status.EventAuto:     (*Telling).unpinned,
	status.EventRestart:  (*Telling).restarting,
	status.EventHealth:   (*Telling).turned,
}

// started tells of a session starting: the account it started on, and
// nothing more.
func (*Telling) started(e status.Event, _ time.Time) (status.Words, status.Words) {
	return account(e.Account), nil
}

// moved tells of a session's move, from and to, as "work → side", and why,
// as status.EventWords says: a choice that passed an account over under
// pressure with when that account's latest pressure event said it runs out,
// where the move came before then.
func (t *Telling) moved(e status.Event, at time.Time) (status.Words, status.Words) {
	moves := status.Words{named(e.From), said(" → "), named(e.To)}
	runsOut := t.pressure[status.PassedOver(e.Reason)]
	if !e.At.Before(runsOut) {
		runsOut = time.Time{}
	}
	facts := status.Facts{From: e.From, To: e.To, Until: runsOut, Now: at}
	return moves, status.EventWords(e.Reason, facts)
}

// pressured tells of an account coming under pressure: "its 5-hour to run
// out at 16:05".
func (t *Telling) pressured(e status.Event, at time.Time) (status.Words, status.Words) {
	what := "under pressure"
	if !e.Until.IsZero() {
		what = "its " + t.windowOf(first(e.Windows)) + " to run out at " + status.When(at, e.Until)
	}
	return account(e.Account), clause(what)
}

// capped tells of an account reaching its cap: "95% of its 5-hour window",
// and the sessions it moved, as they move.
func (t *Telling) capped(e status.Event, at time.Time) (status.Words, status.Words) {
	what := t.its(e.Windows, "its window")
	if e.Reserve > 0 {
		what = status.Percent(1-e.Reserve) + " of " + what
	}
	return account(e.Account), append(clause(what), moving(e, true)...)
}

// limited tells of an account reaching its limit: "its 5-hour window, till
// 23:58", and the sessions it moved.
func (t *Telling) limited(e status.Event, at time.Time) (status.Words, status.Words) {
	what := t.its(e.Windows, "its limit") + till(e.Until, at)
	return account(e.Account), append(clause(what), moving(e, false)...)
}

// refused tells of the upstream refusing an account's requests: "its token
// refused (401), till 14:00", or of a model family's alone, "its opus
// requests refused (403), till 14:00", and the sessions it moved.
func (t *Telling) refused(e status.Event, at time.Time) (status.Words, status.Words) {
	what := "its token refused"
	if e.Family != "" {
		what = "its " + status.Clean(e.Family) + " requests refused"
	}
	if e.Status != 0 {
		what += " (" + strconv.Itoa(e.Status) + ")"
	}
	return account(e.Account), append(clause(what+till(e.Until, at)), moving(e, false)...)
}

// primed tells of a prime starting an account's window: "its 5-hour window
// started, resets at 12:05".
func (t *Telling) primed(e status.Event, at time.Time) (status.Words, status.Words) {
	what := t.its(e.Windows, "its window") + " started"
	if !e.Until.IsZero() {
		what += ", resets at " + status.When(at, e.Until)
	}
	return account(e.Account), clause(what)
}

// opened tells of an account with room again: "its 5-hour window reset",
// or nothing more where it names no window.
func (t *Telling) opened(e status.Event, _ time.Time) (status.Words, status.Words) {
	if len(e.Windows) == 0 {
		return account(e.Account), nil
	}
	return account(e.Account), clause(t.its(e.Windows, "") + " reset")
}

// pinned tells of routing set by hand: the global pin's accounts, "new
// sessions go there", or with its move, "new and running sessions go
// there", with force, every session's own pin cleared too; or a session's
// own pin's account, "this session goes there"; and who set it, as by says.
func (*Telling) pinned(e status.Event, _ time.Time) (status.Words, status.Words) {
	if e.Session != "" {
		return account(e.Account), clause("this session goes there" + by[e.By])
	}
	what := "new sessions go there"
	if e.Move {
		what = "new and running sessions go there"
	}
	return pinAccounts(e), clause(what + forced(e) + by[e.By])
}

// unpinned tells of routing given back to the router: "new sessions back
// from personal to the router's choice", or of a session's own pin, "this
// session back from side to the router's choice"; the global pin's with
// force, every session's own pin cleared too; and who did it, as by says.
// It names no account in the account column.
func (*Telling) unpinned(e status.Event, _ time.Time) (status.Words, status.Words) {
	who, from := "new sessions back", e.Accounts
	if e.Session != "" {
		who, from = "this session back", []string{e.Account}
	}
	what := status.Words{said(who)}
	if names := cleaned(from); len(names) > 0 {
		what = append(what, said(" from "))
		what = append(what, joined(names, " and ")...)
	}
	return nil, append(what, said(" to the router's choice"+forced(e)+by[e.By]))
}

// restarting tells of a restart falling due: "due: the config changed".
func (*Telling) restarting(e status.Event, _ time.Time) (status.Words, status.Words) {
	reason := status.Clean(e.Reason)
	if words, ok := restartWords[reason]; ok {
		reason = words
	}
	if reason == "" {
		return nil, clause("due")
	}
	return nil, clause("due: " + reason)
}

// turned tells of the router's health turning: "unhealthy: <its reason>",
// or "healthy again".
func (*Telling) turned(e status.Event, _ time.Time) (status.Words, status.Words) {
	if reason := status.Clean(e.Reason); reason != "" {
		return nil, clause("unhealthy: " + reason)
	}
	return nil, clause("healthy again")
}

// moving tells of the sessions a limit, a cap or a refusal moved, after a
// colon: "its 3 sessions move to side", "its session moves to side", or
// where they went to several accounts, "its 3 sessions move to other
// accounts"; a cap's, as their next requests come. It's nothing where it
// moved none.
func moving(e status.Event, asTheyCome bool) status.Words {
	if e.Count == 0 {
		return nil
	}
	subject, where, as := ": its "+status.SessionCount(e.Count)+" move to ", "other accounts", " as their next requests come"
	if e.Count == 1 {
		subject, where, as = ": its session moves to ", "another account", " as its next request comes"
	}
	w := status.Words{said(subject)}
	if e.To != "" {
		w = append(w, named(e.To))
	} else {
		w = append(w, said(where))
	}
	if asTheyCome {
		w = append(w, said(as))
	}
	return w
}

// its names the windows with the given keys as the account's: "its 5-hour
// window", or "its 5-hour and week windows"; none where there are no keys.
func (t *Telling) its(keys []string, none string) string {
	if len(keys) == 0 {
		return none
	}
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = t.windowOf(key)
	}
	if len(names) == 1 {
		return "its " + names[0] + " window"
	}
	return "its " + prose.List(names) + " windows"
}

// windowOf names the window with the given key, as "5-hour": "window" where
// there's none.
func (t *Telling) windowOf(key string) string {
	switch {
	case key == "":
		return "window"
	case t.window == nil:
		return status.Clean(key)
	}
	return t.window(key)
}

// till says until when what's told holds, as ", till 23:58", shown beside
// at: nothing where it doesn't say.
func till(until, at time.Time) string {
	if until.IsZero() {
		return ""
	}
	return ", till " + status.When(at, until)
}

// pinAccounts are the global pin's accounts, joined with ", ": its first,
// as Account gives it, where it lists none.
func pinAccounts(e status.Event) status.Words {
	names := cleaned(e.Accounts)
	if len(names) == 0 {
		return account(e.Account)
	}
	return joined(names, ", ")
}

// joined are the accounts named, ", " between them, but last before the
// last, as " and " runs them together as English does.
func joined(names []string, last string) status.Words {
	var w status.Words
	for i, name := range names {
		switch {
		case i == 0:
		case i == len(names)-1:
			w = append(w, said(last))
		default:
			w = append(w, said(", "))
		}
		w = append(w, named(name))
	}
	return w
}

// cleaned are the ids given, cleaned, but those that clean to nothing.
func cleaned(ids []string) []string {
	var names []string
	for _, id := range ids {
		if name := status.Clean(id); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// account is the account with the given id, as the account column names
// it: none for "".
func account(id string) status.Words {
	if status.Clean(id) == "" {
		return nil
	}
	return status.Words{named(id)}
}

// clause is words around no account: none for "".
func clause(text string) status.Words {
	if text == "" {
		return nil
	}
	return status.Words{said(text)}
}

// said is words around the accounts named.
func said(text string) status.Part {
	return status.Part{Text: text}
}

// named is the account with the given id, cleaned, as it's from elsewhere.
func named(id string) status.Part {
	return status.Part{Text: status.Clean(id), Kind: status.PartAccount}
}

// first is the first of keys, or "" for none.
func first(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}
