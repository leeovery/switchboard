package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/views"
)

// hoursWeekdayNames are a week's names from Monday, as the grid's rows name
// them.
var hoursWeekdayNames = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// hoursLevelOf is the level the sample page shades the hour of the weekday
// at row day at: every level in turn along each row, from the row's own.
func hoursLevelOf(day, hour int) int {
	return (day + hour) % shadeLevels
}

// hoursSample is a page of a week from Monday, each hour at its level as
// hoursLevelOf has it, keyed by the cuts given, worded as the page words
// them; Sunday known on no day, its hours at none.
func hoursSample(keyCuts ...string) views.HoursPage {
	p := views.HoursPage{Days: make([]views.HoursDay, len(hoursWeekdayNames)), KeyCuts: keyCuts, Weeks: 4, Description: "the 4 weeks to Wed 30 Sep"}
	if len(keyCuts) > 0 {
		p.Cuts = []float64{20, 50, 90}
	}
	for i, name := range hoursWeekdayNames {
		d := views.HoursDay{Weekday: time.Weekday((i + 1) % 7), Name: name, Known: 4}
		if name == "Sun" {
			d.Known = 0
			p.Days[i] = d
			continue
		}
		for h := range d.Hours {
			d.Hours[h] = views.HoursTile{Requests: float64(hoursLevelOf(i, h)), Level: hoursLevelOf(i, h)}
		}
		p.Days[i] = d
	}
	return p
}

// hoursIn is the page drawn in look, width cells wide, from the top left of
// a canvas of its own as tall as it takes.
func hoursIn(p views.HoursPage, look Look, width int) *canvas {
	g := hoursGrid{page: p, look: look}
	c := newCanvas(width, g.rows(width))
	g.draw(c, 0, 0, width)
	return c
}

// hoursRow is a weekday's row of tiles as it reads, cells an hour: the
// tile, every cell of an hour but its last, then a gap, where an hour has
// more than one cell; each tile at its level's glyph, as glyph gives it.
func hoursRow(cells int, glyph func(level int) string, levelOf func(hour int) int) string {
	var b strings.Builder
	for h := range hoursADay {
		tile := max(cells-1, 1)
		b.WriteString(strings.Repeat(glyph(levelOf(h)), tile))
		b.WriteString(strings.Repeat(" ", cells-tile))
	}
	return strings.TrimRight(b.String(), " ")
}

func TestWhenYouWorksWidthsAreItsHoursFromATileAloneToFourCells(t *testing.T) {
	if got, want := hoursPart(), (flex.Part{Least: 28, Natural: 52, Most: 100, Grows: true}); got != want {
		t.Errorf("WHEN YOU WORK's part is %+v, want %+v: 4 + 24 at its least, two cells an hour its natural, four its most", got, want)
	}
	tests := []struct {
		width, cells, tile int
	}{
		{width: 20, cells: 1, tile: 1},
		{width: 28, cells: 1, tile: 1},
		{width: 51, cells: 1, tile: 1},
		{width: 52, cells: 2, tile: 1},
		{width: 75, cells: 2, tile: 1},
		{width: 76, cells: 3, tile: 2},
		{width: 100, cells: 4, tile: 3},
		{width: 173, cells: 4, tile: 3},
	}
	for _, tt := range tests {
		if got := hourCells(tt.width); got != tt.cells {
			t.Errorf("%d cells wide, an hour takes %d cells, want ⌊(%d − 4) / 24⌋ held from 1 to 4, %d", tt.width, got, tt.width, tt.cells)
		}
		if got := tileCells(tt.cells); got != tt.tile {
			t.Errorf("an hour of %d cells has a tile of %d, want %d", tt.cells, got, tt.tile)
		}
	}
}

