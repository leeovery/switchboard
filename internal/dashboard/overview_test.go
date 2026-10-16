package dashboard

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

func TestTheOverviewShowsUSAGEROUTINGCOMINGUPAndLATELYAtFirst(t *testing.T) {
	var shown Sections
	if got, want := shown.Names(), []string{"usage", "routing", "coming", "lately"}; !slices.Equal(got, want) {
		t.Errorf("the zero Sections show %q, want %q", got, want)
	}
	for _, s := range []Section{SectionGraphs, SectionSessions} {
		if shown.Has(s) {
			t.Errorf("the zero Sections show %s, want it hidden at first", s.heading())
		}
	}
}

func TestASectionsKeyShowsItWhereItsHiddenAndHidesItWhereItShows(t *testing.T) {
	var shown Sections
	for _, s := range EverySection() {
		toggled := shown.Toggled(s)
		if toggled.Has(s) == shown.Has(s) {
			t.Errorf("toggling %s left it showing: %v", s.heading(), toggled.Has(s))
		}
		for _, other := range EverySection() {
			if other != s && toggled.Has(other) != shown.Has(other) {
				t.Errorf("toggling %s changed %s too", s.heading(), other.heading())
			}
		}
		if again := toggled.Toggled(s); again != shown {
			t.Errorf("toggling %s twice shows %q, want %q, as before", s.heading(), again.Names(), shown.Names())
		}
	}
}

func TestTheSectionsShownAreNamedAsThePreferencesKeepThem(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  []string
	}{
		{name: "none kept: those shown at first", names: nil, want: []string{"usage", "routing", "coming", "lately"}},
		{name: "none shown, kept as none", names: []string{}, want: []string{}},
		{name: "those kept, in the page's order", names: []string{"lately", "graphs", "usage"}, want: []string{"usage", "graphs", "lately"}},
		{name: "every one", names: []string{"usage", "graphs", "routing", "sessions", "coming", "lately"}, want: []string{"usage", "graphs", "routing", "sessions", "coming", "lately"}},
		{name: "a name of none, as a later switchboard's, left out", names: []string{"sessions", "a-later-section"}, want: []string{"sessions"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SectionsNamed(tt.names).Names()
			if got == nil || !slices.Equal(got, tt.want) {
				t.Errorf("SectionsNamed(%#v).Names() = %#v, want %#v", tt.names, got, tt.want)
			}
			if again := SectionsNamed(got); again != SectionsNamed(tt.names) {
				t.Errorf("named again, %q show %q", got, again.Names())
			}
		})
	}
}

func TestEachSectionIsShownAndHiddenByItsKey(t *testing.T) {
	tests := []struct {
		key  string
		want Section
		word string
	}{
		{key: "u", want: SectionUsage, word: "usage"},
		{key: "g", want: SectionGraphs, word: "graphs"},
		{key: "r", want: SectionRouting, word: "routing"},
		{key: "s", want: SectionSessions, word: "sessions"},
		{key: "c", want: SectionComing, word: "coming up"},
		{key: "l", want: SectionLately, word: "lately"},
	}
	for _, tt := range tests {
		got, ok := SectionKeyed(tt.key)
		if !ok || got != tt.want || got.Key() != tt.key || got.Word() != tt.word {
			t.Errorf("SectionKeyed(%q) = %s, %v, its key %q and word %q; want %s, keyed %q, worded %q", tt.key, got.heading(), ok, got.Key(), got.Word(), tt.want.heading(), tt.key, tt.word)
		}
	}
	for _, key := range []string{"b", "U", "w", "", "us"} {
		if got, ok := SectionKeyed(key); ok {
			t.Errorf("SectionKeyed(%q) = %s, want no section", key, got.heading())
		}
	}
}

func TestEachSectionsLeastAndMostRows(t *testing.T) {
	doc := threeRouted()
	at := func(width int, by By) Frame {
		f := frameOf(width, 50)
		f.By = by
		return f
	}
	running := at(160, ByWindow)
	running.Sessions = flippingSessions()
	phone := at(52, ByWindow)
	phone.Sessions = flippingSessions()
	tests := []struct {
		name        string
		section     section
		least, most int
	}{
		{name: "USAGE from 150 columns: the compact matrix, 2a + 7, to the big bars, a window's heading, its accounts and the pool", section: at(160, ByWindow).usageSection(doc, now), least: 13, most: 24},
		{name: "USAGE under 150 columns: stacked, as usage is, its key line's marks on a line of their own", section: at(149, ByWindow).usageSection(doc, now), least: 25, most: 25},
		{name: "USAGE by account: a block an account", section: at(160, ByAccount).usageSection(doc, now), least: 22, most: 22},
		{name: "GRAPHS: h + 4, 5 rows of chart to 19", section: at(160, ByWindow).graphsSection(doc, now), least: 9, most: 23},
		{name: "ROUTING: a + 3, to 3a + 2", section: at(160, ByWindow).routingSection(doc, now), least: 6, most: 11},
		{name: "ROUTING by account under 150 columns: a section of its own", section: at(149, ByAccount).routingSection(doc, now), least: 6, most: 11},
		{name: "ROUTING by account from 150 columns: beside USAGE, no rows of its own", section: at(150, ByAccount).routingSection(doc, now), least: 0, most: 0},
		{name: "ROUTING under 110 columns: two rows an account, 3a + 1", section: at(109, ByWindow).routingSection(doc, now), least: 10, most: 10},
		{name: "SESSIONS: a row saying none runs", section: at(160, ByWindow).sessionsSection(doc, now), least: 3, most: 3},
		{name: "SESSIONS: a row a session on each account its models go to", section: running.sessionsSection(doc, now), least: 8, most: 8},
		{name: "SESSIONS on a phone: two rows a session", section: phone.sessionsSection(doc, now), least: 14, most: 14},
	}
	for _, tt := range tests {
		if tt.section.least != tt.least || tt.section.most != tt.most {
			t.Errorf("%s: least %d, most %d; want %d, %d", tt.name, tt.section.least, tt.section.most, tt.least, tt.most)
		}
	}
}

func TestCOMINGUPAndLATELYTakeTheirLinesAndTwoFromThreeToTwelve(t *testing.T) {
	tests := []struct {
		lines, want int
	}{
		{lines: 0, want: 5},
		{lines: 3, want: 5},
		{lines: 7, want: 9},
		{lines: 12, want: 14},
		{lines: 20, want: 14},
	}
	for _, tt := range tests {
		if got := linesRows(tt.lines); got != tt.want {
			t.Errorf("linesRows(%d) = %d, want %d", tt.lines, got, tt.want)
		}
	}
}

