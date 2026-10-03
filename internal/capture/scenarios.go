package capture

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// idB3E9 is the session the routing scenario starts.
const idB3E9 = "b3e94c07-6a1d-4f82-9e5b-0c7d2a8f6e31"

// scenarios are every scenario, starting at now, in nord: the README's demos.
// Each one's cues are what its router does, a request at a time, each at its
// offset in seconds from the start; the keys are its tape's, in demo/, which
// presses them as the cues come.
func scenarios(now time.Time) []Scenario {
	nord, _ := theme.Builtin(theme.DefaultDark)
	all := []Scenario{
		{Name: "routing", Size: wide(34), start: now, setting: routingWorld, cues: routingCues},
		{Name: "usage", Size: wide(27), start: now, setting: usageWorld, cues: usageCues},
		{Name: "themes", Size: wide(34), start: now, setting: routingWorld, cues: themesCues},
	}
	for i := range all {
		all[i].theme = nord
	}
	return all
}

// routingWorld is the router the routing scenario starts with, at now:
// work, personal, the primary, and side, routed among, new sessions going to
// side. work is nearly spent: at its last half hour's rate its session runs
// out at 14:45, before its reset at 17:10, so it's under pressure, and new
// sessions go elsewhere.
func routingWorld(now time.Time) world {
	w, p, s := routingWork(now), routingPersonal(now), routingSide(now)
	events := []status.Event{
		{At: on(now, 1, 14, 41), Kind: status.EventStarted, Account: "side", Session: idC61B, Model: opus, Reason: status.ReasonNew},
		pressured(now, on(now, 1, 14, 38), w),
		primed(on(now, 1, 13, 50), s),
		{At: on(now, 1, 13, 20), Kind: status.EventStarted, Account: "work", Session: idD28C, Model: opus, Reason: status.ReasonNew},
		primed(on(now, 1, 12, 10), w),
		primed(on(now, 1, 11, 30), p),
	}
	return world{samples: []sample{w, p, s}, best: "side", events: events}
}

// routingWork is work as the routing scenario starts, at now, with three
// sessions on it.
func routingWork(now time.Time) sample {
	return sample{
		id: "work", label: "Work", seed: 1,
		session: fiveHours{used: 0.94, resets: on(now, 1, 17, 10), out: on(now, 1, 14, 45)},
		week:    weekly{used: 0.34, resets: on(now, 5, 21, 0)},
		fable:   weekly{used: 0.12, resets: on(now, 5, 21, 0)},
		prime:   on(now, 2, 3, 50),
		seats: []seat{
			{session: idD28C, model: opus, assigned: on(now, 1, 13, 20), seen: ago(now, 50*time.Second)},
			{session: id7F3A, model: haiku, assigned: on(now, 1, 13, 10), seen: ago(now, 9*time.Minute)},
			{session: idDB8A, model: sonnet, assigned: on(now, 1, 13, 5), seen: ago(now, 3*time.Second)},
		},
	}
}

// routingPersonal is personal, the primary, keeping a tenth of every window
// back, as the routing scenario starts, at now: its session started at
// 11:30, heading for 62% by its reset at 16:30, and its week, 66% used, at
// its pace reaching its reserve on Saturday, before it resets on Sunday.
func routingPersonal(now time.Time) sample {
	return sample{
		id: "personal", label: "Personal", primary: true, reserve: 0.1, seed: 2,
		session: fiveHours{used: 0.38, resets: on(now, 1, 16, 30), heading: 0.62},
		week:    weekly{used: 0.66, resets: on(now, 4, 2, 0)},
		fable:   weekly{resets: on(now, 4, 2, 0)},
		prime:   on(now, 2, 5, 30),
		seats: []seat{
			{session: id41E0, model: sonnet, assigned: on(now, 1, 11, 42), seen: ago(now, 40*time.Second)},
		},
	}
}

// routingSide is side, where new sessions go, as the routing scenario
// starts, at now: its session, started at 13:50 by a prime, heads for 54% by
// its reset.
func routingSide(now time.Time) sample {
	return sample{
		id: "side", label: "Side", seed: 3,
		session: fiveHours{used: 0.12, resets: on(now, 1, 18, 50), heading: 0.54},
		week:    weekly{used: 0.33, resets: on(now, 5, 10, 0)},
		fable:   weekly{used: 0.04, resets: on(now, 5, 10, 0)},
		prime:   on(now, 2, 7, 10),
		seats: []seat{
			{session: idC61B, model: opus, assigned: on(now, 1, 14, 41), seen: ago(now, 2*time.Second)},
		},
	}
}

