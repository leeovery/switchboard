package status_test

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// built is when the documents worked out here were built: a Monday, 13:12
// UTC.
var built = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

const day = 24 * time.Hour

// labels are the windows' labels, by key.
var labels = map[string]string{"5h": "Session", "7d": "Week", "7d_oi": "Fable week"}

// windowIn is the window with the given key, used so far, resetting resetsIn
// after built.
func windowIn(key string, used float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: key, Label: labels[key], Utilization: used, ResetsAt: built.Add(resetsIn)}
}

// spentIn is the window with the given key, read spent, resetting resetsIn
// after built.
func spentIn(key string, used float64, resetsIn time.Duration) quota.Window {
	w := windowIn(key, used, resetsIn)
	w.Status = quota.StatusRejected
	return w
}

// readAt is an account labelled as its id, with a usable token, its windows
// those given, read as the document was built.
func readAt(id string, windows ...quota.Window) status.Account {
	return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: built, Windows: windows}
}

// routerDocOf is the router's document of the accounts, built at built.
func routerDocOf(accounts ...status.Account) status.Document {
	return status.Document{GeneratedAt: built, Source: status.SourceRouter, Accounts: accounts}
}

// pinnedTo is doc, the global pin naming the accounts with the given ids.
func pinnedTo(doc status.Document, ids ...string) status.Document {
	doc.Pin = status.Pin{Accounts: ids, Since: built.Add(-time.Hour)}
	return doc
}

// reserving has the account leave a tenth of every window unused.
func reserving(a *status.Account) {
	a.Reserve = 0.1
}

// pressedAt has the router see the account's session used at rate lately.
func pressedAt(rate float64) func(*status.Account) {
	return func(a *status.Account) {
		a.Pressure = status.Pressure{Window: "5h", Rate: rate, Recent: true}
		a.Rates = []status.Rate{{Window: "5h", Rate: rate}}
	}
}