func TestTheOverviewDrawsTheSectionsShownInTheirOrder(t *testing.T) {
	tests := []struct {
		name  string
		shown Sections
		want  []string
	}{
		{name: "at first", want: []string{"USAGE", "ROUTING", "COMING UP", "LATELY"}},
		{name: "GRAPHS shown, ROUTING hidden", shown: Sections{}.Toggled(SectionGraphs).Toggled(SectionRouting), want: []string{"USAGE", "GRAPHS", "COMING UP", "LATELY"}},
		{name: "SESSIONS alone", shown: SectionsNamed([]string{"sessions"}), want: []string{"SESSIONS"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 80)
			f.Sections = tt.shown
			var headings []string
			for _, row := range f.Draw(threeRouted(), now) {
				for _, s := range EverySection() {
					if strings.Contains(row, s.heading()+"  ") {
						headings = append(headings, s.heading())
					}
				}
			}
			if !slices.Equal(headings, tt.want) {
				t.Errorf("the Overview's headings are %q, want %q", headings, tt.want)
			}
		})
	}
}

// planOf lays the sections out afresh in rows rows from row top, as the
// frame's planner lays the Overview's out where it keeps no layout.
func planOf(f Frame, sections []section, top, rows int) []planned {
	return f.place(allot(f.unitsOf(sections), rows), top)
}

// sized is a section of the kind given that takes least rows to most, its
// body drawing nothing: one that lists lines, holding a line each row of its
// most but its heading's.
func sized(s Section, least, most int) section {
	sized := section{Section: s, least: least, most: most, draw: func(*canvas, bounds) {}}
	if s.lists() {
		sized.lines = func() int { return most - sectionHead }
	}
	return sized
}

// lastRow is the last row the section takes as it's laid out, counted from 0:
// its heading's where it folds.
func (p planned) lastRow() int {
	if p.fold {
		return p.head.y
	}
	return p.body.y + p.body.rows - 1
}

// planWorld is a frame's world as the planner sees it: its accounts and windows,
// the sessions running, and the lines COMING UP and LATELY have.
type planWorld struct {
	accounts, windows, running, coming, lately int
}

// The frames' worlds: auto's, three accounts, as the pinned, bad-hour and
// limit worlds have too; and the many worlds'.
var (
	autoWorld = planWorld{accounts: 3, windows: 3, running: 6, coming: 7, lately: 12}
	sixWorld  = planWorld{accounts: 6, windows: 3, coming: 3, lately: 3}
	tenWorld  = planWorld{accounts: 10, windows: 3, coming: 3, lately: 3}
)

// sections are the sections shown of a frame of the world, width columns
// wide, grouped by by, sized as the design counts their rows: USAGE from its
// compact matrix, 2a + 7, to its big bars, 2 + w(2a + 5); by account, a block
// an account and the pool's, ROUTING beside them from column 107; on a phone,
// stacked, its key on two lines; GRAPHS h + 4, h from 5 to 19; ROUTING a + 3
// to 3a + 2, and on a phone 3a + 1; SESSIONS a row a session, two on a
// phone; and COMING UP and LATELY a row a line, from 3 to 12; each with its
// heading's two rows.
func (w planWorld) sections(width int, by By, shown Sections) []section {
	a, k := w.accounts, w.windows
	usage := sized(SectionUsage, 2*a+7, 2+k*(2*a+5))
	routing := sized(SectionRouting, a+3, 3*a+2)
	sessions := sized(SectionSessions, 2+w.running, 2+w.running)
	switch {
	case by == ByAccount:
		blocks := 2 + (a+1)*(2*k-1) + 2*a + 2
		usage, routing = sized(SectionUsage, blocks, blocks), sized(SectionRouting, 0, 0)
		usage.aside = 107
	case width < phoneUnder:
		usage = sized(SectionUsage, usage.most+1, usage.most+1)
		routing = sized(SectionRouting, 3*a+1, 3*a+1)
		sessions = sized(SectionSessions, 2+2*w.running, 2+2*w.running)
	}
	lines := func(s Section, n int) section {
		return section{Section: s, least: 2 + 3, lines: func() int { return n }, draw: func(*canvas, bounds) {}}
	}
	all := []section{usage, sized(SectionGraphs, 2+1+5+1, 2+1+19+1), routing, sessions, lines(SectionComing, w.coming), lines(SectionLately, w.lately)}
	return slices.DeleteFunc(all, func(s section) bool { return !shown.Has(s.Section) })
}

// rowsOf is where a section is laid out, counted from 1 as the frames count
// rows: its first row, its heading's, and its last; and whether it folds.
type rowsOf struct {
	section  Section
	from, to int
	fold     bool
}

// The Overview's frames' sections, toggled with the keys pressed.
var (
	gs    = Sections{}.Toggled(SectionGraphs).Toggled(SectionSessions)
	every = SectionsNamed([]string{"usage", "graphs", "routing", "sessions", "coming", "lately"})
)

