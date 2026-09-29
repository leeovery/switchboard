package status_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// policy scores the windows as Claude's are: the session and the week apply
// to every model, the week is perishable, and the session's reset decides
// between accounts scoring near enough equal.
var policy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h"}

func TestCollect(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	workUsage := quota.Usage{
		Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)},
			{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC)},
		},
		Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
	}
	prober := &fakeProber{results: map[string]probeResult{
		"test-token-work": {usage: workUsage},
		"test-token-side": {err: errors.New("HTTP 401 · Invalid bearer token")},
	}}
	collector := status.Collector{
		Prober: prober,
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": " test-token-side\n"}.Read,
		Now:    func() time.Time { return now },
	}

	got := collector.Collect(t.Context(), accounts)
	want := status.Document{
		GeneratedAt: now.UTC(),
		Source:      "probe",
		Best:        "work",
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(), Usage: workUsage},
			{ID: "personal", Label: "Personal", TokenSet: false, Error: tokenstest.Missing("personal").Error()},
			{ID: "side", Label: "Side", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Collect() =\n%+v\nwant\n%+v", got, want)
	}
	if got, want := prober.probed(), []string{"test-token-side", "test-token-work"}; !slices.Equal(got, want) {
		t.Errorf("probed tokens %q, want %q", got, want)
	}
}

func TestCollectGivesThePrimaryAndTheWindowsAtEachReserve(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: now.Add(5 * time.Hour)}
	week := func(utilization float64) quota.Window {
		return quota.Window{Key: "7d", Label: "Week", Utilization: utilization, ResetsAt: now.Add(24 * time.Hour)}
	}
	collector := status.Collector{
		Prober: &fakeProber{results: map[string]probeResult{
			"test-token-work": withWindows(session, week(0.93)),
			"test-token-side": withWindows(session, week(0.97)),
		}},
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
		Now:    func() time.Time { return now },
	}
	accounts := []config.Account{
		{ID: "work", Label: "Work", Primary: true, Reserve: 0.1},
		{ID: "personal", Label: "Personal", Reserve: 0.05},
		{ID: "side", Label: "Side"},
	}

	doc := collector.Collect(t.Context(), accounts)
	if doc.Primary != "work" {
		t.Errorf("Collect().Primary = %q, want work, the account marked primary", doc.Primary)
	}
	want := map[string]status.Account{
		"work":     {ID: "work", Label: "Work", Primary: true, Reserve: 0.1, AtReserve: []string{"7d"}},
		"personal": {ID: "personal", Label: "Personal", Reserve: 0.05},
		"side":     {ID: "side", Label: "Side"},
	}
	for _, got := range doc.Accounts {
		w := want[got.ID]
		if got.Primary != w.Primary || got.Reserve != w.Reserve || !slices.Equal(got.AtReserve, w.AtReserve) {
			t.Errorf("account %s is primary %v, reserve %v, at its reserve in %q; want %v, %v, %q",
				got.ID, got.Primary, got.Reserve, got.AtReserve, w.Primary, w.Reserve, w.AtReserve)
		}
	}
	if doc.Best != "side" {
		t.Errorf("Collect().Best = %q, want side: work's quota would need using first, but its week has reached its reserve", doc.Best)
	}
}

