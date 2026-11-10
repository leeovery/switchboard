package status_test

import (
	"cmp"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// heading is where a window is heading at a moment, as its WindowNow says it.
type heading struct {
	kind status.WindowKind
	// projected is its heading's kind, and atReset the use it heads for by
	// its reset, where it's on pace.
	projected score.Kind
	atReset   float64
	// out is when it runs out, zero where it doesn't, at its reserve where
	// reserve is set.
	out     time.Time
	reserve bool
	// recent is set where it heads at its recent rate.
	recent bool
}

// headingOf is where the window w is heading, as heading has it.
func headingOf(w status.WindowNow) heading {
	return heading{
		kind: w.Kind, projected: w.Out.Kind, atReset: w.Out.AtReset, out: w.Out.At, reserve: w.Out.Reserve, recent: w.Out.Recent,
	}
}

// near reports whether h is where want is, its run-out within a time and its
// use at its reset within a share of it.
func (h heading) near(want heading, within time.Duration, share float64) bool {
	return h.kind == want.kind && h.projected == want.projected && math.Abs(h.atReset-want.atReset) <= share &&
		h.out.IsZero() == want.out.IsZero() && h.out.Sub(want.out).Abs() <= within && h.reserve == want.reserve && h.recent == want.recent
}

// runsOutIn is a window running out so long after the document was built,
// at its limit.
func runsOutIn(after time.Duration) heading {
	return heading{kind: status.WindowRunsOut, projected: score.RunsOut, out: built.Add(after)}
}

// onPaceFor is a window open to its reset, on pace for the use given then.
func onPaceFor(atReset float64) heading {
	return heading{kind: status.WindowOpen, projected: score.OnPace, atReset: atReset}
}

// forecasting is doc with the pool's use at the day rate given, in Pro weeks
// an hour, as the router measures it, and the hold curve given.
func forecasting(doc status.Document, day float64, hold ...status.HoldPoint) status.Document {
	doc.Demand = &status.Demand{Day: day, Hours: 24, Hold: hold}
	return doc
}

// holdingAll is a hold curve that keeps all of an account's fast rate on it
// for its 48 hours.
var holdingAll = []status.HoldPoint{{Hours: 1, Share: 1}, {Hours: 3, Share: 1}, {Hours: 6, Share: 1}, {Hours: 12, Share: 1}, {Hours: 24, Share: 1}, {Hours: 48, Share: 1}}

// holdingNone is a hold curve that keeps none of an account's fast rate on it
// from an hour on.
var holdingNone = []status.HoldPoint{{Hours: 1}, {Hours: 3}, {Hours: 6}, {Hours: 12}, {Hours: 24}, {Hours: 48}}

// fastAt has the router see the account's window with the given key rise at
// the fast rate given, a share of it an hour.
func fastAt(key string, rate float64) func(*status.Account) {
	return func(a *status.Account) {
		a.Rates = append(slices.Clip(a.Rates), status.Rate{Window: key, Fast: rate})
	}
}

// recentAt has the router see the account's window with the given key used
// at the rate given over the last half hour.
func recentAt(key string, rate float64) func(*status.Account) {
	return func(a *status.Account) {
		a.Rates = append(slices.Clip(a.Rates), status.Rate{Window: key, Rate: rate, Since: built.Add(-30 * time.Minute)})
	}
}

// on is the account a as each change leaves it.
func on(a status.Account, changes ...func(*status.Account)) status.Account {
	for _, change := range changes {
		change(&a)
	}
	return a
}

// working is work, with a usable token, read as the document was built: its
// session 40% used two hours in, its week half used four days in, and
// Fable's week 30% used four days in.
func working(changes ...func(*status.Account)) status.Account {
	return on(readAt("work", windowIn("5h", 0.4, 3*time.Hour), windowIn("7d", 0.5, 3*day), windowIn("7d_oi", 0.3, 3*day)), changes...)
}

// TestAWindowHeadsAsItsDocumentSays holds each window of one account to where
// it heads: its shared week as the document's forecast plays the pool's use,
// a constant 1% an hour where the router's day and fast rates are both that,
// from its half used to its floor; else at the pace its use since it started
// sets, which ends the week at 87.5%; the session at its recent rate.
func TestAWindowHeadsAsItsDocumentSays(t *testing.T) {
	tests := []struct {
		name string
		doc  status.Document
		key  string
		want heading
	}{
		{
			name: "the week, as the forecast plays it, running out at its limit",
			doc:  forecasting(routerDocOf(working(fastAt("7d", 0.01))), 0.01),
			key:  "7d",
			want: runsOutIn(50 * time.Hour),
		},
		{
			name: "the week heading short of its limit, on pace for its use at its reset as the forecast plays it",
			doc:  forecasting(routerDocOf(working(fastAt("7d", 0.002))), 0.002),
			key:  "7d",
			want: onPaceFor(0.5 + 0.002*72),
		},
		{
			name: "the week reaching its reserve, which holds it back",
			doc:  forecasting(routerDocOf(working(fastAt("7d", 0.01), reserving)), 0.01),
			key:  "7d",
			want: heading{kind: status.WindowRunsOut, projected: score.RunsOut, out: built.Add(40 * time.Hour), reserve: true},
		},
		{
			name: "the week reaching its limit, the global pin spending its reserve",
			doc:  forecasting(pinnedTo(routerDocOf(working(fastAt("7d", 0.01), reserving)), "work"), 0.01),
			key:  "7d",
			want: runsOutIn(50 * time.Hour),
		},
		{
			name: "the week read spent, exhausted, whatever the forecast",
			doc: forecasting(routerDocOf(on(readAt("work", windowIn("5h", 0.4, 3*time.Hour), spentIn("7d", 1, 3*day)), fastAt("7d", 0.01))),
				0.01),
			key:  "7d",
			want: heading{kind: status.WindowLimited, projected: score.Exhausted},
		},
		{
			name: "the week of a document without demand, at its use since it started, though its recent rate runs it out sooner",
			doc:  routerDocOf(working(recentAt("7d", 0.05))),
			key:  "7d",
			want: onPaceFor(0.875),
		},
		{
			name: "the week of a document without demand, nowhere said before 5% of it has passed",
			doc:  routerDocOf(readAt("work", windowIn("5h", 0.4, 3*time.Hour), windowIn("7d", 0.01, 7*day-4*time.Hour))),
			key:  "7d",
			want: heading{kind: status.WindowOpen},
		},
		{
			name: "Fable's week, at its use since it started, though its rates run it out sooner",
			doc:  forecasting(routerDocOf(working(fastAt("7d", 0.01), recentAt("7d_oi", 0.05), fastAt("7d_oi", 0.05))), 0.01),
			key:  "7d_oi",
			want: onPaceFor(0.3 + 0.3*72/96),
		},
		{
			name: "the session, at its recent rate, in a document with demand",
			doc:  forecasting(routerDocOf(working(recentAt("5h", 0.3), fastAt("7d", 0.01))), 0.01),
			key:  "5h",
			want: heading{kind: status.WindowRunsOut, projected: score.RunsOut, out: built.Add(2 * time.Hour), recent: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, doc := range []status.Document{tt.doc, tt.doc.WorkedOut(policy)} {
				a, _ := doc.At(built, policy).Account("work")
				w, _ := a.Window(tt.key)
				if got := headingOf(w); !got.near(tt.want, time.Second, 1e-9) {
					t.Errorf("work's %s heads %+v, want %+v", tt.key, got, tt.want)
				}
			}
		})
	}
}

// TestTheForecastPlaysThePoolsUseAsTheDocumentHasIt holds what the forecast
// plays to the document: two accounts, each with its week, resetting in three
// days, the pool's use 1% of a Pro week an hour but where it says, and where
// each heads.
func TestTheForecastPlaysThePoolsUseAsTheDocumentHasIt(t *testing.T) {
	tests := []struct {
		name string
		doc  status.Document
		want map[string]heading
	}{
		{
			// side, its week 60% used, holds its own use, all of it, till it
			// runs out in 40 hours; then the pool's use goes to work, which
			// takes its 32 hours to its reset.
			name: "an account holds its own use as the hold curve keeps it, and the pool's use goes on once it runs out",
			doc: forecasting(routerDocOf(
				readAt("work", windowIn("7d", 0.5, 3*day)),
				on(readAt("side", windowIn("7d", 0.6, 3*day)), fastAt("7d", 0.01)),
			), 0.01, holdingAll...),
			want: map[string]heading{"work": onPaceFor(0.82), "side": runsOutIn(40 * time.Hour)},
		},
		{
			// side takes all of the pool's use work doesn't hold, which falls
			// straight from all of work's fast rate to none an hour on: 1% an
			// hour, less half a percent all told, from its half used.
			name: "the pool's use that isn't held goes to the global pin's account",
			doc: forecasting(pinnedTo(routerDocOf(
				on(readAt("work", windowIn("7d", 0.5, 3*day)), fastAt("7d", 0.01)),
				readAt("side", windowIn("7d", 0.5, 3*day)),
			), "side"), 0.01, holdingNone...),
			want: map[string]heading{"side": runsOutIn(50*time.Hour + 30*time.Minute)},
		},
		{
			name: "an account whose token is refused takes no new sessions",
			doc: forecasting(routerDocOf(
				on(readAt("work", windowIn("7d", 0.5, 3*day)), fastAt("7d", 0.01)),
				on(readAt("side", windowIn("7d", 0.2, 3*day)), func(a *status.Account) {
					a.Refused = status.Refusal{Until: built.Add(day), Status: 401}
				}),
			), 0.01),
			want: map[string]heading{"work": runsOutIn(50 * time.Hour), "side": {kind: status.WindowStale, projected: score.OnPace, atReset: 0.2}},
		},
		{
			// work's own use, 2% of its week an hour, all the pool's, goes to
			// side from now, which runs out from 10% in 45 hours, where work,
			// holding it, would run out in 25 and side 45 after.
			name: "an account whose token is refused holds none of its use, which goes where a new session would",
			doc: forecasting(routerDocOf(
				on(readAt("work", windowIn("7d", 0.5, 3*day)), fastAt("7d", 0.02), func(a *status.Account) {
					a.Refused = status.Refusal{Until: built.Add(day), Status: 401}
				}),
				readAt("side", windowIn("7d", 0.1, 3*day)),
			), 0.02, holdingAll...),
			want: map[string]heading{"side": runsOutIn(45 * time.Hour), "work": {kind: status.WindowStale, projected: score.OnPace, atReset: 0.5}},
		},
		{
			name: "an account without a usable token takes no new sessions",
			doc: forecasting(routerDocOf(
				on(readAt("work", windowIn("7d", 0.5, 3*day)), fastAt("7d", 0.01)),
				on(readAt("side", windowIn("7d", 0.2, 3*day)), func(a *status.Account) { a.TokenSet = false }),
			), 0.01),
			want: map[string]heading{"work": runsOutIn(50 * time.Hour)},
		},
		{
			// work, an Ultra week, 10 Pro weeks, holds the pool's use, a tenth
			// of a Pro week an hour, at 1% of its own week an hour till it
			// runs out in 40 hours; side, a Pro week, takes it then, at 10% of
			// its own an hour, and runs out in five.
			name: "the pool's use moves between accounts by their plans' sizes",
			doc: forecasting(routerDocOf(
				on(readAt("work", windowIn("7d", 0.6, 3*day)), fastAt("7d", 0.01), planned(config.PlanUltra)),
				on(readAt("side", windowIn("7d", 0.5, 3*day)), planned(config.PlanPro)),
			), 0.1, holdingAll...),
			want: map[string]heading{"work": runsOutIn(40 * time.Hour), "side": runsOutIn(45 * time.Hour)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.doc.WorkedOut(policy).At(built, policy)
			for id, want := range tt.want {
				a, _ := m.Account(id)
				w, _ := a.Window("7d")
				if got := headingOf(w); !got.near(want, time.Minute, 1e-3) {
					t.Errorf("%s's week heads %+v, want %+v", id, got, want)
				}
			}
		})
	}
}

// A week reset since it was read shows no run-out, as it never has, but it
// enters the pool as it stands as the document was built, empty, its next
// reset a week on, and holding its account's use, so the forecast plays the
// fresh week, not the reading, and that shows in where the pool's other
// weeks run out. side, read at 99% two hours before the document was built,
// holds its own use, 2.5% of its week an hour, all of the pool's; work, half
// used, is where the global pin sends what side doesn't hold. Reset an hour
// before the document was built, side's fresh week holds it for 40 hours
// before work takes it and runs out 20 hours later; still to reset, side
// runs out 24 minutes on, and work 20 hours after.
func TestAWeekResetSinceItWasReadIsForecastAfresh(t *testing.T) {
	tests := []struct {
		name       string
		resetsIn   time.Duration
		side, work heading
	}{
		{name: "reset since it was read", resetsIn: -time.Hour, side: heading{kind: status.WindowOpen}, work: runsOutIn(60 * time.Hour)},
		{name: "still to reset", resetsIn: 6 * day, side: runsOutIn(24 * time.Minute), work: runsOutIn(20*time.Hour + 24*time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			side := on(readAt("side", windowIn("7d", 0.99, tt.resetsIn)), fastAt("7d", 0.025), func(a *status.Account) {
				a.FetchedAt = built.Add(-2 * time.Hour)
			})
			doc := pinnedTo(routerDocOf(readAt("work", windowIn("7d", 0.5, 3*day)), side), "work")
			doc = forecasting(doc, 0.025, holdingAll...).WorkedOut(policy)
			m := doc.At(built, policy)
			for id, want := range map[string]heading{"side": tt.side, "work": tt.work} {
				a, _ := m.Account(id)
				w, _ := a.Window("7d")
				if got := headingOf(w); !got.near(want, time.Minute, 1e-3) {
					t.Errorf("%s's week heads %+v, want %+v", id, got, want)
				}
				if id == "side" && w.ResetSinceRead != (tt.resetsIn < 0) {
					t.Errorf("side's week reads reset since it was read %t, want %t", w.ResetSinceRead, tt.resetsIn < 0)
				}
			}
			coming := slices.ContainsFunc(doc.ComingUp, func(u status.Upcoming) bool { return u.Account == "side" && u.Kind == status.UpcomingRunsOut })
			if want := !tt.side.out.IsZero(); coming != want {
				t.Errorf("side running out coming up is %t, want %t: ComingUp = %+v", coming, want, doc.ComingUp)
			}
		})
	}
}

// A week never read enters the pool as nothing read: the forecast has no Out
// of it, and plays a new session to it only where the global pin names it
// and none of the pin's accounts with a week read has room. Else work, its
// week half used, takes the pool's use, 1% of a week an hour as it fades in
// from none, and runs out 53 hours on.
func TestAWeekNeverReadTakesNewSessionsOnlyWhereThePinNamesIt(t *testing.T) {
	unread := readAt("side", windowIn("5h", 0.1, 2*time.Hour))
	tests := []struct {
		name string
		pin  []string
		want heading
	}{
		{name: "nothing pinned", want: runsOutIn(53 * time.Hour)},
		{name: "pinned to the account never read", pin: []string{"side"}, want: onPaceFor(0.5)},
		{name: "pinned to it and to an account read with room", pin: []string{"side", "work"}, want: runsOutIn(53 * time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routerDocOf(readAt("work", windowIn("7d", 0.5, 3*day)), unread)
			if tt.pin != nil {
				doc = pinnedTo(doc, tt.pin...)
			}
			doc = forecasting(doc, 0.01).WorkedOut(policy)

			work, _ := doc.At(built, policy).Account("work")
			w, _ := work.Window("7d")
			if got := headingOf(w); !got.near(tt.want, time.Minute, 1e-3) {
				t.Errorf("work's week heads %+v, want %+v", got, tt.want)
			}
			if got := doc.Project(unread, quota.Window{Key: "7d"}, built, policy); got != (status.Heading{}) {
				t.Errorf("side's week, never read, heads %+v, want nowhere: the forecast has no Out of it", got)
			}
		})
	}
}

// planned has the account on the plan given.
func planned(plan string) func(*status.Account) {
	return func(a *status.Account) { a.Plan = plan }
}

// An account's week is as big as its plan's, in Pro weeks, as the price table
// sizes it, and one whose plan isn't known is as big as the plan most of the
// accounts are on, as History's replay sizes them. Work holds all of its own
// use, half a percent of its week an hour, all of the pool's, which the router
// measures in Pro weeks by the plan's size: its week, half used, heads for
// 74% at its reset two days on, and side, on Ultra, takes nothing. Sized
// otherwise, work would hold more of its week than that, or the pool's use
// would spill onto side.
func TestTheForecastSizesAnAccountByItsPlan(t *testing.T) {
	tests := []struct {
		name, plan string
		size       float64
	}{
		{name: "pro", plan: config.PlanPro, size: 1},
		{name: "max", plan: config.PlanMax, size: 5},
		{name: "ultra", plan: config.PlanUltra, size: 10},
		{name: "not known, as the plan most are on", size: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := forecasting(routerDocOf(
				on(readAt("work", windowIn("7d", 0.5, 2*day)), fastAt("7d", 0.005), planned(tt.plan)),
				on(readAt("side", windowIn("7d", 0.2, 2*day)), planned(config.PlanUltra)),
			), tt.size*0.005, holdingAll...)
			m := doc.WorkedOut(policy).At(built, policy)
			for id, want := range map[string]heading{"work": onPaceFor(0.74), "side": onPaceFor(0.2)} {
				a, _ := m.Account(id)
				w, _ := a.Window("7d")
				if got := headingOf(w); !got.near(want, time.Minute, 1e-6) {
					t.Errorf("%s's week heads %+v, want %+v", id, got, want)
				}
			}
		})
	}
}

