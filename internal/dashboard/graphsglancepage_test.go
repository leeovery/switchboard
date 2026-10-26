package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/views"
)

// glanceFrame is Graphs of trio's accounts at the size given, its sections
// those named, drawn in nord, of trio's trails, and the pool's room over
// each window's span built of trio at printNow, as glanceOf builds it.
func glanceFrame(t *testing.T, width, height int, names ...string) Frame {
	t.Helper()
	f := graphsFrame(width, height, GraphsSectionsNamed(names))
	f.Look, f.Policy, f.Trails = Screen(builtin(t, "nord")), printPolicy, trioTrails()
	f.Graphs.Accounts, f.Graphs.Spans = trioConfigured(), glanceOf(trio(), Look{}).pools
	return f
}

// glanceWhole is Graphs of the frame drawn whole, of trio at printNow, and
// how it's laid out.
func glanceWhole(f Frame) (*canvas, graphsLaid) {
	laid := f.graphsLaid()
	c := newCanvas(f.Width, laid.rows)
	f.graphsDrawn(c, f.Moment(trio(), printNow), laid)
	return c, laid
}

// pairsFrom is the width GLANCE and WHEN YOU WORK pair from, of the
// configured accounts given: GLANCE's natural, glanceWhenGap and WHEN YOU
// WORK's natural.
func pairsFrom(accounts config.Accounts) int {
	return glancePart(accounts).Natural + glanceWhenGap + hoursPart().Natural
}

// GLANCE is drawn under its heading and blank row, from the section's left,
// by glance.draw as layGlance lays it across the width, cell for cell,
// filling its rows, its key last: its windows side by side at 173 and 108,
// its words narrowed at 108, and stacked on a phone, 73 and 52.
func TestGraphsDrawsGlanceInItsRows(t *testing.T) {
	tests := []struct {
		width, height int
		stacked       bool
		narrowed      bool
	}{
		{width: 173, height: 53},
		{width: 108, height: 44, narrowed: true},
		{width: 73, height: 61, stacked: true},
		{width: 52, height: 63, stacked: true, narrowed: true},
	}
	for _, tt := range tests {
		f := glanceFrame(t, tt.width, tt.height, "week", "glance")
		c, laid := glanceWhole(f)
		body := placedOf(t, laid, GraphsGlance).body(tt.width)
		l := layGlance(f.Graphs.Accounts, tt.width)
		if l.stacked != tt.stacked || (l.words < fullWidest()) != tt.narrowed || l.rows() != body.rows {
			t.Errorf("at %d columns, GLANCE is laid stacked %t, its words %d cells of %d, in %d rows of a body of %d; want stacked %t, its words narrowed %t, in its body's rows",
				tt.width, l.stacked, l.words, fullWidest(), l.rows(), body.rows, tt.stacked, tt.narrowed)
		}
		want := newCanvas(tt.width, laid.rows)
		glance{m: f.Moment(trio(), printNow), trails: f.Trails, pools: f.Graphs.Spans, look: f.Look}.draw(want, l, 0, body.y)
		if x, y, differ := sameRows(c, want, body.y, laid.rows); differ {
			t.Errorf("at %d columns, GLANCE's row %d, column %d, is %+v; glance draws %+v", tt.width, y, x, *c.at(x, y), *want.at(x, y))
		}
		if key := rowOf(c, body.y+body.rows-len(l.key)); !strings.HasPrefix(key, sparkTile+" room left") || rowOf(c, body.y+body.rows-1) == "" {
			t.Errorf("at %d columns, GLANCE's key reads %q from its row %d of %d, want its key on its last rows", tt.width, key, body.rows-len(l.key), body.rows)
		}
	}
}

