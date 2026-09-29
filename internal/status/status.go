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
	"github.com/leeovery/switchboard/internal/tokens"
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

// How the router was found when it was asked for its document and couldn't
// give it, as a Fallback says.
const (
	// RouterNotRunning is nothing answering on the router's socket.
	RouterNotRunning = "not running"
	// RouterUnhealthy is something answering there, but not as a router does.
	RouterUnhealthy = "unhealthy"
)

// Document is the status of every configured account, as `status --json`
// prints it. Fields may be added to it, never renamed.
type Document struct {
	GeneratedAt time.Time `json:"generated_at"`
	// Source says where the usage came from, such as SourceProbe.
	Source string `json:"source"`
	// Fallback says why a document built by probing isn't the router's, when
	// the router was asked for its own first: zero in the router's own, and
	// in one probed as asked.
	Fallback Fallback `json:"fallback,omitzero"`
	// Best is the account to use next: of those with room in the windows every
	// model shares, the one whose quota most needs using. Empty when there's none.
	Best string `json:"best,omitempty"`
	// Pin is the router's global pin: zero when there's none, and in a
	// document that isn't the router's.
	Pin Pin `json:"pin,omitzero"`
	// Router is the router's health: zero in a document that isn't the
	// router's.
	Router Health `json:"router,omitzero"`
	// Sessions is how many sessions the router has sent anywhere in the last
	// hour, each counted once however many accounts its models went to: zero
	// in a document that isn't the router's.
	Sessions int       `json:"sessions,omitzero"`
	Accounts []Account `json:"accounts"`
}

// Fallback is why a document was built by probing though the router was
// asked for its own first.
type Fallback struct {
	// Router is how the router was found: RouterNotRunning or RouterUnhealthy.
	Router string `json:"router"`
	// Reason says what was wrong with its answer, when it answered.
	Reason string `json:"reason,omitempty"`
}

// Health is how the router has fared with the requests it routed over the
// last five minutes: how many it answered, and how many of those it failed
// itself, rather than passing on the upstream's answer.
type Health struct {
	Healthy  bool `json:"healthy"`
	Requests int  `json:"requests"`
	Failures int  `json:"failures"`
	// Reason says why the router is unhealthy; empty while it's healthy.
	Reason string `json:"reason,omitempty"`
}

// Pin sends every new session to Account, and with Move, every session that
// was running when it was set too, on its next request.
type Pin struct {
	Account string    `json:"account"`
	Since   time.Time `json:"since"`
	Move    bool      `json:"move"`
}

// Account is one account's status.
type Account struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// TokenSet is whether the account's token file is there, and usable.
	TokenSet bool `json:"token_set"`
	// FetchedAt is when Usage was read; zero when it wasn't.
	FetchedAt time.Time `json:"fetched_at,omitzero"`
	quota.Usage
	// Error says why Usage couldn't be read.
	Error string `json:"error,omitempty"`
	// Limit is the limit the router saw the account reach, while it holds:
	// zero when there's none, and in a document that isn't the router's.
	Limit Limit `json:"limit,omitzero"`
	// Refused is the upstream's refusal of requests on the account the router
	// saw, while it holds: zero when there's none, and in a document that
	// isn't the router's.
	Refused Refusal `json:"refused,omitzero"`
	// Sessions is how many sessions the router has sent to the account in the
	// last hour: zero in a document that isn't the router's.
	Sessions int `json:"sessions,omitzero"`
}

// Limit is a limit an account reached, as the upstream answered a request on
// it: the windows it named as reached, if any, and when the account is to
// have room again. Until then it has none for the requests those windows
// count, or for any request when they're none, whatever its windows read.
type Limit struct {
	Windows []string  `json:"windows,omitempty"`
	Until   time.Time `json:"until"`
}

// Holds reports whether the limit still holds at now.
func (l Limit) Holds(now time.Time) bool {
	return l.Until.After(now)
}

// Refusal is the upstream refusing requests on an account, answering with
// Status, as the router saw it: until Until, the account has no room for them,
// whatever its windows read. They're every request, or, when Family is set,
// the requests of that model family alone.
type Refusal struct {
	Until  time.Time `json:"until"`
	Status int       `json:"status"`
	Family string    `json:"family,omitempty"`
}

// Holds reports whether the refusal still holds at now.
func (r Refusal) Holds(now time.Time) bool {
	return r.Until.After(now)
}

// Prober reads an account's usage with its token.
type Prober interface {
	Probe(ctx context.Context, token string) (quota.Probe, error)
}

// Collector builds a status document by probing every account.
type Collector struct {
	Prober Prober
	// Policy is the provider's say in which account is best.
	Policy score.Policy
	// Token reads an account's token, by the account's id, from its file.
	Token func(id string) (tokens.Token, error)
	// Now reads the clock. Concurrent probes call it.
	Now func() time.Time
}

// Collect probes every account that has a usable token, all at once, and
// reports them in the order given, along with the best of them. An account
// without one isn't probed: its status says why, and what would put it right.
func (c Collector) Collect(ctx context.Context, accounts []config.Account) Document {
	statuses := make([]Account, len(accounts))
	var wg sync.WaitGroup
	for i, acct := range accounts {
		statuses[i] = Account{ID: acct.ID, Label: acct.Label}
		token, err := c.Token(acct.ID)
		if err != nil {
			statuses[i].Error = err.Error()
			logger.Debug("not probed: no usable token", "account", acct.ID, "error", err)
			continue
		}
		statuses[i].TokenSet = true
		wg.Go(func() { c.probe(ctx, &statuses[i], token) })
	}
	wg.Wait()
	now := c.Now()
	return Document{GeneratedAt: now.UTC(), Source: SourceProbe, Best: Best(c.Policy, statuses, now), Accounts: statuses}
}

// probe fills in an account's usage, or why it couldn't be read, and logs
// how the probe went.
func (c Collector) probe(ctx context.Context, account *Account, token tokens.Token) {
	started := time.Now()
	probed, err := c.Prober.Probe(ctx, token.Reveal())
	took := time.Since(started).Round(time.Millisecond)
	if err != nil {
		account.Error = err.Error()
		logger.Warn("probe failed", "account", account.ID, "duration", took, "error", err)
		return
	}
	account.Usage, account.FetchedAt = probed.Usage, c.Now().UTC()
	logger.Debug("probed account", "account", account.ID, "duration", took, "windows", len(probed.Windows))
	for _, f := range probed.Failures {
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