func TestThePlannerLaysEachOverviewFrameOutOnItsRows(t *testing.T) {
	defaults := []rowsOf{{SectionUsage, 5, 17, false}, {SectionRouting, 19, 29, false}, {SectionComing, 31, 35, false}, {SectionLately, 31, 35, false}}
	tests := []struct {
		stem          string
		width, height int
		by            By
		shown         Sections
		world         planWorld
		want          []rowsOf
	}{
		{stem: "overview-default-160", width: 160, height: 37, world: autoWorld, want: defaults},
		{stem: "overview-pinned-160", width: 160, height: 37, world: autoWorld, want: defaults},
		{stem: "overview-bad-hour-160", width: 160, height: 37, world: autoWorld, want: defaults},
		{stem: "routing-auto-160", width: 160, height: 37, world: autoWorld, want: defaults},
		{stem: "routing-two-160", width: 160, height: 37, world: autoWorld, want: defaults},
		{stem: "routing-move-160", width: 160, height: 37, world: autoWorld, want: defaults},
		{stem: "routing-limit-160", width: 160, height: 37, world: autoWorld, want: defaults},
		{stem: "overview-by-account-160", width: 160, height: 42, by: ByAccount, world: autoWorld, want: []rowsOf{
			{SectionUsage, 5, 34, false}, {SectionRouting, 5, 34, false}, {SectionComing, 36, 40, false}, {SectionLately, 36, 40, false},
		}},
		{stem: "overview-usage-alone-160", width: 160, height: 37, shown: SectionsNamed([]string{"usage"}), world: autoWorld, want: []rowsOf{
			{SectionUsage, 5, 17, false},
		}},
		{stem: "overview-everything-160x39", width: 160, height: 39, shown: gs, world: autoWorld, want: []rowsOf{
			{SectionUsage, 5, 17, false}, {SectionGraphs, 19, 27, false}, {SectionRouting, 29, 34, false},
			{SectionSessions, 36, 36, true}, {SectionComing, 37, 37, true}, {SectionLately, 37, 37, true},
		}},
		{stem: "overview-six-accounts-160", width: 160, height: 41, world: sixWorld, want: []rowsOf{
			{SectionUsage, 5, 23, false}, {SectionRouting, 25, 33, false}, {SectionComing, 35, 39, false}, {SectionLately, 35, 39, false},
		}},
		{stem: "overview-ten-accounts-160", width: 160, height: 53, world: tenWorld, want: []rowsOf{
			{SectionUsage, 5, 31, false}, {SectionRouting, 33, 45, false}, {SectionComing, 47, 51, false}, {SectionLately, 47, 51, false},
		}},
		{stem: "overview-default-216", width: 216, height: 70, world: autoWorld, want: []rowsOf{
			{SectionUsage, 5, 39, false}, {SectionRouting, 41, 51, false}, {SectionComing, 53, 66, false}, {SectionLately, 53, 66, false},
		}},
		{stem: "overview-sessions-216", width: 216, height: 70, shown: Sections{}.Toggled(SectionSessions), world: autoWorld, want: []rowsOf{
			{SectionUsage, 5, 39, false}, {SectionRouting, 41, 51, false}, {SectionSessions, 53, 60, false}, {SectionComing, 62, 68, false}, {SectionLately, 62, 68, false},
		}},
		{stem: "overview-sessions-graphs-216", width: 216, height: 70, shown: gs, world: autoWorld, want: []rowsOf{
			{SectionUsage, 5, 17, false}, {SectionGraphs, 19, 41, false}, {SectionRouting, 43, 53, false},
			{SectionSessions, 55, 62, false}, {SectionComing, 64, 68, false}, {SectionLately, 64, 68, false},
		}},
		{stem: "overview-everything-216x106", width: 216, height: 106, shown: gs, world: autoWorld, want: []rowsOf{
			{SectionUsage, 5, 39, false}, {SectionGraphs, 41, 63, false}, {SectionRouting, 65, 75, false},
			{SectionSessions, 77, 84, false}, {SectionComing, 86, 99, false}, {SectionLately, 86, 99, false},
		}},
		{stem: "overview-phone-52", width: 52, height: 66, world: autoWorld, want: []rowsOf{
			{SectionUsage, 6, 41, false}, {SectionRouting, 43, 52, false}, {SectionComing, 54, 58, false}, {SectionLately, 60, 64, false},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, View: Overview, By: tt.by}
			laid := planOf(f, tt.world.sections(tt.width, tt.by, tt.shown), f.top(), f.viewRows())
			var got []rowsOf
			for _, p := range laid {
				got = append(got, rowsOf{p.Section, p.head.y + 1, p.lastRow() + 1, p.fold})
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("laid out\n  %v\nwant\n  %v", got, tt.want)
			}
			heldToItsText(t, tt.stem, laid)
		})
	}
}

// heldToItsText holds the sections as they're laid out to the text of the
// frame they're laid out for: each heading's name where its heading starts,
// and its key, after a rule, at its last column; and every row under the
// last section blank, to the key line.
func heldToItsText(t *testing.T, stem string, laid []planned) {
	t.Helper()
	rows := referenceRows(t, stem, "txt")
	foot := 0
	for _, p := range laid {
		row := []rune(rows[p.head.y] + strings.Repeat(" ", p.head.x+p.head.width))
		name, key := []rune(p.heading()), p.head.x+p.head.width-1
		if got := string(row[p.head.x : p.head.x+len(name)]); got != string(name) {
			t.Errorf("%s's heading is laid out from column %d of row %d, where the frame has %q", string(name), p.head.x+1, p.head.y+1, got)
		}
		if got, want := string(row[key-2:key+1]), "─ "+p.Key(); got != want {
			t.Errorf("%s's key is laid out at column %d of row %d, where the frame has %q, want %q", string(name), key+1, p.head.y+1, got, want)
		}
		foot = max(foot, p.lastRow()+1)
	}
	for y := foot; y < len(rows)-1; y++ {
		if strings.TrimSpace(rows[y]) != "" {
			t.Errorf("row %d reads %q, under the last section laid out, on row %d: want it empty", y+1, rows[y], foot)
		}
	}
}

// folds are the sections of those laid out that fold.
func folds(laid []planned) []Section {
	var folded []Section
	for _, p := range laid {
		if p.fold {
			folded = append(folded, p.Section)
		}
	}
	return folded
}

// rowsGiven are the rows each section of those laid out takes, its
// heading's among them, by its kind: its heading's alone where it folds.
func rowsGiven(laid []planned) map[Section]int {
	given := make(map[Section]int)
	for _, p := range laid {
		given[p.Section] = p.lastRow() - p.head.y + 1
	}
	return given
}

