package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// placedAt are the rows the sections laid out are at, from the first
// heading, in the page's order.
func placedAt(l graphsLaid) []int {
	var at []int
	for _, s := range l.sections {
		at = append(at, s.at)
	}
	return at
}

// The design's worked sizes, with WEEK shown, three accounts two cards a row
// where two fit: at 160 by 37, the words have rows of their own and the
// charts 8 rows; at 108 by 44, 12; on a phone of 73 by 61, a card a row, 11;
// and with four accounts on that phone, 7.
func TestGraphsLaysTheDesignsWorkedSizesOut(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		accounts      []string
		apart         bool
		chart         int
		at            []int
		poolLine      int
		rows, room    int
	}{
		{name: "160 by 37", width: 160, height: 37, accounts: []string{"work", "personal", "side"}, apart: true, chart: 8, at: []int{0}, poolLine: 28, rows: 29, room: 30},
		{name: "108 by 44", width: 108, height: 44, accounts: []string{"work", "personal", "side"}, apart: true, chart: 12, at: []int{0}, poolLine: 36, rows: 37, room: 37},
		{name: "a phone of 73 by 61", width: 73, height: 61, accounts: []string{"work", "personal", "side"}, apart: true, chart: 11, at: []int{0}, poolLine: 50, rows: 51, room: 53},
		{name: "four accounts on that phone", width: 73, height: 61, accounts: []string{"work", "personal", "side", "team"}, apart: true, chart: 7, at: []int{0}, poolLine: 50, rows: 51, room: 53},
	}
	for _, tt := range tests {
		f := graphsFrame(tt.width, tt.height, GraphsSections{})
		f.Graphs.Accounts = configured(tt.accounts...)
		l := f.graphsLaid()
		if l.apart != tt.apart || l.chart != tt.chart {
			t.Errorf("%s: the words apart %t, the charts %d rows; want apart %t, %d rows", tt.name, l.apart, l.chart, tt.apart, tt.chart)
		}
		if got := placedAt(l); !slices.Equal(got, tt.at) || l.poolLine != tt.poolLine || l.rows != tt.rows || l.room != tt.room {
			t.Errorf("%s: the headings at %v, the pool's line at %d, %d rows in %d; want %v, %d, %d rows in %d", tt.name, got, l.poolLine, l.rows, l.room, tt.at, tt.poolLine, tt.rows, tt.room)
		}
		if most, want := f.Scrolling(threeRouted(), now).Most, max(tt.rows-tt.room, 0); most != want {
			t.Errorf("%s: the page scrolls %d rows, want %d", tt.name, most, want)
		}
	}
}

// rowsToLastDrawn are the rows from c's first to its last that isn't blank.
func rowsToLastDrawn(c *canvas) int {
	rows := 0
	for y := range c.height() {
		if rowOf(c, y) != "" {
			rows = y + 1
		}
	}
	return rows
}

// Each section takes its heading's rows and its own, as what draws it
// counts them: WEEK's to the foot of its last card's axis; POOL's as
// its stack takes them; GLANCE's to the last row it draws across its
// columns, of the configured accounts and whether they're a pool; WHEN YOU
// WORK's as its grid counts them across its columns; and GLANCE and WHEN
// YOU WORK side by side, each the taller's. So at the design's sections
// frame, 173 by 53, with WEEK, POOL, GLANCE and WHEN YOU WORK shown, at 180
// by 53, where they pair, and on a phone, of one account and of three.
func TestGraphsHoldsEachSectionToItsOwnRows(t *testing.T) {
	shown := GraphsSectionsNamed([]string{"week", "pool", "glance", "when"})
	tests := []struct {
		width, height int
		accounts      config.Accounts
	}{
		{width: 173, height: 53, accounts: trioConfigured()},
		{width: 180, height: 53, accounts: trioConfigured()},
		{width: 73, height: 61, accounts: trioConfigured()},
		{width: 73, height: 61, accounts: trioConfigured()[:1]},
	}
	for _, tt := range tests {
		f := graphsFrame(tt.width, tt.height, shown)
		f.Graphs.Accounts = tt.accounts
		l := f.graphsLaid()
		owns, tallest := make([]int, len(l.sections)), map[int]int{}
		for i, s := range l.sections {
			switch s.section {
			case GraphsPool:
				owns[i] = l.pool.rows(l.width, l.chart)
			case GraphsGlance:
				c, _ := glanceIn(glanceOf(trio(), Look{}), tt.accounts, s.across.Size, 5)
				owns[i] = rowsToLastDrawn(c)
			case GraphsWhen:
				owns[i] = whenRows(s.across.Size, f.Look)
			default:
				cards := l.cards(s)
				last := cards[len(cards)-1].chart
				owns[i] = last.y + last.rows - s.at - sectionHead
			}
			tallest[s.at] = max(tallest[s.at], owns[i])
		}
		for i, s := range l.sections {
			if s.rows != sectionHead+tallest[s.at] {
				t.Errorf("at %dx%d, of %d accounts, %s takes %d rows, want %d, its heading's and the tallest's at its row, %d; its own %d",
					tt.width, tt.height, len(tt.accounts), s.section.heading(), s.rows, sectionHead+tallest[s.at], tallest[s.at], owns[i])
			}
		}
	}
}

