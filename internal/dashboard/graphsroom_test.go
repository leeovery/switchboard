package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

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

// The design's worked sizes, with WEEK and 5-HOUR shown: at 160 by 37, three
// accounts' words have rows of their own and the charts 7 rows; at 108 by 44
// the words share the labels' rows and the charts take 5, the sections
// filling the rows over the legend exactly; on a phone of 73 by 61, the same,
// a card a row; and with four accounts on that phone, the page scrolls,
// laid at the least.
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
		{name: "160 by 37", width: 160, height: 37, accounts: []string{"work", "personal", "side"}, apart: true, chart: 7, at: []int{0, 14}, poolLine: 28, rows: 29, room: 30},
		{name: "108 by 44", width: 108, height: 44, accounts: []string{"work", "personal", "side"}, chart: 5, at: []int{0, 18}, poolLine: 36, rows: 37, room: 37},
		{name: "a phone of 73 by 61", width: 73, height: 61, accounts: []string{"work", "personal", "side"}, chart: 5, at: []int{0, 26}, poolLine: 52, rows: 53, room: 53},
		{name: "four accounts on that phone", width: 73, height: 61, accounts: []string{"work", "personal", "side", "team"}, chart: 5, at: []int{0, 34}, poolLine: 68, rows: 69, room: 53},
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

