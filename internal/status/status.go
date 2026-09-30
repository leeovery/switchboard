// Package status builds the status document: every account's usage, or why it
// couldn't be read. `switchboard status` prints it.
package status

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/prime"
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
	// model shares, the one whose quota most needs using, or, from the router,
	// of those the global pin names while one has room, passing over those
	// under pressure while another isn't. Empty when there's none.
	Best string `json:"best,omitempty"`
	// Primary is the id of the primary account, whose token Claude Code
	// holds: empty only when no account is marked the primary.
	Primary string `json:"primary,omitempty"`
	// Prime is the priming schedule: zero when priming is off.
	Prime Prime `json:"prime,omitzero"`
	// Pin is the router's global pin: zero when there's none, and in a
	// document that isn't the router's.
	Pin Pin `json:"pin,omitzero"`
	// Router is the router's health: zero in a document that isn't the
	// router's.
	Router Health `json:"router,omitzero"`
	// Restart is the restart the router has due: zero when none is, and in a
	// document that isn't the router's.
	Restart Restart `json:"restart,omitzero"`
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

// Restart is a restart the router has due, having found what it was started
// from changed, such as its config file. The service's router restarts
// itself once no request is in flight; one run by hand, only when it's run
// again.
type Restart struct {
	// Reason says why it's due, such as "config changed".
	Reason string `json:"reason"`
	// Since is when the router found it due.
	Since time.Time `json:"since"`
	// InFlight is how many requests the router had in flight as it gave the
	// document.
	InFlight int `json:"in_flight"`
	// ByHand is set when the router was run by hand, with serve.
	ByHand bool `json:"by_hand,omitempty"`
}

// Due reports whether a restart is due.
func (r Restart) Due() bool {
	return r.Reason != ""
}

// Pin sends every new session to the best of Accounts, and with Move, every
// session that was running on another account when it was set too, on its
// next request.
type Pin struct {
	// Accounts are the ids of the accounts pinned, in the order configured.
	Accounts []string
	Since    time.Time
	Move     bool
}

// pinJSON is a Pin as JSON gives it. Account is the first of its accounts,
// which a switchboard from before pins named several reads a pin by, and
// writes it with alone.
type pinJSON struct {
	Accounts []string  `json:"accounts"`
	Account  string    `json:"account"`
	Since    time.Time `json:"since"`
	Move     bool      `json:"move"`
}

// MarshalJSON gives the pin as JSON, its first account as its account too.
func (p Pin) MarshalJSON() ([]byte, error) {
	j := pinJSON{Accounts: p.Accounts, Since: p.Since, Move: p.Move}
	if len(p.Accounts) > 0 {
		j.Account = p.Accounts[0]
	}
	return json.Marshal(j)
}

// UnmarshalJSON reads a pin, one that names its account alone, as a
// switchboard from before pins named several wrote it, as a pin to that one.
func (p *Pin) UnmarshalJSON(data []byte) error {
	var j pinJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	*p = Pin{Accounts: j.Accounts, Since: j.Since, Move: j.Move}
	if len(p.Accounts) == 0 && j.Account != "" {
		p.Accounts = []string{j.Account}
	}
	return nil
}

// IsZero reports whether there's no pin: it names no account.
func (p Pin) IsZero() bool {
	return len(p.Accounts) == 0
}

// Has reports whether the pin names the account with the given id.
func (p Pin) Has(id string) bool {
	return slices.Contains(p.Accounts, id)
}

// Account is one account's status.
type Account struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Primary is set on the primary account.
	Primary bool `json:"primary,omitempty"`
	// Reserve is the share of every window the router leaves unused on the
	// account.
	Reserve float64 `json:"reserve,omitempty"`
	// TokenSet is whether the account's token file is there, and usable.
	TokenSet bool `json:"token_set"`
	// FetchedAt is when Usage was read; zero when it wasn't.
	FetchedAt time.Time `json:"fetched_at,omitzero"`
	quota.Usage
	// Lapsed are the keys of the windows that have lapsed, in Usage's order:
	// the window a request starts, its reset passed with nothing read since,
	// which reads empty, as it isn't running until a request starts it.
	Lapsed []string `json:"lapsed,omitempty"`
	// AtReserve are the keys of the windows that have reached the reserve,
	// short of their limits, in Usage's order: while there are any, the
	// router's own choices pass the account over for the requests those
	// windows count, and only a pin spends the reserve.
	AtReserve []string `json:"at_reserve,omitempty"`
	// Error says why Usage couldn't be read.
	Error string `json:"error,omitempty"`
	// Limit is the limit the router saw the account reach, while it holds:
	// zero when there's none, and in a document that isn't the router's.
	Limit Limit `json:"limit,omitzero"`
	// Refused is the upstream's refusal of requests on the account the router
	// saw, while it holds: zero when there's none, and in a document that
	// isn't the router's.
	Refused Refusal `json:"refused,omitzero"`
	// Pressure is how fast the router has seen the account's pressure window
	// used, and where that's heading: zero when it can't say, and in a
	// document that isn't the router's.
	Pressure Pressure `json:"pressure,omitzero"`
	// Rates are how fast the router has seen the account's windows used
	// lately, in Usage's order: those it has a recent rate of alone, and none
	// in a document that isn't the router's.
	Rates []Rate `json:"rates,omitempty"`
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