// stepIndex is the step's place among graphsSteps.
func stepIndex(t *testing.T, s graphsStep) int {
	t.Helper()
	i := slices.Index(graphsSteps(), s)
	if i < 0 {
		t.Fatalf("the step %+v is none of the steps", s)
	}
	return i
}

// Rows to spare go first to the cards' words, a row of their own in every
// card at once, then to every chart, the cards' and POOL's,
// the same height, a row at a time, to 19; the rest is blank at the foot. A
// step is taken only where it fits, and the steps are taken in order, one
// at a time, as the rows grow from the least.
func TestGraphsRowsToSpareGoToTheWordsThenToEveryChartARowAtATime(t *testing.T) {
	for _, shown := range []GraphsSections{{}, GraphsSectionsNamed([]string{"week", "pool"})} {
		name := strings.Join(shown.Names(), " and ")
		steps := graphsSteps()
		last := 0
		for height := 30; height <= 200; height++ {
			f := graphsFrame(108, height, shown)
			l := f.graphsLaid()
			i := stepIndex(t, l.graphsStep)
			if i < last || i > last+1 {
				t.Errorf("%s at %d rows: step %d after step %d, want the steps one at a time, in order", name, height, i, last)
			}
			last = i
			if i > 0 && !l.fits() {
				t.Errorf("%s at %d rows: step %+v taken where it doesn't fit, %d rows in %d", name, height, l.graphsStep, l.rows, l.room)
			}
			if i+1 < len(steps) && l.laidAt(steps[i+1]).fits() {
				t.Errorf("%s at %d rows: step %+v taken where the next fits", name, height, l.graphsStep)
			}
			for _, s := range l.sections {
				if s.section == GraphsPool && s.rows != sectionHead+l.pool.rows(l.width, l.chart) {
					t.Errorf("%s at %d rows: POOL takes %d rows, want its heading's and its own with its chart %d rows", name, height, s.rows, l.chart)
				}
				if s.section != GraphsWeek {
					continue
				}
				if got := cardChartRows(l, s); got != l.chart {
					t.Errorf("%s at %d rows: %s's charts are %d rows, want every one %d", name, height, s.section.heading(), got, l.chart)
				}
			}
		}
		if last != len(steps)-1 {
			t.Errorf("%s: at 200 rows, the step is %+v, want the last, the charts %d rows", name, steps[last], mostChart)
		}
	}
}

// cardChartRows is how tall WEEK's cards' charts are, as laid out: -1 where
// they differ.
func cardChartRows(l graphsLaid, s graphsPlaced) int {
	cards := l.cards(s)
	h := cards[0].chart.rows - 1
	for _, c := range cards {
		if c.chart.rows-1 != h {
			return -1
		}
	}
	return h
}

// sectionsNamed are the sections of the bits of mask shown, the first bit
// WEEK's.
func sectionsNamed(mask int) GraphsSections {
	names := []string{}
	for _, s := range EveryGraphsSection() {
		if mask&(1<<s) != 0 {
			names = append(names, wordingOf(s).name)
		}
	}
	return GraphsSectionsNamed(names)
}

