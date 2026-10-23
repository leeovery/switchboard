package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
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
	return views.Story{Start: storyAt(8, 12, 2, 0), End: storyAt(8, 14, 42, 7), Sent: views.SentAt(minutely(storyAt(8, 12, 2, 0), storyAt(8, 14, 41, 0))...),
		Moves:     []views.SessionMove{capMove("work", "side", storyAt(8, 14, 31, 0), 182_000, 182)},
		Stretches: []views.Stretch{{Account: "work", From: storyAt(8, 12, 2, 0)}, {Account: "side", From: storyAt(8, 14, 31, 0)}}}
}

// resumedStory is f41a's story, as session-resumed-160 draws it: on side
// yesterday from 16:40 to 18:05, folded, then rescored after 15h 7m idle
// onto personal at 09:12, and on it to now, 14:42:07.
func resumedStory() views.Story {
	back := storyAt(8, 9, 12, 0)
	return views.Story{Start: storyAt(7, 16, 40, 0), End: storyAt(8, 14, 42, 7),
		Sent:      views.SentAt(slices.Concat(minutely(storyAt(7, 16, 40, 0), storyAt(7, 18, 5, 0)), minutely(back, storyAt(8, 14, 41, 0)))...),
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
	f.sessionStory(c, status.Moment{}, f.layoutOf(accounts), story, chosen)
	return c
}

// minutely are a request a minute from from to to, both included.
func minutely(from, to time.Time) []time.Time {
	var sent []time.Time
	for at := from; !at.After(to); at = at.Add(time.Minute) {
		sent = append(sent, at)
	}
	return sent
}

// sentAt are n requests sent at at.
func sentAt(n int, at time.Time) []time.Time {
	return slices.Repeat([]time.Time{at}, n)
}

// wideStory is the layout of a session's page with its story whole, wide,
// from the frame's first row: its heading on row 0, the label on 1, its
// columns from 2 to 6, the ribbon on 7 and the axis on 8.
var wideStory = sessionLayout{wide: true, story: true, body: storyColumnRows}

// twentyMinutes is a story 20 minutes old, laid across 10 columns two
// minutes each, but the last's three, with the requests given.
func twentyMinutes(sent ...[]time.Time) views.Story {
	return views.Story{Start: storyAt(8, 12, 0, 0), End: storyAt(8, 12, 20, 0), Sent: views.SentAt(slices.Concat(sent...)...),
		Stretches: []views.Stretch{{Account: "work", From: storyAt(8, 12, 0, 0)}}}
}

// A story's columns fill round(rate ÷ scale × 40) eighths of their five
// rows, halves to even, its rate each column's mean requests a minute: at
// the scale the frame pins, else the busiest column's, never less than 8.
// A column with none has the floor, ▁ in text.faint, on its bottom row.
func TestAStorysColumnsFillEighthsOfItsRowsToItsScale(t *testing.T) {
	// Columns of two minutes: 1 request is half a request a minute, 3 one
	// and a half, 16 eight and 40 twenty.
	busy := twentyMinutes(sentAt(1, storyAt(8, 12, 0, 0)), sentAt(3, storyAt(8, 12, 2, 0)), sentAt(16, storyAt(8, 12, 4, 0)), sentAt(40, storyAt(8, 12, 6, 0)))
	quiet := twentyMinutes(sentAt(1, storyAt(8, 12, 0, 0)), sentAt(3, storyAt(8, 12, 2, 0)))
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
			f.sessionStory(c, status.Moment{}, wideStory, tt.story, views.Turn{})
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
			storyFrame(10, 9, Screen(builtin(t, "nord"))).sessionStory(c, status.Moment{}, wideStory, story, tt.chosen)
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
// personal, its cache run out by each, so none cost anything.
func movesStory(at ...time.Time) views.Story {
	s := views.Story{Start: storyAt(8, 12, 0, 0), End: storyAt(8, 14, 40, 30), Sent: views.SentAt(minutely(storyAt(8, 12, 0, 0), storyAt(8, 14, 40, 0))...)}
	accounts := []string{"work", "side", "personal"}
	for i, t := range at {
		s.Moves = append(s.Moves, capMove(accounts[i], accounts[i+1], t, 182_000, 182))
		s.Moves[i].Rewrote, s.Moves[i].Cost = false, nil
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
				layout, row = sessionLayout{story: true, body: storyColumnRows}, 0
			}
			c := newCanvas(tt.width, 9)
			storyFrame(tt.width, 9, Look{}).sessionStory(c, status.Moment{}, layout, tt.story, views.Turn{})
			if got := c.row(row, Look{}); got != tt.want {
				t.Errorf("the label's row reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// The story names each account as the rest of the page does, by its label
// where the config gives one other than its id: in a move's label, whole or
// short, in its names alone where it doesn't fit, and on the ribbon.
func TestAStoryNamesItsAccountsByTheirLabels(t *testing.T) {
	doc := status.Document{Accounts: []status.Account{{ID: "work", Label: "office"}, {ID: "side", Label: "spare"}, {ID: "personal", Label: "home"}}}
	tests := []struct {
		name   string
		width  int
		phone  bool
		story  views.Story
		label  string
		ribbon []string
	}{
		{
			name: "whole", width: 160, story: movedStory(),
			label: "office reached its cap → spare · 182k cached again, $1.82  14:31 ┊", ribbon: []string{" office ", " spare "},
		},
		{name: "short, on a phone", width: 52, phone: true, story: movedStory(), label: "its cap → spare, $1.82  14:31 ┊"},
		{name: "its names alone", width: 52, phone: true, story: resumedStory(), label: "┊ 09:12  spare → home"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, row, tall := wideStory, 1, storyColumnRows
			if tt.phone {
				layout, row = sessionLayout{story: true, body: storyColumnRows}, 0
			}
			f := storyFrame(tt.width, 9, Look{})
			c := newCanvas(tt.width, 9)
			f.sessionStory(c, f.Moment(doc, tt.story.End), layout, tt.story, views.Turn{})
			if got := strings.TrimSpace(c.row(row, Look{})); got != tt.label {
				t.Errorf("the label reads %q, want %q", got, tt.label)
			}
			ribbon := c.row(row+tall+1, Look{})
			for _, name := range tt.ribbon {
				if !strings.Contains(ribbon, name) {
					t.Errorf("the ribbon reads %q, want it naming %q", ribbon, name)
				}
			}
		})
	}
}

// The story names an account as a label's column holds it, as the rest of
// the page does: a label of 30 cells cut to 24, with an ellipsis, in a
// move's label and on the ribbon, never whole.
func TestAStoryNamesALongLabelAsALabelsColumnHoldsIt(t *testing.T) {
	long := "work-team-platform-engineering"
	doc := status.Document{Accounts: []status.Account{{ID: "work", Label: long}, {ID: "side", Label: "spare"}}}
	f := storyFrame(160, 9, Look{})
	c := newCanvas(160, 9)
	f.sessionStory(c, f.Moment(doc, movedStory().End), wideStory, movedStory(), views.Turn{})
	cut := prose.Cut(long, labelMost)
	for _, row := range []struct {
		what string
		y    int
	}{{"the move's label", 1}, {"the ribbon", 2 + storyColumnRows}} {
		got := c.row(row.y, Look{})
		if !strings.Contains(got, cut) || strings.Contains(got, long) {
			t.Errorf("%s reads %q, want it naming %q, cut to %d cells", row.what, got, cut, labelMost)
		}
	}
}

// Where a move's label says its names alone, a phone's story names a long
// label as a label's column holds it too: at 70 columns, f41a's rescore to
// a label of 30 cells, which its short words don't fit, cut to 24.
func TestAMovesNamesAloneNameALongLabelAsALabelsColumnHoldsIt(t *testing.T) {
	long := "personal-research-and-writing1"
	doc := status.Document{Accounts: []status.Account{{ID: "side", Label: "spare"}, {ID: "personal", Label: long}}}
	f := storyFrame(70, 9, Look{})
	c := newCanvas(70, 9)
	f.sessionStory(c, f.Moment(doc, resumedStory().End), sessionLayout{story: true, body: storyColumnRows}, resumedStory(), views.Turn{})
	if got, want := strings.TrimSpace(c.row(0, Look{})), "┊ 09:12  spare → "+prose.Cut(long, labelMost); got != want {
		t.Errorf("the move's label reads %q, want %q", got, want)
	}
}

// STORY folds to its heading where the page hasn't room for it whole, as
// the Overview folds a section, saying so in text.subtle, its rule to two
// cells short of the edge; on a phone it's left out; and where its heading
// has given way too, or under the chrome alone, nothing is drawn.
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
		{name: "wide, 12 rows: its heading given way", width: 160, height: 12},
		{name: "wide, 9 rows: the chrome alone", width: 160, height: 9},
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

// youngStory is a story 20 minutes old, on work from 12:05 to now, 12:25:30.
func youngStory() views.Story {
	return views.Story{Start: storyAt(8, 12, 5, 0), End: storyAt(8, 12, 25, 30), Sent: views.SentAt(minutely(storyAt(8, 12, 5, 0), storyAt(8, 12, 25, 0))...),
		Stretches: []views.Stretch{{Account: "work", From: storyAt(8, 12, 5, 0)}}}
}

// foldedStory is a story folded 32 times: 33 bursts on work, of a request a
// minute for three minutes, an hour and a minute apart, from midnight to
// now, 30 seconds after its last request.
func foldedStory() views.Story {
	var s views.Story
	var sent []time.Time
	for b := range 33 {
		at := storyAt(8, 0, 0, 0).Add(time.Duration(63*b) * time.Minute)
		sent = append(sent, minutely(at, at.Add(2*time.Minute))...)
		s.Stretches = append(s.Stretches, views.Stretch{Account: "work", From: at})
		if b > 0 {
			s.Folds = append(s.Folds, views.Fold{From: at.Add(-61 * time.Minute), To: at})
		}
	}
	s.Start, s.End, s.Sent = sent[0], sent[len(sent)-1].Add(30*time.Second), views.SentAt(sent...)
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
				l := f.layoutOf(1)
				c := storyDrawnOn(f, 1, story, views.Turn{})
				cols := story.Columns(width)
				tall, top := l.body, l.storyAt
				if l.wide {
					top = l.storyAt + 1
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
