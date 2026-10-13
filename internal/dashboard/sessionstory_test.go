package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// storyAt is the time on the day of October 2026 given, at hour:minute:second
// UTC, the zone a story's times are said in.
func storyAt(day, hour, minute, second int) time.Time {
	return time.Date(2026, 10, day, hour, minute, second, 0, time.UTC)
}

// storyCost is a move's cost of the dollars given, in hundredths.
func storyCost(cents int64) *ledger.Picodollars {
	return new(ledger.Picodollars(cents * 10_000_000_000))
}

// capMove is a move off from, at its cap, to to at at, that wrote the
// tokens given again, at the cost given in hundredths of a dollar.
func capMove(from, to string, at time.Time, written int, cents int64) views.SessionMove {
	return views.SessionMove{At: at, Model: "claude-opus-5-5", From: from, To: to, Reason: "moved: " + from + " is at its reserve",
		Written: written, Cost: storyCost(cents), Rewrote: true}
}

// movedStory is d28c's story, as session-moved-160 and session-phone-52 draw
// it: from 12:02 to now, 14:42:07, moved from work to side at its cap at 14:31.
func movedStory() views.Story {
	return views.Story{Start: storyAt(8, 12, 2, 0), End: storyAt(8, 14, 42, 7), Sent: minutely(storyAt(8, 12, 2, 0), storyAt(8, 14, 41, 0)),
		Moves:     []views.SessionMove{capMove("work", "side", storyAt(8, 14, 31, 0), 182_000, 182)},
		Stretches: []views.Stretch{{Account: "work", From: storyAt(8, 12, 2, 0)}, {Account: "side", From: storyAt(8, 14, 31, 0)}}}
}

// quietStory is 9c2e's story, as session-quiet-160 draws it: on personal
// from 11:55 to now, 14:42:07.
func quietStory() views.Story {
	return views.Story{Start: storyAt(8, 11, 55, 0), End: storyAt(8, 14, 42, 7), Sent: minutely(storyAt(8, 11, 55, 0), storyAt(8, 14, 33, 0)),
		Stretches: []views.Stretch{{Account: "personal", From: storyAt(8, 11, 55, 0)}}}
}

// endedStory is b9c4's story, as session-ended-160 draws it: yesterday from
// 13:20 to its end at 18:47, moved from work to side at its cap at 16:02.
func endedStory() views.Story {
	return views.Story{Start: storyAt(7, 13, 20, 0), End: storyAt(7, 18, 47, 0), Ended: true, Sent: minutely(storyAt(7, 13, 20, 0), storyAt(7, 18, 47, 0)),
		Moves:     []views.SessionMove{capMove("work", "side", storyAt(7, 16, 2, 0), 188_000, 188)},
		Stretches: []views.Stretch{{Account: "work", From: storyAt(7, 13, 20, 0)}, {Account: "side", From: storyAt(7, 16, 2, 0)}}}
}

// resumedStory is f41a's story, as session-resumed-160 draws it: on side
// yesterday from 16:40 to 18:05, folded, then rescored after 15h 7m idle
// onto personal at 09:12, and on it to now, 14:42:07.
func resumedStory() views.Story {
	back := storyAt(8, 9, 12, 0)
	return views.Story{Start: storyAt(7, 16, 40, 0), End: storyAt(8, 14, 42, 7),
		Sent:      slices.Concat(minutely(storyAt(7, 16, 40, 0), storyAt(7, 18, 5, 0)), minutely(back, storyAt(8, 14, 41, 0))),
		Folds:     []views.Fold{{From: storyAt(7, 18, 5, 0), To: back}},
		Moves:     []views.SessionMove{{At: back, Model: "claude-opus-5-5", From: "side", To: "personal", Reason: "rescored after 15h 7m idle"}},
		Stretches: []views.Stretch{{Account: "side", From: storyAt(7, 16, 40, 0)}, {Account: "personal", From: back}}}
}

// storyFrame is a frame of the size given in the look given, its traces'
// scale pinned at 8, as the frames draw it.
func storyFrame(width, height int, look Look) Frame {
	f := frameOf(width, height)
	f.Look, f.TraceScale = look, 8
	return f
}

