// Package statustest builds status documents for tests, as the router, or
// probing, builds them, at the moments views have got them wrong at, for
// each view's facts to be checked against the moment's at each of them.
package statustest

import (
	"fmt"
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
// before, and what's wanted of the moment then.
type HardMoment struct {
	Name string
	Doc  status.Document
	Now  time.Time
	// Want is what the moment of Doc at Now has, of every account and every
	// window, as a policy like Claude's judges it: the 5-hour window and the
	// week every model's, the 5-hour started by a request and judged for
	// pressure.
	Want Facts
}

// Facts are what a moment has of its document: the account ▲ marks, and how
// each account stands, by id.
type Facts struct {
	Next     string
	Accounts map[string]AccountFacts
}

// AccountFacts are how an account stands at a moment, as its AccountNow has
// it, and each of its windows, by key.
type AccountFacts struct {
	Routing                                            status.RoutingKind
	Held, CapHolds, Idle, OutOfQuota, CanRise, Pressed bool
	LimitHolds                                         bool
	LimitSince                                         time.Time
	Windows                                            map[string]WindowFacts
}

// String says how the account stands but for its windows, its times as
// sinceBuilt has them.
func (a AccountFacts) String() string {
	return fmt.Sprintf("{Routing:%d Held:%v CapHolds:%v Idle:%v OutOfQuota:%v CanRise:%v Pressed:%v LimitHolds:%v LimitSince:%s}",
		a.Routing, a.Held, a.CapHolds, a.Idle, a.OutOfQuota, a.CanRise, a.Pressed, a.LimitHolds, sinceBuilt(a.LimitSince))
}

// WindowFacts are how a window stands at a moment, as its WindowNow has it.
type WindowFacts struct {
	Kind                   status.WindowKind
	At                     time.Time
	Used                   float64
	Next                   time.Time
	ResetSinceRead, Lapsed bool
	Held                   bool
	Since                  time.Time
	Capped, CappedAsRead   bool
	RunsOut, RunsOutAhead  bool
	Out                    time.Time
}

// String says how the window stands, its times as sinceBuilt has them.
func (w WindowFacts) String() string {
	return fmt.Sprintf("{Kind:%d At:%s Used:%v Next:%s ResetSinceRead:%v Lapsed:%v Held:%v Since:%s Capped:%v CappedAsRead:%v RunsOut:%v RunsOutAhead:%v Out:%s}",
		w.Kind, sinceBuilt(w.At), w.Used, sinceBuilt(w.Next), w.ResetSinceRead, w.Lapsed, w.Held, sinceBuilt(w.Since),
		w.Capped, w.CappedAsRead, w.RunsOut, w.RunsOutAhead, sinceBuilt(w.Out))
}

// sinceBuilt shows t as how long after Built it is, as "built+112h0m0s", or
// "none" where it's zero.
func sinceBuilt(t time.Time) string {
	if t.IsZero() {
		return "none"
	}
	d := t.Sub(Built)
	if d < 0 {
		return "built" + d.String()
	}
	return "built+" + d.String()
}

// FactsOf are the facts of the moment m, as Facts has them.
func FactsOf(m status.Moment) Facts {
	f := Facts{Next: m.Next, Accounts: make(map[string]AccountFacts, len(m.Accounts))}
	for _, a := range m.Accounts {
		af := AccountFacts{
			Routing: a.Routing.Kind, Held: a.Held, CapHolds: a.CapHolds, Idle: a.Idle,
			OutOfQuota: a.OutOfQuota, CanRise: a.CanRise, Pressed: a.Pressure.Under, LimitHolds: a.LimitHolds, LimitSince: a.LimitSince,
			Windows: make(map[string]WindowFacts, len(a.Windows)),
		}
		for _, w := range a.Windows {
			af.Windows[w.Key] = WindowFacts{
				Kind: w.Kind, At: w.At, Used: w.Used, Next: w.Next, ResetSinceRead: w.ResetSinceRead, Lapsed: w.Lapsed,
				Held: w.Held, Since: w.Since, Capped: w.Capped, CappedAsRead: w.CappedAsRead,
				RunsOut: w.RunsOut, RunsOutAhead: w.RunsOutAhead, Out: w.Out.At,
			}
		}
		f.Accounts[a.ID] = af
	}
	return f
}

// HardMoments are the moments views have got the document wrong at, each
// document built through AsOf as the router, or probing, builds it, and
// worked out as policy judges it: of accounts work and side, but where a
// moment's of one alone.
func HardMoments(policy score.Policy) []HardMoment {
	side := readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.2, 5*day))
	sideOpen := AccountFacts{Routing: status.RoutingNext, CanRise: true, Windows: map[string]WindowFacts{
		"5h": {Kind: status.WindowOpen, At: Built.Add(4 * time.Hour), Used: 0.1, Next: Built.Add(4 * time.Hour)},
		"7d": {Kind: status.WindowOpen, At: Built.Add(5 * day), Used: 0.2, Next: Built.Add(5 * day)},
	}}
	limitedWork := with(readAt("work", spentIn("5h", 1, 40*time.Minute), windowIn("7d", 0.6, 3*day)), func(a *status.Account) {
		a.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: Built.Add(40 * time.Minute)}
	})
	limitedWorkEvents := []status.Event{{
		ID: 9, At: Built.Add(-20 * time.Minute), Kind: status.EventLimit, Account: "work",
		Windows: []string{"5h"}, Until: Built.Add(40 * time.Minute), Limit: 4,
	}}
	limited := Router(policy, nil, Routed{Account: limitedWork}, Routed{Account: side})
	limited.Events = limitedWorkEvents
	workLimited := AccountFacts{
		Routing: status.RoutingLimited, Held: true, OutOfQuota: true, LimitHolds: true, LimitSince: Built.Add(-20 * time.Minute),
		Windows: map[string]WindowFacts{
			"5h": {
				Kind: status.WindowLimited, At: Built.Add(40 * time.Minute), Used: 1, Next: Built.Add(40 * time.Minute),
				Held: true, Since: Built.Add(-20 * time.Minute),
			},
			"7d": {
				Kind: status.WindowRunsOut, At: Built.Add(64 * time.Hour), Used: 0.6, Next: Built.Add(3 * day),
				RunsOut: true, RunsOutAhead: true, Out: Built.Add(64 * time.Hour),
			},
		},
	}
	// pinnedToLimited has the global pin name work alone, set from the
	// command line once its limit was reached, as the routing card won't.
	pinnedToLimited := Router(policy, []string{"work"}, Routed{Account: limitedWork}, Routed{Account: side})
	pinnedToLimited.Pin.Since, pinnedToLimited.Events = Built.Add(-5*time.Minute), limitedWorkEvents
	// weekOut has work's week spent, the router's limit of it holding until
	// it resets an hour after Built, its session open.
	weekOut := Router(policy, nil,
		Routed{Account: with(readAt("work", windowIn("5h", 0.3, 3*time.Hour), spentIn("7d", 1, time.Hour)), func(a *status.Account) {
			a.Limit = status.Limit{ID: 6, Windows: []string{"7d"}, Until: Built.Add(time.Hour)}
		})},
		Routed{Account: side},
	)
	weekOut.Events = []status.Event{{
		ID: 10, At: Built.Add(-2 * day), Kind: status.EventLimit, Account: "work",
		Windows: []string{"7d"}, Until: Built.Add(time.Hour), Limit: 6,
	}}
	// fableOut has work's Fable week spent, the router's limit of it holding
	// until it resets, its windows every model shares open.
	fableOut := Router(policy, nil,
		Routed{Account: with(readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.4, 3*day), spentIn("7d_oi", 1, 2*day)), func(a *status.Account) {
			a.Limit = status.Limit{ID: 7, Windows: []string{"7d_oi"}, Until: Built.Add(2 * day)}
		})},
		Routed{Account: side},
	)
	fableOut.Events = []status.Event{{
		ID: 11, At: Built.Add(-3 * time.Hour), Kind: status.EventLimit, Account: "work",
		Windows: []string{"7d_oi"}, Until: Built.Add(2 * day), Limit: 7,
	}}
	// unnamed has work held back by a limit naming no window, and changed as
	// change changes it, beside other, the global pin naming those pinned.
	unnamed := func(pinned []string, change func(*status.Account), other status.Account) status.Document {
		return Router(policy, pinned,
			Routed{Account: with(readAt("work", windowIn("5h", 0.5, 3*time.Hour), windowIn("7d", 0.6, 3*day)), func(a *status.Account) {
				a.Limit = status.Limit{ID: 5, Until: Built.Add(40 * time.Minute)}
				change(a)
			})},
			Routed{Account: other},
		)
	}
	unchanged := func(*status.Account) {}
	tokenRefused := func(a *status.Account) {
		a.Refused = status.Refusal{Until: Built.Add(time.Hour), Status: 401}
	}
	workUnnamed := AccountFacts{
		Routing: status.RoutingLimited, Held: true, OutOfQuota: true, LimitHolds: true,
		Windows: map[string]WindowFacts{
			"5h": {
				Kind: status.WindowLimited, At: Built.Add(40 * time.Minute), Used: 0.5, Next: Built.Add(3 * time.Hour),
				Held: true, RunsOutAhead: true, Out: Built.Add(2 * time.Hour),
			},
			"7d": {
				Kind: status.WindowLimited, At: Built.Add(40 * time.Minute), Used: 0.6, Next: Built.Add(3 * day),
				Held: true, RunsOutAhead: true, Out: Built.Add(64 * time.Hour),
			},
		},
	}
	capped := func(pinned []string) status.Document {
		return Router(policy, pinned,
			Routed{Account: with(readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.92, time.Hour)), reserving)},
			Routed{Account: side},
		)
	}
	lapsed := Router(policy, nil,
		Routed{Account: with(readAt("work", windowIn("5h", 0.5, -time.Hour), windowIn("7d", 0.3, 3*day)), readEarlier(Built.Add(-6*time.Hour)))},
		Routed{Account: side},
	)
	lapsed.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: []status.Slot{
		{Account: "work", At: "13:30", Next: Built.Add(18 * time.Minute)},
		{Account: "side", At: "16:00", Next: Built.Add(5 * time.Hour)},
	}}
	moments := []HardMoment{
		{
			Name: "just after a reset, before the reread",
			Doc: Router(policy, nil,
				Routed{Account: readAt("work", windowIn("5h", 0.8, time.Hour), windowIn("7d", 0.5, 2*day))},
				Routed{Account: readAt("side", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.7, 30*time.Minute))},
			),
			Now: Built.Add(time.Hour + 5*time.Second),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingIdle, Idle: true, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowNotStarted, ResetSinceRead: true, Lapsed: true},
					"7d": {Kind: status.WindowOpen, At: Built.Add(2 * day), Used: 0.5, Next: Built.Add(2 * day)},
				}},
				"side": {Routing: status.RoutingNext, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(30*time.Minute + 7*day), Next: Built.Add(30*time.Minute + 7*day), ResetSinceRead: true},
				}},
			}},
		},
		{
			Name: "a failed read after an earlier one",
			Doc: Router(policy, nil,
				Routed{Account: with(readAt("work", windowIn("5h", 0.7, 2*time.Hour), windowIn("7d", 0.4, 3*day)), func(a *status.Account) {
					a.FetchedAt, a.Error = Built.Add(-20*time.Minute), "HTTP 529 · Overloaded"
				}), Rate: 0.3},
				Routed{Account: side},
			),
			Now: Built.Add(5 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingPressed, CanRise: true, Pressed: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowStale, At: Built.Add(-20 * time.Minute), Used: 0.7, Next: Built.Add(2 * time.Hour), Out: Built.Add(time.Hour)},
					"7d": {Kind: status.WindowStale, At: Built.Add(-20 * time.Minute), Used: 0.4, Next: Built.Add(3 * day)},
				}},
				"side": sideOpen,
			}},
		},
		{
			Name: "its token refused",
			Doc: Router(policy, nil,
				Routed{Account: with(readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.5, 2*day)), func(a *status.Account) {
					a.Refused = status.Refusal{Until: Built.Add(time.Hour), Status: 401}
				}), Rate: 0.4},
				Routed{Account: side},
			),
			Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingRefused, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowStale, At: Built, Used: 0.4, Next: Built.Add(2 * time.Hour), Out: Built.Add(90 * time.Minute)},
					"7d": {Kind: status.WindowStale, At: Built, Used: 0.5, Next: Built.Add(2 * day)},
				}},
				"side": sideOpen,
			}},
		},
		{
			Name: "a stale document, built three hours before now",
			Doc: Router(policy, nil,
				Routed{Account: readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.6, 4*day))},
				Routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.3, 5*day))},
			),
			Now: Built.Add(3 * time.Hour),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingIdle, Idle: true, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowNotStarted, ResetSinceRead: true, Lapsed: true},
					"7d": {
						Kind: status.WindowRunsOut, At: Built.Add(2 * day), Used: 0.6, Next: Built.Add(4 * day),
						RunsOut: true, RunsOutAhead: true, Out: Built.Add(2 * day),
					},
				}},
				"side": {Routing: status.RoutingNext, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(4 * time.Hour), Used: 0.1, Next: Built.Add(4 * time.Hour)},
					"7d": {
						Kind: status.WindowRunsOut, At: Built.Add(112 * time.Hour), Used: 0.3, Next: Built.Add(5 * day),
						RunsOut: true, RunsOutAhead: true, Out: Built.Add(112 * time.Hour),
					},
				}},
			}},
		},
		{
			Name: "its session lapsed", Doc: lapsed, Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "work", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingIdle, Idle: true, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowNotStarted, Lapsed: true},
					"7d": {Kind: status.WindowOpen, At: Built.Add(3 * day), Used: 0.3, Next: Built.Add(3 * day)},
				}},
				"side": {Routing: status.RoutingOpen, CanRise: true, Windows: sideOpen.Windows},
			}},
		},
		{
			Name: "a week at its reserve when built", Doc: capped(nil), Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingCapped, CapHolds: true, OutOfQuota: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
					"7d": {Kind: status.WindowCapped, At: Built.Add(time.Hour), Used: 0.92, Next: Built.Add(time.Hour), Capped: true, CappedAsRead: true},
				}},
				"side": sideOpen,
			}},
		},
		{
			Name: "a week at its reserve when built, then reset", Doc: capped(nil), Now: Built.Add(time.Hour + time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingOpen, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(time.Hour + 7*day), Next: Built.Add(time.Hour + 7*day), ResetSinceRead: true, CappedAsRead: true},
				}},
				"side": sideOpen,
			}},
		},
		{
			Name: "a week at its reserve when built, the global pin naming it", Doc: capped([]string{"work"}), Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "work", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingCappedPinned, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(time.Hour), Used: 0.92, Next: Built.Add(time.Hour)},
				}},
				"side": {Routing: status.RoutingOpen, CanRise: true, Windows: sideOpen.Windows},
			}},
		},
		{
			// Side's week needs using first, and its session, at its rate,
			// reaches its reserve before it resets, but its limit after: the
			// pin spends the reserve, so side isn't under pressure.
			Name: "the global pin naming both, the better under pressure only at its reserve",
			Doc: Router(policy, []string{"work", "side"},
				Routed{Account: readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.3, 4*day))},
				Routed{Account: with(readAt("side", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.3, 2*day)), reserving), Rate: 0.26},
			),
			Now: Built.Add(5 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingOpen, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(4 * day), Used: 0.3, Next: Built.Add(4 * day)},
				}},
				"side": {Routing: status.RoutingNext, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(2 * time.Hour), Used: 0.4, Next: Built.Add(2 * time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(2 * day), Used: 0.3, Next: Built.Add(2 * day)},
				}},
			}},
		},
		{
			Name: "a limit holding", Doc: limited, Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{"work": workLimited, "side": sideOpen}},
		},
		{
			Name: "a limit holding, then lifted", Doc: limited, Now: Built.Add(41 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingIdle, Idle: true, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowNotStarted, ResetSinceRead: true, Lapsed: true},
					"7d": {
						Kind: status.WindowRunsOut, At: Built.Add(64 * time.Hour), Used: 0.6, Next: Built.Add(3 * day),
						RunsOut: true, RunsOutAhead: true, Out: Built.Add(64 * time.Hour),
					},
				}},
				"side": sideOpen,
			}},
		},
		{
			Name: "a limit naming no window holding", Doc: unnamed(nil, unchanged, side), Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{"work": workUnnamed, "side": sideOpen}},
		},
		{
			Name: "the global pin's one account held back, new sessions going elsewhere",
			Doc:  unnamed([]string{"work"}, unchanged, side), Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{"work": workUnnamed, "side": sideOpen}},
		},
		{
			Name: "no account able to take a request",
			Doc:  unnamed(nil, unchanged, with(side, tokenRefused)), Now: Built.Add(10 * time.Minute),
			Want: Facts{Accounts: map[string]AccountFacts{
				"work": workUnnamed,
				"side": {Routing: status.RoutingRefused, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowStale, At: Built, Used: 0.1, Next: Built.Add(4 * time.Hour)},
					"7d": {Kind: status.WindowStale, At: Built, Used: 0.2, Next: Built.Add(5 * day)},
				}},
			}},
		},
		{
			Name: "its token refused, a limit naming no window holding",
			Doc:  unnamed(nil, tokenRefused, side),
			Now:  Built.Add(10 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {
					Routing: status.RoutingLimited, Held: true, OutOfQuota: true, LimitHolds: true,
					Windows: map[string]WindowFacts{
						"5h": {
							Kind: status.WindowLimited, At: Built.Add(40 * time.Minute), Used: 0.5, Next: Built.Add(3 * time.Hour),
							Held: true, Out: Built.Add(2 * time.Hour),
						},
						"7d": {
							Kind: status.WindowLimited, At: Built.Add(40 * time.Minute), Used: 0.6, Next: Built.Add(3 * day),
							Held: true, Out: Built.Add(64 * time.Hour),
						},
					},
				},
				"side": sideOpen,
			}},
		},
		{
			Name: "one account without a token, another not read yet",
			Doc: Router(policy, nil,
				Routed{ID: "work", Label: "work"},
				Routed{ID: "side", Label: "side", TokenSet: true},
			),
			Now: Built.Add(time.Minute),
			Want: Facts{Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingTokenless, Windows: map[string]WindowFacts{}},
				"side": {Routing: status.RoutingUnread, Windows: map[string]WindowFacts{}},
			}},
		},
		{
			Name: "its first read failed, nothing read before",
			Doc: Router(policy, nil,
				Routed{ID: "work", Label: "work", TokenSet: true, Error: "HTTP 529 · Overloaded"},
				Routed{Account: side},
			),
			Now: Built.Add(time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingUnreadable, Windows: map[string]WindowFacts{}},
				"side": sideOpen,
			}},
		},
		{
			Name: "probed, of several accounts",
			Doc: Probed(policy,
				readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.5, 2*day)),
				readAt("side", windowIn("5h", 0.6, time.Hour), windowIn("7d", 0.05, 6*day)),
			),
			Now: Built.Add(time.Minute),
			Want: Facts{Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingOpen, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(2 * day), Used: 0.5, Next: Built.Add(2 * day)},
				}},
				"side": {Routing: status.RoutingOpen, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(time.Hour), Used: 0.6, Next: Built.Add(time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(6 * day), Used: 0.05, Next: Built.Add(6 * day)},
				}},
			}},
		},
		{
			Name: "probed, of one account, with no room",
			Doc:  Probed(policy, readAt("work", windowIn("5h", 0.3, 3*time.Hour), spentIn("7d", 1, 2*day))),
			Now:  Built.Add(time.Minute),
			Want: Facts{Next: "work", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingLimited, Held: true, OutOfQuota: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
					"7d": {Kind: status.WindowLimited, At: Built.Add(2 * day), Used: 1, Next: Built.Add(2 * day), Held: true},
				}},
			}},
		},
		{
			// Where new sessions go is the router's to say, and it isn't
			// running: no account is ▲.
			Name: "probed, one of several at its limit",
			Doc:  Probed(policy, readAt("work", spentIn("5h", 1, 40*time.Minute), windowIn("7d", 0.6, 3*day)), side),
			Now:  Built.Add(time.Minute),
			Want: Facts{Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingLimited, Held: true, OutOfQuota: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowLimited, At: Built.Add(40 * time.Minute), Used: 1, Next: Built.Add(40 * time.Minute), Held: true},
					"7d": workLimited.Windows["7d"],
				}},
				"side": {Routing: status.RoutingOpen, CanRise: true, Windows: sideOpen.Windows},
			}},
		},
		{
			// The limit lifts as the week resets: every fact of it changes
			// on the tick after, before the router reads it again.
			Name: "a week at its limit, reset since read, before the reread", Doc: weekOut, Now: Built.Add(time.Hour + 5*time.Second),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingOpen, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(time.Hour + 7*day), Next: Built.Add(time.Hour + 7*day), ResetSinceRead: true},
				}},
				"side": sideOpen,
			}},
		},
		{
			// Work's week has more left to use a day till it resets than
			// side's, so new sessions go to it.
			Name: "a model's own window at its limit, its account open to the rest", Doc: fableOut, Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "work", Accounts: map[string]AccountFacts{
				"work": {
					Routing: status.RoutingModelLimited, Held: true, CanRise: true, LimitHolds: true, LimitSince: Built.Add(-3 * time.Hour),
					Windows: map[string]WindowFacts{
						"5h": {Kind: status.WindowOpen, At: Built.Add(3 * time.Hour), Used: 0.3, Next: Built.Add(3 * time.Hour)},
						"7d": {Kind: status.WindowOpen, At: Built.Add(3 * day), Used: 0.4, Next: Built.Add(3 * day)},
						"7d_oi": {
							Kind: status.WindowLimited, At: Built.Add(2 * day), Used: 1, Next: Built.Add(2 * day),
							Held: true, Since: Built.Add(-3 * time.Hour),
						},
					},
				},
				"side": {Routing: status.RoutingOpen, CanRise: true, Windows: sideOpen.Windows},
			}},
		},
		{
			Name: "the global pin set from the command line to an account at its limit", Doc: pinnedToLimited, Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{"work": workLimited, "side": sideOpen}},
		},
		{
			Name: "the global pin naming one account, a pool of one",
			Doc: Router(policy, []string{"side"},
				Routed{Account: readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.5, 2*day))},
				Routed{Account: side},
			),
			Now: Built.Add(10 * time.Minute),
			Want: Facts{Next: "side", Accounts: map[string]AccountFacts{
				"work": {Routing: status.RoutingOpen, CanRise: true, Windows: map[string]WindowFacts{
					"5h": {Kind: status.WindowOpen, At: Built.Add(2 * time.Hour), Used: 0.4, Next: Built.Add(2 * time.Hour)},
					"7d": {Kind: status.WindowOpen, At: Built.Add(2 * day), Used: 0.5, Next: Built.Add(2 * day)},
				}},
				"side": sideOpen,
			}},
		},
	}
	for i := range moments {
		moments[i].Doc = moments[i].Doc.WorkedOut(policy)
	}
	return moments
}

