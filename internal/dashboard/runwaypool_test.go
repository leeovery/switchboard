package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// poolShown is Runway's live view at routedAt, at 160 columns, its
// lanes those of doc's accounts, its pool's lane as given.
func poolShown(doc status.Document, pool views.Pool) RunwayShown {
	lanes := &views.Lanes{Pool: pool}
	for _, a := range doc.Accounts {
		lanes.Accounts = append(lanes.Accounts, views.Lane{ID: a.ID})
	}
	return RunwayShown{Timeline: views.LiveTimeline(routedAt, RunwayShown{}.Columns(160)), Lanes: lanes}
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
// full cells a background, the top cell part-filled, as Graphs fills its
// columns; coloured as usage colours the pool's use, the share without
// room, its ramp's first stop under 70%, its second from 70%, its last from
// 90%, and with any room, ▃ at the least; ▂ in state.destructive on its
// lower row where none of its accounts has room, a limit's ink, a
// quarter-cell along its floor; and blank where none of their states is
// known.
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
		{name: "one of ten, 90% without", col: views.PoolColumn{With: 1, Known: true}, of: 10, top: blankCell, bottom: part("▃", theme.VizRamp4)},
		{name: "none of three", col: views.PoolColumn{Known: true}, of: 3, top: blankCell, bottom: seenCell{"▂", errorInk}},
		{name: "none known", col: views.PoolColumn{}, of: 3, top: blankCell, bottom: blankCell},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := slices.Repeat([]views.PoolColumn{tt.col}, RunwayShown{}.Columns(160))
			c, top := poolDrawn(poolShown(runwayDoc(tt.of), views.Pool{Of: tt.of, Key: "7d", Columns: columns}), runwayDoc(tt.of), Screen(builtin(t, "nord")))
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
// the past is: now's and those after it aren't, and on a week gone
// stepped back to, which has no now, none is.
func TestThePoolsPastIsDimmed(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{With: 2, Known: true}}, RunwayShown{}.Columns(160))
	live := poolShown(doc, views.Pool{Of: 3, Key: "7d", Columns: columns})
	gone := live
	gone.Back = civilDay(9, 21)
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
		{name: "a week gone's first", shown: gone, column: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, top := poolDrawn(tt.shown, doc, Screen(builtin(t, "nord")))
			x := RunwayShown{}.lanesFrom(160) + tt.column
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

// ▂ on the lane's floor means the pool at its limit alone: a column of the
// pool's with any of its accounts with room, in any share, down to 1 of 40,
// fills ▃ at the least on its lower row, or the whole cell. On its upper
// row, ▂ stands on a full cell, so it can't be read as the limit.
func TestAPoolsColumnWithRoomFillsThreeEighthsAtTheLeast(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	pools := []int{40}
	for of := 2; of <= 24; of++ {
		pools = append(pools, of)
	}
	for _, of := range pools {
		doc := runwayDoc(of)
		columns := make([]views.PoolColumn, RunwayShown{}.Columns(160))
		for i := range columns {
			columns[i] = views.PoolColumn{With: 1 + i%of, Known: true}
		}
		c, top := poolDrawn(poolShown(doc, views.Pool{Of: of, Key: "7d", Columns: columns}), doc, look)
		x := RunwayShown{}.lanesFrom(160)
		for i, col := range columns {
			if lower := cellAt(c, x+i, top+1); !atLeastThreeEighths(lower) {
				t.Errorf("%d of %d with room, the column's lower row is %+v, want ▃ or fuller", col.With, of, lower)
			}
		}
	}
}

// atLeastThreeEighths reports whether the cell is filled three eighths of
// the way up or more: ▃ to ▇, or a full cell, its ground.
func atLeastThreeEighths(cell seenCell) bool {
	return slices.Contains([]string{"▃", "▄", "▅", "▆", "▇"}, cell.glyph) || cell.glyph == " " && cell.ink.on != (hue{})
}

// A column of the pool's where none of its accounts has room is dimmed
// before now's column as the past is, and where the look has no colour, its
// ▂ is bold, as a lane's held back from every request is.
func TestThePoolsColumnsWithNoRoomAreDrawnAsALanesHeldBack(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{Known: true}}, RunwayShown{}.Columns(160))
	shown := poolShown(doc, views.Pool{Of: 3, Key: "7d", Columns: columns})
	now := shown.Timeline.NowColumn()
	tests := []struct {
		name   string
		look   func(t *testing.T) Look
		column int
		want   seenCell
	}{
		{name: "before now's column", look: func(t *testing.T) Look { return Screen(builtin(t, "nord")) }, column: now - 1, want: seenCell{"▂", ink{token: errorInk.token, fade: pastFade}}},
		{name: "after now's column", look: func(t *testing.T) Look { return Screen(builtin(t, "nord")) }, column: now + 1, want: seenCell{"▂", errorInk}},
		{name: "without colour", look: func(*testing.T) Look { return NoColour() }, column: now + 1, want: seenCell{"▂", exhaustedInk}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, top := poolDrawn(shown, doc, tt.look(t))
			x := RunwayShown{}.lanesFrom(160) + tt.column
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
// columns is █, as Graphs draws one.
func TestThePoolsFullCellsAreBlocksWhereTheLookBlendsNothing(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{With: 2, Known: true}}, RunwayShown{}.Columns(160))
	c, top := poolDrawn(poolShown(doc, views.Pool{Of: 3, Key: "7d", Columns: columns}), doc, NoColour())
	if got := string([]rune(rowOf(c, top+1))[RunwayShown{}.lanesFrom(160):]); got != strings.Repeat("█", RunwayShown{}.Columns(160)) {
		t.Errorf("the pool's bottom row reads %q, want █ in every column", got)
	}
	if got := string([]rune(rowOf(c, top))[RunwayShown{}.lanesFrom(160):]); got != strings.Repeat("▃", RunwayShown{}.Columns(160)) {
		t.Errorf("the pool's top row reads %q, want its part-filled cells", got)
	}
}