func TestRowsShortSectionsFoldSESSIONSThenCOMINGUPAndLATELYThenGRAPHSThenROUTINGAndUSAGEScrollsLast(t *testing.T) {
	sections := []section{
		sized(SectionUsage, 13, 35), sized(SectionGraphs, 9, 23), sized(SectionRouting, 6, 11),
		sized(SectionSessions, 8, 8), sized(SectionComing, 5, 9), sized(SectionLately, 5, 14),
	}
	tests := []struct {
		rows  int
		folds []Section
		usage int
	}{
		{rows: 45, usage: 13},
		{rows: 44, folds: []Section{SectionSessions}, usage: 13},
		{rows: 38, folds: []Section{SectionSessions}, usage: 13},
		{rows: 37, folds: []Section{SectionSessions, SectionComing, SectionLately}, usage: 13},
		{rows: 33, folds: []Section{SectionSessions, SectionComing, SectionLately}, usage: 13},
		{rows: 32, folds: []Section{SectionGraphs, SectionSessions, SectionComing, SectionLately}, usage: 13},
		{rows: 25, folds: []Section{SectionGraphs, SectionSessions, SectionComing, SectionLately}, usage: 13},
		{rows: 24, folds: []Section{SectionGraphs, SectionRouting, SectionSessions, SectionComing, SectionLately}, usage: 13},
		{rows: 18, folds: []Section{SectionGraphs, SectionRouting, SectionSessions, SectionComing, SectionLately}, usage: 13},
		{rows: 17, folds: []Section{SectionGraphs, SectionRouting, SectionSessions, SectionComing, SectionLately}, usage: 12},
		{rows: 6, folds: []Section{SectionGraphs, SectionRouting, SectionSessions, SectionComing, SectionLately}, usage: sectionHead},
	}
	for _, tt := range tests {
		laid := planOf(frameOf(160, 50), sections, 0, tt.rows)
		if got := folds(laid); !slices.Equal(got, tt.folds) {
			t.Errorf("in %d rows, %v fold, want %v", tt.rows, got, tt.folds)
		}
		if got := rowsGiven(laid)[SectionUsage]; got != tt.usage {
			t.Errorf("in %d rows, USAGE takes %d, want %d", tt.rows, got, tt.usage)
		}
		if last := slices.MaxFunc(laid, func(a, b planned) int { return a.lastRow() - b.lastRow() }); last.lastRow() >= tt.rows && tt.usage > sectionHead {
			t.Errorf("in %d rows, %s ends on row %d, past them", tt.rows, last.heading(), last.lastRow()+1)
		}
	}
}

func TestNothingGrowsWhileAnythingIsFoldedAndTheRowsOverStayEmptyAtTheFoot(t *testing.T) {
	sections := []section{
		sized(SectionUsage, 13, 35), sized(SectionGraphs, 9, 23), sized(SectionRouting, 6, 11),
		sized(SectionSessions, 8, 8), sized(SectionComing, 5, 9), sized(SectionLately, 5, 14),
	}
	laid := planOf(frameOf(160, 50), sections, 0, 44)
	want := map[Section]int{SectionUsage: 13, SectionGraphs: 9, SectionRouting: 6, SectionSessions: 1, SectionComing: 5, SectionLately: 5}
	if got := rowsGiven(laid); !maps.Equal(got, want) {
		t.Errorf("in 44 rows, SESSIONS folded, the sections take %v, want their least, %v", got, want)
	}
	if last := laid[len(laid)-1].lastRow(); last != 37 {
		t.Errorf("the last section ends on row %d, want 38, the 6 rows over empty under it", last+1)
	}
}

func TestRowsToSpareGoToGRAPHSThenTheBigBarsThenROUTINGWhereEachFitsThenCOMINGUPAndLATELY(t *testing.T) {
	sections := []section{
		sized(SectionUsage, 13, 35), sized(SectionGraphs, 9, 23), sized(SectionRouting, 6, 11),
		sized(SectionComing, 5, 9), sized(SectionLately, 5, 14),
	}
	tests := []struct {
		name                           string
		rows                           int
		usage, graphs, routing, coming int
	}{
		{name: "none to spare: each at its least", rows: 36, usage: 13, graphs: 9, routing: 6, coming: 5},
		{name: "GRAPHS a row at a time", rows: 40, usage: 13, graphs: 13, routing: 6, coming: 5},
		{name: "GRAPHS to 19 rows of chart", rows: 50, usage: 13, graphs: 23, routing: 6, coming: 5},
		{name: "the big bars not fitting, ROUTING's two rows of trace", rows: 55, usage: 13, graphs: 23, routing: 11, coming: 5},
		{name: "then COMING UP and LATELY a line at a time", rows: 60, usage: 13, graphs: 23, routing: 11, coming: 10},
		{name: "the big bars fitting, before ROUTING", rows: 72, usage: 35, graphs: 23, routing: 6, coming: 5},
		{name: "ROUTING not fitting, COMING UP and LATELY growing past it", rows: 76, usage: 35, graphs: 23, routing: 6, coming: 9},
		{name: "ROUTING fitting after the big bars", rows: 77, usage: 35, graphs: 23, routing: 11, coming: 5},
		{name: "each at its most", rows: 86, usage: 35, graphs: 23, routing: 11, coming: 14},
		{name: "past every most", rows: 100, usage: 35, graphs: 23, routing: 11, coming: 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			laid := planOf(frameOf(160, 120), sections, 0, tt.rows)
			want := map[Section]int{SectionUsage: tt.usage, SectionGraphs: tt.graphs, SectionRouting: tt.routing, SectionComing: tt.coming, SectionLately: tt.coming}
			if got := rowsGiven(laid); !maps.Equal(got, want) {
				t.Errorf("in %d rows, the sections take %v, want %v", tt.rows, got, want)
			}
			if got, want := laid[len(laid)-1].lastRow()+1, min(tt.rows, 86); got != want {
				t.Errorf("in %d rows, the last section ends on row %d, want %d, the rest empty at the foot", tt.rows, got, want)
			}
		})
	}
}

func TestCOMINGUPAndLATELYSitSideBySideButOnAPhoneWhereLATELYIsUnderCOMINGUP(t *testing.T) {
	tests := []struct {
		name           string
		width          int
		shown          []Section
		coming, lately bounds
	}{
		{name: "at 160 columns", width: 160, shown: []Section{SectionComing, SectionLately}, coming: bounds{width: 72}, lately: bounds{x: 82, width: 78}},
		{name: "at 216 columns", width: 216, shown: []Section{SectionComing, SectionLately}, coming: bounds{width: 97}, lately: bounds{x: 110, width: 106}},
		{name: "COMING UP alone, at the full width", width: 160, shown: []Section{SectionComing}, coming: bounds{width: 160}},
		{name: "LATELY alone, at the full width", width: 160, shown: []Section{SectionLately}, lately: bounds{width: 160}},
		{name: "on a phone", width: 52, shown: []Section{SectionComing, SectionLately}, coming: bounds{width: 52}, lately: bounds{y: 6, width: 52}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sections []section
			for _, s := range tt.shown {
				sections = append(sections, sized(s, 5, 5))
			}
			for _, p := range planOf(frameOf(tt.width, 40), sections, 0, 20) {
				want := tt.coming
				if p.Section == SectionLately {
					want = tt.lately
				}
				head := bounds{x: p.head.x, y: p.head.y, width: p.head.width}
				body := bounds{x: p.body.x, y: p.body.y - sectionHead, width: p.body.width}
				if head != want || body != want {
					t.Errorf("%s's heading is at %+v, its body at %+v (as from its heading's row); want each at %+v", p.heading(), head, body, want)
				}
			}
		})
	}
}

