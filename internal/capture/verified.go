package capture

import (
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// verifiedFrom is when the verified Ultra accounts of the worlds that have
// them stopped reporting their 5-hour windows: Friday 25 September's start,
// six days before the frames' moment.
func verifiedFrom(now time.Time) time.Time {
	return dayStart(now, today-6)
}

// refusalHolds is how long the router holds an account it was refused on
// without room, as its refusedFor does.
const refusalHolds = 10 * time.Minute

// unreported are the accounts whose answers stopped carrying their 5-hour
// windows, as a verified Ultra account's do, by id, each from when.
type unreported map[string]time.Time

// retired reports whether the account with the given id no longer reports
// its 5-hour window at at.
func (u unreported) retired(id string, at time.Time) bool {
	from, ok := u[id]
	return ok && !at.Before(from)
}

// history is h without each 5-hour reading of an account's taken once it no
// longer reported the window: those before stand, as history.
func (u unreported) history(h history) history {
	return slices.DeleteFunc(slices.Clone(h), func(r readings.Reading) bool {
		return r.Key == sessionKey && u.retired(r.Account, r.At)
	})
}

// forgotten is h, the readings of the account with the given id to at,
// without any of its 5-hour window where it no longer reports it by then, as
// the router forgets a window it retires.
func (u unreported) forgotten(id string, h history, at time.Time) history {
	if !u.retired(id, at) {
		return h
	}
	return slices.DeleteFunc(slices.Clone(h), func(r readings.Reading) bool { return r.Key == sessionKey })
}

// events are the events told, oldest first, unnumbered, as the router tells
// them where u's accounts stopped reporting their 5-hour windows, its lines
// written as lines retells them: an event of an account's 5-hour window once
// it no longer reports it, a prime, its room again or its pressure, untold;
// a cap or a limit on it a refusal instead, as refusedInstead has it; and
// every reason as reason retells it.
func (u unreported) events(told []status.Event, written []ledger.Line) []status.Event {
	var kept []status.Event
	for _, e := range told {
		e.Reason = u.reason(e.Reason, e.At)
		if !u.retired(e.Account, e.At) || !slices.Contains(e.Windows, sessionKey) {
			kept = append(kept, e)
			continue
		}
		if e.Kind == status.EventCap || e.Kind == status.EventLimit {
			kept = append(kept, refusedInstead(e, written))
		}
	}
	return kept
}

// refusedInstead is the cap or limit e on a 5-hour window told as the
// upstream refusing the account's token instead, as the first request of
// those written at or after it that was refused there met it, or at e where
// none did, holding the account as long as the router does, the sessions it
// moved the same.
func refusedInstead(e status.Event, written []ledger.Line) status.Event {
	at := e.At
	refused := ledger.Tried{Account: e.Account, Why: status.WhyRefused}
	if i := slices.IndexFunc(written, func(l ledger.Line) bool { return !l.At.Before(e.At) && slices.Contains(l.Tried, refused) }); i >= 0 {
		at = written[i].At
	}
	return status.Event{
		Kind: status.EventRefused, At: at, Account: e.Account, Until: at.Add(refusalHolds), Status: http.StatusUnauthorized,
		Count: e.Count, To: e.To,
	}
}

// reason is the router's reason given, at at, but one moving a session off
// an account of u's at its reserve or its limit once it no longer reports its
// 5-hour window, which every cap and limit the worlds tell of is on, moving
// it off refused instead.
func (u unreported) reason(reason string, at time.Time) string {
	off, ok := strings.CutPrefix(reason, status.ReasonMovedOff)
	if !ok {
		return reason
	}
	for id := range u {
		if !u.retired(id, at) {
			continue
		}
		if why, ok := strings.CutPrefix(off, id+" "); ok && (why == status.WhyReserve || why == status.WhyLimit) {
			return status.ReasonMovedOff + id + " " + status.WhyRefused
		}
	}
	return reason
}

// lines are the lines given, oldest first, as a router where u's accounts
// stopped reporting their 5-hour windows writes them: an answer's 5-hour
// headers gone where its account no longer reports the window; and each move
// off an account at its reserve or limit refused instead, as reason says,
// the first being the request the upstream refused there, tried there first,
// and those after it while the router holds the account sent on without
// trying it.
func (u unreported) lines(lines []ledger.Line) []ledger.Line {
	held := make(map[string]time.Time)
	for i, l := range lines {
		if u.retired(l.Account, l.At) {
			lines[i].Limits = withoutFiveHour(l.Limits)
		}
		if lines[i].Reason = u.reason(l.Reason, l.At); lines[i].Reason == l.Reason {
			continue
		}
		tried := slices.DeleteFunc(slices.Clone(l.Tried), func(t ledger.Tried) bool { return t.Account == l.From })
		if until, ok := held[l.From]; !ok || !l.At.Before(until) {
			tried, held[l.From] = slices.Insert(tried, 0, ledger.Tried{Account: l.From, Why: status.WhyRefused}), l.At.Add(refusalHolds)
		}
		lines[i].Tried, lines[i].Attempts = tried, len(tried)+1
	}
	return lines
}

// withoutFiveHour are an answer's anthropic-ratelimit-unified-* headers, as a
// line keeps them, without the 5-hour window's.
func withoutFiveHour(limits map[string]string) map[string]string {
	if limits == nil {
		return nil
	}
	kept := maps.Clone(limits)
	maps.DeleteFunc(kept, func(name, _ string) bool { return strings.HasPrefix(name, fiveHourKey+"-") })
	return kept
}

// routed are the sessions given as the router lists them where u's accounts
// stopped reporting their 5-hour windows: each assignment's reason as reason
// retells it, as of when it was assigned.
func (u unreported) routed(sessions []status.Session) []status.Session {
	for _, s := range sessions {
		for i, a := range s.Assignments {
			s.Assignments[i].Reason = u.reason(a.Reason, a.AssignedAt)
		}
	}
	return sessions
}

// verified is the auto world with the accounts given verified Ultra, on
// Ultra, from verifiedFrom: their answers carry no 5-hour window from then,
// so the router holds none of theirs, its records keep their 5-hour readings
// from before alone, and it tells of none of their 5-hour windows since, as
// unreported's events, lines and routed have it, its records telling its own
// events, as of the lines it writes. The rest keep their 5-hour windows, on
// Max.
func (a accountWorlds) verified(name string, ids ...string) accountWorld {
	return a.kept(name, func() accountWorld {
		u := make(unreported, len(ids))
		for _, id := range ids {
			u[id] = verifiedFrom(a.now)
		}
		w := a.autoOf(a.today().three(true))
		written := u.lines(sessionsLines(a.now))
		w.unreported, w.history = u, u.history(w.history)
		w.events, w.ownEvents = ofTheRun(u.events(happenings{a.now}.auto(), written)), true
		w.routed = func(now time.Time) []status.Session { return u.routed(routedSessions(now)) }
		w.lines = func(time.Time) []ledger.Line { return slices.Clone(written) }
		for i, acct := range w.accounts {
			w.accounts[i].Plan = config.PlanMax
			if _, ok := u[acct.ID]; ok {
				w.accounts[i].Plan = config.PlanUltra
			}
		}
		return w
	})
}

// ultra is the auto world with all three verified Ultra, as verified has it:
// work open, its week at 34%, its cap of 95% on its week alone; personal's
// week running out on Saturday at 06:00; side's at 78%, where new sessions
// go. Work's cap at 14:31:03 is its token refused instead, as d28c's request
// of 14:31:13 met it, till 14:41:13, moving its three sessions to side as the
// cap did.
func (a accountWorlds) ultra() accountWorld {
	return a.verified("ultra", three...)
}

// mixed is the ultra world but side, on Max, keeping its 5-hour window, as
// the auto world has it: primed at 13:50, at 13% now, resetting at 18:50.
func (a accountWorlds) mixed() accountWorld {
	return a.verified("mixed", "work", "personal")
}
