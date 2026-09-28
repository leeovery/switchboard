// Package status builds the status document: every account's usage, or why it
// couldn't be read. `switchboard status` prints it.
package status

import (
	"context"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// The sources a document can come from.
const (
	// SourceProbe marks a document built by probing every account.
	SourceProbe = "probe"
	// SourceRouter marks a document from the router: usage read off the
	// responses it forwards, and off probes where there were none.
	SourceRouter = "router"
)

var logger = logs.For("status")

// Document is the status of every configured account, as `status --json`
// prints it. Fields may be added to it, never renamed.
type Document struct {
	GeneratedAt time.Time `json:"generated_at"`
	// Source says where the usage came from, such as SourceProbe.
	Source string `json:"source"`
	// Best is the account to use next: of those with room in the windows every
	// model shares, the one whose quota most needs using. Empty when there's none.
	Best     string    `json:"best,omitempty"`
	Accounts []Account `json:"accounts"`
}

// Account is one account's status.
type Account struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	TokenSet bool   `json:"token_set"`
	// FetchedAt is when Usage was read; zero when it wasn't.
	FetchedAt time.Time `json:"fetched_at,omitzero"`
	quota.Usage
	// Error says why Usage couldn't be read.
	Error string `json:"error,omitempty"`
}

// Prober reads an account's usage with its token.
type Prober interface {
	Probe(ctx context.Context, token string) (quota.Usage, error)
}

// Collector builds a status document by probing every account.
type Collector struct {
	Prober Prober
	// Policy is the provider's say in which account is best.
	Policy score.Policy
	// Getenv reads the variables holding the accounts' tokens.
	Getenv func(key string) string
	// Now reads the clock. Concurrent probes call it.
	Now func() time.Time
}

// Collect probes every account that has a token, all at once, and reports
// them in the order given, along with the best of them. An account without a
// token isn't probed: its status says which variable to set.
func (c Collector) Collect(ctx context.Context, accounts []config.Account) Document {
	statuses := make([]Account, len(accounts))
	var wg sync.WaitGroup
	for i, acct := range accounts {
		statuses[i] = Account{ID: acct.ID, Label: acct.Label}
		token, ok := acct.Token(c.Getenv)
		if !ok {
			statuses[i].Error = TokenMissing(acct)
			logger.Debug("not probed: token missing", "account", acct.ID)
			continue
		}
		statuses[i].TokenSet = true
		wg.Go(func() { c.probe(ctx, &statuses[i], token) })
	}
	wg.Wait()
	now := c.Now()
	return Document{GeneratedAt: now.UTC(), Source: SourceProbe, Best: Best(c.Policy, statuses, now), Accounts: statuses}
}

// TokenMissing is the error of an account whose token isn't set: it says
// which variable to set.
func TokenMissing(acct config.Account) string {
	return "token missing: set " + acct.TokenEnv
}

// probe fills in an account's usage, or why it couldn't be read, and logs
// how the probe went.
func (c Collector) probe(ctx context.Context, account *Account, token config.Token) {
	started := time.Now()
	usage, err := c.Prober.Probe(ctx, token.Reveal())
	took := time.Since(started).Round(time.Millisecond)
	if err != nil {
		account.Error = err.Error()
		logger.Warn("probe failed", "account", account.ID, "duration", took, "error", err)
		return
	}
	account.Usage, account.FetchedAt = usage, c.Now().UTC()
	logger.Debug("probed account", "account", account.ID, "duration", took, "windows", len(usage.Windows))
	for _, f := range usage.Failures {
		logger.Warn("window unread", "account", account.ID, "window", f.Window, "error", f.Error)
	}
}

// Best is the account of those given that policy picks for a request of any
// model, or empty when none can take one.
func Best(policy score.Policy, accounts []Account, now time.Time) string {
	candidates := make([]score.Candidate, len(accounts))
	for i, account := range accounts {
		candidates[i] = score.Candidate{ID: account.ID, Windows: account.Windows}
	}
	id, _ := policy.Pick(candidates, policy.IsShared, "", now)
	return id
}