func TestOnAPhoneCOMINGUPAndLATELYEachGrowALineAtATimeInTurnAndFoldTogether(t *testing.T) {
	sections := []section{sized(SectionComing, 5, 9), sized(SectionLately, 5, 14)}
	tests := []struct {
		rows           int
		coming, lately int
		fold           bool
	}{
		{rows: 11, coming: 5, lately: 5},
		{rows: 14, coming: 7, lately: 6},
		{rows: 30, coming: 9, lately: 14},
		{rows: 10, coming: 1, lately: 1, fold: true},
	}
	for _, tt := range tests {
		laid := planOf(frameOf(52, 40), sections, 0, tt.rows)
		given := rowsGiven(laid)
		if given[SectionComing] != tt.coming || given[SectionLately] != tt.lately || laid[0].fold != tt.fold || laid[1].fold != tt.fold {
			t.Errorf("in %d rows, COMING UP takes %d and LATELY %d, folded %v and %v; want %d and %d, folded %v",
				tt.rows, given[SectionComing], given[SectionLately], laid[0].fold, laid[1].fold, tt.coming, tt.lately, tt.fold)
		}
	}
}

func TestBesideUSAGEASectionOfNoRowsOfItsOwnSharesUSAGEsRowsAndElsewhereIsntLaidOut(t *testing.T) {
	usage := sized(SectionUsage, 30, 30)
	usage.aside = 107
	beside := []section{usage, sized(SectionGraphs, 9, 9), sized(SectionRouting, 0, 0)}
	laid := planOf(frameOf(160, 60), beside, 4, 50)
	want := []planned{
		{section: beside[0], head: bounds{y: 4, width: 104, rows: 1}, body: bounds{y: 6, width: 160, rows: 28}},
		{section: beside[2], head: bounds{x: 107, y: 4, width: 53, rows: 1}, body: bounds{x: 107, y: 6, width: 53, rows: 28}},
		{section: beside[1], head: bounds{y: 35, width: 160, rows: 1}, body: bounds{y: 37, width: 160, rows: 7}},
	}
	if len(laid) != len(want) {
		t.Fatalf("laid out %d sections, want %d", len(laid), len(want))
	}
	for i, p := range laid {
		if p.Section != want[i].Section || p.head != want[i].head || p.body != want[i].body {
			t.Errorf("%s's heading is at %+v and its body at %+v; want %+v and %+v", p.heading(), p.head, p.body, want[i].head, want[i].body)
		}
	}

	alone := planOf(frameOf(160, 60), []section{usage}, 4, 50)
	if len(alone) != 1 || alone[0].head.width != 160 {
		t.Errorf("with nothing beside it, USAGE's heading is %+v, want it the full width", alone[0].head)
	}
	nowhere := planOf(frameOf(160, 60), []section{sized(SectionUsage, 13, 13), sized(SectionRouting, 0, 0)}, 4, 50)
	if len(nowhere) != 1 || nowhere[0].Section != SectionUsage {
		t.Errorf("laid out %d sections, want USAGE alone, as it leaves no room beside it", len(nowhere))
	}
}

func TestTheLayoutChangesOnlyOnAKeyOrAResizeNeverOnItsOwn(t *testing.T) {
	doc := threeRouted()
	f := frameOf(160, 40)
	f.Sections = Sections{}.Toggled(SectionSessions)
	kept := f.LayoutOf(doc, now)
	sessionsOf := func(f Frame, doc status.Document) planned {
		laid := f.laidOut(doc, now, f.top())
		return laid[slices.IndexFunc(laid, func(p planned) bool { return p.Section == SectionSessions })]
	}
	if p := sessionsOf(f, doc); p.fold || p.body.rows != 1 {
		t.Fatalf("with none running, SESSIONS is laid out folded %v, its body %d rows; want a row saying so", p.fold, p.body.rows)
	}

	f.Sessions = flippingSessions()
	if p := sessionsOf(f, doc); !p.fold {
		t.Errorf("laid out afresh with six running, SESSIONS has %d rows, want it folded, as they don't fit", p.body.rows)
	}
	f.Layout = kept
	if p := sessionsOf(f, doc); p.fold || p.body.rows != 1 {
		t.Errorf("as the layout kept has it, with six running, SESSIONS is folded %v, its body %d rows; want the row it was given", p.fold, p.body.rows)
	}
	if again := f.LayoutOf(doc, now); again != kept {
		t.Errorf("the layout as kept is made again otherwise: %+v, want %+v", again, kept)
	}

	resized, regrouped, toggled := f, f, f
	resized.Height = 41
	regrouped.By = ByAccount
	toggled.Sections = toggled.Sections.Toggled(SectionComing)
	tests := map[string]struct {
		f   Frame
		doc status.Document
	}{
		"a resize":             {f: resized, doc: doc},
		"b":                    {f: regrouped, doc: doc},
		"a section's key":      {f: toggled, doc: doc},
		"an account more":      {f: f, doc: routerDoc("side", 5, append(slices.Clone(doc.Accounts), readAccount("spare"))...)},
		"the zero Layout kept": {f: func() Frame { g := f; g.Layout = Layout{}; return g }(), doc: doc},
	}
	for name, tt := range tests {
		fresh := tt.f
		fresh.Layout = Layout{}
		if got, want := tt.f.LayoutOf(tt.doc, now), fresh.LayoutOf(tt.doc, now); got != want || got == kept {
			t.Errorf("after %s, the layout is %+v, want it made afresh, %+v", name, got, want)
		}
	}
	if off := f; func() bool { off.View = Sessions; return off.LayoutOf(doc, now) != (Layout{}) }() {
		t.Errorf("off the Overview, LayoutOf gives a layout, want none")
	}
}

// A session's page over the Overview hides it: LayoutOf builds none of its
// sections to lay out, giving the zero Layout, as off the Overview.
func TestUnderASessionsPageTheOverviewIsLaidOutNotAtAll(t *testing.T) {
	doc := threeRouted()
	f := frameOf(160, 40)
	f.Sessions = flippingSessions()
	f.Layout = f.LayoutOf(doc, now)
	f.Session = &SessionShown{Page: views.SessionPage{Accounts: []views.SessionAccount{{Account: "work"}}}}
	if got := f.LayoutOf(doc, now); got != (Layout{}) {
		t.Errorf("under a session's page, LayoutOf gives %+v, want none", got)
	}
	if allocs := testing.AllocsPerRun(10, func() { f.LayoutOf(doc, now) }); allocs != 0 {
		t.Errorf("under a session's page, LayoutOf allocates %.0f times, want none, as it builds none of the Overview's sections", allocs)
	}
}