// Σ pool heads the pool's lane on its first row, as an account's name heads
// its lane, Σ in text.subtle at the label column's start, and pool in
// text.secondary; and no rule runs over it, the row before it the
// last lane's blank row, with three rows an account, or its words, with two.
func TestSigmaPoolHeadsThePoolsLaneWithNoRuleOverIt(t *testing.T) {
	tests := []struct {
		name     string
		accounts int
		pool     int
		over     string
		sigma    string
	}{
		{name: "three, after the last lane's blank row", accounts: 3, pool: 18, sigma: "Σ pool"},
		{name: "ten, after the last lane's words", accounts: 10, pool: 29, over: "  week", sigma: "Σ pool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := runwayDoc(tt.accounts)
			c, top := poolDrawn(poolShown(doc, views.Pool{Of: tt.accounts, Key: "7d"}), doc, Screen(builtin(t, "nord")))
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

// lackingFiveHour is the moment at routedAt of the auto world the frames
// were drawn of, as framesDoc has it, the accounts with the given ids reading
// as verified Ultra accounts do, with no 5-hour window.
func lackingFiveHour(ids ...string) status.Moment {
	doc := framesDoc()
	doc.Accounts = slices.Clone(doc.Accounts)
	for i, a := range doc.Accounts {
		if slices.Contains(ids, a.ID) {
			doc.Accounts[i].Windows = slices.DeleteFunc(slices.Clone(a.Windows), func(w quota.Window) bool { return w.Key == "5h" })
		}
	}
	return doc.WorkedOut(claudeLike).At(routedAt, claudeLike)
}

// The pool's room is said of how many of its accounts have the window it's
// of, as the window's Of counts them: of 2 in the 5-hour where one of three
// is verified Ultra, of 3 in its week, which all have, collapsed and
// expanded alike; and nothing where the pool keeps no 5-hour, as with side
// alone having one.
func TestThePoolsRoomIsSaidOfItsAccountsThatHaveTheWindow(t *testing.T) {
	tests := []struct {
		name         string
		lacking      []string
		key          string
		short, words string
		without      bool
	}{
		{name: "the 5-hour, every account with one", key: "5h", short: "1.5/3", words: "1.5 of 3 left"},
		{name: "the 5-hour, work verified Ultra", lacking: []string{"work"}, key: "5h", short: "1.5/2", words: "1.5 of 2 left"},
		{name: "the week, work verified Ultra", lacking: []string{"work"}, key: "7d", short: "1.2/3", words: "1.2 of 3 left"},
		{name: "the 5-hour, side alone with one", lacking: []string{"work", "personal"}, key: "5h", without: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ok := runwayRoom(lackingFiveHour(tt.lacking...), tt.key)
			if ok == tt.without {
				t.Fatalf("the pool has a room in %s %t, want %t", tt.key, ok, !tt.without)
			}
			if tt.without {
				return
			}
			if got := poolShort(r).plain(); got != tt.short {
				t.Errorf("collapsed, the pool's room reads %q, want %q", got, tt.short)
			}
			if got := poolWords(r.room, r.accounts, r.pinned).full.plain(); got != tt.words {
				t.Errorf("the pool's room reads %q, want %q", got, tt.words)
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

// With the account column collapsed, the pool's lane starts at column 0,
// without Σ, and on the row of its own under the lane, from column 0, its
// room short, the room the expanded lane words, of how many accounts the
// pool is: the room bold text.secondary, brightening as it changes, and "/3"
// in text.muted, at every width; and where it has no room to say, as on a
// week gone stepped back to, nothing under it.
func TestCollapsedThePoolsRoomIsSaidShortUnderItsLane(t *testing.T) {
	doc, pinned := framesDoc(), framesDoc()
	pinned.Pin = status.Pin{Accounts: []string{"personal", "side"}, Since: routedClock(1, 13, 40)}
	pinned = pinned.WorkedOut(claudeLike)
	collapsed := func(r RunwayShown, width int) RunwayShown {
		r.Collapsed = true
		r.Timeline = r.TimelineAt(routedAt, time.Monday, width)
		return r
	}
	live := poolShown(doc, views.Pool{Of: 3, Key: "7d"})
	gone := live
	gone.Back = civilDay(9, 21)
	tests := []struct {
		name  string
		doc   status.Document
		shown RunwayShown
		width int
		room  string
	}{
		{name: "live", doc: doc, shown: collapsed(live, 160), width: 160, room: "1.2/3"},
		{name: "pinned to two", doc: pinned, shown: collapsed(live, 160), width: 160, room: "0.6/2"},
		{name: "on a phone", doc: doc, shown: collapsed(live, 52), width: 52, room: "1.2/3"},
		{name: "a week gone stepped back to", doc: doc, shown: collapsed(gone, 160), width: 160},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			c := runwayDrawn(f, tt.doc)
			fit := f.runwayFitted(f.top())
			if x := f.Runway.lanesFrom(f.Width); x != 0 {
				t.Errorf("the pool's lane starts at column %d, want 0", x)
			}
			if got := rowOf(c, fit.pool); strings.Contains(got, "Σ") {
				t.Errorf("the pool's lane reads %q, want no Σ", got)
			}
			under := fit.pool + runwayPoolRows
			if got := rowOf(c, under); got != tt.room {
				t.Errorf("the row under the pool's lane reads %q, want %q", got, tt.room)
			}
			if tt.room == "" {
				return
			}
			if fit.room != under {
				t.Errorf("the pool's room is laid on row %d, want the row under its lane, %d", fit.room, under)
			}
			if k, of := c.at(0, under).ink, c.at(3, under).ink; k.token != theme.TextSecondary || !k.bold || of != mutedInk {
				t.Errorf("the room is in %+v and its accounts in %+v; want bold text.secondary and text.muted", k, of)
			}
			key := tt.shown.Lanes.Pool.Key
			if got, want := f.Numbers(tt.doc, routedAt)[Number{Kind: NumberRoom, Of: key}], tt.room[:3]; got != want {
				t.Errorf("the room brightens as %q, want %q", got, want)
			}
		})
	}
}

// The pool's room, from column 2, is usage's words for the pool in the
// week: its name in text.subtle, the room bold text.secondary, brightening
// as it changes, and " of 3 left" in text.muted, " · pinned" after where a
// pin holds; drawn while the timeline holds now, live and on this week
// stepped back to, never on a week gone. It's
// on a row of its own under the pool's lane, at every width, whole parts
// left off its end that don't fit before the frame's edge; nothing after
// it.
func TestThePoolsRoomIsUsagesWordsForThePoolInTheWeek(t *testing.T) {
	doc, pinned := framesDoc(), framesDoc()
	pinned.Pin = status.Pin{Accounts: []string{"personal", "side"}, Since: routedClock(1, 13, 40)}
	pinned = pinned.WorkedOut(claudeLike)
	live := poolShown(doc, views.Pool{Of: 3, Key: "7d"})
	today := live
	today.Back = civilDay(9, 28)
	today.Timeline = today.TimelineAt(routedAt, time.Monday, 160)
	gone := live
	gone.Back = civilDay(9, 21)
	gone.Timeline = gone.TimelineAt(routedAt, time.Monday, 160)
	phone := live
	phone.Timeline = views.LiveTimeline(routedAt, RunwayShown{}.Columns(52))
	tests := []struct {
		name               string
		doc                status.Document
		shown              RunwayShown
		width, height, row int
		want               string
	}{
		{name: "live", doc: doc, shown: live, width: 160, height: 37, row: 20, want: "  week 1.2 of 3 left"},
		{name: "pinned to two", doc: pinned, shown: live, width: 160, height: 37, row: 20, want: "  week 0.6 of 2 left · pinned"},
		{name: "this week stepped back to", doc: doc, shown: today, width: 160, height: 37, row: 20, want: "  week 1.2 of 3 left"},
		{name: "a week gone stepped back to", doc: doc, shown: gone, width: 160, height: 37, row: 20, want: ""},
		{name: "on a phone", doc: doc, shown: phone, width: 52, height: 38, row: 23, want: "  week 1.2 of 3 left"},
		{name: "pinned to two, on a phone", doc: pinned, shown: phone, width: 52, height: 38, row: 23, want: "  week 0.6 of 2 left · pinned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			c := runwayDrawn(f, tt.doc)
			if got := rowOf(c, tt.row); got != tt.want {
				t.Errorf("the pool's room's row reads %q, want %q", got, tt.want)
			}
			if tt.want == "" {
				return
			}
			name, room, rest := c.at(2, tt.row).ink, c.at(strings.IndexAny(tt.want, "0123456789"), tt.row), c.at(strings.Index(tt.want, " of")+1, tt.row).ink
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

// notHeld is a hold curve that keeps none of an account's fast rate on it
// from an hour on.
var notHeld = []status.HoldPoint{{Hours: 1}, {Hours: 3}, {Hours: 6}, {Hours: 12}, {Hours: 24}, {Hours: 48}}

// forecastDoc is doc with the pool's use at the day rate given, in Pro weeks
// an hour, none of it held on an account, as the router measures it, worked
// out, so its forecast plays it.
func forecastDoc(doc status.Document, day float64) status.Document {
	doc.Demand = &status.Demand{Day: day, Hours: 24, Hold: notHeld}
	return doc.WorkedOut(claudeLike)
}

// pinnedToSide is the auto world the frames were drawn of, as framesDoc has
// it, the pool's pin naming side alone.
func pinnedToSide() status.Document {
	doc := framesDoc()
	doc.Pin = status.Pin{Accounts: []string{"side"}, Since: routedClock(1, 13, 40)}
	return doc
}

// poolLasting is the auto world the frames were drawn of, as framesDoc has
// it, its pool's use such that it lasts the week, 20% to spare; and
// poolRunsShort, such that it runs out on Sunday, lasting at 20% less.
func poolLasting() status.Document   { return forecastDoc(framesDoc(), 0.02) }
func poolRunsShort() status.Document { return forecastDoc(framesDoc(), 0.03) }

// After the pool's room in the week, Runway says how near the pool is to
// running out, after " · ", and after " · pinned" where a pin holds: in full
// where it fits, short where only that does, as on a phone; short alone with
// the account column collapsed; and
// nothing where it says nothing, as without a forecast, nor where the room
// isn't said, as on a week gone stepped back to.
func TestThePoolsMarginFollowsItsRoomInTheWeek(t *testing.T) {
	live := poolShown(framesDoc(), views.Pool{Of: 3, Key: "7d"})
	phone := live
	phone.Timeline = views.LiveTimeline(routedAt, RunwayShown{}.Columns(52))
	collapsed := live
	collapsed.Collapsed = true
	collapsed.Timeline = collapsed.TimelineAt(routedAt, time.Monday, 160)
	gone := live
	gone.Back = civilDay(9, 21)
	gone.Timeline = gone.TimelineAt(routedAt, time.Monday, 160)
	tests := []struct {
		name          string
		doc           status.Document
		shown         RunwayShown
		width, height int
		want          string
	}{
		{name: "lasting", doc: poolLasting(), shown: live, width: 160, height: 37, want: "  week 1.2 of 3 left · lasts the week, 20% to spare"},
		{name: "running out", doc: poolRunsShort(), shown: live, width: 160, height: 37, want: "  week 1.2 of 3 left · runs out ~Sun · lasts if you use 20% less"},
		{name: "running out, on a phone", doc: poolRunsShort(), shown: phone, width: 52, height: 38, want: "  week 1.2 of 3 left · out ~Sun"},
		{name: "running out, collapsed", doc: poolRunsShort(), shown: collapsed, width: 160, height: 37, want: "1.2/3 · out ~Sun"},
		{name: "lasting, collapsed", doc: poolLasting(), shown: collapsed, width: 160, height: 37, want: "1.2/3 · 20% to spare"},
		{name: "without a forecast", doc: framesDoc(), shown: live, width: 160, height: 37, want: "  week 1.2 of 3 left"},
		{name: "pinned", doc: forecastDoc(pinnedToSide(), 0.02), shown: live, width: 160, height: 37, want: "  week 0.2 of 1 left · pinned · lasts the week, 20% to spare"},
		{name: "pinned, on a phone", doc: forecastDoc(pinnedToSide(), 0.02), shown: phone, width: 52, height: 38, want: "  week 0.2 of 1 left · pinned · 20% to spare"},
		{name: "a week gone stepped back to", doc: poolRunsShort(), shown: gone, width: 160, height: 37},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			fit := f.runwayFitted(f.top())

			c := runwayDrawn(f, tt.doc)

			if got := rowOf(c, fit.pool+runwayPoolRows); got != tt.want {
				t.Errorf("the row under the pool's lane reads %q, want %q", got, tt.want)
			}
		})
	}
}

// The margin's words are inked as the pool's room is: " · " and its words in
// text.muted, its figures bold text.secondary, as the room is; and its
// run-out's time bold accent.attention, as an account's run-out ahead is.
func TestThePoolsMarginIsInkedAsItsRoomAndARunOutAhead(t *testing.T) {
	f := Frame{Width: 160, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: poolShown(framesDoc(), views.Pool{Of: 3, Key: "7d"})}
	row := f.runwayFitted(f.top()).room
	c := runwayDrawn(f, poolRunsShort())
	said := rowOf(c, row)
	at := func(s string) int { return len([]rune(said[:strings.Index(said, s)])) }
	tests := []struct {
		name string
		x    int
		want ink
	}{
		{name: "the dot", x: at(" · runs") + 1, want: mutedInk},
		{name: "its words", x: at("runs"), want: mutedInk},
		{name: "its run-out's time", x: at("Sun"), want: alertInk},
		{name: "its figure", x: at("20%"), want: strongInk},
		{name: "its words after", x: at(" less") + 1, want: mutedInk},
	}
	for _, tt := range tests {
		if got := c.at(tt.x, row).ink; got != tt.want {
			t.Errorf("%s, at column %d of %q, is in %+v, want %+v", tt.name, tt.x, said, got, tt.want)
		}
	}
}

// The margin is said in full in the cells given where it fits, else short,
// and short alone with the account column collapsed; nothing where neither
// fits; and nothing in the view of any window but the week the forecast
// plays, the policy's Perishable.
func TestThePoolsMarginTakesTheFormThatFitsInTheWeekAlone(t *testing.T) {
	m := poolRunsShort().At(routedAt, claudeLike)
	full, short := " · runs out ~Sun · lasts if you use 20% less", " · out ~Sun"
	cells := func(s string) int { return len([]rune(s)) }
	tests := []struct {
		name      string
		key       string
		collapsed bool
		cells     int
		want      string
	}{
		{name: "full, where it fits", key: "7d", cells: cells(full), want: full},
		{name: "short, a cell short of full", key: "7d", cells: cells(full) - 1, want: short},
		{name: "short, where it alone fits", key: "7d", cells: cells(short), want: short},
		{name: "nothing, a cell short of short", key: "7d", cells: cells(short) - 1},
		{name: "short, collapsed, with room for full", key: "7d", collapsed: true, cells: cells(full), want: short},
		{name: "nothing, of the 5-hour", key: "5h", cells: 160},
		{name: "nothing, of Fable's week", key: "7d_oi", cells: 160},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := poolShown(framesDoc(), views.Pool{Of: 3, Key: tt.key})
			shown.Collapsed = tt.collapsed
			f := Frame{Width: 160, Height: 37, View: Runway, Policy: claudeLike, Runway: shown}

			if got := f.marginSaid(m, tt.cells).plain(); got != tt.want {
				t.Errorf("in %d cells, the margin reads %q, want %q", tt.cells, got, tt.want)
			}
		})
	}
}

// Where Runway's lanes scroll, what's out of view keeps the cells it takes
// without the margin, the margin said in those left before it: in full with
// room, short where only that fits.
func TestThePoolsMarginYieldsToWhatsOutOfView(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		room, says string
	}{
		{name: "with room", width: 160, room: "  week 9.0 of 10 left · lasts the week, over 50% to spare", says: "▲ 2 more accounts · ▼ 1 more account"},
		{name: "short", width: 90, room: "  week 9.0 of 10 left · over 50% to spare", says: "▲ 2 more accounts · ▼ 2 more accounts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayFrame(t, 27, lanesOf(10), thingsComing(3))
			f.Width, f.Scroll = tt.width, 2
			f.Runway.Timeline = views.LiveTimeline(routedAt, f.Runway.Columns(tt.width))
			fit := f.runwayFitted(f.top())

			row := rowOf(runwayDrawn(f, forecastDoc(runwayDoc(10), 0.01)), fit.room)

			if !strings.HasPrefix(row, tt.room+"  ") || !strings.HasSuffix(row, " "+tt.says) || len([]rune(row)) != tt.width {
				t.Errorf("the room's row reads %q, want %q, then %q ending at the edge", row, tt.room, tt.says)
			}
		})
	}
}
