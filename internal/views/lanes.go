package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// Lanes are Runway's lanes at a look, facts the dashboard words and colours:
// a lane an account, in the moment's order; the pool's lane under them; and
// what they draw, as the legend keys it. Each look's are built once, by
// Look, from the last look's, which holds what it judged of the timeline's
// past until what that's judged from changes.
type Lanes struct {
	Accounts []Lane
	Pool     Pool
	Drawn    Drawn

	// then judges how an account stood at a moment gone, as
	// status.Moment.Then does where it's nil; and past is what was judged of
	// the timeline's past, held.
	then judge
	past heldPast
}

// judge is how an account stood at a moment gone, as status.Moment.Then has
// it.
type judge func(m status.Moment, a status.AccountNow, read []readings.Reading, reach status.Reach, told []status.Span, t time.Time) status.Was

// LanesFrom are what Runway's lanes are built from at a look: the moment;
// the readings history's readings, oldest first, those the past is judged
// by, each window's run before the timeline carried in, as
// readings.CarriedRuns has them, so a hold begun before it is said to begin
// as it did; the summaries of the days the timeline shows, oldest first,
// today's as it's read, and any before them carried in; Kept, when the first
// day the readings history holds starts, a day before it drawn from its
// summary, as the readings of it are gone, zero where it holds every day;
// Seen, when the readings the watch read itself, among the readings, begin,
// zero where there are none; the timeline, its Now the moment's; and the
// window the view is named for.
type LanesFrom struct {
	Moment   status.Moment
	Readings []readings.Reading
	Days     []ledger.Summary
	Kept     time.Time
	Seen     time.Time
	Timeline Timeline
	Window   RunwayWindow
}

// reach is how far back the readings reach, as Then takes it: to the
// readings history's keep, and the first the watch read itself.
func (from LanesFrom) reach() status.Reach {
	return status.Reach{Kept: from.Kept, Seen: from.Seen}
}

// Lane is an account's lane: its id; how it stands in each column of the
// timeline; where the view's window resets in it, as │ marks it, in order;
// each stretch it has no room through, in order; whether it has room through
// every column from now's, or through every column of a day or week stepped
// back to, as nothing else is said of it; and its use of the view's window.
type Lane struct {
	ID        string
	Cells     []Cell
	Resets    []Reset
	Stretches []LaneStretch
	Room      bool
	Use       Use
}

// Cell is how an account stands in a column of its lane, at the column's
// moment, as the lane draws it.
type Cell int

// The cells, in the order the legend keys them.
const (
	// CellUnknown is an account nothing is known of then: nothing read of
	// it by then, or nothing read of it yet. Its lane is blank there.
	CellUnknown Cell = iota
	// CellRoom is an account with room.
	CellRoom
	// CellRunsOut is an account with room that runs out at its pace before
	// its window resets: from now's column on alone.
	CellRunsOut
	// CellCapped is an account its cap holds back: read at it, or, ahead,
	// as a run-out that reaches it first projects it.
	CellCapped
	// CellLimited is an account a limit holds back from every request: read
	// spent, told of by the router, or, ahead, as a run-out projects it.
	CellLimited
	// CellRefused is an account whose token the upstream refuses.
	CellRefused
	// CellTokenless is an account without a usable token, with no room all
	// along.
	CellTokenless
)

// HasRoom reports whether an account standing so has room, as the pool
// counts it: one running out at its pace has room until it does.
func (c Cell) HasRoom() bool {
	return c == CellRoom || c == CellRunsOut
}

// held reports whether an account standing so is held back, as a stretch
// without room is.
func (c Cell) held() bool {
	return c >= CellCapped
}

// Reset is a reset of the view's window in an account's lane, as │ marks it:
// its column; when; and Back, set where a stretch without room ends at it, as
// its own hold lifts then, though another may hold the account on.
type Reset struct {
	Column int
	At     time.Time
	Back   bool
}

// LaneStretch is a stretch an account has no room through, over the columns from
// First to Last, held back alike: by what holds it at each column's moment,
// until it lifts, as how the account stood there says; the moment's hold, in
// now's column.
type LaneStretch struct {
	// Cell is how its columns stand: CellCapped, CellLimited, CellRefused or
	// CellTokenless.
	Cell Cell
	// Start is when it began, or begins: as the router's event of it tells,
	// else the first reading of those read that shows it, else zero, as of a
	// hold from before the timeline, or with no start, as no token has.
	// Projected is set where a run-out projects it, as ~ says.
	Start     time.Time
	Projected bool
	// Back is when room returns, zero where that isn't known: past its own
	// hold's lift where another holds the account on, as backWithRoom has
	// it. Lifts is when its own hold lifts where the stretch after it holds
	// the account on, as heldOnBy has it, so the stretch is one of a run,
	// back as the run's last is: zero where it's back as it lifts.
	// AsItResets is set where the time it's said to end, as Ends has it, is
	// as the view's window resets.
	Back, Lifts time.Time
	AsItResets  bool
	// Column is where its words start: the column its start falls in, the
	// first, of a start before the timeline, and First, of one not known.
	Column, First, Last int
	// Over is set where it's over, as the timeline's over has it, so not
	// said.
	Over bool
}

