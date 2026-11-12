package capture

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// scene is the auto world as Sessions' frames draw it at 14:42:07: the
// router listing the sessions as sceneSessions has them, and its request
// stream telling of their requests in flight on side as sceneStream does.
func (a accountWorlds) scene() accountWorld {
	return a.kept("scene", func() accountWorld {
		w := a.auto()
		w.routed, w.stream = sceneSessions, sceneStream(a.now, "side")
		return w
	})
}

// sceneSessions are the sessions the router lists at now, Thursday's
// 14:42:07, as Sessions' frames draw them, each its own conversation's
// model, opus, in the frames' order: on side, c61b, d28c and 7f3a, each with
// a request in flight, seen as the stream last told of it, and db8a, idle
// 50s, the last three moved from work as it reached its cap; on personal,
// 41e0, idle 34s, and 9c2e, idle since 14:33:07.
func sceneSessions(now time.Time) []status.Session {
	at := func(hour, minute, second int) time.Time { return clockAt(now, 0, hour, minute, second) }
	return []status.Session{
		{ID: idC61B, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/mobile", status.ReasonNew, at(14, 41, 11), ago(now, heardC61B), status.Answering)}},
		{ID: idD28C, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/switchboard", reasonCapped, at(14, 31, 13), ago(now, askedD28C), status.Asking)}},
		{ID: id7F3A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/api", reasonCapped, at(14, 31, 20), ago(now, heard7F3A), status.Answering)}},
		{ID: idDB8A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/infra", reasonCapped, at(14, 31, 40), ago(now, 50*time.Second), "")}},
		{ID: id41E0, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/docs", status.ReasonNew, at(12, 47, 12), ago(now, 34*time.Second), "")}},
		{ID: id9C2E, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/website", status.ReasonNew, at(11, 55, 31), at(14, 33, 7), "")}},
	}
}

// How long before 14:42:07 the scene's stream last told of each request in
// flight: c61b's answer's progress, d28c's request going out, and 7f3a's
// answer's progress, so the stream saw them last in that order, as the
// frames list side's sessions. d28c's went out as its pulse's head, frozen,
// reaches the middle of its cord.
const (
	heardC61B = 100 * time.Millisecond
	askedD28C = 270 * time.Millisecond
	heard7F3A = 400 * time.Millisecond
)

// The scene's answers streaming back, each its first byte come a whole
// number of the shimmer's steps before 14:42:07, as many as leave the
// frozen shimmer lighting the cells whose place, plus 2, divides by 4: 70
// steps of c61b's, 78 of 7f3a's.
const (
	firstC61B = 70 * 80 * time.Millisecond
	first7F3A = 78 * 80 * time.Millisecond
)

// sceneStream is what the scene's request stream tells of at now, of the
// requests in flight on the account given, each of opus: c61b's and 7f3a's
// answers streaming back, each sent 400ms before its first byte came, its
// progress told as heard says; and d28c's request going out.
func sceneStream(now time.Time, account string) []router.StreamEvent {
	c61b := request{id: "00000501", session: idC61B, model: opus, account: account}
	d28c := request{id: "00000502", session: idD28C, model: opus, account: account}
	f73a := request{id: "00000503", session: id7F3A, model: opus, account: account}
	answering := func(r request, first, heard time.Duration, chars int) []router.StreamEvent {
		progress := r.told(router.StreamProgress, ago(now, heard))
		progress.Chars = chars
		return []router.StreamEvent{
			r.told(router.StreamSent, ago(now, first+400*time.Millisecond)), r.told(router.StreamFirst, ago(now, first)), progress,
		}
	}
	told := slices.Concat(
		answering(c61b, firstC61B, heardC61B, chars1200),
		answering(f73a, first7F3A, heard7F3A, chars860),
		[]router.StreamEvent{d28c.told(router.StreamSent, ago(now, askedD28C))},
	)
	slices.SortStableFunc(told, func(a, b router.StreamEvent) int { return a.At.Compare(b.At) })
	return told
}

// The sessions the frame of ten accounts adds, on client, lab, extra and
// studio.
const (
	id1F9A = "1f9a3b7c-5d2e-4f80-9a6b-c4d1e2f3a5b6"
	id6C2E = "6c2e8d4f-1a3b-4c5d-8e9f-0a1b2c3d4e5f"
	id0D4B = "0d4b7e2a-9c1f-4a3d-b5e6-7f8a9b0c1d2e"
	idB7E5 = "b7e51c3d-2f4a-4b6c-8d9e-0f1a2b3c4d5e"
	id93FA = "93fa4e6b-7c8d-4e9f-a0b1-c2d3e4f5a6b7"
	id4AE8 = "4ae80b1c-3d5e-4f6a-9b7c-8d9e0f1a2b3c"
	idC0D1 = "c0d1e2f3-4a5b-4c6d-8e7f-9a0b1c2d3e4f"
	idF3B1 = "f3b1a2c3-d4e5-4f6a-8b7c-9d0e1f2a3b4c"
)

// ten is the world of ten accounts as Sessions' frame of them draws it: the
// scene's sessions, and eight more, as tenSessions has them, its stream the
// scene's.
func (a accountWorlds) ten() accountWorld {
	return a.kept("ten", func() accountWorld {
		w := a.many(10)
		w.routed, w.stream = tenSessions, sceneStream(a.now, "side")
		return w
	})
}

// sixRunning is the world of six accounts as the Overview's frame of every
// section at 173 × 53 draws it: the sessions the world of ten lists on those
// six, as tenSessions has them, eleven, its stream the scene's.
func (a accountWorlds) sixRunning() accountWorld {
	return a.kept("six running", func() accountWorld {
		w := a.many(6)
		w.routed, w.stream = routedOn(w.accounts, tenSessions), sceneStream(a.now, "side")
		return w
	})
}

// routedOn are the sessions routed lists, but those assigned to none of the
// accounts given.
func routedOn(accounts config.Accounts, routed func(now time.Time) []status.Session) func(now time.Time) []status.Session {
	return func(now time.Time) []status.Session {
		return slices.DeleteFunc(routed(now), func(s status.Session) bool {
			return !slices.ContainsFunc(s.Assignments, func(as status.Assignment) bool {
				return slices.ContainsFunc(accounts, func(a config.Account) bool { return a.ID == as.Account })
			})
		})
	}
}

// tenSessions are the sessions the router lists in the world of ten at now,
// in the frame's order: the scene's; and on client 1f9a, 6c2e and 0d4b, on
// lab b7e5 and 93fa, on extra 4ae8, and on studio c0d1 and f3b1, each
// started there, and idle as long as the frame says.
func tenSessions(now time.Time) []status.Session {
	idle := func(id, account, dir string, d time.Duration) status.Session {
		seen := ago(now, d)
		return status.Session{ID: id, Assignments: []status.Assignment{assigned(opus, account, dir, status.ReasonNew, seen, seen, "")}}
	}
	return slices.Concat(sceneSessions(now), []status.Session{
		idle(id1F9A, "client", "~/Code/billing", 2*time.Minute),
		idle(id6C2E, "client", "~/Code/ios-app", 14*time.Minute),
		idle(id0D4B, "client", "~/Code/scraper", 41*time.Second),
		idle(idB7E5, "lab", "~/Code/notebooks", 6*time.Minute),
		idle(id93FA, "lab", "~/Code/evals", time.Minute),
		idle(id4AE8, "extra", "~/Code/blog", 22*time.Minute),
		idle(idC0D1, "studio", "~/Code/design", 3*time.Minute),
		idle(idF3B1, "studio", "~/Code/site-v2", 8*time.Second),
	})
}

// aloneOnWork is work alone as Sessions' frame of it draws it: one's world,
// the router listing the scene's sessions on work, as onWork has them, its
// stream telling of their requests in flight there.
func (a accountWorlds) aloneOnWork() accountWorld {
	return a.kept("aloneOnWork", func() accountWorld {
		w := a.one()
		w.routed, w.stream = onWork, sceneStream(a.now, "work")
		return w
	})
}

// onWork are the scene's sessions, but db8a, each on work, new there, in
// the frame's order: 41e0 and 9c2e, then c61b, d28c and 7f3a.
func onWork(now time.Time) []status.Session {
	scene := sceneSessions(now)
	sessions := slices.Concat(scene[4:], scene[:3])
	for _, s := range sessions {
		for i := range s.Assignments {
			s.Assignments[i].Account, s.Assignments[i].Reason = "work", status.ReasonNew
		}
	}
	return sessions
}

// pinnedAtTheLimit is the world of Sessions' pinned frame: routing's world
// of a limit, personal at its 5-hour limit since 14:12 till 15:54, its event
// counting the two sessions it moved, 41e0 and 9c2e; new sessions pinned to
// personal and side since 13:40, so work, at its cap, is out of the pool;
// and every session on side, the router listing them as pinnedSessions has
// them, its stream the scene's, its lines today as pinnedLines has them.
func (a accountWorlds) pinnedAtTheLimit() accountWorld {
	return a.kept("pinnedAtTheLimit", func() accountWorld {
		w, h := a.limit(), happenings{a.now}
		w.pin = status.Pin{Accounts: []string{"personal", "side"}, Since: h.today(13, 40, 0).UTC()}
		w.best, w.events = "side", ofTheRun(append(h.atPersonalsLimit(), h.pinned(h.today(13, 40, 0), "personal", "side")))
		w.routed, w.stream, w.lines = pinnedSessions, sceneStream(a.now, "side"), pinnedLines
		return w
	})
}

// pinnedSessions are the scene's sessions, as the pinned frame lists them:
// 41e0 and 9c2e moved to side as personal reached its limit at 14:12, the
// rest as they are, on side.
func pinnedSessions(now time.Time) []status.Session {
	limited := clockAt(now, 0, 14, 12, 0)
	sessions := sceneSessions(now)
	for _, s := range sessions {
		for i, a := range s.Assignments {
			if a.Account == "personal" {
				s.Assignments[i] = assigned(a.Model, "side", a.Dir, reasonLimited, limited, a.LastSeen, a.InFlight)
			}
		}
	}
	return sessions
}

// pinnedLines are the sessions' world's lines as the pinned frame's world
// has them: 41e0's and 9c2e's on side from personal's limit at 14:12, the
// first of each model's there moved from personal; and 7f3a's on side from
// its start, where it began as the pin had it, so none of its moved.
func pinnedLines(now time.Time) []ledger.Line {
	limited := clockAt(now, 0, 14, 12, 0)
	lines := sessionsLines(now)
	moved := make(map[[2]string]bool)
	for i, l := range lines {
		switch {
		case l.Session == id7F3A:
			lines[i].Account, lines[i].From, lines[i].Tried, lines[i].Attempts = "side", "", nil, 1
			if lines[i].Reason == reasonCapped {
				lines[i].Reason = status.ReasonSticky
			}
		case (l.Session == id41E0 || l.Session == id9C2E) && l.Account == "personal" && !l.At.Before(limited):
			lines[i].Account = "side"
			if seat := [2]string{l.Session, l.Model}; !moved[seat] {
				lines[i].From, lines[i].Reason, moved[seat] = "personal", reasonLimited, true
			}
		}
	}
	return lines
}

// How long after 14:42:07 the frames at personal's limit are drawn: at
// 16:05:41, as 41e0's request there is refused, and a second on, as it
// moves to side.
const (
	refusedAfter = time.Hour + 23*time.Minute + 34*time.Second
	movedAfter   = refusedAfter + time.Second
)

// How long before their moments 41e0's request was refused at personal's
// limit: as its red pulse's head, frozen, reaches cell 14 of 44 from the
// jack, at 16:05:41; and as the request, retried on side once that pulse
// came home, reaches cell 7 of 49 there, at 16:05:42.
const (
	refusedFor = 178 * time.Millisecond
	movedFor   = 620 * time.Millisecond
)

// refusedAt and movedAt are the auto world as personal reaches its 5-hour
// limit, as 41e0 asks there, as limitedAt has it: at 16:05:41, the request
// refused, the router listing the sessions as routedAtTheLimit does; and at
// 16:05:42, 41e0 moved to side, as routedAfterTheLimit does.
func (a accountWorlds) refusedAt() accountWorld {
	return a.kept("refusedAt", func() accountWorld {
		now := a.now.Add(refusedAfter)
		return a.limitedAt(now, ago(now, refusedFor), 0, routedAtTheLimit)
	})
}

func (a accountWorlds) movedAt() accountWorld {
	return a.kept("movedAt", func() accountWorld {
		now := a.now.Add(movedAfter)
		return a.limitedAt(now, ago(now, movedFor), 1, routedAfterTheLimit)
	})
}

// limitedAt is the auto world at now, personal reaching its 5-hour limit at
// refused, as 41e0's request there is refused, till its window resets at
// 16:30, its event counting moved sessions moved by now; side's 5-hour
// window at 31% by 16:05; the router listing the sessions as routed has
// them, and read fresh, as it read personal reaching its limit; its stream
// telling of 41e0's request, as limitStream has it, and of c61b's answer
// streaming back.
func (a accountWorlds) limitedAt(now, refused time.Time, moved int, routed func(time.Time) []status.Session) accountWorld {
	accounts, t, h := firstAccounts(3), a.today().towardTheLimit(refused), happenings{a.now}
	until := h.today(16, 30, 0)
	limit := status.Event{
		Kind: status.EventLimit, At: refused, Account: "personal", Windows: []string{fiveHourKey}, Until: until, Count: moved, To: "side",
		Limit: todaysLimit,
	}
	return accountWorld{
		accounts: accounts, history: t.history(accounts), sessions: autoSessions, best: framesChoice,
		limits: map[string]status.Limit{"personal": {ID: todaysLimit, Windows: []string{fiveHourKey}, Until: until.UTC()}},
		routed: routed, events: ofTheRun(append(h.auto(), limit)), stream: limitStream(now, refused, moved > 0),
		traffic: true, fresh: true,
	}
}

// towardTheLimit takes in today's readings of the three as the auto world
// has them, but personal's 5-hour window rising on to its limit at refused,
// and side's to 31% by 16:05.
func (t *todayOf) towardTheLimit(refused time.Time) *todayOf {
	t.work()
	t.personal(level{refused, 1})
	t.side(sidesFable, level{t.at(16, 5, 0), .31})
	return t
}

// limitStream is what the request stream tells of at now, 41e0's request on
// personal refused at refused, as the limit there has it, 300ms after it
// went; where it has moved, moved to side then, and gone out again there;
// and c61b's answer streaming back on side, as the scene's does.
func limitStream(now, refused time.Time, moved bool) []router.StreamEvent {
	asked := request{id: "00000601", session: id41E0, model: opus, account: "personal"}
	limited := asked.told(router.StreamLimited, refused)
	limited.Status = 429
	told := []router.StreamEvent{asked.told(router.StreamSent, refused.Add(-300*time.Millisecond)), limited}
	if moved {
		retried := request{id: asked.id, session: id41E0, model: opus, account: "side"}
		told = append(told, retried.moved("personal", reasonLimited, refused), retried.told(router.StreamSent, refused))
	}
	c61b := request{id: "00000602", session: idC61B, model: opus, account: "side"}
	progress := c61b.told(router.StreamProgress, ago(now, heardC61B))
	progress.Chars = chars1200
	told = append(told, c61b.told(router.StreamSent, ago(now, firstC61B+400*time.Millisecond)), c61b.told(router.StreamFirst, ago(now, firstC61B)), progress)
	slices.SortStableFunc(told, func(a, b router.StreamEvent) int { return a.At.Compare(b.At) })
	return told
}

// inTheFramesOrder are the sessions routed lists, in the frame's order, as
// a look before its moment saw them, so the Switchboard keeps them so: the
// one listed first seen last, each a second before the next, none with a
// request in flight.
func inTheFramesOrder(routed func(now time.Time) []status.Session) func(now time.Time) []status.Session {
	return func(now time.Time) []status.Session {
		sessions := routed(now)
		for i, s := range sessions {
			for j := range s.Assignments {
				s.Assignments[j].LastSeen, s.Assignments[j].InFlight = ago(now, time.Duration(i+1)*time.Second).UTC(), ""
			}
		}
		return sessions
	}
}