// storyDrawnOn is the story drawn on a canvas of f's size, in the rows f
// lays out for a session that ran on the accounts given, the turn given
// chosen.
func storyDrawnOn(f Frame, accounts int, story views.Story, chosen views.Turn) *canvas {
	c := newCanvas(f.Width, f.Height)
	f.sessionStory(c, f.sessionLayout(accounts), story, chosen)
	return c
}

// fillShades are the shades a story's columns are filled in, in the look
// given: accent.key as the traces mix it, and as the turn chosen's are lit.
func fillShades(look Look) []shade {
	fill, _ := traceInks(false)
	return []shade{shadeOf(look.colour(fill)), shadeOf(look.colour(ink{token: fill.token, fade: storyLit}))}
}

// columnOwn reports whether the cell is a column's own: blank, or filled,
// whole or by an eighth block, in one of the shades given, but the floor,
// which a column fills none of.
func columnOwn(s seen, canvas shade, fills []shade) bool {
	filled := func(c shade) bool { return slices.ContainsFunc(fills, c.near) }
	switch {
	case s.glyph == ' ':
		return !s.bg.set || s.bg.near(canvas) || filled(s.bg)
	case eighthsOf(s.glyph) > 0:
		return filled(s.fg)
	}
	return false
}

// eighthsOf are the eighths of a cell the glyph fills, as an eighth block:
// none where it's none.
func eighthsOf(glyph rune) int {
	return max(slices.Index(levels[:], string(glyph)), 0)
}

// columnEighths are the eighths each of a story's columns fills, width of
// them, read off the cells of its rows, top to bottom, from the bottom up: a
// cell filled whole counts eight, and the one above it its eighth block.
func columnEighths(cells [][]seen, top, bottom, width int, canvas shade, fills []shade) []int {
	eighths := make([]int, width)
	for x := range eighths {
		for y := bottom; y >= top; y-- {
			s := seenAt(cells, x, y)
			if s.glyph == ' ' && s.bg.set && !s.bg.near(canvas) {
				eighths[x] += 8
				continue
			}
			if columnOwn(s, canvas, fills) {
				eighths[x] += eighthsOf(s.glyph)
			}
			break
		}
	}
	return eighths
}

// sentLike are when the requests of the story were sent, for its columns,
// width cells wide, to fill where the frame's eighths given fill and nowhere
// else, each column's rows as the frame's as near as whole requests a minute
// come: each minute's requests as many as fill the rows of the column it
// counts most in, at 8 a minute filling them all; and none where it counts
// in a column the frame leaves empty.
func sentLike(story views.Story, width, rows int, eighths []int) []time.Time {
	var sent []time.Time
	for t := story.Start.Truncate(time.Minute); !t.After(story.End); t = t.Add(time.Minute) {
		probe := story
		probe.Sent = []time.Time{t}
		rates := probe.Columns(width).Rates
		most, empty := -1, false
		for i, r := range rates {
			if r == 0 {
				continue
			}
			empty = empty || eighths[i] == 0
			if most < 0 || r > rates[most] {
				most = i
			}
		}
		if most < 0 || empty {
			continue
		}
		for range (eighths[most] + 7) / 8 * 8 / rows {
			sent = append(sent, t)
		}
	}
	return sent
}

