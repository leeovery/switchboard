package capture

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// accountWorld is a frame's accounts at its moment: as configured, as the readings
// history read them, and as the router stands with them then, which the
// readings don't hold: its pin, the limits and refusals in force, when it
// last read an account it couldn't read since, how many sessions it sent
// each in the last hour, and where it sends new sessions, where the frames
// say so; and whether today's traffic of the three, the sessions' world's,
// is the world's.
type accountWorld struct {
	accounts config.Accounts
	history  history
	pin      status.Pin
	limits   map[string]status.Limit
	refused  map[string]status.Refusal
	stale    map[string]time.Time
	sessions map[string]int
	best     string
	traffic  bool
}

// document is the router's status document of the world at now, read
// readAgo before, as the router builds it and serves it, with what's worked
// out of it: each account's windows as last read, standing as they do then;
// how fast each has been used lately, as a router that took up the readings
// history judges it, and the pressure that puts the account under; the
// accounts pinned spending their reserves; and the best account to use next,
// but where the world says where new sessions go.
func (w accountWorld) document(now time.Time) status.Document {
	at := now.Add(-readAgo)
	doc := status.Document{
		GeneratedAt: at.UTC(), Source: status.SourceRouter, Pin: w.pin, Router: status.Health{Healthy: true},
	}
	var open []status.Account
	for _, a := range w.accounts {
		st := w.account(a, at)
		doc.Accounts = append(doc.Accounts, st)
		doc.Sessions += st.Sessions
		if !shut(st, at) {
			open = append(open, st)
		}
	}
	doc.Best = cmp.Or(w.best, status.Best(claude.Policy, open, w.pin.Accounts, at))
	doc.Primary = status.PrimaryOf(doc.Accounts)
	return doc.WorkedOut(claude.Policy)
}

// account is the account a's status at at, as the router gives it: as last
// read at at, or, where the world has the router unable to read it since a
// time, then, its rates and pressure worked out from its readings to then.
func (w accountWorld) account(a config.Account, at time.Time) status.Account {
	read := at
	if stale, ok := w.stale[a.ID]; ok {
		read = stale
	}
	st := status.Configured(a)
	st.TokenSet, st.FetchedAt = true, read.UTC()
	latest := w.history.windows(a.ID, read, claude.WindowLabel)
	st.Windows = latest
	st = st.AsOf(claude.Policy, at)
	st.Limit, st.Refused, st.Sessions = w.limits[a.ID], w.refused[a.ID], w.sessions[a.ID]
	trails := w.trails(a.ID, read)
	for _, l := range latest {
		if rate, since, ok := score.RecentRate(l, trails[l.Key], at); ok {
			st.Rates = append(st.Rates, status.Rate{Window: l.Key, Rate: rate, Since: since.UTC()})
		}
	}
	st.Pressure = pressureOf(st, latest, trails, w.pin.Has(a.ID), at)
	return st
}

// trails are how each of the account with the given id's windows has been
// read lately, by the window's key, as a router that took up the readings
// history as it started holds them: each level its use was read at, its
// baseline among them, a window that started afresh keeping none of those
// before.
func (w accountWorld) trails(id string, at time.Time) map[string][]score.Reading {
	trails := make(map[string][]score.Reading)
	last := make(map[string]quota.Window)
	for _, r := range w.history.before(at) {
		if r.Account != id {
			continue
		}
		held, kept := last[r.Key], r.Window()
		trail := trails[r.Key]
		if !kept.ResetsAt.Equal(held.ResetsAt) || score.ResetByHand(held, kept) {
			trail = nil
		}
		trail = score.Lately(trail, r.At)
		switch n := len(trail); {
		case n == 0 || kept.Utilization > trail[n-1].Utilization:
			trail = append(trail, score.Reading{At: r.At, Utilization: kept.Utilization, Last: r.At})
		case kept.Utilization == trail[n-1].Utilization:
			trail[n-1].Last = r.At
		}
		trails[r.Key], last[r.Key] = trail, kept
	}
	return trails
}

// pressureOf is how the account st's pressure window stands at at, its windows
// as last read latest, as the router gives it: how fast it's being used, and
// when, at that rate, it reaches where the account's room ends, its reserve,
// or, pinned, its limit; under pressure where that's before it resets while
// the account can take a request.
func pressureOf(st status.Account, latest []quota.Window, trails map[string][]score.Reading, pinned bool, at time.Time) status.Pressure {
	i := slices.IndexFunc(latest, func(w quota.Window) bool { return w.Key == claude.Policy.Pressure })
	if i < 0 {
		return status.Pressure{}
	}
	pace, ok := score.PaceOf(latest[i], trails[latest[i].Key], at)
	if !ok {
		return status.Pressure{}
	}
	c := score.Candidate{ID: st.ID, Windows: st.Windows, Reserve: st.Reserve, Rate: pace.Rate}
	if pinned {
		c.Reserve = 0
	}
	p := claude.Policy.PressureOf(c, at)
	takes := !shut(st, at) && score.Available(st.Windows, c.Reserve, claude.Policy.IsShared, at)
	return status.Pressure{
		Window: claude.Policy.Pressure, Rate: pace.Rate, Recent: pace.Recent, Since: pace.Since.UTC(),
		RunsOut: p.RunsOut.UTC(), Under: p.Under && takes,
	}
}

