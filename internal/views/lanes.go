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
type judge func(m status.Moment, a status.AccountNow, read []readings.Reading, told []status.Span, t time.Time) status.Was

// LanesFrom are what Runway's lanes are built from at a look: the moment;
// the readings history's readings, oldest first, those the past is judged
// by, each window's last before the timeline carried in; the summaries of
// the days the timeline shows, oldest first, today's as it's read, and any
// before them carried in; Kept, when the first day the readings history
// holds starts, a day before it drawn from its summary, as the readings of
// it are gone, zero where it holds every day; the timeline, its Now the
// moment's; and the window the view is named for.
type LanesFrom struct {
	Moment   status.Moment
	Readings []readings.Reading
	Days     []ledger.Summary
	Kept     time.Time
	Timeline Timeline
	Window   RunwayWindow
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
// its column; when; and Back, set where a stretch without room ends at it.
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
	// Back is when room returns, zero where that isn't known, and AsItResets
	// is set where that's as the view's window resets.
	Back       time.Time
	AsItResets bool
	// Column is where its words start: the column its start falls in, the
	// first, of a start before the timeline, and First, of one not known.
	Column, First, Last int
	// Over is set where it's over, as the timeline's over has it, so not
	// said.
	Over bool
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
// the rest as the moment has it stand and heading.
func (from LanesFrom) laneOf(a status.AccountNow, past pastOf, then judge) Lane {
	tl := from.Timeline
	h := aheadOf(a, from.Window)
	if h.hold.since.IsZero() {
		h.hold.since = from.readSince(a, past, then, h.hold.cell)
	}
	stood := make([]stood, max(tl.Columns, 0))
	for c := range stood {
		if c < len(past.was) {
			stood[c] = stoodThen(past.was[c])
		} else {
			stood[c] = h.at(tl.Moment(c))
		}
	}
	lane := Lane{ID: a.ID, Cells: make([]Cell, len(stood)), Stretches: laneStretches(stood, tl, h.hold), Use: from.useOf(a)}
	for c, s := range stood {
		lane.Cells[c] = s.cell
	}
	resets := from.resetsKnown(a, past)
	for i, s := range lane.Stretches {
		lane.Stretches[i].AsItResets = !s.Back.IsZero() && slices.ContainsFunc(resets, s.Back.Equal)
	}
	lane.Resets = from.marked(a, lane.Stretches)
	lane.Room = roomThrough(lane.Cells[from.saysFrom():])
	return lane
}

// readSince is when the hold at now holding the account a back, as the cell
// given, began, as then finds its readings, as its past holds them, show it,
// where it judges it held alike at now; zero where nothing holds it at now,
// or the timeline has no now.
func (from LanesFrom) readSince(a status.AccountNow, past pastOf, then judge, held Cell) time.Time {
	if !held.held() || from.Timeline.NowColumn() < 0 {
		return time.Time{}
	}
	if was := then(from.Moment, a, past.read, past.told, from.Timeline.Now); wasCells[was.Kind] == held {
		return was.Since
	}
	return time.Time{}
}

// laneStretches are the stretches without room the columns, standing as stood,
// draw, in order: each run of columns held back alike, the hold of one
// holding on, as its until says, at the next's moment; its start that of its
// first column, but where it holds now's, where the hold at now, given, is
// of its kind, that hold's, where that's known.
func laneStretches(stood []stood, tl Timeline, hold stood) []LaneStretch {
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
		if last := &stretches[len(stretches)-1]; c == now && hold.cell == s.cell && !hold.since.IsZero() {
			last.Start, last.Projected = hold.since, false
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
