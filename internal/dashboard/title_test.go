package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestTheTitleRow(t *testing.T) {
	tests := []struct {
		name  string
		width int
		shown View
		// want are the rows it draws, and wantRows how many it takes.
		want     []string
		wantRows int
	}{
		{name: "the six tabs, in their order, and the hint to move between them", width: 160, shown: Overview, want: []string{
			"  SWITCHBOARD    Overview   Accounts   History   Sessions   Runway   Log   tab ⇥" + blanks(59) + "Mon 28 Sep  13:12:00",
		}, wantRows: 1},
		{name: "on a phone, the tabs on a row of their own, filling it", width: 52, shown: Sessions, want: []string{
			"  SWITCHBOARD" + blanks(33) + "13:12",
			"",
			" Overview  Accounts  History  Sessions  Runway  Log",
		}, wantRows: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 4, View: tt.shown}
			c := newCanvas(f.Width, f.Height)
			f.title(c, now)
			if got := f.titleRows(); got != tt.wantRows {
				t.Errorf("titleRows() = %d, want %d", got, tt.wantRows)
			}
			if rows := c.rows(Look{})[:len(tt.want)]; !slices.Equal(rows, tt.want) {
				t.Errorf("the title reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestTheTitleRowFallsBackToTheTimeAlone(t *testing.T) {
	f := Frame{Width: 100, Height: 1, View: Overview}
	c := newCanvas(f.Width, f.Height)
	f.title(c, now)

	if row := c.rows(Look{})[0]; !strings.HasSuffix(row, " 13:12:00") || strings.Contains(row, "Mon 28 Sep") {
		t.Errorf("the title reads %q, want the time alone, the date not fitting beside the tabs", row)
	}
}

func TestTheShownTabIsPickedOut(t *testing.T) {
	f := Frame{Width: 160, Height: 1, View: Sessions}
	c := newCanvas(f.Width, f.Height)
	f.title(c, now)

	tests := []struct {
		x    int
		want ink
	}{
		{x: 1, want: ink{token: theme.Canvas, on: hue{token: theme.AccentPrimary}, bold: true}},
		{x: 17, want: tabInk},
		{x: 50, want: shownTabInk},
		{x: 75, want: faintInk},
	}
	for _, tt := range tests {
		if got := c.at(tt.x, 0).ink; got != tt.want {
			t.Errorf("column %d is in %+v, want %+v", tt.x, got, tt.want)
		}
	}
	if shownTabInk.on != (hue{token: theme.BgSelection}) {
		t.Errorf("the tab shown is on %v, want bg.selection", shownTabInk.on)
	}
}

// blanks is n blank cells.
func blanks(n int) string {
	return strings.Repeat(" ", n)
}