// GLANCE and WHEN YOU WORK sit side by side where both show and both fit at
// their naturals with 10 cells between, GLANCE from column 0 at its natural
// and WHEN YOU WORK after it to the width's end, their headings on the one
// row; a cell short of that, WHEN YOU WORK comes under GLANCE, each across
// the width. So with the sample's names, from 175 columns, GLANCE's natural
// 113; and so of one account and of four, one named at a label's most.
func TestGlanceSitsBesideWhenYouWorkWhereBothFitAtTheirNaturals(t *testing.T) {
	if got := pairsFrom(trioConfigured()); got != 175 || glancePart(trioConfigured()).Natural != 113 {
		t.Errorf("with the sample's names, GLANCE's natural is %d and the pair fits from %d columns, want 113 and 175", glancePart(trioConfigured()).Natural, got)
	}
	four := append(trioConfigured(), config.Account{ID: "lab", Label: "research-lab-and-consult"})
	for _, accounts := range []config.Accounts{trioConfigured(), trioConfigured()[:1], four} {
		from, natural := pairsFrom(accounts), glancePart(accounts).Natural
		for _, width := range []int{from, from + 40} {
			f := graphsFrame(width, 80, GraphsSectionsNamed([]string{"glance", "when"}))
			f.Graphs.Accounts = accounts
			l := f.graphsLaid()
			g, w := placedOf(t, l, GraphsGlance), placedOf(t, l, GraphsWhen)
			at := natural + glanceWhenGap
			if g.at != w.at || g.across != (flex.Span{Size: natural}) || w.across != (flex.Span{At: at, Size: width - at}) {
				t.Errorf("of %d accounts at %d columns, GLANCE is at row %d across %+v, WHEN YOU WORK at row %d across %+v; want them on one row, across {0 %d} and {%d %d}",
					len(accounts), width, g.at, g.across, w.at, w.across, natural, at, width-at)
			}
		}
		f := graphsFrame(from-1, 80, GraphsSectionsNamed([]string{"glance", "when"}))
		f.Graphs.Accounts = accounts
		l := f.graphsLaid()
		g, w := placedOf(t, l, GraphsGlance), placedOf(t, l, GraphsWhen)
		if w.at != g.at+g.rows+1 || g.across != (flex.Span{Size: from - 1}) || w.across != g.across {
			t.Errorf("of %d accounts a cell short, at %d columns, GLANCE is at row %d of %d across %+v, WHEN YOU WORK at row %d across %+v; want it under GLANCE, each across the width",
				len(accounts), from-1, g.at, g.rows, g.across, w.at, w.across)
		}
	}
	for _, alone := range []string{"glance", "when"} {
		f := graphsFrame(250, 80, GraphsSectionsNamed([]string{"week", alone}))
		f.Graphs.Accounts = trioConfigured()
		for _, s := range f.graphsLaid().sections {
			if s.across != (flex.Span{Size: 250}) {
				t.Errorf("with %s shown alone of the pair, %s is across %+v, want the width", alone, s.section.heading(), s.across)
			}
		}
	}
}

// Side by side, the pair takes the rows of the taller, the planner counting
// each at the width it's laid at: WHEN YOU WORK's of one account; GLANCE's
// of four.
func TestGlanceBesideWhenYouWorkTakesTheTallersRows(t *testing.T) {
	four := append(trioConfigured(), config.Account{ID: "lab", Label: "lab"})
	tests := []struct {
		name     string
		accounts config.Accounts
		taller   GraphsSection
	}{
		{name: "one account", accounts: trioConfigured()[:1], taller: GraphsWhen},
		{name: "four", accounts: four, taller: GraphsGlance},
	}
	for _, tt := range tests {
		width := pairsFrom(tt.accounts) + 7
		f := graphsFrame(width, 80, GraphsSectionsNamed([]string{"glance", "when"}))
		f.Graphs.Accounts = tt.accounts
		l := f.graphsLaid()
		g, w := placedOf(t, l, GraphsGlance), placedOf(t, l, GraphsWhen)
		own := map[GraphsSection]int{
			GraphsGlance: layGlance(tt.accounts, g.across.Size).rows(),
			GraphsWhen:   whenRows(w.across.Size, f.Look),
		}
		other := GraphsGlance + GraphsWhen - tt.taller
		if want := sectionHead + own[tt.taller]; g.rows != want || w.rows != want || own[other] >= own[tt.taller] || l.rows != want {
			t.Errorf("%s: GLANCE takes %d rows and WHEN YOU WORK %d, the page %d, their own %v; want each, and the page, %d, %s's the taller",
				tt.name, g.rows, w.rows, l.rows, own, want, tt.taller.heading())
		}
	}
}

// Side by side, each heading is on the one row across its own columns, as
// headSection draws one, its key in bold accent.key in its own last column,
// the 10 cells between blank; and each is a target over its own columns, a
// click on which presses its key.
func TestGraphsHeadsGlanceAndWhenYouWorkSideBySide(t *testing.T) {
	f := glanceFrame(t, 180, 53, "glance", "when")
	rows := textOf(f.Draw(trio(), printNow))
	natural := glancePart(f.Graphs.Accounts).Natural
	wantAt := map[string]flex.Span{"g": {Size: natural}, "h": {At: natural + glanceWhenGap, Size: 180 - natural - glanceWhenGap}}
	keys, _ := pressed(f)
	if !slices.Equal(keys, []string{"g", "h"}) {
		t.Fatalf("the headings that are targets press %q, want g and h", keys)
	}
	for _, tt := range f.Targets(trio(), printNow) {
		if tt.On != OnPress {
			continue
		}
		section, _ := GraphsSectionKeyed(tt.Key)
		if want := wantAt[tt.Key]; tt.X != want.At || tt.Width != want.Size || tt.Y != f.top() {
			t.Errorf("%s's target is at %d from %d, %d wide; want on row %d from %d, %d wide", section.heading(), tt.Y, tt.X, tt.Width, f.top(), want.At, want.Size)
		}
		if got := under(rows, tt); !pressedOver(got, tt.Key) {
			t.Errorf("%s's target is over %q, want its heading, from its name to its key", section.heading(), got)
		}
	}
	if gap := []rune(rows[f.top()])[natural : natural+glanceWhenGap]; strings.TrimSpace(string(gap)) != "" {
		t.Errorf("the cells between the headings read %q, want them blank", string(gap))
	}
	c := newCanvas(f.Width, f.Height)
	f.graphsTab(c, trio(), printNow, f.top())
	for _, last := range []int{natural - 1, 179} {
		if cell := c.at(last, f.top()); cell.ink != keyInk {
			t.Errorf("the heading's last column, %d, holds %q in %+v, want its key in accent.key", last, cell.glyph, cell.ink)
		}
	}
}

