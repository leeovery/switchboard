// Package statustest builds status documents for tests, as the router, or
// probing, builds them, at the moments views have got them wrong at, for
// each view's facts to be checked against the moment's at each of them.
package statustest

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// Built is when the hard moments' documents were built.
var Built = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

const day = 24 * time.Hour

// HardMoment is a document the router, or probing, builds, worked out as the
// dashboard is given it, judged at a moment a view has got it wrong at
// before.
type HardMoment struct {
	Name string
	Doc  status.Document
	Now  time.Time
}

// HardMoments are the moments views have got the document wrong at, each
// document built through AsOf as the router, or probing, builds it, and
// worked out as policy judges it: of accounts work and side, but where a
// moment's of one alone.
func HardMoments(policy score.Policy) []HardMoment {
	b := builder{policy: policy}
	side := readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.2, 5*day))
	limited := b.routed(nil,
		routed{Account: with(readAt("work", spentIn("5h", 1, 40*time.Minute), windowIn("7d", 0.6, 3*day)), func(a *status.Account) {
			a.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: Built.Add(40 * time.Minute)}
		})},
		routed{Account: side},
	)
	limited.Events = []status.Event{{
		ID: 9, At: Built.Add(-20 * time.Minute), Kind: status.EventLimit, Account: "work",
		Windows: []string{"5h"}, Until: Built.Add(40 * time.Minute), Limit: 4,
	}}
	capped := func(pinned []string) status.Document {
		return b.routed(pinned,
			routed{Account: with(readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.92, time.Hour)), reserving)},
			routed{Account: side},
		)
	}
	lapsed := b.routed(nil,
		routed{Account: with(readAt("work", windowIn("5h", 0.5, -time.Hour), windowIn("7d", 0.3, 3*day)), readEarlier(Built.Add(-6*time.Hour)))},
		routed{Account: side},
	)
	lapsed.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: []status.Slot{
		{Account: "work", At: "13:30", Next: Built.Add(18 * time.Minute)},
		{Account: "side", At: "16:00", Next: Built.Add(5 * time.Hour)},
	}}
	moments := []HardMoment{
		{
			Name: "just after a reset, before the reread",
			Doc: b.routed(nil,
				routed{Account: readAt("work", windowIn("5h", 0.8, time.Hour), windowIn("7d", 0.5, 2*day))},
				routed{Account: readAt("side", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.7, 30*time.Minute))},
			),
			Now: Built.Add(time.Hour + 5*time.Second),
		},
		{
			Name: "a failed read after an earlier one",
			Doc: b.routed(nil,
				routed{Account: with(readAt("work", windowIn("5h", 0.7, 2*time.Hour), windowIn("7d", 0.4, 3*day)), func(a *status.Account) {
					a.FetchedAt, a.Error = Built.Add(-20*time.Minute), "HTTP 529 · Overloaded"
				}), rate: 0.3},
				routed{Account: side},
			),
			Now: Built.Add(5 * time.Minute),
		},
		{
			Name: "its token refused",
			Doc: b.routed(nil,
				routed{Account: with(readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.5, 2*day)), func(a *status.Account) {
					a.Refused = status.Refusal{Until: Built.Add(time.Hour), Status: 401}
				}), rate: 0.4},
				routed{Account: side},
			),
			Now: Built.Add(10 * time.Minute),
		},
		{
			Name: "a stale document, built three hours before now",
			Doc: b.routed(nil,
				routed{Account: readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.6, 4*day))},
				routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.3, 5*day))},
			),
			Now: Built.Add(3 * time.Hour),
		},
		{Name: "its session lapsed", Doc: lapsed, Now: Built.Add(10 * time.Minute)},
		{Name: "a week at its reserve when built", Doc: capped(nil), Now: Built.Add(10 * time.Minute)},
		{Name: "a week at its reserve when built, then reset", Doc: capped(nil), Now: Built.Add(time.Hour + time.Minute)},
		{Name: "a week at its reserve when built, the global pin naming it", Doc: capped([]string{"work"}), Now: Built.Add(10 * time.Minute)},
		{Name: "a limit holding", Doc: limited, Now: Built.Add(10 * time.Minute)},
		{Name: "a limit holding, then lifted", Doc: limited, Now: Built.Add(41 * time.Minute)},
		{
			Name: "probed, of several accounts",
			Doc: b.probed(
				readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.5, 2*day)),
				readAt("side", windowIn("5h", 0.6, time.Hour), windowIn("7d", 0.05, 6*day)),
			),
			Now: Built.Add(time.Minute),
		},
		{
			Name: "probed, of one account, with no room",
			Doc:  b.probed(readAt("work", windowIn("5h", 0.3, 3*time.Hour), spentIn("7d", 1, 2*day))),
			Now:  Built.Add(time.Minute),
		},
	}
	for i := range moments {
		moments[i].Doc = moments[i].Doc.WorkedOut(policy)
	}
	return moments
}

