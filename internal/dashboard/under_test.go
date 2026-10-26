package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
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

			first := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, strings.ToUpper(tt.view.Title())) }) + 2
			last := len(rows) - 3
			if first < 2 || first != f.top() || rows[len(rows)-1] == "" {
				t.Fatalf("the frame reads\n%s\nwant a heading row, the view from row %d, and the key line", strings.Join(rows, "\n"), first+1)
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, tt.height)
			f.View, f.Card = Runway, tt.card
			rows := textOf(f.Draw(runwayThree(), now))

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
				for x := range tt.width {
					want := UnderNothing
					if left <= x && x <= right && top <= y && y <= foot {
						want = UnderCard
					}
					if got := f.Under(runwayThree(), now, x, y); got != want {
						t.Fatalf("under %d,%d is %v, want %v: the card's drawn from %d,%d to %d,%d", x, y, got, want, left, top, right, foot)
					}
				}
			}
		})
	}
}
