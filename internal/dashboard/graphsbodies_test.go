package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// graphsWhole is Graphs of the frame drawn whole, as graphsTab draws it
// before it's paged, of threeRouted at now, and how it's laid out.
func graphsWhole(f Frame) (*canvas, graphsLaid) {
	laid := f.graphsLaid()
	c := newCanvas(f.Width, laid.rows)
	f.graphsDrawn(c, f.Moment(threeRouted(), now), laid)
	return c, laid
}

// placedOf is where the page lays the section out, failing the test where
// it lays it nowhere.
func placedOf(t *testing.T, l graphsLaid, s GraphsSection) graphsPlaced {
	t.Helper()
	i := slices.IndexFunc(l.sections, func(p graphsPlaced) bool { return p.section == s })
	if i < 0 {
		t.Fatalf("the page lays out %+v, want %s among them", l.sections, s.heading())
	}
	return l.sections[i]
}

// sameRows reports the first cell, by row and column, at which got and want
// differ over the rows from to to, false where none does.
func sameRows(got, want *canvas, from, to int) (x, y int, differ bool) {
	for y := from; y < to; y++ {
		for x := range got.width {
			if *got.at(x, y) != *want.at(x, y) {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// liveStack is a stack of the configured accounts given over Runway's
// week's live timeline at now, columns across, each holding the room given
// in every column.
func liveStack(columns int, room float64, ids ...string) views.PoolStack {
	s := views.PoolStack{Timeline: views.LiveTimeline(views.RunwayWeek, now, columns)}
	for _, id := range ids {
		rooms := make([]views.AccountRoom, columns)
		for c := range rooms {
			rooms[c] = views.AccountRoom{Room: room, Known: true}
		}
		s.Accounts = append(s.Accounts, views.PoolStackAccount{ID: id, Label: id, Rooms: rooms})
	}
	return s
}

// POOL is drawn under its heading and blank row, by poolStacked.draw, cell
// for cell, across the width, its chart as tall as the planner has every
// chart, its axis and its key filling the section's rows to its last; the
// blank row after it, before WHEN YOU WORK's heading, left blank.
func TestGraphsDrawsPoolInItsRowsAtThePlannersSizes(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	tests := []struct {
		width, height, chart int
	}{
		{width: 173, height: 53, chart: 9},
		{width: 108, height: 44, chart: 5},
		{width: 73, height: 61, chart: 5},
		{width: 52, height: 63, chart: 5},
	}
	for _, tt := range tests {
		f := graphsFrame(tt.width, tt.height, GraphsSectionsNamed([]string{"week", "pool", "when"}))
		f.Look, f.Graphs.Pool = look, liveStack(tt.width, 0.6, "work", "personal", "side")
		c, laid := graphsWhole(f)
		placed := placedOf(t, laid, GraphsPool)
		body := placed.body(tt.width)
		p := poolStacked{stack: f.Graphs.Pool, look: look}
		if laid.chart != tt.chart || p.rows(tt.width, laid.chart) != body.rows {
			t.Errorf("at %d×%d, POOL's chart is %d rows, its body %d; want %d, the %d its stack takes", tt.width, tt.height, laid.chart, body.rows, tt.chart, p.rows(tt.width, laid.chart))
		}
		want := newCanvas(tt.width, laid.rows)
		p.draw(want, 0, body.y, tt.width, laid.chart)
		if x, y, differ := sameRows(c, want, body.y, body.y+body.rows); differ {
			t.Errorf("at %d×%d, POOL's row %d, column %d, is %+v; poolStacked draws %+v", tt.width, tt.height, y, x, *c.at(x, y), *want.at(x, y))
		}
		if now, y := f.Graphs.Pool.Timeline.NowColumn(), body.y+laid.chart-1; c.at(now, y).glyph != nowLine {
			t.Errorf("at %d×%d, now's column, %d, holds %q at the chart's foot, want │", tt.width, tt.height, now, c.at(now, y).glyph)
		}
		if after := rowOf(c, body.y+body.rows); after != "" || !strings.HasPrefix(rowOf(c, body.y+body.rows+1), "WHEN YOU WORK") {
			t.Errorf("at %d×%d, the row after POOL reads %q, the next %q; want it blank, then WHEN YOU WORK's heading", tt.width, tt.height, after, rowOf(c, body.y+body.rows+1))
		}
	}
}

// POOL's key names the pool's members, as a pin narrows them, never taking
// more rows than the configured accounts' key the planner counts: a pin to
// two names them alone, in the one row it takes of the two the configured
// three's take; a key that would take more, as of labels longer than the
// config's, is cut at the section's last row, and nothing is drawn on the
// blank row after it.
func TestGraphsPoolsKeyOfAPinsAccountsStaysWithinItsRows(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	const width = 73
	tests := []struct {
		name   string
		labels []string
		want   []string
	}{
		{name: "a pin to two", labels: []string{"work", "side"}, want: []string{"█ work    █ side    █ faint: heading, at its recent rate    │ now", ""}},
		{
			name:   "labels longer than the config's",
			labels: []string{"work-labelled-far-longer-than-the-config", "side-labelled-far-longer-than-the-config"},
			want:   []string{"█ work-labelled-far-longer-than-the-config", "█ side-labelled-far-longer-than-the-config"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := graphsFrame(width, 61, GraphsSectionsNamed([]string{"pool", "when"}))
			f.Look, f.Graphs.Pool = look, liveStack(width, 0.5, "work", "side")
			for i, label := range tt.labels {
				f.Graphs.Pool.Accounts[i].Label = label
			}
			c, laid := graphsWhole(f)
			body := placedOf(t, laid, GraphsPool).body(width)
			var got []string
			for y := body.y + laid.chart + runwayAxisRows; y < body.y+body.rows; y++ {
				got = append(got, rowOf(c, y))
			}
			if !slices.Equal(got, tt.want) || laid.pool.keyRows(width) != len(tt.want) {
				t.Errorf("the key's rows read %q, of the %d the configured accounts' take; want %q", got, laid.pool.keyRows(width), tt.want)
			}
			if after := rowOf(c, body.y+body.rows); after != "" {
				t.Errorf("the row after POOL reads %q, want it blank", after)
			}
		})
	}
}

// Where the stack has nothing to stack, as with no week window in the
// policy, nothing in the pool yet, or before the watch builds it, POOL draws
// what holds, over Runway's week's live timeline at now: now's │ down the
// chart, the chart empty, the axis, and the configured accounts' key.
func TestGraphsPoolWithNothingToStackDrawsNowTheAxisAndTheKey(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	const width = 108
	tl := views.LiveTimeline(views.RunwayWeek, now, width)
	tests := []struct {
		name  string
		stack views.PoolStack
	}{
		{name: "not built yet"},
		{name: "no account in the pool", stack: views.PoolStack{Timeline: tl}},
		{name: "built across other columns", stack: liveStack(width+10, 0.5, "work", "personal", "side")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := graphsFrame(width, 44, GraphsSectionsNamed([]string{"pool"}))
			f.Look, f.Graphs.Pool = look, tt.stack
			c, laid := graphsWhole(f)
			body := placedOf(t, laid, GraphsPool).body(width)
			nowAt := tl.NowColumn()
			for y := body.y; y < body.y+laid.chart; y++ {
				for x := range width {
					if here, line := *c.at(x, y), x == nowAt; line != (here.glyph == nowLine) || !line && here != blank {
						t.Fatalf("the chart's row %d, column %d, holds %+v; want it empty, but for │ down now's column, %d", y, x, here, nowAt)
					}
				}
			}
			axis := newCanvas(width, runwayAxisRows)
			timeAxis(axis, tl, views.RunwayWeek, 0, 0)
			for r := range runwayAxisRows {
				if got, want := rowOf(c, body.y+laid.chart+r), rowOf(axis, r); got != want {
					t.Errorf("the axis's row %d reads %q, want Runway's week's, %q", r, got, want)
				}
			}
			key, want := rowOf(c, body.y+laid.chart+runwayAxisRows), "█ work    █ personal    █ side    █ faint: heading, at its recent rate    │ now"
			if key != want || c.at(0, body.y+laid.chart+runwayAxisRows).ink != accountHue(0).ink() {
				t.Errorf("the key reads %q in %+v, want %q, work's █ in its hue", key, c.at(0, body.y+laid.chart+runwayAxisRows).ink, want)
			}
		})
	}
}

// WHEN YOU WORK is drawn under its heading and blank row, from the
// section's left, by hoursGrid.draw at the width flex lays it at, cell for
// cell, filling its rows; where the watch built no page, it says nothing is
// recorded yet, in text.subtle.
func TestGraphsDrawsWhenYouWorkInItsRows(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	sample := hoursSample("20", "50", "90")
	tests := []struct {
		width, laid int
	}{
		{width: 173, laid: 100},
		{width: 100, laid: 100},
		{width: 73, laid: 73},
		{width: 52, laid: 52},
	}
	for _, tt := range tests {
		f := graphsFrame(tt.width, 80, GraphsSectionsNamed([]string{"pool", "when"}))
		f.Look, f.Graphs.Hours = look, &sample
		c, laid := graphsWhole(f)
		body := placedOf(t, laid, GraphsWhen).body(hoursLaid(tt.width))
		g := hoursGrid{page: sample, look: look}
		if body.width != tt.laid || g.rows(tt.laid) != body.rows {
			t.Errorf("at %d columns, WHEN YOU WORK is laid %d wide in %d rows; want %d wide, in the %d its grid takes", tt.width, body.width, body.rows, tt.laid, g.rows(tt.laid))
		}
		want := newCanvas(tt.width, laid.rows)
		g.draw(want, 0, body.y, tt.laid)
		if x, y, differ := sameRows(c, want, body.y, laid.rows); differ {
			t.Errorf("at %d columns, WHEN YOU WORK's row %d, column %d, is %+v; hoursGrid draws %+v", tt.width, y, x, *c.at(x, y), *want.at(x, y))
		}
		if last := rowOf(c, body.y+body.rows-1); last == "" {
			t.Errorf("at %d columns, WHEN YOU WORK's last row is blank, want its key there", tt.width)
		}
	}

	f := graphsFrame(173, 53, GraphsSectionsNamed([]string{"when"}))
	f.Look = look
	c, laid := graphsWhole(f)
	body := placedOf(t, laid, GraphsWhen).body(173)
	if got, k := rowOf(c, body.y), c.at(0, body.y).ink; got != nothingRecorded || k != dimInk || k.token != theme.TextSubtle {
		t.Errorf("with no page, WHEN YOU WORK reads %q in %+v, want %q in text.subtle", got, k, nothingRecorded)
	}
}