// shut reports whether the account can take no request of any model at at:
// a limit holds back every request, or the upstream refuses its token.
func shut(st status.Account, at time.Time) bool {
	limited := st.Limit.Holds(at) && (len(st.Limit.Windows) == 0 || slices.ContainsFunc(st.Limit.Windows, claude.Policy.IsShared))
	return limited || st.TokenRefused(at)
}

// --------------------------------------------------------- the worlds

// accountWorlds are the accounts' worlds at a moment, over the one year of
// their use the frames draw: each a frame's, the year's readings history to
// today's start shared, today's as each world has it; and today's traffic of
// the three, the sessions' world's lines and the router's events, made once
// for the worlds that hold it.
type accountWorlds struct {
	now     time.Time
	year    year
	past    history
	traffic func() ([]ledger.Held, []events.Line)
}

// newAccountWorlds are the accounts' worlds at now.
func newAccountWorlds(now time.Time) accountWorlds {
	y := newYear(now)
	traffic := sync.OnceValues(func() ([]ledger.Held, []events.Line) { return heldAsWritten(sessionsLines(now)), routerEvents(now) })
	return accountWorlds{now: now, year: y, past: y.readings().history(), traffic: traffic}
}

// level is a use a window was read at, and when.
type level struct {
	at  time.Time
	use float64
}

// todayOf takes in today's readings of a world's accounts.
type todayOf struct {
	worlds accountWorlds
	read   readingsOf
}

// at is a time of today.
func (t *todayOf) at(hour, minute, second int) time.Time {
	return dayAt(t.worlds.now, today, hour, minute, second)
}

// path takes in w's use rising from one level to each next in turn, evenly,
// a reading each hundredth of it.
func (t *todayOf) path(w window, from level, to ...level) {
	for _, l := range to {
		t.read.rise(w, from.at, l.at, from.use, l.use, 0.01)
		from = l
	}
}

// session takes in the account with the given id's 5-hour window, from the
// time of today given, primed or not, rising through the levels given.
func (t *todayOf) session(id string, start time.Time, primed bool, to ...level) {
	w := window{account: id, key: sessionKey, resets: start.Add(sessionLength)}
	source := readings.FromAnswer
	if primed {
		source = readings.FromPrime
	}
	t.read.from(w, start, 0, source)
	t.path(w, level{start, 0}, to...)
}

// week takes in the account with the given id's week still running, and its
// Fable week, rising today from where they began it, from 08:00, to a
// hundredth short of the first level given by 13:00, slowly as a week rises,
// then to the levels given, the Fable week to fable at the last of them.
func (t *todayOf) week(id string, fable float64, to ...level) {
	week, fabled, used, fableUsed := t.worlds.year.soFar(id)
	t.path(week, level{t.at(8, 0, 0), hundredths(used)}, slices.Insert(to, 0, level{t.at(13, 0, 0), to[0].use - 0.01})...)
	if fable > hundredths(fableUsed) {
		t.read.at(fabled, to[len(to)-1].at, fable)
	}
}

// history is the world's readings history: the year's to today, of the
// accounts given, and today's.
func (t *todayOf) history(accounts config.Accounts) history {
	return slices.Concat(t.worlds.past, t.read.history()).of(accounts.IDs())
}

// today begins taking in today's readings of a world's accounts.
func (a accountWorlds) today() *todayOf {
	return &todayOf{worlds: a}
}

// three takes in today's readings of the three as the auto world has them,
// but personal's, which open gives: work busy until it reached its cap, 95%
// of its 5-hour window, at 14:31:03, its window primed at 12:10, its week at
// 34% then; side's window primed at 13:50, quiet until it took work's
// sessions at 14:31, at 13% now; its week at 78%.
func (t *todayOf) three(open bool) *todayOf {
	t.session("work", t.at(7, 5, 0), true, level{t.at(11, 35, 0), .62})
	t.session("work", t.at(12, 10, 0), true, level{t.at(14, 31, 3), .95})
	t.week("work", .12, level{t.at(14, 31, 3), .34})
	if open {
		t.personal()
	}
	t.session("side", t.at(0, 31, 0), false, level{t.at(0, 41, 0), .02})
	t.session("side", t.at(8, 47, 0), true, level{t.at(12, 47, 0), .20})
	t.session("side", t.at(13, 50, 0), true, level{t.at(14, 10, 0), .03}, level{t.at(14, 31, 0), .03}, level{t.at(14, 41, 30), .13})
	t.week("side", .04, level{t.at(14, 40, 0), .78})
	return t
}

