package capture

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// The frames' sample sessions, each a Claude Code session's id, which the
// frames cut to its first four characters.
const (
	idD28C = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
	id7F3A = "7f3a0c94-5d2e-4b18-a6f7-31c8e9d24b05"
	idDB8A = "db8a71e3-9f04-4c6d-b2a8-5e1f7c39d860"
	idC61B = "c61b2f8d-04a7-4e93-9b5c-d81e6a7f2c34"
	id41E0 = "41e0b6c2-8d3f-4a71-95e4-2c7b0f9a1d68"
	id9E21 = "9e21d4a8-6b0c-4f35-8e72-a39c5d1f0b47"
	id5C7A = "5c7a93f1-2e6d-4b08-a4c9-7f1e0d3b8a26"
	idE4B0 = "e4b06a2d-c71f-4e98-b3d5-0a8f2e6c9b14"
)

// The models the sample sessions use, one of each family the frames name.
const (
	opus   = "claude-opus-5-5"
	sonnet = "claude-sonnet-5-5"
	haiku  = "claude-haiku-4-5-20251001"
)

// The windows' keys, as Claude names them.
const (
	fiveHourKey = "5h"
	weekKey     = "7d"
	fableKey    = "7d_oi"
)

// reasonLimit is the router's reason for a sample session moved off personal
// as it reached its limit, as its log has it.
const reasonLimit = status.ReasonMovedOff + "personal has no room"

// readAgo is how long before the moment drawn the router's document was read,
// as the frames' footers have it: "read 4s ago".
const readAgo = 4 * time.Second

// sample is an account as the frames draw it, from the generator that drew
// them: enough to give its status, its sessions and its windows' history.
type sample struct {
	// id names it, and label is what it's shown as: its id, as the frames
	// have it, where it's "".
	id, label string
	primary   bool
	// reserve is the share of every window the router leaves unused.
	reserve float64
	session fiveHours
	week    weekly
	fable   weekly
	// read is when its usage was last read: zero is as the document was.
	read time.Time
	// seed shapes its history, as the generator's seed does.
	seed int
	// prime is when the router next primes it.
	prime time.Time
	seats []seat
}

// fiveHours is a sample's five-hour window, as last read: one whose reset has
// passed since has lapsed, and reads empty.
type fiveHours struct {
	used   float64
	resets time.Time
	// heading is the share of it used by its reset, and out when it runs
	// out before its reset, at the rate the router saw it used over the last
	// half hour, as the frames have it. Neither is set where the router saw
	// no rate.
	heading float64
	out     time.Time
	// limited is when the account reached its limit in it: zero when it
	// hasn't. limit is that limit's identity, as the router gives it, its
	// event and the account's limit alike.
	limited time.Time
	limit   int
}

// The identities of the samples' limits, as the router numbers its limits
// from 1 as it starts: personal's, reached first, and work's, reached in
// the storyboard's last frames.
const (
	personalLimit = 1
	workLimit     = 2
)

// weekly is a sample's week, or Fable's, used at the pace its use since it
// started sets.
type weekly struct {
	used   float64
	resets time.Time
}

// seat is a session's model on a sample account: the session, the model, when
// the router assigned it there and why, whether the session's own pin put it
// there, and when it was given that pin as it ran, zero for one it was
// launched with; and when it was last seen.
type seat struct {
	session, model string
	reason         string
	assigned, seen time.Time
	pinned         bool
	pinnedAt       time.Time
}

// on is a time of day on a day of the frames' month, in now's time zone.
func on(now time.Time, day, hour, minute int) time.Time {
	return time.Date(now.Year(), now.Month(), day, hour, minute, 0, 0, now.Location())
}

// ago is d before now.
func ago(now time.Time, d time.Duration) time.Time {
	return now.Add(-d)
}