func TestEachWindowsEvenPaceAndAllowance(t *testing.T) {
	week := windowIn("7d", 0.5, 2*day)
	tests := []struct {
		name    string
		doc     status.Document
		wantOK  bool
		pace    float64
		allowed quota.Allowance
	}{
		{
			name:    "the session, two hours of five gone, its room by the hour to its reset",
			doc:     routerDocOf(readAt("work", windowIn("5h", 0.4, 3*time.Hour))),
			wantOK:  true,
			pace:    0.4,
			allowed: quota.Allowance{Share: 0.2, Per: quota.PerHour},
		},
		{
			name:    "the week, five days of seven gone, its room by the day to its reset",
			doc:     routerDocOf(readAt("work", week)),
			wantOK:  true,
			pace:    5.0 / 7,
			allowed: quota.Allowance{Share: 0.25, Per: quota.PerDay},
		},
		{
			name:    "the session, its reset within the hour: the room itself",
			doc:     routerDocOf(readAt("work", windowIn("5h", 0.66, 30*time.Minute))),
			wantOK:  true,
			pace:    0.9,
			allowed: quota.Allowance{Share: 0.34},
		},
		{
			name:    "the week, its reset within the day: the room itself",
			doc:     routerDocOf(readAt("work", windowIn("7d", 0.66, 6*time.Hour))),
			wantOK:  true,
			pace:    162.0 / 168,
			allowed: quota.Allowance{Share: 0.34},
		},
		{
			name:    "to its cap, where its reserve holds it back",
			doc:     routerDocOf(with(readAt("work", week), reserving)),
			wantOK:  true,
			pace:    5.0 / 7,
			allowed: quota.Allowance{Share: 0.2, Per: quota.PerDay},
		},
		{
			name:    "to its limit, the global pin spending its cap",
			doc:     pinnedTo(routerDocOf(with(readAt("work", week), reserving)), "work"),
			wantOK:  true,
			pace:    5.0 / 7,
			allowed: quota.Allowance{Share: 0.25, Per: quota.PerDay},
		},
		{
			name:    "to its cap, probed, as no pin is known",
			doc:     status.Document{GeneratedAt: built, Source: status.SourceProbe, Accounts: []status.Account{with(readAt("work", week), reserving)}},
			wantOK:  true,
			pace:    5.0 / 7,
			allowed: quota.Allowance{Share: 0.2, Per: quota.PerDay},
		},
		{
			name:   "at its cap: no room, so no allowance",
			doc:    routerDocOf(with(readAt("work", windowIn("7d", 0.93, 2*day)), reserving)),
			wantOK: true,
			pace:   5.0 / 7,
		},
		{
			name:   "over its limit: no room, never less",
			doc:    routerDocOf(readAt("work", spentIn("5h", 1.04, time.Hour))),
			wantOK: true,
			pace:   0.8,
		},
		{
			name: "held by the router's limit, though it reads some room",
			doc: routerDocOf(with(readAt("work", windowIn("5h", 0.98, time.Hour)), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"5h"}, Until: built.Add(time.Hour)}
			})),
			wantOK: true,
			pace:   0.8,
		},
		{
			name:    "reset since it was read: from empty, from that reset, to its next a length on",
			doc:     routerDocOf(readAt("work", spentIn("7d", 1, -day))),
			wantOK:  true,
			pace:    1.0 / 7,
			allowed: quota.Allowance{Share: 1.0 / 6, Per: quota.PerDay},
		},
		{
			name: "reset by hand: from when it started again",
			doc: routerDocOf(readAt("work", func() quota.Window {
				w := windowIn("7d", 0.04, 4*day)
				w.RestartedAt = built.Add(-6 * time.Hour)
				return w
			}())),
			wantOK:  true,
			pace:    6.0 / 102,
			allowed: quota.Allowance{Share: 0.24, Per: quota.PerDay},
		},
		{
			name: "a session that has lapsed: not running",
			doc: routerDocOf(with(readAt("work", quota.Window{Key: "5h", Label: "Session"}), func(a *status.Account) {
				a.Lapsed = []string{"5h"}
			})),
		},
		{
			name: "a session whose reset has passed since it was read: lapsed, not running",
			doc:  routerDocOf(readAt("work", windowIn("5h", 0.6, -time.Minute))),
		},
		{name: "its reset not known", doc: routerDocOf(readAt("work", quota.Window{Key: "7d", Label: "Week", Utilization: 0.5}))},
		{name: "its length not known", doc: routerDocOf(readAt("work", windowIn("burst", 0.5, time.Hour)))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := tt.doc.WorkedOut(policy).Accounts[0].Windows[0]
			if (w.Pace != nil) != tt.wantOK || w.Pace != nil && math.Abs(*w.Pace-tt.pace) > 1e-12 {
				t.Errorf("its even pace is %s, want %s", paceOf(w.Pace), paceWanted(tt.wantOK, tt.pace))
			}
			if math.Abs(w.Allowance.Share-tt.allowed.Share) > 1e-12 || w.Allowance.Per != tt.allowed.Per {
				t.Errorf("its allowance is %+v, want %+v", w.Allowance, tt.allowed)
			}
		})
	}
}

// paceOf shows an even pace, or that there's none.
func paceOf(pace *float64) string {
	if pace == nil {
		return "none"
	}
	return fmt.Sprintf("%.6g", *pace)
}

// paceWanted shows the even pace a test wants: pace, or none unless ok.
func paceWanted(ok bool, pace float64) string {
	if !ok {
		return "none"
	}
	return paceOf(&pace)
}