// Routed is an account as the router knows it as it builds its document:
// read, as the router last read it, before it's judged as it stands; and how
// fast its session has been used over the last half hour, where the router
// has a recent rate of it, 0 where it has none.
type Routed struct {
	status.Account
	Rate float64
}

// trails are how the router has read the account's session lately, as
// score.Noted keeps them, where it has a recent rate of it: from where its
// Rate puts its use the half hour before gen, never below none, to where it
// was last read.
func (r Routed) trails(policy score.Policy, gen time.Time) map[string][]score.Reading {
	session, ok := r.Window(policy.Pressure)
	if !ok || r.Rate == 0 {
		return nil
	}
	from := gen.Add(-score.Recent)
	return map[string][]score.Reading{session.Key: {
		{At: from, Utilization: max(session.Utilization-r.Rate*score.Recent.Hours(), 0)},
		{At: r.FetchedAt, Utilization: session.Utilization},
	}}
}

// Router is the router's document at Built of the accounts given, before
// it's worked out, the global pin naming those pinned, built as the router
// builds its own, as policy judges its windows: each account as it stands
// then, as AsOf has it; with a usable token, its pressure, as
// status.PressureFrom has it, and its recent rates, as status.RatesOf has
// them, over its trails; where a new session goes, as status.Best has it;
// and the primary.
func Router(policy score.Policy, pinned []string, accounts ...Routed) status.Document {
	gen := Built
	doc := status.Document{GeneratedAt: gen, Source: status.SourceRouter, Router: status.Health{Healthy: true}}
	if len(pinned) > 0 {
		doc.Pin = status.Pin{Accounts: pinned, Since: gen.Add(-time.Hour)}
	}
	for _, r := range accounts {
		a := r.AsOf(policy, gen)
		if a.TokenSet {
			trails := r.trails(policy, gen)
			a.Pressure = status.PressureFrom(policy, status.Pressing{
				ID: a.ID, Reserve: a.Reserve, Latest: r.Windows, Trails: trails, Spent: slices.Contains(pinned, a.ID), Shut: a.Shut(gen, policy),
			}, gen)
			a.Rates = status.RatesOf(r.Windows, trails, gen)
		}
		doc.Accounts = append(doc.Accounts, a)
	}
	doc.Best, doc.Primary = status.Best(policy, doc.Accounts, pinned, gen), status.PrimaryOf(doc.Accounts)
	return doc
}

// Probed is the document probing builds at Built of the accounts given, each
// read as given, before it's worked out, as Collect builds it, as policy
// judges their windows: each account as it stands then, the best of them, as
// status.ProbedBest has it, and the primary, the router not running.
func Probed(policy score.Policy, accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: Built, Source: status.SourceProbe, Fallback: status.Fallback{Router: status.RouterNotRunning}}
	for _, a := range accounts {
		doc.Accounts = append(doc.Accounts, a.AsOf(policy, Built))
	}
	doc.Best, doc.Primary = status.ProbedBest(policy, doc.Accounts, Built), status.PrimaryOf(doc.Accounts)
	return doc
}

// labels are the windows' labels, by key.
var labels = map[string]string{"5h": "Session", "7d": "Week", "7d_oi": "Fable week"}

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
