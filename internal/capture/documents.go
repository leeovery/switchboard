package capture

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// accountWorld is a frame's accounts at its moment: as configured; as the
// readings history read them, which its document and its records are both
// made from; and as the router stands with them then, which the readings
// don't hold: its pin, the limits and refusals in force, when it
// last read an account it couldn't read since, how many sessions it sent
// each in the last hour, and where it sends new sessions, where the frames
// say so; the sessions it lists at a moment, as GET /sessions gives them,
// where the world has them; what it has told of since it started, its events,
// oldest first, as it numbers them; what its request stream tells as it's
// opened, where anything's in flight; whether today's traffic of the three,
// the sessions' world's, is the world's, and where its ledger holds lines of
// its own in their place, those, by now; whether its document is read
// fresh, at the moment drawn, as a router read it as an account reached its
// limit then; and, where the worlds keep it, its records, built once, as
// they're first asked for.
type accountWorld struct {
	accounts config.Accounts
	history  history
	pin      status.Pin
	limits   map[string]status.Limit
	refused  map[string]status.Refusal
	stale    map[string]time.Time
	sessions map[string]int
	best     string
	routed   func(now time.Time) []status.Session
	events   []status.Event
	stream   []router.StreamEvent
	traffic  bool
	lines    func(now time.Time) []ledger.Line
	fresh    bool
	records  func() *records
}

// document is the router's status document of the world at now, read as
// read has it, as the router builds it and serves it, with what's worked out
// of it: each account's windows as last read, standing as they do then;
// how fast each has been used lately, as a router that took up the readings
// history judges it, and the pressure that puts the account under; the
// accounts pinned spending their reserves; and the best account to use next,
// but where the world says where new sessions go; and the newest of its
// events, by then.
func (w accountWorld) document(now time.Time) status.Document {
	at := w.read(now)
	doc := status.Document{
		GeneratedAt: at.UTC(), Source: status.SourceRouter, Pin: w.pin, Router: status.Health{Healthy: true}, Events: toldAt(w.events, at),
	}
	var open []status.Account
	held := w.history.byAccount()
	for _, a := range w.accounts {
		st := w.account(a, held[a.ID], at)
		doc.Accounts = append(doc.Accounts, st)
		doc.Sessions += st.Sessions
		if !st.Shut(at, claude.Policy()) {
			open = append(open, st)
		}
	}
	doc.Best = cmp.Or(w.best, status.Best(claude.Policy(), open, w.pin.Accounts, at))
	doc.Primary = status.PrimaryOf(doc.Accounts)
	return doc.WorkedOut(claude.Policy())
}

// extraOff is extra usage as the provider gives it of an account that has
// none turned on: rejected, as every account of the frames reads it.
var extraOff = quota.ExtraUsage{Status: quota.StatusRejected}

// account is the account a's status at at, its readings history h, as the
// router gives it: as last read at at, or, where the world has the router
// unable to read it since a time, then, its rates and pressure worked out
// from its readings to then, and its extra usage off.
func (w accountWorld) account(a config.Account, h history, at time.Time) status.Account {
	read := w.readAt(a.ID, at)
	st := status.Configured(a)
	st.TokenSet, st.FetchedAt = true, read.UTC()
	held := h.before(read)
	latest := held.windows(claude.WindowLabel)
	st.Windows, st.Extra = latest, extraOff
	st = st.AsOf(claude.Policy(), at)
	st.Limit, st.Refused, st.Sessions = w.limits[a.ID], w.refused[a.ID], w.sessions[a.ID]
	trails := held.trails()
	st.Rates = status.RatesOf(latest, trails, at)
	st.Pressure = pressureOf(st, latest, trails, w.pin.Has(a.ID), at)
	return st
}

// read is when the router last read the world, as of now: readAgo before,
// or, where it's read fresh, at now.
func (w accountWorld) read(now time.Time) time.Time {
	if w.fresh {
		return now
	}
	return now.Add(-readAgo)
}

// readAt is when the router last read the account with the given id, as of
// at: then, or where the world has it unable to read the account since a
// time, that time.
func (w accountWorld) readAt(id string, at time.Time) time.Time {
	if stale, ok := w.stale[id]; ok {
		return stale
	}
	return at
}

