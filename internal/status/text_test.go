package status_test

import (
	"fmt"
	"reflect"
	"slices"
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
			name: "windows heading every way, probed: no account to use next, where new sessions go being the router's to say",
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
  Week        93%  resets in 5d 11h · Sun 02:10 · runs out ~Mon 16:54
  5-hour      23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  Fable offline: HTTP 529 · Overloaded

side · Side
  5-hour       0%  not started
  Fable week 104%  resets in 7m · Mon 14:19 · exhausted
  30d          5%
  burst      100%  exhausted
  limit reached

personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

spare · Spare
  HTTP 401 · Invalid bearer token

probed directly
`,
		},
		{
			name: "the router's, no account to use next, as none has room",
			doc: status.Document{
				GeneratedAt: now.UTC(), Source: status.SourceRouter, Router: status.Health{Healthy: true},
				Accounts: []status.Account{personal, {ID: "spare", Label: "Spare", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"}},
			},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

spare · Spare
  HTTP 401 · Invalid bearer token

from the router: healthy  ·  no sessions  ·  routing automatically
`,
		},
		{
			name: "probed, a lone account to use next, whatever its room, as ▲ marks it",
			doc:  status.Document{GeneratedAt: now.UTC(), Source: status.SourceProbe, Accounts: []status.Account{personal}},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

best next: personal · Personal
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
  Week    40%  resets in 5d 11h · Sun 02:10 · runs out ~Wed 20:15
  5-hour  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  2 sessions

side · Side
  5-hour  60%  resets in 2h 58m · Mon 17:10
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
  5-hour  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  refused (401) until 14:20

side · Side
  5-hour 100%  resets in 6h 48m · Mon 21:00 · exhausted
  limit until Mon 21:00
  refused (403, opus) until 14:21
  1 session

spare · Spare
  5-hour  50%  resets in 2h 58m · Mon 17:10 · runs out ~Mon 16:14

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
  Week    40%  resets in 5d 11h · Sun 02:10 · runs out ~Wed 20:15
  5-hour   0%  not started

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
  Week    40%  resets in 5d 11h · Sun 02:10 · runs out ~Wed 20:15
  5-hour  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%

side · Side
  5-hour   0%  not started · next prime Mon 14:17

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

best next: side · Side
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

best next: personal · Personal
from the router: healthy  ·  no sessions  ·  routing automatically
restart due since Mon 13:12 (upgraded), once no request is in flight (3 now): switchboard service restart restarts it now, cutting off requests still in flight after 30 seconds
`,
		},
		{
			name: "the router's, with a restart held",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Restart:     status.Restart{Reason: "upgraded", Since: now.Add(-time.Hour), InFlight: 3, Held: "invalid config"},
				Accounts:    []status.Account{personal},
			},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

best next: personal · Personal
from the router: healthy  ·  no sessions  ·  routing automatically
restart due since Mon 13:12 (upgraded), held: invalid config
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

best next: personal · Personal
probed directly: the router isn't running
`,
		},
		{
			name: "probed, as the router didn't answer as it should",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 5s"},
				Accounts:    []status.Account{personal},
			},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

