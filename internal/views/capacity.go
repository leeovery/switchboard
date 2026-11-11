package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// Weeks' headlines, by whether there are as many accounts as the use needs:
// what a Capacity's Verdict says, which the dashboard colours it by.
const (
	OneMoreThanYouNeed = "one more than you need"
	OneTooFew          = "one too few"
	TooFew             = "too few"
	AboutRight         = "about right"
)

// Weeks' verdicts on an account, and Accounts' on every account together:
// what an AccountVerdict's Verdict says, which the dashboard colours it by.
const (
	CouldGo       = "could go"
	BarelyUsed    = "barely used"
	HitsItsLimit  = "hits its limit most weeks"
	PlentyToSpare = "plenty to spare most weeks"
	EndsWithRoom  = "ends most weeks with room"
	RoomToSpare   = "room to spare together"
	LittleToSpare = "little to spare together"
)

// The cuts the verdicts on an account are judged by, as Weeks gives them of
// VerdictWeeks weeks, held in proportion to the weeks there are: it hits its
// limit most weeks where it hit it in LimitWeeks of them or more; it has
// plenty to spare, or is barely used, where its weeks had PlentyLeft, or
// BarelyLeft, percent or more left at a reset, on average; and it could go
// where it used CouldGoUsed percent or less of them, on average. Every
// account together has room to spare where their weeks had RoomLeft percent
// or more left.
const (
	VerdictWeeks = 7
	LimitWeeks   = 3
	PlentyLeft   = 40
	BarelyLeft   = 75
	CouldGoUsed  = 10
	RoomLeft     = 25
)

// Capacity is Weeks' verdicts over the whole weeks the days span, each
// replayed alone, as Weeks replays its last seven, those it can judge, every
// account's week's peak known in it and its week ended: Weeks, how many there
// are; Short, how many were short, a week some account hit its limit, as the
// accounts stand; WithOneFewer, how many a replay with an account fewer
// finds, dropping the one whose loss makes the fewest short, left out with
// one account; and WithOneMore, how many a replay with one more does. An
// account hits its limit where its week's peak reaches its cap, as capOf has
// it. Verdict
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
	Account string `json:"account,omitempty"`
	status.AccountName
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
	return judgingOf(placed, isWhole, accounts, p, key).capacity()
}

// judging is what Weeks judges the accounts by: the accounts, the replay,
// and each account's record, in the config's order.
type judging struct {
	accounts config.Accounts
	r        replay
	records  []record
}

// judgingOf is what Weeks judges by over the weeks of placed that isWhole
// reports are whole, of the accounts given, their plans p, their week the
// window with the given key.
func judgingOf(placed []placedWeek, isWhole func(week string) bool, accounts config.Accounts, p plans, key string) judging {
	j := judging{accounts: accounts, r: replayOf(placed, isWhole, accounts, p, key), records: make([]record, len(accounts))}
	for x := range accounts {
		j.records[x] = recordOf(placed, isWhole, x, j.r.caps[x], key)
	}
	return j
}

// capacity is Capacity of the accounts judged.
func (j judging) capacity() Capacity {
	r, accounts := j.r, j.accounts
	c := Capacity{Weeks: len(r.weeks), Short: r.count(r.shortAsNow), WithOneMore: r.count(r.shortWithOneMore)}
	without := make([]int, len(accounts))
	for x := range accounts {
		without[x] = r.count(func(week []millionths) bool { return r.shortWithout(week, x) })
	}
	c.judge(r, accounts, without)
	c.Accounts = make([]AccountVerdict, 0, len(accounts)+1)
	several := len(accounts) > 1
	for x, a := range accounts {
		v := j.records[x].verdict(c.Verdict == OneMoreThanYouNeed && without[x] <= c.Short)
		v.Account, v.AccountName = a.ID, status.ConfiguredName(a)
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
	if c.Verdict = c.headline(); c.Verdict != OneMoreThanYouNeed {
		return
	}
	c.Drop = accounts[dropped].ID
	if len(accounts) > 2 {
		c.WithTwoFewer = r.withTwoFewer(dropped)
	}
}

// headline is Weeks' headline: where now is short on 4 or more of every 7,
// one too few, where one more would halve that, else too few; otherwise one
// more than you need, where one fewer would be short on no more weeks than
// now; else about right. A set short on 4 or more never has one more than it
// needs: where every week is short already, one fewer can't be short on
// more, which says nothing of what it needs.
func (c Capacity) headline() string {
	short := 7*c.Short >= 4*c.Weeks
	switch {
	case short && 2*c.WithOneMore <= c.Short:
		return OneTooFew
	case short:
		return TooFew
	case c.WithOneFewer != nil && *c.WithOneFewer <= c.Short:
		return OneMoreThanYouNeed
	}
	return AboutRight
}

// replay is the weeks Weeks replays, each account's week's peak in each, in
// the config's order, the accounts' caps, as capOf has them, and their sizes
// in Pro weeks, with that of the plan most of them have, usual.
type replay struct {
	weeks [][]millionths
	caps  []millionths
	sizes []int64
	usual int64
}

// replayOf is the replay of the weeks of placed that isWhole reports are
// whole and that can be judged, as judged says, the window with the given
// key, of the accounts given, sized by their plans p.
func replayOf(placed []placedWeek, isWhole func(week string) bool, accounts config.Accounts, p plans, key string) replay {
	r := replay{caps: make([]millionths, len(accounts))}
	for i, a := range accounts {
		r.caps[i] = capOf(a)
	}
	r.sizes, r.usual = sizesOf(accounts, p.table)
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

// sizesOf are the accounts' sizes in Pro weeks, as the replay weighs them,
// and that of the plan most of them have, as the price table sizes them.
func sizesOf(accounts config.Accounts, table ledger.Table) (sizes []int64, usual int64) {
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.Plan
	}
	return table.Sizes(ids)
}

// capOf is the share of its week where the account stops taking new sessions,
// as the router holds it: its cap, where its reserve starts, else its limit.
// It's the reserve the config has now, as a week's own isn't recorded beyond
// the moments an account was held at its cap.
func capOf(a config.Account) millionths {
	return millionthsOf(score.ReserveFrom(a.Reserve))
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
// at its cap.
func (r replay) shortAsNow(week []millionths) bool {
	return r.shortIn(week) != nil
}

// shortWithout reports whether the week would have been short without the
// accounts with the indexes dropped: each one's peak times its size is shared
// among the rest by their sizes, so each of theirs rises by all of that over
// all their sizes, and it's short where any would reach its cap.
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
		if !slices.Contains(dropped, j) && int64(use)*room+load >= int64(r.caps[j])*room {
			return true
		}
	}
	return false
}