// The grid: every third hour from 00 over its column, in text.faint; under
// it a row a weekday in the page's order, its name in text.subtle, then from
// the 4th column its 24 hours, each a tile ▆ in its level's viz.day ink, as
// wide as an hour's cells but its last, a gap; a tile alone at its least.
// A weekday known on no day has every hour at none.
func TestWhenYouWorkDrawsAWeekdayARowAndEveryThirdHourOverIt(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	p := hoursSample("20", "50", "90")
	tile := func(int) string { return hourTile }
	tests := []struct {
		width, cells int
		labels       string
	}{
		{width: 28, cells: 1, labels: "    00 03 06 09 12 15 18 21"},
		{width: 40, cells: 1, labels: "    00 03 06 09 12 15 18 21"},
		{width: 52, cells: 2, labels: "    00    03    06    09    12    15    18    21"},
		{width: 76, cells: 3, labels: "    00       03       06       09       12       15       18       21"},
		{width: 100, cells: 4, labels: "    00          03          06          09          12          15          18          21"},
		{width: 130, cells: 4, labels: "    00          03          06          09          12          15          18          21"},
	}
	for _, tt := range tests {
		c := hoursIn(p, nord, tt.width)
		if got := rowOf(c, 0); got != tt.labels {
			t.Errorf("%d wide, the hours read %q, want %q", tt.width, got, tt.labels)
		}
		for h := 0; h < hoursADay; h += 3 {
			if k := c.at(4+h*tt.cells, 0).ink; k != faintInk {
				t.Errorf("%d wide, hour %02d's label is in %+v, want text.faint", tt.width, h, k)
			}
		}
		for i, name := range hoursWeekdayNames {
			want := name + " " + hoursRow(tt.cells, tile, func(h int) int { return p.Days[i].Hours[h].Level })
			if got := rowOf(c, 1+i); got != want {
				t.Errorf("%d wide, %s's row reads %q, want %q", tt.width, name, got, want)
			}
			if k := c.at(0, 1+i).ink; k != dimInk {
				t.Errorf("%d wide, %s's name is in %+v, want text.subtle", tt.width, name, k)
			}
			for h := range hoursADay {
				if k := c.at(4+h*tt.cells, 1+i).ink; k != dayInk(p.Days[i].Hours[h].Level) {
					t.Errorf("%d wide, %s's hour %02d is in %+v, want viz.day.%d", tt.width, name, h, k, p.Days[i].Hours[h].Level)
				}
			}
		}
		if sun := p.Days[6]; slices.ContainsFunc(sun.Hours[:], func(tl views.HoursTile) bool { return tl.Level != 0 }) || c.at(4, 7).ink != dayInk(0) {
			t.Errorf("%d wide, Sunday, known on no day, is drawn %q in %+v; want every hour at none", tt.width, rowOf(c, 7), c.at(4, 7).ink)
		}
	}
}

// In a look that draws no shades of its own, each tile is its level's
// glyph, by its density, as History's shades are.
func TestWhenYouWorksTilesAreTheirLevelsGlyphsWhereTheLookDrawsNoShades(t *testing.T) {
	p := hoursSample("20", "50", "90")
	for _, look := range []Look{Screen(builtin(t, "terminal")), NoColour()} {
		c := hoursIn(p, look, 76)
		for i, name := range hoursWeekdayNames {
			want := name + " " + hoursRow(3, shadeGlyph, func(h int) int { return p.Days[i].Hours[h].Level })
			if got := rowOf(c, 1+i); got != want {
				t.Errorf("without shades, %s's row reads %q, want %q", name, got, want)
			}
		}
	}
}

// The key, under a blank row, from the grid's column: each shade's tile and
// its bounds, in text.subtle, then requests an hour, in text.faint, flowed
// onto as many rows as the width needs, an item never broken.
func TestWhenYouWorksKeyFlowsOntoTheRowsItsWidthNeeds(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	p := hoursSample("20", "50", "90")
	tests := []struct {
		width int
		want  []string
	}{
		{width: 100, want: []string{"▆ none   ▆ under 20   ▆ 20–50   ▆ 50–90   ▆ over 90   requests an hour"}},
		{width: 63, want: []string{"▆ none   ▆ under 20   ▆ 20–50   ▆ 50–90   ▆ over 90", "requests an hour"}},
		{width: 52, want: []string{"▆ none   ▆ under 20   ▆ 20–50   ▆ 50–90", "▆ over 90   requests an hour"}},
		{width: 28, want: []string{"▆ none   ▆ under 20", "▆ 20–50   ▆ 50–90", "▆ over 90", "requests an hour"}},
	}
	for _, tt := range tests {
		c := hoursIn(p, nord, tt.width)
		if got := rowOf(c, 8); got != "" {
			t.Errorf("%d wide, the row between the grid and its key reads %q, want it blank", tt.width, got)
		}
		var got []string
		for y := 9; y < c.height(); y++ {
			if row := rowOf(c, y); row != "" {
				got = append(got, strings.TrimPrefix(row, "    "))
			}
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%d wide, the key reads %q, want %q", tt.width, got, tt.want)
		}
		if k := c.at(4, 9).ink; k != dayInk(0) {
			t.Errorf("%d wide, none's tile is in %+v, want viz.day.0", tt.width, k)
		}
		if k := c.at(6, 9).ink; k != dimInk {
			t.Errorf("%d wide, none is in %+v, want text.subtle", tt.width, k)
		}
		unit := c.height() - 1
		for rowOf(c, unit) == "" {
			unit--
		}
		if at := strings.Index(rowOf(c, unit), "requests"); at < 0 || c.at(at, unit).ink != faintInk {
			t.Errorf("%d wide, requests an hour is in %+v, want text.faint", tt.width, c.at(max(at, 0), unit).ink)
		}
	}
}

