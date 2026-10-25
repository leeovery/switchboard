package dashboard

import (
	"slices"
	"testing"
)

func TestTheSevenTabsInTheirOrder(t *testing.T) {
	var titles []string
	for _, v := range Views() {
		titles = append(titles, v.Title())
	}
	if want := []string{"Overview", "Accounts", "History", "Sessions", "Runway", "Graphs", "Log"}; !slices.Equal(titles, want) {
		t.Errorf("the tabs are %q, want %q", titles, want)
	}
}

// Graphs heads its own sections, as the Overview does: no heading row, its
// view from the heading row's row; and till its sections come, it draws
// nothing between the status line and the key line, and help explains
// nothing after its keys.
func TestGraphsHeadsItselfAndDrawsNothingYet(t *testing.T) {
	for _, width := range []int{160, 52} {
		f := frameOf(width, 37)
		f.View = Graphs
		rows := textOf(f.Draw(threeRouted(), now))

		if pieces := f.headingPieces(); pieces != nil {
			t.Errorf("at %d columns, Graphs' heading row is %+v, want none", width, pieces)
		}
		if f.top() != f.headingY() {
			t.Errorf("at %d columns, Graphs' view starts at row %d, want the heading row's, %d", width, f.top()+1, f.headingY()+1)
		}
		for y := f.statusRow() + 1; y < f.Height-1; y++ {
			if rows[y] != "" {
				t.Errorf("at %d columns, row %d reads %q, want nothing drawn", width, y+1, rows[y])
			}
		}
		if parts := f.helpParts(); parts != nil {
			t.Errorf("at %d columns, help explains %v, want nothing", width, parts)
		}
	}
}