// personal takes in personal's readings today as the auto world has them:
// steady, its 5-hour window primed at 11:30 at 39% now; its week read at 65%
// at 13:32:41 and at 66% at 14:20, a rate that has it run out on Saturday at
// 06:00, before it resets on Sunday at 02:00.
func (t *todayOf) personal() {
	t.session("personal", t.at(6, 30, 0), false, level{t.at(11, 10, 0), .18})
	t.session("personal", t.at(11, 30, 0), true, level{t.at(14, 40, 0), .39})
	t.week("personal", 0, level{t.at(13, 32, 41), .65}, level{t.at(14, 20, 0), .66})
}

// limited takes in personal's readings today as a world where it reached its
// 5-hour limit has them: its window primed at 10:54, at its limit at 14:12
// till it resets at 15:54; its week as the auto world has it.
func (t *todayOf) limited() *todayOf {
	t.session("personal", t.at(5, 50, 0), false, level{t.at(10, 30, 0), .15})
	t.session("personal", t.at(10, 54, 0), true, level{t.at(14, 12, 0), 1})
	t.week("personal", 0, level{t.at(13, 32, 41), .65}, level{t.at(14, 20, 0), .66})
	return t
}

// autoSessions are how many sessions the router sent each account in the last
// hour, as the auto world's frames count them: the six on personal and side,
// and the eight more on the accounts of the frames of ten.
var autoSessions = map[string]int{"personal": 2, "side": 4, "client": 3, "lab": 2, "extra": 1, "studio": 2}

// framesChoice is where new sessions go in every frame drawn in auto, which
// marks side ▲. It's the frames' choice, not the scoring's: by the design's
// scoring the router would choose personal, whose week resets sooner with
// more left, or, of six or ten accounts, spare.
const framesChoice = "side"

// auto is the world of most frames, routing on its merits (DASHBOARD-
// OVERVIEW-SPEC.md, "The sample data"): work at its cap till 17:10;
// personal, the primary, open; side open, where new sessions go. Today's
// traffic is the sessions' world's.
func (a accountWorlds) auto() accountWorld {
	accounts := firstAccounts(3)
	return accountWorld{
		accounts: accounts, history: a.today().three(true).history(accounts), sessions: autoSessions, best: framesChoice, traffic: true,
	}
}

// pinned is the auto world's accounts, read as it reads them, with new
// sessions pinned to personal since 14:02, three of the sessions on it.
func (a accountWorlds) pinned() accountWorld {
	accounts := firstAccounts(3)
	return accountWorld{
		accounts: accounts, history: a.today().three(true).history(accounts),
		pin:      status.Pin{Accounts: []string{"personal"}, Since: dayAt(a.now, today, 14, 2, 0).UTC()},
		sessions: map[string]int{"personal": 3, "side": 3},
	}
}

// limit is routing's world of a limit: the auto world with personal at its
// 5-hour limit since 14:12 till 15:54, its sessions moved to side.
func (a accountWorlds) limit() accountWorld {
	accounts := firstAccounts(3)
	return accountWorld{
		accounts: accounts, history: a.today().three(false).limited().history(accounts),
		limits:   map[string]status.Limit{"personal": personalLimited(a.now)},
		sessions: map[string]int{"side": 6},
	}
}

// personalLimited is personal's limit, reached at 14:12, till 15:54.
func personalLimited(now time.Time) status.Limit {
	return status.Limit{ID: personalLimit, Windows: []string{sessionKey}, Until: dayAt(now, today, 15, 54, 0).UTC()}
}

