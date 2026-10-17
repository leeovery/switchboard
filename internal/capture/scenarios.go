package capture

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// idB3E9 is the session the routing scenario starts.
const idB3E9 = "b3e94c07-6a1d-4f82-9e5b-0c7d2a8f6e31"

// scenarios are every scenario, in nord, over the worlds of the frames'
// moment: the README's demos. Each one's cues are what its router does, a
// request at a time, each at its offset in seconds from the start; the keys
// are its tape's, in demo/, which presses them as the cues come, and which
// the themes, runway-week, phone and help tapes press over routing's.
func scenarios() []Scenario {
	all := []Scenario{
		{Name: "routing", Size: wide(37), world: accountWorlds.auto, cues: routingCues},
		{Name: "dashboard", Size: wide(37), world: accountWorlds.busyOnWork, cues: dashboardCues},
		{Name: "repatch", Size: wide(37), world: accountWorlds.beforeTheLimit, from: repatchFrom, cues: repatchCues},
	}
	for i := range all {
		all[i].worlds, all[i].theme = worldsNow, framesTheme()
	}
	return all
}

// routingCues are the routing scenario's, over the auto world from 14:42:07,
// as the Overview's frames draw it: the three requests in flight landing in
// the first 3 seconds; side's sessions asking every second and a half or
// so, and personal's, 41e0 and 9c2e, every 4; and b3e9 starting on side,
// the best, at 2.5, shown at the look by 5. demo/routing.tape presses p at
// 9, 3 at 10.5, m at 12, the card counting 41e0 and 9c2e to move, and enter
// at 14.5, pinning new and running sessions to side: neither asks again till
// then, when 41e0's next request, at 17, and 9c2e's, at 21, each go to side,
// a move shown at each of the looks by 20 and 25, and the tape hides at 27.
// 9c2e asks clear of the look at 20, which a cue on it races.
var routingCues = []cue{
	asks(1.2, idDB8A, opus, brief),
	starts(2.5, idB3E9, opus, "~/Code/cli", long),
	asks(3.4, idC61B, opus, brief),
	asks(4, id41E0, sonnet, brief),
	asks(4.6, id7F3A, sonnet, steady),
	asks(5.8, idD28C, opus, brief),
	asks(6.2, id9C2E, opus, brief),
	asks(7, idDB8A, opus, steady),
	asks(8, id41E0, sonnet, steady),
	asks(8.4, idB3E9, opus, brief),
	asks(9.6, idC61B, opus, steady),
	asks(10.2, id9C2E, opus, brief),
	asks(11, id7F3A, sonnet, brief),
	asks(12, id41E0, sonnet, brief),
	asks(12.6, idD28C, opus, steady),
	asks(14, id9C2E, opus, brief),
	asks(14.2, idDB8A, opus, brief),
	asks(15.6, idB3E9, opus, steady),
	asks(17, id41E0, sonnet, steady),
	asks(17.4, idC61B, opus, brief),
	asks(18.8, id7F3A, sonnet, brief),
	asks(21, id9C2E, opus, steady),
	asks(21.2, idD28C, opus, brief),
	asks(22.5, idDB8A, opus, steady),
	asks(23.8, idB3E9, opus, brief),
	asks(24.6, id41E0, sonnet, brief),
	asks(25.2, idC61B, opus, brief),
}

// busyOnWork is the dashboard scenario's world: work alone, as Sessions'
// frame of it lists its sessions, aloneOnWork, its requests in flight there;
// the router telling of its afternoon as Accounts' frame of work alone has
// it, oneLately; and today's lines on work, as onWorkLines has them, so its
// trace and its sessions' costs are drawn from the start.
func (a accountWorlds) busyOnWork() accountWorld {
	return a.kept("busyOnWork", func() accountWorld {
		w := a.aloneOnWork()
		w.events, w.lines = a.oneLately().events, onWorkLines
		return w
	})
}

// onWorkLines are the sessions' world's lines, each put on work, as onWork
// puts its sessions: none moved or tried elsewhere, nor sent anywhere afresh,
// work keeping each session while its cache is warm.
func onWorkLines(now time.Time) []ledger.Line {
	lines := sessionsLines(now)
	for i := range lines {
		l := &lines[i]
		l.Account, l.From, l.Tried, l.Attempts = "work", "", nil, 1
		if l.Reason != status.ReasonNew && l.Reason != reasonUnsessioned {
			l.Reason = status.ReasonSticky
		}
	}
	return lines
}

