package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// poolShown is Runway's live 5-hour view at routedAt, at 160 columns, its
// lanes those of doc's accounts, its pool's lane as given.
func poolShown(doc status.Document, pool views.Pool) RunwayShown {
	lanes := &views.Lanes{Pool: pool}
	for _, a := range doc.Accounts {
		lanes.Accounts = append(lanes.Accounts, views.Lane{ID: a.ID})
	}
	return RunwayShown{Timeline: views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayColumns(160)), Lanes: lanes}
}

// poolDrawn is the canvas of Runway shown as r of doc at routedAt, 160 by
// 37, in the look given, alone, and the row its pool's lane starts on.
func poolDrawn(r RunwayShown, doc status.Document, look Look) (*canvas, int) {
	f := Frame{Width: 160, Height: 37, Look: look, View: Runway, Policy: claudeLike, Runway: r}
	return runwayDrawn(f, doc), f.runwayFitted(f.top()).pool
}

// framesDoc is the router's document of the auto world the frames were drawn
// of at routedAt, as routedWorld has it, work's reserve 5%: work at its cap
// till 17:10, its 5-hour at 95%, its week at 34%; personal's at 39% and
// 66%; and side's, where new sessions go, at 13% and 78%.
func framesDoc() status.Document {
	doc, _ := routedWorld(false)
	doc.Accounts[0].Reserve = 0.05
	return doc.WorkedOut(claudeLike)
}

// stepAt is a step of a dip, of the count with room given, from and until
// the times given, over the columns given: zero where a time's unknown.
func stepAt(with int, from, until time.Time, first, last int) views.Step {
	return views.Step{With: with, From: views.Edge{At: from}, Until: views.Edge{At: until}, First: first, Last: last}
}

// The pool's columns are each as high as the share of the pool's accounts
// with room, of its two rows, in eighths of a cell, rounded half to even:
// full cells a background, the top cell part-filled, as GRAPHS fills its
// columns; coloured as usage colours the pool's use, the share without
// room, its ramp's first stop under 70%, its second from 70%, its last from
// 90%; and blank where none of its accounts' states is known, or none has
// room.
func TestThePoolsColumnsAreAsHighAsTheShareOfItsAccountsWithRoom(t *testing.T) {
	full := func(tok theme.Token) seenCell { return seenCell{" ", ink{on: hue{token: tok}}} }
	part := func(glyph string, tok theme.Token) seenCell { return seenCell{glyph, ink{token: tok}} }
	tests := []struct {
		name        string
		col         views.PoolColumn
		of          int
		top, bottom seenCell
	}{
		{name: "all three", col: views.PoolColumn{With: 3, Known: true}, of: 3, top: full(theme.VizRamp1), bottom: full(theme.VizRamp1)},
		{name: "two of three", col: views.PoolColumn{With: 2, Known: true}, of: 3, top: part("▃", theme.VizRamp1), bottom: full(theme.VizRamp1)},
		{name: "one of three, two thirds without", col: views.PoolColumn{With: 1, Known: true}, of: 3, top: blankCell, bottom: part("▅", theme.VizRamp1)},
		{name: "nine of ten", col: views.PoolColumn{With: 9, Known: true}, of: 10, top: part("▆", theme.VizRamp1), bottom: full(theme.VizRamp1)},
		{name: "eight of ten", col: views.PoolColumn{With: 8, Known: true}, of: 10, top: part("▅", theme.VizRamp1), bottom: full(theme.VizRamp1)},
		{name: "three of ten, 70% without", col: views.PoolColumn{With: 3, Known: true}, of: 10, top: blankCell, bottom: part("▅", theme.VizRamp2)},
		{name: "one of ten, 90% without", col: views.PoolColumn{With: 1, Known: true}, of: 10, top: blankCell, bottom: part("▂", theme.VizRamp4)},
		{name: "none of three", col: views.PoolColumn{Known: true}, of: 3, top: blankCell, bottom: blankCell},
		{name: "none known", col: views.PoolColumn{}, of: 3, top: blankCell, bottom: blankCell},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := slices.Repeat([]views.PoolColumn{tt.col}, RunwayColumns(160))
			c, top := poolDrawn(poolShown(runwayDoc(tt.of), views.Pool{Of: tt.of, Key: "5h", Columns: columns}), runwayDoc(tt.of), Screen(builtin(t, "nord")))
			const x = 100
			if got := cellAt(c, x, top); got != tt.top {
				t.Errorf("the column's top cell is %+v, want %+v", got, tt.top)
			}
			if got := cellAt(c, x, top+1); got != tt.bottom {
				t.Errorf("the column's bottom cell is %+v, want %+v", got, tt.bottom)
			}
		})
	}
}

