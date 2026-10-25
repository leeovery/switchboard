package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// stackColumns are the columns of the stacks POOL's tests draw, an hour
// each from 08:00 on 1 October 2026.
const stackColumns = 10

// stackTimeline is a timeline of stackColumns hours from 08:00 on 1 October
// 2026, now in the column given, or none where it's negative.
func stackTimeline(now int) views.Timeline {
	start := clockOn(8, 0)
	tl := views.Timeline{Start: start, End: start.Add(stackColumns * time.Hour), Columns: stackColumns}
	if now >= 0 {
		tl.Now = start.Add(time.Duration(now)*time.Hour + 30*time.Minute)
	}
	return tl
}

// held is an account's room, known, the same in every column.
func held(room float64) []views.AccountRoom {
	rooms := make([]views.AccountRoom, stackColumns)
	for c := range rooms {
		rooms[c] = views.AccountRoom{Room: room, Known: true}
	}
	return rooms
}

// stackOf is a stack over stackTimeline(now) of the accounts whose rooms are
// given, in turn, named work, personal, side, then a3 on, by their ids.
func stackOf(now int, rooms ...[]views.AccountRoom) views.PoolStack {
	ids := []string{"work", "personal", "side"}
	s := views.PoolStack{Timeline: stackTimeline(now), Accounts: make([]views.PoolStackAccount, len(rooms))}
	for i, r := range rooms {
		id := "a" + string(rune('0'+i))
		if i < len(ids) {
			id = ids[i]
		}
		s.Accounts[i] = views.PoolStackAccount{ID: id, Rooms: r}
	}
	return s
}

// heldStack is a stack, now in the last column, of accounts each holding
// the room given.
func heldStack(rooms ...float64) views.PoolStack {
	return heldStackAt(stackColumns-1, rooms...)
}

// heldStackAt is a stack, now in the column given, of accounts each holding
// the room given.
func heldStackAt(now int, rooms ...float64) views.PoolStack {
	each := make([][]views.AccountRoom, len(rooms))
	for i, r := range rooms {
		each[i] = held(r)
	}
	return stackOf(now, each...)
}

// poolIn is the stack drawn in look, width cells wide and its chart h rows
// tall, from the top left of a canvas of its own as tall as it takes.
func poolIn(s views.PoolStack, look Look, width, h int) *canvas {
	p := poolStacked{stack: s, look: look}
	c := newCanvas(width, p.rows(width, h))
	p.draw(c, 0, 0, width, h)
	return c
}

// stackCells are the cells of column x of a chart h rows tall on c, from
// its foot up.
func stackCells(c *canvas, x, h int) []cell {
	cells := make([]cell, h)
	for r := range h {
		cells[r] = *c.at(x, h-1-r)
	}
	return cells
}

// stackFilled are the eighths column x of a chart h rows tall on c is
// filled to, read back from its cells: a cell on a surface counts eight, as
// a full one does, and one with an eighth block its eighths.
func stackFilled(c *canvas, x, h int) int {
	filled := 0
	for _, here := range stackCells(c, x, h) {
		switch i := slices.Index(levels[:], here.glyph); {
		case here.ink.on != (hue{}):
			filled += 8
		case i > 0:
			filled += i
		}
	}
	return filled
}

// hueAt is the hue of the account at place i, from 0, as POOL draws it.
func hueAt(i int) hue {
	return hue{token: theme.VizAccount1 + theme.Token(i)}
}

