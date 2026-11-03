package capture

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// drawnView is the view the fixture draws, as its name says it: setup's
// screen's step, and the card open over it; else the tab, a session's page,
// the routing card, the help or usage's printout its name leads with, past
// page- for a page drawn to judge by eye, and the tab's own view, of those
// dashboard.View's SubViews names, its name says, as a word of it, else the
// one the tab opens on. A page whose name leads with a tab's own view alone,
// as page-year's, is of that tab; one whose name says no view, as
// page-not-answering's, is a view of its own of the Overview, which the
// watch opens on.
func drawnView(f Fixture) string {
	if f.setup != nil {
		return strings.TrimSpace("setup " + strconv.Itoa(int(f.setup.step)) + " " + f.setup.card)
	}
	name := strings.TrimPrefix(f.Name, "page-")
	lead, _, _ := strings.Cut(name, "-")
	kind := lead
	if !slices.Contains(drawnKinds(), lead) {
		kind = tabOfSubView(lead)
	}
	subs := dashboard.View(kind).SubViews()
	switch {
	case kind == "":
		return string(dashboard.Overview) + " " + name
	case len(subs) == 0:
		return kind
	}
	for _, sub := range subs {
		if namesWord(name, sub) {
			return kind + " " + sub
		}
	}
	return kind + " " + subs[0]
}

// watchPages are what the watch shows of itself, as a fixture's name leads
// with them: each tab, and a session's page.
func watchPages() []dashboard.View {
	return append(dashboard.Views(), dashboard.SessionPage)
}

// drawnKinds are what a fixture's name leads with, where it draws one of
// the pages, the routing card or the help open over one, or usage's
// printout.
func drawnKinds() []string {
	kinds := []string{"routing", "help", "usage"}
	for _, v := range watchPages() {
		kinds = append(kinds, string(v))
	}
	return kinds
}

// tabOfSubView is the tab one of whose own views the word given names, as
// namesWord has it: "" where none's does.
func tabOfSubView(word string) string {
	for _, v := range dashboard.Views() {
		if slices.ContainsFunc(v.SubViews(), func(sub string) bool { return namesWord(word, sub) }) {
			return string(v)
		}
	}
	return ""
}

// namesWord reports whether the name, its words split by hyphens, has the
// view's name given as a word of it, or one less its plural s, as
// log-event-details names the Log's events.
func namesWord(name, view string) bool {
	words := "-" + name + "-"
	return strings.Contains(words, "-"+view+"-") || strings.Contains(words, "-"+strings.TrimSuffix(view, "s")+"-")
}

// ofAPage reports whether the watch draws the fixture as one of its pages,
// as drawnView reads it, not the routing card or the help: the fixtures the
// fit and scroll sweeps choose among. They close what's open over a page,
// so the page under the routing card or the help is drawn there as another
// fixture draws it.
func ofAPage(f Fixture) bool {
	kind, _, _ := strings.Cut(drawnView(f), " ")
	return f.watched() && slices.Contains(watchPages(), dashboard.View(kind))
}

// takesKeys reports whether the fixture takes keys: every one but a
// printout, which the key sweep draws at every width whatever it chooses.
func takesKeys(f Fixture) bool {
	return !f.Printout
}

// sweptFully are the fixtures, by name, a sweep draws at every size of its
// grid, or takes every key at every width, of those among says it chooses
// from: of each view, as drawnView has it, the one that lays out the most,
// as byAccounts orders them, and the one drawn narrowest, as byWidth does.
// The rest are drawn at a few sizes, as sweptSizes has them.
func sweptFully(t *testing.T, among func(Fixture) bool) map[string]bool {
	t.Helper()
	var fixtures []Fixture
	for _, name := range Names() {
		if f := fixture(t, name); among(f) {
			fixtures = append(fixtures, f)
		}
	}
	full := make(map[string]bool)
	for _, order := range []func(a, b Fixture) int{byAccounts, byWidth} {
		chosen := make(map[string]bool)
		for _, f := range slices.SortedFunc(slices.Values(fixtures), order) {
			if view := drawnView(f); !chosen[view] {
				chosen[view], full[f.Name] = true, true
			}
		}
	}
	return full
}

// byAccounts orders fixtures by their accounts, the most first; then by
// the keys pressed to reach them, the most first, as a list scrolled or a
// row chosen takes; then by name.
func byAccounts(a, b Fixture) int {
	return cmp.Or(cmp.Compare(len(b.accounts), len(a.accounts)), cmp.Compare(len(b.keys), len(a.keys)), strings.Compare(a.Name, b.Name))
}