func TestThePool(t *testing.T) {
	work := readAt("work", windowIn("5h", 0.4, 3*time.Hour), windowIn("7d", 0.5, 2*day))
	side := readAt("side", windowIn("5h", 0.2, time.Hour), windowIn("7d", 0.1, 6*day), windowIn("7d_oi", 0.3, 6*day))
	reservingMore := func(a *status.Account) { a.Reserve = 0.2 }
	weekCapped := with(readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.95, 2*day)), reserving)
	plainSide := readAt("side", windowIn("5h", 0.2, time.Hour), windowIn("7d", 0.1, 6*day))
	tests := []struct {
		name     string
		doc      status.Document
		accounts []string
		pinned   bool
		// want are its windows, as poolLines shows them.
		want []string
	}{
		{name: "none with one account", doc: routerDocOf(work)},
		{
			name:     "every account on auto, a window each that any has, shortest first",
			doc:      routerDocOf(work, side),
			accounts: []string{"work", "side"},
			want: []string{
				"5h Session: room 1.4, used 0.3, pace 0.6",
				"7d Week: room 1.4, used 0.3, pace 0.428571",
				"7d_oi Fable week: room 0.7, used 0.65, pace 0.142857",
			},
		},
		{
			name:     "probed, the pin unknown: every account, to each one's cap",
			doc:      status.Document{GeneratedAt: built, Source: status.SourceProbe, Accounts: []status.Account{with(work, reserving), with(side, reservingMore)}},
			accounts: []string{"work", "side"},
			want: []string{
				"5h Session: room 1.1, used 0.45, pace 0.6",
				"7d Week: room 1.1, used 0.45, pace 0.428571",
				"7d_oi Fable week: room 0.5, used 0.75, pace 0.142857",
			},
		},
		{
			name:     "pinned: the accounts the pin names, each to its limit, as the pin spends its cap",
			doc:      pinnedTo(routerDocOf(with(work, reserving), readAt("personal", windowIn("5h", 0.9, time.Hour)), with(side, reservingMore)), "work", "side"),
			accounts: []string{"work", "side"},
			pinned:   true,
			want: []string{
				"5h Session: room 1.4, used 0.3, pace 0.6",
				"7d Week: room 1.4, used 0.3, pace 0.428571",
				"7d_oi Fable week: room 0.7, used 0.65, pace 0.142857",
			},
		},
		{
			name: "none in the session of an account at its limit, nor in the window its limit holds",
			doc: routerDocOf(with(readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.97, 2*day)), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"7d"}, Until: built.Add(2 * day)}
			}), plainSide),
			accounts: []string{"work", "side"},
			want: []string{
				"5h Session: room 0.8, used 0.6, pace 0.6",
				"7d Week: room 0.9, used 0.55, pace 0.428571",
			},
		},
		{
			name:     "none in the session of an account at its cap, nor below none in the window at it",
			doc:      routerDocOf(weekCapped, plainSide),
			accounts: []string{"work", "side"},
			want: []string{
				"5h Session: room 0.8, used 0.6, pace 0.6",
				"7d Week: room 0.9, used 0.55, pace 0.428571",
			},
		},
		{
			name:     "an account at its cap, which the pin spends",
			doc:      pinnedTo(routerDocOf(weekCapped, plainSide), "work", "side"),
			accounts: []string{"work", "side"},
			pinned:   true,
			want: []string{
				"5h Session: room 1.5, used 0.25, pace 0.6",
				"7d Week: room 0.95, used 0.525, pace 0.428571",
			},
		},
		{
			name: "none from an account whose token is refused, nor one not read, though both count",
			doc: routerDocOf(with(work, func(a *status.Account) {
				a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401}
			}), status.Account{ID: "personal", Label: "personal", TokenSet: true}, plainSide),
			accounts: []string{"work", "personal", "side"},
			want: []string{
				"5h Session: room 0.8, used 0.733333, pace 0.6",
				"7d Week: room 0.9, used 0.7, pace 0.428571",
			},
		},
		{
			name:     "none rather than less over a limit, and all of a window reset since it was read",
			doc:      routerDocOf(readAt("work", spentIn("5h", 1.04, time.Hour), windowIn("7d", 0.9, -day)), plainSide),
			accounts: []string{"work", "side"},
			want: []string{
				"5h Session: room 0.8, used 0.6, pace 0.8",
				"7d Week: room 1.9, used 0.05, pace 0.142857",
			},
		},
		{
			name: "all of a session that has lapsed, its pace the running sessions' alone",
			doc: routerDocOf(with(readAt("work", quota.Window{Key: "5h", Label: "Session"}, windowIn("7d", 0.5, 2*day)), func(a *status.Account) {
				a.Lapsed = []string{"5h"}
			}), plainSide),
			accounts: []string{"work", "side"},
			want: []string{
				"5h Session: room 1.8, used 0.1, pace 0.8",
				"7d Week: room 1.4, used 0.3, pace 0.428571",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := tt.doc.WorkedOut(policy).Pool
			if !slices.Equal(pool.Accounts, tt.accounts) || pool.Pinned != tt.pinned {
				t.Errorf("the pool is of %q, pinned %v, want %q, pinned %v", pool.Accounts, pool.Pinned, tt.accounts, tt.pinned)
			}
			if got := poolLines(pool); !slices.Equal(got, tt.want) {
				t.Errorf("the pool's windows are %q, want %q", got, tt.want)
			}
		})
	}
}

