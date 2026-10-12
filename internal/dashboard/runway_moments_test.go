package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// runwayMoment is a document the router, or probing, builds, judged at a
// moment views have got it wrong at before, as status's hard moments have
// them.
type runwayMoment struct {
	name string
	doc  status.Document
	now  time.Time
}

// runwayMoments are status's hard moments: the documents the router, or
// probing, builds at gen, through AsOf, and when each is judged.
func runwayMoments() []runwayMoment {
	gen := now.Add(-time.Hour).UTC()
	labels := map[string]string{"5h": "Session", "7d": "Week"}
	in := func(key string, used float64, resetsIn time.Duration) quota.Window {
		return quota.Window{Key: key, Label: labels[key], Utilization: used, ResetsAt: gen.Add(resetsIn)}
	}
	spent := func(key string, resetsIn time.Duration) quota.Window {
		w := in(key, 1, resetsIn)
		w.Status = quota.StatusRejected
		return w
	}
	read := func(id string, windows ...quota.Window) status.Account {
		return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: gen, Windows: windows}
	}
	side := read("side", in("5h", 0.1, 4*time.Hour), in("7d", 0.2, 5*day))

	failed := read("work", in("5h", 0.7, 2*time.Hour), in("7d", 0.4, 3*day))
	failed.FetchedAt, failed.Error = gen.Add(-20*time.Minute), "HTTP 529 · Overloaded"
	refused := read("work", in("5h", 0.4, 2*time.Hour), in("7d", 0.5, 2*day))
	refused.Refused = status.Refusal{Until: gen.Add(time.Hour), Status: 401}
	lapsed := read("work", in("5h", 0.5, -time.Hour), in("7d", 0.3, 3*day))
	lapsed.FetchedAt = gen.Add(-6 * time.Hour)
	reserved := read("work", in("5h", 0.3, 3*time.Hour), in("7d", 0.92, time.Hour))
	reserved.Reserve = 0.1
	limited := read("work", spent("5h", 40*time.Minute), in("7d", 0.6, 3*day))
	limited.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: gen.Add(40 * time.Minute)}
	held := routerBuilt(gen, nil, limited, side)
	held.Events = []status.Event{{
		ID: 9, At: gen.Add(-20 * time.Minute), Kind: status.EventLimit, Account: "work",
		Windows: []string{"5h"}, Until: gen.Add(40 * time.Minute), Limit: 4,
	}}
	capped := routerBuilt(gen, nil, reserved, side)

	return []runwayMoment{
		{"just after a reset, before the reread", routerBuilt(gen, nil,
			read("work", in("5h", 0.8, time.Hour), in("7d", 0.5, 2*day)),
			read("side", in("5h", 0.3, 3*time.Hour), in("7d", 0.7, 30*time.Minute)),
		), gen.Add(time.Hour + 5*time.Second)},
		{"a failed read after an earlier one", routerBuilt(gen, map[string]float64{"work": 0.3}, failed, side), gen.Add(5 * time.Minute)},
		{"its token refused", routerBuilt(gen, map[string]float64{"work": 0.4}, refused, side), gen.Add(10 * time.Minute)},
		{"a stale document, built three hours before now", routerBuilt(gen, nil,
			read("work", in("5h", 0.4, 2*time.Hour), in("7d", 0.6, 4*day)),
			read("side", in("5h", 0.1, 4*time.Hour), in("7d", 0.3, 5*day)),
		), gen.Add(3 * time.Hour)},
		{"its session lapsed", routerBuilt(gen, nil, lapsed, side), gen.Add(10 * time.Minute)},
		{"a week at its reserve when built", capped, gen.Add(10 * time.Minute)},
		{"a week at its reserve when built, then reset", capped, gen.Add(time.Hour + time.Minute)},
		{"a limit holding", held, gen.Add(10 * time.Minute)},
		{"a limit holding, then lifted", held, gen.Add(41 * time.Minute)},
		{"probed, of several accounts", probeBuilt(gen,
			read("work", in("5h", 0.3, 3*time.Hour), in("7d", 0.5, 2*day)),
			read("side", in("5h", 0.6, time.Hour), in("7d", 0.05, 6*day)),
		), gen.Add(time.Minute)},
		{"probed, of one account, with no room", probeBuilt(gen, read("work", in("5h", 0.3, 3*time.Hour), spent("7d", 2*day))), gen.Add(time.Minute)},
	}
}

