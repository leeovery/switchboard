package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// hardBuilt is when the hard moments' documents were built: a Monday, 13:12
// UTC, as status's own hard moments have it.
var hardBuilt = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

// hardMoment is a document the router, or probing, builds, worked out, as the
// dashboard is given it, and a moment a view has got it wrong at before:
// status's hard moments, each built as there.
type hardMoment struct {
	name string
	doc  status.Document
	now  time.Time
}

// hardWindow is the window with the given key, labelled as Claude's are, used
// so far, resetting a while after hardBuilt.
func hardWindow(key string, used float64, resetsIn time.Duration) quota.Window {
	labels := map[string]string{"5h": "Session", "7d": "Week"}
	return quota.Window{Key: key, Label: labels[key], Utilization: used, ResetsAt: hardBuilt.Add(resetsIn)}
}

// hardSpent is hardWindow, read spent.
func hardSpent(key string, used float64, resetsIn time.Duration) quota.Window {
	w := hardWindow(key, used, resetsIn)
	w.Status = quota.StatusRejected
	return w
}

// hardAccount is an account labelled as its id, with a usable token, its
// windows those given, read as the document was built, as change leaves it.
func hardAccount(id string, windows []quota.Window, change ...func(*status.Account)) status.Account {
	a := status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: hardBuilt, Windows: windows}
	for _, c := range change {
		c(&a)
	}
	return a
}

// hardWindows are the windows given, as a hard account has them.
func hardWindows(windows ...quota.Window) []quota.Window {
	return windows
}

// hardRouted is an account as the router knows it as it builds its document,
// and how fast its session has been used over the last half hour, 0 where it
// has no recent rate of it.
type hardRouted struct {
	account status.Account
	rate    float64
}

// trails are how the router has read the account's session lately, where it
// has a recent rate of it: from where rate puts its use the half hour before
// gen, never below none, to where it was last read.
func (r hardRouted) trails(gen time.Time) map[string][]score.Reading {
	session, ok := r.account.Window(claudeLike.Pressure)
	if !ok || r.rate == 0 {
		return nil
	}
	return map[string][]score.Reading{session.Key: {
		{At: gen.Add(-score.Recent), Utilization: max(session.Utilization-r.rate*score.Recent.Hours(), 0)},
		{At: r.account.FetchedAt, Utilization: session.Utilization},
	}}
}

// hardRouterDoc is the router's document at gen of the accounts given, before
// it's worked out, built as the router builds its own: each account as it
// stands at gen, its pressure and recent rates over its trails, the best of
// those not shut, and the primary.
func hardRouterDoc(gen time.Time, accounts ...hardRouted) status.Document {
	doc := status.Document{GeneratedAt: gen, Source: status.SourceRouter, Router: status.Health{Healthy: true}}
	var open []status.Account
	for _, r := range accounts {
		a := r.account.AsOf(claudeLike, gen)
		if a.TokenSet {
			trails := r.trails(gen)
			a.Pressure = status.PressureFrom(claudeLike, status.Pressing{
				ID: a.ID, Reserve: a.Reserve, Latest: r.account.Windows, Trails: trails, Shut: a.Shut(gen, claudeLike),
			}, gen)
			a.Rates = status.RatesOf(r.account.Windows, trails, gen)
		}
		doc.Accounts = append(doc.Accounts, a)
		if !a.Shut(gen, claudeLike) {
			open = append(open, a)
		}
	}
	doc.Best, doc.Primary = status.Best(claudeLike, open, nil, gen), status.PrimaryOf(doc.Accounts)
	return doc
}

// hardProbedDoc is the document probing builds at gen of the accounts given,
// before it's worked out: each as it stands at gen, the best of them and the
// primary.
func hardProbedDoc(gen time.Time, accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: gen, Source: status.SourceProbe}
	for _, a := range accounts {
		doc.Accounts = append(doc.Accounts, a.AsOf(claudeLike, gen))
	}
	doc.Best, doc.Primary = status.Best(claudeLike, doc.Accounts, nil, gen), status.PrimaryOf(doc.Accounts)
	return doc
}

// hardMoments are status's hard moments, the documents built as there, each
// worked out, as the dashboard is given it, and judged at its moment.
func hardMoments() []hardMoment {
	moments := builtHardMoments()
	for i := range moments {
		moments[i].doc = moments[i].doc.WorkedOut(claudeLike)
	}
	return moments
}