// The story is drawn as its frames draw it, its scale pinned at 8: the
// heading; the move's label at its marker, whole; the marker down the rows
// its column leaves empty; the floor under each column with no requests; a
// fold's ╱ ╱; the ribbon and the axis. Each column's own cells, which the
// frames fill by curves of their own, which whole requests a minute can't
// follow, are passed over, but each column fills where the frame's does.
func TestASessionsStoryIsDrawnAsItsFrames(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	canvas, fills := shadeOf(nord.Canvas()), fillShades(nord)
	tests := []struct {
		stem          string
		width, height int
		story         views.Story
		chosen        views.Turn
		// flagged are the cells the frame draws otherwise than views lays
		// the story out.
		flagged []spot
	}{
		{
			stem: "session-moved-160", width: 160, height: 37, story: movedStory(),
			chosen: views.Turn{Turn: 13, Started: storyAt(8, 14, 40, 0)},
		},
		{
			stem: "session-quiet-160", width: 160, height: 37, story: quietStory(),
			chosen: views.Turn{Turn: 10, Started: storyAt(8, 14, 29, 0), Ended: storyAt(8, 14, 32, 40)},
		},
		{
			stem: "session-ended-160", width: 160, height: 37, story: endedStory(),
			chosen: views.Turn{Turn: 16, Started: storyAt(7, 18, 36, 0), Ended: storyAt(7, 18, 46, 30)},
		},
		{
			// The router writes the idle as it is, 15h 7m, where the frame's
			// label rounds it to 15h. And the frame draws the fold's last
			// column, where the session came back, as its first minute back,
			// the marker stopping at it; views lays out a fold's five columns
			// with no requests, so the marker runs down to the floor.
			stem: "session-resumed-160", width: 160, height: 37, story: resumedStory(),
			chosen:  views.Turn{Turn: 21, Started: storyAt(8, 14, 25, 0)},
			flagged: []spot{{10, 64, 81}, {14, 36, 36}, {15, 36, 36}},
		},
		{
			stem: "session-phone-52", width: 52, height: 32, story: movedStory(),
			chosen: views.Turn{Turn: 13, Started: storyAt(8, 14, 40, 0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			// The wide frames' story is on rows 9 to 17, its columns 11 to
			// 15; the phone's on 10 to 16, its columns 11 to 14.
			in, top, bottom := region{top: 9, bottom: 17, right: tt.width - 1}, 11, 15
			if tt.width < sessionWideFrom {
				in, bottom = region{top: 10, bottom: 16, right: tt.width - 1}, 14
			}
			frame, story := reference(t, tt.stem), tt.story
			story.Sent = sentLike(story, tt.width, bottom-top+1, columnEighths(frame, top, bottom, tt.width, canvas, fills))
			drawn := screenOf(t, storyDrawnOn(storyFrame(tt.width, tt.height, nord), 1, story, tt.chosen).rows(nord))
			whole := slices.Clone(tt.flagged)
			for y := top; y <= bottom; y++ {
				for x := range tt.width {
					if columnOwn(seenAt(frame, x, y), canvas, fills) && columnOwn(seenAt(drawn, x, y), canvas, fills) {
						whole = append(whole, spot{y, x, x})
					}
				}
			}
			heldTo(t, tt.stem, frame, drawn, in, canvas, whole, nil)
			want, got := litColumns(frame, top, bottom, tt.width, fills), litColumns(drawn, top, bottom, tt.width, fills)
			for x := range tt.width {
				if want[x] != unfilled && got[x] != unfilled && want[x] != got[x] {
					t.Errorf("column %d is lit %t, the frame's %t", x, got[x] == lit, want[x] == lit)
				}
			}
		})
	}
}

// minutely are a request a minute from from to to, both included.
func minutely(from, to time.Time) []time.Time {
	var sent []time.Time
	for at := from; !at.After(to); at = at.Add(time.Minute) {
		sent = append(sent, at)
	}
	return sent
}

// times are n requests at at.
func times(n int, at time.Time) []time.Time {
	return slices.Repeat([]time.Time{at}, n)
}

// wideStory is the layout of a session's page with its story whole, wide,
// from the frame's first row: its heading on row 0, the label on 1, its
// columns from 2 to 6, the ribbon on 7 and the axis on 8.
var wideStory = sessionLayout{wide: true, story: true}

// twentyMinutes is a story 20 minutes old, laid across 10 columns two
// minutes each, but the last's three, with the requests given.
func twentyMinutes(sent ...[]time.Time) views.Story {
	return views.Story{Start: storyAt(8, 12, 0, 0), End: storyAt(8, 12, 20, 0), Sent: slices.Concat(sent...),
		Stretches: []views.Stretch{{Account: "work", From: storyAt(8, 12, 0, 0)}}}
}

// A story's columns fill round(rate ÷ scale × 40) eighths of their five
// rows, halves to even, its rate each column's mean requests a minute: at
// the scale the frame pins, else the busiest column's, never less than 8.
// A column with none has the floor, ▁ in text.faint, on its bottom row.
func TestAStorysColumnsFillEighthsOfItsRowsToItsScale(t *testing.T) {
	// Columns of two minutes: 1 request is half a request a minute, 3 one
	// and a half, 16 eight and 40 twenty.
	busy := twentyMinutes(times(1, storyAt(8, 12, 0, 0)), times(3, storyAt(8, 12, 2, 0)), times(16, storyAt(8, 12, 4, 0)), times(40, storyAt(8, 12, 6, 0)))
	quiet := twentyMinutes(times(1, storyAt(8, 12, 0, 0)), times(3, storyAt(8, 12, 2, 0)))
	tests := []struct {
		name  string
		story views.Story
		scale float64
		want  []int
	}{
		{name: "pinned at 8: 2.5 to 2, 7.5 to 8, eight a minute full", story: busy, scale: 8, want: []int{2, 8, 40, 40, 0, 0, 0, 0, 0, 0}},
		{name: "live, the busiest twenty a minute", story: busy, want: []int{1, 3, 16, 40, 0, 0, 0, 0, 0, 0}},
		{name: "live, never less than 8", story: quiet, want: []int{2, 8, 0, 0, 0, 0, 0, 0, 0, 0}},
	}
	_, lane := traceInks(false)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := storyFrame(10, 9, Screen(builtin(t, "nord")))
			f.TraceScale = tt.scale
			c := newCanvas(10, 9)
			f.sessionStory(c, wideStory, tt.story, views.Turn{})
			if got := tracedOf(c, 0, 10, 6, 5, 4, 3, 2); !slices.Equal(got, tt.want) {
				t.Errorf("the columns fill %v eighths, want %v", got, tt.want)
			}
			for x, eighths := range tt.want {
				if floor := *c.at(x, 6); eighths == 0 && floor != (cell{glyph: traceLane, ink: lane}) {
					t.Errorf("column %d, with no requests, has %+v on its bottom row, want the floor", x, floor)
				}
			}
		})
	}
}