best next: personal · Personal
probed directly: the router is unhealthy, no answer within 5s
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
			if got := tt.doc.At(now, policy).Text(); got != tt.want {
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
		// week is work's week's line, at its cap, which it runs out nowhere
		// past, unless the pin spends its reserve; and work are the lines
		// status gives work beside its usage.
		week, work string
	}{
		{
			name: "at its reserve",
			pin:  status.Pin{Accounts: []string{"side"}, Since: now.UTC().Add(-time.Hour)},
			week: "  Week    93%  resets in 5d 11h · Sun 02:10\n",
			work: "  limit until Mon 21:00\n  at its reserve (90%)\n  2 sessions\n",
		},
		{
			name: "spending its reserve, pinned",
			pin:  status.Pin{Accounts: []string{"work"}, Since: now.UTC().Add(-time.Hour)},
			week: "  Week    93%  resets in 5d 11h · Sun 02:10 · runs out ~Mon 16:54\n",
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
				tt.week +
				"  5-hour  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%\n" +
				tt.work + "\n" +
				"side · Side\n" +
				"  5-hour  60%  resets in 2h 58m · Mon 17:10 · runs out ~Mon 15:33\n" +
				"\n" +
				"from the router: healthy  ·  2 sessions  ·  " + doc.Routing() + "\n"
			if got := doc.At(now, policy).Text(); got != want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestTextKeepsItsWordsForAModelsOwnWindowAtItsReserve(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	work := status.Account{
		ID: "work", Label: "Work", Reserve: 0.1, TokenSet: true, FetchedAt: now.UTC(),
		Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)},
			{Key: "7d_oi", Label: "Fable week", Utilization: 0.92, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC)},
		},
		AtReserve: []string{"7d_oi"},
	}
	doc := status.Document{GeneratedAt: now.UTC(), Source: status.SourceRouter, Router: status.Health{Healthy: true}, Accounts: []status.Account{work}}

	if got := doc.At(now, policy).Text(); !strings.Contains(got, "\n  at its reserve (90%)\n") || strings.Contains(got, "other models") {
		t.Errorf("Text() =\n%s\nwant it to say \"at its reserve (90%%)\" as it always has, naming no window", got)
	}
}

