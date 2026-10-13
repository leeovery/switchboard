package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// AccountLately are the events of the document m is a moment of, as Detail's
// LATELY tells them of the account with the given id at m's Now, the newest
// first, n at most, folded as Lately folds them: told from its side, which
// they leave unnamed, as "reached its cap: 3 sessions moved to side". Of an
// account they're the events it befell, the sessions a cap, a limit or a
// refusal of another moved onto it, and any other move onto it, the pins
// naming it, and the sessions that started on it. Of every account, id "",
// they're every account's, each naming its account, as Lately tells them.
// Restarts, the router's health, and routing given back to it name no
// account, so neither tells them. Each tells a session starting on the
// account new sessions go to, which ▲ marks, ", the best".
func AccountLately(m status.Moment, id string, n int) []Sentence {
	if id == "" {
		return toldFrom(m, n, everyAccount(m.Next))
	}
	return toldFrom(m, n, accountSide{id: id, next: m.Next}.side())
}

// unnamed are the kinds of events that name no account.
var unnamed = []string{status.EventAuto, status.EventRestart, status.EventHealth}

// everyAccount is every account's side, the account new sessions go to the
// one with the given id: the Overview's, but the events that name no
// account, a session starting there said to have started on the best.
func everyAccount(next string) side {
	return side{
		tells: func(e status.Event) bool { return overview.tells(e) && !slices.Contains(unnamed, e.Kind) },
		tell: func(t latelyTeller, e status.Event, f eventFact) (string, Said, Said) {
			kind, full, short := overview.tell(t, e, f)
			if e.Kind == status.EventStarted {
				full.Rest = t.startedWhy(e, f, e.Account == next)
				short.Rest = full.Rest
			}
			return kind, full, short
		},
	}
}

// accountSide is the side of the account with the given id, new sessions
// going to the one next names.
type accountSide struct {
	id, next string
}

// side is the account's side, as AccountLately tells it.
func (s accountSide) side() side {
	return side{tells: s.tells, tell: s.tell}
}

// tells reports whether e is on the account's side: it befell the account,
// moved a session onto it, or pinned it.
func (s accountSide) tells(e status.Event) bool {
	switch e.Kind {
	case status.EventStarted, status.EventPressure, status.EventPrimed, status.EventRoom:
		return e.Account == s.id
	case status.EventCap, status.EventLimit, status.EventRefused:
		return e.Account == s.id || s.cameFrom(e)
	case status.EventMoved:
		return e.To == s.id
	case status.EventPin:
		return s.pinned(e)
	}
	return false
}

// cameFrom reports whether e, a cap, a limit or a refusal of another
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

// tell tells e from the account's side, as it tells it of its kind.
func (s accountSide) tell(t latelyTeller, e status.Event, f eventFact) (string, Said, Said) {
	switch e.Kind {
	case status.EventStarted:
		said := Said{Subject: session(e.Session), Words: clause(" started here"), Rest: t.startedWhy(e, f, s.id == s.next)}
		return e.Kind, said, said
	case status.EventMoved:
		said := t.cameHere(e, f)
		return e.Kind, said, said
	case status.EventPressure:
		full, short := t.pressedHere(e)
		return e.Kind, full, short
	case status.EventCap, status.EventLimit, status.EventRefused:
		if s.cameFrom(e) {
			full, short := cameFromHeld(e, f)
			return status.EventMoved, full, short
		}
		full, short := befell(e, f)
		return e.Kind, heldBack(nil, e, f, full, false), heldBack(nil, e, f, short, true)
	case status.EventPin:
		full, short := s.pinnedHere(e)
		return e.Kind, full, short
	}
	full, short := befell(e, f)
	return e.Kind, Said{Words: clause(full)}, Said{Words: clause(short)}
}

// cameHere tells of a session's model moving onto the account: "db8a came
// from personal", and why, in the views' words, as in ": personal came under
// pressure".
func (t latelyTeller) cameHere(e status.Event, f eventFact) Said {
	said := Said{Subject: session(e.Session), Words: clause(" came from "), Object: account(e.From)}
	if why := status.EventWords(e.Reason, t.facts(f, e.From, e.To)); len(why) > 0 {
		said.Rest = slices.Concat(clause(": "), why)
	}
	return said
}

// pressedHere tells of the account coming under pressure, and when its
// window runs out at its pace, where the event says: "came under pressure:
// out ~16:05 at this pace", or short, "under pressure: out ~16:05".
func (t latelyTeller) pressedHere(e status.Event) (Said, Said) {
	full, short := Said{Words: clause("came under pressure")}, Said{Words: clause("under pressure")}
	if e.Until.IsZero() {
		return full, short
	}
	out := clause("~" + prose.Dated(t.now, e.Until))
	full = Said{Words: clause("came under pressure: out "), Object: out, Rest: clause(" at this pace")}
	short = Said{Words: clause("under pressure: out "), Object: out}
	return full, short
}

// cameFromHeld tells of the sessions another account's cap, limit or
// refusal moved onto the account: "3 sessions came from work: work reached
// its cap", or short, "3 came from work: work at its cap".
func cameFromHeld(e status.Event, f eventFact) (Said, Said) {
	full, short := befell(e, f)
	from := func(count, what string) Said {
		return Said{Subject: clause(count), Words: clause(" came from "), Object: account(e.Account),
			Rest: slices.Concat(clause(": "), account(e.Account), clause(" "+what))}
	}
	return from(prose.SessionCount(f.moved), full), from(prose.Grouped(f.moved), short)
}

// pinnedHere tells of routing set by hand to the account: the global pin,
// "pinned here, by hand", or with others, "pinned with side, by hand"; or a
// session's own pin, "c61b pinned here, by hand". Short, it's without ", by
// hand".
func (s accountSide) pinnedHere(e status.Event) (Said, Said) {
	if e.Session != "" {
		return Said{Subject: session(e.Session), Words: clause(" pinned here, by hand")}, Said{Subject: session(e.Session), Words: clause(" pinned here")}
	}
	others := slices.DeleteFunc(pinnedNames(e), func(name string) bool { return name == s.id })
	if len(others) == 0 {
		return Said{Words: clause("pinned here, by hand")}, Said{Words: clause("pinned here")}
	}
	with := joined(others, prose.And)
	return Said{Words: clause("pinned with "), Object: with, Rest: clause(", by hand")}, Said{Words: clause("pinned with "), Object: with}
}
