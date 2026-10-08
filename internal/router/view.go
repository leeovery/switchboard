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
	// their windows as last read and their reserves.
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
	// limited holds, by id, the identity of the limit that holds the request
	// back on each account, of those barred, where a limit does.
	limited map[string]int
	// spending are the accounts, by id, whose reserve the request may spend,
	// as a pin names them: a pin is the user's, and runs its account to its
	// limit, where the reserve holds back the router's own choices.
	spending []string
}

// without returns the view with the accounts given barred as well.
func (v view) without(ids []string) view {
	v.barred = slices.Concat(v.barred, ids)
	return v
}

// within returns the view with the accounts given alone for the request to go
// out on.
func (v view) within(ids []string) view {
	v.candidates = slices.DeleteFunc(slices.Clone(v.candidates), func(c score.Candidate) bool { return !slices.Contains(ids, c.ID) })
	return v
}

// spend returns the view with the request able to spend the reserves of the
// accounts given, as pins name them.
func (v view) spend(ids ...string) view {
	v.spending = slices.Concat(v.spending, ids)
	return v
}

// candidate returns the account with the given id as the request finds it.
// It reports false when the request can't go out on the account.
func (v view) candidate(id string) (score.Candidate, bool) {
	i := slices.IndexFunc(v.candidates, func(c score.Candidate) bool { return c.ID == id })
	if i < 0 {
		return score.Candidate{}, false
	}
	return v.found(v.candidates[i]), true
}

// found is c as the request finds it: without its reserve, when the request
// may spend it.
func (v view) found(c score.Candidate) score.Candidate {
	if slices.Contains(v.spending, c.ID) {
		c.Reserve = 0
	}
	return c
}

// has reports whether the request can go out on the account with the given id.
func (v view) has(id string) bool {
	_, ok := v.candidate(id)
	return ok
}

// fallback returns the account a request goes to when none has room, for the
// upstream to refuse it there, saying why: the first of those with the ids
// given, else of the rest, in the order configured, that the request can go
// out on, that hasn't refused it lately, which would only refuse it again,
// and that isn't held back by its reserve alone, which would take it and
// spend the reserve. It reports false when every account is passed over.
func (v view) fallback(first ...string) (string, bool) {
	for _, id := range slices.Concat(first, v.ids()) {
		if v.has(id) && !slices.Contains(v.refused, id) && !v.reserved(id) {
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
// counts the request has reached the account's reserve, or its limit when the
// request may spend the reserve. An account nothing has been read of has room
// as far as anyone knows, so a session on it stays and a pin to it holds.
func (v view) room(id string) bool {
	c, ok := v.candidate(id)
	if !ok || slices.Contains(v.barred, id) {
		return false
	}
	return len(c.Windows) == 0 || score.Available(c.Windows, c.Reserve, v.applies, v.now)
}

// reserved reports whether the account with the given id is held back from
// the request by its reserve alone: it would take the request up to its
// limit, but its windows have reached its reserve, which the request may not
// spend.
func (v view) reserved(id string) bool {
	c, ok := v.candidate(id)
	return ok && !slices.Contains(v.barred, id) && len(c.Windows) > 0 &&
		score.Available(c.Windows, 0, v.applies, v.now) && !score.Available(c.Windows, c.Reserve, v.applies, v.now)
}

// holding returns what holds the request back on the account with the given
// id, and the identity of the limit, where that's what does: a limit it
// reached, else its refusing the request lately, else its reserve alone. It
// returns zero where the account can take the request, and where only its
// windows leave it no room.
func (v view) holding(id string) (Hold, int) {
	switch limit, limited := v.limited[id]; {
	case limited:
		return HeldByLimit, limit
	case slices.Contains(v.refused, id):
		return HeldByRefusal, 0
	case v.reserved(id):
		return HeldByReserve, 0
	}
	return 0, 0
}

// letGo reports whether any account is held back from the request by its
// reserve alone, and returns when the first of them is let go, as far as
// that's known, or zero when it isn't known of any.
func (v view) letGo() (time.Time, bool) {
	var soonest time.Time
	held := false
	for _, c := range v.open() {
		if !v.reserved(c.ID) {
			continue
		}
		held = true
		if at, ok := score.LetGo(c.Windows, c.Reserve, v.applies, v.now); ok && (soonest.IsZero() || at.Before(soonest)) {
			soonest = at
		}
	}
	return soonest, held
}

// pick returns the account whose quota most needs using, of those known to
// have room for the request, passing over those under pressure while another
// isn't, and keeping to preferred unless another is well ahead of it. It
// reports false when none has room.
func (v view) pick(preferred string) (score.Choice, bool) {
	return v.policy.Pick(v.open(), v.applies, preferred, v.now)
}

// full returns the ids of the accounts whose windows as last read leave no
// room for the request, and that aren't barred: a probe could find room on
// them after all, were a window to have reset unseen.
func (v view) full() []string {
	var ids []string
	for _, c := range v.open() {
		if len(c.Windows) > 0 && !score.Available(c.Windows, c.Reserve, v.applies, v.now) {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// open returns the candidates that aren't barred, as the request finds them.
func (v view) open() []score.Candidate {
	var open []score.Candidate
	for _, c := range v.candidates {
		if !slices.Contains(v.barred, c.ID) {
			open = append(open, v.found(c))
		}
	}
	return open
}
