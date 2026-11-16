package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

func TestUnderTheViewsRowsIsTheViewAndElsewhereNothing(t *testing.T) {
	tests := []struct {
		width int
		view  View
	}{
		{width: 160, view: Runway},
		{width: 52, view: Runway},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s at %d columns", tt.view, tt.width), func(t *testing.T) {
			f := frameOf(tt.width, 40)
			f.View = tt.view
			rows := textOf(f.Draw(runwayThree(), now))

			first := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, strings.ToUpper(tt.view.Title())) }) + 1
			last := len(rows) - 3
			if first < 2 || first != f.top() || rows[len(rows)-1] == "" {
				t.Fatalf("the frame reads\n%s\nwant a heading row, the view from the row under it, %d, and the key line", strings.Join(rows, "\n"), first+1)
			}
			heldUnder(t, f, runwayThree(), rows, first, last)
		})
	}
}

func TestUnderTheOverviewIsTheViewOverUSAGEsBodyAloneAsItScrollsAndElsewhereNothing(t *testing.T) {
	tests := []struct {
		name  string
		width int
		shown Sections
	}{
		{name: "wide", width: 160},
		{name: "on a phone", width: 52},
		{name: "every section shown", width: 160, shown: SectionsNamed([]string{"usage", "routing", "sessions", "coming", "lately"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.Sections = tt.shown
			rows := textOf(f.Draw(runwayThree(), now))

			usage := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "USAGE ") })
			next := usage + 1 + slices.IndexFunc(rows[usage+1:], func(r string) bool { return strings.HasPrefix(r, "ROUTING ") })
			if usage < 0 || next <= usage {
				t.Fatalf("the frame reads\n%s\nwant USAGE's heading, and a section's under it", strings.Join(rows, "\n"))
			}
			heldUnder(t, f, runwayThree(), rows, usage+sectionHead, next-2)
		})
	}

	hidden := frameOf(160, 60)
	hidden.Sections = SectionsNamed([]string{"routing", "coming", "lately"})
	heldUnder(t, hidden, runwayThree(), textOf(hidden.Draw(runwayThree(), now)), 1, 0)
}

// edgesOf are the columns of a row width cells wide where what's under them
// can change, about a box from column left to right: the row's ends, each
// of the box's edges and the columns either side, and its middle.
func edgesOf(width, left, right int) []int {
	var columns []int
	for _, x := range []int{0, left - 1, left, (left + right) / 2, right, right + 1, width - 1} {
		if 0 <= x && x < width {
			columns = append(columns, x)
		}
	}
	slices.Sort(columns)
	return slices.Compact(columns)
}

// heldUnder holds the frame's cells under the pointer, its rows drawn of
// doc: the view's from row first to last, those that scroll, along each,
// and nothing on any other row, nor off the frame.
func heldUnder(t *testing.T, f Frame, doc status.Document, rows []string, first, last int) {
	t.Helper()
	for y := range rows {
		want := UnderNothing
		if first <= y && y <= last {
			want = UnderView
		}
		for _, x := range []int{0, f.Width / 2, f.Width - 1} {
			if got := f.Under(doc, now, x, y); got != want {
				t.Errorf("under %d,%d, reading %q, is %v; want %v", x, y, rows[y], got, want)
			}
		}
	}
	for _, cell := range [][2]int{{-1, first}, {f.Width, first}, {0, -1}, {0, len(rows)}} {
		if got := f.Under(doc, now, cell[0], cell[1]); got != UnderNothing {
			t.Errorf("under %d,%d, off the frame, is %v; want nothing", cell[0], cell[1], got)
		}
	}
}

func TestUnderAFrameTooShortForItsViewIsNothing(t *testing.T) {
	f := frameOf(160, 9)
	f.View = Runway
	for y := range f.Height {
		if got := f.Under(threeRouted(), now, 10, y); got != UnderNothing {
			t.Errorf("under 10,%d of a frame too short for its view is %v, want nothing", y, got)
		}
	}
}