// The turn chosen is lit, its columns in accent.key mixed with the canvas so
// 95% of it shows, from its first request's to its end's, or the newest
// while it's going; the rest as the traces are, 80%; none where no turn is
// chosen. A column with no requests keeps its floor, unlit.
func TestTheTurnChosensColumnsAreLit(t *testing.T) {
	// Columns 5 to 7, from 12:10 to 12:15, have no requests.
	story := twentyMinutes(minutely(storyAt(8, 12, 0, 0), storyAt(8, 12, 9, 0)), minutely(storyAt(8, 12, 16, 0), storyAt(8, 12, 20, 0)))
	none := []int{5, 6, 7}
	tests := []struct {
		name   string
		chosen views.Turn
		lit    []int
	}{
		{name: "none chosen"},
		{name: "ended", chosen: views.Turn{Turn: 2, Started: storyAt(8, 12, 2, 0), Ended: storyAt(8, 12, 7, 30)}, lit: []int{1, 2, 3}},
		{name: "going, to the newest, past a column with none", chosen: views.Turn{Turn: 3, Started: storyAt(8, 12, 14, 0)}, lit: []int{7, 8, 9}},
	}
	_, lane := traceInks(false)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(10, 9)
			storyFrame(10, 9, Screen(builtin(t, "nord"))).sessionStory(c, wideStory, story, tt.chosen)
			for x := range 10 {
				bottom, want := c.at(x, 6).ink, traceFade
				if slices.Contains(tt.lit, x) {
					want = storyLit
				}
				empty := slices.Contains(none, x)
				switch {
				case empty && bottom != lane:
					t.Errorf("column %d, with no requests, is drawn in %+v, want its floor's", x, bottom)
				case !empty && bottom.fade != want:
					t.Errorf("column %d is mixed %v into the canvas, want %v", x, bottom.fade, want)
				}
			}
		})
	}
}

// movesStory is a story 160 minutes old, a minute a column from 12:00, its
// session moved at its cap at each time given, from work to side, then on to
// personal.
func movesStory(at ...time.Time) views.Story {
	s := views.Story{Start: storyAt(8, 12, 0, 0), End: storyAt(8, 14, 40, 30), Sent: minutely(storyAt(8, 12, 0, 0), storyAt(8, 14, 40, 0))}
	accounts := []string{"work", "side", "personal"}
	for i, t := range at {
		s.Moves = append(s.Moves, capMove(accounts[i], accounts[i+1], t, 182_000, 182))
		s.Moves[i].Rewrote = false
	}
	return s
}

