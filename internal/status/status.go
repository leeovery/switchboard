// Package status builds the status document: every account's usage, or why it
// couldn't be read. `switchboard status` prints it.
package status

import (
	"context"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
)

// SourceProbe marks a document built by probing every account.
const SourceProbe = "probe"

// Document is the status of every configured account, as `status --json`
// prints it. Fields may be added to it, never renamed.
type Document struct {
	GeneratedAt time.Time `json:"generated_at"`
	// Source says where the usage came from, such as SourceProbe.
	Source   string    `json:"source"`
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
	// Getenv reads the variables holding the accounts' tokens.
	Getenv func(key string) string
	// Now reads the clock. Concurrent probes call it.
	Now func() time.Time
}

// Collect probes every account that has a token, all at once, and reports
// them in the order given. An account without a token isn't probed: its status
// says which variable to set.
func (c Collector) Collect(ctx context.Context, accounts []config.Account) Document {
	statuses := make([]Account, len(accounts))
	var wg sync.WaitGroup
	for i, acct := range accounts {
		statuses[i] = Account{ID: acct.ID, Label: acct.Label}
		token, ok := acct.Token(c.Getenv)
		if !ok {
			statuses[i].Error = "token missing: set " + acct.TokenEnv
			continue
		}
		statuses[i].TokenSet = true
		wg.Go(func() { c.probe(ctx, &statuses[i], token) })
	}
	wg.Wait()
	return Document{GeneratedAt: c.Now().UTC(), Source: SourceProbe, Accounts: statuses}
}

// probe fills in an account's usage, or why it couldn't be read.
func (c Collector) probe(ctx context.Context, account *Account, token config.Token) {
	usage, err := c.Prober.Probe(ctx, token.Reveal())
	if err != nil {
		account.Error = err.Error()
		return
	}
	account.Usage, account.FetchedAt = usage, c.Now().UTC()
}
