package status_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestText(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	personal := status.Account{ID: "personal", Label: "Personal", Error: "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"}
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{
			name: "windows heading every way, and the account to use next",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceProbe,
				Best:        "work",
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(),
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC), Status: quota.StatusAllowed},
							{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC), Status: quota.StatusAllowedWarning},
						},
						Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
					},
					{
						ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC(),
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 1, ResetsAt: time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC), Status: quota.StatusRejected},
							{Key: "7d_oi", Label: "Fable week", Utilization: 1.04, ResetsAt: time.Date(2026, 9, 28, 13, 19, 0, 0, time.UTC)},
							{Key: "30d", Label: "30d", Utilization: 0.05},
							{Key: "burst", Label: "burst", Utilization: 1},
						},
					},
					personal,
					{ID: "spare", Label: "Spare", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
				},
			},
			want: `work · Work
  Session     23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  Week        93%  resets in 5d 11h · Sun 02:10 · runs out ~Mon 16:54
  Fable offline: HTTP 529 · Overloaded

side · Side
  Session    100%  resets now · Mon 14:00
  Fable week 104%  resets in 7m · Mon 14:19 · exhausted
  30d          5%
  burst      100%  exhausted

personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

spare · Spare
  HTTP 401 · Invalid bearer token

best next: work · Work
probed directly
`,
		},
		{
			name: "no account to use next",
			doc:  status.Document{GeneratedAt: now.UTC(), Source: status.SourceProbe, Accounts: []status.Account{personal}},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

probed directly
`,
		},
		{
			name: "the router's, pinned, with each account's sessions and a limit",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Best:        "work",
				Pin:         status.Pin{Accounts: []string{"side"}, Since: now.UTC().Add(-time.Hour)},
				Router:      status.Health{Healthy: true, Requests: 12},
				Sessions:    3,
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(), Sessions: 2,
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)},
							{Key: "7d", Label: "Week", Utilization: 0.4, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC)},
						},
					},
					{
						ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC(), Sessions: 1,
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)},
						},
						Limit: status.Limit{Until: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)},
					},
					personal,
				},
			},
			want: `work · Work
  Session  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  Week     40%  resets in 5d 11h · Sun 02:10 · runs out ~Wed 20:15
  2 sessions

side · Side
  Session  60%  resets in 2h 58m · Mon 17:10 · runs out ~Mon 15:33
  limit until Mon 21:00
  1 session

personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

best next: work · Work
from the router: healthy  ·  3 sessions  ·  pinned to side · Side
`,
		},
		{
			name: "the router's, with refusals",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true, Requests: 12},
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(),
						Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)}},
						Refused: status.Refusal{Until: time.Date(2026, 9, 28, 13, 20, 0, 0, time.UTC), Status: 401},
					},
					{
						ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC(), Sessions: 1,
						Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 1, ResetsAt: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)}},
						Limit:   status.Limit{Windows: []string{"5h"}, Until: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)},
						Refused: status.Refusal{Until: time.Date(2026, 9, 28, 13, 21, 0, 0, time.UTC), Status: 403, Family: "opus"},
					},
					{
						ID: "spare", Label: "Spare", TokenSet: true, FetchedAt: now.UTC(),
						Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.5, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)}},
						Refused: status.Refusal{Until: now.Add(-time.Minute), Status: 403, Family: "fable"},
					},
				},
			},
			want: `work · Work
  Session  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  refused (401) until 14:20

side · Side
  Session 100%  resets in 6h 48m · Mon 21:00 · exhausted
  limit until Mon 21:00
  refused (403, opus) until 14:21
  1 session

spare · Spare
  Session  50%  resets in 2h 58m · Mon 17:10 · runs out ~Mon 16:14

from the router: healthy  ·  no sessions  ·  routing automatically
`,
		},
		{
			name: "the router's, with a session that has lapsed",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Best:        "work",
				Router:      status.Health{Healthy: true},
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC().Add(-6 * time.Hour),
						Windows: []quota.Window{
							{Key: "5h", Label: "Session"},
							{Key: "7d", Label: "Week", Utilization: 0.4, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC)},
						},
						Lapsed: []string{"5h"},
					},
				},
			},
			want: `work · Work
  Session   0%  not started
  Week     40%  resets in 5d 11h · Sun 02:10 · runs out ~Wed 20:15

best next: work · Work
from the router: healthy  ·  no sessions  ·  routing automatically
`,
		},
		{
			name: "the router's, priming, with a session that has lapsed and a prime due again",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Best:        "work",
				Router:      status.Health{Healthy: true},
				Prime: status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{
					{Account: "work", At: "04:10", Next: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)},
					{Account: "side", At: "06:40", Next: time.Date(2026, 9, 28, 13, 17, 0, 0, time.UTC)},
				}},
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(),
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)},
							{Key: "7d", Label: "Week", Utilization: 0.4, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC)},
						},
					},
					{
						ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC().Add(-6 * time.Hour),
						Windows: []quota.Window{{Key: "5h", Label: "Session"}},
						Lapsed:  []string{"5h"},
					},
				},
			},
			want: `work · Work
  Session  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  Week     40%  resets in 5d 11h · Sun 02:10 · runs out ~Wed 20:15

side · Side
  Session   0%  not started · next prime Mon 14:17

priming 08:00-23:00: work at 04:10 and side at 06:40
next reset: work · Work, Mon 17:10  ·  next prime: side · Side, Mon 14:17
best next: work · Work
from the router: healthy  ·  no sessions  ·  routing automatically
`,
		},
		{
			name: "the router's, unhealthy, routing automatically, and a limit that has lifted",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Router:      status.Health{Requests: 8, Failures: 6, Reason: "6 of the 8 requests in the last 5 minutes failed"},
				Accounts: []status.Account{
					{ID: "side", Label: "Side", TokenSet: true, Error: "HTTP 401 · Invalid bearer token", Limit: status.Limit{Until: now.Add(-time.Minute)}},
				},
			},
			want: `side · Side
  HTTP 401 · Invalid bearer token

from the router: unhealthy, 6 of the 8 requests in the last 5 minutes failed  ·  no sessions  ·  routing automatically
`,
		},
		{
			name: "the router's, with a restart due",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Restart:     status.Restart{Reason: "upgraded", Since: now.Add(-time.Hour), InFlight: 3},
				Accounts:    []status.Account{personal},
			},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

from the router: healthy  ·  no sessions  ·  routing automatically
restart due since Mon 13:12 (upgraded), once no request is in flight (3 now): switchboard service restart restarts it now, cutting off requests still in flight after 30 seconds
`,
		},
		{
			name: "probed, as the router isn't running",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterNotRunning},
				Accounts:    []status.Account{personal},
			},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