// seenCell is a canvas cell's glyph and ink, its mark aside.
type seenCell struct {
	glyph string
	ink   ink
}

// blankCell is a cell with nothing drawn in it.
var blankCell = seenCell{" ", ink{}}

// cellAt is the canvas's cell at x along row y, as seenCell has it.
func cellAt(c *canvas, x, y int) seenCell {
	here := c.at(x, y)
	return seenCell{here.glyph, here.ink}
}

// The pool's columns before now's are dimmed, mix(canvas, colour, 0.45), as
// the past is: now's and those after it aren't, and on a day stepped back
// to, which has no now, none is.
func TestThePoolsPastIsDimmed(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{With: 2, Known: true}}, RunwayColumns(160))
	live := poolShown(doc, views.Pool{Of: 3, Key: "5h", Columns: columns})
	gone := live
	gone.Back = civilDay(9, 30)
	gone.Timeline = gone.TimelineAt(routedAt, time.Monday, 160)
	now := live.Timeline.NowColumn()
	tests := []struct {
		name   string
		shown  RunwayShown
		column int
		fade   float64
	}{
		{name: "the column before now's", shown: live, column: now - 1, fade: pastFade},
		{name: "the first", shown: live, column: 0, fade: pastFade},
		{name: "now's", shown: live, column: now},
		{name: "after now's", shown: live, column: now + 1},
		{name: "a day stepped back to's first", shown: gone, column: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, top := poolDrawn(tt.shown, doc, Screen(builtin(t, "nord")))
			x := runwayLanesFrom + tt.column
			if got := c.at(x, top).ink.fade; got != tt.fade {
				t.Errorf("column %d's top cell fades %v, want %v", tt.column, got, tt.fade)
			}
			if got := c.at(x, top+1).ink.on.fade; got != tt.fade {
				t.Errorf("column %d's full cell fades %v, want %v", tt.column, got, tt.fade)
			}
		})
	}
	if got, want := mixed(t, pastFade, theme.VizRamp1), "#637262"; got != want {
		t.Errorf("the past's green is %s, want %s, as the frames dim it", got, want)
	}
}

// mixed is the colour of the token given faded so far into nord's canvas.
func mixed(t *testing.T, fade float64, tok theme.Token) string {
	t.Helper()
	nord := Screen(builtin(t, "nord"))
	return shadeOf(nord.colour(ink{token: tok, fade: fade})).String()
}

// Where the look blends nothing, as NO_COLOR's, a full cell of the pool's
// columns is █, as GRAPHS draws one.
func TestThePoolsFullCellsAreBlocksWhereTheLookBlendsNothing(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{With: 2, Known: true}}, RunwayColumns(160))
	c, top := poolDrawn(poolShown(doc, views.Pool{Of: 3, Key: "5h", Columns: columns}), doc, NoColour())
	if got := string([]rune(rowOf(c, top+1))[runwayLanesFrom:]); got != strings.Repeat("█", RunwayColumns(160)) {
		t.Errorf("the pool's bottom row reads %q, want █ in every column", got)
	}
	if got := string([]rune(rowOf(c, top))[runwayLanesFrom:]); got != strings.Repeat("▃", RunwayColumns(160)) {
		t.Errorf("the pool's top row reads %q, want its part-filled cells", got)
	}
}