// Rate is how fast the router has seen a window used lately, as a share of it
// an hour, never negative: its rise over the last half hour, as
// score.RecentRate measures it.
type Rate struct {
	// Window is the window's key, such as "7d".
	Window string  `json:"window"`
	Rate   float64 `json:"rate"`
	// Since is when the rate is measured from.
	Since time.Time `json:"since,omitzero"`
}

// Pressure is how fast the router has seen an account's pressure window
// used, and where, at that rate, it's heading. While it runs out before it
// resets, the account is under pressure: a choice made afresh passes it over
// for an account that isn't.
type Pressure struct {
	// Window is the window's key, such as "5h".
	Window string `json:"window"`
	// Rate is the share of the window used an hour: its recent rate, as a
	// Rate is, while it has one, else its use since it started.
	Rate float64 `json:"rate"`
	// Recent is set when Rate is its recent rate, and Since is when that's
	// measured from.
	Recent bool      `json:"recent,omitempty"`
	Since  time.Time `json:"since,omitzero"`
	// RunsOut is when, at Rate, the window reaches where the account's room
	// ends: where its reserve starts, or its limit, without one or where the
	// global pin spends it. Zero when it never does, at no rate, and when it
	// has already.
	RunsOut time.Time `json:"runs_out,omitzero"`
	// Under is set when RunsOut comes before the window resets.
	Under bool `json:"under,omitempty"`
}

// Prober reads an account's usage with its token.
type Prober interface {
	Probe(ctx context.Context, token string) (quota.Probe, error)
}

// Collector builds a status document by probing every account.
type Collector struct {
	Prober Prober
	// Policy is the provider's say in which account is best, and which
	// window a prime starts.
	Policy score.Policy
	// Prime says when priming starts the accounts' windows, whose schedule
	// the document gives.
	Prime config.Prime
	// Token reads an account's token, by the account's id, from its file.
	Token func(id string) (tokens.Token, error)
	// Now reads the clock. Concurrent probes call it.
	Now func() time.Time
}

// Collect probes every account that has a usable token, all at once, and
// reports them in the order given, as they stand once read, along with the
// best of them, and the priming schedule over them. An account without one
// isn't probed: its status says why, and what would put it right.
func (c Collector) Collect(ctx context.Context, accounts []config.Account) Document {
	statuses := make([]Account, len(accounts))
	var usable []string
	var wg sync.WaitGroup
	for i, acct := range accounts {
		statuses[i] = Configured(acct)
		token, err := c.Token(acct.ID)
		if err != nil {
			statuses[i].Error = err.Error()
			logger.Debug("not probed: no usable token", "account", acct.ID, "error", err)
			continue
		}
		statuses[i].TokenSet = true
		usable = append(usable, acct.ID)
		wg.Go(func() { c.probe(ctx, &statuses[i], token) })
	}
	wg.Wait()
	now := c.Now()
	for i, a := range statuses {
		statuses[i] = a.AsOf(c.Policy, now)
	}
	doc := Document{
		GeneratedAt: now.UTC(),
		Source:      SourceProbe,
		Best:        Best(c.Policy, statuses, nil, now),
		Primary:     PrimaryOf(statuses),
		Accounts:    statuses,
	}
	if schedule, ok := prime.New(c.Prime.Day, usable, c.Policy); ok {
		doc.Prime = Priming(schedule)
	}
	return doc
}

// Configured is an account's status as the config gives it, before anything
// is read of it: its id and label, whether it's the primary, and its reserve.
func Configured(a config.Account) Account {
	return Account{ID: a.ID, Label: a.Label, Primary: a.Primary, Reserve: a.Reserve}
}

