package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// AccountLately are the events of the document m is a moment of, as Detail's
// LATELY tells them of the account with the given id at m's Now, the newest
// first, n at most, folded as Lately folds them: told from its side, which
// they leave unnamed, as "reached its cap: 3 sessions moved to side". Of an
// account they're the events it befell, the sessions a cap, a limit, a
// refusal or a hold of another moved onto it, and any other move onto it,
// the pins naming it, and the sessions that started on it. Of every
// account, id "", they're every account's, each naming its account, as
// Lately tells them.
// Restarts, the router's health, and routing given back to it name no
// account, so neither tells them. Each tells a session that started on the
// router's best, as its reason says, ", the best", as it started, wherever ▲
// marks since. Each is told in full alone, its Short its Full, as stacked
// Detail tells LATELY in full too. Each names a session as Lately does.
func AccountLately(m status.Moment, id string, n int, dirs SessionDirs) []Sentence {
	return toldFrom(m, given(m), n, sideOf(id), dirs)
}

// AccountLatelyTo are the router's events held, oldest first, as
// ReadLatelyTo reads them, as Detail's LATELY tells them of the account
// with the given id, or every account's for "", at the end of a past
// period's last day, until, the newest first, n at most: those AccountLately
// would tell of them had it been until then, of the week before it alone,
// as latelyFrom has it, each as it stood then, as stoodAt puts it back,
// their facts worked out over those before it alone, so a hold lifted
// after it, or a cap whose moves came after it, is told as it stood. Each
// is told in AccountLately's words at m's Now, as LATELY at now tells them.
func AccountLatelyTo(m status.Moment, held []Event, id string, until time.Time, n int, dirs SessionDirs) []Sentence {
	since, of := latelyFrom(until), sideOf(id)
	within := side{tells: func(e status.Event) bool { return !e.At.Before(since) && of.tells(e) }, tell: of.tell}
	var before []Event
	for _, e := range held {
		if e.Event.At.Before(until) {
			before = append(before, e)
		}
	}
	newest := stoodAt(before, until)
	slices.Reverse(newest)
	return toldFrom(m, newest, n, within, dirs)
}

// ReadLatelyTo reads, through filed, the router's files' events Detail's
// LATELY tells at the end of a past period's last day, until, as
// AccountLatelyTo takes them, oldest first, each as it last stands: those
// of the week before until, as latelyFrom has it, and the FactsReach
// before that their facts are worked out over.
func ReadLatelyTo(filed Events, until time.Time) []Event {
	return readBetween(filed, latelyFrom(until).Add(-FactsReach), until).Events()
}

// sideOf is the side Detail's LATELY tells the account with the given id
// from: every account's for "".
func sideOf(id string) side {
	if id == "" {
		return everyAccount
	}
	return accountSide{id: id}.side()
}

// unnamed are the kinds of events that name no account.
var unnamed = []string{status.EventAuto, status.EventRestart, status.EventHealth}

// everyAccount is every account's side: the Overview's, in full, but the
// events that name no account, a session that started on the router's best
// said to have.
var everyAccount = side{
	tells: func(e status.Event) bool { return overview.tells(e) && !slices.Contains(unnamed, e.Kind) },
	tell: func(t latelyTeller, e status.Event, f eventFact) (string, Said, Said) {
		kind, full, _ := overview.tell(t, e, f)
		if e.Kind == status.EventStarted {
			full.Rest = t.startedWhy(e, f, true)
		}
		return kind, full, full
	},
}

// accountSide is the side of the account with the given id.
type accountSide struct {
	id string
}

// side is the account's side, as AccountLately tells it.
func (s accountSide) side() side {
	return side{tells: s.tells, tell: s.tell}
}

// tells reports whether e is on the account's side: it befell the account,
// moved a session onto it, or pinned it.
func (s accountSide) tells(e status.Event) bool {
	switch e.Kind {
	case status.EventStarted, status.EventPressure, status.EventForeign, status.EventPrimed, status.EventRoom:
		return e.Account == s.id
	case status.EventCap, status.EventLimit, status.EventRefused, status.EventHeld:
		return e.Account == s.id || s.cameFrom(e)
	case status.EventMoved:
		return e.To == s.id
	case status.EventPin:
		return s.pinned(e)
	}
	return false
}

