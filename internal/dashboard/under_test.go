package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestUnderTheViewsRowsIsTheViewAndElsewhereNothing(t *testing.T) {
	tests := []struct {
		width int
		view  View
	}{
		{width: 160, view: Overview},
		{width: 160, view: Sessions},
		{width: 160, view: Runway},
		{width: 52, view: Overview},
		{width: 52, view: Sessions},
		{width: 52, view: Runway},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s at %d columns", tt.view, tt.width), func(t *testing.T) {
			f := frameOf(tt.width, 40)
			f.View = tt.view
			rows := textOf(f.Draw(runwayThree(), now))

			status := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "● healthy") })
			first := status + 2
			if heading := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, strings.ToUpper(tt.view.Title())) }); tt.view != Overview {
				first = heading + 2
			}
			last := len(rows) - 3
			if status < 0 || rows[first] == "" || rows[len(rows)-1] == "" {
				t.Fatalf("the frame reads\n%s\nwant a status line, the view drawn from row %d, and the key line", strings.Join(rows, "\n"), first+1)
			}
			for y := range rows {
				want := UnderNothing
				if first <= y && y <= last {
					want = UnderView
				}
				for _, x := range []int{0, tt.width / 2, tt.width - 1} {
					if got := f.Under(x, y); got != want {
						t.Errorf("under %d,%d, reading %q, is %v; want %v", x, y, rows[y], got, want)
					}
				}
			}
			for _, cell := range [][2]int{{-1, first}, {tt.width, first}, {0, -1}, {0, len(rows)}} {
				if got := f.Under(cell[0], cell[1]); got != UnderNothing {
					t.Errorf("under %d,%d, off the frame, is %v; want nothing", cell[0], cell[1], got)
				}
			}
		})
	}
}

func TestUnderAFrameTooShortForItsViewIsNothing(t *testing.T) {
	f := frameOf(160, 9)
	f.View = Runway
	for y := range f.Height {
		if got := f.Under(10, y); got != UnderNothing {
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
					if got := f.Under(x, y); got != want {
						t.Fatalf("under %d,%d is %v, want %v: the card's drawn from %d,%d to %d,%d", x, y, got, want, left, top, right, foot)
					}
				}
			}
		})
	}
}