// Each shade's tile in the key is in its level's ink, a cell wide whatever
// an hour's cells.
func TestWhenYouWorksKeyTilesAreEachShadesACellWide(t *testing.T) {
	c := hoursIn(hoursSample("20", "50", "90"), Screen(builtin(t, "nord")), 100)
	row := rowOf(c, 9)
	for level := range shadeLevels {
		at := 4
		for range level {
			at += strings.Index(row[at:], "   ▆") + 3
		}
		if here := c.at(runeColumn(row, at), 9); here.glyph != hourTile || here.ink != dayInk(level) || c.at(runeColumn(row, at)+1, 9).glyph != " " {
			t.Errorf("the key's tile for level %d is %+v, want a single ▆ in viz.day.%d", level, here, level)
		}
	}
}

// runeColumn is the column of the byte offset at in row, a cell a rune.
func runeColumn(row string, at int) int {
	return len([]rune(row[:at]))
}

// Days known, but no hour active, the grid shows every hour at none and no
// key; no day known, no grid, but that nothing is recorded, in
// text.subtle. Either way, the section takes the rows it would with its
// key, so the page holds still as the reads come.
func TestWhenYouWorkWithNothingActiveHasNoKeyAndWithNothingKnownSaysSo(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	quiet := hoursSample()
	for i := range quiet.Days {
		for h := range quiet.Days[i].Hours {
			quiet.Days[i].Hours[h] = views.HoursTile{}
		}
	}
	c := hoursIn(quiet, nord, 52)
	if got := rowOf(c, 1); got != "Mon "+hoursRow(2, func(int) string { return hourTile }, func(int) int { return 0 }) {
		t.Errorf("nothing active, Monday reads %q, want every hour at none", got)
	}
	for y := 8; y < c.height(); y++ {
		if got := rowOf(c, y); got != "" {
			t.Errorf("nothing active, row %d reads %q, want no key", y, got)
		}
	}

	c = hoursIn(views.HoursPage{}, nord, 52)
	if got := rowOf(c, 0); got != nothingRecorded || c.at(0, 0).ink != dimInk {
		t.Errorf("nothing known, the section reads %q in %+v, want %q in text.subtle", got, c.at(0, 0).ink, nothingRecorded)
	}
	for y := 1; y < c.height(); y++ {
		if got := rowOf(c, y); got != "" {
			t.Errorf("nothing known, row %d reads %q, want it blank", y, got)
		}
	}

	for _, width := range []int{28, 52, 63, 100, 173} {
		keyed := hoursGrid{page: hoursSample("20", "50", "90"), look: nord}.rows(width)
		for name, p := range map[string]views.HoursPage{"nothing active": quiet, "nothing known": {}, "the widest cuts reckoned": hoursSample("9,950", "9,950", "9,950")} {
			if got := (hoursGrid{page: p, look: nord}).rows(width); got != keyed {
				t.Errorf("%d wide, %s takes %d rows, want %d, as with its key", width, name, got, keyed)
			}
		}
		if got := 1 + hoursWeekdays + 1 + len(hoursGrid{page: hoursSample("20", "50", "90"), look: nord}.key(width)); got > keyed {
			t.Errorf("%d wide, the grid and its key take %d rows, more than the %d reckoned", width, got, keyed)
		}
	}
}

// The section draws nothing outside the rows and columns it's given.
func TestWhenYouWorkDrawsWithinItsBounds(t *testing.T) {
	g := hoursGrid{page: hoursSample("20", "50", "90"), look: Screen(builtin(t, "nord"))}
	for _, width := range []int{20, 28, 52} {
		rows := g.rows(width)
		c := newCanvas(width+4, rows+2)
		g.draw(c, 2, 1, width)
		for y := range c.height() {
			for x := range c.width {
				inside := x >= 2 && x < 2+width && y >= 1 && y < 1+rows
				if !inside && c.at(x, y).glyph != " " {
					t.Errorf("%d wide, the cell at %d,%d holds %q, outside the section", width, x, y, c.at(x, y).glyph)
				}
			}
		}
	}
}

// WHEN YOU WORK's heading says its page's weeks; with no page, as while the
// ledger's days are unread, nothing.
func TestWhenYouWorksHeadingSaysItsPagesWeeksOnceItHasOne(t *testing.T) {
	p := hoursSample("20", "50", "90")
	for _, tt := range []struct {
		hours *views.HoursPage
		want  string
	}{
		{hours: &p, want: "WHEN YOU WORK   the 4 weeks to Wed 30 Sep ─"},
		{want: "WHEN YOU WORK  ─"},
	} {
		f := graphsFrame(160, 37, GraphsSectionsNamed([]string{"when"}))
		f.Graphs.Hours = tt.hours
		if row := textOf(f.Draw(threeRouted(), now))[f.top()]; !strings.HasPrefix(row, tt.want) {
			t.Errorf("WHEN YOU WORK's heading reads %q, want it to start %q", row, tt.want)
		}
	}
}