func TestCollectLogsEachAccount(t *testing.T) {
	log := logstest.Capture(t)
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	collector := status.Collector{
		Prober: &fakeProber{results: map[string]probeResult{
			"test-token-work": {usage: quota.Usage{
				Windows: []quota.Window{
					{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: now.Add(5 * time.Hour)},
					{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: now.Add(72 * time.Hour)},
				},
				Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
			}},
			"test-token-side": {err: errors.New("HTTP 401 · Invalid bearer token")},
		}},
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
		Now:    func() time.Time { return now },
	}

	collector.Collect(t.Context(), accounts)
	for _, want := range [][]string{
		{"level=DEBUG", `msg="probed account" component=status`, "account=work", "duration=", "windows=2"},
		{"level=WARN", `msg="window unread" component=status`, "account=work", "window=7d_oi", `error="HTTP 529 · Overloaded"`},
		{"level=DEBUG", `msg="not probed: no usable token" component=status`, "account=personal", `error="` + tokenstest.Missing("personal").Error() + `"`},
		{"level=WARN", `msg="probe failed" component=status`, "account=side", "duration=", `error="HTTP 401 · Invalid bearer token"`},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	for _, private := range []string{"test-token-work", "test-token-side", "Work", "Personal", "Side"} {
		if strings.Contains(log.String(), private) {
			t.Errorf("log reads\n%s\nwant no tokens or labels, only ids, but it has %q", log, private)
		}
	}
}

func TestCollectProbesAccountsAtOnce(t *testing.T) {
	collector := status.Collector{
		Prober: &gatheringProber{waitFor: len(accounts), gathered: make(chan struct{})},
		Token:  tokenstest.Files{"work": "test-token-work", "personal": "test-token-personal", "side": "test-token-side"}.Read,
		Now:    time.Now,
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	for _, account := range collector.Collect(ctx, accounts).Accounts {
		if account.Error != "" {
			t.Errorf("account %s: %s", account.ID, account.Error)
		}
	}
}

func TestCollectBest(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: now.Add(3 * time.Hour)}
	sessionEndingSooner := session
	sessionEndingSooner.ResetsAt = now.Add(time.Hour)
	week := func(utilization float64, resetsIn time.Duration) quota.Window {
		return quota.Window{Key: "7d", Label: "Week", Utilization: utilization, ResetsAt: now.Add(resetsIn)}
	}
	weekRefused := week(1, -time.Minute)
	weekRefused.Status = quota.StatusRejected
	fableRefused := quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 1, ResetsAt: now.Add(48 * time.Hour), Status: quota.StatusRejected}
	unreadable := probeResult{err: errors.New("HTTP 401 · Invalid bearer token")}
	tests := []struct {
		name       string
		work, side probeResult
		// workReserve is the share of work's every window left unused.
		workReserve float64
		want        string
	}{
		{
			name: "the account whose week resets soonest",
			work: withWindows(session, week(0.5, 5*24*time.Hour)),
			side: withWindows(session, week(0.5, 24*time.Hour)),
			want: "side",
		},
		{
			name: "of two scoring near enough equal, the one whose session resets soonest",
			work: withWindows(session, week(0.5, 50*time.Hour)),
			side: withWindows(sessionEndingSooner, week(0.55, 50*time.Hour)),
			want: "side",
		},
		{
			name:        "passing over an account at its reserve",
			work:        withWindows(session, week(0.9, 24*time.Hour)),
			side:        withWindows(session, week(0.5, 5*24*time.Hour)),
			workReserve: 0.1,
			want:        "side",
		},
		{
			name:        "judging an account by the room before its reserve",
			work:        withWindows(session, week(0.5, 24*time.Hour)),
			side:        withWindows(session, week(0.5, 30*time.Hour)),
			workReserve: 0.2,
			want:        "side",
		},
		{
			name: "judged on the windows every model shares",
			work: withWindows(session, week(0.5, 24*time.Hour), fableRefused),
			side: withWindows(session, week(0.5, 5*24*time.Hour)),
			want: "work",
		},
		{
			name: "a refused week that has reset by the clock",
			work: withWindows(session, weekRefused),
			side: unreadable,
			want: "work",
		},
		{
			name: "none when no account can take a request",
			work: withWindows(session, week(1, 24*time.Hour)),
			side: unreadable,
			want: "",
		},
		{
			name: "none when no account could be read",
			work: unreadable,
			side: unreadable,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := status.Collector{
				Prober: &fakeProber{results: map[string]probeResult{"test-token-work": tt.work, "test-token-side": tt.side}},
				Policy: policy,
				Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
				Now:    func() time.Time { return now },
			}
			accounts := []config.Account{{ID: "work", Label: "Work", Reserve: tt.workReserve}, {ID: "side", Label: "Side"}}

			if got := collector.Collect(t.Context(), accounts).Best; got != tt.want {
				t.Errorf("Collect().Best = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDocumentJSON(t *testing.T) {
	generated := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{
			name: "with an account to use next",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Best:        "work",
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: generated,
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed},
							{Key: "7d", Label: "Week", Utilization: 0.93},
						},
						Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
					},
					{ID: "personal", Label: "Personal", Error: "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"},
					{ID: "side", Label: "Side", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "best": "work",
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "fetched_at": "2026-09-28T13:12:00Z",
      "windows": [
        {
          "key": "5h",
          "label": "Session",
          "utilization": 0.23,
          "resets_at": "2026-09-28T18:10:00Z",
          "status": "allowed"
        },
        {
          "key": "7d",
          "label": "Week",
          "utilization": 0.93
        }
      ],
      "failures": [
        {
          "label": "Fable",
          "window": "7d_oi",
          "error": "HTTP 529 · Overloaded"
        }
      ]
    },
    {
      "id": "personal",
      "label": "Personal",
      "token_set": false,
      "error": "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true,
      "error": "HTTP 401 · Invalid bearer token"
    }
  ]
}`,
		},
		{
			name: "without one",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Accounts:    []status.Account{{ID: "personal", Label: "Personal", Error: "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "accounts": [
    {
      "id": "personal",
      "label": "Personal",
      "token_set": false,
      "error": "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"
    }
  ]
}`,
		},
		{
			name: "probed, as the router isn't running",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterNotRunning},
				Accounts:    []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "fallback": {
    "router": "not running"
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "probed, as the router didn't answer as it should",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"},
				Accounts:    []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "fallback": {
    "router": "unhealthy",
    "reason": "no answer within 500ms"
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "the router's, with its pin, its health and the sessions",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Pin:         status.Pin{Account: "side", Since: generated.Add(-time.Hour), Move: true},
				Router:      status.Health{Requests: 8, Failures: 6, Reason: "6 of the 8 requests in the last 5 minutes failed"},
				Sessions:    2,
				Accounts: []status.Account{
					{ID: "work", Label: "Work", TokenSet: true, Sessions: 2},
					{ID: "side", Label: "Side", TokenSet: true, Limit: status.Limit{Windows: []string{"5h"}, Until: generated.Add(2 * time.Hour)}},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "pin": {
    "account": "side",
    "since": "2026-09-28T12:12:00Z",
    "move": true
  },
  "router": {
    "healthy": false,
    "requests": 8,
    "failures": 6,
    "reason": "6 of the 8 requests in the last 5 minutes failed"
  },
  "sessions": 2,
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "sessions": 2
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true,
      "limit": {
        "windows": [
          "5h"
        ],
        "until": "2026-09-28T15:12:00Z"
      }
    }
  ]
}`,
		},
		{
			name: "the router's, with refusals",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true, Requests: 2},
				Accounts: []status.Account{
					{ID: "work", Label: "Work", TokenSet: true, Refused: status.Refusal{Until: generated.Add(10 * time.Minute), Status: 401}},
					{ID: "side", Label: "Side", TokenSet: true, Refused: status.Refusal{Until: generated.Add(9 * time.Minute), Status: 403, Family: "opus"}},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "router": {
    "healthy": true,
    "requests": 2,
    "failures": 0
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "refused": {
        "until": "2026-09-28T13:22:00Z",
        "status": 401
      }
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true,
      "refused": {
        "until": "2026-09-28T13:21:00Z",
        "status": 403,
        "family": "opus"
      }
    }
  ]
}`,
		},
		{
			name: "with the primary, its reserve, and the windows that have reached it",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Primary:     "work",
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", Primary: true, Reserve: 0.1, TokenSet: true, FetchedAt: generated,
						Windows:   []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.95, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)}},
						AtReserve: []string{"5h"},
					},
					{ID: "side", Label: "Side", TokenSet: true},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "primary": "work",
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "primary": true,
      "reserve": 0.1,
      "token_set": true,
      "fetched_at": "2026-09-28T13:12:00Z",
      "windows": [
        {
          "key": "5h",
          "label": "Session",
          "utilization": 0.95,
          "resets_at": "2026-09-28T18:10:00Z"
        }
      ],
      "at_reserve": [
        "5h"
      ]
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "the router's, healthy",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true, Requests: 12, Failures: 1},
				Accounts:    []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "router": {
    "healthy": true,
    "requests": 12,
    "failures": 1
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    }
  ]
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.MarshalIndent(tt.doc, "", "  ")
			if err != nil {
				t.Fatalf("MarshalIndent() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("MarshalIndent() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

type probeResult struct {
	usage quota.Usage
	err   error
}

// withWindows is a probe that reads windows.
func withWindows(windows ...quota.Window) probeResult {
	return probeResult{usage: quota.Usage{Windows: windows}}
}

// fakeProber answers each token with its result, and records the tokens it's given.
type fakeProber struct {
	results map[string]probeResult

	mu     sync.Mutex
	tokens []string
}

func (p *fakeProber) Probe(_ context.Context, token string) (quota.Probe, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens = append(p.tokens, token)
	result := p.results[token]
	return quota.Probe{Usage: result.usage}, result.err
}

// probed returns the tokens the prober was given, sorted.
func (p *fakeProber) probed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Sorted(slices.Values(p.tokens))
}

// gatheringProber holds each probe until waitFor of them have started, which
// they only can when they run at the same time.
type gatheringProber struct {
	waitFor  int
	gathered chan struct{}

	mu      sync.Mutex
	started int
}

func (p *gatheringProber) Probe(ctx context.Context, _ string) (quota.Probe, error) {
	p.mu.Lock()
	if p.started++; p.started == p.waitFor {
		close(p.gathered)
	}
	p.mu.Unlock()
	select {
	case <-p.gathered:
		return quota.Probe{}, nil
	case <-ctx.Done():
		return quota.Probe{}, errors.New("probed alone: the probes ran one at a time")
	}
}

// accounts are three accounts, which the tests give tokens as they need.
var accounts = []config.Account{
	{ID: "work", Label: "Work"},
	{ID: "personal", Label: "Personal"},
	{ID: "side", Label: "Side"},
}