probed directly: the router isn't running
`,
		},
		{
			name: "probed, as the router didn't answer as it should",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"},
				Accounts:    []status.Account{personal},
			},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

probed directly: the router is unhealthy, no answer within 500ms
`,
		},
		{
			name: "no accounts",
			doc:  status.Document{GeneratedAt: now.UTC(), Source: status.SourceProbe},
			want: "probed directly\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.doc.Text(now); got != tt.want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestTextMarksThePrimaryAndAReserveHoldingItsAccountBack(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	work := status.Account{
		ID: "work", Label: "Work", Primary: true, Reserve: 0.1, TokenSet: true, FetchedAt: now.UTC(), Sessions: 2,
		Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)},
			{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC)},
		},
		AtReserve: []string{"7d"},
		Limit:     status.Limit{Windows: []string{"7d_oi"}, Until: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)},
	}
	side := status.Account{
		ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC(),
		Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)}},
	}
	tests := []struct {
		name string
		pin  status.Pin
		// work are the lines status gives work beside its usage.
		work string
	}{
		{
			name: "at its reserve",
			pin:  status.Pin{Accounts: []string{"side"}, Since: now.UTC().Add(-time.Hour)},
			work: "  limit until Mon 21:00\n  at its reserve (90%)\n  2 sessions\n",
		},
		{
			name: "spending its reserve, pinned",
			pin:  status.Pin{Accounts: []string{"work"}, Since: now.UTC().Add(-time.Hour)},
			work: "  limit until Mon 21:00\n  spending its reserve (pinned)\n  2 sessions\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Primary:     "work",
				Pin:         tt.pin,
				Router:      status.Health{Healthy: true},
				Sessions:    2,
				Accounts:    []status.Account{work, side},
			}
			want := "work · Work (primary)\n" +
				"  Session  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%\n" +
				"  Week     93%  resets in 5d 11h · Sun 02:10 · runs out ~Mon 16:54\n" +
				tt.work + "\n" +
				"side · Side\n" +
				"  Session  60%  resets in 2h 58m · Mon 17:10 · runs out ~Mon 15:33\n" +
				"\n" +
				"from the router: healthy  ·  2 sessions  ·  " + doc.Routing() + "\n"
			if got := doc.Text(now); got != want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestReserved(t *testing.T) {
	atReserve := status.Account{ID: "work", Label: "Work", Reserve: 0.15, AtReserve: []string{"5h"}}
	tests := []struct {
		name    string
		account status.Account
		// pin are the accounts the global pin names.
		pin  []string
		want string
	}{
		{name: "at its reserve, where the reserve starts", account: atReserve, want: "at its reserve (85%)"},
		{name: "at its reserve, the global pin elsewhere", account: atReserve, pin: []string{"side"}, want: "at its reserve (85%)"},
		{name: "spent by the global pin", account: atReserve, pin: []string{"work"}, want: "spending its reserve (pinned)"},
		{name: "spent by the global pin, among others", account: atReserve, pin: []string{"work", "side"}, want: "spending its reserve (pinned)"},
		{name: "short of its reserve", account: status.Account{ID: "work", Label: "Work", Reserve: 0.15}},
		{name: "short of its reserve, pinned", account: status.Account{ID: "work", Label: "Work", Reserve: 0.15}, pin: []string{"work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{Pin: status.Pin{Accounts: tt.pin}, Accounts: []status.Account{tt.account}}
			if got := doc.Reserved(tt.account); got != tt.want {
				t.Errorf("Reserved() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPressed(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	under := status.Pressure{Window: "5h", Rate: 0.3, Recent: true, RunsOut: time.Date(2026, 9, 28, 17, 21, 0, 0, time.UTC), Under: true}
	tests := []struct {
		name    string
		account status.Account
		// pin are the accounts the global pin names.
		pin  []string
		want string
	}{
		{name: "running out", account: status.Account{ID: "work", Pressure: under}, want: "under pressure: runs out ~18:21"},
		{name: "reaching its reserve", account: status.Account{ID: "work", Reserve: 0.1, Pressure: under}, want: "under pressure: at its reserve ~18:21"},
		{name: "running out, its reserve spent by the global pin", account: status.Account{ID: "work", Reserve: 0.1, Pressure: under}, pin: []string{"work"}, want: "under pressure: runs out ~18:21"},
		{name: "running out after its reset", account: status.Account{ID: "work", Pressure: status.Pressure{Window: "5h", Rate: 0.1, RunsOut: under.RunsOut}}},
		{name: "its pace unknown", account: status.Account{ID: "work"}},
		{
			name:    "held back from one model's requests, under pressure as the router says",
			account: status.Account{ID: "work", Pressure: under, Refused: status.Refusal{Until: now.Add(time.Minute), Status: 403, Family: "opus"}},
			want:    "under pressure: runs out ~18:21",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{Pin: status.Pin{Accounts: tt.pin}, Accounts: []status.Account{tt.account}}
			if got := doc.Pressed(tt.account, now); got != tt.want {
				t.Errorf("Pressed() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTextSaysWhenAnAccountIsUnderPressure(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	// The session, 60% used, resets at 17:10, having started at 12:10: 30% an
	// hour runs it out at 15:32, a reserve of a tenth reached at 15:12, and
	// its use since it started runs it out at 15:33.
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)}
	tests := []struct {
		name     string
		reserve  float64
		pressure status.Pressure
		// heading is where the session heads, as its line says.
		heading string
		// notes are the lines status gives the account beside its usage.
		notes string
	}{
		{
			name:     "at its rate over the last half hour",
			pressure: status.Pressure{Window: "5h", Rate: 0.3, Recent: true, RunsOut: time.Date(2026, 9, 28, 14, 32, 0, 0, time.UTC), Under: true},
			heading:  "runs out ~Mon 15:32 at its rate over the last 30 min",
			notes:    "  under pressure: runs out ~15:32 at Session's rate over the last 30 min, before its reset at 17:10\n",
		},
		{
			name: "at its rate over the 18 minutes it's measured over",
			pressure: status.Pressure{
				Window: "5h", Rate: 0.3, Recent: true, Since: now.Add(-18 * time.Minute).UTC(),
				RunsOut: time.Date(2026, 9, 28, 14, 32, 0, 0, time.UTC), Under: true,
			},
			heading: "runs out ~Mon 15:32 at its rate over the last 18 min",
			notes:   "  under pressure: runs out ~15:32 at Session's rate over the last 18 min, before its reset at 17:10\n",
		},
		{
			name:     "at its rate since it started",
			pressure: status.Pressure{Window: "5h", Rate: 0.6 / (2*time.Hour + 2*time.Minute).Hours(), RunsOut: time.Date(2026, 9, 28, 14, 33, 20, 0, time.UTC), Under: true},
			heading:  "runs out ~Mon 15:33",
			notes:    "  under pressure: runs out ~15:33 at Session's rate since it started, before its reset at 17:10\n",
		},
		{
			name:     "reaching its reserve",
			reserve:  0.1,
			pressure: status.Pressure{Window: "5h", Rate: 0.3, Recent: true, RunsOut: time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC), Under: true},
			heading:  "runs out ~Mon 15:32 at its rate over the last 30 min",
			notes:    "  under pressure: at its reserve ~15:12 at Session's rate over the last 30 min, before its reset at 17:10\n",
		},
		{
			name:     "not under pressure",
			pressure: status.Pressure{Window: "5h", Rate: 0.1, Recent: true, RunsOut: time.Date(2026, 9, 28, 17, 12, 0, 0, time.UTC)},
			heading:  "on pace for 90% at its rate over the last 30 min",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The router gives the session's recent rate where it's the one
			// its pressure goes by.
			var rates []status.Rate
			if tt.pressure.Recent {
				rates = []status.Rate{{Window: "5h", Rate: tt.pressure.Rate, Since: tt.pressure.Since}}
			}
			doc := status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Accounts: []status.Account{{
					ID: "work", Label: "Work", Reserve: tt.reserve, TokenSet: true, FetchedAt: now.UTC(),
					Windows: []quota.Window{session}, Pressure: tt.pressure, Rates: rates,
				}},
			}
			want := "work · Work\n" +
				"  Session  60%  resets in 2h 58m · Mon 17:10 · " + tt.heading + "\n" +
				tt.notes + "\n" +
				"from the router: healthy  ·  no sessions  ·  routing automatically\n"
			if got := doc.Text(now); got != want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestTextProjectsAWeekAtWhicheverRateRunsItOutSooner(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	// The week, 99% used, began three days ago: its use since it started
	// runs it out at 14:55, and 7% an hour at 14:20.
	week := quota.Window{Key: "7d", Label: "Week", Utilization: 0.99, ResetsAt: time.Date(2026, 10, 2, 13, 12, 0, 0, time.UTC)}
	tests := []struct {
		name  string
		rates []status.Rate
		// heading is where the week heads, as its line says.
		heading string
	}{
		{name: "at its rate over the last half hour, sooner", rates: []status.Rate{{Window: "7d", Rate: 0.07}}, heading: "runs out ~Mon 14:20 at its rate over the last 30 min"},
		{
			name:    "at its rate over the 18 minutes it's measured over, sooner",
			rates:   []status.Rate{{Window: "7d", Rate: 0.07, Since: now.Add(-18 * time.Minute)}},
			heading: "runs out ~Mon 14:20 at its rate over the last 18 min",
		},
		{name: "at its use since it started, its recent rate slower", rates: []status.Rate{{Window: "7d", Rate: 0.001}}, heading: "runs out ~Mon 14:55"},
		{name: "without a recent rate", heading: "runs out ~Mon 14:55"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Accounts: []status.Account{{
					ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(),
					Windows: []quota.Window{week}, Pressure: status.Pressure{Window: "5h"}, Rates: tt.rates,
				}},
			}
			want := "work · Work\n" +
				"  Week  99%  resets in 4d · Fri 14:12 · " + tt.heading + "\n\n" +
				"from the router: healthy  ·  no sessions  ·  routing automatically\n"
			if got := doc.Text(now); got != want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestTextListsTheRoutersSessionsGiven(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	doc := status.Document{
		GeneratedAt: now.UTC(),
		Source:      status.SourceRouter,
		Best:        "work",
		Router:      status.Health{Healthy: true, Requests: 4},
		Sessions:    2,
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(), Sessions: 2},
			{ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC(), Sessions: 1},
		},
	}
	sessions := []status.Session{
		{
			ID: "18bb978f-3c2d-4e5f-8a9b-0c1d2e3f4a5b",
			Assignments: []status.Assignment{
				{Model: "claude-sonnet-5-5", Family: "sonnet", Account: "work", LastSeen: now.Add(-20 * time.Second)},
			},
		},
		{
			ID:  "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e",
			Pin: "side",
			Assignments: []status.Assignment{
				{Model: "claude-opus-5-5", Family: "opus", Account: "side", Pinned: true, LastSeen: now.Add(-2 * time.Minute)},
				{Model: "claude-haiku-4-5-20251001", Family: "haiku", Account: "work", LastSeen: now.Add(-50 * time.Minute)},
			},
		},
	}
	want := `work · Work
  2 sessions

side · Side
  1 session

sessions
  18bb978f  sonnet on work  ·  seen just now
  0b5c6f2e  opus on side, haiku on work  ·  pinned to side  ·  seen 2m ago

best next: work · Work
from the router: healthy  ·  2 sessions  ·  routing automatically
`
	if got := doc.Text(now, sessions...); got != want {
		t.Errorf("Text() =\n%s\nwant\n%s", got, want)
	}
	if got, without := doc.Text(now, []status.Session{}...), doc.Text(now); got != without || strings.Contains(got, "sessions\n  ") {
		t.Errorf("Text() given no sessions =\n%s\nwant it as without them, listing none\n%s", got, without)
	}
}

func TestSessionLine(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC)
	const id = "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e"
	on := func(family, model, account string, idle time.Duration) status.Assignment {
		return status.Assignment{Model: model, Family: family, Account: account, LastSeen: now.Add(-idle)}
	}
	tests := []struct {
		name    string
		session status.Session
		want    string
	}{
		{
			name:    "its models on one account together",
			session: status.Session{ID: id, Assignments: []status.Assignment{on("opus", "claude-opus-5-5", "work", 0), on("haiku", "claude-haiku-4-5", "work", time.Hour)}},
			want:    "0b5c6f2e  opus and haiku on work  ·  seen just now",
		},
		{
			name: "its models on two accounts, the one used last first",
			session: status.Session{ID: id, Assignments: []status.Assignment{
				on("haiku", "claude-haiku-4-5", "side", 90*time.Second),
				on("opus", "claude-opus-5-5", "work", 2*time.Minute),
				on("fable", "claude-fable-5-1", "side", 3*time.Minute),
			}},
			want: "0b5c6f2e  haiku and fable on side, opus on work  ·  seen 1m ago",
		},
		{
			name:    "its own pin",
			session: status.Session{ID: id, Pin: "side", Assignments: []status.Assignment{on("opus", "claude-opus-5-5", "work", 59*time.Minute)}},
			want:    "0b5c6f2e  opus on work  ·  pinned to side  ·  seen 59m ago",
		},
		{
			name:    "a family's two models, named once",
			session: status.Session{ID: id, Assignments: []status.Assignment{on("opus", "claude-opus-5-5", "work", time.Hour), on("opus", "claude-opus-4-1", "work", time.Hour)}},
			want:    "0b5c6f2e  opus on work  ·  seen 1h ago",
		},
		{
			name:    "a model without a family, by its id, and one without an id",
			session: status.Session{ID: id, Assignments: []status.Assignment{on("", "claude-next", "work", 0), on("", "", "side", 0)}},
			want:    "0b5c6f2e  claude-next on work, unknown model on side  ·  seen just now",
		},
		{
			name:    "an id shorter than it's cut to",
			session: status.Session{ID: "one", Assignments: []status.Assignment{on("opus", "claude-opus-5-5", "work", 0)}},
			want:    "one  opus on work  ·  seen just now",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.Line(now); got != tt.want {
				t.Errorf("Line() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortID(t *testing.T) {
	for id, want := range map[string]string{
		"0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e": "0b5c6f2e",
		"0b5c6f2e":                             "0b5c6f2e",
		"short":                                "short",
	} {
		if got := status.ShortID(id); got != want {
			t.Errorf("ShortID(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestTextShowsWhatCameFromElsewhereCleaned(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC)
	// clear clears the screen, as a terminal shown it takes it.
	const clear = "\x1b[2J"
	work := status.Account{
		ID: "work", Label: "Work" + clear, TokenSet: true,
		Windows:  []quota.Window{{Key: "5h", Label: "Session" + clear, Utilization: 0.23}},
		Failures: []quota.Failure{{Label: "Fable" + clear, Window: "7d_oi", Error: "HTTP 529 · " + clear + "Overloaded"}},
		Error:    "HTTP 401 · " + clear + "Invalid bearer token",
	}
	running := status.Session{
		ID:          "0b5c" + clear + "6f2e",
		Pin:         "side" + clear,
		Assignments: []status.Assignment{{Model: "claude-next" + clear, Family: "next" + clear, Account: "work" + clear, LastSeen: now}},
	}
	routers := status.Document{
		Source:   status.SourceRouter,
		Best:     "work",
		Pin:      status.Pin{Accounts: []string{"gone" + clear}},
		Router:   status.Health{Reason: clear + "6 of the 8 requests in the last 5 minutes failed"},
		Accounts: []status.Account{work},
	}.Text(now, running)
	shown := map[string]string{
		"the router's document, and its sessions": routers,
		"a session, as its line reads":            running.Line(now),
		"a document probed, as something else answered for the router": status.Document{
			Source:   status.SourceProbe,
			Fallback: status.Fallback{Router: status.RouterUnhealthy, Reason: "answered " + clear},
			Accounts: []status.Account{work},
		}.Text(now),
	}
	for name, text := range shown {
		if strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) {
			t.Errorf("%s reads %q, want no control character but its line ends", name, text)
		}
	}
	want := "work · Work [2J\n" +
		"  Session [2J  23%\n" +
		"  Fable [2J offline: HTTP 529 · [2JOverloaded\n" +
		"  HTTP 401 · [2JInvalid bearer token\n"
	if !strings.HasPrefix(routers, want) {
		t.Errorf("Text() =\n%s\nwant it to start\n%s", routers, want)
	}
	if got, want := running.Line(now), "0b5c [2J  next [2J on work [2J  ·  pinned to side [2J  ·  seen just now"; got != want {
		t.Errorf("Line() = %q, want %q", got, want)
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "Work", want: "Work"},
		{text: "  Work \t team\n", want: "Work team"},
		{text: "Work\x1b[31m team\a", want: "Work [31m team"},
		{text: "\x1b]0;a title\a", want: "]0;a title"},
	}
	for _, tt := range tests {
		if got := status.Clean(tt.text); got != tt.want {
			t.Errorf("Clean(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestSessionCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "no sessions"},
		{n: 1, want: "1 session"},
		{n: 2, want: "2 sessions"},
		{n: 12, want: "12 sessions"},
	}
	for _, tt := range tests {
		if got := status.SessionCount(tt.n); got != tt.want {
			t.Errorf("SessionCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestRouting(t *testing.T) {
	accounts := []status.Account{{ID: "work", Label: "Work"}, {ID: "side", Label: "Side"}}
	tests := []struct {
		name string
		pin  status.Pin
		want string
	}{
		{name: "unpinned", want: "routing automatically"},
		{name: "pinned", pin: status.Pin{Accounts: []string{"side"}}, want: "pinned to side · Side"},
		{name: "pinned, moving running sessions", pin: status.Pin{Accounts: []string{"work"}, Move: true}, want: "pinned to work · Work"},
		{name: "pinned to an account the document lacks", pin: status.Pin{Accounts: []string{"gone"}}, want: "pinned to gone"},
		{name: "pinned to two", pin: status.Pin{Accounts: []string{"work", "side"}}, want: "pinned to work · Work and side · Side"},
		{name: "pinned to three, one the document lacks", pin: status.Pin{Accounts: []string{"work", "gone", "side"}}, want: "pinned to work · Work, gone and side · Side"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{Source: status.SourceRouter, Pin: tt.pin, Accounts: accounts}
			if got := doc.Routing(); got != tt.want {
				t.Errorf("Routing() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDestination(t *testing.T) {
	doc := status.Document{Accounts: []status.Account{{ID: "work", Label: "Work"}, {ID: "side", Label: "Side\x1b[2J"}}}
	tests := []struct {
		ids  []string
		want string
	}{
		{ids: []string{"side"}, want: "side · Side [2J"},
		{ids: []string{"work", "side"}, want: "the best of work · Work and side · Side [2J"},
		{ids: []string{"gone\x07"}, want: "gone"},
	}
	for _, tt := range tests {
		if got := doc.Destination(tt.ids); got != tt.want {
			t.Errorf("Destination(%q) = %q, want %q", tt.ids, got, tt.want)
		}
	}
}

func TestRestart(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	since := time.Date(2026, 9, 28, 12, 2, 0, 0, time.UTC)
	tests := []struct {
		name    string
		restart status.Restart
		want    string
	}{
		{
			name:    "the service's router, waiting for no request in flight",
			restart: status.Restart{Reason: "config changed", Since: since, InFlight: 1},
			want: "restart due since Mon 13:02 (config changed), once no request is in flight (1 now): " +
				"switchboard service restart restarts it now, cutting off requests still in flight after 30 seconds",
		},
		{
			name:    "a router run by hand",
			restart: status.Restart{Reason: "time zone changed", Since: since, ByHand: true},
			want:    "restart due since Mon 13:02 (time zone changed): run switchboard serve again to take it up",
		},
		{
			name:    "a reason from elsewhere, cleaned",
			restart: status.Restart{Reason: "upgraded\x1b[2J", Since: since, ByHand: true},
			want:    "restart due since Mon 13:02 (upgraded [2J): run switchboard serve again to take it up",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.restart.Due() {
				t.Error("Due() = false, want true")
			}
			if got := tt.restart.Text(now); got != tt.want {
				t.Errorf("Text() = %q, want %q, in now's time zone", got, tt.want)
			}
		})
	}
	if (status.Restart{}).Due() {
		t.Error("no restart is due")
	}
}

func TestRefusal(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	until := time.Date(2026, 9, 28, 13, 20, 0, 0, time.UTC)
	tests := []struct {
		name    string
		refusal status.Refusal
		want    string
	}{
		{name: "its token", refusal: status.Refusal{Until: until, Status: 401}, want: "refused (401) until 14:20"},
		{name: "a family's requests", refusal: status.Refusal{Until: until, Status: 403, Family: "opus"}, want: "refused (403, opus) until 14:20"},
		{name: "a family named from elsewhere, cleaned", refusal: status.Refusal{Until: until, Status: 403, Family: "claude-x\x1b[2J"}, want: "refused (403, claude-x [2J) until 14:20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.refusal.Text(now); got != tt.want {
				t.Errorf("Text() = %q, want %q, in now's time zone", got, tt.want)
			}
		})
	}
	refusal := status.Refusal{Until: until, Status: 401}
	for _, tt := range []struct {
		at   time.Time
		want bool
	}{
		{at: now, want: true},
		{at: until.Add(-time.Second), want: true},
		{at: until, want: false},
	} {
		if got := refusal.Holds(tt.at); got != tt.want {
			t.Errorf("Holds(%s) = %v, want %v", tt.at.Format(time.Kitchen), got, tt.want)
		}
	}
	if (status.Refusal{}).Holds(now) {
		t.Error("no refusal holds")
	}
}

func TestLimit(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	limit := status.Limit{Windows: []string{"5h"}, Until: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)}
	if got, want := limit.Text(now), "limit until Mon 21:00"; got != want {
		t.Errorf("Text() = %q, want %q, in now's time zone", got, want)
	}
	for _, tt := range []struct {
		at   time.Time
		want bool
	}{
		{at: now, want: true},
		{at: limit.Until.Add(-time.Second), want: true},
		{at: limit.Until, want: false},
		{at: limit.Until.Add(time.Hour), want: false},
	} {
		if got := limit.Holds(tt.at); got != tt.want {
			t.Errorf("Holds(%s) = %v, want %v", tt.at.Format(time.Kitchen), got, tt.want)
		}
	}
	if (status.Limit{}).Holds(now) {
		t.Error("no limit holds")
	}
}

func TestOver(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC)
	tests := []struct {
		name  string
		since time.Time
		want  string
	}{
		{name: "the half hour", since: now.Add(-score.Recent), want: fmt.Sprintf("last %.0f min", score.Recent.Minutes())},
		{name: "less, without a level that far back", since: now.Add(-18 * time.Minute), want: "last 18 min"},
		{name: "more, across a gap", since: now.Add(-2 * time.Hour), want: "last 2h"},
		{name: "more, and minutes", since: now.Add(-(time.Hour + 30*time.Minute)), want: "last 1h 30m"},
		{name: "not said, the half hour", want: "last 30 min"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Over(tt.since, now); got != tt.want {
				t.Errorf("Over() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProjection(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name       string
		projection score.Projection
		want       string
	}{
		{name: "on pace", projection: score.Projection{Kind: score.OnPace, AtReset: 0.92}, want: "on pace for 92%"},
		{name: "on pace, having used nothing", projection: score.Projection{Kind: score.OnPace}, want: "on pace for 0%"},
		{name: "runs out, in now's time zone", projection: score.Projection{Kind: score.RunsOut, At: time.Date(2026, 10, 2, 18, 40, 0, 0, time.UTC)}, want: "runs out ~Fri 19:40"},
		{name: "exhausted", projection: score.Projection{Kind: score.Exhausted, At: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)}, want: "exhausted"},
		{name: "exhausted, back at an unknown time", projection: score.Projection{Kind: score.Exhausted}, want: "exhausted"},
		{name: "nothing to say", projection: score.Projection{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Projection(now, tt.projection); got != tt.want {
				t.Errorf("Projection(%+v) = %q, want %q", tt.projection, got, tt.want)
			}
		})
	}
}

func TestCountdown(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name  string
		until time.Duration
		want  string
	}{
		{name: "past", until: -time.Minute, want: "now"},
		{name: "due", until: 0, want: "now"},
		{name: "under a minute", until: 30 * time.Second, want: "0m"},
		{name: "minutes, rounded down", until: 7*time.Minute + 59*time.Second, want: "7m"},
		{name: "just under an hour", until: time.Hour - time.Second, want: "59m"},
		{name: "an hour", until: time.Hour, want: "1h"},
		{name: "an hour, and seconds too few for a minute", until: time.Hour + 59*time.Second, want: "1h"},
		{name: "just past an hour", until: time.Hour + time.Minute, want: "1h 1m"},
		{name: "whole hours", until: 4 * time.Hour, want: "4h"},
		{name: "hours and minutes", until: 4*time.Hour + 57*time.Minute + 30*time.Second, want: "4h 57m"},
		{name: "just under a day", until: 24*time.Hour - time.Second, want: "23h 59m"},
		{name: "a day", until: 24 * time.Hour, want: "1d"},
		{name: "a day, and minutes too few for an hour", until: 24*time.Hour + 59*time.Minute, want: "1d"},
		{name: "just past a day", until: 25 * time.Hour, want: "1d 1h"},
		{name: "whole days", until: 6 * 24 * time.Hour, want: "6d"},
		{name: "days and hours, rounded down", until: 5*24*time.Hour + 12*time.Hour + 59*time.Minute, want: "5d 12h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Countdown(now, now.Add(tt.until)); got != tt.want {
				t.Errorf("Countdown(now, now+%v) = %q, want %q", tt.until, got, tt.want)
			}
		})
	}
}

func TestResets(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name  string
		until time.Duration
		want  string
	}{
		{name: "past", until: -time.Minute, want: "resets now"},
		{name: "due", until: 0, want: "resets now"},
		{name: "minutes", until: 7 * time.Minute, want: "resets in 7m"},
		{name: "days and hours", until: 5*24*time.Hour + 12*time.Hour, want: "resets in 5d 12h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Resets(now, now.Add(tt.until)); got != tt.want {
				t.Errorf("Resets(now, now+%v) = %q, want %q", tt.until, got, tt.want)
			}
		})
	}
}

func TestPercent(t *testing.T) {
	tests := []struct {
		utilization float64
		want        string
	}{
		{utilization: 0, want: "0%"},
		{utilization: 0.004, want: "0%"},
		{utilization: 0.23, want: "23%"},
		{utilization: 0.996, want: "100%"},
		{utilization: 1.04, want: "104%"},
	}
	for _, tt := range tests {
		if got := status.Percent(tt.utilization); got != tt.want {
			t.Errorf("Percent(%v) = %q, want %q", tt.utilization, got, tt.want)
		}
	}
}

func TestClock(t *testing.T) {
	utc := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name          string
		now, time     time.Time
		want, wantDay string
	}{
		{name: "afternoon", now: utc, time: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), want: "Mon 18:10", wantDay: "18:10"},
		{name: "just after midnight", now: utc, time: time.Date(2026, 10, 4, 0, 5, 0, 0, time.UTC), want: "Sun 00:05", wantDay: "00:05"},
		{
			name:    "in now's time zone",
			now:     utc.In(time.FixedZone("UTC-7", -7*60*60)),
			time:    time.Date(2026, 9, 29, 6, 30, 0, 0, time.UTC),
			want:    "Mon 23:30",
			wantDay: "23:30",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Clock(tt.now, tt.time); got != tt.want {
				t.Errorf("Clock(%v, %v) = %q, want %q", tt.now, tt.time, got, tt.want)
			}
			if got := status.TimeOfDay(tt.now, tt.time); got != tt.wantDay {
				t.Errorf("TimeOfDay(%v, %v) = %q, want %q", tt.now, tt.time, got, tt.wantDay)
			}
		})
	}
}
