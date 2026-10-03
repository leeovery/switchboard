// Package score judges accounts by their usage windows: which have lapsed,
// how far through each window they are, how fast it's being used and where
// its use is heading, whether an account can take a request, within its
// limits or within its reserve, whether it's under pressure, how urgently its
// quota needs using, and which account to use next. Every
// function is pure and is handed the clock. None knows a provider's windows
// by name: a Policy names the ones that matter.
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
	// preferred account's before Pick leaves it: short of it, the preferred
	// account stays, so near-equal accounts don't trade places.
	hysteresis = 0.2
	// band is how far below the highest score another can fall and still be
	// near enough equal to it that the tiebreak window decides between them.
	band = 0.2
	// Tolerance is how close two scores, or two shares of a window, must be
	// to count as equal: far below any real difference, far above rounding.
	Tolerance = 1e-9
	// endless is how long until a reset nothing says is coming: longer than
	// until any that is.
	endless = time.Duration(math.MaxInt64)
)

// Policy is a provider's say in scoring, naming its windows by key.
type Policy struct {
	// Shared are the windows that apply to every model.
	Shared []string
	// Perishable is the window perishability is measured on.
	Perishable string
	// Tiebreak is the window whose reset decides between accounts scoring
	// near enough equal: what's left of it at its reset is lost.
	Tiebreak string
	// Started is the window a request starts when it isn't running. Once its
	// reset, as last read, has passed with nothing read since, it has lapsed,
	// and it isn't running until a request starts it again.
	Started string
	// Pressure is the window whose pace of use is watched: an account that,
	// at the rate it's being used, runs it out before it resets is under
	// pressure, and Pick passes it over while it can pick another.
	Pressure string
}

// IsShared reports whether the window named key applies to every model.
func (p Policy) IsShared(key string) bool {
	return slices.Contains(p.Shared, key)
}

// Lapsed returns the keys of the windows, in the order given, that have
// lapsed at now: the window a request starts, its reset passed with nothing
// read since.
func (p Policy) Lapsed(windows []quota.Window, now time.Time) []string {
	var keys []string
	for _, w := range windows {
		if p.lapsed(w, now) {
			keys = append(keys, w.Key)
		}
	}
	return keys
}

// AsOf returns windows, in the order given, as they stand at now: one that
// has lapsed reads empty, nothing used and no reset, as it isn't running, and
// every other stands as read, whether its reset has passed or not.
func (p Policy) AsOf(windows []quota.Window, now time.Time) []quota.Window {
	standing := slices.Clone(windows)
	for i, w := range standing {
		if p.lapsed(w, now) {
			standing[i] = quota.Window{Key: w.Key, Label: w.Label}
		}
	}
	return standing
}

// lapsed reports whether w has lapsed at now: it's the window a request
// starts, and it has reset since it was read.
func (p Policy) lapsed(w quota.Window, now time.Time) bool {
	return w.Key == p.Started && hasReset(w, now)
}

// Candidate is an account Pick can choose.
type Candidate struct {
	ID      string
	Windows []quota.Window
	// Reserve is the share of every window Pick leaves unused on the
	// account: a window's room ends at 1 − Reserve.
	Reserve float64
	// Rate is how fast the account's pressure window is being used, as a
	// share of it an hour: zero when that isn't known.
	Rate float64
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
	w, ok := find(windows, p.Perishable)
	if !ok {
		return 0, false
	}
	length, ok := quota.Length(w.Key)
	if !ok || w.ResetsAt.IsZero() {
		return 0, false
	}
	room := 1 - reserve
	remaining, until := room-w.Utilization, w.ResetsAt.Sub(now)
	if hasReset(w, now) {
		remaining, until = room, length
	}
	return min(max(remaining, 0), room) / max(until, minUntilReset).Hours(), true
}

// Choice is the account Pick chooses, and the one under pressure it passed
// over, when passing over those under pressure changed its choice.
type Choice struct {
	ID string
	// PassedOver is the account under pressure that Pick would have chosen,
	// or, when that one isn't, the highest scoring of those under pressure,
	// whose score set the band the choice would have been made in. It's ""
	// when pressure changed nothing.
	PassedOver string
	// Pressure is how PassedOver stands under pressure, as Pick judged it,
	// at the room the candidate gave it: zero when PassedOver is "".
	Pressure Pressure
}

// Pick chooses the account whose quota most needs using: of the candidates
// that are available within their reserves, as applies judges which windows
// count, and that aren't under pressure, as PressureOf judges them, unless
// every one is, the one with the highest perishability. Scores within 20% of
// the highest, at least 0.8 of it, are near enough equal that the tiebreak
// window decides between them: the account whose tiebreak window resets
// soonest wins, as what's left of it then is lost, ahead of any whose reset
// isn't known or has passed, which rank alike. Equal resets go to the higher
// score, then to the account using less of its shortest window that applies,
// then to the first given. While preferred qualifies, Pick keeps it unless
// another account scores at least 20% higher. It reports false when no
// candidate qualifies.
func (p Policy) Pick(candidates []Candidate, applies func(key string) bool, preferred string, now time.Time) (Choice, bool) {
	ratings := p.qualifying(candidates, applies, now)
	if len(ratings) == 0 {
		return Choice{}, false
	}
	chosen := among(ratings)
	c := Choice{ID: best(chosen, preferred)}
	if regardless := best(ratings, preferred); len(chosen) < len(ratings) && regardless != c.ID {
		passed := passedOver(ratings, regardless)
		c.PassedOver, c.Pressure = passed.id, passed.pressure
	}
	return c, true
}

