package dashboard

import (
	"slices"
	"strings"
	"testing"
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
		{name: "USAGE under 150 columns: stacked, as usage is", section: at(149, ByWindow).usageSection(doc, now), least: 24, most: 24},
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

func TestThePlannerGivesEachSectionItsLeastRowsInTheirOrder(t *testing.T) {
	folds := func() line { return line{{"…", dimInk}} }
	sections := []section{
		{Section: SectionUsage, least: 13},
		{Section: SectionRouting},
		{Section: SectionComing, least: 5, folded: folds},
		{Section: SectionLately, least: 5, folded: folds},
	}
	laid := frameOf(160, 40).plan(sections, 4, 20)
	want := []planned{
		{section: sections[0], heading: 4, body: bounds{y: 6, width: 160, rows: 11}},
		{section: sections[2], heading: 18, body: bounds{y: 20, width: 160, rows: 3}},
		{section: sections[3], heading: 24, fold: true, body: bounds{y: 26, width: 160, rows: 3}},
	}
	if len(laid) != len(want) {
		t.Fatalf("plan laid out %d sections, want %d: one taking no rows of its own left out", len(laid), len(want))
	}
	for i, p := range laid {
		if p.Section != want[i].Section || p.heading != want[i].heading || p.fold != want[i].fold || p.body != want[i].body {
			t.Errorf("%s: heading on %d, folded %v, its body %+v; want on %d, folded %v, %+v",
				p.Section.heading(), p.heading, p.fold, p.body, want[i].heading, want[i].fold, want[i].body)
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
					if strings.HasPrefix(row, s.heading()) {
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