// Every section shown is laid out, in the page's order, a hidden one taking
// no room, a blank row between two; the pool's line after WEEK, where it
// shows, after its blank row, and a blank row before the next; and the
// legend while WEEK shows, none else: at 120 columns, where GLANCE and WHEN
// YOU WORK don't sit side by side, and at 173, where they do, WHEN YOU WORK
// on GLANCE's row, from a column past its start, the next section after the
// taller of the two.
func TestGraphsLaysOutEverySectionShownAndNoneHidden(t *testing.T) {
	for _, width := range []int{120, 173} {
		for mask := 1; mask < 1<<len(EveryGraphsSection()); mask++ {
			shown := sectionsNamed(mask)
			name := fmt.Sprintf("at %d columns, %s", width, strings.Join(shown.Names(), ", "))
			l := graphsFrame(width, 200, shown).graphsLaid()
			week := -1
			var laid []string
			for i, s := range l.sections {
				laid = append(laid, wordingOf(s.section).name)
				if s.section == GraphsWeek {
					week = i
				}
			}
			if !slices.Equal(laid, shown.Names()) {
				t.Errorf("%s: laid out %q, want those shown, in order", name, laid)
			}
			next, poolLine := 0, -1
			for i, s := range l.sections {
				paired := width == 173 && s.section == GraphsWhen && shown.Has(GraphsGlance)
				want, top := next, s.at
				if paired {
					glance := l.sections[i-1]
					want, top = glance.at, glance.at+max(glance.rows-s.rows, 0)
				}
				if s.at != want || (s.across.At > 0) != paired {
					t.Errorf("%s: %s at row %d across %+v, want at %d, beside GLANCE %t", name, s.section.heading(), s.at, s.across, want, paired)
				}
				next = top + s.rows + 1
				if i == week {
					poolLine, next = next, next+2
				}
			}
			if l.poolLine != poolLine || l.rows != next-1 {
				t.Errorf("%s: the pool's line at %d, %d rows; want it at %d, %d rows", name, l.poolLine, l.rows, poolLine, next-1)
			}
			if got, want := len(l.legend), min(week+1, 1); got != want {
				t.Errorf("%s: the legend takes %d rows, want %d", name, got, want)
			}
		}
	}
}

// With one account there's no pool, and no pool's line; with two, there is.
func TestGraphsHasThePoolsLineOnlyWithAPool(t *testing.T) {
	for _, tt := range []struct {
		accounts []string
		line     bool
	}{
		{accounts: []string{"work"}},
		{accounts: []string{"work", "personal"}, line: true},
	} {
		f := graphsFrame(160, 37, GraphsSections{})
		f.Graphs.Accounts = configured(tt.accounts...)
		text := strings.Join(textOf(f.Draw(threeRouted(), now)), "\n")
		if l := f.graphsLaid(); (l.poolLine >= 0) != tt.line || strings.Contains(text, "Σ pool") != tt.line {
			t.Errorf("of %d accounts, the pool's line is at %d, the page reads\n%s\nwant a line %t", len(tt.accounts), l.poolLine, text, tt.line)
		}
	}
}

// Cards lie across as the design's rule has them: two side by side at the
// most, where both are 35 cells or more, 4 apart, ⌊(w + 4) / k⌋ − 4 each, the
// odd cells left at the end, the rest on the rows under, in the same columns;
// one account's card across the width; and where two don't fit, a card
// across the width.
func TestGraphsCardsLieAcrossByTheDesignsRule(t *testing.T) {
	two := func(each int) []flex.Span { return []flex.Span{{At: 0, Size: each}, {At: each + 4, Size: each}} }
	tests := []struct {
		width, accounts int
		want            []flex.Span
	}{
		{width: 160, accounts: 1, want: []flex.Span{{At: 0, Size: 160}}},
		{width: 160, accounts: 2, want: two(78)},
		{width: 160, accounts: 3, want: two(78)},
		{width: 160, accounts: 4, want: two(78)},
		{width: 160, accounts: 5, want: two(78)},
		{width: 300, accounts: 4, want: two(148)},
		{width: 173, accounts: 3, want: two(84)},
		{width: 108, accounts: 3, want: two(52)},
		{width: 74, accounts: 3, want: two(35)},
		{width: 73, accounts: 3, want: []flex.Span{{At: 0, Size: 73}}},
		{width: 52, accounts: 3, want: []flex.Span{{At: 0, Size: 52}}},
		{width: 30, accounts: 1, want: []flex.Span{{At: 0, Size: 30}}},
	}
	for _, tt := range tests {
		if got := cardsAcross(tt.accounts, tt.width); !slices.Equal(got, tt.want) {
			t.Errorf("%d cards across %d columns lie at %v, want %v", tt.accounts, tt.width, got, tt.want)
		}
	}
}

