package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// graphsFrame is a frame of a terminal the size given, as text alone,
// showing Graphs with the sections given.
func graphsFrame(width, height int, shown GraphsSections) Frame {
	f := frameOf(width, height)
	f.View, f.Graphs.Sections = Graphs, shown
	return f
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

// Each section shown has its heading from the view's top, in the page's
// order, a blank row under it and a blank row before the next, a hidden one
// taking no room: its name, a rule and its key in the last column.
func TestGraphsDrawsEachSectionShownsHeadingStackedFromTheViewsTop(t *testing.T) {
	tests := []struct {
		name  string
		shown GraphsSections
		want  []string
	}{
		{name: "at first", want: []string{"WEEK", "", "", "5-HOUR"}},
		{name: "every one", shown: everyGraphsSection(), want: []string{"WEEK", "", "", "5-HOUR", "", "", "POOL", "", "", "GLANCE", "", "", "WHEN YOU WORK"}},
		{name: "POOL and GLANCE", shown: GraphsSectionsNamed([]string{"pool", "glance"}), want: []string{"POOL", "", "", "GLANCE"}},
	}
	for _, tt := range tests {
		f := graphsFrame(160, 37, tt.shown)
		text, top := textOf(f.Draw(threeRouted(), now)), f.top()
		var named []string
		for _, row := range text[top : top+len(tt.want)+1] {
			name, _, _ := strings.Cut(row, "  ")
			named = append(named, name)
		}
		if !slices.Equal(named, append(slices.Clone(tt.want), "")) {
			t.Errorf("%s: the view's rows read\n%s\nwant headings %q", tt.name, strings.Join(text[top:top+len(tt.want)+1], "\n"), tt.want)
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
		f.Look, f.Loading, f.Graphs.Hours = nord, tt.loading, &hours
		rows := f.Draw(threeRouted(), now)
		text, top := textOf(rows), f.top()
		for i, want := range tt.want {
			if row := text[top+i*graphsSectionRows]; !strings.HasPrefix(row, want) {
				t.Errorf("%s: heading %d reads %q, want it to start %q", tt.name, i+1, row, want)
			}
		}
		cells := screenOf(t, rows)
		for i, want := range tt.want {
			if at := strings.Index(want, "   "); at >= 0 {
				if said := cells[top+i*graphsSectionRows][at+3]; said.fg != shadeOf(nord.theme.Colour(tt.ink)) {
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

// Each heading, its key among it, is a target a click on which presses its
// key, while it's in the view's rows.
func TestGraphsHeadingIsATargetThatPressesItsKeyWhileItsInTheViewsRows(t *testing.T) {
	f := graphsFrame(160, 37, everyGraphsSection())
	rows := textOf(f.Draw(threeRouted(), now))
	var pressed []string
	for _, tt := range f.Targets(threeRouted(), now) {
		if tt.On != OnPress {
			continue
		}
		pressed = append(pressed, tt.Key)
		section, _ := GraphsSectionKeyed(tt.Key)
		if got := under(rows, tt); !strings.HasPrefix(got, section.heading()+" ") || !strings.HasSuffix(got, "─ "+tt.Key) {
			t.Errorf("%s's target is over %q, want its heading, from its name to its key", section.heading(), got)
		}
	}
	if want := []string{"w", "f", "o", "g", "h"}; !slices.Equal(pressed, want) {
		t.Errorf("the headings that are targets press %q, want %q", pressed, want)
	}

	f.Height = 12
	var headings []string
	for _, tt := range f.Targets(threeRouted(), now) {
		if tt.On != OnPress {
			continue
		}
		headings = append(headings, tt.Key)
		if tt.Y >= f.Height-underView {
			t.Errorf("at 12 rows, %s's heading is a target on row %d, past the view's rows", tt.Key, tt.Y+1)
		}
	}
	if want := []string{"w", "f"}; !slices.Equal(headings, want) {
		t.Errorf("at 12 rows, the headings that are targets press %q, want %q, those in the view's rows", headings, want)
	}
}