// The pool's rule runs across the whole row, in border; Σ pool sits on its
// lane's second row, Σ in text.subtle in the accounts' numbers' column, the
// ones', and pool in text.secondary.
func TestThePoolsRuleAndSigmaPool(t *testing.T) {
	tests := []struct {
		name     string
		accounts int
		rule     int
		sigma    string
	}{
		{name: "three, the rule on the last lane's blank row", accounts: 3, rule: 18, sigma: "Σ pool"},
		{name: "ten, the rule on a row of its own, numbered from ten", accounts: 10, rule: 29, sigma: " Σ pool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := runwayDoc(tt.accounts)
			c, _ := poolDrawn(poolShown(doc, views.Pool{Of: tt.accounts, Key: "5h"}), doc, Screen(builtin(t, "nord")))
			if got := rowOf(c, tt.rule); got != strings.Repeat("─", 160) {
				t.Errorf("row %d reads %q, want the rule across it", tt.rule, got)
			}
			if got := c.at(159, tt.rule).ink; got != borderInk {
				t.Errorf("the rule is in %+v, want border", got)
			}
			if got := rowOf(c, tt.rule+2); got != tt.sigma {
				t.Errorf("the pool's second row reads %q, want %q", got, tt.sigma)
			}
			sigma := strings.Index(tt.sigma, "Σ")
			if k, pool := c.at(sigma, tt.rule+2).ink, c.at(sigma+2, tt.rule+2).ink; k != dimInk || pool != noteInk {
				t.Errorf("Σ is in %+v and pool in %+v, want text.subtle and text.secondary", k, pool)
			}
		})
	}
}

// With one account, there's no pool's lane: no rule, no columns, no Σ pool
// and no room, as they'd only repeat the account.
func TestOneAccountHasNoPoolsLane(t *testing.T) {
	doc := runwayDoc(1)
	c, _ := poolDrawn(poolShown(doc, views.Pool{}), doc, Screen(builtin(t, "nord")))
	for y, row := range textOf(c.rows(Look{})) {
		if strings.HasPrefix(row, "─") || strings.Contains(row, "Σ") || strings.Contains(row, " left") {
			t.Errorf("row %d reads %q, want nothing of a pool", y, row)
		}
	}
}

// Lanes of a pool of three drawn for a moment of no accounts draw no pool's
// lane, as they draw no account's: there's no account to name it under.
func TestThePoolsLaneIsntDrawnForAMomentOfNoAccounts(t *testing.T) {
	c := runwayDrawn(runwayFrame(t, 37, lanesOf(3), nil), status.Document{})
	for y, row := range textOf(c.rows(Look{})) {
		if strings.Contains(row, "Σ") || strings.Contains(row, " with room") {
			t.Errorf("row %d reads %q, want nothing of a pool", y, row)
		}
	}
}

// The pool's room, from column 2, is usage's words for the pool in the
// view's window: the window's name in text.subtle, the room bold
// text.secondary, brightening as it changes, and " of 3 left" in
// text.muted, " · pinned" after where a pin holds; drawn while the timeline
// holds now, live and on today stepped back to, never on a day gone.
func TestThePoolsRoomIsUsagesWordsForThePoolInTheViewsWindow(t *testing.T) {
	doc, pinned := framesDoc(), framesDoc()
	pinned.Pin = status.Pin{Accounts: []string{"personal", "side"}, Since: routedClock(1, 13, 40)}
	pinned = pinned.WorkedOut(claudeLike)
	live := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
	week := live
	week.Window, week.Lanes = views.RunwayWeek, &views.Lanes{Accounts: live.Lanes.Accounts, Pool: views.Pool{Of: 3, Key: "7d"}}
	week.Timeline = week.TimelineAt(routedAt, time.Monday, 160)
	today := live
	today.Back = civilDay(10, 1)
	today.Timeline = today.TimelineAt(routedAt, time.Monday, 160)
	gone := live
	gone.Back = civilDay(9, 30)
	gone.Timeline = gone.TimelineAt(routedAt, time.Monday, 160)
	tests := []struct {
		name  string
		doc   status.Document
		shown RunwayShown
		want  string
	}{
		{name: "the 5-hour", doc: doc, shown: live, want: "  5-hour 1.5 of 3 left"},
		{name: "the week", doc: doc, shown: week, want: "  week 1.2 of 3 left"},
		{name: "pinned to two", doc: pinned, shown: live, want: "  5-hour 1.5 of 2 left · pinned"},
		{name: "today stepped back to", doc: doc, shown: today, want: "  5-hour 1.5 of 3 left"},
		{name: "a day stepped back to", doc: doc, shown: gone, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 160, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			c := runwayDrawn(f, tt.doc)
			if got := rowOf(c, 21); got != tt.want {
				t.Errorf("the pool's words' row reads %q, want %q", got, tt.want)
			}
			if tt.want == "" {
				return
			}
			name, room, rest := c.at(2, 21).ink, c.at(strings.Index(tt.want, "1."), 21), c.at(strings.Index(tt.want, " of")+1, 21).ink
			if name != dimInk || room.ink != strongInk || rest != mutedInk {
				t.Errorf("the window's name is in %+v, the room in %+v and the rest in %+v; want text.subtle, bold text.secondary and text.muted", name, room.ink, rest)
			}
			key := tt.shown.Lanes.Pool.Key
			if got, want := f.Numbers(tt.doc, routedAt)[Number{Kind: NumberRoom, Of: key}], strings.Fields(tt.want)[1]; got != want {
				t.Errorf("the room brightens as %q, want %q", got, want)
			}
		})
	}
}