// unreadOf is doc with nothing read yet of its accounts, as the router
// lists them before its first read: no windows.
func unreadOf(doc status.Document) status.Document {
	doc.Accounts = slices.Clone(doc.Accounts)
	for i, a := range doc.Accounts {
		doc.Accounts[i] = status.Account{ID: a.ID, Label: a.Label, TokenSet: true}
	}
	return doc
}

func TestTheLayoutIsMadeAfreshAsASectionsShapeMovesNeverAsSessionsStartOrLinesCome(t *testing.T) {
	doc := threeRouted()
	f := frameOf(160, 40)
	f.Sections = Sections{}.Toggled(SectionSessions)
	f.Layout = f.LayoutOf(unreadOf(doc), now)
	fresh := f
	fresh.Layout = Layout{}
	if got, want := f.LayoutOf(doc, now), fresh.LayoutOf(doc, now); got != want || got != f.Layout {
		t.Errorf("as the accounts' windows every model shares are first read, the layout is %+v, want it as kept, %+v, each account's rows shown before any read", got, f.Layout)
	}

	fabled := doc
	fabled.Accounts = slices.Clone(doc.Accounts)
	fabled.Accounts[2].Windows = append(slices.Clone(doc.Accounts[2].Windows), fableOf(0.1, 5*quota.Day))
	if got, want := f.LayoutOf(fabled, now), fresh.LayoutOf(fabled, now); got != want || got == f.Layout {
		t.Errorf("as a model's own window is first read, the layout is %+v, want it made afresh, %+v", got, want)
	}
	if got := f.Scrolling(fabled, now); got.Most != 0 {
		t.Errorf("as a model's own window is first read, USAGE scrolls %+v, want every account shown, as there's room for them", got)
	}

	f.Layout = f.LayoutOf(doc, now)
	busy := doc
	busy.Events = []status.Event{
		{ID: 1, At: now.Add(-time.Minute).UTC(), Kind: status.EventRoom, Account: "work"},
		{ID: 2, At: now.Add(-2 * time.Minute).UTC(), Kind: status.EventRoom, Account: "side"},
		{ID: 3, At: now.Add(-3 * time.Minute).UTC(), Kind: status.EventRoom, Account: "personal"},
		{ID: 4, At: now.Add(-4 * time.Minute).UTC(), Kind: status.EventRoom, Account: "work"},
	}
	ran := f
	ran.Sessions = flippingSessions()
	tests := map[string]struct {
		f   Frame
		doc status.Document
	}{
		"sessions starting": {f: ran, doc: doc},
		"lines coming":      {f: f, doc: busy},
	}
	for name, tt := range tests {
		if got := tt.f.LayoutOf(tt.doc, now); got != f.Layout {
			t.Errorf("after %s, the layout is %+v, want it kept, %+v", name, got, f.Layout)
		}
	}
}