// byWidth orders fixtures by the width they're drawn at, the narrowest
// first; then by their accounts, the most first; then by the keys pressed
// to reach them, the fewest first, so the view as it opens is drawn fully
// beside the one byAccounts finds; then by name.
func byWidth(a, b Fixture) int {
	return cmp.Or(cmp.Compare(a.Size.Width, b.Size.Width), cmp.Compare(len(b.accounts), len(a.accounts)), cmp.Compare(len(a.keys), len(b.keys)), strings.Compare(a.Name, b.Name))
}

// sweepGrid is a size sweep's grid: the widths and heights it draws a
// fixture at in full, and the short heights it holds one it doesn't to.
type sweepGrid struct {
	widths, heights, short []int
}

// swept is a width a size sweep draws a fixture at, and the heights it
// draws it at there.
type swept struct {
	width   int
	heights []int
}

// sweptSizes are the sizes a sweep of the grid given draws the fixture at,
// a width at a time: every one of the grid, where the sweep draws it fully,
// as sweptFully has it; else its own size, each of the grid's widths at its
// middle height, and its short heights at the fixture's own width.
func sweptSizes(f Fixture, fully bool, g sweepGrid) []swept {
	if fully {
		sizes := make([]swept, 0, len(g.widths))
		for _, width := range g.widths {
			sizes = append(sizes, swept{width: width, heights: g.heights})
		}
		return sizes
	}
	sorted := slices.Sorted(slices.Values(g.heights))
	sizes := make([]swept, 0, len(g.widths)+1)
	for _, width := range g.widths {
		sizes = withSize(sizes, width, sorted[len(sorted)/2])
	}
	for _, height := range append([]int{f.Size.Height}, g.short...) {
		sizes = withSize(sizes, f.Size.Width, height)
	}
	return sizes
}

// atWidth runs check, a size sweep's work on a fixture at the width given,
// in a subtest of its own, run in parallel, so no fixture's sizes run on
// one core.
func atWidth(t *testing.T, width int, check func(t *testing.T)) {
	t.Helper()
	t.Run(strconv.Itoa(width), func(t *testing.T) {
		t.Parallel()
		check(t)
	})
}

// withSize is sizes with the size given among them.
func withSize(sizes []swept, width, height int) []swept {
	i := slices.IndexFunc(sizes, func(s swept) bool { return s.width == width })
	if i < 0 {
		return append(sizes, swept{width: width, heights: []int{height}})
	}
	if !slices.Contains(sizes[i].heights, height) {
		sizes[i].heights = slices.Sorted(slices.Values(append(slices.Clone(sizes[i].heights), height)))
	}
	return sizes
}

// sweptFully picks one fixture or two of each view, as drawnView has it,
// among those a sweep chooses from, never more: of the pages for the fit
// and scroll sweeps, and of every fixture that takes keys for the key
// sweep. TestEveryPageTheWatchHasIsDrawnFully is what holds a view the
// fixtures miss.
func TestSweptFullyPicksOneFixtureOrTwoOfEachView(t *testing.T) {
	tests := []struct {
		name  string
		among func(Fixture) bool
	}{
		{name: "the fit and scroll sweeps", among: ofAPage},
		{name: "the key sweep", among: takesKeys},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			full := sweptFully(t, tt.among)
			drawn, fully := make(map[string]bool), make(map[string][]string)
			for _, name := range Names() {
				f := fixture(t, name)
				if !tt.among(f) {
					continue
				}
				view := drawnView(f)
				drawn[view] = true
				if full[name] {
					fully[view] = append(fully[view], name)
				}
			}
			for _, view := range slices.Sorted(maps.Keys(drawn)) {
				if names := fully[view]; len(names) < 1 || len(names) > 2 {
					t.Errorf("view %q is drawn fully by %d fixtures %v, want one or two", view, len(names), names)
				}
			}
		})
	}
}

// Every page the watch has, each tab's own views among them, as
// dashboard.View's SubViews names them, is drawn fully by a fixture the fit
// and scroll sweeps choose, and so by the key sweep's too. So a new tab or
// a tab's new view can't slip out of the full grid unnoticed.
func TestEveryPageTheWatchHasIsDrawnFully(t *testing.T) {
	full := sweptFully(t, ofAPage)
	fully := make(map[string]bool)
	for name := range full {
		fully[drawnView(fixture(t, name))] = true
	}
	for _, v := range watchPages() {
		views := []string{string(v)}
		if subs := v.SubViews(); len(subs) > 0 {
			views = nil
			for _, sub := range subs {
				views = append(views, string(v)+" "+sub)
			}
		}
		for _, view := range views {
			if !fully[view] {
				t.Errorf("no fixture the fit and scroll sweeps draw fully draws %q", view)
			}
		}
	}
}

