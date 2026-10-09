package views

import (
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
// replayed alone, as Weeks replays its last seven, those in which some
// account's week's peak is known: Weeks, how many there are; Short, how many
// were short, a week some account hit its limit, as the accounts stand;
// WithOneFewer, how many a replay with an account fewer finds, dropping the
// one whose loss makes the fewest short, left out with one account; and
// WithOneMore, how many a replay with one more does. Drop is the account
// whose loss makes no more short than now, where there's one. Verdict is the
// headline, as Weeks words it. Accounts are each configured account's
// verdict, in the config's order, then every account's together.
type Capacity struct {
	Weeks        int              `json:"weeks"`
	Short        int              `json:"short"`
	WithOneFewer *int             `json:"with_one_fewer,omitempty"`
	WithOneMore  int              `json:"with_one_more"`
	Drop         string           `json:"drop,omitempty"`
	Verdict      string           `json:"verdict,omitempty"`
	Accounts     []AccountVerdict `json:"accounts"`
}

// AccountVerdict is Weeks' verdict on an account, over the whole weeks its
// week's peak is known in, Weeks: how many it hit its limit in, Limits; the
// share its weeks had left at their resets, on average, LeftAtReset; and how
// many the rest would have been short without it, WithoutIt, left out with
// one account. Of every account together, All, its verdict is Accounts', its
// limits the weeks some account hit its limit in, and its share left every
// account's weeks', on average.
type AccountVerdict struct {
	Account     string   `json:"account,omitempty"`
	All         bool     `json:"all,omitempty"`
	Verdict     string   `json:"verdict,omitempty"`
	Weeks       int      `json:"weeks"`
	Limits      int      `json:"limits"`
	LeftAtReset *float64 `json:"left_at_reset,omitempty"`
	WithoutIt   *int     `json:"without_it,omitempty"`
}

// capacityOf is Capacity over the weeks of placed that whole reports are
// whole, of the accounts given, their plans p, their week the window with the
// given key.
func capacityOf(placed []placedWeek, whole func(week string) bool, accounts config.Accounts, p plans, key string) Capacity {
	r := replayOf(placed, whole, accounts, p, key)
	c := Capacity{Weeks: len(r.weeks), Short: r.count(r.shortAsNow), WithOneMore: r.count(r.shortWithOneMore)}
	several := len(accounts) > 1
	records, without := make([]record, len(accounts)), make([]int, len(accounts))
	for x := range accounts {
		records[x] = r.recordOf(x)
		without[x] = r.count(func(week []peak) bool { return r.shortWithout(week, x) })
	}
	if several {
		fewest := fewest(without, records)
		c.WithOneFewer = &without[fewest]
		if c.Weeks > 0 && without[fewest] <= c.Short {
			c.Drop = accounts[fewest].ID
		}
	}
	if c.Weeks > 0 {
		c.Verdict = c.headline()
	}
	c.Accounts = make([]AccountVerdict, 0, len(accounts)+1)
	var every record
	for x, a := range accounts {
		v := records[x].verdict(several && without[x] <= c.Short)
		v.Account = a.ID
		if several {
			v.WithoutIt = &without[x]
		}
		c.Accounts = append(c.Accounts, v)
		every.add(records[x])
	}
	all := every.together()
	all.Weeks, all.Limits = c.Weeks, c.Short
	c.Accounts = append(c.Accounts, all)
	return c
}

// headline is Weeks' headline: one more than you need, where one fewer would
// be short on no more weeks than now; one too few, where now is short on 4
// or more of every 7 and one more would halve that; else about right.
func (c Capacity) headline() string {
	switch {
	case c.WithOneFewer != nil && *c.WithOneFewer <= c.Short:
		return oneMoreThanYouNeed
	case 7*c.Short >= 4*c.Weeks && 2*c.WithOneMore <= c.Short:
		return oneTooFew
	}
	return aboutRight
}

// fewest is the index of the account whose loss makes the fewest weeks
// short, by without, of those that tie the one that used least of its
// weeks, by their records, and of those the first configured.
func fewest(without []int, records []record) int {
	best := 0
	for x := 1; x < len(without); x++ {
		if without[x] < without[best] || without[x] == without[best] && records[x].usedLess(records[best]) {
			best = x
		}
	}
	return best
}

// peak is an account's week's peak in a week, where known.
type peak struct {
	use   millionths
	known bool
}

// replay is the weeks Weeks replays, each account's week's peak in each, in
// the config's order, and the accounts' sizes in Pros, with that of the plan
// most of them have, usual.
type replay struct {
	weeks [][]peak
	sizes []int64
	usual int64
}

// replayOf is the replay of the weeks of placed that whole reports are whole,
// and in which some account's week's peak is known, the window with the
// given key, of the accounts given, sized by their plans p.
func replayOf(placed []placedWeek, whole func(week string) bool, accounts config.Accounts, p plans, key string) replay {
	r := replay{}
	r.sizes, r.usual = sizesOf(accounts, p)
	for _, w := range placed {
		if !whole(w.week) {
			continue
		}
		week, known := make([]peak, len(accounts)), false
		for i := range accounts {
			if use, ok := w.peaks[i][key]; ok {
				week[i], known = peak{use: use, known: true}, true
			}
		}
		if known {
			r.weeks = append(r.weeks, week)
		}
	}
	return r
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
func (r replay) count(short func(week []peak) bool) int {
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
func (r replay) shortAsNow(week []peak) bool {
	for _, p := range week {
		if p.known && p.use >= whole {
			return true
		}
	}
	return false
}

// shortWithout reports whether the week would have been short without the
// account with index x: its peak times its size is shared among the rest by
// their sizes, so each one's peak rises by that over all their sizes, and
// it's short where any would reach 100%. Where no other's peak is known, it's
// short where x used any of its week.
func (r replay) shortWithout(week []peak, x int) bool {
	var load, room int64
	if week[x].known {
		load = int64(week[x].use) * r.sizes[x]
	}
	for j, p := range week {
		if j != x && p.known {
			room += r.sizes[j]
		}
	}
	if room == 0 {
		return load > 0
	}
	for j, p := range week {
		if j != x && p.known && int64(p.use)*room+load >= int64(whole)*room {
			return true
		}
	}
	return false
}

// shortWithOneMore reports whether the week would have been short with one
// account more, of the plan most of the accounts have: every peak falls in
// proportion, the same use over more room, and it's short where any would
// still reach 100%.
func (r replay) shortWithOneMore(week []peak) bool {
	var room int64
	for j, p := range week {
		if p.known {
			room += r.sizes[j]
		}
	}
	for _, p := range week {
		if p.known && int64(p.use)*room >= int64(whole)*(room+r.usual) {
			return true
		}
	}
	return false
}

// record is how an account's weeks went, over those its week's peak is known
// in: what it used of them, their peaks summed, and what they had left at
// their resets, summed; how many there are, and how many it hit its limit
// in.
type record struct {
	used, left    millionths
	weeks, limits int
}

// recordOf is the record of the account with index x.
func (r replay) recordOf(x int) record {
	var rec record
	for _, week := range r.weeks {
		p := week[x]
		if !p.known {
			continue
		}
		rec.used, rec.left, rec.weeks = rec.used+p.use, rec.left+p.use.left(), rec.weeks+1
		if p.use >= whole {
			rec.limits++
		}
	}
	return rec
}

// add adds other's weeks to the record's.
func (rec *record) add(other record) {
	rec.used += other.used
	rec.left += other.left
	rec.weeks += other.weeks
	rec.limits += other.limits
}

// usedLess reports whether the record's account used less of its weeks, on
// average, than other's: one whose use no week knows never does, as its use
// isn't known.
func (rec record) usedLess(other record) bool {
	switch {
	case rec.weeks == 0:
		return false
	case other.weeks == 0:
		return true
	}
	return int64(rec.used)*int64(other.weeks) < int64(other.used)*int64(rec.weeks)
}

// verdict is Weeks' verdict on the record's account, spared, where the rest
// would cover it with no more weeks short; none where no week knows its use.
// The thresholds Weeks gives of 7 weeks hold in proportion to the weeks
// there are.
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

// together is Accounts' verdict on every account, the record every account's
// weeks together: room to spare together where they had a quarter or more
// left at a reset, on average, else little to spare together; none where no
// week knows any account's use.
func (rec record) together() AccountVerdict {
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
