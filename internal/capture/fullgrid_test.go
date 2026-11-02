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

// subViews are the sub-views of each tab that has them, as the fixtures'
// names lead their words for them, as week leads weeks, the one the tab
// opens on first.
var subViews = map[string][]string{
	"history":  {"year", "week", "day", "token"},
	"accounts": {"compare", "detail"},
	"sessions": {"switchboard", "list"},
	"runway":   {"week", "5-hour"},
	"log":      {"request", "event"},
}

// drawnView is the view the fixture draws, as its name says it: setup's
// screen's step; else the tab, a session's page, the routing card or
// usage's printout its name leads with, past page- for a page drawn to
// judge by eye, and the sub-view of a tab that has them the name says, else
// the one the tab opens on. A page whose name leads with a sub-view alone,
// as page-year's, is of that sub-view's tab; one whose name says no view is
// a view of its own.
func drawnView(f Fixture) string {
	if f.setup != nil {
		return "setup " + strconv.Itoa(int(f.setup.step))
	}
	name := strings.TrimPrefix(f.Name, "page-")
	lead, _, _ := strings.Cut(name, "-")
	tab := lead
	if !slices.Contains(drawnKinds(), lead) {
		tab = tabOfSubView(lead)
	}
	subs := subViews[tab]
	switch {
	case tab == "":
		return name
	case len(subs) == 0:
		return tab
	}
	for _, sub := range subs {
		if strings.Contains("-"+name, "-"+sub) {
			return tab + " " + sub
		}
	}
	return tab + " " + subs[0]
}

// drawnKinds are what a fixture's name leads with, where it draws a tab, a
// session's page, the routing card or usage's printout.
func drawnKinds() []string {
	kinds := []string{string(dashboard.SessionPage), "routing", "usage"}
	for _, v := range dashboard.Views() {
		kinds = append(kinds, string(v))
	}
	return kinds
}

// tabOfSubView is the tab of the sub-view given, as subViews names it: ""
// where none has it.
func tabOfSubView(sub string) string {
	for _, tab := range slices.Sorted(maps.Keys(subViews)) {
		if slices.Contains(subViews[tab], sub) {
			return tab
		}
	}
	return ""
}

// sweptFully are the fixtures, by name, each size sweep draws at every size
// of its grid, and the key sweep takes every key at every width: of each
// view, as drawnView has it, the one that lays out the most, as byAccounts
// orders them, and the one drawn narrowest, as byWidth does. The rest are
// drawn at a few sizes, as sweptSizes has them.
func sweptFully(t *testing.T) map[string]bool {
	t.Helper()
	var fixtures []Fixture
	for _, name := range Names() {
		fixtures = append(fixtures, fixture(t, name))
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
// first, then as byAccounts does.
func byWidth(a, b Fixture) int {
	return cmp.Or(cmp.Compare(a.Size.Width, b.Size.Width), byAccounts(a, b))
}

// swept is a width a size sweep draws a fixture at, and the heights it
// draws it at there.
type swept struct {
	width   int
	heights []int
}

// sweptSizes are the sizes a sweep of the widths and heights given draws
// the fixture at, a width at a time: every one of its grid, where the
// sweep draws it fully, as sweptFully has it; else its own size, each of
// the widths at the middle height, and the least height at its own width.
func sweptSizes(f Fixture, fully bool, widths, heights []int) []swept {
	if fully {
		sizes := make([]swept, 0, len(widths))
		for _, width := range widths {
			sizes = append(sizes, swept{width: width, heights: heights})
		}
		return sizes
	}
	sorted := slices.Sorted(slices.Values(heights))
	sizes := make([]swept, 0, len(widths)+1)
	for _, width := range widths {
		sizes = withSize(sizes, width, sorted[len(sorted)/2])
	}
	return withSize(withSize(sizes, f.Size.Width, f.Size.Height), f.Size.Width, sorted[0])
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

// Every view the fixtures draw, as drawnView has it, is one each sweep draws
// at every size of its grid for one fixture or two: the fit and scroll
// sweeps of those the watch draws, and the key sweep of every one. So a new
// view can't slip out of the full grid unnoticed, nor a view's fixtures
// all slip into it.
func TestEverySweepDrawsEachViewFullyForOneFixtureOrTwo(t *testing.T) {
	full := sweptFully(t)
	tests := []struct {
		name  string
		draws func(Fixture) bool
	}{
		{name: "the fit and scroll sweeps", draws: Fixture.watched},
		{name: "the key sweep", draws: func(Fixture) bool { return true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn, fully := make(map[string]bool), make(map[string][]string)
			for _, name := range Names() {
				f := fixture(t, name)
				if !tt.draws(f) {
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
		{name: "history-weeks-ten-160", want: "history week"},
		{name: "history-tokens-models-160", want: "history token"},
		{name: "page-year-160x24", want: "history year"},
		{name: "accounts-phone-detail-52", want: "accounts detail"},
		{name: "page-detail-list-focus", want: "accounts detail"},
		{name: "sessions-108x44", want: "sessions switchboard"},
		{name: "sessions-list-phone-52", want: "sessions list"},
		{name: "page-sessions-short", want: "sessions switchboard"},
		{name: "runway-5-hour-160", want: "runway 5-hour"},
		{name: "runway-ten-accounts-160", want: "runway week"},
		{name: "log-event-details-160", want: "log event"},
		{name: "log-not-live-160", want: "log request"},
		{name: "page-log-find", want: "log request"},
		{name: "graphs-sections-173x53", want: "graphs"},
		{name: "page-help-scrolled", want: "help-scrolled"},
		{name: "setup-link-done-160", want: "setup " + strconv.Itoa(int(dashboard.SetupLink))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := drawnView(fixture(t, tt.name)); got != tt.want {
				t.Errorf("drawnView(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// A fixture a sweep doesn't draw fully is drawn at its own size, at each of
// the sweep's widths at its middle height, and at its least height at its
// own width; one it does, at every size of its grid.
func TestASweepDrawsAFixtureAtItsOwnSizeEachWidthAndItsLeastHeight(t *testing.T) {
	widths, heights := []int{52, 120}, []int{12, 14, 16, 20}
	tests := []struct {
		name  string
		size  watch.Size
		fully bool
		want  []swept
	}{
		{
			name: "drawn fully", size: watch.Size{Width: 160, Height: 37}, fully: true,
			want: []swept{{width: 52, heights: heights}, {width: 120, heights: heights}},
		},
		{
			name: "at a width of its own", size: watch.Size{Width: 160, Height: 37},
			want: []swept{{width: 52, heights: []int{16}}, {width: 120, heights: []int{16}}, {width: 160, heights: []int{12, 37}}},
		},
		{
			name: "at one of the sweep's widths", size: watch.Size{Width: 52, Height: 33},
			want: []swept{{width: 52, heights: []int{12, 16, 33}}, {width: 120, heights: []int{16}}},
		},
		{
			name: "at the middle height of one of the sweep's widths", size: watch.Size{Width: 120, Height: 16},
			want: []swept{{width: 52, heights: []int{16}}, {width: 120, heights: []int{12, 16}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sweptSizes(Fixture{Size: tt.size}, tt.fully, widths, heights)
			if !slices.EqualFunc(got, tt.want, func(a, b swept) bool { return a.width == b.width && slices.Equal(a.heights, b.heights) }) {
				t.Errorf("sweptSizes(%v, %t, %v, %v) = %v, want %v", tt.size, tt.fully, widths, heights, got, tt.want)
			}
		})
	}
}