// poolLines shows each of the pool's windows: its key, label, room, use and
// even pace.
func poolLines(p status.Pool) []string {
	var lines []string
	for _, w := range p.Windows {
		lines = append(lines, fmt.Sprintf("%s %s: room %.6g, used %.6g, pace %s", w.Key, w.Label, w.Room, w.Used, paceOf(w.Pace)))
	}
	return lines
}

func TestWhatsComingUp(t *testing.T) {
	quiet := func(id string, weekIn time.Duration) status.Account {
		return readAt(id, windowIn("5h", 0.1, 2*time.Hour), windowIn("7d", 0.2, weekIn))
	}
	reset := func(id, key string, in time.Duration) status.Upcoming {
		return status.Upcoming{At: built.Add(in), Account: id, Kind: status.UpcomingReset, Window: key}
	}
	back := func(id, key string, in time.Duration, capped bool) status.Upcoming {
		return status.Upcoming{At: built.Add(in), Account: id, Kind: status.UpcomingBack, Window: key, Cap: capped}
	}
	runsOut := func(id, key string, in time.Duration, capped bool) status.Upcoming {
		return status.Upcoming{At: built.Add(in), Account: id, Kind: status.UpcomingRunsOut, Window: key, Cap: capped}
	}
	// pressed's session began an hour ago, half used, so its use since it
	// started has it run out in an hour; at the quarter of it an hour it's
	// been used at lately, in two.
	pressed := with(readAt("work", windowIn("5h", 0.5, 4*time.Hour), windowIn("7d", 0.2, 3*day)), pressedAt(0.25))
	limited := with(readAt("work", spentIn("5h", 1, time.Hour), windowIn("7d", 0.89, 3*day)), func(a *status.Account) {
		a.Limit = status.Limit{Windows: []string{"5h"}, Until: built.Add(time.Hour)}
	})
	tests := []struct {
		name string
		doc  status.Document
		want []status.Upcoming
	}{
		{
			name: "each window resetting, soonest first, those at one time in the accounts' order",
			doc:  routerDocOf(quiet("work", 3*day), quiet("side", 2*day)),
			want: []status.Upcoming{reset("work", "5h", 2*time.Hour), reset("side", "5h", 2*time.Hour), reset("side", "7d", 2*day), reset("work", "7d", 3*day)},
		},
		{
			name: "a limit lifting, the reset that lifts it not told again, and no run-out while it holds",
			doc:  routerDocOf(limited),
			want: []status.Upcoming{back("work", "", time.Hour, false), reset("work", "7d", 3*day)},
		},
		{
			name: "a limit of a model's own week lifting, named, the session still running",
			doc: routerDocOf(with(readAt("fable", windowIn("5h", 0.1, 2*time.Hour), windowIn("7d", 0.2, 3*day), spentIn("7d_oi", 1, 4*day)), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: built.Add(4 * day)}
			})),
			want: []status.Upcoming{reset("fable", "5h", 2*time.Hour), reset("fable", "7d", 3*day), back("fable", "7d_oi", 4*day, false)},
		},
		{
			name: "a limit of a model's own week lifting as the week resets, which is still told",
			doc: routerDocOf(with(readAt("fable", windowIn("5h", 0.1, 2*time.Hour), windowIn("7d", 0.2, 3*day), spentIn("7d_oi", 1, 3*day)), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: built.Add(3 * day)}
			})),
			want: []status.Upcoming{reset("fable", "5h", 2*time.Hour), back("fable", "7d_oi", 3*day, false), reset("fable", "7d", 3*day)},
		},
		{
			name: "a cap lifting as the capped week resets",
			doc:  routerDocOf(with(readAt("work", windowIn("5h", 0.2, 2*time.Hour), windowIn("7d", 0.95, 2*day)), reserving)),
			want: []status.Upcoming{reset("work", "5h", 2*time.Hour), back("work", "", 2*day, true)},
		},
		{
			name: "a limit and a cap: back once both have lifted, the cap last",
			doc: routerDocOf(with(readAt("work", spentIn("5h", 1, time.Hour), windowIn("7d", 0.95, 2*day)), func(a *status.Account) {
				a.Reserve, a.Limit = 0.1, status.Limit{Windows: []string{"5h"}, Until: built.Add(time.Hour)}
			})),
			want: []status.Upcoming{reset("work", "5h", time.Hour), back("work", "", 2*day, true)},
		},
		{
			name: "a cap on a model's own week lifting, named",
			doc:  routerDocOf(with(readAt("fable", windowIn("5h", 0.2, 2*time.Hour), windowIn("7d", 0.3, 3*day), windowIn("7d_oi", 0.95, 2*day)), reserving)),
			want: []status.Upcoming{reset("fable", "5h", 2*time.Hour), back("fable", "7d_oi", 2*day, true), reset("fable", "7d", 3*day)},
		},
		{
			name: "the session running out at its recent rate, slower than its use since it started",
			doc:  routerDocOf(pressed),
			want: []status.Upcoming{runsOut("work", "5h", 2*time.Hour, false), reset("work", "5h", 4*time.Hour), reset("work", "7d", 3*day)},
		},
		{
			name: "reaching its cap first, where its reserve holds it back",
			doc:  routerDocOf(with(pressed, reserving)),
			want: []status.Upcoming{runsOut("work", "5h", 96*time.Minute, true), reset("work", "5h", 4*time.Hour), reset("work", "7d", 3*day)},
		},
		{
			name: "at its limit, the global pin spending its cap",
			doc:  pinnedTo(routerDocOf(with(pressed, reserving)), "work"),
			want: []status.Upcoming{runsOut("work", "5h", 2*time.Hour, false), reset("work", "5h", 4*time.Hour), reset("work", "7d", 3*day)},
		},
		{
			name: "probed, at the pace of its use since it started",
			doc:  status.Document{GeneratedAt: built, Source: status.SourceProbe, Accounts: []status.Account{readAt("work", windowIn("5h", 0.5, 4*time.Hour))}},
			want: []status.Upcoming{runsOut("work", "5h", time.Hour, false), reset("work", "5h", 4*time.Hour)},
		},
		{
			name: "no run-out while its token is refused",
			doc: routerDocOf(with(pressed, func(a *status.Account) {
				a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401}
			})),
			want: []status.Upcoming{reset("work", "5h", 4*time.Hour), reset("work", "7d", 3*day)},
		},
		{
			name: "a week reset since it was read: its next reset a length on",
			doc:  routerDocOf(readAt("work", windowIn("5h", 0.1, 2*time.Hour), windowIn("7d", 0.9, -day))),
			want: []status.Upcoming{reset("work", "5h", 2*time.Hour), reset("work", "7d", 6*day)},
		},
		{
			name: "a model's own week resetting only where it doesn't with the week",
			doc: routerDocOf(
				readAt("work", windowIn("5h", 0.1, 2*time.Hour), windowIn("7d", 0.2, 3*day), windowIn("7d_oi", 0.1, 3*day)),
				readAt("side", windowIn("5h", 0.1, 2*time.Hour), windowIn("7d", 0.2, 3*day), windowIn("7d_oi", 0.1, 2*day)),
			),
			want: []status.Upcoming{reset("work", "5h", 2*time.Hour), reset("side", "5h", 2*time.Hour), reset("side", "7d_oi", 2*day), reset("work", "7d", 3*day), reset("side", "7d", 3*day)},
		},
		{
			name: "neither a reset nor a run-out of a session that has lapsed",
			doc: routerDocOf(with(readAt("work", quota.Window{Key: "5h", Label: "Session"}, windowIn("7d", 0.2, 3*day)), func(a *status.Account) {
				a.Lapsed = []string{"5h"}
			})),
			want: []status.Upcoming{reset("work", "7d", 3*day)},
		},
		{
			name: "the router's primes, each account's next",
			doc: func() status.Document {
				doc := routerDocOf(quiet("work", 3*day))
				doc.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: []status.Slot{
					{Account: "spare", At: "13:50", Next: built.Add(38 * time.Minute)},
					{Account: "work", At: "03:50", Next: built.Add(14 * time.Hour)},
					{Account: "side", At: "08:00"},
				}}
				return doc
			}(),
			want: []status.Upcoming{
				{At: built.Add(38 * time.Minute), Account: "spare", Kind: status.UpcomingPrime},
				reset("work", "5h", 2*time.Hour),
				{At: built.Add(14 * time.Hour), Account: "work", Kind: status.UpcomingPrime},
				reset("work", "7d", 3*day),
			},
		},
		{
			name: "nothing at or before the document was built",
			doc: func() status.Document {
				doc := routerDocOf(with(quiet("work", 3*day), func(a *status.Account) {
					a.Limit = status.Limit{Until: built.Add(-time.Minute)}
				}))
				doc.Prime = status.Prime{Slots: []status.Slot{{Account: "work", Next: built}}}
				return doc
			}(),
			want: []status.Upcoming{reset("work", "5h", 2*time.Hour), reset("work", "7d", 3*day)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.doc.WorkedOut(policy).ComingUp; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ComingUp =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// fullDocument is a router's document with something of everything worked
// out of it: a pin, a session under pressure, a cap, a limit, a refusal, a
// session lapsed, a week reset since it was read, and primes.
func fullDocument() status.Document {
	doc := pinnedTo(routerDocOf(
		with(readAt("work", windowIn("5h", 0.5, 4*time.Hour), windowIn("7d", 0.95, 2*day), windowIn("7d_oi", 0.3, 3*day)), func(a *status.Account) {
			reserving(a)
			pressedAt(0.25)(a)
		}),
		with(readAt("personal", spentIn("5h", 1, time.Hour), windowIn("7d", 0.89, -day)), func(a *status.Account) {
			a.Limit = status.Limit{ID: 2, Windows: []string{"5h"}, Until: built.Add(time.Hour)}
		}),
		with(readAt("side", quota.Window{Key: "5h", Label: "Session"}, windowIn("7d", 0.4, 5*day)), func(a *status.Account) {
			a.Lapsed = []string{"5h"}
			a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 403, Family: "opus"}
		}),
	), "work", "side")
	doc.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: []status.Slot{{Account: "side", At: "13:50", Next: built.Add(38 * time.Minute)}}}
	return doc
}