func TestPoolStacksEachAccountsRoomFromTheFootInItsHue(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	full := func(h hue) cell { return cell{glyph: " ", ink: ink{on: h}} }
	block := func(n int, h hue) cell { return cell{glyph: levels[n], ink: h.ink()} }
	meet := func(n int, lower, upper hue) cell {
		k := lower.ink()
		k.on = upper
		return cell{glyph: levels[n], ink: k}
	}
	tests := []struct {
		name  string
		rooms []float64
		h     int
		want  []cell
	}{
		{name: "one account, full", rooms: []float64{1}, h: 2, want: []cell{full(hueAt(0)), full(hueAt(0))}},
		{name: "one account, none left", rooms: []float64{0}, h: 2, want: []cell{blank, blank}},
		// round(0.07 × 8) = 1, round(0.05 × 8) = 0.
		{name: "a sliver: an eighth", rooms: []float64{0.07}, h: 1, want: []cell{block(1, hueAt(0))}},
		{name: "less than a sliver: nothing", rooms: []float64{0.05}, h: 1, want: []cell{blank}},
		// Of two over two rows, each of the 16 eighths is an eighth of one
		// account's room: work's 8, personal's 4.
		{name: "two accounts", rooms: []float64{1, 0.5}, h: 2, want: []cell{full(hueAt(0)), block(4, hueAt(1))}},
		{name: "three accounts, full", rooms: []float64{1, 1, 1}, h: 3, want: []cell{full(hueAt(0)), full(hueAt(1)), full(hueAt(2))}},
		// Each of four over two rows is 4 eighths: two meet in each cell.
		{name: "four accounts, two meeting in each cell", rooms: []float64{1, 1, 1, 1}, h: 2, want: []cell{meet(4, hueAt(0), hueAt(1)), meet(4, hueAt(2), hueAt(3))}},
		// 2, 3 and 5 eighths of 16: the first cell holds all three, the
		// middle giving way, and the top's 2 the next.
		{name: "three meeting in a cell", rooms: []float64{0.375, 0.5625, 0.9375}, h: 2, want: []cell{meet(2, hueAt(0), hueAt(2)), block(2, hueAt(2))}},
		// 1 and 2 eighths, the stack's top in the cell: its 3, in the hue
		// with the most of them.
		{name: "two meeting at the stack's top", rooms: []float64{0.25, 0.5}, h: 1, want: []cell{block(3, hueAt(1))}},
		{name: "two meeting at the stack's top, even", rooms: []float64{0.25, 0.25}, h: 1, want: []cell{block(2, hueAt(0))}},
		// Each of two full over a row is 4 eighths, the cell full.
		{name: "two full in a row", rooms: []float64{1, 1}, h: 1, want: []cell{meet(4, hueAt(0), hueAt(1))}},
		// 1 − (1 − 0.05) is 0.050000000000000044: of four over five rows,
		// 0.5 eighths settled, a half to even, none; unsettled, one.
		{name: "a room worked out by subtraction, settled", rooms: []float64{less(1, less(1, 0.05)), 0, 0, 0}, h: 5, want: []cell{blank, blank, blank, blank, blank}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := poolIn(heldStack(tt.rooms...), nord, stackColumns, tt.h)
			if got := stackCells(c, 0, tt.h); !slices.Equal(got, tt.want) {
				t.Errorf("the column reads, from its foot, %+v; want %+v", got, tt.want)
			}
		})
	}
}

// However the accounts' eighths meet, the column is filled to their sum,
// each account's round(room × 8h / n), no cell counted twice or lost, to the
// chart's top at the most.
func TestPoolsColumnsAreFilledToTheSumOfTheAccountsEighths(t *testing.T) {
	tests := []struct {
		rooms []float64
		h     int
		want  int
	}{
		{rooms: []float64{0.3, 0.7, 0.2}, h: 5, want: 4 + 9 + 3},
		{rooms: []float64{0.9, 0.05, 0.45, 0.6}, h: 3, want: 5 + 0 + 3 + 4},
		{rooms: []float64{0.1, 0.1, 0.1, 0.1, 0.1}, h: 7, want: 5 * 1},
		{rooms: []float64{1, 1, 1}, h: 1, want: 8},
		{rooms: []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5}, h: 19, want: 6 * 13},
	}
	for _, slug := range []string{"nord", "terminal"} {
		look := Screen(builtin(t, slug))
		for _, tt := range tests {
			// Column 0 before now, and column 1 ahead of it.
			for col, now := range []int{stackColumns - 1, 0} {
				if got := stackFilled(poolIn(heldStackAt(now, tt.rooms...), look, stackColumns, tt.h), col, tt.h); got != tt.want {
					t.Errorf("in %s, rooms %v over %d rows fill column %d, now in %d, to %d eighths, want %d", slug, tt.rooms, tt.h, col, now, got, tt.want)
				}
			}
		}
	}
}

func TestAnAccountsHueIsByItsPlaceAmongThePoolsFadedFromTheFifth(t *testing.T) {
	older := func(tok theme.Token) hue { return hue{token: tok, fade: theme.OlderFade} }
	want := []hue{
		hueAt(0), hueAt(1), hueAt(2), hueAt(3),
		older(theme.VizAccount1), older(theme.VizAccount2), older(theme.VizAccount3), older(theme.VizAccount4),
		older(theme.VizAccount1),
	}
	for i, w := range want {
		if got := accountHue(i); got != w {
			t.Errorf("the account at place %d is in %+v, want %+v", i+1, got, w)
		}
	}
	c := poolIn(heldStack(1, 1, 1, 1, 1), Screen(builtin(t, "nord")), stackColumns, 5)
	if got := c.at(0, 0).ink.on; got != older(theme.VizAccount1) {
		t.Errorf("the fifth account's stack is in %+v, want viz.account.1 faded", got)
	}
}