// A frame built builds its sections once, and each look it takes after
// shares them: a copy of it changed after it was built, here with sessions
// started, which SESSIONS lists and ROUTING counts, draws them, lays them out
// and numbers them as they were built.
func TestAFrameBuiltLooksAtTheSectionsItBuilt(t *testing.T) {
	doc := threeRouted()
	f := frameOf(160, 50)
	f.Sections = Sections{}.Toggled(SectionSessions)
	ran := f
	ran.Sessions = flippingSessions()
	built := f.Built(doc, now)
	built.Sessions = ran.Sessions

	if got, want, ran := textOf(built.Draw(doc, now)), textOf(f.Draw(doc, now)), textOf(ran.Draw(doc, now)); !slices.Equal(got, want) || slices.Equal(got, ran) {
		t.Errorf("drew\n%s\nwant it as built, with no sessions running\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got, want := built.LayoutOf(doc, now), f.LayoutOf(doc, now); got != want || got == ran.LayoutOf(doc, now) {
		t.Errorf("laid out %+v, want it as built, with no sessions running, %+v", got, want)
	}
	if got, want := built.Numbers(doc, now), f.Numbers(doc, now); !maps.Equal(got, want) || maps.Equal(got, ran.Numbers(doc, now)) {
		t.Errorf("numbered %v, want them as built, with no sessions running, %v", got, want)
	}
}

// The watch sets a frame's Layout, KeyLine and Brightened as it stands
// since the frame was built, and a card opens over it: the sections draw
// none of them, so a frame built draws as one built afresh with them.
func TestAFrameBuiltDrawsAsOneAfreshWhatItsSectionsDrawNoneOf(t *testing.T) {
	doc := threeRouted()
	f := frameOf(160, 40)
	f.Sections = Sections{}.Toggled(SectionSessions)
	kept := f.LayoutOf(doc, now)
	f.Sessions = flippingSessions()
	set := func(f Frame) Frame {
		f.Layout, f.KeyLine = kept, KeyLine{Note: "pinned to work"}
		f.Brightened = map[Number]float64{{Kind: NumberSessions, Account: "work"}: 1}
		return f
	}
	for _, card := range []Opened{nil, Help{}} {
		built, afresh := set(f.Built(doc, now)), set(f)
		built.Card, afresh.Card = card, card
		if got, want := built.Draw(doc, now), afresh.Draw(doc, now); !slices.Equal(got, want) {
			t.Errorf("under %T, drew\n%s\nwant it as a frame built afresh draws it\n%s", card, strings.Join(textOf(got), "\n"), strings.Join(textOf(want), "\n"))
		}
	}
}

func TestAFrameBuiltBuildsItsSectionsAfreshAtAnotherMoment(t *testing.T) {
	doc := comingOf(status.Upcoming{At: now.Add(time.Hour).UTC(), Account: "work", Kind: status.UpcomingReset, Window: "5h"})
	f := frameOf(160, 40)
	later := now.Add(time.Minute)
	if got, want := textOf(f.Built(doc, now).Draw(doc, later)), textOf(f.Draw(doc, later)); !slices.Equal(got, want) {
		t.Errorf("built at %s and drawn a minute on, drew\n%s\nwant it as drawn then, COMING UP's countdown a minute on\n%s", now.Format("15:04"), strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestASectionsHeadingIsDrawnAsTheFramesDrawIt(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	words := map[Section]line{
		SectionRouting:  {{"auto", strongInk}},
		SectionSessions: {{"6 running: 2 on personal, 4 on side", mutedInk}},
		SectionComing:   {{"16:30  ", secondaryInk}, {"personal", titleInk}, {"'s 5-hour resets", mutedInk}},
		SectionLately:   {{"14:41  ", secondaryInk}, {"▲ ", accentInk}, {"c61b", titleInk}, {" started on side", mutedInk}},
	}
	tests := []struct {
		stem          string
		width, height int
		by            By
		shown         Sections
		// rows are the headings' rows, counted from 1.
		rows []int
	}{
		{stem: "overview-default-160", width: 160, height: 37, rows: []int{5, 19, 31}},
		{stem: "overview-by-account-160", width: 160, height: 42, by: ByAccount, rows: []int{5, 36}},
		{stem: "overview-everything-160x39", width: 160, height: 39, shown: gs, rows: []int{5, 29, 36, 37}},
		{stem: "overview-default-216", width: 216, height: 70, rows: []int{5, 41, 53}},
		{stem: "overview-phone-52", width: 52, height: 66, rows: []int{6, 43, 54, 60}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, View: Overview, By: tt.by, Look: nord}
			sections := autoWorld.sections(tt.width, tt.by, tt.shown)
			for i := range sections {
				if said, ok := words[sections[i].Section]; ok {
					sections[i].folded = func() line { return said }
					if sections[i].Section == SectionRouting {
						sections[i].description = sections[i].folded
					}
				}
			}
			c := newCanvas(tt.width, tt.height)
			f.drawSections(c, planOf(f, sections, f.top(), f.viewRows()))
			frame, drawn := reference(t, tt.stem), screenOf(t, c.rows(nord))
			for _, row := range tt.rows {
				heldTo(t, tt.stem, frame, drawn, region{top: row - 1, bottom: row - 1, right: tt.width - 1}, shadeOf(nord.Canvas()), nil, nil)
			}
		})
	}
}

func TestWhatAHeadingSaysIsCutToKeepACellOfRuleBeforeItsKey(t *testing.T) {
	c := newCanvas(30, 1)
	long := func() line { return line{{"a very long description that won't fit", strongInk}} }
	frameOf(30, 40).sectionHeading(c, planned{Section: SectionRouting, description: long, head: bounds{width: 30, rows: 1}})
	if got, want := textOf(c.rows(Look{}))[0], "ROUTING   a very long des… ─ r"; got != want {
		t.Errorf("the heading reads %q, want %q", got, want)
	}
}

func TestAFoldedHeadingSaysWhatItHoldsThenWhyOrThatItHasMore(t *testing.T) {
	holds := func(text string) func() line { return func() line { return line{{text, mutedInk}} } }
	nothing := func() line { return nil }
	lines := func(n int) func() int { return func() int { return n } }
	tests := []struct {
		name    string
		section section
		fold    bool
		want    string
	}{
		{name: "SESSIONS, there being no room at the frame's height", section: section{Section: SectionSessions, folded: holds("6 running")}, fold: true, want: "6 running   no room at 39 rows"},
		{name: "GRAPHS, holding nothing to say", section: section{Section: SectionGraphs, folded: nothing}, fold: true, want: "no room at 39 rows"},
		{name: "ROUTING, saying nothing it holds", section: section{Section: SectionRouting}, fold: true, want: "no room at 39 rows"},
		{name: "COMING UP, its first line and more", section: section{Section: SectionComing, folded: holds("16:30  personal's 5-hour resets"), lines: lines(2)}, fold: true, want: "16:30  personal's 5-hour resets  …"},
		{name: "COMING UP, its first line alone", section: section{Section: SectionComing, folded: holds("16:30  personal's 5-hour resets"), lines: lines(1)}, fold: true, want: "16:30  personal's 5-hour resets"},
		{name: "LATELY, its first line and more", section: section{Section: SectionLately, folded: holds("14:41  ▲ c61b started on side"), lines: lines(2)}, fold: true, want: "14:41  ▲ c61b started on side  …"},
		{name: "LATELY, saying why it has none", section: section{Section: SectionLately, folded: holds("nothing lately"), lines: lines(0)}, fold: true, want: "nothing lately"},
		{name: "LATELY, with nothing to show", section: section{Section: SectionLately, folded: nothing, lines: lines(0)}, fold: true, want: ""},
		{name: "ROUTING unfolded, its description", section: section{Section: SectionRouting, description: holds("auto"), folded: holds("held")}, want: "auto"},
		{name: "USAGE, with no description", section: section{Section: SectionUsage}, want: ""},
	}
	f := frameOf(160, 39)
	for _, tt := range tests {
		if got := f.says(planned{section: tt.section, fold: tt.fold}).plain(); got != tt.want {
			t.Errorf("%s: the heading says %q after its name, want %q", tt.name, got, tt.want)
		}
	}
}

func TestAFoldedSectionsHeadingSaysOnceThatThereIsNoRoomAsTheFrameAndTheDesignWordIt(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	doc, sessions, stream := overviewWorld()
	f := Frame{Width: 160, Height: 39, Look: nord, View: Overview, Policy: claudeLike, Sessions: sessions, Stream: stream, Sections: gs}
	const stem = "overview-everything-160x39"
	heldTo(t, stem, reference(t, stem), screenOf(t, f.Draw(doc, routedAt)), region{top: 35, bottom: 35, right: 159}, shadeOf(nord.Canvas()), nil, nil)

	tests := []struct {
		height  int
		section Section
		want    string
	}{
		{height: 37, section: SectionSessions, want: "SESSIONS   6 running: 2 on personal, 4 on side   no room at 37 rows ─"},
		{height: 35, section: SectionGraphs, want: "GRAPHS   work   no room at 35 rows ─"},
		{height: 30, section: SectionRouting, want: "ROUTING   auto   no room at 30 rows ─"},
	}
	for _, tt := range tests {
		f.Height = tt.height
		rows := textOf(f.Draw(doc, routedAt))
		i := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, tt.section.heading()+" ") })
		if i < 0 || !strings.HasPrefix(rows[i], tt.want) || strings.Count(rows[i], "no room") != 1 {
			t.Errorf("at %d rows, the screen reads\n%s\nwant %s's heading folded, reading %q…", tt.height, strings.Join(rows, "\n"), tt.section.heading(), tt.want)
		}
	}
}

