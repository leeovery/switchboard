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
	// barred are the accounts that have no room whatever their windows say,
	// by id: they refused the request lately, a limit they reached holds the
	// request back, or the request has been tried on them.
	barred []string
	// refused are the accounts, of those barred, that refused the request
	// lately, by id: they'd only refuse it again.
	refused []string
}

// without returns the view with the accounts given barred as well.
func (v view) without(ids []string) view {
	v.barred = slices.Concat(v.barred, ids)
	return v
}

// has reports whether the request can go out on the account with the given id.
func (v view) has(id string) bool {
	return slices.ContainsFunc(v.candidates, func(c score.Candidate) bool { return c.ID == id })
}

// unrefused returns the first of the accounts with the ids given that the
// request can go out on and that hasn't refused it lately, else the first
// such of the rest, in the order configured. It reports false when every
// account has refused it.
func (v view) unrefused(first ...string) (string, bool) {
	for _, id := range slices.Concat(first, v.ids()) {
		if v.has(id) && !slices.Contains(v.refused, id) {
			return id, true
		}
	}
	return "", false
}

// ids returns the ids of the accounts the request can go out on, in the
// order configured.
func (v view) ids() []string {
	ids := make([]string, len(v.candidates))
	for i, c := range v.candidates {
		ids[i] = c.ID
	}
	return ids
}

// room reports whether the account with the given id can take the request:
// it's one the request can go out on, it isn't barred, and no window that
// counts the request is spent. An account nothing has been read of has room
// as far as anyone knows, so a session on it stays and a pin to it holds.
func (v view) room(id string) bool {
	i := slices.IndexFunc(v.candidates, func(c score.Candidate) bool { return c.ID == id })
	if i < 0 || slices.Contains(v.barred, id) {
		return false
	}
	windows := v.candidates[i].Windows
	return len(windows) == 0 || score.Available(windows, v.applies, v.now)
}

// pick returns the account whose quota most needs using, of those known to
// have room for the request, keeping to preferred unless another is well
// ahead of it. It reports false when none has room.
func (v view) pick(preferred string) (string, bool) {
	return v.policy.Pick(v.open(), v.applies, preferred, v.now)
}

// full returns the ids of the accounts whose windows as last read leave no
// room for the request, and that aren't barred: a probe could find room on
// them after all, were a window to have reset unseen.
func (v view) full() []string {
	var ids []string
	for _, c := range v.open() {
		if len(c.Windows) > 0 && !score.Available(c.Windows, v.applies, v.now) {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// open returns the candidates that aren't barred.
func (v view) open() []score.Candidate {
	return slices.DeleteFunc(slices.Clone(v.candidates), func(c score.Candidate) bool { return slices.Contains(v.barred, c.ID) })
}
