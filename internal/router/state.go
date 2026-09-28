package router

import (
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// origin is what a reading of an account's usage came from.
type origin string

const (
	fromResponse origin = "response"
	fromProbe    origin = "probe"
)

// reading is a window as it was last read, and when.
type reading struct {
	quota.Window
	at time.Time
}

// usage is what the router knows of one account's usage.
type usage struct {
	// windows holds the latest reading of each window, by key.
	windows map[string]reading
	// updated is when a reading last came in, and from is what it came from.
	updated time.Time
	from    origin
	// probeErr says why the last probe read nothing, until a reading comes in.
	probeErr string
	// failures are the windows the last probe expected and couldn't read,
	// each until it's read.
	failures []quota.Failure
}

// state is what the router knows of every account's usage, learnt from the
// responses it forwards and from probes. It's safe for concurrent use.
type state struct {
	accounts accounts
	policy   score.Policy
	now      func() time.Time

	mu    sync.Mutex
	usage map[string]*usage
}

func newState(accounts accounts, policy score.Policy, now func() time.Time) *state {
	s := &state{accounts: accounts, policy: policy, now: now, usage: make(map[string]*usage, len(accounts))}
	for _, a := range accounts {
		s.usage[a.ID] = &usage{windows: make(map[string]reading)}
	}
	return s
}

// record takes in a reading of an account's windows. Each window replaces the
// reading of its key before it, and the account's other windows stand.
func (s *state) record(id string, windows []quota.Window, from origin) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].take(windows, from, at)
}

// recordProbe takes in what probing an account found: its usage, or why it
// read none.
func (s *state) recordProbe(id string, probed quota.Usage, err error) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	if err != nil {
		u.probeErr = err.Error()
		return
	}
	u.failures = slices.Clone(probed.Failures)
	u.take(probed.Windows, fromProbe, at)
}

// take takes in windows read at a time.
func (u *usage) take(windows []quota.Window, from origin, at time.Time) {
	for _, w := range windows {
		u.windows[w.Key] = reading{Window: w, at: at}
		u.failures = slices.DeleteFunc(u.failures, func(f quota.Failure) bool { return f.Window == w.Key })
	}
	u.updated, u.from, u.probeErr = at, from, ""
}

// document reports every account's usage as the router knows it, in the
// order configured, with the best account to use next.
func (s *state) document() status.Document {
	now := s.now()
	accounts := s.statuses()
	return status.Document{
		GeneratedAt: now.UTC(),
		Source:      status.SourceRouter,
		Best:        status.Best(s.policy, accounts, now),
		Accounts:    accounts,
	}
}

func (s *state) statuses() []status.Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	statuses := make([]status.Account, len(s.accounts))
	for i, a := range s.accounts {
		statuses[i] = s.usage[a.ID].status(a)
	}
	return statuses
}

// status is the account's usage as last read, or why there's none.
func (u *usage) status(a account) status.Account {
	st := status.Account{ID: a.ID, Label: a.Label, TokenSet: a.hasToken}
	if !a.hasToken {
		st.Error = status.TokenMissing(a.Account)
		return st
	}
	st.FetchedAt = u.updated
	st.Windows = u.latest()
	st.Failures = slices.Clone(u.failures)
	st.Error = u.probeErr
	return st
}

// latest returns the latest reading of each window, in quota.Sort's order, or
// nil when there's none.
func (u *usage) latest() []quota.Window {
	if len(u.windows) == 0 {
		return nil
	}
	windows := make([]quota.Window, 0, len(u.windows))
	for _, r := range u.windows {
		windows = append(windows, r.Window)
	}
	quota.Sort(windows)
	return windows
}