// The pool's words share its room's row, from their dip's column, two cells
// after the room at the least.
func TestThePoolsWordsStartTwoCellsAfterItsRoom(t *testing.T) {
	pinned := framesDoc()
	pinned.Pin = status.Pin{Accounts: []string{"personal", "side"}, Since: routedClock(1, 13, 40)}
	pinned = pinned.WorkedOut(claudeLike)
	tests := []struct {
		name string
		doc  status.Document
		of   int
		want string
	}{
		{name: "from its dip's column, clear of the room", doc: framesDoc(), of: 3, want: "  5-hour 1.5 of 3 left   2 of 3 with room till 15:30"},
		{name: "two cells after the room, where the dip's column is under it", doc: pinned, of: 2, want: "  5-hour 1.5 of 2 left · pinned  1 of 2 with room till 15:30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dip := views.Dip{Steps: []views.Step{stepAt(tt.of-1, routedClock(1, 13, 52), routedClock(1, 15, 30), 2, 10)}, Column: 1, First: 2, Last: 10}
			c, top := poolDrawn(poolShown(tt.doc, views.Pool{Of: tt.of, Key: "5h", Dips: []views.Dip{dip}}), tt.doc, Screen(builtin(t, "nord")))
			if got := rowOf(c, top+runwayPoolRows); got != tt.want {
				t.Errorf("the pool's words' row reads %q, want %q", got, tt.want)
			}
		})
	}
}

// phraseRead is a phrase of the pool's words as a test reads it: its column
// and its text.
type phraseRead struct {
	x    int
	text string
}

// phrasesRead are the phrases placed, as a test reads them.
func phrasesRead(placed []placed) []phraseRead {
	var said []phraseRead
	for _, p := range placed {
		said = append(said, phraseRead{p.x, p.said.plain()})
	}
	return said
}

