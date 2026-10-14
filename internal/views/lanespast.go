package views

import (
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// heldPast is what was judged of a timeline's past, each account as it stood
// at the moment of each column before now's, or of every column of a day or
// week gone, and what that was judged from: the timeline, by its span, its
// columns and now's; the view; the readings, as heldReadings knows them; the
// readings history's keep, and the days before it, by their dates; the
// policy; and each account, as pastOf knows it, in the moment's order.
type heldPast struct {
	start, end         time.Time
	columns, nowColumn int
	window             RunwayWindow
	read               heldReadings
	kept               time.Time
	summarised         []string
	policy             score.Policy
	accounts           []pastOf
}

// heldReadings knows the readings a past was judged from by how many there
// were, and their first and last: as the readings history only gains, oldest
// first, what's given again is what was judged from where all three are as
// they were.
type heldReadings struct {
	n           int
	first, last readings.Reading
}

// heldReadingsOf knows the readings given, as heldReadings does.
func heldReadingsOf(read []readings.Reading) heldReadings {
	if len(read) == 0 {
		return heldReadings{}
	}
	return heldReadings{n: len(read), first: read[0], last: read[len(read)-1]}
}

// pastOf is an account's past: what Then reads of the account besides its
// readings, its id, whether its token is usable, its reserve and whether it
// holds, and the holds the router told of that the readings can't show; its
// readings, oldest first, those the days before the keep stand for among
// them; the resets of the view's window they give; and how it stood at each
// column's moment.
type pastOf struct {
	id           string
	tokenSet     bool
	reserve      float64
	reserveHolds bool
	told         []status.Span
	read         []readings.Reading
	resets       []time.Time
	was          []status.Was
}

// holds reports whether the past held was judged from what's given, so
// judging it again would give it again.
func (p heldPast) holds(from LanesFrom) bool {
	tl, m := from.Timeline, from.Moment
	if !p.start.Equal(tl.Start) || !p.end.Equal(tl.End) || p.columns != tl.Columns || p.nowColumn != tl.NowColumn() ||
		p.window != from.Window || p.read != heldReadingsOf(from.Readings) || !p.kept.Equal(from.Kept) || !slices.Equal(p.summarised, from.summarisedDays()) ||
		!p.policy.Equal(m.Policy()) || len(p.accounts) != len(m.Accounts) {
		return false
	}
	for i, a := range m.Accounts {
		if !p.accounts[i].of(a, from.told(a.ID)) {
			return false
		}
	}
	return true
}

// told are the holds of the account with the given id the router told of
// that the readings can't show, as the moment's Told has them, where the
// timeline holds now; on a day or a week gone, those its events tell alone,
// as one no event tells, from before anything, is known only to hold now.
func (from LanesFrom) told(id string) []status.Span {
	told := from.Moment.Told(id)
	if from.Timeline.NowColumn() >= 0 {
		return told
	}
	return slices.DeleteFunc(told, func(s status.Span) bool { return s.From.IsZero() })
}

// of reports whether the past is the account a's, the router having told of
// the holds given.
func (p pastOf) of(a status.AccountNow, told []status.Span) bool {
	return p.id == a.ID && p.tokenSet == a.TokenSet && p.reserve == a.Reserve && p.reserveHolds == a.ReserveHolds &&
		slices.EqualFunc(p.told, told, sameSpan)
}

// sameSpan reports whether two spans are the one hold.
func sameSpan(a, b status.Span) bool {
	return a.Kind == b.Kind && a.From.Equal(b.From) && a.Until.Equal(b.Until)
}

// judgePast is the past of the timeline given, as heldPast has it, each
// account at each column's moment as l judges it stood, by its readings, the
// days before the readings history's keep standing for theirs.
func (l Lanes) judgePast(from LanesFrom) heldPast {
	m, tl, then := from.Moment, from.Timeline, l.judging()
	past := heldPast{
		start: tl.Start, end: tl.End, columns: tl.Columns, nowColumn: tl.NowColumn(), window: from.Window,
		read: heldReadingsOf(from.Readings), kept: from.Kept, summarised: from.summarisedDays(), policy: m.Policy(),
	}
	judged := max(tl.Columns, 0)
	if now := tl.NowColumn(); now >= 0 {
		judged = now
	}
	read, key := from.readingsAll(), from.viewKey()
	for _, a := range m.Accounts {
		p := pastOf{id: a.ID, tokenSet: a.TokenSet, reserve: a.Reserve, reserveHolds: a.ReserveHolds, told: from.told(a.ID)}
		p.read = slices.DeleteFunc(slices.Clone(read), func(r readings.Reading) bool { return r.Account != a.ID })
		p.resets = resetsRead(p.read, key)
		p.was = make([]status.Was, judged)
		for c := range p.was {
			at := tl.Moment(c)
			p.was[c] = then(m, a, readBy(p.read, at), from.Kept, p.told, at)
		}
		past.accounts = append(past.accounts, p)
	}
	return past
}

// readBy are the readings of those given, oldest first, read by t.
func readBy(read []readings.Reading, t time.Time) []readings.Reading {
	return read[:sort.Search(len(read), func(i int) bool { return read[i].At.After(t) })]
}

// resetsRead are the resets of the window with the given key the readings
// given give, each as read.
func resetsRead(read []readings.Reading, key string) []time.Time {
	var resets []time.Time
	for _, r := range read {
		if r.Key == key && !r.ResetsAt.IsZero() {
			resets = append(resets, r.ResetsAt)
		}
	}
	return resets
}

// readingsAll are the readings the past is judged by, each account's oldest
// first: those the days before the readings history's keep stand for, then
// those it holds, given.
func (from LanesFrom) readingsAll() []readings.Reading {
	return slices.Concat(from.summarised(), from.Readings)
}

// summarisedDays are the dates of the days given before the readings
// history's keep, which their summaries stand for.
func (from LanesFrom) summarisedDays() []string {
	var dates []string
	for _, s := range from.Days {
		if from.beforeKept(s.Day) {
			dates = append(dates, s.Day)
		}
	}
	return dates
}

// beforeKept reports whether the day with the given date starts before the
// readings history's keep, in the timeline's time zone: none does where the
// keep is zero, as nothing starts before that.
func (from LanesFrom) beforeKept(date string) bool {
	day, ok := dayfile.Civil(date)
	return ok && dayStartOf(day, from.Timeline.Start.Location()).Before(from.Kept)
}

// summarised are the readings the days given before the readings history's
// keep stand for, as their summaries have them, oldest first: of each
// account each day, a reading of each window it read at the day's start,
// with room, but where the day before left it at a limit, as Then judges
// whether that holds on; one at each reset it saw, with room; and one at each
// limit it reached, spent, till the window was to reset.
func (from LanesFrom) summarised() []readings.Reading {
	var read []readings.Reading
	for _, s := range from.Days {
		if !from.beforeKept(s.Day) {
			continue
		}
		day, _ := dayfile.Civil(s.Day)
		start := dayStartOf(day, from.Timeline.Start.Location())
		for _, a := range s.Accounts {
			read = append(read, dayRead(a, start, read)...)
		}
	}
	return read
}

// dayRead are the readings an account's day stands for, as summarised has
// them, oldest first, the day starting at start, those of the days before it
// given, oldest first.
func dayRead(a ledger.AccountDay, start time.Time, before []readings.Reading) []readings.Reading {
	var read []readings.Reading
	for _, key := range slices.Sorted(maps.Keys(a.Highest)) {
		if !leftAtLimit(before, a.Account, key) {
			read = append(read, readings.Reading{At: start, Account: a.Account, Key: key})
		}
	}
	for _, r := range a.Resets {
		read = append(read, readings.Reading{At: r.At, Account: a.Account, Key: r.Window})
	}
	for _, l := range a.Limits {
		read = append(read, readings.Reading{At: l.At, Account: a.Account, Key: l.Window, Utilization: 1, ResetsAt: l.ResetsAt, Status: quota.StatusRejected})
	}
	slices.SortStableFunc(read, byReadingTime)
	return read
}

// leftAtLimit reports whether the last of the readings given, oldest first,
// as summarised has them, of the window with the given key of the account
// with the given id is one at a limit it reached.
func leftAtLimit(read []readings.Reading, id, key string) bool {
	for _, r := range slices.Backward(read) {
		if r.Account == id && r.Key == key {
			return r.Status == quota.StatusRejected
		}
	}
	return false
}

// byReadingTime orders readings oldest first.
func byReadingTime(a, b readings.Reading) int {
	return a.At.Compare(b.At)
}