// routerBuilt is the router's document at gen of the accounts given, each as
// read, worked out, built as the router builds its own: each account as it
// stands at gen, as AsOf has it; with a usable token, its pressure and its
// recent rates over its session's trail, risen at the rate rates gives it, by
// id, over the last half hour, where it gives one; the best of those not
// shut; and the primary.
func routerBuilt(gen time.Time, rates map[string]float64, accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: gen, Source: status.SourceRouter, Router: status.Health{Healthy: true}}
	var open []status.Account
	for _, read := range accounts {
		a := read.AsOf(claudeLike, gen)
		if a.TokenSet {
			trails := sessionTrail(read, rates[read.ID], gen)
			a.Pressure = status.PressureFrom(claudeLike, status.Pressing{
				ID: a.ID, Reserve: a.Reserve, Latest: read.Windows, Trails: trails, Shut: a.Shut(gen, claudeLike),
			}, gen)
			a.Rates = status.RatesOf(read.Windows, trails, gen)
		}
		doc.Accounts = append(doc.Accounts, a)
		if !a.Shut(gen, claudeLike) {
			open = append(open, a)
		}
	}
	doc.Best, doc.Primary = status.Best(claudeLike, open, nil, gen), status.PrimaryOf(doc.Accounts)
	return doc.WorkedOut(claudeLike)
}

// sessionTrail is how the router has read account a's session lately, risen
// at rate an hour over the half hour before gen to where it was last read:
// none where rate is 0.
func sessionTrail(a status.Account, rate float64, gen time.Time) map[string][]score.Reading {
	session, ok := a.Window(claudeLike.Pressure)
	if !ok || rate == 0 {
		return nil
	}
	return map[string][]score.Reading{session.Key: {
		{At: gen.Add(-score.Recent), Utilization: max(session.Utilization-rate*score.Recent.Hours(), 0)},
		{At: a.FetchedAt, Utilization: session.Utilization},
	}}
}

// probeBuilt is the document probing builds at gen of the accounts given,
// worked out, as Collect builds it: each account as it stands at gen, the
// best of them, and the primary.
func probeBuilt(gen time.Time, accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: gen, Source: status.SourceProbe}
	for _, a := range accounts {
		doc.Accounts = append(doc.Accounts, a.AsOf(claudeLike, gen))
	}
	doc.Best, doc.Primary = status.Best(claudeLike, doc.Accounts, nil, gen), status.PrimaryOf(doc.Accounts)
	return doc.WorkedOut(claudeLike)
}

// At each hard moment, Runway's lanes, its badges and its weeks' resets, and
// GRAPHS' spans, draw each account as its moment has it, and agree with the
// one verdict of how it stands.
func TestRunwayAndGRAPHSAgreeWithTheMomentAtEachHardMoment(t *testing.T) {
	for _, tt := range runwayMoments() {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.doc.At(tt.now, claudeLike)
			for _, a := range m.Accounts {
				checkLaneAgrees(t, a, tt.now)
				checkWeeksAgree(t, a, tt.now)
				checkChartsAgree(t, a, tt.now)
			}
		})
	}
}

// routingHolds reports whether ROUTING's verdict k holds an account back from
// every request.
func routingHolds(k status.RoutingKind) bool {
	switch k {
	case status.RoutingTokenless, status.RoutingLimited, status.RoutingRefused, status.RoutingUnreadable, status.RoutingCapped:
		return true
	default:
		return false
	}
}

