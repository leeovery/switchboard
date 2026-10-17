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

// The pool's columns are each as high as the share of the pool's accounts
// with room, of its two rows, in eighths of a cell, rounded half to even:
// full cells a background, the top cell part-filled, as GRAPHS fills its
// columns; coloured as usage colours the pool's use, the share without
// room, its ramp's first stop under 70%, its second from 70%, its last from
// 90%; ─ in state.destructive on its lower row where none of its accounts
// has room, as a lane held back from every request is drawn; and blank where
// none of their states is known.
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
		{name: "none of three", col: views.PoolColumn{Known: true}, of: 3, top: blankCell, bottom: seenCell{"─", errorInk}},
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

// A column of the pool's where none of its accounts has room is dimmed
// before now's column as the past is, and where the look has no colour, its
// ─ is bold, as a lane's held back from every request is.
func TestThePoolsColumnsWithNoRoomAreDrawnAsALanesHeldBack(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{Known: true}}, RunwayColumns(160))
	shown := poolShown(doc, views.Pool{Of: 3, Key: "5h", Columns: columns})
	now := shown.Timeline.NowColumn()
	tests := []struct {
		name   string
		look   func(t *testing.T) Look
		column int
		want   seenCell
	}{
		{name: "before now's column", look: func(t *testing.T) Look { return Screen(builtin(t, "nord")) }, column: now - 1, want: seenCell{"─", ink{token: errorInk.token, fade: pastFade}}},
		{name: "after now's column", look: func(t *testing.T) Look { return Screen(builtin(t, "nord")) }, column: now + 1, want: seenCell{"─", errorInk}},
		{name: "without colour", look: func(*testing.T) Look { return NoColour() }, column: now + 1, want: seenCell{"─", exhaustedInk}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, top := poolDrawn(shown, doc, tt.look(t))
			x := runwayLanesFrom + tt.column
			if got := cellAt(c, x, top+1); got != tt.want {
				t.Errorf("column %d's lower cell is %+v, want %+v", tt.column, got, tt.want)
			}
			if got := cellAt(c, x, top); got != blankCell {
				t.Errorf("column %d's upper cell is %+v, want it blank", tt.column, got)
			}
		})
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

// Σ pool heads the pool's lane on its first row, as an account's name heads
// its lane, Σ in text.subtle in the accounts' numbers' column, the ones', and
// pool in text.secondary; and no rule runs over it, the row before it the
// last lane's blank row, with three rows an account, or its words, with two.
func TestSigmaPoolHeadsThePoolsLaneWithNoRuleOverIt(t *testing.T) {
	tests := []struct {
		name     string
		accounts int
		pool     int
		over     string
		sigma    string
	}{
		{name: "three, after the last lane's blank row", accounts: 3, pool: 19, sigma: "Σ pool"},
		{name: "ten, after the last lane's words, numbered from ten", accounts: 10, pool: 30, over: "  5-hour", sigma: " Σ pool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := runwayDoc(tt.accounts)
			c, top := poolDrawn(poolShown(doc, views.Pool{Of: tt.accounts, Key: "5h"}), doc, Screen(builtin(t, "nord")))
			if top != tt.pool {
				t.Errorf("the pool's lane is on row %d, want %d", top, tt.pool)
			}
			if got := rowOf(c, top-1); !strings.HasPrefix(got, tt.over) || strings.Contains(got, "─") {
				t.Errorf("the row over the pool's lane reads %q, want %q and no rule", got, tt.over)
			}
			if got := rowOf(c, top); got != tt.sigma {
				t.Errorf("the pool's first row reads %q, want %q", got, tt.sigma)
			}
			sigma := strings.Index(tt.sigma, "Σ")
			if k, pool := c.at(sigma, top).ink, c.at(sigma+2, top).ink; k != dimInk || pool != noteInk {
				t.Errorf("Σ is in %+v and pool in %+v, want text.subtle and text.secondary", k, pool)
			}
		})
	}
}

// With one account, there's no pool's lane: no columns, no Σ pool and no
// room, as they'd only repeat the account.
func TestOneAccountHasNoPoolsLane(t *testing.T) {
	doc := runwayDoc(1)
	c, _ := poolDrawn(poolShown(doc, views.Pool{}), doc, Screen(builtin(t, "nord")))
	for y, row := range textOf(c.rows(Look{})) {
		if strings.Contains(row, "Σ") || strings.Contains(row, " left") {
			t.Errorf("row %d reads %q, want nothing of a pool", y, row)
		}
	}
}