// Among returns the ids of the candidates Pick chooses among, in the order
// given: those that qualify, as Pick says, but those under pressure while
// another isn't.
func (p Policy) Among(candidates []Candidate, applies func(key string) bool, now time.Time) []string {
	var ids []string
	for _, r := range among(p.qualifying(candidates, applies, now)) {
		ids = append(ids, r.id)
	}
	return ids
}

// among are the ratings Pick chooses among: those not under pressure, or
// every one where each is.
func among(ratings []rating) []rating {
	if relieved := slices.DeleteFunc(slices.Clone(ratings), func(r rating) bool { return r.pressure.Under }); len(relieved) > 0 {
		return relieved
	}
	return ratings
}

// best is the account of those rated whose quota most needs using, keeping to
// preferred unless another scores at least 20% higher, as Pick says.
func best(ratings []rating, preferred string) string {
	highest := slices.MaxFunc(ratings, byScore).score
	if i := slices.IndexFunc(ratings, func(r rating) bool { return r.id == preferred }); i >= 0 && !worthMoving(highest, ratings[i].score) {
		return preferred
	}
	near := slices.DeleteFunc(slices.Clone(ratings), func(r rating) bool { return !nearEnough(r.score, highest) })
	// MinFunc returns the first of equals, which settles a full tie by order.
	return slices.MinFunc(near, rank).id
}

// passedOver returns the rating of the account under pressure that turned
// Pick's choice from regardless, the one it makes of every rating:
// regardless's, when it's under pressure, else the highest scoring of those
// under pressure, whose score set the band regardless was chosen in.
func passedOver(ratings []rating, regardless string) rating {
	pressed := slices.DeleteFunc(slices.Clone(ratings), func(r rating) bool { return !r.pressure.Under })
	if i := slices.IndexFunc(pressed, func(r rating) bool { return r.id == regardless }); i >= 0 {
		return pressed[i]
	}
	return slices.MaxFunc(pressed, byScore)
}

// rating is how a candidate that qualifies ranks.
type rating struct {
	id    string
	score float64
	// untilTiebreak is how long until its tiebreak window resets, which
	// decides between near-equal scores.
	untilTiebreak time.Duration
	// shortest is how much of its shortest window that applies it has used,
	// which breaks ties.
	shortest float64
	// pressure is how it stands under pressure: under it, it's set aside.
	pressure Pressure
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
		ratings = append(ratings, rating{
			id:            c.ID,
			score:         score,
			untilTiebreak: p.untilTiebreak(c.Windows, applies, now),
			shortest:      shortestUse(c.Windows, applies, now),
			pressure:      p.PressureOf(c, now),
		})
	}
	return ratings
}

// untilTiebreak returns how long until the tiebreak window among windows
// resets: endless when it doesn't count the request, as applies judges, or
// when no reset of it is known to be coming, as when it has lapsed, its reset
// passed with nothing read since.
func (p Policy) untilTiebreak(windows []quota.Window, applies func(string) bool, now time.Time) time.Duration {
	w, ok := find(windows, p.Tiebreak)
	if !ok || !applies(w.Key) || !w.ResetsAt.After(now) {
		return endless
	}
	return w.ResetsAt.Sub(now)
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

// find returns the window named key, reporting false when there's none.
func find(windows []quota.Window, key string) (quota.Window, bool) {
	i := slices.IndexFunc(windows, func(w quota.Window) bool { return w.Key == key })
	if i < 0 {
		return quota.Window{}, false
	}
	return windows[i], true
}

// byScore orders ratings by their scores, lowest first.
func byScore(a, b rating) int {
	return cmp.Compare(a.score, b.score)
}

// rank orders ratings best first: the sooner their tiebreak windows reset,
// then the higher their scores, counting scores within Tolerance as equal,
// then the less of their shortest windows they've used.
func rank(a, b rating) int {
	return cmp.Or(
		cmp.Compare(a.untilTiebreak, b.untilTiebreak),
		compareScores(b.score, a.score),
		cmp.Compare(a.shortest, b.shortest),
	)
}

// compareScores compares two scores as cmp.Compare does, counting those
// within Tolerance as equal.
func compareScores(a, b float64) int {
	if math.Abs(a-b) <= Tolerance {
		return 0
	}
	return cmp.Compare(a, b)
}

// worthMoving reports whether score beats the preferred account's score by
// the hysteresis margin, allowing for rounding. A score no higher never beats
// it, however small the two are.
func worthMoving(score, preferred float64) bool {
	return score > preferred && score >= preferred*(1+hysteresis)-Tolerance
}

// nearEnough reports whether score is within the band below the highest,
// allowing for rounding.
func nearEnough(score, highest float64) bool {
	return score >= highest*(1-band)-Tolerance
}