func TestReserved(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC)
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.85, ResetsAt: now.Add(time.Hour)}
	atReserve := status.Account{ID: "work", Label: "Work", Reserve: 0.15, Windows: []quota.Window{session}}
	reset := atReserve
	reset.Windows = []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.85, ResetsAt: now}}
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
		{name: "its window reset since it was read", account: reset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{Pin: status.Pin{Accounts: tt.pin}, Accounts: []status.Account{tt.account}}
			if got := doc.Reserved(tt.account, now); got != tt.want {
				t.Errorf("Reserved() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPressed(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	// The session, 40% used, resets at 18:12, having started at 13:12: at its
	// rate over the last half hour, 30% an hour, it runs out at 16:12, and a
	// reserve of a tenth is reached at 15:52.
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: time.Date(2026, 9, 28, 17, 12, 0, 0, time.UTC)}
	pressed := status.Account{
		ID: "work", Windows: []quota.Window{session}, Rates: []status.Rate{{Window: "5h", Rate: 0.3}},
		Pressure: status.Pressure{Window: "5h", Rate: 0.3, Recent: true, RunsOut: time.Date(2026, 9, 28, 15, 12, 0, 0, time.UTC), Under: true},
	}
	tests := []struct {
		name    string
		account status.Account
		// pin are the accounts the global pin names.
		pin  []string
		want string
	}{
		{name: "running out", account: pressed, want: "under pressure: runs out ~16:12"},
		{name: "reaching its reserve", account: with(pressed, reserving), want: "under pressure: at its reserve ~15:52"},
		{name: "running out, its reserve spent by the global pin", account: with(pressed, reserving), pin: []string{"work"}, want: "under pressure: runs out ~16:12"},
		{
			name:    "when its window runs out, whatever time the router gave",
			account: with(pressed, func(a *status.Account) { a.Pressure.RunsOut = time.Date(2026, 9, 28, 17, 21, 0, 0, time.UTC) }),
			want:    "under pressure: runs out ~16:12",
		},
		{
			name:    "under pressure as the router says, its window not running out before it resets at the rate it goes by",
			account: with(pressed, func(a *status.Account) { a.Rates = []status.Rate{{Window: "5h", Rate: 0.05}} }),
			want:    "under pressure",
		},
		{name: "running out after its reset", account: with(pressed, func(a *status.Account) { a.Pressure.Under = false })},
		{
			name: "its window reset since it was read, before it's read again",
			account: with(pressed, func(a *status.Account) {
				a.Windows = []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: now.Add(-time.Second).UTC()}}
			}),
		},
		{name: "its pace unknown", account: status.Account{ID: "work"}},
		{
			name: "held back from one model's requests, under pressure as the router says",
			account: with(pressed, func(a *status.Account) {
				a.Refused = status.Refusal{Until: now.Add(time.Minute), Status: 403, Family: "opus"}
			}),
			want: "under pressure: runs out ~16:12",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{Pin: status.Pin{Accounts: tt.pin}, Accounts: []status.Account{tt.account}}
			if got := doc.Pressed(tt.account, now, policy); got != tt.want {
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
			notes:    "  under pressure: runs out ~15:32 at 5-hour's rate over the last 30 min, before its reset at 17:10\n",
		},
		{
			name: "at its rate over the 18 minutes it's measured over",
			pressure: status.Pressure{
				Window: "5h", Rate: 0.3, Recent: true, Since: now.Add(-18 * time.Minute).UTC(),
				RunsOut: time.Date(2026, 9, 28, 14, 32, 0, 0, time.UTC), Under: true,
			},
			heading: "runs out ~Mon 15:32 at its rate over the last 18 min",
			notes:   "  under pressure: runs out ~15:32 at 5-hour's rate over the last 18 min, before its reset at 17:10\n",
		},
		{
			name:     "at its rate since it started",
			pressure: status.Pressure{Window: "5h", Rate: 0.6 / (2*time.Hour + 2*time.Minute).Hours(), RunsOut: time.Date(2026, 9, 28, 14, 33, 20, 0, time.UTC), Under: true},
			heading:  "runs out ~Mon 15:33",
			notes:    "  under pressure: runs out ~15:33 at 5-hour's rate since it started, before its reset at 17:10\n",
		},
		{
			name:     "reaching its reserve",
			reserve:  0.1,
			pressure: status.Pressure{Window: "5h", Rate: 0.3, Recent: true, RunsOut: time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC), Under: true},
			heading:  "runs out ~Mon 15:12 at its rate over the last 30 min",
			notes:    "  under pressure: at its reserve ~15:12 at 5-hour's rate over the last 30 min, before its reset at 17:10\n",
		},
		{
			name:     "under pressure as the router says, its window not running out before it resets at the rate it goes by: nothing of why",
			pressure: status.Pressure{Window: "5h", Rate: 0.05, Recent: true, RunsOut: time.Date(2026, 9, 28, 14, 32, 0, 0, time.UTC), Under: true},
			heading:  "on pace for 75% at its rate over the last 30 min",
			notes:    "  under pressure\n",
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
				"  5-hour  60%  resets in 2h 58m · Mon 17:10 · " + tt.heading + "\n" +
				tt.notes + "\n" +
				"best next: work · Work\n" +
				"from the router: healthy  ·  no sessions  ·  routing automatically\n"
			if got := doc.At(now, policy).Text(); got != want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// status judges a session at now, as the dashboard does: read running and
// under pressure, on the tick after it resets, before it's read again, it's
// empty and not started, and nothing says it's under pressure.
func TestTextSaysASessionThatResetSinceItWasReadHasntStarted(t *testing.T) {
	read := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	reset := time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)
	doc := status.Document{
		GeneratedAt: read.UTC(),
		Source:      status.SourceRouter,
		Router:      status.Health{Healthy: true},
		Accounts: []status.Account{{
			ID: "work", Label: "Work", TokenSet: true, FetchedAt: read.UTC(),
			Windows:  []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: reset}},
			Pressure: status.Pressure{Window: "5h", Rate: 0.3, Recent: true, RunsOut: time.Date(2026, 9, 28, 14, 32, 0, 0, time.UTC), Under: true},
		}},
	}
	want := "work · Work\n" +
		"  5-hour   0%  not started\n" +
		"\n" +
		"best next: work · Work\n" +
		"from the router: healthy  ·  no sessions  ·  routing automatically\n"
	if got := doc.At(reset.Add(time.Second).In(read.Location()), policy).Text(); got != want {
		t.Errorf("Text() =\n%s\nwant\n%s", got, want)
	}
}