// Each card lies in its column of its row of cards, a blank row between two
// rows: its label; its words on a row of their own and a blank row under
// them, or on the label's; then its chart and its axis. An odd one last lies
// under the first, as wide, so every card is the same size.
func TestGraphsPlacesEachCardsRowsUnderItsSectionsHeading(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		accounts      []string
		least         bool
		want          []cardLaid
	}{
		{name: "one, the words apart, across the width", width: 160, height: 37, accounts: []string{"work"}, want: []cardLaid{
			{label: 2, words: 3, chart: bounds{x: 0, y: 5, width: 160, rows: 20}},
		}},
		{name: "three, the words apart, the third under the first", width: 160, height: 37, accounts: []string{"work", "personal", "side"}, want: []cardLaid{
			{label: 2, words: 3, chart: bounds{x: 0, y: 5, width: 78, rows: 9}},
			{label: 2, words: 3, chart: bounds{x: 82, y: 5, width: 78, rows: 9}},
			{label: 15, words: 16, chart: bounds{x: 0, y: 18, width: 78, rows: 9}},
		}},
		{name: "three, the words on the labels' rows", width: 108, height: 44, accounts: []string{"work", "personal", "side"}, least: true, want: []cardLaid{
			{label: 2, words: 2, chart: bounds{x: 0, y: 3, width: 52, rows: 6}},
			{label: 2, words: 2, chart: bounds{x: 56, y: 3, width: 52, rows: 6}},
			{label: 10, words: 10, chart: bounds{x: 0, y: 11, width: 52, rows: 6}},
		}},
		{name: "five, rows of two, the fifth under the first", width: 160, height: 37, accounts: []string{"work", "personal", "side", "team", "client"}, least: true, want: []cardLaid{
			{label: 2, words: 2, chart: bounds{x: 0, y: 3, width: 78, rows: 6}},
			{label: 2, words: 2, chart: bounds{x: 82, y: 3, width: 78, rows: 6}},
			{label: 10, words: 10, chart: bounds{x: 0, y: 11, width: 78, rows: 6}},
			{label: 10, words: 10, chart: bounds{x: 82, y: 11, width: 78, rows: 6}},
			{label: 18, words: 18, chart: bounds{x: 0, y: 19, width: 78, rows: 6}},
		}},
	}
	for _, tt := range tests {
		f := graphsFrame(tt.width, tt.height, GraphsSections{})
		f.Graphs.Accounts = configured(tt.accounts...)
		l := f.graphsLaid()
		if tt.least {
			l = l.laidAt(graphsSteps()[0])
		}
		if got := l.cards(l.sections[0]); !slices.Equal(got, tt.want) {
			t.Errorf("%s: WEEK's cards lie at %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// changedRouted is threeRouted with every account's every reading changed,
// and new sessions pinned to two of them, so every word the page says of them
// changes.
func changedRouted() status.Document {
	return status.Document{
		GeneratedAt: now.UTC(), Source: status.SourceRouter, Best: "personal", Router: status.Health{Healthy: true},
		Pin:      status.Pin{Accounts: []string{"personal", "side"}, Since: now.UTC()},
		Accounts: []status.Account{limitedAccount("work"), readAccount("personal", sessionOf(0.91, time.Hour), weekOf(0.05, 6*quota.Day)), pressedAccount("side")},
	}.WorkedOut(claudeLike)
}

// The layout changes only on a key, a resize or the config, never as the
// data does: a read that changes every word, and pins the pool to two of its
// three accounts, leaves every heading, the pool's line and the legend on
// their rows, at 120 columns, each heading at a row's start; at 173, where
// WHEN YOU WORK's heading is on GLANCE's row, beside it; at 68 columns too,
// where POOL's key of the two pinned would take a row fewer than of the
// three configured.
func TestGraphsLayoutHoldsStillAsTheDataChanges(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	marks := []string{"WEEK ", "POOL ", "GLANCE ", "WHEN YOU WORK ", "Σ pool", "▆▆ room left"}
	for _, width := range []int{120, 173, 68} {
		f := graphsFrame(width, 120, everyGraphsSection())
		f.Look = look
		if width == 68 && poolKeyed(configured("personal", "side"), look).keyRows(width) == poolKeyed(f.Graphs.Accounts, look).keyRows(width) {
			t.Fatalf("at %d columns, POOL's key of two accounts takes the rows it does of three; want a width they differ at", width)
		}
		rowsOf := func(doc status.Document) []int {
			text := textOf(f.Draw(doc, now))
			var rows []int
			for _, mark := range marks {
				rows = append(rows, slices.IndexFunc(text, func(row string) bool {
					if width == 173 && mark == "WHEN YOU WORK " {
						return strings.HasPrefix(row, "GLANCE ") && strings.Contains(row, " "+mark)
					}
					return strings.HasPrefix(row, mark)
				}))
			}
			return rows
		}
		before, after := rowsOf(threeRouted()), rowsOf(changedRouted())
		if !slices.Equal(before, after) || slices.Contains(before, -1) {
			t.Errorf("at %d columns, the headings, the pool's line and the legend are on rows %v, then after a read on %v; want them on the same rows, each drawn", width, before, after)
		}
	}
}