// dashboardCues are the dashboard scenario's, over work alone from 14:42:07:
// the three requests in flight landing in the first 3 seconds, and work's
// five sessions asking in turn, a request a second, each once its last is
// answered, enough for work's 5-hour window to read a hundredth up, to 59%,
// at 14.1, shown at the look by 15. demo/dashboard.tape presses g at 6,
// GRAPHS shown, then tab four times from 11, to Runway, and > at 12.2 to its
// 5-hour.
var dashboardCues = []cue{
	asks(0.6, id41E0, opus, long),
	asks(1.6, id9C2E, opus, long),
	asks(2.6, idC61B, opus, long),
	asks(3.6, id7F3A, opus, long),
	asks(4.6, idD28C, opus, long),
	asks(5.8, id41E0, opus, long),
	asks(6.8, id9C2E, opus, long),
	asks(7.8, idC61B, opus, long),
	asks(8.8, id7F3A, opus, long),
	asks(9.8, idD28C, opus, long),
	asks(11, id41E0, opus, long),
	asks(12, id9C2E, opus, long),
	asks(13, idC61B, opus, long),
	asks(14.2, id7F3A, opus, steady),
	asks(15.2, idD28C, opus, steady),
	asks(16.4, id41E0, opus, steady),
	asks(17.6, id9C2E, opus, steady),
	asks(18.8, idC61B, opus, steady),
}

// repatchLead is how long before personal reaches its limit, at 16:05:41,
// the repatch scenario starts; and repatchFrom, how long after the worlds'
// moment that is: at 16:05:26.
const (
	repatchLead = 15 * time.Second
	repatchFrom = refusedAfter - repatchLead
)

// beforeTheLimit is the repatch scenario's world: the auto world as it
// stands at 16:05:26, as refusedAt's would have it then, before personal
// reaches its limit: its readings toward it read by then, personal's last
// below it; the router reading it fresh, telling of the auto world's day,
// and listing the sessions as routedBeforeTheLimit has them, none in flight.
func (a accountWorlds) beforeTheLimit() accountWorld {
	return a.kept("beforeTheLimit", func() accountWorld {
		now, refused := a.now.Add(repatchFrom), ago(a.now.Add(refusedAfter), refusedFor)
		accounts := firstAccounts(3)
		return accountWorld{
			accounts: accounts, history: a.today().towardTheLimit(refused).history(accounts).before(now), sessions: autoSessions,
			best: framesChoice, routed: routedBeforeTheLimit, events: autoEvents(a.now), traffic: true, fresh: true,
		}
	})
}

// routedBeforeTheLimit are the sessions the router lists at 16:05:26, as
// routedAtTheLimit has them 15 seconds on, each idle: on personal, 41e0,
// since 16:04:48, and 9c2e, since 15:52:30; on side c61b, since 16:04:40,
// and, moved from work as it reached its cap, d28c, since 16:05:12, db8a,
// since 16:04:55, and 7f3a, since 16:03:12.
func routedBeforeTheLimit(now time.Time) []status.Session {
	at := func(hour, minute, second int) time.Time { return clockAt(now, 0, hour, minute, second) }
	return []status.Session{
		{ID: idD28C, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/switchboard", reasonCapped, at(14, 31, 13), at(16, 5, 12), "")}},
		{ID: idDB8A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/infra", reasonCapped, at(14, 31, 40), at(16, 4, 55), "")}},
		{ID: id41E0, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/docs", status.ReasonNew, at(12, 47, 12), at(16, 4, 48), "")}},
		{ID: idC61B, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/mobile", status.ReasonNew, at(14, 41, 11), at(16, 4, 40), "")}},
		{ID: id7F3A, Assignments: []status.Assignment{assigned(opus, "side", "~/Code/api", reasonCapped, at(14, 31, 20), at(16, 3, 12), "")}},
		{ID: id9C2E, Assignments: []status.Assignment{assigned(opus, "personal", "~/Code/website", status.ReasonNew, at(11, 55, 31), at(15, 52, 30), "")}},
	}
}

// repatchCues are the repatch scenario's, from 16:05:26: side's sessions at
// work, c61b's answer streaming through personal's limit, as the frames at it
// draw; 41e0's request at 14.5 reaching personal's limit, refused at 14.8,
// as the frame at 16:05:41 has it, and sent on to side at 15.34, as the
// refusal's pulse comes home, before the frame at 16:05:42, its old cord
// hanging from the look by 20, as the limit counts it; and 9c2e asking at
// 21, after that look, moved to side, the limit counting it second, its cord
// hanging from the look by 25. demo/repatch.tape presses tab three times,
// hidden, to Sessions' Switchboard, shows it from 2, and hides it at 27,
// the second cord hanging 2 seconds.
var repatchCues = []cue{
	asks(1, idDB8A, opus, brief),
	asks(3, id7F3A, opus, brief),
	asks(4, idC61B, opus, lengthy),
	asks(7, idD28C, opus, brief),
	asks(9.5, idDB8A, opus, steady),
	asks(12, id7F3A, opus, brief),
	reaches(14.5, id41E0, opus, steady),
	asks(17, idD28C, opus, brief),
	asks(19, idDB8A, opus, brief),
	asks(21, id9C2E, opus, steady),
	asks(22.5, id7F3A, opus, steady),
	asks(23.5, idC61B, opus, brief),
}