// trails are each account's windows as the router holds them at now, as it
// answers GET /history from the readings it took of them: each window as the
// document doc reads it, and its readings taken to when the account was last
// read.
func (w accountWorld) trails(doc status.Document, now time.Time) []trail {
	held := w.history.byAccount()
	var trails []trail
	for _, a := range doc.Accounts {
		read := held[a.ID].before(w.readAt(a.ID, w.read(now)))
		for _, window := range a.Windows {
			t := trail{account: a.ID, window: window}
			for _, r := range read {
				if r.Key == window.Key {
					t.read = append(t.read, r)
				}
			}
			trails = append(trails, t)
		}
	}
	return trails
}

// trails are how each of the windows of the account whose readings history h
// is has been read, by the window's key, as a router that took up that
// history as it started holds them: each reading taken in, in turn, as
// score.Noted takes it.
func (h history) trails() map[string][]score.Reading {
	trails := make(map[string][]score.Reading)
	last := make(map[string]quota.Window)
	for _, r := range h {
		kept := r.Window()
		trails[r.Key], last[r.Key] = score.Noted(trails[r.Key], last[r.Key], kept, r.At), kept
	}
	return trails
}

// pressureOf is how the account st's pressure window stands at at, its windows
// as last read latest, as the router gives it, as status.PressureFrom says: at
// its pace over its trail, and, pinned, spending its reserve.
func pressureOf(st status.Account, latest []quota.Window, trails map[string][]score.Reading, pinned bool, at time.Time) status.Pressure {
	return status.PressureFrom(claude.Policy(), status.Pressing{
		ID: st.ID, Reserve: st.Reserve, Latest: latest, Trails: trails, Spent: pinned, Shut: st.Shut(at, claude.Policy()),
	}, at)
}

// --------------------------------------------------------- the worlds

// accountWorlds are the accounts' worlds at a moment, over the one year of
// their use the frames draw: each a frame's, the year's readings history to
// today's start shared, today's as each world has it; today's traffic of the
// three, the sessions' world's lines and the router's events, made once for
// the worlds that hold it; and each world they've built, kept.
type accountWorlds struct {
	now     time.Time
	year    year
	past    history
	traffic func() ([]filed, []events.Line)
	built   *builtWorlds
}

// builtWorlds are the worlds built, by name, each built once, as it's first
// asked for.
type builtWorlds struct {
	mu     sync.Mutex
	worlds map[string]func() accountWorld
}

// newAccountWorlds are the accounts' worlds at now.
func newAccountWorlds(now time.Time) accountWorlds {
	y := newYear(now)
	traffic := sync.OnceValues(func() ([]filed, []events.Line) { return filedOf(heldAsWritten(sessionsLines(now))), routerEvents(now) })
	built := &builtWorlds{worlds: make(map[string]func() accountWorld)}
	return accountWorlds{now: now, year: y, past: y.readings().history(), traffic: traffic, built: built}
}