// Before now's column, the stack is as it is; from the column after now's,
// faded so half shows; and down now's column, │ in text.primary through
// every row, wherever now falls.
func TestPoolsStackFadesAheadOfNowsLineDownItsColumn(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	const h = 2
	for _, now := range []int{0, 4, stackColumns - 1} {
		c := poolIn(stackOf(now, held(1), held(1), held(1), held(1), held(1)), nord, stackColumns, h)
		for col := range stackColumns {
			cells := stackCells(c, col, h)
			switch {
			case col == now:
				for r, got := range cells {
					if want := (cell{glyph: nowLine, ink: poolNowInk}); got != want {
						t.Errorf("now in column %d: row %d up of its column is %+v, want %+v", now, r, got, want)
					}
				}
			case col < now:
				if got := cells[0].ink.fade; got != 0 {
					t.Errorf("now in column %d: column %d's foot is faded %v, want as it is", now, col, got)
				}
			default:
				if got := cells[0].ink; got.fade != aheadFade || got.on.fade != aheadFade || !got.dim {
					t.Errorf("now in column %d: column %d's foot is %+v, want faded %v, on its upper faded as much", now, col, got, aheadFade)
				}
			}
		}
	}
	ahead := poolIn(stackOf(0, held(1), held(1), held(1), held(1), held(1)), nord, stackColumns, 5)
	if got, want := ahead.at(1, 0).ink.on.fade, 1-(1-theme.OlderFade)*(1-aheadFade); got != want {
		t.Errorf("the fifth account's stack ahead is faded %v, want %v, its own and the heading's", got, want)
	}
}

// Under the chart, Runway's week's axis over the stack's timeline, cell for
// cell as Runway draws it over the same.
func TestPoolsAxisIsRunwaysOverItsTimeline(t *testing.T) {
	shown := runwayLive(views.RunwayWeek)
	const h, width = 4, 136
	runway := newCanvas(width, runwayAxisRows)
	runwayShowing(shown).runwayAxis(runway, runwayLaid{fit: runwayFit{fits: true, axis: 0}})
	s := views.PoolStack{Timeline: shown.Timeline, Accounts: []views.PoolStackAccount{{ID: "work", Rooms: make([]views.AccountRoom, width)}}}
	c := poolIn(s, Screen(builtin(t, "nord")), width, h)
	for y := range runwayAxisRows {
		for x := range width {
			if got, want := *c.at(x, h+y), *runway.at(x, y); got != want {
				t.Fatalf("the axis's row %d, column %d, is %+v; Runway's is %+v", y, x, got, want)
			}
		}
	}
}

// The key: each account's █ in its hue and its name, muted, as the
// dashboard names it, cleaned; a █ faded as the stack ahead is and what
// that means; and │ and now; four apart, flowed onto as many rows as the
// width takes, an item never broken.
func TestPoolsKeyNamesEachAccountsHueFlowedOntoTheRowsItTakes(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	s := stackOf(4, held(1), held(1))
	s.Accounts[1].ID, s.Accounts[1].Label = "a1", "per\tsonal  \x00label"
	tests := []struct {
		width int
		want  []string
	}{
		{width: 80, want: []string{"█ work    █ per sonal label    █ faint: heading, at its recent rate    │ now"}},
		{width: 70, want: []string{"█ work    █ per sonal label    █ faint: heading, at its recent rate", "│ now"}},
		{width: 50, want: []string{"█ work    █ per sonal label", "█ faint: heading, at its recent rate    │ now"}},
		{width: 40, want: []string{"█ work    █ per sonal label", "█ faint: heading, at its recent rate", "│ now"}},
		{width: 12, want: []string{"█ work", "█ per sonal label", "█ faint: heading, at its recent rate", "│ now"}},
	}
	for _, tt := range tests {
		p := poolStacked{stack: s, look: look}
		var got []string
		for _, l := range p.key(tt.width) {
			var b strings.Builder
			l.draw(&b, Look{})
			got = append(got, b.String())
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("at %d columns the key reads %q, want %q", tt.width, got, tt.want)
		}
		if rows := p.keyRows(tt.width); rows != len(tt.want) {
			t.Errorf("at %d columns the key takes %d rows, want %d", tt.width, rows, len(tt.want))
		}
	}
	key := poolStacked{stack: s, look: look}.key(80)[0]
	inks := []ink{hueAt(0).ink(), mutedInk, {}, hueAt(1).ink(), mutedInk, {}, hueAt(1).fadedBy(aheadFade).ink(), mutedInk, {}, poolNowInk, mutedInk}
	for i, k := range inks {
		if key[i].ink != k {
			t.Errorf("the key's span %d, %q, is in %+v, want %+v", i, key[i].text, key[i].ink, k)
		}
	}
	// Each account's █ is the item's first span, three spans to an item with
	// its name and the gap after it: the fifth's is viz.account.1 faded.
	five := poolStacked{stack: heldStack(1, 1, 1, 1, 1), look: look}.key(200)[0]
	if got, want := five[4*3].ink, (hue{token: theme.VizAccount1, fade: theme.OlderFade}).ink(); got != want {
		t.Errorf("the fifth account's █ in the key is in %+v, want %+v", got, want)
	}
}

