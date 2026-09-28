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
)

// policy scores the windows as Claude's are: the session and the week apply
// to every model, and the week is perishable.
var policy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d"}

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
		Getenv: envFrom(map[string]string{
			"CLAUDE_TOKEN_WORK": "test-token-work",
			"CLAUDE_TOKEN_SIDE": " test-token-side\n",
		}),
		Now: func() time.Time { return now },
	}
	accounts := []config.Account{
		{ID: "work", Label: "Work", TokenEnv: "CLAUDE_TOKEN_WORK"},
		{ID: "personal", Label: "Personal", TokenEnv: "CLAUDE_TOKEN_PERSONAL"},
		{ID: "side", Label: "Side", TokenEnv: "CLAUDE_TOKEN_SIDE"},
	}

	got := collector.Collect(t.Context(), accounts)
	want := status.Document{
		GeneratedAt: now.UTC(),
		Source:      "probe",
		Best:        "work",
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(), Usage: workUsage},
			{ID: "personal", Label: "Personal", TokenSet: false, Error: "token missing: set CLAUDE_TOKEN_PERSONAL"},
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
		Getenv: envFrom(map[string]string{"CLAUDE_TOKEN_WORK": "test-token-work", "CLAUDE_TOKEN_SIDE": "test-token-side"}),
		Now:    func() time.Time { return now },
	}
	accounts := []config.Account{
		{ID: "work", Label: "Work", TokenEnv: "CLAUDE_TOKEN_WORK"},
		{ID: "personal", Label: "Personal", TokenEnv: "CLAUDE_TOKEN_PERSONAL"},
		{ID: "side", Label: "Side", TokenEnv: "CLAUDE_TOKEN_SIDE"},
	}

	collector.Collect(t.Context(), accounts)
	for _, want := range [][]string{
		{"level=DEBUG", `msg="probed account" component=status`, "account=work", "duration=", "windows=2"},
		{"level=WARN", `msg="window unread" component=status`, "account=work", "window=7d_oi", `error="HTTP 529 · Overloaded"`},
		{"level=DEBUG", `msg="not probed: token missing" component=status`, "account=personal"},
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
	accounts := []config.Account{
		{ID: "work", Label: "Work", TokenEnv: "CLAUDE_TOKEN_WORK"},
		{ID: "personal", Label: "Personal", TokenEnv: "CLAUDE_TOKEN_PERSONAL"},
		{ID: "side", Label: "Side", TokenEnv: "CLAUDE_TOKEN_SIDE"},
	}
	collector := status.Collector{
		Prober: &gatheringProber{waitFor: len(accounts), gathered: make(chan struct{})},
		Getenv: envFrom(map[string]string{
			"CLAUDE_TOKEN_WORK":     "test-token-work",
			"CLAUDE_TOKEN_PERSONAL": "test-token-personal",
			"CLAUDE_TOKEN_SIDE":     "test-token-side",
		}),
		Now: time.Now,
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
		want       string
	}{
		{
			name: "the account whose week resets soonest",
			work: withWindows(session, week(0.5, 5*24*time.Hour)),
			side: withWindows(session, week(0.5, 24*time.Hour)),
			want: "side",
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
				Getenv: envFrom(map[string]string{"CLAUDE_TOKEN_WORK": "test-token-work", "CLAUDE_TOKEN_SIDE": "test-token-side"}),
				Now:    func() time.Time { return now },
			}
			accounts := []config.Account{
				{ID: "work", Label: "Work", TokenEnv: "CLAUDE_TOKEN_WORK"},
				{ID: "side", Label: "Side", TokenEnv: "CLAUDE_TOKEN_SIDE"},
			}

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
					{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"},
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
      "error": "token missing: set CLAUDE_TOKEN_PERSONAL"
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
				Accounts:    []status.Account{{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "accounts": [
    {
      "id": "personal",
      "label": "Personal",
      "token_set": false,
      "error": "token missing: set CLAUDE_TOKEN_PERSONAL"
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

func (p *fakeProber) Probe(_ context.Context, token string) (quota.Usage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens = append(p.tokens, token)
	result := p.results[token]
	return result.usage, result.err
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

func (p *gatheringProber) Probe(ctx context.Context, _ string) (quota.Usage, error) {
	p.mu.Lock()
	if p.started++; p.started == p.waitFor {
		close(p.gathered)
	}
	p.mu.Unlock()
	select {
	case <-p.gathered:
		return quota.Usage{}, nil
	case <-ctx.Done():
		return quota.Usage{}, errors.New("probed alone: the probes ran one at a time")
	}
}

// envFrom returns a getenv backed by vars, so tests never read the real environment.
func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}