func TestUnderACardIsTheCardInItsBoxAndElsewhereNothing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		width, height int
		card          Opened
		title         string
	}{
		{name: "help, wide", width: 160, height: 40, card: Help{Keys: []Key{{Key: "tab", Does: "views"}, {Key: "q", Does: "quit"}}}, title: "╭─ HELP · RUNWAY "},
		{name: "help, on a phone", width: 52, height: 40, card: Help{Keys: []Key{{Key: "tab", Does: "views"}, {Key: "q", Does: "quit"}}}, title: "╭─ HELP · RUNWAY "},
		{name: "a card with keys, wide", width: 160, height: 37, card: laidCard{body: make([]line, 13), keys: []Key{{Key: "esc", Does: "cancel"}}}, title: "╭─ ROUTING · NEW SESSIONS "},
		{name: "a card with keys, on a phone", width: 52, height: 37, card: laidCard{body: make([]line, 13), keys: []Key{{Key: "esc", Does: "cancel"}}}, title: "╭─ ROUTING · NEW SESSIONS "},
	}
	doc := runwayThree()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := frameOf(tt.width, tt.height)
			f.View, f.Card = Runway, tt.card
			f = f.At(doc, now)
			rows := textOf(f.Draw(doc, now))

			top := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, tt.title) })
			if top < 0 {
				t.Fatalf("the frame reads\n%s\nwant the card over it", strings.Join(rows, "\n"))
			}
			edge := []rune(rows[top])
			left, right := slices.Index(edge, '╭'), slices.Index(edge, '╮')
			foot := slices.IndexFunc(rows, func(r string) bool { return len([]rune(r)) > left && []rune(r)[left] == '╰' })
			if foot < 0 || []rune(rows[foot])[right] != '╯' {
				t.Fatalf("the frame reads\n%s\nwant the card's foot under its top, from column %d to %d", strings.Join(rows, "\n"), left+1, right+1)
			}
			for y := range rows {
				for _, x := range edgesOf(tt.width, left, right) {
					want := UnderNothing
					if left <= x && x <= right && top <= y && y <= foot {
						want = UnderCard
					}
					if got := f.Under(doc, now, x, y); got != want {
						t.Errorf("under %d,%d is %v, want %v: the card's drawn from %d,%d to %d,%d", x, y, got, want, left, top, right, foot)
					}
				}
			}
		})
	}
}

// Under Projects' list, its projects shown and the line saying how many
// more, where any are out of view, is the list; under SESSIONS' lines, the
// view; and under its heads, its rule, every project's row and the chosen
// project's head, nothing: of a list that runs past its rows, one that fits
// whole, and one of a project alone.
func TestUnderProjectsListIsTheListAndUnderSessionsTheView(t *testing.T) {
	page := chosenPage(chosenSessions())
	tests := []struct {
		name      string
		projects  int
		overflows bool
	}{
		{name: "past its rows", projects: 30, overflows: true},
		{name: "whole", projects: 3},
		{name: "a project alone", projects: 1},
	}
	for _, tt := range tests {
		for _, size := range [][2]int{{73, 61}, {160, 37}} {
			t.Run(fmt.Sprintf("%s at %dx%d", tt.name, size[0], size[1]), func(t *testing.T) {
				f := projectsFrame(t, size[0], size[1], projectRows(tt.projects), 0, &page)
				doc := framedDoc(nil, nil)
				rows := textOf(f.Draw(doc, routedAt))
				rowOf := func(text string) int {
					t.Helper()
					y := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, text) })
					if y < 0 {
						t.Fatalf("the frame reads\n%s\nwant %q on it", strings.Join(rows, "\n"), text)
					}
					return y
				}
				first, every := rowOf("agentic-"), rowOf("all projects")
				more := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, " more") })
				if last := every - 2; (more >= 0) != tt.overflows || more >= 0 && more != last || more < 0 && last-first+1 != tt.projects {
					t.Fatalf("the frame reads\n%s\nwant the projects, the line of how many more where any are out of view, the rule and every project's row", strings.Join(rows, "\n"))
				}
				want := map[int]Under{
					first - 1: UnderNothing, every - 1: UnderNothing, every: UnderNothing, every + 2: UnderNothing,
					rowOf("d28c "): UnderView, rowOf("3d7a "): UnderView,
				}
				for y := first; y <= every-2; y++ {
					want[y] = UnderProjectsList
				}
				for y, under := range want {
					for _, x := range []int{0, f.Width / 2, f.Width - 1} {
						if got := f.Under(doc, routedAt, x, y); got != under {
							t.Errorf("under %d,%d, reading %q, is %v; want %v", x, y, rows[y], got, under)
						}
					}
				}
			})
		}
	}
}

// Of no projects, nothing is under Projects' list, which says there's no
// session of the period.
func TestUnderProjectsListOfNoProjectsIsNothingOfIt(t *testing.T) {
	for _, size := range [][2]int{{73, 61}, {160, 37}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			f := frameOf(size[0], size[1])
			f.View, f.Projects = Projects, ProjectsShown{Page: &views.ProjectsPage{}}
			doc := framedDoc(nil, nil)
			rows := textOf(f.Draw(doc, routedAt))
			for y := range rows {
				for _, x := range []int{0, f.Width / 2, f.Width - 1} {
					if got := f.Under(doc, routedAt, x, y); got == UnderProjectsList {
						t.Errorf("under %d,%d, reading %q, is Projects' list; want no list of no projects", x, y, rows[y])
					}
				}
			}
		})
	}
}
