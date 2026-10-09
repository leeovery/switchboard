package ledger

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// Caps say where each account's cap is, as a summary counts the minutes the
// account spent at its cap, and at a limit.
type Caps struct {
	// Reserves are the accounts' reserves, by their ids, as the config gives
	// them as the summary is made, as the readings history keeps none: an
	// account's cap is 1 less its reserve, and one without a reserve has no
	// cap.
	Reserves map[string]float64
	// Shared are the keys of the windows every model shares: a window of a
	// model's own, as Fable's week, holds back that model alone, and counts
	// toward neither minute.
	Shared []string
}

// CapsOf returns the caps of the accounts configured, the windows every model
// shares those shared names by their keys.
func CapsOf(accounts []config.Account, shared []string) Caps {
	reserves := make(map[string]float64, len(accounts))
	for _, a := range accounts {
		reserves[a.ID] = a.Reserve
	}
	return Caps{Reserves: reserves, Shared: shared}
}

// windowDay is how a window of an account went through a day, as it's
// summarised from its readings, taken in the order they came, each standing
// from when it was read until the next, its reset, or until, where the day
// ends, or has gone as far as it has: from the last reading of the week
// before the day, where it hadn't reset by the day's start, standing from
// then.
//
// Its climb is how far its use rose, as Climb says, and which reading it
// stands at: from the use carried into the day, or nothing after a reset, as
// the window empties. A run whose start is unknown, of a window first read
// that day, nothing read of it in the week before, begins at the use it was
// first read at, unless the window began within the day, as its reset and its
// length say.
type windowDay struct {
	start, until time.Time
	climb        *Climb
	stood        []stood
}

// newWindowDay returns the window with the given key's day that starts at
// start, until until, before any reading of it is taken.
func newWindowDay(key string, start, until time.Time) *windowDay {
	return &windowDay{start: start, until: until, climb: NewClimb(key, func(w quota.Window) float64 { return beganAt(w, start) })}
}

// stood is a reading of a window, standing from a time until another.
type stood struct {
	from, to time.Time
	window   quota.Window
}

// carry takes w, the last reading of the window in the week before the day,
// which hadn't reset by the day's start, as the use the window began the day
// at, standing from the day's start.
func (d *windowDay) carry(w quota.Window) {
	d.climb.Carry(w, d.start)
}

// afresh notes that the window began the day empty: read in the week before,
// it had reset by the day's start.
func (d *windowDay) afresh() {
	d.climb.BeginAt(0)
}

// read reports whether the window's day has a reading it stands at.
func (d *windowDay) read() bool {
	return d.climb.read
}

// take takes w, a reading of the window read at at, in the day, as its climb
// takes it: the reading it stood at before stands until then, or its reset.
// Nor does one read from until on say anything of the window.
func (d *windowDay) take(w quota.Window, at time.Time) {
	if !at.Before(d.until) {
		return
	}
	last, from, read := d.climb.last, d.climb.at, d.climb.read
	if _, ok := d.climb.Take(w, at); ok && read {
		d.stand(last, from, at)
	}
}

// end ends the window's day at until: the reading it stands at stands until
// then, or until its reset, which ends its run, where that comes first.
func (d *windowDay) end() {
	d.stand(d.climb.last, d.climb.at, d.until)
	d.climb.End(d.until)
}

// stand has w, a reading of the window read at from, stand until to, or until
// its reset, where that comes first.
func (d *windowDay) stand(w quota.Window, from, to time.Time) {
	if w.ResetBy(to) {
		to = w.ResetsAt
	}
	if from.Before(to) {
		d.stood = append(d.stood, stood{from: from, to: to, window: w})
	}
}

// Climb follows a window's use through its readings, taken in the order they
// were read: one rule for how far each reading climbs it. Each run of the
// window, from a reset to the next, climbs from the use it began at to its
// highest, a reading under that highest, as one taken out of turn gives,
// climbing it none; the climbs of its runs added together are its rise. A
// day's summary climbs each window so through the day's readings, and a
// session's page through its accounts' answers, sharing each climb out.
type Climb struct {
	key string
	// began gives the use the window's run began at before its first
	// reading, unless BeginAt or Carry says.
	began func(first quota.Window) float64
	// last is the reading taken last, read at at, once read is set; top is
	// the highest use since its run began, once based is set; and latest is
	// the latest reset read.
	last   quota.Window
	at     time.Time
	read   bool
	top    float64
	based  bool
	latest time.Time
	rise   float64
	// resets are the resets that ended its runs, and ended each run's reset
	// as its readings gave it.
	resets []Reset
	ended  []time.Time
}