// cameFrom reports whether e, a cap, a limit, a refusal or a hold of another
// account, moved sessions onto the account, each of them.
func (s accountSide) cameFrom(e status.Event) bool {
	return e.Account != s.id && e.To == s.id && e.Count > 0
}

// pinned reports whether e, a pin, names the account: the global pin among
// its accounts, or a session's own pin as its account.
func (s accountSide) pinned(e status.Event) bool {
	if e.Session != "" {
		return e.Account == s.id
	}
	return slices.Contains(pinnedNames(e), s.id)
}

// tell tells e from the account's side, in full, as told tells it.
func (s accountSide) tell(t latelyTeller, e status.Event, f eventFact) (string, Said, Said) {
	kind, said := s.told(t, e, f)
	return kind, said, said
}

// told tells e from the account's side, as it tells it of its kind, with the
// kind its mark is drawn for.
func (s accountSide) told(t latelyTeller, e status.Event, f eventFact) (string, Said) {
	switch e.Kind {
	case status.EventStarted:
		return e.Kind, Said{Subject: t.session(e), Words: clause(" started here"), Rest: t.startedWhy(e, f, true)}
	case status.EventMoved:
		return e.Kind, t.cameHere(e, f)
	case status.EventPressure:
		return e.Kind, t.pressedHere(e)
	case status.EventCap, status.EventLimit, status.EventRefused, status.EventHeld:
		if s.cameFrom(e) {
			return status.EventMoved, cameFromHeld(e, f, t.now)
		}
		full, _ := befell(e, f, t.now)
		return e.Kind, heldBack(nil, e, f, full, false)
	case status.EventPin:
		return e.Kind, s.pinnedHere(t, e)
	case status.EventForeign:
		return e.Kind, Said{Words: clause(prose.ForeignToken)}
	}
	full, _ := befell(e, f, t.now)
	return e.Kind, Said{Words: clause(full)}
}

// cameHere tells of a session's model moving onto the account: "api came
// from personal", and why, in the views' words, as in ": personal came under
// pressure".
func (t latelyTeller) cameHere(e status.Event, f eventFact) Said {
	said := Said{Subject: t.session(e), Words: clause(" came from "), Object: account(e.From)}
	if why := status.EventWords(e.Reason, t.facts(f, e.From, e.To)); why.String() != "" {
		said.Rest = slices.Concat(clause(": "), why)
	}
	return said
}

// pressedHere tells of the account coming under pressure, and when its
// window runs out at its pace, where the event says: "came under pressure:
// out ~16:05 at this pace".
func (t latelyTeller) pressedHere(e status.Event) Said {
	if e.Until.IsZero() {
		return Said{Words: clause("came under pressure")}
	}
	return Said{Words: clause("came under pressure: out "), Object: clause("~" + prose.Befell(t.now, e.Until)), Rest: clause(" at this pace")}
}

// cameFromHeld tells of the sessions another account's cap, limit, refusal
// or hold moved onto the account: "3 sessions came from work: work reached
// its cap".
func cameFromHeld(e status.Event, f eventFact, now time.Time) Said {
	what, _ := befell(e, f, now)
	return Said{Subject: clause(prose.SessionCount(f.moved)), Words: clause(" came from "), Object: account(e.Account),
		Rest: slices.Concat(clause(": "), account(e.Account), clause(" "+what))}
}

// pinnedHere tells of routing set by hand to the account: the global pin,
// "pinned here, by hand", or with others, "pinned with side, by hand"; or a
// session's own pin, "api pinned here, by hand", the session named as t names it.
func (s accountSide) pinnedHere(t latelyTeller, e status.Event) Said {
	if e.Session != "" {
		return Said{Subject: t.session(e), Words: clause(" pinned here, by hand")}
	}
	others := slices.DeleteFunc(pinnedNames(e), func(name string) bool { return name == s.id })
	if len(others) == 0 {
		return Said{Words: clause("pinned here, by hand")}
	}
	return Said{Words: clause("pinned with "), Object: joined(others, prose.And), Rest: clause(", by hand")}
}