// A pool whose accounts are all on one plan heads the same way whatever the
// plan's size, as the router measures the pool's use in Pro weeks by that
// size too. Side, 60% used, holds its own use, 1% of its week an hour, till it
// runs out in 40 hours; work, half used, holds its own, half a percent, and
// takes side's then, running out 20 hours later, before its reset in three
// days.
func TestAOnePlanPoolsForecastIsTheSameWhateverThePlansSize(t *testing.T) {
	for _, plan := range append([]string{""}, config.Plans...) {
		t.Run(cmp.Or(plan, "no plan"), func(t *testing.T) {
			sizes, _ := ledger.Pricing.Sizes([]string{plan, plan})
			doc := forecasting(routerDocOf(
				on(readAt("work", windowIn("7d", 0.5, 3*day)), fastAt("7d", 0.005), planned(plan)),
				on(readAt("side", windowIn("7d", 0.6, 3*day)), fastAt("7d", 0.01), planned(plan)),
			), float64(sizes[0])*0.015, holdingAll...)
			m := doc.WorkedOut(policy).At(built, policy)
			for id, want := range map[string]heading{"work": runsOutIn(60 * time.Hour), "side": runsOutIn(40 * time.Hour)} {
				a, _ := m.Account(id)
				w, _ := a.Window("7d")
				if got := headingOf(w); !got.near(want, time.Minute, 1e-6) {
					t.Errorf("%s's week heads %+v, want %+v", id, got, want)
				}
			}
		})
	}
}