// NewClimb returns the climb of the window with the given key before any
// reading of it is taken: unless BeginAt or Carry says otherwise, its run
// begins at the use began gives of its first reading taken, or, where began
// is nil, the use that reading reads.
func NewClimb(key string, began func(first quota.Window) float64) *Climb {
	if began == nil {
		began = func(w quota.Window) float64 { return w.Utilization }
	}
	return &Climb{key: key, began: began}
}

// BeginAt begins the window's run at use, before any reading of it is
// taken.
func (c *Climb) BeginAt(use float64) {
	c.top, c.based = use, true
}

// Carry takes w, the last reading of the window before those followed, read
// at at, as the use its run began at.
func (c *Climb) Carry(w quota.Window, at time.Time) {
	c.last, c.at, c.read, c.latest = w, at, true, w.ResetsAt
	c.BeginAt(w.Utilization)
}

// Take takes w, the window's next reading, read at at, and returns how far it
// climbed the window's use above its highest since its run began: a reset
// between the reading before and w, as resetBetween tells it, ending that
// run, the next beginning at nothing. It reports false, taking nothing toward
// the climb, of a reading that says nothing of the run since: one past its
// own reset as it was read, as an answer that came late gives, the window
// having started afresh since; or one whose reset is earlier than the latest
// read, a reading of an earlier run read late, as late takes it.
func (c *Climb) Take(w quota.Window, at time.Time) (float64, bool) {
	switch {
	case w.ResetBy(at):
		return 0, false
	case !w.ResetsAt.IsZero() && w.ResetsAt.Before(c.latest):
		c.late(w)
		return 0, false
	}
	if c.read {
		if reset, ok := resetBetween(c.last, w, at); ok {
			c.reset(reset)
		}
	}
	if !c.based {
		c.BeginAt(c.began(w))
	}
	climbed := max(w.Utilization-c.top, 0)
	c.top, c.rise = max(c.top, w.Utilization), c.rise+climbed
	c.last, c.at, c.read = w, at, true
	if w.ResetsAt.After(c.latest) {
		c.latest = w.ResetsAt
	}
	return climbed, true
}

// late takes w, a reading of a run of the window that has reset since, read
// late, as an answer that began before the reset and ended after it gives
// one: a true reading of that run, which says nothing of the run since.
// Where it reads higher than the use that run was last read at, the run's
// reset takes it as the use before it.
func (c *Climb) late(w quota.Window) {
	if i := slices.IndexFunc(c.ended, w.ResetsAt.Equal); i >= 0 {
		c.resets[i].Before = max(c.resets[i].Before, w.Utilization)
	}
}

// End ends the window's climb at until, noting the reset of the reading taken
// last where it came by then.
func (c *Climb) End(until time.Time) {
	if c.read && c.last.ResetBy(until) {
		c.reset(c.last.ResetsAt)
	}
}

// Rise returns how far the window's use climbed, its runs' climbs added
// together.
func (c *Climb) Rise() float64 {
	return c.rise
}

// reset notes the window's reset at at, its use as last read before it, and
// ends the run it was in, the next beginning at nothing.
func (c *Climb) reset(at time.Time) {
	c.resets = append(c.resets, Reset{Window: c.key, At: at.UTC(), Before: c.last.Utilization})
	c.ended = append(c.ended, c.last.ResetsAt)
	c.top = 0
}

// resetBetween returns when a window read was reset before w, the next
// reading of it, read at at, reporting false where the two tell of no reset:
// w shows it reset by hand, as score.ResetByHand says, at at; was's reset had
// passed by at, w being of another window, or of none, at was's reset; or w's
// reset is later than was's, which hadn't come, the window having started
// again before it, at at.
func resetBetween(was, w quota.Window, at time.Time) (time.Time, bool) {
	switch {
	case score.ResetByHand(was, w):
		return at, true
	case was.ResetsAt.IsZero() || w.ResetsAt.Equal(was.ResetsAt):
		return time.Time{}, false
	case was.ResetBy(at):
		return was.ResetsAt, true
	case w.ResetsAt.After(was.ResetsAt):
		return at, true
	}
	return time.Time{}, false
}

