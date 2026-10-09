package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// Weeks' headlines, by whether there are as many accounts as the use needs.
const (
	oneMoreThanYouNeed = "one more than you need"
	oneTooFew          = "one too few"
	aboutRight         = "about right"
)

// Weeks' verdicts on an account, and Accounts' on every account together.
const (
	couldGo       = "could go"
	barelyUsed    = "barely used"
	hitsItsLimit  = "hits its limit most weeks"
	plentyToSpare = "plenty to spare most weeks"
	endsWithRoom  = "ends most weeks with room"
	roomToSpare   = "room to spare together"
	littleToSpare = "little to spare together"
)

// Capacity is Weeks' verdicts over the whole weeks the days span, each
// replayed alone, as Weeks replays its last seven, those it can judge, every
// account's week's peak known in it and its week ended: Weeks, how many there
// are; Short, how many were short, a week some account hit its limit, as the
// accounts stand; WithOneFewer, how many a replay with an account fewer
// finds, dropping the one whose loss makes the fewest short, left out with
// one account; and WithOneMore, how many a replay with one more does. Verdict
// is the headline, as Weeks words it. Where it's one more than you need, Drop
// is that account, and WithTwoFewer, with three accounts or more, how many a
// replay without it and the one of the rest dropped as it was finds.
// Accounts are each configured account's verdict, in the config's order,
// then every account's together.
type Capacity struct {
	Weeks        int              `json:"weeks"`
	Short        int              `json:"short"`
	WithOneFewer *int             `json:"with_one_fewer,omitempty"`
	WithTwoFewer *int             `json:"with_two_fewer,omitempty"`
	WithOneMore  int              `json:"with_one_more"`
	Drop         string           `json:"drop,omitempty"`
	Verdict      string           `json:"verdict,omitempty"`
	Accounts     []AccountVerdict `json:"accounts"`
}

// AccountVerdict is Weeks' verdict on an account, over the whole weeks its
// week's peak is known in, its week ended, Weeks: how many it hit its limit
// in, Limits; the share its weeks had left at their resets, on average,
// LeftAtReset; and how many of the weeks Capacity judges the rest would have
// been short in without it, WithoutIt, left out with one account. Of every
// account together, All, its verdict is Accounts', over the weeks Capacity
// judges: its limits the weeks some account hit its limit in, and its share
// left every account's weeks', on average.
type AccountVerdict struct {
	Account     string   `json:"account,omitempty"`
	All         bool     `json:"all,omitempty"`
	Verdict     string   `json:"verdict,omitempty"`
	Weeks       int      `json:"weeks"`
	Limits      int      `json:"limits"`
	LeftAtReset *float64 `json:"left_at_reset,omitempty"`
	WithoutIt   *int     `json:"without_it,omitempty"`
}

// capacityOf is Capacity over the weeks of placed that isWhole reports are
// whole, of the accounts given, their plans p, their week the window with the
// given key.
func capacityOf(placed []placedWeek, isWhole func(week string) bool, accounts config.Accounts, p plans, key string) Capacity {
	r := replayOf(placed, isWhole, accounts, p, key)
	c := Capacity{Weeks: len(r.weeks), Short: r.count(r.shortAsNow), WithOneMore: r.count(r.shortWithOneMore)}
	without := make([]int, len(accounts))
	for x := range accounts {
		without[x] = r.count(func(week []millionths) bool { return r.shortWithout(week, x) })
	}
	c.judge(r, accounts, without)
	c.Accounts = make([]AccountVerdict, 0, len(accounts)+1)
	several := len(accounts) > 1
	for x, a := range accounts {
		v := recordOf(placed, isWhole, x, key).verdict(c.Verdict == oneMoreThanYouNeed && without[x] <= c.Short)
		v.Account = a.ID
		if several {
			v.WithoutIt = &without[x]
		}
		c.Accounts = append(c.Accounts, v)
	}
	all := r.together()
	all.Weeks, all.Limits = c.Weeks, c.Short
	c.Accounts = append(c.Accounts, all)
	return c
}