// builtHardMoments are the hard moments, their documents as built, before
// they're worked out.
func builtHardMoments() []hardMoment {
	day := 24 * time.Hour
	side := hardRouted{account: hardAccount("side", hardWindows(hardWindow("5h", 0.1, 4*time.Hour), hardWindow("7d", 0.2, 5*day)))}
	limited := hardRouterDoc(hardBuilt,
		hardRouted{account: hardAccount("work", hardWindows(hardSpent("5h", 1, 40*time.Minute), hardWindow("7d", 0.6, 3*day)), func(a *status.Account) {
			a.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: hardBuilt.Add(40 * time.Minute)}
		})},
		side,
	)
	limited.Events = []status.Event{{
		ID: 9, At: hardBuilt.Add(-20 * time.Minute), Kind: status.EventLimit, Account: "work",
		Windows: []string{"5h"}, Until: hardBuilt.Add(40 * time.Minute), Limit: 4,
	}}
	capped := hardRouterDoc(hardBuilt,
		hardRouted{account: hardAccount("work", hardWindows(hardWindow("5h", 0.3, 3*time.Hour), hardWindow("7d", 0.92, time.Hour)), func(a *status.Account) {
			a.Reserve = 0.1
		})},
		side,
	)
	lapsed := hardRouterDoc(hardBuilt,
		hardRouted{account: hardAccount("work", hardWindows(hardWindow("5h", 0.5, -time.Hour), hardWindow("7d", 0.3, 3*day)), func(a *status.Account) {
			a.FetchedAt = hardBuilt.Add(-6 * time.Hour)
		})},
		side,
	)
	lapsed.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: []status.Slot{
		{Account: "work", At: "13:30", Next: hardBuilt.Add(18 * time.Minute)},
		{Account: "side", At: "16:00", Next: hardBuilt.Add(5 * time.Hour)},
	}}
	return []hardMoment{
		{
			name: "just after a reset, before the reread",
			doc: hardRouterDoc(hardBuilt,
				hardRouted{account: hardAccount("work", hardWindows(hardWindow("5h", 0.8, time.Hour), hardWindow("7d", 0.5, 2*day)))},
				hardRouted{account: hardAccount("side", hardWindows(hardWindow("5h", 0.3, 3*time.Hour), hardWindow("7d", 0.7, 30*time.Minute)))},
			),
			now: hardBuilt.Add(time.Hour + 5*time.Second),
		},
		{
			name: "a failed read after an earlier one",
			doc: hardRouterDoc(hardBuilt,
				hardRouted{account: hardAccount("work", hardWindows(hardWindow("5h", 0.7, 2*time.Hour), hardWindow("7d", 0.4, 3*day)), func(a *status.Account) {
					a.FetchedAt, a.Error = hardBuilt.Add(-20*time.Minute), "HTTP 529 · Overloaded"
				}), rate: 0.3},
				side,
			),
			now: hardBuilt.Add(5 * time.Minute),
		},
		{
			name: "its token refused",
			doc: hardRouterDoc(hardBuilt,
				hardRouted{account: hardAccount("work", hardWindows(hardWindow("5h", 0.4, 2*time.Hour), hardWindow("7d", 0.5, 2*day)), func(a *status.Account) {
					a.Refused = status.Refusal{Until: hardBuilt.Add(time.Hour), Status: 401}
				}), rate: 0.4},
				side,
			),
			now: hardBuilt.Add(10 * time.Minute),
		},
		{
			name: "a stale document, built three hours before now",
			doc: hardRouterDoc(hardBuilt,
				hardRouted{account: hardAccount("work", hardWindows(hardWindow("5h", 0.4, 2*time.Hour), hardWindow("7d", 0.6, 4*day)))},
				hardRouted{account: hardAccount("side", hardWindows(hardWindow("5h", 0.1, 4*time.Hour), hardWindow("7d", 0.3, 5*day)))},
			),
			now: hardBuilt.Add(3 * time.Hour),
		},
		{name: "its session lapsed", doc: lapsed, now: hardBuilt.Add(10 * time.Minute)},
		{name: "a week at its reserve when built", doc: capped, now: hardBuilt.Add(10 * time.Minute)},
		{name: "a week at its reserve when built, then reset", doc: capped, now: hardBuilt.Add(time.Hour + time.Minute)},
		{name: "a limit holding", doc: limited, now: hardBuilt.Add(10 * time.Minute)},
		{name: "a limit holding, then lifted", doc: limited, now: hardBuilt.Add(41 * time.Minute)},
		{
			name: "probed, of several accounts",
			doc: hardProbedDoc(hardBuilt,
				hardAccount("work", hardWindows(hardWindow("5h", 0.3, 3*time.Hour), hardWindow("7d", 0.5, 2*day))),
				hardAccount("side", hardWindows(hardWindow("5h", 0.6, time.Hour), hardWindow("7d", 0.05, 6*day))),
			),
			now: hardBuilt.Add(time.Minute),
		},
		{
			name: "probed, of one account, with no room",
			doc:  hardProbedDoc(hardBuilt, hardAccount("work", hardWindows(hardWindow("5h", 0.3, 3*time.Hour), hardSpent("7d", 1, 2*day)))),
			now:  hardBuilt.Add(time.Minute),
		},
	}
}
