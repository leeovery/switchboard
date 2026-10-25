package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// graphsFrame is a frame of a terminal the size given, as text alone,
// showing Graphs with the sections given, of the three accounts threeRouted
// has configured, none with a reserve.
func graphsFrame(width, height int, shown GraphsSections) Frame {
	f := frameOf(width, height)
	f.View, f.Graphs.Sections, f.Graphs.Accounts = Graphs, shown, configured("work", "personal", "side")
	return f
}

// configured are accounts with the ids given configured, in their order, each
// labelled by its id, none with a reserve.
func configured(ids ...string) config.Accounts {
	accounts := make(config.Accounts, len(ids))
	for i, id := range ids {
		accounts[i] = config.Account{ID: id, Label: id}
	}
	return accounts
}

// headingRows are the rows of the frame's Graphs' headings, in the page's
// order, scrolled to its top, as it lays them out.
func headingRows(f Frame) []int {
	var rows []int
	for _, s := range f.graphsLaid().sections {
		rows = append(rows, f.top()+s.at)
	}
	return rows
}

// everyGraphsSection are Graphs' sections, every one shown.
func everyGraphsSection() GraphsSections {
	return GraphsSectionsNamed([]string{"week", "5-hour", "pool", "glance", "when"})
}

func TestGraphsSectionsAreKeyedAndWordedAsTheDesignHasThem(t *testing.T) {
	tests := []struct {
		section            GraphsSection
		key, word, heading string
	}{
		{section: GraphsWeek, key: "w", word: "week", heading: "WEEK"},
		{section: GraphsFiveHour, key: "f", word: "5-hour", heading: "5-HOUR"},
		{section: GraphsPool, key: "o", word: "pool", heading: "POOL"},
		{section: GraphsGlance, key: "g", word: "glance", heading: "GLANCE"},
		{section: GraphsWhen, key: "h", word: "when", heading: "WHEN YOU WORK"},
	}
	var order []GraphsSection
	for _, tt := range tests {
		order = append(order, tt.section)
		if got := (struct{ key, word, heading string }{tt.section.Key(), tt.section.Word(), tt.section.heading()}); got != (struct{ key, word, heading string }{tt.key, tt.word, tt.heading}) {
			t.Errorf("%s is keyed %q, worded %q and headed %q, want %q, %q and %q", tt.heading, got.key, got.word, got.heading, tt.key, tt.word, tt.heading)
		}
		if got, ok := GraphsSectionKeyed(tt.key); !ok || got != tt.section {
			t.Errorf("GraphsSectionKeyed(%q) = %v, %v; want %s", tt.key, got, ok, tt.heading)
		}
	}
	if !slices.Equal(EveryGraphsSection(), order) {
		t.Errorf("Graphs' sections are in the order %v, want %v", EveryGraphsSection(), order)
	}
	for _, key := range []string{"u", "v", "W", ""} {
		if _, ok := GraphsSectionKeyed(key); ok {
			t.Errorf("GraphsSectionKeyed(%q) names a section, want none", key)
		}
	}
}

// Graphs shows WEEK and 5-HOUR at first; each key shows its section where
// it's hidden and hides it where it shows; and the preferences keep them by
// name, nil for those shown at first, none kept as none, a name of no section
// passed over.
func TestGraphsSectionsShowAtFirstToggleAndAreKeptByName(t *testing.T) {
	var shown GraphsSections
	if got := shown.Names(); !slices.Equal(got, []string{"week", "5-hour"}) {
		t.Errorf("at first Graphs shows %q, want week and 5-hour", got)
	}
	shown = shown.Toggled(GraphsWeek).Toggled(GraphsWhen)
	if got := shown.Names(); !slices.Equal(got, []string{"5-hour", "when"}) {
		t.Errorf("w and h toggled, Graphs shows %q, want 5-hour and when", got)
	}
	tests := []struct {
		name  string
		names []string
		want  []string
	}{
		{name: "none kept", names: nil, want: []string{"week", "5-hour"}},
		{name: "none shown", names: []string{}, want: []string{}},
		{name: "those kept, in the page's order", names: []string{"when", "pool"}, want: []string{"pool", "when"}},
		{name: "a later build's passed over", names: []string{"glance", "a-later-section"}, want: []string{"glance"}},
	}
	for _, tt := range tests {
		if got := GraphsSectionsNamed(tt.names).Names(); !slices.Equal(got, tt.want) || got == nil {
			t.Errorf("%s: Graphs shows %#v, want %#v", tt.name, got, tt.want)
		}
	}
}