// Without colour, the accounts can't be told apart: the stack is the pool's
// total alone, in █, its eighths the accounts' summed, and the key says so.
func TestPoolWithoutColourDrawsThePoolsTotalAlone(t *testing.T) {
	const h = 2
	c := poolIn(stackOf(9, held(1), held(0.5), held(0.25)), NoColour(), 60, h)
	// 16 eighths of three: round(5.33) + round(2.67) + round(1.33) = 9.
	want := []cell{{glyph: levels[8]}, {glyph: levels[1]}}
	if got := stackCells(c, 0, h); !slices.Equal(got, want) {
		t.Errorf("the column reads, from its foot, %+v; want %+v", got, want)
	}
	if key, want := rowOf(c, h+runwayAxisRows), "█ the pool's room; its accounts by colour alone    │ now"; key != want {
		t.Errorf("the key reads %q, want %q", key, want)
	}
}

// POOL draws nothing outside the width and rows it's given, nor anything
// where its stack has no columns.
func TestPoolDrawsWithinItsBounds(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	s := stackOf(4, held(1), held(1))
	p := poolStacked{stack: s, look: look}
	c := newCanvas(stackColumns+4, p.rows(6, 3)+2)
	p.draw(c, 2, 1, 6, 3)
	for y := range c.height() {
		for x := range c.width {
			inside := x >= 2 && x < 8 && y >= 1 && y < 1+p.rows(6, 3)
			if here := *c.at(x, y); !inside && here != blank {
				t.Errorf("POOL drew %+v at %d, %d, outside its bounds", here, x, y)
			}
		}
	}
	empty := newCanvas(stackColumns, 8)
	poolStacked{look: look}.draw(empty, 0, 0, stackColumns, 3)
	for y := range empty.height() {
		if row := rowOf(empty, y); row != "" {
			t.Errorf("a stack without columns drew %q on row %d", row, y)
		}
	}
}

// In a look that doesn't blend, which paints no ground, a cell where parts
// meet is █ alone, in the hue of the part with the most of it, the lowest
// of those even, faded ahead of now as the stack is: never on a surface.
func TestPoolsMeetingCellsAreAGlyphAloneWhereTheLookDoesntBlend(t *testing.T) {
	terminal := Screen(builtin(t, "terminal"))
	tests := []struct {
		name  string
		rooms []float64
		h     int
		want  []hue
	}{
		// 4 eighths each, even: the lower's hue.
		{name: "two meeting", rooms: []float64{1, 1}, h: 1, want: []hue{hueAt(0)}},
		// Of four over three rows, 6 eighths a full one's: work's 6,
		// personal's 1 and side's 1 meet in the first cell; side's 2 more
		// and the fourth's 6 in the second, the fourth's the most.
		{name: "the fourth on top", rooms: []float64{1, 1.0 / 6, 0.5, 1}, h: 3, want: []hue{hueAt(0), hueAt(3)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for col, now := range []int{stackColumns - 1, 0} {
				c := poolIn(heldStackAt(now, tt.rooms...), terminal, stackColumns, tt.h)
				for r, h := range tt.want {
					if col > now {
						h = h.fadedBy(aheadFade)
					}
					if got, want := stackCells(c, col, tt.h)[r], (cell{glyph: levels[8], ink: h.ink()}); got != want {
						t.Errorf("column %d, now in %d: row %d up is %+v, want %+v", col, now, r, got, want)
					}
				}
			}
		})
	}
}