// work, the primary, keeping a tenth of every window back, is under pressure:
// at its last half hour's rate, its session runs out at 16:05, before its
// reset at 17:10, reaching its reserve first.
func work(now time.Time) sample {
	return sample{
		id: "work", primary: true, reserve: 0.1, seed: 1,
		session: fiveHours{used: 0.58, resets: on(now, 1, 17, 10), out: on(now, 1, 16, 5)},
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

// personal reached its session's limit at 14:12, which holds until the
// session resets at 15:54, moving its three sessions to side. Its week, 89%
// used, runs out at its pace on Friday at 04:06, before it resets on Sunday:
// 0.8902 is the use that has it run out then, and reads 89%.
func personal(now time.Time) sample {
	return sample{
		id: "personal", seed: 2,
		session: fiveHours{used: 1, resets: on(now, 1, 15, 54), limited: on(now, 1, 14, 12), limit: personalLimit},
		week:    weekly{used: 0.8902, resets: on(now, 4, 2, 0)},
		fable:   weekly{resets: on(now, 4, 2, 0)},
		prime:   on(now, 2, 5, 30),
	}
}

// side is the account new sessions go to: its session, started at 13:50 by a
// prime, heads for 54% by its reset.
func side(now time.Time) sample {
	return sample{
		id: "side", seed: 3,
		session: fiveHours{used: 0.12, resets: on(now, 1, 18, 50), heading: 0.54},
		week:    weekly{used: 0.33, resets: on(now, 5, 10, 0)},
		fable:   weekly{used: 0.04, resets: on(now, 5, 10, 0)},
		prime:   on(now, 2, 7, 10),
		seats: []seat{
			{session: idC61B, model: opus, assigned: on(now, 1, 14, 41), seen: ago(now, 2*time.Second)},
			{session: idDB8A, model: opus, reason: reasonLimit, assigned: on(now, 1, 14, 12), seen: ago(now, 4*time.Second)},
			{session: id41E0, model: sonnet, reason: reasonLimit, assigned: on(now, 1, 14, 12), seen: ago(now, 4*time.Minute)},
		},
	}
}

// client keeps a fifth of every window back, its week, 78% used, nearing it.
// Its one session was pinned to it at 14:39.
func client(now time.Time) sample {
	return sample{
		id: "client", reserve: 0.2, seed: 4,
		session: fiveHours{used: 0.31, resets: on(now, 1, 16, 40), heading: 0.48},
		week:    weekly{used: 0.78, resets: on(now, 6, 9, 0)},
		fable:   weekly{used: 0.2, resets: on(now, 6, 9, 0)},
		prime:   on(now, 2, 8, 40),
		seats: []seat{
			{session: id9E21, model: sonnet, reason: status.ReasonPinned, pinned: true, pinnedAt: on(now, 1, 14, 39), assigned: on(now, 1, 14, 39), seen: ago(now, 5*time.Second)},
		},
	}
}

// spare is idle: its session lapsed at 11:20, nothing read of it since 07:20,
// and the router primes it at 16:20.
func spare(now time.Time) sample {
	return sample{
		id: "spare", seed: 5, read: on(now, 1, 7, 20),
		session: fiveHours{resets: on(now, 1, 11, 20)},
		week:    weekly{used: 0.05, resets: on(now, 7, 14, 0)},
		fable:   weekly{resets: on(now, 7, 14, 0)},
		prime:   on(now, 1, 16, 20),
	}
}

// lab has a session busy on it.
func lab(now time.Time) sample {
	return sample{
		id: "lab", seed: 6,
		session: fiveHours{used: 0.44, resets: on(now, 1, 17, 30), heading: 0.71},
		week:    weekly{used: 0.52, resets: on(now, 3, 18, 0)},
		fable:   weekly{resets: on(now, 3, 18, 0)},
		prime:   on(now, 2, 9, 10),
		seats: []seat{
			{session: id5C7A, model: sonnet, assigned: on(now, 1, 14, 20), seen: ago(now, 6*time.Second)},
		},
	}
}

// team's session was started at 14:05 by the session on it, idle since.
func team(now time.Time) sample {
	return sample{
		id: "team", seed: 7,
		session: fiveHours{used: 0.08, resets: on(now, 1, 19, 5), heading: 0.2},
		week:    weekly{used: 0.21, resets: on(now, 3, 8, 0)},
		fable:   weekly{resets: on(now, 3, 8, 0)},
		prime:   on(now, 2, 9, 50),
		seats: []seat{
			{session: idE4B0, model: haiku, assigned: on(now, 1, 14, 5), seen: ago(now, 12*time.Minute)},
		},
	}
}

// extra is idle, as spare is: its session lapsed at 12:40, nothing read of it
// since 08:40, and the router primes it at 17:40.
func extra(now time.Time) sample {
	return sample{
		id: "extra", seed: 8, read: on(now, 1, 8, 40),
		session: fiveHours{resets: on(now, 1, 12, 40)},
		week:    weekly{used: 0.02, resets: on(now, 6, 22, 0)},
		fable:   weekly{resets: on(now, 6, 22, 0)},
		prime:   on(now, 1, 17, 40),
	}
}

// withoutFable is the samples with nothing used of Fable's week, as the
// frames of four, six and eight accounts have them, so it hides from every
// card.
func withoutFable(samples ...sample) []sample {
	for i := range samples {
		samples[i].fable.used = 0
	}
	return samples
}

// oneAccount is work alone, which new sessions go to under pressure as it is:
// there's no other.
func oneAccount(now time.Time) source {
	return newSource(now, []sample{work(now)}, "work",
		pressured(now, on(now, 1, 14, 38), work(now)),
		primed(on(now, 1, 12, 10), work(now)),
	)
}

// threeAccounts are work, personal and side, as most frames draw them.
func threeAccounts(now time.Time) source {
	return newSource(now, []sample{work(now), personal(now), side(now)}, "side", lately(now)...)
}

// fourAccounts are threeAccounts and client, nothing used of Fable's week.
func fourAccounts(now time.Time) source {
	return newSource(now, withoutFable(work(now), personal(now), side(now), client(now)), "side", lately(now)...)
}

// fiveAccounts are threeAccounts, client and spare, and among what the router
// tells of lately, client's session pinned to it, from side.
func fiveAccounts(now time.Time) source {
	moved := status.Event{
		At: on(now, 1, 14, 39), Kind: status.EventMoved, Session: id9E21, Model: sonnet,
		From: "side", To: "client", Reason: status.ReasonPinned,
	}
	events := slices.Insert(lately(now), 1, moved)
	return newSource(now, []sample{work(now), personal(now), side(now), client(now), spare(now)}, "side", events...)
}

// sixAccounts are fourAccounts, spare and lab, nothing used of Fable's week.
func sixAccounts(now time.Time) source {
	return newSource(now, withoutFable(work(now), personal(now), side(now), client(now), spare(now), lab(now)), "side", lately(now)...)
}

// eightAccounts are sixAccounts, team and extra, nothing used of Fable's
// week.
func eightAccounts(now time.Time) source {
	samples := withoutFable(work(now), personal(now), side(now), client(now), spare(now), lab(now), team(now), extra(now))
	return newSource(now, samples, "side", lately(now)...)
}

// lately is what the router tells of lately, the newest first, as the frames
// of three or more accounts have it: c61b starting on side, work coming under
// pressure, personal reaching its limit, moving its three sessions to side,
// and side's prime, which RECENT tells of; and, older, or counted in the
// limit, what the cards' backs tell of besides: db8a's opus and 41e0 among
// the sessions the limit moved, d28c starting on work, and work's and
// personal's primes.
func lately(now time.Time) []status.Event {
	return []status.Event{
		{At: on(now, 1, 14, 41), Kind: status.EventStarted, Account: "side", Session: idC61B, Model: opus, Reason: status.ReasonNew},
		pressured(now, on(now, 1, 14, 38), work(now)),
		{At: on(now, 1, 14, 12), Kind: status.EventMoved, Session: id41E0, Model: sonnet, From: "personal", To: "side", Reason: reasonLimit, Limit: forced},
		{At: on(now, 1, 14, 12), Kind: status.EventMoved, Session: idDB8A, Model: opus, From: "personal", To: "side", Reason: reasonLimit, Limit: forced},
		{At: on(now, 1, 14, 12), Kind: status.EventLimit, Account: "personal", Windows: []string{fiveHourKey}, Until: on(now, 1, 15, 54), Count: 3, To: "side", Limit: personalLimit},
		primed(on(now, 1, 13, 50), side(now)),
		{At: on(now, 1, 13, 20), Kind: status.EventStarted, Account: "work", Session: idD28C, Model: opus, Reason: status.ReasonNew},
		primed(on(now, 1, 12, 10), work(now)),
		primed(on(now, 1, 10, 54), personal(now)),
	}
}

// pressured is the sample coming under pressure at a time: its session
// running out where the router has it, at its last half hour's rate.
func pressured(now, at time.Time, s sample) status.Event {
	return status.Event{
		At: at, Kind: status.EventPressure, Account: s.id, Windows: []string{fiveHourKey},
		Until: s.account(now).Pressure.RunsOut, Since: at.Add(-score.Recent),
	}
}

// primed is the sample primed at a time, starting the session it's in now.
func primed(at time.Time, s sample) status.Event {
	return status.Event{At: at, Kind: status.EventPrimed, Account: s.id, Windows: []string{fiveHourKey}, Until: s.session.resets}
}

// account is the sample's status at now, as the router gives it: its windows
// as read, standing as they do at now, so a session whose reset has passed
// since has lapsed and reads empty; held back by its limit while it holds;
// and its session's rate, where the router saw one, and the pressure that
// puts it under.
func (s sample) account(now time.Time) status.Account {
	read := s.read
	if read.IsZero() {
		read = ago(now, readAgo)
	}
	a := status.Account{
		ID: s.id, Label: cmp.Or(s.label, s.id), Primary: s.primary, Reserve: s.reserve, TokenSet: true, FetchedAt: read,
		Windows: s.windows(), Sessions: len(s.seats),
	}
	if !s.session.limited.IsZero() {
		a.Limit = status.Limit{ID: s.session.limit, Windows: []string{fiveHourKey}, Until: s.session.resets}
	}
	if rate := s.session.rate(now); rate > 0 {
		since := ago(now, score.Recent)
		a.Rates = []status.Rate{{Window: fiveHourKey, Rate: rate, Since: since}}
		a.Pressure = pressure(a, rate, since, now)
	}
	return a.AsOf(claude.Policy, now)
}

// windows are the sample's windows as read, in quota.Sort's order.
func (s sample) windows() []quota.Window {
	session := quota.Window{Key: fiveHourKey, Label: "Session", Utilization: s.session.used, ResetsAt: s.session.resets}
	if !s.session.limited.IsZero() {
		session.Status = quota.StatusRejected
	}
	return []quota.Window{
		session,
		{Key: weekKey, Label: "Week", Utilization: s.week.used, ResetsAt: s.week.resets},
		{Key: fableKey, Label: "Fable week", Utilization: s.fable.used, ResetsAt: s.fable.resets},
	}
}

// rate is the share of the window the router saw used an hour over the last
// half hour, at now: the one that has it run out, or head, where the frames
// have it, or none where they have neither.
func (f fiveHours) rate(now time.Time) float64 {
	switch {
	case !f.out.IsZero():
		return (1 - f.used) / f.out.Sub(now).Hours()
	case f.heading > f.used:
		return (f.heading - f.used) / f.resets.Sub(now).Hours()
	default:
		return 0
	}
}

// pressure is how the account's session stands at now, used from now on at
// rate, measured since since, as the router judges it: when it reaches where
// the account's room ends, and whether that's before the session resets.
func pressure(a status.Account, rate float64, since, now time.Time) status.Pressure {
	p := claude.Policy.PressureOf(score.Candidate{ID: a.ID, Windows: a.Windows, Reserve: a.Reserve, Rate: rate}, now)
	return status.Pressure{Window: fiveHourKey, Rate: rate, Recent: true, Since: since, RunsOut: p.RunsOut, Under: p.Under}
}