// routingCues are the routing scenario's, which demo/routing.tape plays
// through Accounts, Sessions from 8 seconds, Runway from 17 and Accounts
// again from 24: a new session starting on side, the best; traffic on every
// account; work reaching its session limit at 12 seconds, its sessions moving
// to side, one with the request the limit refused, the others with their
// next; and while a card is flipped and a session on it pinned to personal,
// at about 29 seconds, traffic on side, but none from there till 31.4, when
// each of side's sessions asks again in turn, so whichever was pinned shows
// its pin for a moment, then goes with its next request.
var routingCues = []cue{
	asks(1, idDB8A, sonnet, brief),
	starts(2.5, idB3E9, opus, long),
	asks(4, idD28C, opus, brief),
	asks(5.5, id41E0, sonnet, brief),
	asks(6.5, idC61B, opus, brief),

	asks(9, idC61B, opus, brief),
	asks(9.3, idD28C, opus, long),
	asks(9.8, idB3E9, opus, brief),
	asks(10.4, id41E0, sonnet, brief),
	asks(10.8, id7F3A, haiku, brief),
	reaches(12, idDB8A, sonnet, long),
	asks(14, id7F3A, haiku, brief),
	asks(15.2, idD28C, opus, long),
	asks(16, idB3E9, opus, brief),
	asks(16.6, id41E0, sonnet, brief),

	asks(19.5, idB3E9, opus, long),
	asks(21.5, idDB8A, sonnet, brief),

	asks(24.5, idC61B, opus, steady),
	asks(25.2, idDB8A, sonnet, steady),
	asks(25.9, idD28C, opus, steady),
	asks(26.6, id7F3A, haiku, brief),
	asks(27.3, idB3E9, opus, steady),
	asks(28.2, idC61B, opus, brief),
	asks(28.9, idDB8A, sonnet, brief),
	asks(29, id41E0, sonnet, brief),
	asks(31.4, idB3E9, opus, brief),
	asks(31.8, idC61B, opus, brief),
	asks(32.2, idDB8A, sonnet, brief),
	asks(32.6, idD28C, opus, brief),
	asks(33, id7F3A, haiku, brief),
	asks(33.6, id41E0, sonnet, brief),
	asks(34.2, idB3E9, opus, brief),
	asks(34.6, idC61B, opus, brief),
}

// usageWorld is the router the usage scenario starts with, at now: work
// alone, as accounts-1 has it, labelled as the routing scenario's is, under
// pressure, at its last half hour's rate reaching its reserve at 15:45,
// before its reset at 17:10.
func usageWorld(now time.Time) world {
	w := work(now)
	w.label = "Work"
	events := []status.Event{
		pressured(now, on(now, 1, 14, 38), w),
		primed(on(now, 1, 12, 10), w),
	}
	return world{samples: []sample{w}, best: "work", events: events}
}

// usageCues are the usage scenario's, which demo/usage.tape plays through the
// card's charts and Runway: each of work's sessions at work, 7f3a, idle 9
// minutes as it starts, among them.
var usageCues = []cue{
	asks(1, idDB8A, sonnet, brief),
	asks(2.5, id7F3A, haiku, brief),
	asks(4, idD28C, opus, long),
	asks(7, idDB8A, sonnet, long),
	asks(9, id7F3A, haiku, brief),
	asks(10.5, idD28C, opus, long),
	asks(12.5, idDB8A, sonnet, brief),
	asks(14, id7F3A, haiku, long),
	asks(16, idD28C, opus, brief),
	asks(17.5, idDB8A, sonnet, brief),
}

// themesCues are the themes scenario's, which demo/themes.tape plays under
// the theme picker: the routing scenario's world, and its sessions at work,
// but for any limit.
var themesCues = []cue{
	asks(1, idDB8A, sonnet, brief),
	asks(2, idC61B, opus, long),
	asks(3.5, id41E0, sonnet, brief),
	asks(5, idD28C, opus, brief),
	asks(6.5, idC61B, opus, brief),
	asks(8, idDB8A, sonnet, long),
	asks(9.5, id41E0, sonnet, brief),
	asks(11, idD28C, opus, brief),
}