func TestEverySectionHiddenTheFirstRowSaysHowToBringThemBack(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		width int
		want  []string
	}{
		{width: 160, want: []string{"every section is hidden:  u usage  g graphs  r routing  s sessions  c coming up  l lately"}},
		{width: 52, want: []string{"every section is hidden:  u usage  g graphs", "r routing  s sessions  c coming up  l lately"}},
	}
	for _, tt := range tests {
		f := frameOf(tt.width, 40)
		f.Sections, f.Look = SectionsNamed([]string{}), nord
		rows := f.Draw(threeRouted(), now)
		text, top := textOf(rows), f.top()
		if got := text[top : top+len(tt.want)+1]; !slices.Equal(got, append(slices.Clone(tt.want), "")) {
			t.Errorf("at %d columns, the view's first rows read\n%s\nwant\n%s", tt.width, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
		}
		cells := screenOf(t, rows)[top]
		key, word := cells[strings.Index(tt.want[0], "u usage")], cells[strings.Index(tt.want[0], "usage")]
		faint, accent := shadeOf(nord.theme.Colour(theme.TextFaint)), shadeOf(nord.theme.Colour(theme.AccentKey))
		if cells[0].fg != faint || !key.bold || key.fg != accent || word.fg != faint || word.bold {
			t.Errorf("at %d columns, its words are %s, the key %s and its word %s; want them in text.faint, the key in bold accent.key", tt.width, cells[0], key, word)
		}
	}
}

func TestASectionsHeadingIsATargetThatHidesItWhileItsInTheViewsRows(t *testing.T) {
	f := frameOf(160, 39)
	f.Sections = every
	rows := textOf(f.Draw(threeRouted(), now))
	var hides []Section
	for _, tt := range f.Targets(threeRouted(), now) {
		if tt.On != OnSection {
			continue
		}
		hides = append(hides, tt.Section)
		if got := under(rows, tt); !strings.HasPrefix(got, tt.Section.heading()+" ") || !strings.HasSuffix(got, "─ "+tt.Section.Key()) {
			t.Errorf("%s's target is over %q, want its heading, from its name to its key", tt.Section.heading(), got)
		}
	}
	if !slices.Equal(hides, EverySection()) {
		t.Errorf("the headings that are targets are %v, want every section's, those folded among them", hides)
	}

	f.Height = 12
	for _, tt := range f.Targets(threeRouted(), now) {
		if tt.On == OnSection && tt.Y >= f.Height-underView {
			t.Errorf("at 12 rows, %s's heading is a target on row %d, past the view's rows", tt.Section.heading(), tt.Y+1)
		}
	}
}

func TestTheBodyOfASectionNotFoldedHasItsTargetsWhereItsLaidOut(t *testing.T) {
	account := func(id string) func(bounds) []Target {
		return func(b bounds) []Target { return []Target{{X: b.x, Y: b.y, Width: b.width, On: OnAccount, Account: id}} }
	}
	usage, routing := sized(SectionUsage, 13, 13), sized(SectionRouting, 6, 6)
	usage.targets, routing.targets = account("work"), account("side")
	f := frameOf(160, 40)
	laid := planOf(f, []section{usage, routing}, f.top(), 16)
	var got []Target
	for _, t := range f.targetsOf(laid) {
		if t.On == OnAccount {
			got = append(got, t)
		}
	}
	if want := []Target{{Y: 6, Width: 160, On: OnAccount, Account: "work"}}; !slices.Equal(got, want) {
		t.Errorf("the bodies' targets are %+v, want USAGE's alone, where its body's laid out, as ROUTING folds", got)
	}
}

// An account's row is a click's target, which moves the focus to it, only
// where the Overview has a focus: not by account from 150 columns while
// GRAPHS charts every account beside USAGE's blocks.
func TestAnAccountsRowIsATargetOnlyWhereTheOverviewHasAFocus(t *testing.T) {
	graphs := Sections{}.Toggled(SectionGraphs)
	tests := []struct {
		name     string
		by       By
		width    int
		sections Sections
		want     bool
	}{
		{name: "by account, GRAPHS beside USAGE", by: ByAccount, width: 160, sections: graphs},
		{name: "by account under 150 columns, GRAPHS shown", by: ByAccount, width: 149, sections: graphs, want: true},
		{name: "by account, GRAPHS hidden", by: ByAccount, width: 160, want: true},
		{name: "by window, GRAPHS shown", by: ByWindow, width: 160, sections: graphs, want: true},
	}
	for _, tt := range tests {
		f := frameOf(tt.width, 60)
		f.By, f.Sections = tt.by, tt.sections
		rows := slices.ContainsFunc(f.Targets(usageWorld(), now), func(t Target) bool { return t.On == OnAccount })
		if rows != tt.want || f.Focuses() != tt.want {
			t.Errorf("%s: an account's row is a target %t, the Overview focuses %t; want both %t", tt.name, rows, f.Focuses(), tt.want)
		}
	}
}

func TestUSAGEScrollsWithinTheBodyItsLaidOutIn(t *testing.T) {
	usage := sized(SectionUsage, 30, 30)
	usage.scrolling = func(rows, _ int) Scrolling { return Scrolling{Most: 30 - sectionHead - rows, Page: rows} }
	laid := planOf(frameOf(160, 40), []section{usage, sized(SectionRouting, 6, 6)}, 4, 20)
	if got, want := scrollingOf(laid, 0), (Scrolling{Most: 12, Page: 16}); got != want {
		t.Errorf("USAGE scrolls %+v, want %+v, as its scrolling has it in the 16 rows of body it's given, squeezed, ROUTING folded", got, want)
	}
	if got := scrollingOf(planOf(frameOf(160, 40), []section{sized(SectionRouting, 6, 6)}, 4, 20), 0); got != (Scrolling{}) {
		t.Errorf("with USAGE hidden, the Overview scrolls %+v, want it not to", got)
	}
}

func TestAFrameBuiltBuildsItsSectionsAfreshOfAnotherDocument(t *testing.T) {
	f := frameOf(160, 40)
	other := twoWithRoom()
	if got, want := textOf(f.Built(threeRouted(), now).Draw(other, now)), textOf(f.Draw(other, now)); !slices.Equal(got, want) {
		t.Errorf("built of one document and drawn of another at the same moment, drew\n%s\nwant the other as drawn afresh\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