// Ends is when the stretch is said to end, and whether that's as its own
// hold lifts, of one of a run but the last, as Lifts has it, rather than as
// room returns, as Back has it: zero where that isn't known.
func (s LaneStretch) Ends() (at time.Time, lifts bool) {
	if !s.Lifts.IsZero() {
		return s.Lifts, true
	}
	return s.Back, false
}

// Use is an account's use of the view's window, as the label under its name
// says it.
type Use struct {
	// Key is the window's key: "" where the account has none.
	Key string
	// Used is the share of it used, where Read is set: as it stands at the
	// moment, live; stepped back, the highest the days shown read, as
	// Highest says. Read isn't set where it's not read: without a usable
	// token, nothing read of it yet, or nothing read of it the days shown.
	Used          float64
	Read, Highest bool
	// Tokenless and Unread say why a live one isn't read: no usable token,
	// or nothing read of it yet. Stale is set where its token is refused or
	// its last read failed, as the moment has its window stale, ReadAt when
	// it was last read.
	Tokenless, Unread, Stale bool
	ReadAt                   time.Time
}

// Look is Runway's lanes at a look, built from what's given: each column
// before now's, or of a day or week gone, as Then, or l's judge, judges the
// account stood at its moment, held from l where what that's judged from
// hasn't changed; now's column and those after it as the moment has the
// account stand, and heading.
func (l Lanes) Look(from LanesFrom) Lanes {
	past := l.past
	if !past.holds(from) {
		past = l.judgePast(from)
	}
	look := Lanes{then: l.then, past: past, Accounts: make([]Lane, len(from.Moment.Accounts))}
	for i, a := range from.Moment.Accounts {
		look.Accounts[i] = from.laneOf(a, past.accounts[i], l.judging())
	}
	look.Pool = from.poolOf(look.Accounts)
	look.Drawn = drawnOf(look.Accounts)
	return look
}

// judging is how l judges how an account stood at a moment gone: its judge,
// else status.Moment.Then.
func (l Lanes) judging() judge {
	if l.then == nil {
		return status.Moment.Then
	}
	return l.then
}

// laneOf is the account a's lane, as Lane has it, its past as past holds it,
// the rest as the moment has it stand and heading. Its resets are marked
// where a stretch's own hold lifts, before each stretch is back as its
// account next has room.
func (from LanesFrom) laneOf(a status.AccountNow, past pastOf, then judge) Lane {
	tl := from.Timeline
	h := from.sinceRead(aheadOf(a, from.Window), a, past, then)
	stood := make([]stood, max(tl.Columns, 0))
	for c := range stood {
		if c < len(past.was) {
			stood[c] = stoodThen(past.was[c])
		} else {
			stood[c] = h.at(tl.Moment(c))
		}
	}
	lane := Lane{ID: a.ID, Cells: make([]Cell, len(stood)), Stretches: laneStretches(stood, tl, h.holds), Use: from.useOf(a)}
	for c, s := range stood {
		lane.Cells[c] = s.cell
	}
	lane.Resets = from.marked(a, lane.Stretches)
	lane.Stretches = backWithRoom(lane.Stretches, h, tl.NowColumn())
	resets := from.resetsKnown(a, past)
	for i, s := range lane.Stretches {
		ends, _ := s.Ends()
		lane.Stretches[i].AsItResets = !ends.IsZero() && slices.ContainsFunc(resets, ends.Equal)
	}
	lane.Room = roomThrough(lane.Cells[from.saysFrom():])
	return lane
}

// sinceRead is h, each of its holds whose start no event tells begun as then
// finds the account a's readings, as its past holds them, show it, where the
// timeline holds now: as then judges the account stood as the hold is first
// drawn, as drawnFrom has it, where it judges it held alike then. A hold
// under another is so begun as the readings will have it once its columns
// are past.
func (from LanesFrom) sinceRead(h ahead, a status.AccountNow, past pastOf, then judge) ahead {
	if from.Timeline.NowColumn() < 0 {
		return h
	}
	for i, p := range h.holds {
		at, drawn := h.drawnFrom(i, from.Timeline.Now)
		if !p.since.IsZero() || !drawn {
			continue
		}
		if was := then(from.Moment, a, past.read, from.reach(), past.told, at); wasCells[was.Kind] == p.cell {
			h.holds[i].since = was.Since
		}
	}
	return h
}

