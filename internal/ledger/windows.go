package ledger

import (
	"cmp"
	"encoding/json"
	"fmt"
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

func (d *windowDay) read() bool {
	return d.climb.read
}

// take takes w, a reading of the window read at at, in the day, as its climb
// takes it: the reading it stood at before stands until then, or its reset.
// Nor does one read from until on say anything of the window. It reports
// false of a reading of a run that has reset since, read late, as the window
// never stood at it.
func (d *windowDay) take(w quota.Window, at time.Time) bool {
	if !at.Before(d.until) {
		return true
	}
	last, from, read := d.climb.last, d.climb.at, d.climb.read
	_, took := d.climb.take(w, at)
	if took == taken && read {
		d.stand(last, from, at)
	}
	return took != late
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
	// resets are the resets that ended its runs that read above nothing, and
	// ended each such run's reset as its readings gave it.
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

// Clone is a copy of c that takes readings on as c would, without changing
// c, nor c it.
func (c *Climb) Clone() *Climb {
	d := *c
	d.resets, d.ended = slices.Clone(c.resets), slices.Clone(c.ended)
	return &d
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
// between the reading before and w, as score.ResetBetween tells it, ending
// that run, the next beginning at nothing. It reports false, taking nothing
// toward the climb, of a reading that says nothing of the run since: one past
// its own reset as it was read, as an answer that came late gives, the window
// having started afresh since; or one whose reset is earlier than the latest
// read, a reading of an earlier run read late, as readLate takes it.
func (c *Climb) Take(w quota.Window, at time.Time) (float64, bool) {
	climbed, took := c.take(w, at)
	return climbed, took == taken
}

// taking is what a climb made of a reading: taken toward it; passed over, as
// past its own reset as it was read; or late, of a run that has reset since.
type taking int

const (
	taken taking = iota
	pastReset
	late
)

// take takes w as Take says, saying what it made of it.
func (c *Climb) take(w quota.Window, at time.Time) (float64, taking) {
	switch {
	case w.ResetBy(at):
		return 0, pastReset
	case !w.ResetsAt.IsZero() && w.ResetsAt.Before(c.latest):
		c.readLate(w)
		return 0, late
	}
	if c.read {
		if reset, ok := score.ResetBetween(c.last, w, at); ok {
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
	return climbed, taken
}

// readLate takes w, a reading of a run of the window that has reset since, read
// late, as an answer that began before the reset and ended after it gives
// one: a true reading of that run, which says nothing of the run since.
// Where it reads higher than that run's highest use, the run's reset takes it
// as the use before it.
func (c *Climb) readLate(w quota.Window) {
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

// Resets returns the resets that ended the window's runs, in the order they
// came, as a day's summary files them: those that read above nothing, each
// with the highest use of the run it ended.
func (c *Climb) Resets() []Reset {
	return slices.Clone(c.resets)
}

// reset notes the window's reset at at, the highest use of the run it ended
// before it, and ends that run, the next beginning at nothing. A run that
// never read above nothing, the use carried into it included, notes none:
// nothing was open to reset, as of a window no request opened, its reset
// moving on with each reading.
func (c *Climb) reset(at time.Time) {
	if c.top != 0 {
		c.resets = append(c.resets, Reset{Window: c.key, At: at.UTC(), Before: c.top})
		c.ended = append(c.ended, c.last.ResetsAt)
	}
	c.top = 0
}

// storedClimb is a climb as a stored form holds it, as MarshalJSON writes it
// and UnmarshalJSON reads it back: its window's key; the reading it took
// last, when, and whether it took one; its highest use since its run began,
// and whether that run's start is known; the latest reset read; its rise; and
// the resets that ended its runs, with the reset each such run's readings
// gave. Not how it begins a run whose start isn't known, a function: read
// back, it begins one at its first reading's use, as NewClimb has it given
// none, as a page's points begin every climb they follow. Its keys are short
// and its zeros left out.
type storedClimb struct {
	Key    string        `json:"k,omitempty"`
	Last   storedReading `json:"l,omitzero"`
	At     storedTime    `json:"t,omitzero"`
	Read   bool          `json:"r,omitzero"`
	Top    float64       `json:"tp,omitzero"`
	Based  bool          `json:"b,omitzero"`
	Latest storedTime    `json:"lt,omitzero"`
	Rise   float64       `json:"rs,omitzero"`
	Resets []storedReset `json:"rr,omitempty"`
	Ended  []storedTime  `json:"e,omitempty"`
}

// storedReading is a window's reading as a climb's stored form holds it,
// every field of it.
type storedReading struct {
	Key            string       `json:"k,omitempty"`
	Label          string       `json:"lb,omitempty"`
	Utilization    float64      `json:"u,omitzero"`
	ResetsAt       storedTime   `json:"r,omitzero"`
	Status         quota.Status `json:"s,omitempty"`
	RestartedAt    storedTime   `json:"rs,omitzero"`
	Pace           *float64     `json:"p,omitempty"`
	AllowanceShare float64      `json:"as,omitzero"`
	AllowancePer   string       `json:"ap,omitempty"`
}

// storedReset is a reset as a climb's stored form holds it.
type storedReset struct {
	Window string     `json:"w,omitempty"`
	At     storedTime `json:"t,omitzero"`
	Before float64    `json:"b,omitzero"`
}

// storedTime is a time as a climb's stored form holds it, exactly, whatever
// its year: the seconds since the zero time and the nanoseconds after, so
// the zero time, as a reading whose reset wasn't said gives, is left out.
// It's read back in UTC.
type storedTime struct {
	Sec  int64 `json:"s,omitzero"`
	Nsec int32 `json:"n,omitzero"`
}

// zeroUnix is the zero time, in seconds since the Unix epoch.
var zeroUnix = time.Time{}.Unix()

// MarshalJSON writes c in its stored form. It refuses a climb holding a time
// in another zone than UTC, which its stored form would read back otherwise,
// and one holding a number that isn't finite, which JSON can't carry.
func (c *Climb) MarshalJSON() ([]byte, error) {
	if !c.inUTC() {
		return nil, fmt.Errorf("encode the climb of %s: a time not in UTC", c.key)
	}
	b, err := json.Marshal(c.stored())
	if err != nil {
		return nil, fmt.Errorf("encode the climb of %s: %w", c.key, err)
	}
	return b, nil
}

// UnmarshalJSON reads c back from its stored form, as MarshalJSON wrote it:
// the climb it was, but that a run whose start isn't known begins at its
// first reading's use, as NewClimb has it given none.
func (c *Climb) UnmarshalJSON(b []byte) error {
	var stored storedClimb
	if err := json.Unmarshal(b, &stored); err != nil {
		return fmt.Errorf("decode a climb: %w", err)
	}
	*c = stored.climb()
	return nil
}

// inUTC reports whether every time c holds is in UTC.
func (c *Climb) inUTC() bool {
	times := append([]time.Time{c.at, c.latest, c.last.ResetsAt, c.last.RestartedAt}, c.ended...)
	for _, r := range c.resets {
		times = append(times, r.At)
	}
	return !slices.ContainsFunc(times, func(t time.Time) bool { return t.Location() != time.UTC })
}

// stored is c as its stored form holds it.
func (c *Climb) stored() storedClimb {
	stored := storedClimb{
		Key: c.key, Last: storedReadingOf(c.last), At: storedTimeOf(c.at), Read: c.read, Top: c.top, Based: c.based,
		Latest: storedTimeOf(c.latest), Rise: c.rise,
	}
	for _, r := range c.resets {
		stored.Resets = append(stored.Resets, storedReset{Window: r.Window, At: storedTimeOf(r.At), Before: r.Before})
	}
	for _, t := range c.ended {
		stored.Ended = append(stored.Ended, storedTimeOf(t))
	}
	return stored
}

// climb is the climb s stores.
func (s *storedClimb) climb() Climb {
	c := NewClimb(s.Key, nil)
	c.last, c.at, c.read, c.top, c.based = s.Last.reading(), s.At.time(), s.Read, s.Top, s.Based
	c.latest, c.rise = s.Latest.time(), s.Rise
	for _, r := range s.Resets {
		c.resets = append(c.resets, Reset{Window: r.Window, At: r.At.time(), Before: r.Before})
	}
	for _, t := range s.Ended {
		c.ended = append(c.ended, t.time())
	}
	return *c
}

// storedReadingOf is w as a climb's stored form holds it.
func storedReadingOf(w quota.Window) storedReading {
	return storedReading{
		Key: w.Key, Label: w.Label, Utilization: w.Utilization, ResetsAt: storedTimeOf(w.ResetsAt), Status: w.Status,
		RestartedAt: storedTimeOf(w.RestartedAt), Pace: w.Pace, AllowanceShare: w.Allowance.Share, AllowancePer: w.Allowance.Per,
	}
}

// reading is the reading s stores.
func (s storedReading) reading() quota.Window {
	return quota.Window{
		Key: s.Key, Label: s.Label, Utilization: s.Utilization, ResetsAt: s.ResetsAt.time(), Status: s.Status,
		RestartedAt: s.RestartedAt.time(), Pace: s.Pace, Allowance: quota.Allowance{Share: s.AllowanceShare, Per: s.AllowancePer},
	}
}

// storedTimeOf is t as a climb's stored form holds it.
func storedTimeOf(t time.Time) storedTime {
	return storedTime{Sec: t.Unix() - zeroUnix, Nsec: int32(t.Nanosecond())}
}

// time is the time s stores, in UTC.
func (s storedTime) time() time.Time {
	return time.Unix(s.Sec+zeroUnix, int64(s.Nsec)).UTC()
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

// edge is where a reading of a window comes to hold its account back, or
// stops: n is 1 at the one, -1 at the other.
type edge struct {
	at   time.Time
	hold score.Hold
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
			if h := score.HoldOf(s.window, reserve); h != score.NotHeld {
				edges = append(edges, edge{at: s.from, hold: h, n: 1}, edge{at: s.to, hold: h, n: -1})
			}
		}
	}
	// Edges at one time, in whatever order, have no time between them.
	slices.SortFunc(edges, func(a, b edge) int { return a.at.Compare(b.at) })
	var holding [score.HeldAtLimit + 1]int
	var held [score.HeldAtLimit + 1]time.Duration
	for i, e := range edges {
		if i > 0 {
			held[heldMost(holding)] += e.at.Sub(edges[i-1].at)
		}
		holding[e.hold] += e.n
	}
	return wholeMinutes(held[score.HeldAtCap]), wholeMinutes(held[score.HeldAtLimit])
}

// heldMost returns the most any window holds an account back, holding
// counting the windows that hold it back as far as each hold.
func heldMost(holding [score.HeldAtLimit + 1]int) score.Hold {
	switch {
	case holding[score.HeldAtLimit] > 0:
		return score.HeldAtLimit
	case holding[score.HeldAtCap] > 0:
		return score.HeldAtCap
	}
	return score.NotHeld
}

// wholeMinutes returns d in minutes, rounded to the nearest.
func wholeMinutes(d time.Duration) int {
	return int(d.Round(time.Minute) / time.Minute)
}