// badHour is the world of a bad hour: new sessions pinned to personal and
// side since 13:40; personal at its 5-hour limit since 14:12 till 15:54; and
// work's token refused, with a 401, since 14:20, nothing read of it since,
// its 5-hour window having reached its cap at 14:15. Every session is on
// side.
func (a accountWorlds) badHour() accountWorld {
	accounts := firstAccounts(3)
	t := a.today()
	t.session("work", t.at(7, 5, 0), true, level{t.at(11, 35, 0), .62})
	t.session("work", t.at(12, 10, 0), true, level{t.at(14, 15, 0), .95})
	t.week("work", .12, level{t.at(14, 15, 0), .34})
	t.limited()
	t.session("side", t.at(0, 31, 0), false, level{t.at(0, 41, 0), .02})
	t.session("side", t.at(8, 47, 0), true, level{t.at(12, 47, 0), .20})
	t.session("side", t.at(13, 50, 0), true, level{t.at(14, 10, 0), .03}, level{t.at(14, 31, 0), .03}, level{t.at(14, 41, 30), .13})
	t.week("side", .04, level{t.at(14, 40, 0), .78})
	refused := dayAt(a.now, today, 14, 41, 0)
	return accountWorld{
		accounts: accounts, history: t.history(accounts),
		pin:      status.Pin{Accounts: []string{"personal", "side"}, Since: dayAt(a.now, today, 13, 40, 0).UTC()},
		limits:   map[string]status.Limit{"personal": personalLimited(a.now)},
		refused:  map[string]status.Refusal{"work": {Until: refused.Add(10 * time.Minute).UTC(), Status: 401}},
		stale:    map[string]time.Time{"work": dayAt(a.now, today, 14, 20, 0)},
		sessions: map[string]int{"side": 6},
	}
}

// many is the world of the frames of six or ten accounts: the auto world's
// three and the first n−3 of the seven more (dash3.EXTRA): client, spare,
// lab, under pressure, running out about 16:05, team, at its 5-hour limit
// since 13:58 till 15:30, extra, ops and studio. Today's traffic is the
// auto world's, the seven having none.
func (a accountWorlds) many(n int) accountWorld {
	accounts := tenAccounts[:n]
	t := a.today().three(true)
	t.session("client", t.at(7, 35, 0), false, level{t.at(11, 35, 0), .30})
	t.session("client", t.at(12, 40, 0), false, level{t.at(14, 40, 0), .52})
	t.week("client", .08, level{t.at(14, 39, 51), .47})
	t.session("spare", t.at(14, 35, 0), false, level{t.at(14, 41, 0), .04})
	t.week("spare", 0, level{t.at(14, 41, 0), .05})
	t.session("lab", t.at(6, 50, 0), false, level{t.at(10, 50, 0), .40})
	t.session("lab", t.at(11, 55, 0), false, level{t.at(14, 10, 24), .60}, level{t.at(14, 41, 0), .71})
	t.week("lab", .21, level{t.at(14, 41, 1), .58})
	t.session("team", t.at(5, 25, 0), false, level{t.at(9, 25, 0), .50})
	t.session("team", t.at(10, 30, 0), false, level{t.at(13, 58, 0), 1})
	t.week("team", .30, level{t.at(13, 58, 0), .83})
	t.session("extra", t.at(7, 50, 0), false, level{t.at(11, 50, 0), .20})
	t.session("extra", t.at(12, 55, 0), false, level{t.at(14, 20, 0), .22})
	t.week("extra", 0, level{t.at(14, 20, 0), .12})
	t.session("ops", t.at(14, 10, 0), false, level{t.at(14, 40, 0), .06})
	t.week("ops", .02, level{t.at(14, 40, 0), .29})
	t.session("studio", t.at(6, 15, 0), false, level{t.at(10, 15, 0), .30})
	t.session("studio", t.at(11, 20, 0), false, level{t.at(14, 41, 59), .33})
	t.week("studio", .10, level{t.at(14, 41, 59), .41})
	w := accountWorld{accounts: accounts, history: t.history(accounts), sessions: autoSessions, best: framesChoice, traffic: true}
	if n > slices.IndexFunc(tenAccounts, func(a config.Account) bool { return a.ID == "team" }) {
		w.limits = map[string]status.Limit{"team": {ID: 1, Windows: []string{sessionKey}, Until: dayAt(a.now, today, 15, 30, 0).UTC()}}
	}
	return w
}

// one is work alone, where new sessions go, under pressure: its 5-hour
// window, primed at 12:10, read at 44% at 14:10:30 and at 58% at 14:41, a
// rate that has it reach its cap at about 16:05, before it resets at 17:10;
// its week at 34%; and nothing used of its Fable week this week. Five
// sessions are on it.
func (a accountWorlds) one() accountWorld {
	accounts := firstAccounts(1)
	t := a.today()
	t.session("work", t.at(7, 5, 0), true, level{t.at(11, 35, 0), .62})
	t.session("work", t.at(12, 10, 0), true, level{t.at(14, 10, 30), .44}, level{t.at(14, 41, 0), .58})
	t.week("work", 0, level{t.at(14, 40, 0), .34})
	week, _, _, _ := a.year.soFar("work")
	h := slices.DeleteFunc(t.history(accounts), func(r readings.Reading) bool {
		return r.Key == modelKey && r.ResetsAt.Equal(week.resets.UTC()) && r.Utilization > 0
	})
	return accountWorld{accounts: accounts, history: h, sessions: map[string]int{"work": 5}}
}