// laneStretches are the stretches without room the columns, standing as stood,
// draw, in order: each run of columns held back alike, the hold of one
// holding on, as its until says, at the next's moment, back as it lifts; its
// start that of its first column, but where it holds now's, as begunAt has
// it of the holds at now given.
func laneStretches(stood []stood, tl Timeline, holds []stood) []LaneStretch {
	var stretches []LaneStretch
	now := tl.NowColumn()
	for c, s := range stood {
		if !s.cell.held() {
			continue
		}
		if n := len(stretches); n > 0 && stretches[n-1].holdsOn(s, c, tl.Moment(c)) {
			stretches[n-1].Last, stretches[n-1].Back = c, s.until
		} else {
			stretches = append(stretches, LaneStretch{Cell: s.cell, Start: s.since, Projected: s.projected, Back: s.until, First: c, Last: c})
		}
		if c == now {
			stretches[len(stretches)-1].begunAt(holds)
		}
	}
	for i := range stretches {
		s := &stretches[i]
		s.Column, s.Over = s.First, tl.over(s.Last)
		if !s.Start.IsZero() {
			s.Column = max(tl.Column(s.Start), 0)
		}
	}
	return stretches
}

// begunAt has the stretch, which holds now's column, begin as the hold at now
// of its kind, of those given, began, where that's known: as the router's
// event of it tells, before the readings; but where its own first column's
// hold began earlier, as then, as the stretch is one unbroken hold, which a
// window can take over from another the readings alone tell no longer.
func (s *LaneStretch) begunAt(holds []stood) {
	for _, h := range holds {
		if h.cell == s.Cell && !h.since.IsZero() && (s.Start.IsZero() || h.since.Before(s.Start)) {
			s.Start, s.Projected = h.since, false
		}
	}
}

// backWithRoom are the stretches given, in order, each back as its account
// next has room, as ROUTING's back says: one reaching now's column, now, or
// past it, as the account standing from now on as h has it is, from when its
// own hold lifts, whether or not a column draws what holds it on; and one
// before, as the next is, where that holds it on, as heldOnBy says. Each the
// next holds on lifts as its own hold does.
func backWithRoom(stretches []LaneStretch, h ahead, now int) []LaneStretch {
	for i := len(stretches) - 1; i >= 0; i-- {
		s := &stretches[i]
		heldOn := i+1 < len(stretches) && s.heldOnBy(stretches[i+1])
		if heldOn {
			s.Lifts = s.Back
		}
		if now >= 0 && s.Last >= now {
			s.Back = h.roomFrom(s.Back)
		} else if heldOn {
			s.Back = stretches[i+1].Back
		}
	}
	return stretches
}

// heldOnBy reports whether the stretch next holds the stretch's account on
// past its own hold's lift: it's in the column after the stretch's last,
// and began by then.
func (s LaneStretch) heldOnBy(next LaneStretch) bool {
	return next.First == s.Last+1 && !s.Back.IsZero() && !next.Start.After(s.Back)
}

// holdsOn reports whether the stretch, its last column before c, holds on
// into c, where the account stands as st at its moment, at: held back alike,
// the hold that held it still holding then, as its back says.
func (s LaneStretch) holdsOn(st stood, c int, at time.Time) bool {
	return s.Last == c-1 && s.Cell == st.cell && (s.Back.IsZero() || s.Back.After(at))
}

// viewKey is the key of the window the view is named for, of those every
// model shares, in the moment's policy's order: the first lasting a day or
// less of the 5-hour view, the first lasting longer of the week's; "" where
// none does.
func (from LanesFrom) viewKey() string {
	for _, key := range from.Moment.Policy().Shared {
		if quota.MultiDay(key) == (from.Window == RunwayWeek) {
			return key
		}
	}
	return ""
}

// saysFrom is the first column a lane's words are said of: now's, live, or
// the first, of a day or week stepped back to.
func (from LanesFrom) saysFrom() int {
	if _, _, named := from.Timeline.Named(); named {
		return 0
	}
	return max(from.Timeline.NowColumn(), 0)
}

// roomThrough reports whether there's room in every one of the cells given.
func roomThrough(cells []Cell) bool {
	return !slices.ContainsFunc(cells, func(c Cell) bool { return !c.HasRoom() })
}