// judge counts the weeks short with one account fewer, of several, dropping
// the one whose loss makes the fewest short, by without, then gives the
// headline; and where that's one more than you need, names that account to
// drop and, of three or more, counts the weeks short with two fewer.
func (c *Capacity) judge(r replay, accounts config.Accounts, without []int) {
	dropped := -1
	if len(accounts) > 1 {
		dropped = r.fewest(without, -1)
		c.WithOneFewer = &without[dropped]
	}
	if c.Weeks == 0 {
		return
	}
	if c.Verdict = c.headline(); c.Verdict != oneMoreThanYouNeed {
		return
	}
	c.Drop = accounts[dropped].ID
	if len(accounts) > 2 {
		c.WithTwoFewer = r.withTwoFewer(dropped)
	}
}

// headline is Weeks' headline: one too few, where now is short on 4 or more
// of every 7 and one more would halve that; else one more than you need,
// where one fewer would be short on no more weeks than now; else about
// right. One too few comes first: where every week is short already, one
// fewer can't be short on more, so one more than you need would hold too.
func (c Capacity) headline() string {
	switch {
	case 7*c.Short >= 4*c.Weeks && 2*c.WithOneMore <= c.Short:
		return oneTooFew
	case c.WithOneFewer != nil && *c.WithOneFewer <= c.Short:
		return oneMoreThanYouNeed
	}
	return aboutRight
}

// replay is the weeks Weeks replays, each account's week's peak in each, in
// the config's order, and the accounts' sizes in Pros, with that of the plan
// most of them have, usual.
type replay struct {
	weeks [][]millionths
	sizes []int64
	usual int64
}

// replayOf is the replay of the weeks of placed that isWhole reports are
// whole and that can be judged, as judged says, the window with the given
// key, of the accounts given, sized by their plans p.
func replayOf(placed []placedWeek, isWhole func(week string) bool, accounts config.Accounts, p plans, key string) replay {
	r := replay{}
	r.sizes, r.usual = sizesOf(accounts, p)
	for _, w := range placed {
		if week, ok := w.judged(len(accounts), key); ok && isWhole(w.week) {
			r.weeks = append(r.weeks, week)
		}
	}
	return r
}

// judged is each of the accounts' peaks of the window with the given key in
// the week, reporting false where any's isn't known, or its week hasn't
// ended: a week the replay can't judge, rather than one in which that account
// had no room.
func (p placedWeek) judged(accounts int, key string) ([]millionths, bool) {
	if accounts == 0 {
		return nil, false
	}
	week := make([]millionths, accounts)
	for i := range week {
		peak, ok := p.peaks[i][key]
		if !ok || !peak.known || peak.open {
			return nil, false
		}
		week[i] = peak.use
	}
	return week, true
}

// sizesOf are the accounts' sizes in Pros, as the replay weighs them: each
// one's plan's, else that of the plan most of them have, the larger of two
// that tie, else, where none has one, 1; and that most usual size.
func sizesOf(accounts config.Accounts, p plans) (sizes []int64, usual int64) {
	sizes = make([]int64, len(accounts))
	known := make(map[int64]int)
	for i, a := range accounts {
		if plan, ok := p.of(a.ID); ok {
			sizes[i] = int64(plan.Size)
			known[sizes[i]]++
		}
	}
	usual = 1
	for size, n := range known {
		if n > known[usual] || n == known[usual] && size > usual {
			usual = size
		}
	}
	for i := range sizes {
		if sizes[i] == 0 {
			sizes[i] = usual
		}
	}
	return sizes, usual
}

// count counts the weeks short reports short.
func (r replay) count(short func(week []millionths) bool) int {
	n := 0
	for _, week := range r.weeks {
		if short(week) {
			n++
		}
	}
	return n
}

// shortAsNow reports whether some account hit its limit in the week, its peak
// at 100%.
func (r replay) shortAsNow(week []millionths) bool {
	return slices.ContainsFunc(week, func(use millionths) bool { return use >= whole })
}

// shortWithout reports whether the week would have been short without the
// accounts with the indexes dropped: each one's peak times its size is shared
// among the rest by their sizes, so each of theirs rises by all of that over
// all their sizes, and it's short where any would reach 100%.
func (r replay) shortWithout(week []millionths, dropped ...int) bool {
	var load, room int64
	for j, use := range week {
		if slices.Contains(dropped, j) {
			load += int64(use) * r.sizes[j]
		} else {
			room += r.sizes[j]
		}
	}
	for j, use := range week {
		if !slices.Contains(dropped, j) && int64(use)*room+load >= int64(whole)*room {
			return true
		}
	}
	return false
}