func TestWhatsComingUpHasAWeekRunningOutAsTheForecastHasIt(t *testing.T) {
	doc := forecasting(routerDocOf(on(readAt("work", windowIn("7d", 0.5, 3*day)), fastAt("7d", 0.01))), 0.01).WorkedOut(policy)
	i := slices.IndexFunc(doc.ComingUp, func(u status.Upcoming) bool { return u.Kind == status.UpcomingRunsOut })
	if i < 0 || doc.ComingUp[i].Window != "7d" || doc.ComingUp[i].At.Sub(built.Add(50*time.Hour)).Abs() > time.Second {
		t.Errorf("ComingUp = %+v, want work's week running out 50 hours after the document was built, as the forecast has it", doc.ComingUp)
	}
}

// forecastable is the router's document of three accounts, each with its
// session and its week, the pool's use given.
func forecastable() status.Document {
	return forecasting(routerDocOf(
		working(fastAt("7d", 0.01)),
		on(readAt("personal", windowIn("5h", 0.2, 4*time.Hour), windowIn("7d", 0.4, 2*day)), fastAt("7d", 0.005)),
		readAt("side", windowIn("5h", 0.1, time.Hour), windowIn("7d", 0.1, 5*day)),
	), 0.02)
}

func TestADocumentsForecastIsPlayedOnceHoweverManyWindowsAndMomentsReadIt(t *testing.T) {
	played := status.CountForecasts(t)
	doc := forecastable().WorkedOut(policy)
	m := doc.At(built.Add(time.Minute), policy)
	_ = m.Text()
	_ = m.At(built.Add(time.Hour)).Pool()
	_ = doc.At(built.Add(2*time.Hour), policy)
	_ = doc.HoldAt(built.Add(3*time.Hour), policy).Moment()
	if got := played(); got != 1 {
		t.Errorf("the forecast was played %d times, want once, as the document was worked out", got)
	}
}

func TestADocumentNotWorkedOutPlaysItsForecastAsItsFirstMomentIsWorkedOut(t *testing.T) {
	played := status.CountForecasts(t)
	m := forecastable().At(built, policy)
	_ = m.Text()
	_ = m.At(built.Add(time.Hour)).Text()
	if got := played(); got != 1 {
		t.Errorf("the forecast was played %d times, want once, as the moment was worked out", got)
	}
}

func TestADocumentWithoutDemandPlaysNoForecast(t *testing.T) {
	played := status.CountForecasts(t)
	doc := forecastable()
	doc.Demand = nil
	_ = doc.WorkedOut(policy).At(built, policy).Text()
	if got := played(); got != 0 {
		t.Errorf("the forecast was played %d times, want none: a document without demand has nothing to play", got)
	}
}