// The pool's words say each dip, from its column, the column its start
// falls in, two cells after the room or the phrase before at the least: under
// way, its count at now till it changes, each change after chained on; ahead,
// from its projected start to its end; on a day stepped back to, from start
// to end, a dip too close for a phrase of its own joining the one before
// with "and" where its count is the same, else its own phrase two cells
// after; a back past the day's midnight with its weekday, the week's times
// with theirs. A dip over isn't said live, but is on today stepped back to.
// Words that don't fit drop whole parts from the end, and a dip whose first
// doesn't fit isn't said, nor any after it; a phrase ends at a step whose
// end isn't known.
func TestThePoolsWordsSayEachDip(t *testing.T) {
	live := RunwayShown{Timeline: views.LiveTimeline(views.RunwayFiveHour, routedAt, 136)}
	week := RunwayShown{Window: views.RunwayWeek, Timeline: views.LiveTimeline(views.RunwayWeek, routedAt, 136)}
	wednesday := RunwayShown{Back: civilDay(9, 30)}
	wednesday.Timeline = wednesday.TimelineAt(routedAt, time.Monday, 160)
	today := RunwayShown{Back: civilDay(10, 1)}
	today.Timeline = today.TimelineAt(routedAt, time.Monday, 160)
	wed := func(hour, minute int) time.Time { return routedClock(1, hour, minute).AddDate(0, 0, -1) }
	capped := routedAt.Add(-11*time.Minute - 4*time.Second)
	aheadOf := func(from, until time.Time, first, last int) views.Dip {
		steps := []views.Step{{With: 2, From: views.Edge{At: from, Projected: true}, Until: views.Edge{At: until}, First: first, Last: last}}
		return views.Dip{Steps: steps, Column: first, First: first, Last: last, Ahead: true}
	}
	ahead := aheadOf(routedClock(1, 16, 5), routedClock(1, 16, 55), 14, 18)
	underWay := views.Dip{Steps: []views.Step{stepAt(2, capped, routedClock(1, 17, 10), 5, 20)}, Column: 5, First: 5, Last: 20}
	soon, later := aheadOf(routedClock(1, 18, 5), routedClock(1, 18, 55), 26, 30), aheadOf(routedClock(1, 20, 25), routedClock(1, 21, 0), 40, 43)
	over := views.Dip{Steps: []views.Step{stepAt(2, routedClock(1, 13, 50), routedClock(1, 14, 10), 1, 2)}, Column: 1, First: 1, Last: 2, Over: true}
	limitWed := views.Dip{Steps: []views.Step{stepAt(2, wed(15, 2), wed(17, 10), 85, 96)}, Column: 85, First: 85, Last: 96}
	tests := []struct {
		name  string
		shown RunwayShown
		pool  views.Pool
		from  int
		edge  int
		want  []phraseRead
	}{
		{
			name: "under way, its counts chained, from its start's column", shown: live, pool: tenPool(), from: 24, edge: 160,
			want: []phraseRead{{25, "8 of 10 with room till 15:30, 9 till ~16:05, 8 till 16:55, 9 till 17:10"}},
		},
		{
			name: "under way, two cells after the room", shown: live, pool: views.Pool{Of: 3, Dips: []views.Dip{underWay}}, from: 31, edge: 160,
			want: []phraseRead{{31, "2 of 3 with room till 17:10"}},
		},
		{
			name: "under way, then a dip ahead too close, of the same count, joined", shown: live, pool: views.Pool{Of: 3, Dips: []views.Dip{underWay, soon}}, from: 24, edge: 160,
			want: []phraseRead{{29, "2 of 3 with room till 17:10 and ~18:05 – 18:55"}},
		},
		{
			name: "under way, then a dip ahead far enough for a phrase of its own", shown: live, pool: views.Pool{Of: 3, Dips: []views.Dip{underWay, later}}, from: 24, edge: 160,
			want: []phraseRead{{29, "2 of 3 with room till 17:10"}, {64, "2 of 3 with room ~20:25 – 21:00"}},
		},
		{
			name: "ahead, from its projected start", shown: live, pool: views.Pool{Of: 3, Dips: []views.Dip{ahead}}, from: 24, edge: 160,
			want: []phraseRead{{38, "2 of 3 with room ~16:05 – 16:55"}},
		},
		{
			name: "over, live, unsaid", shown: live, pool: views.Pool{Of: 3, Dips: []views.Dip{over, underWay}}, from: 24, edge: 160,
			want: []phraseRead{{29, "2 of 3 with room till 17:10"}},
		},
		{
			name:  "over, on today stepped back to, said; the dip under way too close, of the same count, two cells after, as one under way joins none",
			shown: today, pool: views.Pool{Of: 3, Dips: []views.Dip{{Steps: []views.Step{stepAt(2, routedClock(1, 9, 0), routedClock(1, 10, 0), 51, 56)}, Column: 51, First: 51, Last: 56, Over: true}, {Steps: []views.Step{stepAt(2, capped, routedClock(1, 17, 10), 82, 97)}, Column: 82, First: 82, Last: 97}}},
			from: 24, edge: 160,
			want: []phraseRead{{75, "2 of 3 with room 09:00 – 10:00"}, {107, "2 of 3 with room till 17:10"}},
		},
		{
			name: "a day stepped back to, the next dip too close, of the same count, joined", shown: wednesday,
			pool: views.Pool{Of: 3, Dips: []views.Dip{limitWed, {Steps: []views.Step{stepAt(2, wed(19, 12).Add(40*time.Second), wed(23, 58), 109, 135)}, Column: 108, First: 109, Last: 135}}},
			from: 24, edge: 160,
			want: []phraseRead{{109, "2 of 3 with room 15:02 – 17:10 and 19:12 – 23:58"}},
		},
		{
			name: "a day stepped back to, the dip joining it dropped where it doesn't fit", shown: wednesday,
			pool: views.Pool{Of: 3, Dips: []views.Dip{limitWed, {Steps: []views.Step{stepAt(2, wed(19, 12), wed(23, 58), 109, 135)}, Column: 108, First: 109, Last: 135}}},
			from: 24, edge: 150,
			want: []phraseRead{{109, "2 of 3 with room 15:02 – 17:10"}},
		},
		{
			name: "a day stepped back to, the next dip too close, of another count, two cells after", shown: wednesday,
			pool: views.Pool{Of: 3, Dips: []views.Dip{limitWed, {Steps: []views.Step{stepAt(1, wed(19, 12), wed(23, 58), 109, 135)}, Column: 108, First: 109, Last: 135}}},
			from: 24, edge: 200,
			want: []phraseRead{{109, "2 of 3 with room 15:02 – 17:10"}, {141, "1 of 3 with room 19:12 – 23:58"}},
		},
		{
			name: "a day stepped back to, a back past its midnight", shown: wednesday,
			pool: views.Pool{Of: 3, Dips: []views.Dip{{Steps: []views.Step{stepAt(2, wed(22, 0), routedClock(1, 1, 10), 125, 135)}, Column: 125, First: 125, Last: 135}}},
			from: 24, edge: 200,
			want: []phraseRead{{149, "2 of 3 with room 22:00 – Thu 01:10"}},
		},
		{
			name: "the week's, with their weekdays", shown: week,
			pool: views.Pool{Of: 3, Dips: []views.Dip{{Steps: []views.Step{{With: 2, From: views.Edge{At: routedClock(3, 6, 0), Projected: true}, Until: views.Edge{At: routedClock(4, 2, 0)}, First: 51, Last: 66}}, Column: 51, First: 51, Last: 66, Ahead: true}}},
			from: 22, edge: 160,
			want: []phraseRead{{75, "2 of 3 with room ~Sat 06:00 – Sun 02:00"}},
		},
		{
			name: "dropping whole parts from the end", shown: live, pool: tenPool(), from: 24, edge: 80,
			want: []phraseRead{{25, "8 of 10 with room till 15:30, 9 till ~16:05"}},
		},
		{
			name: "a dip whose first part doesn't fit, unsaid, nor any after it", shown: live,
			pool: views.Pool{Of: 3, Dips: []views.Dip{underWay, later, aheadOf(routedClock(1, 22, 5), routedClock(1, 22, 30), 50, 52)}},
			from: 24, edge: 80,
			want: []phraseRead{{29, "2 of 3 with room till 17:10"}},
		},
		{
			name: "a step whose end isn't known ends the phrase", shown: live,
			pool: views.Pool{Of: 3, Dips: []views.Dip{{Steps: []views.Step{stepAt(2, capped, time.Time{}, 5, 20), stepAt(1, time.Time{}, routedClock(1, 22, 0), 21, 30)}, Column: 5, First: 5, Last: 30}}},
			from: 24, edge: 160,
			want: []phraseRead{{29, "2 of 3 with room"}},
		},
		{
			name: "a pool of one", shown: live,
			pool: views.Pool{Of: 1, Dips: []views.Dip{{Steps: []views.Step{stepAt(0, routedClock(1, 13, 58), routedClock(1, 15, 30), 2, 10)}, Column: 1, First: 2, Last: 10}}},
			from: 24, edge: 160,
			want: []phraseRead{{25, "0 of 1 with room till 15:30"}},
		},
		{name: "no dips", shown: live, pool: views.Pool{Of: 3}, from: 24, edge: 160},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phrasesRead(tt.shown.poolPhrases(tt.pool, routedAt, runwayLanesFrom, tt.from, tt.edge))
			if !slices.Equal(got, tt.want) {
				t.Errorf("the pool's words are %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Each phrase's counts are in text.secondary, and the rest in text.muted.
func TestThePoolsWordsInks(t *testing.T) {
	live := RunwayShown{Timeline: views.LiveTimeline(views.RunwayFiveHour, routedAt, 136)}
	got := live.poolPhrases(tenPool(), routedAt, runwayLanesFrom, 24, 160)[0].said.inked()
	want := line{
		{"8 of 10", secondaryInk}, {" with room till 15:30, ", mutedInk}, {"9", secondaryInk}, {" till ~16:05, ", mutedInk},
		{"8", secondaryInk}, {" till 16:55, ", mutedInk}, {"9", secondaryInk}, {" till 17:10", mutedInk},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the phrase is inked %+v, want %+v", got, want)
	}
}

// On a phone, as its frame has them, the pool's words start from the lanes'
// column, the dip's, its room on the row under them, from column 2; the
// rule, the columns and Σ pool as on a frame from 100 columns. Now's column
// is passed over: the lanes' drawer picks it out, and the frame's
// part-filled cell is on the canvas where the design has bg.subtle (D4).
func TestThePoolsLaneOnAPhoneIsDrawnAsItsFrame(t *testing.T) {
	doc := framesDoc()
	nord := Screen(builtin(t, "nord"))
	tl := views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayColumns(52))
	pool := views.Pool{
		Of: 3, Key: "5h", Columns: poolColumnsOf(tl.Columns, 3, 0, 2, 1, 3, 6),
		Dips: []views.Dip{{Steps: []views.Step{stepAt(2, routedAt.Add(-11*time.Minute-4*time.Second), routedClock(1, 17, 10), 1, 5)}, Column: 1, First: 1, Last: 5}},
	}
	f := Frame{Width: 52, Height: 33, Look: nord, View: Runway, Policy: claudeLike, Runway: poolShown(doc, pool)}
	f.Runway.Timeline = tl
	c := newCanvas(f.Width, f.Height)
	fit := f.runwayFittedAs(f.top(), runwayShape{roomRow: true, legend: 2})
	f.runwayPool(c, runwayLaid{x: phoneRunwayLanesFrom, fit: fit, m: f.Moment(doc, routedAt)})
	nows := []spot{{19, 15, 15}, {20, 15, 15}, {21, 15, 15}, {22, 15, 15}}
	heldTo(t, "runway-phone-52", reference(t, "runway-phone-52"), screenOf(t, c.rows(nord)), region{top: 19, bottom: 23, right: 51}, shadeOf(nord.Canvas()), nows, nil)
}

// Runway's pool's lane, its room and its words, and COMING UP, are drawn as
// their frames draw them: the 5-hour's and the week's live, Wednesday
// stepped back to, with no room and no COMING UP, and ten accounts', its
// COMING UP folded, now's column picked out down the pool's words. The rows
// between them, the legend's, are the lanes' drawer's, as are those over the
// rule; and the part-filled cell of now's column is passed over, on the
// canvas in the frames where the design has bg.subtle (D4).
func TestRunwaysPoolAndComingUpAreDrawnAsTheirFrames(t *testing.T) {
	doc := framesDoc()
	m := doc.At(routedAt, claudeLike)
	capped := routedAt.Add(-11*time.Minute - 4*time.Second)
	wed := func(hour, minute int) time.Time { return routedClock(1, hour, minute).AddDate(0, 0, -1) }
	fiveHour := poolShown(doc, views.Pool{
		Of: 3, Key: "5h", Columns: poolColumnsOf(136, 3, 0, 2, 5, 3, 21),
		Dips: []views.Dip{{Steps: []views.Step{stepAt(2, capped, routedClock(1, 17, 10), 5, 20)}, Column: 5, First: 5, Last: 20}},
	})
	fiveHour.Coming = views.RunwayComing(m, views.RunwayFiveHour)
	week := poolShown(doc, views.Pool{
		Of: 3, Key: "7d", Columns: poolColumnsOf(136, 2, 0, 3, 2, 2, 4, 3, 8, 2, 19, 3, 21, 2, 51, 3, 68),
		Dips: []views.Dip{
			{Steps: []views.Step{stepAt(2, wed(15, 2), wed(17, 10), 0, 1)}, Column: 0, First: 0, Last: 1, Over: true},
			{Steps: []views.Step{stepAt(2, wed(19, 12), wed(23, 58), 4, 7)}, Column: 3, First: 4, Last: 7, Over: true},
			{Steps: []views.Step{stepAt(2, capped, routedClock(1, 17, 10), 19, 20)}, Column: 19, First: 19, Last: 20},
			{Steps: []views.Step{{With: 2, From: views.Edge{At: routedClock(3, 6, 0), Projected: true}, Until: views.Edge{At: routedClock(4, 2, 0)}, First: 51, Last: 67}}, Column: 51, First: 51, Last: 67, Ahead: true},
		},
	})
	week.Window, week.Timeline = views.RunwayWeek, views.LiveTimeline(views.RunwayWeek, routedAt, 136)
	week.Coming = []status.Upcoming{
		{At: routedClock(1, 16, 30), Account: "personal", Kind: status.UpcomingReset, Window: "5h"},
		{At: routedClock(1, 17, 10), Account: "work", Kind: status.UpcomingBack, Cap: true},
		{At: routedClock(1, 18, 50), Account: "side", Kind: status.UpcomingReset, Window: "5h"},
		{At: routedClock(3, 6, 0), Account: "personal", Kind: status.UpcomingRunsOut, Window: "7d"},
		{At: routedClock(4, 2, 0), Account: "personal", Kind: status.UpcomingReset, Window: "7d"},
		{At: routedClock(4, 21, 0), Account: "work", Kind: status.UpcomingReset, Window: "7d"},
		{At: routedClock(5, 10, 0), Account: "side", Kind: status.UpcomingReset, Window: "7d"},
	}
	wednesday := poolShown(doc, views.Pool{
		Of: 3, Key: "5h", Columns: poolColumnsOf(136, 3, 0, 2, 85, 3, 97, 2, 109),
		Dips: []views.Dip{
			{Steps: []views.Step{stepAt(2, wed(15, 2), wed(17, 10), 85, 96)}, Column: 85, First: 85, Last: 96},
			{Steps: []views.Step{stepAt(2, wed(19, 12).Add(40*time.Second), wed(23, 58), 109, 135)}, Column: 108, First: 109, Last: 135},
		},
	})
	wednesday.Back = civilDay(9, 30)
	wednesday.Timeline = wednesday.TimelineAt(routedAt, time.Monday, 160)
	ten := tenRunway()
	tenShown := poolShown(ten, tenPool())
	tenShown.Coming = views.RunwayComing(ten.At(routedAt, claudeLike), views.RunwayFiveHour)
	tests := []struct {
		stem  string
		doc   status.Document
		shown RunwayShown
		// regions are those of the frame the pool's and COMING UP's drawers
		// draw, and partFilled the part-filled cell of now's column in them.
		regions    []region
		partFilled []spot
	}{
		{stem: "runway-5-hour-160", doc: doc, shown: fiveHour, regions: []region{{top: 18, bottom: 22, right: 159}, {top: 24, bottom: 35, right: 159}}, partFilled: []spot{{19, 30, 30}}},
		{stem: "runway-week-160", doc: doc, shown: week, regions: []region{{top: 18, bottom: 22, right: 159}, {top: 24, bottom: 35, right: 159}}, partFilled: []spot{{19, 43, 43}}},
		{stem: "runway-stepped-back-160", doc: doc, shown: wednesday, regions: []region{{top: 18, bottom: 22, right: 159}, {top: 24, bottom: 35, right: 159}}},
		{stem: "runway-ten-accounts-160", doc: ten, shown: tenShown, regions: []region{{top: 29, bottom: 32, right: 159}, {top: 34, bottom: 35, right: 159}}, partFilled: []spot{{30, 30, 30}}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: 160, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			f.WindowName = func(key string) string { return map[string]string{"5h": "5-hour", "7d": "week"}[key] }
			drawn, frame := screenOf(t, f.Draw(tt.doc, routedAt)), reference(t, tt.stem)
			for _, in := range tt.regions {
				heldTo(t, tt.stem, frame, drawn, in, shadeOf(f.Look.Canvas()), tt.partFilled, nil)
			}
		})
	}
}