// AsOf is the account, its windows as read, as it stands at now: a window
// that has lapsed, as policy judges, reads empty, and the account notes the
// windows that have lapsed, and those that have reached its reserve.
func (a Account) AsOf(policy score.Policy, now time.Time) Account {
	a.Lapsed = policy.Lapsed(a.Windows, now)
	a.Windows = policy.AsOf(a.Windows, now)
	a.AtReserve = score.AtReserve(a.Windows, a.Reserve, now)
	return a
}

// Heading is where a window is heading, and whether it's at the rate the
// router saw it used over the last half hour.
type Heading struct {
	score.Projection
	// Recent is set when it heads there at that recent rate, rather than at
	// the pace its use since it started sets, and Since is when that rate is
	// measured from.
	Recent bool
	Since  time.Time
}

// Project says where the account's window w is heading at now: at the pace
// its use since it started sets, or at the rate the router saw it used over
// the last half hour, where it has that rate, when that has it run out
// sooner, as score.Sooner judges, so a burst of use shows before the average
// catches up with it. The window the router watches for pressure always goes
// at that recent rate, so it heads where the router judges it to.
func (a Account) Project(w quota.Window, now time.Time) Heading {
	average := Heading{Projection: score.Project(w, now)}
	rate, ok := a.rate(w.Key)
	if !ok {
		return average
	}
	recent := Heading{Projection: score.ProjectAt(w, rate.Rate, now), Recent: true, Since: rate.Since}
	if w.Key == a.Pressure.Window || score.Sooner(average.Projection, recent.Projection) {
		return recent
	}
	return average
}

// rate returns how fast the router has seen the account's window with the
// given key used lately, reporting false when it has no recent rate of it.
func (a Account) rate(key string) (Rate, bool) {
	i := slices.IndexFunc(a.Rates, func(r Rate) bool { return r.Window == key })
	if i < 0 {
		return Rate{}, false
	}
	return a.Rates[i], true
}

// HasLapsed reports whether the account's window w has lapsed, and reads
// empty until a request starts it.
func (a Account) HasLapsed(w quota.Window) bool {
	return slices.Contains(a.Lapsed, w.Key)
}

// PrimaryOf returns the id of the primary among accounts, or "" when none is
// marked the primary.
func PrimaryOf(accounts []Account) string {
	i := slices.IndexFunc(accounts, func(a Account) bool { return a.Primary })
	if i < 0 {
		return ""
	}
	return accounts[i].ID
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

// Best is the account of those given that a new session goes to, as the
// router chooses one for a request of any model: of those pinned, while one
// can take the request, as bestPinned says; else of them all, the one policy
// picks, leaving each one's reserve unused, and passing over those under
// pressure, as pick says. It's empty when none can take one.
func Best(policy score.Policy, accounts []Account, pinned []string, now time.Time) string {
	if id, ok := bestPinned(policy, accounts, pinned, now); ok {
		return id
	}
	return pick(policy, accounts, now)
}

// bestPinned is the account of those given, pinned and with a usable token,
// that a new session goes to, as the router chooses one: the one policy
// picks, spending each one's reserve, as a pin spends it, else the first
// with room, as a pin sends requests to an account whose quota can't be
// scored, or that nothing has been read of. It reports false when none of
// them has room.
func bestPinned(policy score.Policy, accounts []Account, pinned []string, now time.Time) (string, bool) {
	var spending []Account
	for _, a := range accounts {
		if a.TokenSet && slices.Contains(pinned, a.ID) {
			a.Reserve = 0
			spending = append(spending, a)
		}
	}
	if id := pick(policy, spending, now); id != "" {
		return id, true
	}
	i := slices.IndexFunc(spending, func(a Account) bool {
		return len(a.Windows) == 0 || score.Available(a.Windows, 0, policy.IsShared, now)
	})
	if i < 0 {
		return "", false
	}
	return spending[i].ID, true
}

// pick is the account of those given that policy picks for a request of any
// model, leaving each one's reserve unused, and passing over those under
// pressure at the rates the router saw, or empty when none can take one.
func pick(policy score.Policy, accounts []Account, now time.Time) string {
	candidates := make([]score.Candidate, len(accounts))
	for i, account := range accounts {
		candidates[i] = score.Candidate{ID: account.ID, Windows: account.Windows, Reserve: account.Reserve, Rate: account.Pressure.Rate}
	}
	c, _ := policy.Pick(candidates, policy.IsShared, "", now)
	return c.ID
}
