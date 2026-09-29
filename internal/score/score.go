// Package score judges accounts by their usage windows: how far through each
// window they are and where its use is heading, whether an account can take a
// request, within its limits or within its reserve, how urgently its quota
// needs using, and which account to use next. Every function is pure and is
// handed the clock. None knows a provider's windows by name: a Policy names
// the ones that matter.
package score

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// minUntilReset floors the time left before a reset, so a reset minutes
	// away doesn't dominate the score.
	minUntilReset = time.Hour
	// hysteresis is the margin by which another account's score must beat the
	// preferred account's before Pick moves off it, so near-equal accounts
	// don't trade places.
	hysteresis = 0.2
	// tolerance is how close two scores, or two shares of a window, must be
	// to count as equal: far below any real difference, far above rounding.
	tolerance = 1e-9
)

// Policy is a provider's say in scoring, naming its windows by key.
type Policy struct {
	// Shared are the windows that apply to every model.
	Shared []string
	// Perishable is the window perishability is measured on.
	Perishable string
}

// IsShared reports whether the window named key applies to every model.
func (p Policy) IsShared(key string) bool {
	return slices.Contains(p.Shared, key)
}

// Candidate is an account Pick can choose.
type Candidate struct {
	ID      string
	Windows []quota.Window
	// Reserve is the share of every window Pick leaves unused on the
	// account: a window's room ends at 1 − Reserve.
	Reserve float64
}

// Available reports whether an account with these windows can take a
// request, leaving reserve of every window unused: no window that applies, as
// applies judges by key, has reached 1 − reserve, is used up or is refused. A
// window that has reset since it was read counts as fresh. An account without
// windows isn't available: nothing is known about it.
func Available(windows []quota.Window, reserve float64, applies func(key string) bool, now time.Time) bool {
	return len(windows) > 0 && !slices.ContainsFunc(windows, func(w quota.Window) bool {
		return applies(w.Key) && (spent(w) || atReserve(w, reserve)) && !hasReset(w, now)
	})
}

// AtReserve returns the keys of the windows, in the order given, that the
// reserve alone holds back at now: 1 − reserve of each is used, but it isn't
// spent, and it hasn't reset since. There are none without a reserve.
func AtReserve(windows []quota.Window, reserve float64, now time.Time) []string {
	var keys []string
	for _, w := range windows {
		if atReserve(w, reserve) && !hasReset(w, now) {
			keys = append(keys, w.Key)
		}
	}
	return keys
}

// LetGo returns when the reserve lets go of an account it holds back, as
// applies judges which windows count: once each window that applies and has
// reached the reserve has reset, at the latest of their resets. It reports
// false when the reserve holds nothing back, or when one of those windows'
// reset isn't known.
func LetGo(windows []quota.Window, reserve float64, applies func(key string) bool, now time.Time) (time.Time, bool) {
	var last time.Time
	for _, w := range windows {
		switch {
		case !applies(w.Key) || !atReserve(w, reserve) || hasReset(w, now):
		case w.ResetsAt.IsZero():
			return time.Time{}, false
		case w.ResetsAt.After(last):
			last = w.ResetsAt
		}
	}
	return last, !last.IsZero()
}

// Perishability scores how urgently an account's quota needs using before
// it's lost: the unused share of its perishable window per hour until that
// window resets, which is the rate of use that would waste none of it. The
// share unused ends at the reserve, which isn't the router's to use: (1 −
// reserve − utilization) ÷ hours to reset. A reset under an hour away counts
// as an hour away. Once the window has reset, all of it but the reserve is
// unused, a whole window's length from resetting again. It reports false when
// the window is missing, or its length or reset isn't known.
func (p Policy) Perishability(windows []quota.Window, reserve float64, now time.Time) (float64, bool) {
	i := slices.IndexFunc(windows, func(w quota.Window) bool { return w.Key == p.Perishable })
	if i < 0 {
		return 0, false
	}
	w := windows[i]
	_, length, ok := span(w)
	if !ok {
		return 0, false
	}
	room := 1 - reserve
	remaining, until := room-w.Utilization, w.ResetsAt.Sub(now)
	if hasReset(w, now) {
		remaining, until = room, length
	}
	return min(max(remaining, 0), room) / max(until, minUntilReset).Hours(), true
}

// Pick chooses the account whose quota most needs using: of the candidates
// that are available within their reserves, as applies judges which windows
// count, the one with the highest perishability. Equal scores go to the
// account using less of its shortest window that applies, then to the first
// given. While preferred qualifies, Pick keeps it unless another account
// scores at least 20% higher. It reports false when no candidate qualifies.
func (p Policy) Pick(candidates []Candidate, applies func(key string) bool, preferred string, now time.Time) (string, bool) {
	ratings := p.qualifying(candidates, applies, now)
	if len(ratings) == 0 {
		return "", false
	}
	// MaxFunc returns the first of equals, which settles a full tie by order.
	best := slices.MaxFunc(ratings, rank)
	kept := slices.IndexFunc(ratings, func(r rating) bool { return r.id == preferred })
	if kept >= 0 && !worthMoving(best.score, ratings[kept].score) {
		return preferred, true
	}
	return best.id, true
}

// rating is how a candidate that qualifies ranks.
type rating struct {
	id    string
	score float64
	// shortest is how much of its shortest window that applies it has used,
	// which breaks ties.
	shortest float64
}

// qualifying rates the candidates that are available within their reserves
// and have a known score, in the order given.
func (p Policy) qualifying(candidates []Candidate, applies func(string) bool, now time.Time) []rating {
	var ratings []rating
	for _, c := range candidates {
		score, ok := p.Perishability(c.Windows, c.Reserve, now)
		if !ok || !Available(c.Windows, c.Reserve, applies, now) {
			continue
		}
		ratings = append(ratings, rating{id: c.ID, score: score, shortest: shortestUse(c.Windows, applies, now)})
	}
	return ratings
}

// shortestUse is the utilization of the shortest window that applies: 0 once
// that window has reset, or when no window applies.
func shortestUse(windows []quota.Window, applies func(string) bool, now time.Time) float64 {
	applicable := slices.DeleteFunc(slices.Clone(windows), func(w quota.Window) bool { return !applies(w.Key) })
	if len(applicable) == 0 {
		return 0
	}
	shortest := slices.MinFunc(applicable, quota.Compare)
	if hasReset(shortest, now) {
		return 0
	}
	return shortest.Utilization
}

// rank orders ratings by score, counting scores within tolerance as equal,
// then by how little of their shortest window they've used.
func rank(a, b rating) int {
	if math.Abs(a.score-b.score) > tolerance {
		return cmp.Compare(a.score, b.score)
	}
	return cmp.Compare(b.shortest, a.shortest)
}

// worthMoving reports whether score beats the preferred account's score by
// the hysteresis margin, allowing for rounding.
func worthMoving(score, preferred float64) bool {
	return score >= preferred*(1+hysteresis)-tolerance
}