// status says a window hasn't started where the dashboard draws it so, as
// WindowStates stand it, in ROUTING's order: read running, on the tick after
// its session resets, before it's read again, an account the router's limit
// holds in its session has it held, and one whose token is refused has it
// stale, each reading empty, with no reset, as it has lapsed, as the router's
// next document has it; while one whose last read failed, nothing read since
// the reset, has it not started.
func TestTextSaysASessionLapsedAtNowHasntStartedUnlessALimitOrARefusalComesFirst(t *testing.T) {
	read := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	reset := time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)
	now := reset.Add(time.Second).In(read.Location())
	tests := []struct {
		name   string
		change func(*status.Account)
		want   string
	}{
		{
			name: "held by the router's limit",
			change: func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"5h"}, Until: reset.Add(40 * time.Minute)}
			},
			want: "  5-hour   0%\n  limit until Mon 17:50\n",
		},
		{
			name:   "its last read failed",
			change: func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" },
			want:   "  5-hour   0%  not started\n  HTTP 529 · Overloaded\n",
		},
		{
			name:   "its token refused",
			change: func(a *status.Account) { a.Refused = status.Refusal{Until: reset.Add(time.Hour), Status: 401} },
			want:   "  5-hour   0%\n  refused (401) until 18:10\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := status.Account{
				ID: "work", Label: "Work", TokenSet: true, FetchedAt: read.UTC(),
				Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.97, ResetsAt: reset}},
			}
			tt.change(&a)
			doc := status.Document{GeneratedAt: read.UTC(), Source: status.SourceRouter, Router: status.Health{Healthy: true}, Accounts: []status.Account{a}}
			want := "work · Work\n" + tt.want + "\n" +
				"best next: work · Work\n" +
				"from the router: healthy  ·  no sessions  ·  routing automatically\n"
			if got := doc.At(now, policy).Text(); got != want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// status gives a window's reset as it stands at now, beside its use: work,
// its token refused, has its week, read before it reset and not read since,
// empty, and resetting next a week on, not at the reset already past.
func TestTextGivesAWeekResetSinceItWasReadItsNextReset(t *testing.T) {
	read := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	reset := read.Add(time.Hour)
	doc := status.Document{
		GeneratedAt: read.UTC(),
		Source:      status.SourceRouter,
		Router:      status.Health{Healthy: true},
		Accounts: []status.Account{{
			ID: "work", Label: "Work", TokenSet: true, FetchedAt: read.UTC(),
			Windows: []quota.Window{{Key: "7d", Label: "Week", Utilization: 0.6, ResetsAt: reset.UTC()}},
			Refused: status.Refusal{Until: reset.Add(time.Hour).UTC(), Status: 401},
		}},
	}
	want := "work · Work\n" +
		"  Week   0%  resets in 6d 23h · Mon 15:12\n" +
		"  refused (401) until 16:12\n"
	if got := doc.At(reset.Add(time.Second), policy).Text(); !strings.HasPrefix(got, want) {
		t.Errorf("Text() =\n%s\nwant it to start\n%s", got, want)
	}
}