// shortWithOneMore reports whether the week would have been short with one
// account more, of the plan most of the accounts have: every peak falls in
// proportion, the same use over more room, and it's short where any would
// still reach 100%.
func (r replay) shortWithOneMore(week []millionths) bool {
	var room int64
	for _, size := range r.sizes {
		room += size
	}
	return slices.ContainsFunc(week, func(use millionths) bool { return int64(use)*room >= int64(whole)*(room+r.usual) })
}

// withTwoFewer is how many weeks a replay with two accounts fewer finds:
// without the account with index dropped, and the one of the rest whose loss
// then makes the fewest weeks short, as fewest finds it.
func (r replay) withTwoFewer(dropped int) *int {
	without := make([]int, len(r.sizes))
	for x := range without {
		if x != dropped {
			without[x] = r.count(func(week []millionths) bool { return r.shortWithout(week, dropped, x) })
		}
	}
	return &without[r.fewest(without, dropped)]
}

// fewest is the index of the account, but the one with index except, whose
// loss makes the fewest weeks short, by without, of those that tie the one
// that used least of the weeks replayed, and of those the first configured.
func (r replay) fewest(without []int, except int) int {
	best := -1
	for x := range without {
		switch {
		case x == except:
		case best < 0, without[x] < without[best], without[x] == without[best] && r.used(x) < r.used(best):
			best = x
		}
	}
	return best
}

// used is what the account with index x used of the weeks replayed, their
// peaks summed.
func (r replay) used(x int) millionths {
	var used millionths
	for _, week := range r.weeks {
		used += week[x]
	}
	return used
}

// together is Accounts' verdict on every account, over the weeks replayed:
// room to spare together where every account's weeks had a quarter or more
// left at a reset, on average, else little to spare together; none where no
// week is replayed.
func (r replay) together() AccountVerdict {
	var rec record
	for _, week := range r.weeks {
		for _, use := range week {
			rec.add(use)
		}
	}
	v := AccountVerdict{All: true}
	if rec.weeks == 0 {
		return v
	}
	share := meanOf(rec.left, rec.weeks)
	v.LeftAtReset, v.Verdict = &share, littleToSpare
	if 4*rec.left >= whole*millionths(rec.weeks) {
		v.Verdict = roomToSpare
	}
	return v
}

// record is how an account's weeks went: what it used of them, their peaks
// summed, and what they had left at their resets, summed; how many there
// are, and how many it hit its limit in.
type record struct {
	used, left    millionths
	weeks, limits int
}

// recordOf is the record of the account with index x over the weeks of
// placed that isWhole reports are whole and in which its week of the window
// with the given key is known and ended.
func recordOf(placed []placedWeek, isWhole func(week string) bool, x int, key string) record {
	var rec record
	for _, w := range placed {
		if peak, ok := w.peaks[x][key]; ok && peak.known && !peak.open && isWhole(w.week) {
			rec.add(peak.use)
		}
	}
	return rec
}

// add adds a week whose peak was use to the record.
func (rec *record) add(use millionths) {
	rec.used, rec.left, rec.weeks = rec.used+use, rec.left+use.left(), rec.weeks+1
	if use >= whole {
		rec.limits++
	}
}

// verdict is Weeks' verdict on the record's account, spared, where the rest
// would cover it with no more weeks short and the headline is one more than
// you need, so it could go; none where no week knows its use. The thresholds
// Weeks gives of 7 weeks hold in proportion to the weeks there are.
func (rec record) verdict(spared bool) AccountVerdict {
	v := AccountVerdict{Weeks: rec.weeks, Limits: rec.limits}
	if rec.weeks == 0 {
		return v
	}
	share := meanOf(rec.left, rec.weeks)
	v.LeftAtReset = &share
	n := millionths(rec.weeks)
	switch {
	case spared && rec.limits == 0 && 10*rec.used <= whole*n:
		v.Verdict = couldGo
	case 4*rec.left >= 3*whole*n:
		v.Verdict = barelyUsed
	case 7*rec.limits >= 3*rec.weeks:
		v.Verdict = hitsItsLimit
	case 5*rec.left >= 2*whole*n:
		v.Verdict = plentyToSpare
	default:
		v.Verdict = endsWithRoom
	}
	return v
}