// A move's label, on the row over the columns, ends at its marker: what
// moved it and where to, the tokens it wrote again and their worth where its
// cache hadn't run out, two cells, its time, a cell and the marker. With no
// room at the marker's left it starts at the marker, its time first. Too
// long for either side, it says the move's names alone, cut as moveNames
// cuts them, on the side with more room. The newest is placed first; an
// older one ends two cells before the label to its right, cut to fit, and is
// left out under 12 cells, or where a newer covers its marker, its marker
// drawn all the same.
func TestAMovesLabelEndsAtItsMarker(t *testing.T) {
	unpriced := movedStory()
	unpriced.Moves[0].Cost = nil
	tests := []struct {
		name  string
		width int
		phone bool
		story views.Story
		want  string
	}{
		{
			name: "whole, at its marker's left", width: 160, story: movedStory(),
			want: strings.Repeat(" ", 87) + "work reached its cap → side · 182k cached again, $1.82  14:31 ┊",
		},
		{
			name: "the price table can't price what it wrote again", width: 160, story: unpriced,
			want: strings.Repeat(" ", 84) + "work reached its cap → side · 182k cached again, unpriced  14:31 ┊",
		},
		{
			name: "no room at its marker's left: from it, its time first", width: 160, story: resumedStory(),
			want: strings.Repeat(" ", 36) + "┊ 09:12  rescored after 15h 7m idle → personal",
		},
		{
			name: "on a phone, short", width: 52, phone: true, story: movedStory(),
			want: strings.Repeat(" ", 19) + "its cap → side, $1.82  14:31 ┊",
		},
		{
			name: "too long for either side: its names", width: 52, phone: true, story: resumedStory(),
			want: strings.Repeat(" ", 13) + "┊ 09:12  side → personal",
		},
		{
			name: "its cache run out: no cost; an older one cut two cells before the next", width: 160,
			story: movesStory(storyAt(8, 12, 15, 0), storyAt(8, 12, 20, 0)),
			want:  "w… → s…  12:15 ┊    ┊ 12:20  side reached its cap → personal",
		},
		{
			name: "an older one under 12 cells cut: left out", width: 160,
			story: movesStory(storyAt(8, 12, 10, 0), storyAt(8, 12, 20, 0)),
			want:  strings.Repeat(" ", 10) + "┊         ┊ 12:20  side reached its cap → personal",
		},
		{
			name: "an older one under a newer: left out", width: 160,
			story: movesStory(storyAt(8, 13, 0, 0), storyAt(8, 13, 20, 0)),
			want:  strings.Repeat(" ", 41) + "side reached its cap → personal  13:20 ┊",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, row := wideStory, 1
			if tt.phone {
				layout, row = sessionLayout{story: true}, 0
			}
			c := newCanvas(tt.width, 9)
			storyFrame(tt.width, 9, Look{}).sessionStory(c, layout, tt.story, views.Turn{})
			if got := c.row(row, Look{}); got != tt.want {
				t.Errorf("the label's row reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// STORY folds to its heading where the page hasn't room for it whole, as
// the Overview folds a section, saying so in text.subtle, its rule to two
// cells short of the edge; on a phone it's left out, and under the chrome
// alone nothing is drawn.
func TestAStoryWithoutRoomFoldsToItsHeading(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		want          map[int]string
	}{
		{
			name: "wide, 24 rows: its heading alone", width: 160, height: 24,
			want: map[int]string{9: "STORY   no room at 24 rows " + strings.Repeat("─", 131)},
		},
		{name: "a phone, 24 rows: left out", width: 52, height: 24},
		{name: "wide, 20 rows: the chrome alone", width: 160, height: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := storyFrame(tt.width, tt.height, Look{})
			c := storyDrawnOn(f, 1, movedStory(), views.Turn{})
			for y := range tt.height {
				if got := c.row(y, Look{}); got != tt.want[y] {
					t.Errorf("row %d reads %q, want %q", y, got, tt.want[y])
				}
			}
		})
	}
}

// How a column of a story is filled: not at all, in accent.key as the
// traces mix it, or lit as the turn chosen's are.
const (
	unfilled = iota
	filled
	lit
)

// litColumns are how each of a story's columns, width of them, is filled,
// read off the cells of its rows, top to bottom, the turn chosen's lit in the
// second of the fills given.
func litColumns(cells [][]seen, top, bottom, width int, fills []shade) []int {
	columns := make([]int, width)
	for x := range columns {
		for y := top; y <= bottom; y++ {
			s := seenAt(cells, x, y)
			switch {
			case s.bg.near(fills[1]) || s.glyph != ' ' && s.fg.near(fills[1]):
				columns[x] = lit
			case s.bg.near(fills[0]) || s.glyph != ' ' && s.fg.near(fills[0]):
				columns[x] = filled
			}
		}
	}
	return columns
}

// youngStory is a story 20 minutes old, on work from 12:05 to now, 12:25:30.
func youngStory() views.Story {
	return views.Story{Start: storyAt(8, 12, 5, 0), End: storyAt(8, 12, 25, 30), Sent: minutely(storyAt(8, 12, 5, 0), storyAt(8, 12, 25, 0)),
		Stretches: []views.Stretch{{Account: "work", From: storyAt(8, 12, 5, 0)}}}
}

// foldedStory is a story folded 32 times: 33 bursts on work, of a request a
// minute for three minutes, an hour and a minute apart, from midnight to
// now, 30 seconds after its last request.
func foldedStory() views.Story {
	var s views.Story
	for b := range 33 {
		at := storyAt(8, 0, 0, 0).Add(time.Duration(63*b) * time.Minute)
		s.Sent = append(s.Sent, minutely(at, at.Add(2*time.Minute))...)
		s.Stretches = append(s.Stretches, views.Stretch{Account: "work", From: at})
		if b > 0 {
			s.Folds = append(s.Folds, views.Fold{From: at.Add(-61 * time.Minute), To: at})
		}
	}
	s.Start, s.End = s.Sent[0], s.Sent[len(s.Sent)-1].Add(30*time.Second)
	return s
}

// glyphsAt are the glyphs of the n cells from x along row y.
func glyphsAt(c *canvas, x, y, n int) string {
	var b strings.Builder
	for i := range n {
		if here := c.at(x+i, y); here != nil {
			b.WriteString(here.glyph)
		}
	}
	return b.String()
}

// A story is drawn as views lays it out across the width, wide from 122
// columns and on a phone's rows narrower, whatever its moment: each column
// filled its eighths, at 8 requests a minute filling them all, else its
// floor, but under a marker; none past its newest; a fold's middle three ╱ ╱
// on every row; each move's marker on the label's row and down every row
// its column leaves empty, its time beside it; each stretch's ends on the
// ribbon; and each of the axis's labels at its column.
func TestAStoryIsDrawnAsItsColumnsLayItOutAtEachWidth(t *testing.T) {
	moments := map[string]views.Story{
		"a fold": resumedStory(), "a move": movedStory(), "young": youngStory(), "32 folds merging": foldedStory(),
	}
	for _, width := range []int{52, 80, 100, 160, 200} {
		for name, story := range moments {
			t.Run(strconv.Itoa(width)+", "+name, func(t *testing.T) {
				f := storyFrame(width, 37, Look{})
				l := f.sessionLayout(1)
				c := storyDrawnOn(f, 1, story, views.Turn{})
				cols := story.Columns(width)
				tall, top := phoneStoryColumns, l.storyAt
				if l.wide {
					tall, top = wideStoryColumns, l.storyAt+1
				}
				if len(cols.Rates) == 0 {
					t.Fatal("views lays out no columns")
				}
				d := storyLaid{StoryColumns: cols, end: story.End, label: top, floor: top + tall}
				d.columnsDrawn(t, c)
				d.marked(t, c)
				d.ribbonAndAxis(t, c)
			})
		}
	}
}

// storyLaid is a story as views lays it out across a width, its end, and
// the rows it's drawn on: its label's, and its columns' bottom row, the
// ribbon and the axis under it.
type storyLaid struct {
	views.StoryColumns
	end          time.Time
	label, floor int
}

// eighths are the eighths each column drawn on c fills, read off its cells.
func (d storyLaid) eighths(c *canvas) []int {
	var up []int
	for y := d.floor; y > d.label; y-- {
		up = append(up, y)
	}
	return tracedOf(c, 0, c.width, up...)
}

// columnsDrawn holds the columns drawn on c to the story's: each its
// eighths, at 8 a minute filling its rows, its floor where it fills none,
// but a marker's; a fold's middle three ╱ ╱ on every row; and nothing past
// the newest.
func (d storyLaid) columnsDrawn(t *testing.T, c *canvas) {
	t.Helper()
	eighths, rows := d.eighths(c), d.floor-d.label
	marked := map[int]bool{}
	for _, m := range d.Moves {
		marked[m.Column] = true
	}
	for x := range c.width {
		middle := slices.IndexFunc(d.Folds, func(first int) bool { return x > first && x < first+4 })
		switch {
		case x >= len(d.Rates):
			if got := glyphsAt(c, x, d.label+1, 1); got != " " {
				t.Errorf("column %d, past the newest, has %q", x, got)
			}
		case middle >= 0:
			want := string([]rune(foldDrawn)[x-d.Folds[middle]-1])
			for y := d.label + 1; y <= d.floor; y++ {
				if got := glyphsAt(c, x, y, 1); got != want {
					t.Errorf("fold column %d has %q on row %d, want %q", x, got, y, want)
				}
			}
		case eighths[x] != halfEven(min(d.Rates[x]/8, 1)*float64(rows*8)):
			t.Errorf("column %d fills %d eighths at %v a minute", x, eighths[x], d.Rates[x])
		case eighths[x] == 0 && !marked[x] && glyphsAt(c, x, d.floor, 1) != traceLane:
			t.Errorf("column %d, with no requests, has %q on its bottom row, want its floor", x, glyphsAt(c, x, d.floor, 1))
		}
	}
}

// marked holds each move's marker drawn on c: on the label's row, where a
// newer label doesn't cover it, and down every row its column leaves empty;
// and the newest's time beside it.
func (d storyLaid) marked(t *testing.T, c *canvas) {
	t.Helper()
	eighths := d.eighths(c)
	for _, m := range d.Moves {
		if glyphsAt(c, m.Column, d.label, 1) == " " {
			t.Errorf("the move at column %d has nothing on the label's row", m.Column)
		}
		for y := d.label + 1; y <= d.floor-(eighths[m.Column]+7)/8; y++ {
			if got := glyphsAt(c, m.Column, y, 1); got != moveMarker {
				t.Errorf("the move at column %d has %q on row %d, its column empty", m.Column, got, y)
			}
		}
	}
	if len(d.Moves) == 0 {
		return
	}
	newest := d.Moves[len(d.Moves)-1]
	at := prose.TimeOfDay(d.end, newest.At)
	if glyphsAt(c, newest.Column-6, d.label, 7) != at+" "+moveMarker && glyphsAt(c, newest.Column, d.label, 7) != moveMarker+" "+at {
		t.Errorf("the newest move's label doesn't have its time, %s, beside its marker at %d", at, newest.Column)
	}
}

// ribbonAndAxis holds the ribbon and the axis drawn on c: each stretch's
// ends, and each of the axis's labels at its column.
func (d storyLaid) ribbonAndAxis(t *testing.T, c *canvas) {
	t.Helper()
	for _, s := range d.Stretches {
		if got := glyphsAt(c, s.First, d.floor+1, 1) + glyphsAt(c, s.Last, d.floor+1, 1); got != stretchStart+stretchEnd {
			t.Errorf("the stretch on %s from %d to %d ends in %q", s.Account, s.First, s.Last, got)
		}
	}
	for _, a := range d.Axis {
		if got := glyphsAt(c, a.Column, d.floor+2, len([]rune(a.Text))); got != a.Text {
			t.Errorf("the axis has %q at column %d, want %q", got, a.Column, a.Text)
		}
	}
}
