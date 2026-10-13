package views

import (
	"slices"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// EventLine is an event as the Log's Events and LATELY tell it, and events
// prints it: the kind's word, the account it befell and what happened, the
// accounts each names apart, as status.Words part them.
type EventLine struct {
	// Kind is the kind's word, as KindWord gives it.
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

// KindWord is the word the Log's Events, its panel and events name the kind
// of event given by: the router's own, cleaned, but "open" for its room,
// room again; a kind from a later router, as it's given, cleaned.
func KindWord(kind string) string {
	if word, ok := kindWords[kind]; ok {
		return word
	}
	return prose.Clean(kind)
}

// restartWords are why a restart fell due, in the Log's words.
var restartWords = map[string]string{
	status.RestartForConfig:  "the config changed",
	status.RestartForUpgrade: "switchboard upgraded",
	status.RestartForZone:    "the time zone changed",
}

// setFrom says where a pin was set or cleared from, by who did it, as an
// event's By gives it, in the Log's words.
var setFrom = map[string]string{"cli": "the command line", "dashboard": "the dashboard"}

// setBy says who set or cleared a pin, as an event's By gives it, after
// what was set: ", set from the command line"; nothing for anyone else, or
// no one.
func setBy(by string) string {
	if from, ok := setFrom[by]; ok {
		return ", set from " + from
	}
	return ""
}

// restartWhy is why a restart fell due, in the Log's words, as "the config
// changed": a reason it has no words for as the router gives it, cleaned.
func restartWhy(reason string) string {
	reason = prose.Clean(reason)
	if words, ok := restartWords[reason]; ok {
		return words
	}
	return reason
}

// forced says a global pin set or cleared with force cleared every
// session's own pin too: nothing of one without.
func forced(e status.Event) string {
	if !e.Force {
		return ""
	}
	return ", every session's own pin cleared too"
}

// Telling tells the router's events as their lines, oldest first, as they're
// read, working out their facts over those it tells as eventFacts do: so a
// move whose choice passed an account over under pressure says when that
// account's latest pressure event told said it runs out. Its zero value
// names a window as status.NameOfKey does. A Telling isn't safe for
// concurrent use.
type Telling struct {
	facts eventFacts
}

// NewTelling returns a Telling that names a window by its key as window
// does, as "5-hour" or "Week", with no event told yet.
func NewTelling(window func(key string) string) *Telling {
	return &Telling{facts: eventFacts{name: window}}
}

// Line is the event e's line, its times in now's time zone, as the Log's
// Events has it.
func (t *Telling) Line(e status.Event, now time.Time) EventLine {
	line, _ := t.tell(e, now)
	return line
}

// tell is the event e's line, as Line has it, and its facts, worked out
// over the events told before it.
func (t *Telling) tell(e status.Event, now time.Time) (EventLine, eventFact) {
	at := e.At.In(now.Location())
	fact := t.facts.take(e)
	line := EventLine{Kind: KindWord(e.Kind)}
	if tell, ok := tellers[e.Kind]; ok {
		line.Account, line.What = tell(e, fact, at)
	} else {
		line.Account, line.What = account(e.Account), clause(prose.Clean(e.Reason))
	}
	return line, fact
}

// wellAgain reports whether e tells of the router's health well again,
// which its mark is coloured apart for, LATELY's and the Log's alike.
func wellAgain(e status.Event) bool {
	return e.Kind == status.EventHealth && prose.Clean(e.Reason) == ""
}

// tellers tell an event of each kind the Log knows, its facts given: its
// account, and what happened, its times shown beside at, when it happened.
var tellers = map[string]func(e status.Event, f eventFact, at time.Time) (account, what status.Words){
	status.EventStarted:  loggedStart,
	status.EventMoved:    loggedMove,
	status.EventPressure: loggedPressure,
	status.EventCap:      loggedCap,
	status.EventLimit:    loggedLimit,
	status.EventRefused:  loggedRefusal,
	status.EventPrimed:   loggedPrime,
	status.EventRoom:     loggedRoom,
	status.EventPin:      loggedPin,
	status.EventAuto:     loggedAuto,
	status.EventRestart:  loggedRestart,
	status.EventHealth:   loggedHealth,
}

// loggedStart tells of a session starting: the account it started on, and
// nothing more.
func loggedStart(e status.Event, _ eventFact, _ time.Time) (status.Words, status.Words) {
	return account(e.Account), nil
}

// loggedMove tells of a session's move, from and to, as "work → side", and
// why, as status.EventWords says: a choice that passed an account over under
// pressure with when that account runs out, as its facts have it.
func loggedMove(e status.Event, f eventFact, at time.Time) (status.Words, status.Words) {
	moves := status.Words{status.Named(e.From), status.Plain(" → "), status.Named(e.To)}
	facts := status.Facts{From: e.From, To: e.To, Until: f.runsOut, Now: at}
	return moves, status.EventWords(e.Reason, facts)
}

// loggedPressure tells of an account coming under pressure: "its 5-hour to
// run out at 16:05".
func loggedPressure(e status.Event, f eventFact, at time.Time) (status.Words, status.Words) {
	what := "under pressure"
	if !e.Until.IsZero() {
		what = "its " + firstOr(f.windows, "window") + " to run out at " + prose.Dated(at, e.Until)
	}
	return account(e.Account), clause(what)
}

// loggedCap tells of an account reaching its cap: "95% of its 5-hour
// window", the use a window's, and the sessions it moved, as they move.
func loggedCap(e status.Event, f eventFact, _ time.Time) (status.Words, status.Words) {
	what := clause(its(f.windows, "its window"))
	if e.Reserve > 0 {
		what = status.Words{status.Use(prose.Percent(capUse(e))), status.Plain(" of " + what.String())}
	}
	return account(e.Account), append(what, moving(f, true)...)
}

// capUse is how much of its window an account had used as it reached its
// cap, the event e: all but its reserve.
func capUse(e status.Event) float64 {
	return 1 - e.Reserve
}

// loggedLimit tells of an account reaching its limit: "its 5-hour window,
// till 23:58", and the sessions it moved.
func loggedLimit(e status.Event, f eventFact, at time.Time) (status.Words, status.Words) {
	what := its(f.windows, "its limit") + till(e.Until, at)
	return account(e.Account), append(clause(what), moving(f, false)...)
}

// loggedRefusal tells of the upstream refusing an account's requests: "its
// token refused (401), till 14:00", or of a model family's alone, "its opus
// requests refused (403), till 14:00", and the sessions it moved.
func loggedRefusal(e status.Event, f eventFact, at time.Time) (status.Words, status.Words) {
	what := "its token refused"
	if e.Family != "" {
		what = "its " + prose.Clean(e.Family) + " requests refused"
	}
	if e.Status != 0 {
		what += " (" + strconv.Itoa(e.Status) + ")"
	}
	return account(e.Account), append(clause(what+till(e.Until, at)), moving(f, false)...)
}

// loggedPrime tells of a prime starting an account's window: "its 5-hour
// window started, resets at 12:05".
func loggedPrime(e status.Event, f eventFact, at time.Time) (status.Words, status.Words) {
	what := its(f.windows, "its window") + " started"
	if !e.Until.IsZero() {
		what += ", resets at " + prose.Dated(at, e.Until)
	}
	return account(e.Account), clause(what)
}

// loggedRoom tells of an account with room again: "its 5-hour window
// reset", or nothing more where it names no window.
func loggedRoom(e status.Event, f eventFact, _ time.Time) (status.Words, status.Words) {
	if len(f.windows) == 0 {
		return account(e.Account), nil
	}
	return account(e.Account), clause(its(f.windows, "") + " reset")
}

// loggedPin tells of routing set by hand: the global pin's accounts, "new
// sessions go there", or with its move, "new and running sessions go
// there", with force, every session's own pin cleared too; or a session's
// own pin's account, "this session goes there"; and who set it, as setBy says.
func loggedPin(e status.Event, _ eventFact, _ time.Time) (status.Words, status.Words) {
	if e.Session != "" {
		return account(e.Account), clause("this session goes there" + setBy(e.By))
	}
	what := "new sessions go there"
	if e.Move {
		what = "new and running sessions go there"
	}
	return pinAccounts(e), clause(what + forced(e) + setBy(e.By))
}

// loggedAuto tells of routing given back to the router: "new sessions back
// from personal to the router's choice", or of a session's own pin, "this
// session back from side to the router's choice"; the global pin's with
// force, every session's own pin cleared too; and who did it, as setBy says.
// It names no account in the account column.
func loggedAuto(e status.Event, _ eventFact, _ time.Time) (status.Words, status.Words) {
	who, from := "new sessions back", e.Accounts
	if e.Session != "" {
		who, from = "this session back", []string{e.Account}
	}
	what := status.Words{status.Plain(who)}
	if names := cleaned(from); len(names) > 0 {
		what = append(what, status.Plain(" from "))
		what = append(what, joined(names, prose.And)...)
	}
	return nil, append(what, status.Plain(" to the router's choice"+forced(e)+setBy(e.By)))
}

// loggedRestart tells of a restart falling due: "due: the config changed".
func loggedRestart(e status.Event, _ eventFact, _ time.Time) (status.Words, status.Words) {
	reason := restartWhy(e.Reason)
	if reason == "" {
		return nil, clause("due")
	}
	return nil, clause("due: " + reason)
}

// loggedHealth tells of the router's health turning: "unhealthy: <its
// reason>", or "healthy again".
func loggedHealth(e status.Event, _ eventFact, _ time.Time) (status.Words, status.Words) {
	if reason := prose.Clean(e.Reason); reason != "" {
		return nil, clause("unhealthy: " + reason)
	}
	return nil, clause("healthy again")
}

// moving tells of the sessions a limit, a cap or a refusal moved, as its
// facts count them, after a colon: "its 3 sessions move to side", "its
// session moves to side", or where they went to several accounts, "its 3
// sessions move to other accounts"; a cap's, as their next requests come.
// It's nothing where it moved none. The count is a figure, which rises in
// place as its moves count.
func moving(f eventFact, asTheyCome bool) status.Words {
	if f.moved == 0 {
		return nil
	}
	w := status.Words{status.Plain(": its session moves to ")}
	where, as := "another account", " as its next request comes"
	if f.moved > 1 {
		w = slices.Concat(status.Words{status.Plain(": its ")}, CountedWords(f.moved, "session"), status.Words{status.Plain(" move to ")})
		where, as = "other accounts", " as their next requests come"
	}
	if f.to != "" {
		w = append(w, status.Named(f.to))
	} else {
		w = append(w, status.Plain(where))
	}
	if asTheyCome {
		w = append(w, status.Plain(as))
	}
	return w
}

// its names the windows given as the account's, as prose.ListOf names them:
// "its 5-hour window", or "its 5-hour and week windows"; none where none is
// given.
func its(names []string, none string) string {
	if len(names) == 0 {
		return none
	}
	return "its " + prose.ListOf(names, "window")
}

// till says until when what's told holds, as status.Till says it, as ",
// till 23:58", shown beside at: nothing where it doesn't say.
func till(until, at time.Time) string {
	if said := status.Till(at, until); said != "" {
		return ", " + said
	}
	return ""
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

// joined are the accounts named, run together as a list whose last is
// joined by last, as prose.Joiner joins them: ", " between them, but last
// before the last, as prose.And runs them together as English does.
func joined(names []string, last string) status.Words {
	var w status.Words
	for i, name := range names {
		if joiner := prose.Joiner(i, len(names), last); joiner != "" {
			w = append(w, status.Plain(joiner))
		}
		w = append(w, status.Named(name))
	}
	return w
}

// cleaned are the ids given, cleaned, but those that clean to nothing.
func cleaned(ids []string) []string {
	var names []string
	for _, id := range ids {
		if name := prose.Clean(id); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// account is the account with the given id, as the account column names
// it: none for "".
func account(id string) status.Words {
	if prose.Clean(id) == "" {
		return nil
	}
	return status.Words{status.Named(id)}
}

// clause is words around no account: none for "".
func clause(text string) status.Words {
	if text == "" {
		return nil
	}
	return status.Words{status.Plain(text)}
}

// firstOr is the first of names, or none where there are none.
func firstOr(names []string, none string) string {
	if len(names) == 0 {
		return none
	}
	return names[0]
}