// checkLaneAgrees checks account a's lane over the day, at now: blank where
// ROUTING says nothing's read of it; without room where ROUTING holds it back
// from every request, or, as D7 keeps it, its last read failed; draining
// where a window every model shares runs out, as its RunsOut says; else with
// room. Each window counted holds it back as its facts say: its limit until
// it's back, its reserve as read until it resets as read, and from where it
// runs out, as its Out has it, while one stale runs out nowhere (D2).
func checkLaneAgrees(t *testing.T, a status.AccountNow, now time.Time) {
	t.Helper()
	l := Frame{Policy: claudeLike, Span: Day}.laneOf(a, now)
	want := roomOpen
	switch {
	case a.Routing.Kind == status.RoutingUnread:
		want = roomUnknown
	case routingHolds(a.Routing.Kind) || a.Error != "":
		want = roomNone
	case slices.ContainsFunc(a.Windows, func(w status.WindowNow) bool { return w.Shared && w.RunsOut }):
		want = roomDraining
	}
	if got := l.at(now, now); got != want {
		t.Errorf("%s's lane stands %v at now, want %v, as ROUTING has it %v", a.ID, got, want, a.Routing.Kind)
	}
	if !l.known || a.Error != "" {
		return
	}
	ends := func(until time.Time) bool {
		return slices.ContainsFunc(l.stretches, func(s stretch) bool {
			return slices.ContainsFunc(s.causes, func(c cause) bool { return c.until.Equal(until) })
		})
	}
	for _, w := range l.counted {
		switch {
		case w.Held && (!ends(w.Back) || l.at(now, now) != roomNone):
			t.Errorf("%s's %s is held till %v, but its lane has %+v", a.ID, w.Key, w.Back, l.stretches)
		case w.CappedAsRead && !ends(w.Read.ResetsAt):
			t.Errorf("%s's %s was at its reserve as read, but its lane has %+v", a.ID, w.Key, l.stretches)
		case w.RunsOut && (l.at(w.Out.At, now) != roomNone || l.drains.Before(w.Out.At)):
			t.Errorf("%s's %s runs out at %v, but its lane has %+v, draining till %v", a.ID, w.Key, w.Out.At, l.stretches, l.drains)
		case w.Kind == status.WindowStale && !w.Out.At.IsZero() && l.at(w.Out.At, now) != roomOpen:
			t.Errorf("%s's %s, stale, runs out nowhere, but its lane has %+v", a.ID, w.Key, l.stretches)
		}
	}
	if a.TokenRefused && !ends(a.Refusal.Until) {
		t.Errorf("%s's token is refused till %v, but its lane has %+v", a.ID, a.Refusal.Until, l.stretches)
	}
}

// checkWeeksAgree checks account a's badges at now: over the day, ▲ where
// it's next, as its moment has it, and anything is known of it; over the
// week, its week's use, as it stands, in attention where USAGE says it runs
// out, its Kind WindowRunsOut, in destructive at its limit; and its weeks'
// resets, as their Next has them, with what they're heading for where their
// Projected says.
func checkWeeksAgree(t *testing.T, a status.AccountNow, now time.Time) {
	t.Helper()
	day, week := Frame{Policy: claudeLike, Span: Day}, Frame{Policy: claudeLike, Span: Week}
	if got, want := len(day.badge(day.laneOf(a, now))) > 0, a.Next && a.Routing.Kind != status.RoutingUnread; got != want {
		t.Errorf("%s has ▲ over the day: %v, want %v", a.ID, got, want)
	}
	l := week.laneOf(a, now)
	weeks := week.weeks(l)
	if len(weeks) == 0 {
		return
	}
	w := weeks[0]
	wantInk := strongInk
	switch w.Kind {
	case status.WindowLimited:
		wantInk = exhaustedInk
	case status.WindowRunsOut:
		wantInk = alertInk
	}
	if got, want := week.badge(l), (line{{status.Percent(w.Used), wantInk}}); !slices.Equal(got, want) {
		t.Errorf("%s's week's badge is %+v, want %+v, as its week stands as kind %v", a.ID, got, want, w.Kind)
	}
	for _, w := range weeks {
		said := resetSays(w, now).plain()
		if !strings.HasPrefix(said, "resets "+status.Dated(now, w.Next)) || strings.HasSuffix(said, "used by then") != (w.Projected != nil) {
			t.Errorf("%s's %s's reset says %q, its next reset at %v, heading for %v", a.ID, w.Key, said, w.Next, w.Projected)
		}
	}
}

// checkChartsAgree checks account a's charts at now: each over its window's
// span as it stands, ending where its next reset is, as Runway marks it; none
// of a window not running; each with its account's cap.
func checkChartsAgree(t *testing.T, a status.AccountNow, now time.Time) {
	t.Helper()
	for _, w := range a.Windows {
		ch := chartOf(a, shownWindow{key: w.Key, name: w.Name}, Trail{}, now)
		if ch.spanned != (w.Kind != status.WindowNotStarted) || ch.spanned && !ch.start.Add(ch.length).Equal(w.Next) {
			t.Errorf("%s's %s is charted from %v for %v, spanned %v, want it to its next reset, %v, where it's running, as its kind %v says", a.ID, w.Key, ch.start, ch.length, ch.spanned, w.Next, w.Kind)
		}
		if ch.capAt != a.Cap {
			t.Errorf("%s's %s is charted capped at %v, want %v", a.ID, w.Key, ch.capAt, a.Cap)
		}
	}
}
