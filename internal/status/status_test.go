package status_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestCollect(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	workUsage := quota.Usage{
		Windows:  []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)}},
		Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
	}
	prober := &fakeProber{results: map[string]probeResult{
		"test-token-work": {usage: workUsage},
		"test-token-side": {err: errors.New("HTTP 401 · Invalid bearer token")},
	}}
	collector := status.Collector{
		Prober: prober,
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

func TestDocumentJSON(t *testing.T) {
	generated := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	doc := status.Document{
		GeneratedAt: generated,
		Source:      status.SourceProbe,
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
	}
	want := `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
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
}`

	got, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if string(got) != want {
		t.Errorf("MarshalIndent() =\n%s\nwant\n%s", got, want)
	}
}

type probeResult struct {
	usage quota.Usage
	err   error
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
