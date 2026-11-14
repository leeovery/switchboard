package dashboard

import (
	"slices"
	"strings"
	"testing"
)

func TestTheEightTabsInTheirOrder(t *testing.T) {
	var titles []string
	for _, v := range Views() {
		titles = append(titles, v.Title())
	}
	if want := []string{"Overview", "Accounts", "Projects", "History", "Sessions", "Runway", "Graphs", "Log"}; !slices.Equal(titles, want) {
		t.Errorf("the tabs are %q, want %q", titles, want)
	}
}

// Each tab's own views are named as its heading row names them, the one it
// opens on first; a tab without any has none.
func TestEachTabsSubViewsInTheirOrder(t *testing.T) {
	tests := []struct {
		view View
		want []string
	}{
		{view: Overview},
		{view: Accounts, want: []string{"compare", "detail"}},
		{view: Projects},
		{view: History, want: []string{"year", "weeks", "days", "tokens"}},
		{view: Sessions, want: []string{"switchboard", "list"}},
		{view: Runway},
		{view: Graphs},
		{view: Log, want: []string{"requests", "events"}},
		{view: SessionPage},
	}
	for _, tt := range tests {
		t.Run(string(tt.view), func(t *testing.T) {
			if got := tt.view.SubViews(); !slices.Equal(got, tt.want) {
				t.Errorf("%s.SubViews() = %q, want %q", tt.view, got, tt.want)
			}
		})
	}
}

// Graphs heads its own sections, as the Overview does: no heading row, its
// view, its first section's heading, from the heading row's row, with
// nothing over it.
func TestGraphsHeadsItself(t *testing.T) {
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
		for y := f.statusRow() + 1; y < f.top(); y++ {
			if rows[y] != "" {
				t.Errorf("at %d columns, row %d reads %q, want nothing drawn", width, y+1, rows[y])
			}
		}
		if !strings.HasPrefix(rows[f.top()], "WEEK ") {
			t.Errorf("at %d columns, the view's first row reads %q, want WEEK's heading", width, rows[f.top()])
		}
	}
}