// Lanes of a pool of three drawn for a moment of no accounts draw no pool's
// lane, as they draw no account's: there's no account to name it under.
func TestThePoolsLaneIsntDrawnForAMomentOfNoAccounts(t *testing.T) {
	c := runwayDrawn(runwayFrame(t, 37, lanesOf(3), nil), status.Document{})
	for y, row := range textOf(c.rows(Look{})) {
		if strings.Contains(row, "Σ") || strings.Contains(row, " left") {
			t.Errorf("row %d reads %q, want nothing of a pool", y, row)
		}
	}
}

// The pool's room, from column 2, is usage's words for the pool in the
// view's window: the window's name in text.subtle, the room bold
// text.secondary, brightening as it changes, and " of 3 left" in
// text.muted, " · pinned" after where a pin holds; drawn while the timeline
// holds now, live and on today stepped back to, never on a day gone. It's
// on the pool's lane's second row, under Σ pool, as an account's use is
// under its name, whole parts left off its end that don't fit before the
// lanes' column, as " · pinned" doesn't at 160; and on a phone, on a row of
// its own under the lane, whole; nothing after it either way.
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
	phone := live
	phone.Timeline = views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayColumns(52))
	tests := []struct {
		name       string
		doc        status.Document
		shown      RunwayShown
		width, row int
		want       string
	}{
		{name: "the 5-hour", doc: doc, shown: live, width: 160, row: 20, want: "  5-hour 1.5 of 3 left"},
		{name: "the week", doc: doc, shown: week, width: 160, row: 20, want: "  week 1.2 of 3 left"},
		{name: "pinned to two, beside the lane", doc: pinned, shown: live, width: 160, row: 20, want: "  5-hour 1.5 of 2 left"},
		{name: "today stepped back to", doc: doc, shown: today, width: 160, row: 20, want: "  5-hour 1.5 of 3 left"},
		{name: "a day stepped back to", doc: doc, shown: gone, width: 160, row: 20, want: ""},
		{name: "on a phone", doc: doc, shown: phone, width: 52, row: 22, want: "  5-hour 1.5 of 3 left"},
		{name: "pinned to two, on a phone", doc: pinned, shown: phone, width: 52, row: 22, want: "  5-hour 1.5 of 2 left · pinned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			c := runwayDrawn(f, tt.doc)
			if got := rowOf(c, tt.row); got != tt.want {
				t.Errorf("the pool's room's row reads %q, want %q", got, tt.want)
			}
			if tt.want == "" {
				return
			}
			name, room, rest := c.at(2, tt.row).ink, c.at(strings.Index(tt.want, "1."), tt.row), c.at(strings.Index(tt.want, " of")+1, tt.row).ink
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

// On a phone, as its frame has them, the pool's columns from the lanes'
// column, now's column picked out down the lane; and as the design has
// since drawn them, a row up from the frame's, Σ pool on the lane's first
// row and its room on a row of its own under the lane, from column 2, the
// rule over the lane and its words under it gone. The frame's part-filled
// cell of now's column is passed over, on the canvas where the design has
// bg.subtle (D4).
func TestThePoolsLaneOnAPhoneIsDrawnAsItsFrame(t *testing.T) {
	doc := framesDoc()
	nord := Screen(builtin(t, "nord"))
	tl := views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayColumns(52))
	pool := views.Pool{Of: 3, Key: "5h", Columns: poolColumnsOf(tl.Columns, 3, 0, 2, 1, 3, 6)}
	f := Frame{Width: 52, Height: 33, Look: nord, View: Runway, Policy: claudeLike, Runway: poolShown(doc, pool)}
	f.Runway.Timeline = tl
	c := newCanvas(f.Width, f.Height)
	fit := f.runwayFittedAs(f.top(), runwayShape{roomRow: true, legend: 2})
	laid := runwayLaid{x: phoneRunwayLanesFrom, fit: fit, m: f.Moment(doc, routedAt)}
	f.runwayPool(c, laid)
	f.runwayNow(c, laid)
	drawn, frame, canvas := screenOf(t, c.rows(nord)), reference(t, "runway-phone-52"), shadeOf(nord.Canvas())
	heldTo(t, "runway-phone-52", frame, drawn, region{top: 20, bottom: 21, left: phoneRunwayLanesFrom, right: 51}, canvas, []spot{{20, 15, 15}}, nil)
	heldTo(t, "runway-phone-52", frame, aRowUp(drawn), region{top: 21, bottom: 22, right: phoneRunwayLanesFrom - 1}, canvas, nil, nil)
	heldTo(t, "runway-phone-52", frame, aRowUp(drawn), region{top: 23, bottom: 23, right: 51}, canvas, nil, nil)
}

// Runway's pool's lane, its room, and COMING UP, are drawn as their frames
// draw them: the 5-hour's and the week's live, Wednesday stepped back to,
// with no room and no COMING UP, and ten accounts', its COMING UP folded.
// The pool's columns are where the frames draw them, now's column picked out
// down the lane; Σ pool, the room and COMING UP a row up from the frames',
// the rule over the lane and its words under it gone. The rows between them,
// the legend's, are the lanes' drawer's, as are those over the pool's lane;
// and the part-filled cell of now's column is passed over, on the canvas in
// the frames where the design has bg.subtle (D4).
func TestRunwaysPoolAndComingUpAreDrawnAsTheirFrames(t *testing.T) {
	doc := framesDoc()
	m := doc.At(routedAt, claudeLike)
	fiveHour := poolShown(doc, views.Pool{Of: 3, Key: "5h", Columns: poolColumnsOf(136, 3, 0, 2, 5, 3, 21)})
	fiveHour.Coming = views.RunwayComing(m, views.RunwayFiveHour)
	week := poolShown(doc, views.Pool{Of: 3, Key: "7d", Columns: poolColumnsOf(136, 2, 0, 3, 2, 2, 4, 3, 8, 2, 19, 3, 21, 2, 51, 3, 68)})
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
	wednesday := poolShown(doc, views.Pool{Of: 3, Key: "5h", Columns: poolColumnsOf(136, 3, 0, 2, 85, 3, 97, 2, 109)})
	wednesday.Back = civilDay(9, 30)
	wednesday.Timeline = wednesday.TimelineAt(routedAt, time.Monday, 160)
	ten := tenRunway()
	tenShown := poolShown(ten, tenPool())
	tenShown.Coming = views.RunwayComing(ten.At(routedAt, claudeLike), views.RunwayFiveHour)
	columnsFrom := func(top int) []region {
		return []region{{top: top, bottom: top + 1, left: runwayLanesFrom, right: 159}}
	}
	threesUp := []region{{top: 20, bottom: 21, right: runwayLanesFrom - 1}, {top: 24, bottom: 35, right: 159}}
	tests := []struct {
		stem  string
		doc   status.Document
		shown RunwayShown
		// held are the regions of the frame the pool's and COMING UP's
		// drawers draw where it has them, and up those they draw a row up from
		// it; partFilled is the part-filled cell of now's column in them.
		held, up   []region
		partFilled []spot
	}{
		{stem: "runway-5-hour-160", doc: doc, shown: fiveHour, held: columnsFrom(19), up: threesUp, partFilled: []spot{{19, 30, 30}}},
		{stem: "runway-week-160", doc: doc, shown: week, held: columnsFrom(19), up: threesUp, partFilled: []spot{{19, 43, 43}}},
		{stem: "runway-stepped-back-160", doc: doc, shown: wednesday, held: columnsFrom(19), up: threesUp},
		{
			stem: "runway-ten-accounts-160", doc: ten, shown: tenShown, held: columnsFrom(30),
			up: []region{{top: 31, bottom: 32, right: runwayLanesFrom - 1}, {top: 34, bottom: 35, right: 159}}, partFilled: []spot{{30, 30, 30}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: 160, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			f.WindowName = func(key string) string { return map[string]string{"5h": "5-hour", "7d": "week"}[key] }
			drawn, frame, canvas := screenOf(t, f.Draw(tt.doc, routedAt)), reference(t, tt.stem), shadeOf(f.Look.Canvas())
			for _, in := range tt.held {
				heldTo(t, tt.stem, frame, drawn, in, canvas, tt.partFilled, nil)
			}
			for _, in := range tt.up {
				heldTo(t, tt.stem, frame, aRowUp(drawn), in, canvas, nil, nil)
			}
		})
	}
}