// status says a session the router notes has lapsed hasn't started, and when
// it's primed, though the account's last probe failed: the router doesn't
// probe a lapsed account, so its error stands until the prime, and nothing has
// been read since the session reset.
func TestTextSaysASessionNotedLapsedHasntStartedThoughItsProbeFailed(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	doc := status.Document{
		GeneratedAt: now.UTC(),
		Source:      status.SourceRouter,
		Router:      status.Health{Healthy: true},
		Prime:       status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{{Account: "work", At: "16:20", Next: time.Date(2026, 9, 28, 15, 20, 0, 0, time.UTC)}}},
		Accounts: []status.Account{{
			ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.Add(-2 * time.Hour).UTC(), Error: "HTTP 529 · Overloaded",
			Windows: []quota.Window{{Key: "5h", Label: "Session"}},
			Lapsed:  []string{"5h"},
		}},
	}
	want := "work · Work\n" +
		"  5-hour   0%  not started · next prime Mon 16:20\n" +
		"  HTTP 529 · Overloaded\n"
	if got := doc.At(now, policy).Text(); !strings.HasPrefix(got, want) {
		t.Errorf("Text() =\n%s\nwant it to start\n%s", got, want)
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
				"best next: work · Work\n" +
				"from the router: healthy  ·  no sessions  ·  routing automatically\n"
			if got := doc.At(now, policy).Text(); got != want {
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
	if got := doc.At(now, policy).Text(sessions...); got != want {
		t.Errorf("Text() =\n%s\nwant\n%s", got, want)
	}
	if got, without := doc.At(now, policy).Text([]status.Session{}...), doc.At(now, policy).Text(); got != without || strings.Contains(got, "sessions\n  ") {
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

func TestASessionsModelsByAccount(t *testing.T) {
	on := func(family, model, account string) status.Assignment {
		return status.Assignment{Model: model, Family: family, Account: account}
	}
	tests := []struct {
		name    string
		session status.Session
		want    []status.Models
	}{
		{name: "none", session: status.Session{}},
		{
			name: "the account used last first, and of each, the model used last first, a family once",
			session: status.Session{Assignments: []status.Assignment{
				on("haiku", "claude-haiku-4-5", "side"), on("opus", "claude-opus-5-5", "work"),
				on("fable", "claude-fable-5-1", "side"), on("haiku", "claude-haiku-3-5", "side"),
			}},
			want: []status.Models{{Account: "side", Names: []string{"haiku", "fable"}}, {Account: "work", Names: []string{"opus"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.ByAccount(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ByAccount() = %+v, want %+v", got, tt.want)
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

func TestTheDashboardNamesASessionByTheFirstFourCharactersOfItsID(t *testing.T) {
	for id, want := range map[string]string{
		"c61b2f8d-04a7-4e93": "c61b",
		"c61b":               "c61b",
		"ab":                 "ab",
		"\x1b[2Jc61b2f8d":    "[2Jc",
		"éé☃☃d":              "éé☃☃",
		"":                   "",
	} {
		if got := status.ShownID(id); got != want {
			t.Errorf("ShownID(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestASessionsDirectoryIsNamedByItsLastPartCutKeepingItsStart(t *testing.T) {
	tests := []struct {
		dir   string
		cells int
		want  string
	}{
		{dir: "/Users/someone/Code/switchboard", cells: 12, want: "switchboard"},
		{dir: "/Users/someone/Code/switchboard-dashboard", cells: 12, want: "switchboard…"},
		{dir: "/Users/someone/Code/switchboard-dashboard", cells: 11, want: "switchboar…"},
		{dir: "/Users/someone/Code/switchboard/", cells: 12, want: "switchboard"},
		{dir: "/Users/someone/Code/コードの部屋", cells: 12, want: "コードの部屋"},
		{dir: "/Users/someone/Code/コードの部屋です", cells: 12, want: "コードの部…"},
		{dir: "/Users/someone/Code/switch\x1b[2Jboard", cells: 20, want: "switch [2Jboard"},
		{dir: "/", cells: 12, want: "/"},
		{dir: "", cells: 12, want: ""},
	}
	for _, tt := range tests {
		if got := status.DirName(tt.dir, tt.cells); got != tt.want {
			t.Errorf("DirName(%q, %d) = %q, want %q", tt.dir, tt.cells, got, tt.want)
		}
	}
}

func TestASessionsDirectoryIsShownWholeTheHomeAsATildeCutFromItsStart(t *testing.T) {
	const home = "/Users/someone"
	tests := []struct {
		name, dir, home string
		cells           int
		want            string
	}{
		{name: "under the home", dir: "/Users/someone/Code/switchboard", home: home, cells: 40, want: "~/Code/switchboard"},
		{name: "the home itself", dir: "/Users/someone", home: home, cells: 40, want: "~"},
		{name: "the home given with a slash", dir: "/Users/someone/Code", home: home + "/", cells: 40, want: "~/Code"},
		{name: "a home the directory only starts like", dir: "/Users/someone2/Code", home: home, cells: 40, want: "/Users/someone2/Code"},
		{name: "outside the home", dir: "/opt/work/switchboard", home: home, cells: 40, want: "/opt/work/switchboard"},
		{name: "no home known", dir: "/Users/someone/Code", home: "", cells: 40, want: "/Users/someone/Code"},
		{name: "long, cut from its start keeping its name", dir: "/Users/someone/Code/clients/switchboard", home: home, cells: 13, want: "…/switchboard"},
		{name: "none", dir: "", home: home, cells: 40, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.DirPath(tt.dir, tt.home, tt.cells); got != tt.want {
				t.Errorf("DirPath(%q, %q, %d) = %q, want %q", tt.dir, tt.home, tt.cells, got, tt.want)
			}
		})
	}
}

func TestTextShowsWhatCameFromElsewhereCleaned(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC)
	// clear clears the screen, as a terminal shown it takes it.
	const clear = "\x1b[2J"
	work := status.Account{
		ID: "work", Label: "Work" + clear, TokenSet: true, FetchedAt: now.Add(-time.Minute),
		Windows:  []quota.Window{{Key: "7d", Label: "Week" + clear, Utilization: 0.23}},
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
	}.At(now, policy).Text(running)
	shown := map[string]string{
		"the router's document, and its sessions": routers,
		"a session, as its line reads":            running.Line(now),
		"a document probed, as something else answered for the router": status.Document{
			Source:   status.SourceProbe,
			Fallback: status.Fallback{Router: status.RouterUnhealthy, Reason: "answered " + clear},
			Accounts: []status.Account{work},
		}.At(now, policy).Text(),
	}
	for name, text := range shown {
		if strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) {
			t.Errorf("%s reads %q, want no control character but its line ends", name, text)
		}
	}
	want := "work · Work [2J\n" +
		"  Week [2J  23%\n" +
		"  Fable [2J offline: HTTP 529 · [2JOverloaded\n" +
		"  HTTP 401 · [2JInvalid bearer token\n"
	if !strings.HasPrefix(routers, want) {
		t.Errorf("Text() =\n%s\nwant it to start\n%s", routers, want)
	}
	if got, want := running.Line(now), "0b5c [2J  next [2J on work [2J  ·  pinned to side [2J  ·  seen just now"; got != want {
		t.Errorf("Line() = %q, want %q", got, want)
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
		if got := doc.At(built, policy).Destination(tt.ids); got != tt.want {
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
		{
			name:    "held, its binary refusing to start, which neither waiting nor service restart gets past",
			restart: status.Restart{Reason: "upgraded", Since: since, InFlight: 1, Held: "invalid config /test/config.toml:\naccount work: its label is longer than 100 characters"},
			want:    "restart due since Mon 13:02 (upgraded), held: invalid config /test/config.toml: account work: its label is longer than 100 characters",
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

func TestARestartsLabelSaysWhetherItsDueOrHeldAndWhy(t *testing.T) {
	tests := []struct {
		name    string
		restart status.Restart
		want    string
	}{
		{name: "due", restart: status.Restart{Reason: "config changed", InFlight: 3}, want: "restart due (config changed)"},
		{name: "held, its binary refusing to start", restart: status.Restart{Reason: "upgraded", Held: "invalid config"}, want: "restart held (upgraded)"},
		{name: "held for nothing to be read, as due", restart: status.Restart{Reason: "upgraded", Held: "\x1b"}, want: "restart due (upgraded)"},
		{name: "a reason from elsewhere, cleaned", restart: status.Restart{Reason: "upgraded\x1b[2J"}, want: "restart due (upgraded [2J)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.restart.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A refusal's answer is the one form whoever tells of it: its status code,
// then the family refused alone, cleaned, either left out where it's none.
func TestARefusalsAnswerIsItsCodeThenItsFamily(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		family string
		want   string
	}{
		{name: "its token", code: 401, want: "refused (401)"},
		{name: "a family's requests", code: 403, family: "opus", want: "refused (403, opus)"},
		{name: "a family named from elsewhere, cleaned", code: 403, family: "claude-x\x1b[2J", want: "refused (403, claude-x [2J)"},
		{name: "a family that cleans to nothing", code: 403, family: "\x1b", want: "refused (403)"},
		{name: "a family, no code", family: "opus", want: "refused (opus)"},
		{name: "neither", want: "refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.RefusalAnswer(tt.code, tt.family); got != tt.want {
				t.Errorf("RefusalAnswer(%d, %q) = %q, want %q", tt.code, tt.family, got, tt.want)
			}
			if got := (status.Refusal{Status: tt.code, Family: tt.family}).Answer(); got != tt.want {
				t.Errorf("Refusal.Answer() = %q, want %q", got, tt.want)
			}
		})
	}
}

// When an account's back, and till when what holds it holds, are said as
// Dated shows a time, in now's time zone; neither where it's unknown.
func TestBackAndTillSayATimeAsDatedShowsIt(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name       string
		at         time.Time
		back, till string
	}{
		{name: "today", at: time.Date(2026, 9, 28, 14, 54, 0, 0, time.UTC), back: "back 15:54", till: "till 15:54"},
		{name: "another day", at: time.Date(2026, 9, 30, 19, 0, 0, 0, time.UTC), back: "back Wed 20:00", till: "till Wed 20:00"},
		{name: "unknown", back: "", till: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Back(now, tt.at); got != tt.back {
				t.Errorf("Back() = %q, want %q", got, tt.back)
			}
			if got := status.Till(now, tt.at); got != tt.till {
				t.Errorf("Till() = %q, want %q", got, tt.till)
			}
		})
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

// status's text says a limit holding an account back as the moment's hold
// has it, whatever the router's limit says, or where there's none, as
// probing reads it: of one holding back every request, until the account's
// back from every hold, its cap and its token's refusal too; of one holding
// back some models' alone, until it lifts; and that it's reached alone where
// that isn't known.
func TestTextSaysALimitTillTheAccountsBack(t *testing.T) {
	now := built.In(time.FixedZone("UTC+1", 60*60))
	routerLimit := func(windows ...string) func(*status.Account) {
		return func(a *status.Account) {
			a.Limit = status.Limit{ID: 4, Windows: windows, Until: built.Add(40 * time.Minute)}
		}
	}
	refused := func(a *status.Account) { a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401} }
	tests := []struct {
		name string
		doc  status.Document
		// want are the lines the text gives work of what holds it back.
		want []string
	}{
		{
			name: "the router's limit, till it lifts",
			doc:  routerDocOf(with(readAt("work", spentIn("5h", 1, 40*time.Minute), windowIn("7d", 0.3, 3*day)), routerLimit("5h"))),
			want: []string{"limit until Mon 14:52"},
		},
		{
			name: "a limit, its cap holding it back longer: till its cap lets it go",
			doc: routerDocOf(with(readAt("work", windowIn("5h", 0.5, 3*time.Hour), windowIn("7d", 0.85, 70*time.Minute)), func(a *status.Account) {
				a.Reserve = 0.2
				routerLimit()(a)
			})),
			want: []string{"limit until Mon 15:22"},
		},
		{
			name: "a limit, its token refused longer: till the refusal ends",
			doc:  routerDocOf(with(with(readAt("work", spentIn("5h", 1, 40*time.Minute), windowIn("7d", 0.3, 3*day)), routerLimit("5h")), refused)),
			want: []string{"limit until Mon 15:12", "refused (401) until 15:12"},
		},
		{
			name: "the router's limit, its week read spent too: till the week resets",
			doc:  routerDocOf(with(readAt("work", spentIn("5h", 1, 40*time.Minute), spentIn("7d", 1, 2*day)), routerLimit("5h"))),
			want: []string{"limit until Wed 14:12"},
		},
		{
			name: "probed, its week read spent: till it resets",
			doc:  status.Document{GeneratedAt: built, Source: status.SourceProbe, Accounts: []status.Account{readAt("work", windowIn("5h", 0.3, 3*time.Hour), spentIn("7d", 1, 2*day))}},
			want: []string{"limit until Wed 14:12"},
		},
		{
			name: "a model's own week at its limit, its cap holding back the rest longer: till the limit lifts",
			doc: routerDocOf(with(readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.95, 3*day), spentIn("7d_oi", 1, 2*day)), func(a *status.Account) {
				a.Reserve = 0.1
				routerLimit("7d_oi")(a)
			})),
			want: []string{"limit until Wed 14:12"},
		},
		{
			name: "its session read spent, when it resets not known",
			doc:  routerDocOf(readAt("work", quota.Window{Key: "5h", Label: "Session", Utilization: 1, Status: quota.StatusRejected}, windowIn("7d", 0.3, 3*day))),
			want: []string{"limit reached"},
		},
		{
			name: "the router's limit lifted",
			doc: routerDocOf(with(readAt("work", windowIn("5h", 0.3, 3*time.Hour)), func(a *status.Account) {
				a.Limit = status.Limit{Until: built.Add(-time.Minute)}
			})),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, l := range textOf(tt.doc.At(now, policy))["work"] {
				if strings.HasPrefix(l, "limit ") || strings.HasPrefix(l, "refused ") {
					got = append(got, l)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the text says %q of what holds work back, want %q", got, tt.want)
			}
		})
	}
}

func TestLimit(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	limit := status.Limit{Windows: []string{"5h"}, Until: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)}
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

// status's text says where a window is heading as its moment has it: where
// it runs out at Now, at its Out, as runOutOf owns it, so where its cap
// starts where its reserve holds, its projection short of its limit; else
// where its projection has it, but never that it runs out, as of a window
// held, stale, at its cap, not started, or whose run-out the clock has
// reached, whatever its projection says.
func TestProjection(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	out := time.Date(2026, 10, 2, 18, 40, 0, 0, time.UTC)
	heading := func(p score.Projection) status.Heading { return status.Heading{Projection: p} }
	runsOut := heading(score.Projection{Kind: score.RunsOut, At: out})
	tests := []struct {
		name   string
		window status.WindowNow
		want   string
	}{
		{name: "on pace", window: status.WindowNow{Out: status.RunOut{Heading: heading(score.Projection{Kind: score.OnPace, AtReset: 0.92})}}, want: "on pace for 92%"},
		{name: "on pace, having used nothing", window: status.WindowNow{Out: status.RunOut{Heading: heading(score.Projection{Kind: score.OnPace})}}, want: "on pace for 0%"},
		{name: "running out, in now's time zone", window: status.WindowNow{RunsOut: true, Out: status.RunOut{At: out, Heading: runsOut}}, want: "runs out ~Fri 19:40"},
		{
			name:   "running out where its cap starts, its projection short of its limit",
			window: status.WindowNow{RunsOut: true, Out: status.RunOut{At: out, Reserve: true, Heading: heading(score.Projection{Kind: score.OnPace, AtReset: 0.95})}},
			want:   "runs out ~Fri 19:40",
		},
		{name: "its projection running out, the window not, as one held, stale or at its cap", window: status.WindowNow{Out: status.RunOut{Heading: runsOut}}, want: ""},
		{name: "exhausted", window: status.WindowNow{Out: status.RunOut{Heading: heading(score.Projection{Kind: score.Exhausted, At: out})}}, want: "exhausted"},
		{name: "exhausted, back at an unknown time", window: status.WindowNow{Out: status.RunOut{Heading: heading(score.Projection{Kind: score.Exhausted})}}, want: "exhausted"},
		{name: "nothing to say", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Projection(now, tt.window); got != tt.want {
				t.Errorf("Projection(%+v) = %q, want %q", tt.window, got, tt.want)
			}
		})
	}
}

func TestWindowLabelsInWords(t *testing.T) {
	tests := []struct {
		label, prose, short string
	}{
		{label: "Session", prose: "session", short: "Session"},
		{label: "Week", prose: "week", short: "Week"},
		{label: "Fable week", prose: "Fable week", short: "Fable wk"},
		{label: "Opus\x1b[2J  week", prose: "Opus [2J week", short: "Opus [2J wk"},
		{label: "", prose: "", short: ""},
	}
	for _, tt := range tests {
		if got := status.InProse(tt.label); got != tt.prose {
			t.Errorf("InProse(%q) = %q, want %q", tt.label, got, tt.prose)
		}
		if got := status.Short(tt.label); got != tt.short {
			t.Errorf("Short(%q) = %q, want %q", tt.label, got, tt.short)
		}
	}
}