// Each section takes its heading's rows and its own, as what draws it
// counts them: a burn-down's to the foot of its last card's axis; POOL's as
// its stack takes them; GLANCE's and WHEN YOU WORK's as their bodies count
// them, of the configured accounts and whether they're a pool. So at the
// design's sections frame, 173 by 53, with WEEK, POOL, GLANCE and WHEN YOU
// WORK shown, and on a phone, of one account and of three.
func TestGraphsHoldsEachSectionToItsOwnRows(t *testing.T) {
	shown := GraphsSectionsNamed([]string{"week", "pool", "glance", "when"})
	tests := []struct {
		width, height int
		accounts      []string
	}{
		{width: 173, height: 53, accounts: []string{"work", "personal", "side"}},
		{width: 73, height: 61, accounts: []string{"work", "personal", "side"}},
		{width: 73, height: 61, accounts: []string{"work"}},
	}
	for _, tt := range tests {
		f := graphsFrame(tt.width, tt.height, shown)
		f.Graphs.Accounts = configured(tt.accounts...)
		l := f.graphsLaid()
		for _, s := range l.sections {
			var own int
			switch s.section {
			case GraphsPool:
				own = l.pool.rows(l.width, l.chart)
			case GraphsGlance:
				own = glanceRows(len(tt.accounts), len(tt.accounts) > 1)
			case GraphsWhen:
				own = whenRows()
			default:
				cards := l.cards(s)
				last := cards[len(cards)-1].chart
				own = last.y + last.rows - s.at - sectionHead
			}
			if s.rows != sectionHead+own {
				t.Errorf("at %dx%d, of %d accounts, %s takes %d rows, want %d, its heading's and its own %d", tt.width, tt.height, len(tt.accounts), s.section.heading(), s.rows, sectionHead+own, own)
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

// Rows to spare go first to the cards' words, a row of their own in both
// burn-down sections at once, then to every chart, the cards' and POOL's,
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
				if !burnsDown(s.section) {
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

// cardChartRows is how tall the burn-down section's cards' charts are, as
// laid out: -1 where they differ.
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
// no room, a blank row between two; the pool's line after the last
// burn-down section shown, after its blank row, and a blank row before the
// next; and the legend while a burn-down section shows, none else.
func TestGraphsLaysOutEverySectionShownAndNoneHidden(t *testing.T) {
	for mask := 1; mask < 1<<len(EveryGraphsSection()); mask++ {
		shown := sectionsNamed(mask)
		name := strings.Join(shown.Names(), ", ")
		l := graphsFrame(160, 200, shown).graphsLaid()
		lastBurn := -1
		var laid []string
		for i, s := range l.sections {
			laid = append(laid, wordingOf(s.section).name)
			if burnsDown(s.section) {
				lastBurn = i
			}
		}
		if !slices.Equal(laid, shown.Names()) {
			t.Errorf("%s: laid out %q, want those shown, in order", name, laid)
		}
		next, poolLine := 0, -1
		for i, s := range l.sections {
			if s.at != next {
				t.Errorf("%s: %s at row %d, want %d", name, s.section.heading(), s.at, next)
			}
			next = s.at + s.rows + 1
			if i == lastBurn {
				poolLine, next = next, next+2
			}
		}
		if l.poolLine != poolLine || l.rows != next-1 {
			t.Errorf("%s: the pool's line at %d, %d rows; want it at %d, %d rows", name, l.poolLine, l.rows, poolLine, next-1)
		}
		if got, want := len(l.legend), min(lastBurn+1, 1); got != want {
			t.Errorf("%s: the legend takes %d rows, want %d", name, got, want)
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

// Cards lie across as the design's rule has them: as many side by side as
// are 35 cells or more, 4 apart, ⌊(w + 4) / k⌋ − 4 each, the odd cells left
// at the end, the rest on the rows under, in the same columns; narrower than
// one, a card across the width.
func TestGraphsCardsLieAcrossByTheDesignsRule(t *testing.T) {
	tests := []struct {
		width, accounts int
		want            []flex.Span
	}{
		{width: 160, accounts: 3, want: []flex.Span{{At: 0, Size: 50}, {At: 54, Size: 50}, {At: 108, Size: 50}}},
		{width: 173, accounts: 3, want: []flex.Span{{At: 0, Size: 55}, {At: 59, Size: 55}, {At: 118, Size: 55}}},
		{width: 160, accounts: 4, want: []flex.Span{{At: 0, Size: 37}, {At: 41, Size: 37}, {At: 82, Size: 37}, {At: 123, Size: 37}}},
		{width: 108, accounts: 3, want: []flex.Span{{At: 0, Size: 52}, {At: 56, Size: 52}}},
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
// them, or on the label's; then its chart and its axis.
func TestGraphsPlacesEachCardsRowsUnderItsSectionsHeading(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		want          []cardLaid
	}{
		{name: "the words apart, three across", width: 160, height: 37, want: []cardLaid{
			{label: 2, words: 3, chart: bounds{x: 0, y: 5, width: 50, rows: 8}},
			{label: 2, words: 3, chart: bounds{x: 54, y: 5, width: 50, rows: 8}},
			{label: 2, words: 3, chart: bounds{x: 108, y: 5, width: 50, rows: 8}},
		}},
		{name: "the words on the labels' rows, two across", width: 108, height: 44, want: []cardLaid{
			{label: 2, words: 2, chart: bounds{x: 0, y: 3, width: 52, rows: 6}},
			{label: 2, words: 2, chart: bounds{x: 56, y: 3, width: 52, rows: 6}},
			{label: 10, words: 10, chart: bounds{x: 0, y: 11, width: 52, rows: 6}},
		}},
	}
	for _, tt := range tests {
		l := graphsFrame(tt.width, tt.height, GraphsSections{}).graphsLaid()
		if got := l.cards(l.sections[0]); !slices.Equal(got, tt.want) {
			t.Errorf("%s: WEEK's cards lie at %+v, want %+v", tt.name, got, tt.want)
		}
		fiveHour := l.sections[1]
		if got := l.cards(fiveHour)[0].label; got != fiveHour.at+sectionHead {
			t.Errorf("%s: 5-HOUR's first label is on row %d, want %d, under its heading's blank row", tt.name, got, fiveHour.at+sectionHead)
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
// their rows; at 75 columns too, where POOL's key of the two pinned would
// take a row fewer than of the three configured.
func TestGraphsLayoutHoldsStillAsTheDataChanges(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	marks := []string{"WEEK ", "5-HOUR ", "POOL ", "GLANCE ", "WHEN YOU WORK ", "Σ pool", "▆▆ room left"}
	for _, width := range []int{160, 75} {
		f := graphsFrame(width, 120, everyGraphsSection())
		f.Look = look
		if width == 75 && poolKeyed(configured("personal", "side"), look).keyRows(width) == poolKeyed(f.Graphs.Accounts, look).keyRows(width) {
			t.Fatalf("at %d columns, POOL's key of two accounts takes the rows it does of three; want a width they differ at", width)
		}
		rowsOf := func(doc status.Document) []int {
			text := textOf(f.Draw(doc, now))
			var rows []int
			for _, mark := range marks {
				rows = append(rows, slices.IndexFunc(text, func(row string) bool { return strings.HasPrefix(row, mark) }))
			}
			return rows
		}
		before, after := rowsOf(threeRouted()), rowsOf(changedRouted())
		if !slices.Equal(before, after) || slices.Contains(before, -1) {
			t.Errorf("at %d columns, the headings, the pool's line and the legend are on rows %v, then after a read on %v; want them on the same rows, each drawn", width, before, after)
		}
	}
}