// kept is the world with the given name, as build builds it, built once, as
// it's first asked for, and its records with it, once they're first asked
// for. Its accounts are each caller's own, so a caller changing them changes
// no other's.
func (a accountWorlds) kept(name string, build func() accountWorld) accountWorld {
	a.built.mu.Lock()
	world, ok := a.built.worlds[name]
	if !ok {
		world = sync.OnceValue(func() accountWorld {
			w := build()
			w.records = sync.OnceValue(func() *records { return a.recordsOf(w) })
			return w
		})
		a.built.worlds[name] = world
	}
	a.built.mu.Unlock()
	w := world()
	w.accounts = slices.Clone(w.accounts)
	return w
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
// but personal's, which open gives, as work, personal and side have them.
func (t *todayOf) three(open bool) *todayOf {
	t.work()
	if open {
		t.personal()
	}
	t.side()
	return t
}

// work takes in work's readings today as the auto world has them: busy until
// it reached its cap, 95% of its 5-hour window, at 14:31:03, its window
// primed at 12:10, its week at 34% then.
func (t *todayOf) work() {
	t.session("work", t.at(7, 5, 0), true, level{t.at(11, 35, 0), .62})
	t.session("work", t.at(12, 10, 0), true, level{t.at(14, 31, 3), .95})
	t.week("work", .12, level{t.at(14, 31, 3), .34})
}

// personal takes in personal's readings today as the auto world has them:
// steady, its 5-hour window primed at 11:30 at 39% now, rising on to the
// levels given after; its week read at 65% at 13:32:41 and at 66% at 14:20,
// a rate that has it run out on Saturday at 06:00, before it resets on
// Sunday at 02:00.
func (t *todayOf) personal(after ...level) {
	t.session("personal", t.at(6, 30, 0), false, level{t.at(11, 10, 0), .18})
	t.session("personal", t.at(11, 30, 0), true, slices.Concat([]level{{t.at(14, 40, 0), .39}}, after)...)
	t.week("personal", 0, level{t.at(13, 32, 41), .65}, level{t.at(14, 20, 0), .66})
}

// side takes in side's readings today as the auto world has them: its
// window primed at 13:50, quiet until it took work's sessions at 14:31, at
// 13% now, rising on to the levels given after; its week at 78%.
func (t *todayOf) side(after ...level) {
	t.session("side", t.at(0, 31, 0), false, level{t.at(0, 41, 0), .02})
	t.session("side", t.at(8, 47, 0), true, level{t.at(12, 47, 0), .20})
	t.session("side", t.at(13, 50, 0), true, slices.Concat(
		[]level{{t.at(14, 10, 0), .03}, {t.at(14, 31, 0), .03}, {t.at(14, 41, 30), .13}}, after,
	)...)
	t.week("side", .04, level{t.at(14, 40, 0), .78})
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
// personal, the primary, open; side open, where new sessions go. The router
// lists the Overview's six sessions, three with a request in flight, and
// tells of its day as the Log does, and today's traffic is the sessions'
// world's.
func (a accountWorlds) auto() accountWorld {
	return a.kept("auto", func() accountWorld {
		accounts := firstAccounts(3)
		return accountWorld{
			accounts: accounts, history: a.today().three(true).history(accounts), sessions: autoSessions, best: framesChoice,
			routed: routedSessions, events: autoEvents(a.now), stream: inFlight(a.now), traffic: true,
		}
	})
}

// sidesPlanUnknown is the auto world with side on no plan the config names,
// so every account's worth against the plans is over work's and personal's
// alone, 2 of the 3.
func (a accountWorlds) sidesPlanUnknown() accountWorld {
	w := a.auto()
	w.accounts[2].Plan = ""
	return w
}

// pinned is the auto world's accounts, read as it reads them, with new
// sessions pinned to personal since 14:02, three of the sessions on it, the
// router telling of it as happenings' pinnedToPersonal has it.
func (a accountWorlds) pinned() accountWorld {
	return a.kept("pinned", func() accountWorld {
		accounts := firstAccounts(3)
		return accountWorld{
			accounts: accounts,
			history:  a.today().three(true).history(accounts),
			pin:      status.Pin{Accounts: []string{"personal"}, Since: dayAt(a.now, today, 14, 2, 0).UTC()},
			sessions: map[string]int{"personal": 3, "side": 3},
			events:   ofTheRun(happenings{a.now}.pinnedToPersonal()),
		}
	})
}

// limit is routing's world of a limit: the auto world with personal at its
// 5-hour limit since 14:12 till 15:54, its sessions moved to side, where the
// router lists all six, telling of it as happenings' atPersonalsLimit has it.
func (a accountWorlds) limit() accountWorld {
	return a.kept("limit", func() accountWorld {
		accounts := firstAccounts(3)
		return accountWorld{
			accounts: accounts,
			history:  a.today().three(false).limited().history(accounts),
			limits:   map[string]status.Limit{"personal": personalLimited(a.now)},
			sessions: map[string]int{"side": 6},
			routed:   routedPastPersonalsLimit,
			events:   ofTheRun(happenings{a.now}.atPersonalsLimit()),
		}
	})
}

// personalLimited is personal's limit, reached at 14:12, till 15:54.
func personalLimited(now time.Time) status.Limit {
	return status.Limit{ID: todaysLimit, Windows: []string{sessionKey}, Until: dayAt(now, today, 15, 54, 0).UTC()}
}

// badHour is the world of a bad hour: new sessions pinned to personal and
// side since 13:40; personal at its 5-hour limit since 14:12 till 15:54; and
// work's token refused, with a 401, since 14:20, nothing read of it since,
// its 5-hour window having reached its cap at 14:15. Every session is on
// side. The router tells of it as happenings' badHour has it.
func (a accountWorlds) badHour() accountWorld {
	return a.kept("badHour", func() accountWorld {
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
		refused := status.Refusal{Until: dayAt(a.now, today, 14, 51, 0).UTC(), Status: 401}
		return accountWorld{
			accounts: accounts,
			history:  t.history(accounts),
			pin:      status.Pin{Accounts: []string{"personal", "side"}, Since: dayAt(a.now, today, 13, 40, 0).UTC()},
			limits:   map[string]status.Limit{"personal": personalLimited(a.now)},
			refused:  map[string]status.Refusal{"work": refused},
			stale:    map[string]time.Time{"work": dayAt(a.now, today, 14, 20, 0)},
			sessions: map[string]int{"side": 6},
			events:   ofTheRun(happenings{a.now}.badHour(refused)),
		}
	})
}

// many is the world of the frames of six or ten accounts: the auto world's
// three and the first n−3 of the seven more (dash3.EXTRA): client, its
// 5-hour window slowing since 14:10, so it has room till it resets at 17:40;
// spare; lab, under pressure, running out about 16:05; team, at its 5-hour
// limit since 13:58 till 15:30; extra, ops and studio. Today's traffic, and the
// router's events, are the auto world's, the seven having none.
func (a accountWorlds) many(n int) accountWorld {
	return a.kept(fmt.Sprint("many ", n), func() accountWorld {
		accounts := firstAccounts(n)
		t := a.today().three(true)
		t.session("client", t.at(7, 35, 0), false, level{t.at(11, 35, 0), .30})
		t.session("client", t.at(12, 40, 0), false, level{t.at(14, 10, 0), .48}, level{t.at(14, 40, 0), .52})
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
		w := accountWorld{accounts: accounts, history: t.history(accounts), sessions: autoSessions, best: framesChoice, events: autoEvents(a.now), traffic: true}
		if n > slices.IndexFunc(tenAccounts, func(a config.Account) bool { return a.ID == "team" }) {
			w.limits = map[string]status.Limit{"team": {ID: todaysLimit, Windows: []string{sessionKey}, Until: dayAt(a.now, today, 15, 30, 0).UTC()}}
		}
		return w
	})
}

// one is work alone, where new sessions go, under pressure: its 5-hour
// window, primed at 12:10, read at 44% at 14:10:30 and at 58% at 14:41, a
// rate that has it reach its cap at about 16:05, before it resets at 17:10;
// its week at 34%; and nothing used of its Fable week this week. Five
// sessions are on it.
func (a accountWorlds) one() accountWorld {
	return a.kept("one", func() accountWorld {
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
	})
}

// alone is work alone as usage's frame of it counts its sessions: one's
// world, with six on work, where the dashboard's frames of work alone draw
// five, or three.
func (a accountWorlds) alone() accountWorld {
	w := a.one()
	w.sessions = map[string]int{"work": 6}
	return w
}

// oneLately is work alone as Accounts' frame of it has it: one's world, with
// three sessions on work, which the router lists none of, as the frame draws
// no SESSIONS; and the router telling of its afternoon: its 5-hour window
// primed at 12:10, 7f3a started on it at 14:12:30, and its coming under
// pressure at 14:38, as its document has it running out then.
func (a accountWorlds) oneLately() accountWorld {
	return a.kept("oneLately", func() accountWorld {
		w, h := a.one(), happenings{a.now}
		pressed := h.today(14, 38, 0)
		w.sessions = map[string]int{"work": 3}
		w.events = ofTheRun([]status.Event{
			h.primed(h.today(12, 10, 0), "work", h.today(17, 10, 0)),
			h.started(h.today(14, 12, 30), id7F3A, "work", status.ReasonNew),
			{
				Kind: status.EventPressure, At: pressed, Account: "work", Windows: []string{fiveHourKey},
				Until: w.document(a.now).Accounts[0].Pressure.RunsOut, Since: pressed.Add(-score.Recent),
			},
		})
		return w
	})
}