// builder builds documents as the router, or probing, does, as policy judges
// their windows.
type builder struct {
	policy score.Policy
}

// routed is an account as the router knows it as it builds its document:
// read, as the router last read it, before it's judged as it stands; and how
// fast its session has been used over the last half hour, where the router
// has a recent rate of it, 0 where it has none.
type routed struct {
	status.Account
	rate float64
}

// trails are how the router has read the account's session lately, as
// score.Noted keeps them, where it has a recent rate of it: from where rate
// puts its use the half hour before gen, never below none, to where it was
// last read.
func (r routed) trails(policy score.Policy, gen time.Time) map[string][]score.Reading {
	session, ok := r.Window(policy.Pressure)
	if !ok || r.rate == 0 {
		return nil
	}
	from := gen.Add(-score.Recent)
	return map[string][]score.Reading{session.Key: {
		{At: from, Utilization: max(session.Utilization-r.rate*score.Recent.Hours(), 0)},
		{At: r.FetchedAt, Utilization: session.Utilization},
	}}
}

// routed is the router's document at Built of the accounts given, before
// it's worked out, the global pin naming those pinned, built as the router
// builds its own: each account as it stands then, as AsOf has it; with a
// usable token, its pressure, as status.PressureFrom has it, and its recent
// rates, as status.RatesOf has them, over its trails; the best of those not
// shut; and the primary.
func (b builder) routed(pinned []string, accounts ...routed) status.Document {
	gen := Built
	doc := status.Document{GeneratedAt: gen, Source: status.SourceRouter, Router: status.Health{Healthy: true}}
	if len(pinned) > 0 {
		doc.Pin = status.Pin{Accounts: pinned, Since: gen.Add(-time.Hour)}
	}
	var open []status.Account
	for _, r := range accounts {
		a := r.AsOf(b.policy, gen)
		if a.TokenSet {
			trails := r.trails(b.policy, gen)
			a.Pressure = status.PressureFrom(b.policy, status.Pressing{
				ID: a.ID, Reserve: a.Reserve, Latest: r.Windows, Trails: trails, Spent: slices.Contains(pinned, a.ID), Shut: a.Shut(gen, b.policy),
			}, gen)
			a.Rates = status.RatesOf(r.Windows, trails, gen)
		}
		doc.Accounts = append(doc.Accounts, a)
		if !a.Shut(gen, b.policy) {
			open = append(open, a)
		}
	}
	doc.Best, doc.Primary = status.Best(b.policy, open, pinned, gen), status.PrimaryOf(doc.Accounts)
	return doc
}

// probed is the document probing builds at Built of the accounts given, each
// read as given, before it's worked out, as Collect builds it: each account
// as it stands then, the best of them, and the primary, the router not
// running.
func (b builder) probed(accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: Built, Source: status.SourceProbe, Fallback: status.Fallback{Router: status.RouterNotRunning}}
	for _, a := range accounts {
		doc.Accounts = append(doc.Accounts, a.AsOf(b.policy, Built))
	}
	doc.Best, doc.Primary = status.Best(b.policy, doc.Accounts, nil, Built), status.PrimaryOf(doc.Accounts)
	return doc
}

// labels are the windows' labels, by key.
var labels = map[string]string{"5h": "Session", "7d": "Week"}

// windowIn is the window with the given key, used so far, resetting resetsIn
// after Built.
func windowIn(key string, used float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: key, Label: labels[key], Utilization: used, ResetsAt: Built.Add(resetsIn)}
}

// spentIn is the window with the given key, read spent, resetting resetsIn
// after Built.
func spentIn(key string, used float64, resetsIn time.Duration) quota.Window {
	w := windowIn(key, used, resetsIn)
	w.Status = quota.StatusRejected
	return w
}

// readAt is an account labelled as its id, with a usable token, its windows
// those given, read as the document was built.
func readAt(id string, windows ...quota.Window) status.Account {
	return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: Built, Windows: windows}
}

// with is a, changed as change changes it.
func with(a status.Account, change func(*status.Account)) status.Account {
	change(&a)
	return a
}

// reserving has the account keep a reserve of a tenth of each window.
func reserving(a *status.Account) {
	a.Reserve = 0.1
}

// readEarlier has the account read at the time given, rather than as the
// document was built.
func readEarlier(at time.Time) func(*status.Account) {
	return func(a *status.Account) {
		a.FetchedAt = at
	}
}