func TestGraphsTogglesAreTheKeyLinesWithAHiddenSectionsWordFaint(t *testing.T) {
	got := GraphsSections{}.Toggled(GraphsFiveHour).Toggled(GraphsGlance).Toggles()
	want := []Key{
		{Key: "w", Does: "week"},
		{Key: "f", Does: "5-hour", Hidden: true},
		{Key: "o", Does: "pool", Hidden: true},
		{Key: "g", Does: "glance"},
		{Key: "h", Does: "when", Hidden: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Graphs' toggles are %+v, want %+v", got, want)
	}
}

// Each section shown has its heading where the page lays it out, the first
// on the view's top row, in the page's order, a hidden one taking no room:
// its name, a rule and its key in the last column.
func TestGraphsDrawsEachSectionShownsHeadingWhereThePageLaysItOut(t *testing.T) {
	tests := []struct {
		name  string
		shown GraphsSections
		want  []string
	}{
		{name: "at first", want: []string{"WEEK", "5-HOUR"}},
		{name: "every one", shown: everyGraphsSection(), want: []string{"WEEK", "5-HOUR", "POOL", "GLANCE", "WHEN YOU WORK"}},
		{name: "POOL and GLANCE", shown: GraphsSectionsNamed([]string{"pool", "glance"}), want: []string{"POOL", "GLANCE"}},
	}
	for _, tt := range tests {
		f := graphsFrame(160, 120, tt.shown)
		text, rows := textOf(f.Draw(threeRouted(), now)), headingRows(f)
		if len(rows) == 0 || rows[0] != f.top() {
			t.Errorf("%s: the headings are on rows %v, want the first on the view's top row, %d", tt.name, rows, f.top())
		}
		var named []string
		for y, row := range text[f.top() : f.Height-underView] {
			if name, _, ok := strings.Cut(row, "  ─"); ok {
				named = append(named, name)
				if i := len(named) - 1; i >= len(rows) || rows[i] != f.top()+y {
					t.Errorf("%s: %s's heading is on row %d, want it where the page lays it out, of %v", tt.name, name, f.top()+y, rows)
				}
			}
		}
		if !slices.Equal(named, tt.want) {
			t.Errorf("%s: the view's headings read %q, want %q", tt.name, named, tt.want)
		}
	}
	f := graphsFrame(160, 37, GraphsSections{})
	text := textOf(f.Draw(threeRouted(), now))
	if want := "WEEK  " + strings.Repeat("─", 152) + " w"; text[f.top()] != want {
		t.Errorf("WEEK's heading reads\n%q\nwant\n%q", text[f.top()], want)
	}
}

// A heading's name is in text.subtle, its rule in border and its key in
// bold accent.key, in its last column, as the Overview's.
func TestGraphsHeadingIsInkedAsTheOverviewsAre(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	f := graphsFrame(160, 37, GraphsSections{})
	f.Look = nord
	cells := screenOf(t, f.Draw(threeRouted(), now))[f.top()]
	name, ruled, key := cells[0], cells[6], cells[159]
	subtle, border, accent := shadeOf(nord.theme.Colour(theme.TextSubtle)), shadeOf(nord.theme.Colour(theme.Border)), shadeOf(nord.theme.Colour(theme.AccentKey))
	if name.fg != subtle || ruled.glyph != '─' || ruled.fg != border || key.glyph != 'w' || key.fg != accent || !key.bold {
		t.Errorf("WEEK's heading is %s, its rule %s and its key %s; want text.subtle, a rule in border, w in bold accent.key", name, ruled, key)
	}
}

// WHEN YOU WORK's heading says the weeks it counts, in text.faint, three
// cells after its name; and while the page says it's loading, the first
// heading says loading…, in text.subtle, in place of what it would say.
func TestGraphsHeadingsSayWhenYouWorksWeeksAndLoadingOnTheFirst(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	hours := views.HoursPage{Description: "the 4 weeks to Wed 30 Sep"}
	tests := []struct {
		name    string
		shown   GraphsSections
		loading bool
		want    []string
		ink     theme.Token
	}{
		{name: "WHEN YOU WORK's weeks", shown: GraphsSectionsNamed([]string{"week", "when"}), want: []string{"WEEK  ─", "WHEN YOU WORK   the 4 weeks to Wed 30 Sep ─"}, ink: theme.TextFaint},
		{name: "loading, WEEK first", loading: true, want: []string{"WEEK   loading… ─", "5-HOUR  ─"}, ink: theme.TextSubtle},
		{name: "loading, WHEN YOU WORK first", shown: GraphsSectionsNamed([]string{"when"}), loading: true, want: []string{"WHEN YOU WORK   loading… ─"}, ink: theme.TextSubtle},
	}
	for _, tt := range tests {
		f := graphsFrame(160, 37, tt.shown)
		f.Look, f.Loading, f.Graphs.Hours = nord, tt.loading, hours
		rows := f.Draw(threeRouted(), now)
		text, heads := textOf(rows), headingRows(f)
		for i, want := range tt.want {
			if row := text[heads[i]]; !strings.HasPrefix(row, want) {
				t.Errorf("%s: heading %d reads %q, want it to start %q", tt.name, i+1, row, want)
			}
		}
		cells := screenOf(t, rows)
		for i, want := range tt.want {
			if at := strings.Index(want, "   "); at >= 0 {
				if said := cells[heads[i]][at+3]; said.fg != shadeOf(nord.theme.Colour(tt.ink)) {
					t.Errorf("%s: what heading %d says is %s, want %s", tt.name, i+1, said, tt.ink)
				}
			}
		}
	}
}

// Every section hidden, the view's first row says how to bring them back:
// every section is hidden: in text.faint, then the toggles, as the key line
// words them, flowed onto the rows the width needs.
func TestGraphsEverySectionHiddenTheFirstRowSaysHowToBringThemBack(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		width int
		want  []string
	}{
		{width: 160, want: []string{"every section is hidden:  w week  f 5-hour  o pool  g glance  h when"}},
		{width: 52, want: []string{"every section is hidden:  w week  f 5-hour  o pool", "g glance  h when"}},
	}
	for _, tt := range tests {
		f := graphsFrame(tt.width, 40, GraphsSectionsNamed([]string{}))
		f.Look = nord
		rows := f.Draw(threeRouted(), now)
		text, top := textOf(rows), f.top()
		if got := text[top : top+len(tt.want)+1]; !slices.Equal(got, append(slices.Clone(tt.want), "")) {
			t.Errorf("at %d columns, the view's first rows read\n%s\nwant\n%s", tt.width, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
		}
		cells := screenOf(t, rows)[top]
		key, word := cells[strings.Index(tt.want[0], "w week")], cells[strings.Index(tt.want[0], "week")]
		faint, accent := shadeOf(nord.theme.Colour(theme.TextFaint)), shadeOf(nord.theme.Colour(theme.AccentKey))
		if cells[0].fg != faint || !key.bold || key.fg != accent || word.fg != faint || word.bold {
			t.Errorf("at %d columns, its words are %s, the key %s and its word %s; want them in text.faint, the key in bold accent.key", tt.width, cells[0], key, word)
		}
	}
}

// pressed are the keys the targets a click on which presses one press, in
// their order, and the rows they're on.
func pressed(f Frame) (keys []string, rows []int) {
	for _, tt := range f.Targets(threeRouted(), now) {
		if tt.On == OnPress {
			keys, rows = append(keys, tt.Key), append(rows, tt.Y)
		}
	}
	return keys, rows
}

// Each heading the page shows, its key among it, is a target a click on
// which presses its key; one past the rows over the legend is none.
func TestGraphsHeadingIsATargetThatPressesItsKeyWhileItShows(t *testing.T) {
	f := graphsFrame(160, 120, everyGraphsSection())
	rows := textOf(f.Draw(threeRouted(), now))
	for _, tt := range f.Targets(threeRouted(), now) {
		if tt.On != OnPress {
			continue
		}
		section, _ := GraphsSectionKeyed(tt.Key)
		if got := under(rows, tt); !strings.HasPrefix(got, section.heading()+" ") || !strings.HasSuffix(got, "─ "+tt.Key) {
			t.Errorf("%s's target is over %q, want its heading, from its name to its key", section.heading(), got)
		}
	}
	if keys, _ := pressed(f); !slices.Equal(keys, []string{"w", "f", "o", "g", "h"}) {
		t.Errorf("the headings that are targets press %q, want every section's key", keys)
	}

	f.Height = 12
	if keys, _ := pressed(f); !slices.Equal(keys, []string{"w"}) {
		t.Errorf("at 12 rows, the headings that are targets press %q, want w alone, the one the rows over the legend show", keys)
	}
}

// Scrolled, the headings' targets follow their headings up the view, and one
// scrolled out of view is none.
func TestGraphsHeadingsTargetsFollowTheScroll(t *testing.T) {
	f := graphsFrame(73, 61, GraphsSections{})
	f.Graphs.Accounts = configured("work", "personal", "side", "team")
	week, fiveHour := headingRows(f)[0], headingRows(f)[1]
	tests := []struct {
		scroll int
		keys   []string
		rows   []int
	}{
		{scroll: 0, keys: []string{"w", "f"}, rows: []int{week, fiveHour}},
		{scroll: 1, keys: []string{"f"}, rows: []int{fiveHour - 1}},
		{scroll: 16, keys: []string{"f"}, rows: []int{fiveHour - 16}},
	}
	for _, tt := range tests {
		f.Scroll = tt.scroll
		text := textOf(f.Draw(threeRouted(), now))
		keys, rows := pressed(f)
		if !slices.Equal(keys, tt.keys) || !slices.Equal(rows, tt.rows) {
			t.Errorf("scrolled %d, the headings' targets press %q on rows %v, want %q on rows %v", tt.scroll, keys, rows, tt.keys, tt.rows)
			continue
		}
		if last := rows[len(rows)-1]; !strings.HasPrefix(text[last], "5-HOUR ") {
			t.Errorf("scrolled %d, 5-HOUR's target is on %q, want its heading", tt.scroll, text[last])
		}
	}
}

// Short of every least, the page scrolls whole, a row at a time, its
// sections never folding: the state row over the key line says what's out
// of view, in text.subtle, and the legend stays at the foot. Where the page
// fits, it says nothing.
func TestGraphsScrollsWholeShortOfEveryLeastTheLegendHeldAtTheFoot(t *testing.T) {
	legend := "▆▆ room left   ⠂⠄⡀ where it's heading   ✕ runs out"
	tests := []struct {
		name     string
		accounts []string
		scroll   int
		first    string
		state    string
	}{
		{name: "four accounts on a phone", accounts: []string{"work", "personal", "side", "team"}, first: "WEEK ", state: "▼ 16 more below"},
		{name: "scrolled 3", accounts: []string{"work", "personal", "side", "team"}, scroll: 3, state: "▲ 3 more above · ▼ 13 more below"},
		{name: "scrolled to the foot", accounts: []string{"work", "personal", "side", "team"}, scroll: 99, state: "▲ 16 more above"},
		{name: "three, fitting", accounts: []string{"work", "personal", "side"}, first: "WEEK "},
	}
	nord := Screen(builtin(t, "nord"))
	for _, tt := range tests {
		f := graphsFrame(73, 61, GraphsSections{})
		f.Look, f.Scroll, f.Graphs.Accounts = nord, tt.scroll, configured(tt.accounts...)
		rows := f.Draw(threeRouted(), now)
		text, foot := textOf(rows), f.stateRow()
		if text[foot] != tt.state || text[foot-1] != legend {
			t.Errorf("%s: the view's last row reads %q and the state row %q; want the legend, then %q", tt.name, text[foot-1], text[foot], tt.state)
		}
		if tt.first != "" && !strings.HasPrefix(text[f.top()], tt.first) {
			t.Errorf("%s: the view's top reads %q, want %s's heading there", tt.name, text[f.top()], tt.first)
		}
		if said := screenOf(t, rows)[foot][0]; tt.state != "" && said.fg != shadeOf(nord.theme.Colour(theme.TextSubtle)) {
			t.Errorf("%s: what's out of view is said in %s, want text.subtle", tt.name, said)
		}
	}
}

// Scrolled, the page shows the rows whole, a row on for each row scrolled,
// over the legend, and no further than shows the last.
func TestGraphsScrollsARowAtATime(t *testing.T) {
	f := graphsFrame(73, 61, GraphsSections{})
	f.Graphs.Accounts = configured("work", "personal", "side", "team")
	fiveHour := headingRows(f)[1]
	for _, scroll := range []int{1, 2, 10, 16} {
		f.Scroll = scroll
		if text := textOf(f.Draw(threeRouted(), now)); !strings.HasPrefix(text[fiveHour-scroll], "5-HOUR ") {
			t.Errorf("scrolled %d, row %d reads %q, want 5-HOUR's heading, %d rows up", scroll, fiveHour-scroll+1, text[fiveHour-scroll], scroll)
		}
	}
	if s := f.Scrolling(threeRouted(), now); s.Most != 16 || !f.ScrollsRows(threeRouted(), now) {
		t.Errorf("the page scrolls %+v, a row at a time %t; want 16 rows, a row at a time", s, f.ScrollsRows(threeRouted(), now))
	}
}