// Each kind of fixture's name is read as the view it draws.
func TestAFixturesViewIsWhatItsNameSays(t *testing.T) {
	tests := []struct {
		name, want string
	}{
		{name: "overview-phone-52", want: "overview"},
		{name: "routing-auto-160", want: "routing"},
		{name: "page-routing", want: "routing"},
		{name: "usage-5h-52", want: "usage"},
		{name: "session-moved-160", want: "session"},
		{name: "history-phone-year-52", want: "history year"},
		{name: "history-weeks-ten-160", want: "history weeks"},
		{name: "history-tokens-models-160", want: "history tokens"},
		{name: "page-year-160x24", want: "history year"},
		{name: "accounts-phone-detail-52", want: "accounts detail"},
		{name: "page-detail-list-focus", want: "accounts detail"},
		{name: "sessions-108x44", want: "sessions switchboard"},
		{name: "sessions-list-phone-52", want: "sessions list"},
		{name: "sessions-list-long-names-108x44", want: "sessions list"},
		{name: "page-sessions-short", want: "sessions switchboard"},
		{name: "runway-5-hour-160", want: "runway 5-hour"},
		{name: "runway-ten-accounts-160", want: "runway week"},
		{name: "log-event-details-160", want: "log events"},
		{name: "log-not-live-160", want: "log requests"},
		{name: "page-log-find", want: "log requests"},
		{name: "graphs-sections-173x53", want: "graphs"},
		{name: "page-help-scrolled", want: "help"},
		{name: "page-log-help", want: "log requests"},
		{name: "page-not-answering", want: "overview not-answering"},
		{name: "setup-link-done-160", want: "setup " + strconv.Itoa(int(dashboard.SetupLink))},
		{name: "setup-token-paste-160", want: "setup " + strconv.Itoa(int(dashboard.SetupAccounts)) + " PASTE A TOKEN · PERSONAL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := drawnView(fixture(t, tt.name)); got != tt.want {
				t.Errorf("drawnView(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// The fit and scroll sweeps choose among the pages alone, as they close
// what's open over one: never the routing card's fixtures or the help's,
// but the Overview with the router gone among them.
func TestTheSizeSweepsChooseAmongThePagesAlone(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "overview-default-160", want: true},
		{name: "session-moved-160", want: true},
		{name: "page-year-160x24", want: true},
		{name: "page-not-answering", want: true},
		{name: "routing-auto-160"},
		{name: "routing-session-160"},
		{name: "page-routing-phone"},
		{name: "page-help-scrolled"},
		{name: "usage-52"},
		{name: "setup-review-160"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ofAPage(fixture(t, tt.name)); got != tt.want {
				t.Errorf("ofAPage(%s) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}

// A fixture a sweep doesn't draw fully is drawn at its own size, at each of
// the grid's widths at its middle height, and at the grid's short heights
// at its own width; one it does, at every size of the grid.
func TestASweepDrawsAFixtureAtItsOwnSizeEachWidthAndTheShortHeights(t *testing.T) {
	g := sweepGrid{widths: []int{52, 120}, heights: []int{12, 14, 16, 20}, short: []int{12, 14}}
	tests := []struct {
		name  string
		size  watch.Size
		fully bool
		want  []swept
	}{
		{
			name: "drawn fully", size: watch.Size{Width: 160, Height: 37}, fully: true,
			want: []swept{{width: 52, heights: g.heights}, {width: 120, heights: g.heights}},
		},
		{
			name: "at a width of its own", size: watch.Size{Width: 160, Height: 37},
			want: []swept{{width: 52, heights: []int{16}}, {width: 120, heights: []int{16}}, {width: 160, heights: []int{12, 14, 37}}},
		},
		{
			name: "at one of the grid's widths", size: watch.Size{Width: 52, Height: 33},
			want: []swept{{width: 52, heights: []int{12, 14, 16, 33}}, {width: 120, heights: []int{16}}},
		},
		{
			name: "at the middle height of one of the grid's widths", size: watch.Size{Width: 120, Height: 16},
			want: []swept{{width: 52, heights: []int{16}}, {width: 120, heights: []int{12, 14, 16}}},
		},
		{
			name: "at a short height of its own width", size: watch.Size{Width: 160, Height: 14},
			want: []swept{{width: 52, heights: []int{16}}, {width: 120, heights: []int{16}}, {width: 160, heights: []int{12, 14}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sweptSizes(Fixture{Size: tt.size}, tt.fully, g)
			if !slices.EqualFunc(got, tt.want, func(a, b swept) bool { return a.width == b.width && slices.Equal(a.heights, b.heights) }) {
				t.Errorf("sweptSizes(%v, %t, %+v) = %v, want %v", tt.size, tt.fully, g, got, tt.want)
			}
		})
	}
}
