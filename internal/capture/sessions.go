package capture

import (
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
)

// assigned is the account a session's requests of the model go to, as the
// router assigns it, in the directory given, since when and why, last seen
// when given, doing what its request in flight does.
func assigned(model, account, dir, reason string, since, seen time.Time, inFlight string) status.Assignment {
	return status.Assignment{
		Model: model, Family: claude.Provider{}.Family(model), Account: account, Dir: dir, InFlight: inFlight,
		Reason: reason, AssignedAt: since.UTC(), LastSeen: seen.UTC(),
	}
}

// routedSessions are the sessions the router lists at now, Thursday's 14:42:07,
// as GET /sessions gives them, the one seen last first: the Overview's six
// (dash3.SESSIONS), on personal 41e0, sonnet, answering, and 9c2e, opus, idle
// 9m; on side c61b, opus, answering, started at 14:41, and, each moved from
// work as it reached its cap at 14:31, d28c, opus, asking, its haiku going to
// side too, 7f3a, sonnet, idle 1m, and db8a, opus, idle 4m; each in its
// directory (log1.DIRS). Those with a request in flight were last seen as
// it was sent, as the stream tells of it.
func routedSessions(now time.Time) []status.Session {
	at := func(hour, minute, second int) time.Time { return clockAt(now, 0, hour, minute, second) }
	return []status.Session{
		{ID: idD28C, Assignments: []status.Assignment{
			assigned(opus, "side", "~/Code/switchboard", reasonCapped, at(14, 31, 13), ago(now, sentD28C), status.Asking),
			assigned(haikuShort, "side", "~/Code/switchboard", status.ReasonNew, at(14, 31, 44), at(14, 31, 44), ""),
		}},
		{ID: idC61B, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/mobile", status.ReasonNew, at(14, 41, 11), ago(now, sentC61B), status.Answering)}},
		{ID: id41E0, Assignments: []status.Assignment{assigned(sonnet, "personal", "~/Code/docs", status.ReasonNew, at(12, 47, 12), ago(now, sent41E0), status.Answering)}},
		{ID: id7F3A, Assignments: []status.Assignment{assigned(sonnet, "side", "~/Code/api", reasonCapped, at(14, 31, 20), ago(now, time.Minute), "")}},
		{ID: idDB8A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/infra", reasonCapped, at(14, 31, 40), ago(now, 4*time.Minute), "")}},
		{ID: id9C2E, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/website", status.ReasonNew, at(11, 55, 31), at(14, 33, 7), "")}},
	}
}

// routedPastPersonalsLimit are the sessions the router lists at now in
// routing's world of a limit: routedSessions, but for personal's, 41e0 and
// 9c2e, moved to side as personal reached its limit at 14:12.
func routedPastPersonalsLimit(now time.Time) []status.Session {
	limited := clockAt(now, 0, 14, 12, 0)
	sessions := routedSessions(now)
	for _, s := range sessions {
		for i, a := range s.Assignments {
			if a.Account == "personal" {
				s.Assignments[i] = assigned(a.Model, "side", a.Dir, reasonLimited, limited, a.LastSeen, a.InFlight)
			}
		}
	}
	return sessions
}

// listedSessions are the sessions the router lists at now, Thursday's
// 14:42:07, as Sessions' List and the Log draw them (log1.tail), each its
// own conversation's model, opus, seen last as its latest request came: on
// personal 41e0, idle since its answer of 14:41:29, and 9c2e, idle; on side
// c61b, answering, d28c, asking, 7f3a, answering, and db8a, idle since its
// answer of 14:40:59, the last three moved from work as it reached its cap.
func listedSessions(now time.Time) []status.Session {
	at := func(hour, minute, second int) time.Time { return clockAt(now, 0, hour, minute, second) }
	return []status.Session{
		{ID: idD28C, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/switchboard", reasonCapped, at(14, 31, 13), at(14, 42, 0), status.Asking)}},
		{ID: id7F3A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/api", reasonCapped, at(14, 31, 20), at(14, 42, 1), status.Answering)}},
		{ID: idC61B, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/mobile", status.ReasonNew, at(14, 41, 11), at(14, 41, 53), status.Answering)}},
		{ID: id41E0, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/docs", status.ReasonNew, at(12, 47, 12), at(14, 41, 29), "")}},
		{ID: idDB8A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/infra", reasonCapped, at(14, 31, 40), at(14, 40, 59), "")}},
		{ID: id9C2E, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/website", status.ReasonNew, at(11, 55, 31), at(14, 33, 7), "")}},
	}
}

// routedAtTheLimit are the sessions the router lists at 16:05:41, as GET
// /sessions gives them, where personal reaches its 5-hour limit as 41e0 asks
// (SESSIONS-RUNWAY-SPEC.md, "16:05:41 and 16:05:42"): on personal, 41e0, its
// request refused, and 9c2e, idle since 15:52:30; on side, c61b answering
// since 16:05:30, and, moved from work, d28c, idle since 16:05:36, 7f3a,
// since 16:03:12, and db8a, since 16:04:55. Each is opus, as the Log's
// sample has them.
func routedAtTheLimit(now time.Time) []status.Session {
	at := func(hour, minute, second int) time.Time { return clockAt(now, 0, hour, minute, second) }
	return []status.Session{
		{ID: id41E0, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/docs", status.ReasonNew, at(12, 47, 12), now, status.Asking)}},
		{ID: idC61B, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/mobile", status.ReasonNew, at(14, 41, 11), now, status.Answering)}},
		{ID: idD28C, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/switchboard", reasonCapped, at(14, 31, 13), at(16, 5, 36), "")}},
		{ID: idDB8A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/infra", reasonCapped, at(14, 31, 40), at(16, 4, 55), "")}},
		{ID: id7F3A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/api", reasonCapped, at(14, 31, 20), at(16, 3, 12), "")}},
		{ID: id9C2E, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/website", status.ReasonNew, at(11, 55, 31), at(15, 52, 30), "")}},
	}
}

// routedAfterTheLimit are the sessions the router lists a second on, at
// 16:05:42: 41e0's request moved to side, asking there, at the foot of side's
// sessions; 9c2e still on personal, to move as its next request comes; the
// rest as they were.
func routedAfterTheLimit(now time.Time) []status.Session {
	sessions := routedAtTheLimit(now.Add(-time.Second))
	moved := assigned(opus, "side", "~/Code/docs", reasonLimited, now, now, status.Asking)
	sessions[0] = status.Session{ID: id41E0, Assignments: []status.Assignment{moved}}
	sessions[1].Assignments[0].LastSeen = now.UTC()
	return sessions
}

// resumedRouted is f41a as the router lists it in its page's world: on
// personal since 09:12, rescored there after 15 hours idle, an answer
// streaming to it now.
func resumedRouted(now time.Time) status.Session {
	since := clockAt(now, 0, 9, 12, 0)
	return status.Session{ID: idF41A, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/api", status.Rescored(15*time.Hour), since, now, status.Answering)}}
}
