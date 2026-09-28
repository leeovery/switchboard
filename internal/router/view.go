package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

// view is what a choice of account for a request knows of the accounts it
// can go out on.
type view struct {
	policy score.Policy
	now    time.Time
	// candidates are the accounts with a token, in the order configured, with
	// their windows as last read.
	candidates []score.Candidate
	// applies reports whether the window named key counts the request.
	applies func(key string) bool
}

// has reports whether the request can go out on the account with the given id.
func (v view) has(id string) bool {
	return slices.ContainsFunc(v.candidates, func(c score.Candidate) bool { return c.ID == id })
}

// room reports whether the account with the given id can take the request:
// it's one the request can go out on, and no window that counts the request is
// spent. An account nothing has been read of has room as far as anyone knows,
// so a session on it stays and a pin to it holds.
func (v view) room(id string) bool {
	i := slices.IndexFunc(v.candidates, func(c score.Candidate) bool { return c.ID == id })
	if i < 0 {
		return false
	}
	windows := v.candidates[i].Windows
	return len(windows) == 0 || score.Available(windows, v.applies, v.now)
}

// pick returns the account whose quota most needs using, of those known to
// have room for the request, keeping to preferred unless another is well
// ahead of it. It reports false when none has room.
func (v view) pick(preferred string) (string, bool) {
	return v.policy.Pick(v.candidates, v.applies, preferred, v.now)
}