// Beside GLANCE, WHEN YOU WORK is drawn from its own left, by hoursGrid.draw
// at the width flex lays it at in its columns, cell for cell; GLANCE from
// column 0 at its natural, by glance.draw.
func TestGraphsDrawsGlanceAndWhenYouWorkEachInItsOwnColumns(t *testing.T) {
	sample := hoursSample("20", "50", "90")
	for _, width := range []int{180, 220} {
		f := glanceFrame(t, width, 53, "glance", "when")
		f.Graphs.Hours = &sample
		c, laid := glanceWhole(f)
		g, w := placedOf(t, laid, GraphsGlance), placedOf(t, laid, GraphsWhen)
		want := newCanvas(width, laid.rows)
		l := layGlance(f.Graphs.Accounts, g.across.Size)
		glance{m: f.Moment(trio(), printNow), trails: f.Trails, pools: f.Graphs.Spans, look: f.Look}.draw(want, l, 0, g.at+sectionHead)
		hoursGrid{page: sample, look: f.Look}.draw(want, w.across.At, w.at+sectionHead, hoursLaid(w.across.Size))
		if x, y, differ := sameRows(c, want, g.at+sectionHead, laid.rows); differ {
			t.Errorf("at %d columns, the pair's row %d, column %d, is %+v; GLANCE and WHEN YOU WORK draw %+v", width, y, x, *c.at(x, y), *want.at(x, y))
		}
	}
}

// Graphs reads the lanes of the windows its sections shown draw of them, in
// the order the page first draws them: each burn-down's its window, and
// GLANCE's both, which place its ✕ as a card's; none where none of those
// shows.
func TestGraphsWindowsAreThoseItsSectionsShownDraw(t *testing.T) {
	week, five := views.RunwayWeek, views.RunwayFiveHour
	tests := []struct {
		shown []string
		want  []views.RunwayWindow
	}{
		{shown: []string{"week", "5-hour"}, want: []views.RunwayWindow{week, five}},
		{shown: []string{"5-hour"}, want: []views.RunwayWindow{five}},
		{shown: []string{"glance"}, want: []views.RunwayWindow{week, five}},
		{shown: []string{"5-hour", "glance"}, want: []views.RunwayWindow{five, week}},
		{shown: []string{"week", "5-hour", "pool", "glance", "when"}, want: []views.RunwayWindow{week, five}},
		{shown: []string{"pool", "when"}},
	}
	for _, tt := range tests {
		if got := GraphsWindows(GraphsSectionsNamed(tt.shown)); !slices.Equal(got, tt.want) {
			t.Errorf("with %q shown, Graphs reads the lanes of %v, want %v", tt.shown, got, tt.want)
		}
	}
}

// Scrolled, the page shows each head row of GLANCE's parts, its windows
// stacked, with the first row under its blank row, or not at all, as it
// shows each section's heading: at every height a phone's page scrolls at,
// and every offset it scrolls to.
func TestGraphsPagesGlancesStackedPartsHeadsWithTheirFirstRows(t *testing.T) {
	c, laid := glanceWhole(glanceFrame(t, 52, 40, "week", "glance"))
	heads, g := laid.glance.heads(), placedOf(t, laid, GraphsGlance)
	if len(heads) != len(runwayWindows) {
		t.Fatalf("GLANCE stacked has %d part heads, want one a window, %d", len(heads), len(runwayWindows))
	}
	for i, at := range heads {
		if got, want := strings.TrimSpace(rowOf(c, g.at+sectionHead+at)), strings.ToUpper(runwayNames[runwayWindows[i]]); got != want {
			t.Errorf("GLANCE's part head %d, on its row %d, reads %q, want its window's name, %q", i, at, got, want)
		}
	}
	for height := 14; height <= 40; height++ {
		f := glanceFrame(t, 52, height, "week", "glance")
		laid := f.graphsLaid()
		paged, g := laid.paged(), placedOf(t, laid, GraphsGlance)
		if !laid.glance.stacked {
			t.Fatalf("at 52 columns GLANCE isn't stacked, want its windows stacked")
		}
		for offset := 0; offset <= paged.scrolling(0).Most; offset++ {
			from, to := paged.shown(offset)
			for _, at := range laid.glance.heads() {
				head := g.at + sectionHead + at
				if from <= head && head < to && head+sectionHead >= to {
					t.Errorf("at 52×%d scrolled %d, rows %d to %d show GLANCE's part head on row %d without its first row", height, offset, from, to, head)
				}
			}
		}
	}
}