// beganAt returns the use a window first read as w on the day that starts at
// start, nothing read of it in the week before, began its run at: nothing,
// where the window began within the day, as its reset and its length say;
// else w's use, as what it was before is unknown.
func beganAt(w quota.Window, start time.Time) float64 {
	if began, _, ok := w.Span(); ok && !began.Before(start) {
		return 0
	}
	return w.Utilization
}

// risesIn is how many parts of a whole a window's rise is rounded to: a
// millionth, finer than any reading gives a use, so a rise of readings added
// reads as they do, with no trace of the floating point's sums.
const risesIn = 1e6

// risesOf returns how far each of windows rose that day, by its key, as
// windowDay says: empty, never nil, where none was read, as a summary tells
// none from never read.
func risesOf(windows map[string]*windowDay) map[string]float64 {
	rises := make(map[string]float64, len(windows))
	for key, d := range windows {
		rises[key] = math.Round(d.climb.Rise()*risesIn) / risesIn
	}
	return rises
}

// resetsOf returns the resets of windows, in the order they came, those at
// one time by their windows' keys: empty, never nil, where there were none,
// as a summary tells none from never read.
func resetsOf(windows map[string]*windowDay) []Reset {
	resets := []Reset{}
	for _, d := range windows {
		resets = append(resets, d.climb.resets...)
	}
	slices.SortFunc(resets, byWhen)
	return resets
}

// byWhen orders resets by when they came, those at one time by their
// windows' keys.
func byWhen(a, b Reset) int {
	return cmp.Or(a.At.Compare(b.At), strings.Compare(a.Window, b.Window))
}

// hold is how far a reading of a window holds its account back.
type hold int

const (
	// open holds it back at nothing.
	open hold = iota
	// capped is the window at or past the account's cap, short of its limit,
	// as score.HeldByReserve says.
	capped
	// limited is the window refused, or used up, as score.Spent says.
	limited
)

// holdOf returns how far w, a reading of a window of an account whose
// reserve is the one given, holds the account back.
func holdOf(w quota.Window, reserve float64) hold {
	switch {
	case score.Spent(w):
		return limited
	case score.HeldByReserve(w, reserve):
		return capped
	}
	return open
}

// edge is where a reading of a window comes to hold its account back, or
// stops: n is 1 at the one, -1 at the other.
type edge struct {
	at   time.Time
	hold hold
	n    int
}

// minutesAt returns the minutes of the day windows, an account's, held it
// back at its cap, and at a limit, its reserve the one given: those of its
// windows every model shares alone, as shared names them, each from the
// reading that put it there until the next, its reset, or the day's end, as
// windowDay has them stand. A minute one window holds it at its cap, and
// another at a limit, is at a limit. Each is rounded to the nearest minute.
func minutesAt(windows map[string]*windowDay, reserve float64, shared []string) (atCap, atLimit int) {
	var edges []edge
	for key, d := range windows {
		if !slices.Contains(shared, key) {
			continue
		}
		for _, s := range d.stood {
			if h := holdOf(s.window, reserve); h != open {
				edges = append(edges, edge{at: s.from, hold: h, n: 1}, edge{at: s.to, hold: h, n: -1})
			}
		}
	}
	// Edges at one time, in whatever order, have no time between them.
	slices.SortFunc(edges, func(a, b edge) int { return a.at.Compare(b.at) })
	var holding [limited + 1]int
	var held [limited + 1]time.Duration
	for i, e := range edges {
		if i > 0 {
			held[heldMost(holding)] += e.at.Sub(edges[i-1].at)
		}
		holding[e.hold] += e.n
	}
	return wholeMinutes(held[capped]), wholeMinutes(held[limited])
}

// heldMost returns the most any window holds an account back, holding
// counting the windows that hold it back as far as each hold.
func heldMost(holding [limited + 1]int) hold {
	switch {
	case holding[limited] > 0:
		return limited
	case holding[capped] > 0:
		return capped
	}
	return open
}

// wholeMinutes returns d in minutes, rounded to the nearest.
func wholeMinutes(d time.Duration) int {
	return int(d.Round(time.Minute) / time.Minute)
}