// shortIn are the indexes of the accounts that hit their limit in the week,
// their peaks at their caps.
func (r replay) shortIn(week []millionths) []int {
	var short []int
	for x, use := range week {
		if use >= r.caps[x] {
			short = append(short, x)
		}
	}
	return short
}

// shortEach are the indexes of the accounts short in each week short as the
// accounts stand, where the same ones were each time: none where they
// differed, or none was short.
func (r replay) shortEach() []int {
	var each []int
	for _, week := range r.weeks {
		short := r.shortIn(week)
		switch {
		case short == nil:
		case each == nil:
			each = short
		case !slices.Equal(short, each):
			return nil
		}
	}
	return each
}

// without compares the weeks a replay without the account with index x
// finds short with those short as the accounts stand: same, where they're
// the very weeks, and some are short; and noneNew, where none it finds short
// wasn't already.
func (r replay) without(x int) (same, noneNew bool) {
	same, noneNew = true, true
	some := false
	for _, week := range r.weeks {
		was, is := r.shortAsNow(week), r.shortWithout(week, x)
		same = same && was == is
		noneNew = noneNew && (was || !is)
		some = some || was
	}
	return same && some, noneNew
}

// shortWithOneMore reports whether the week would have been short with one
// account more, of the plan most of the accounts have: every peak falls in
// proportion, the same use over more room, and it's short where any would
// still reach its cap.
func (r replay) shortWithOneMore(week []millionths) bool {
	var room int64
	for _, size := range r.sizes {
		room += size
	}
	for x, use := range week {
		if int64(use)*room >= int64(r.caps[x])*(room+r.usual) {
			return true
		}
	}
	return false
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
	v.LeftAtReset, v.Verdict = &share, LittleToSpare
	if rec.leftAtLeast(RoomLeft) {
		v.Verdict = RoomToSpare
	}
	return v
}

// record is how an account's weeks went: what it used of them, their peaks
// summed, and what they had left at their resets, summed; how many there
// are, and how many it hit its limit in, its peak at its cap.
type record struct {
	used, left    millionths
	weeks, limits int
}

// recordOf is the record of the account with index x, its cap that given,
// over the weeks of placed that isWhole reports are whole and in which its
// week of the window with the given key is known and ended.
func recordOf(placed []placedWeek, isWhole func(week string) bool, x int, capAt millionths, key string) record {
	var rec record
	for _, w := range placed {
		if peak, ok := w.peaks[x][key]; ok && peak.known && !peak.open && isWhole(w.week) {
			rec.add(peak.use)
			if peak.use >= capAt {
				rec.limits++
			}
		}
	}
	return rec
}

// add adds a week whose peak was use to what the record's weeks used and
// had left, and how many there are.
func (rec *record) add(use millionths) {
	rec.used, rec.left, rec.weeks = rec.used+use, rec.left+use.left(), rec.weeks+1
}

// leftAtLeast reports whether the record's weeks had percent or more left
// at a reset, on average, as leftSaid has it.
func (rec record) leftAtLeast(percent int) bool {
	return rec.leftSaid() >= percent
}

// leftSaid is what the record's weeks had left at a reset, on average, and
// usedSaid what they used, each as the whole percentage the figure beside
// the verdict says it, which the verdicts are judged on, so none sits beside
// a figure that says otherwise.
func (rec record) leftSaid() int {
	return prose.WholePercent(meanOf(rec.left, rec.weeks))
}

func (rec record) usedSaid() int {
	return prose.WholePercent(meanOf(rec.used, rec.weeks))
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
	switch {
	case spared && rec.limits == 0 && rec.usedSaid() <= CouldGoUsed:
		v.Verdict = CouldGo
	case rec.leftAtLeast(BarelyLeft):
		v.Verdict = BarelyUsed
	case VerdictWeeks*rec.limits >= LimitWeeks*rec.weeks:
		v.Verdict = HitsItsLimit
	case rec.leftAtLeast(PlentyLeft):
		v.Verdict = PlentyToSpare
	default:
		v.Verdict = EndsWithRoom
	}
	return v
}