func TestWorkingADocumentOutAgainChangesNothing(t *testing.T) {
	passed := routerDocOf(status.Account{ID: "work", Label: "work"})
	passed.Prime = status.Prime{Slots: []status.Slot{{Account: "work", Next: built}}}
	tests := []struct {
		name string
		doc  status.Document
	}{
		{name: "with something of everything worked out", doc: fullDocument()},
		{name: "with nothing coming up after it was built", doc: passed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worked := tt.doc.WorkedOut(policy)
			if again := worked.WorkedOut(policy); !reflect.DeepEqual(again, worked) {
				t.Errorf("worked out again, the document is\n%+v\nwant it as it was\n%+v", again, worked)
			}
			given, err := json.Marshal(worked)
			if err != nil {
				t.Fatal(err)
			}
			var read status.Document
			if err := json.Unmarshal(given, &read); err != nil {
				t.Fatal(err)
			}
			if again := read.WorkedOut(policy); !reflect.DeepEqual(again, read) {
				t.Errorf("read as JSON and worked out again, the document is\n%+v\nwant it as it was read\n%+v", again, read)
			}
		})
	}
	if worked := fullDocument().WorkedOut(policy); worked.Pool.Windows == nil || worked.ComingUp == nil || worked.Accounts[0].Windows[0].Pace == nil {
		t.Errorf("WorkedOut() = %+v, want something of everything worked out", worked)
	}
}

func TestWorkingADocumentOutGivesWhatItLacksAndLeavesItAsItWas(t *testing.T) {
	doc := fullDocument()
	given, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var read status.Document
	if err := json.Unmarshal(given, &read); err != nil {
		t.Fatal(err)
	}
	if got, want := read.WorkedOut(policy), doc.WorkedOut(policy); !reflect.DeepEqual(got, want) {
		t.Errorf("read from a router that gives none of it, the document works out as\n%+v\nwant as the router's own\n%+v", got, want)
	}
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			if w.Pace != nil || w.Allowance != (quota.Allowance{}) {
				t.Errorf("worked out, the document given has %s's %s with pace %s and allowance %+v, want it as it was", a.ID, w.Key, paceOf(w.Pace), w.Allowance)
			}
		}
	}
}
