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
		{name: "the badge at the first column, the six tabs two cells after it, the hint, and the clock ending at the last", width: 160, shown: Overview, want: []string{
			" SWITCHBOARD    Overview   Accounts   History   Sessions   Runway   Log   tab ⇥" + blanks(61) + "Mon 28 Sep  13:12:00",
		}, wantRows: 1},
		{name: "the clock two cells clear of the hint", width: 101, shown: Overview, want: []string{
			" SWITCHBOARD    Overview   Accounts   History   Sessions   Runway   Log   tab ⇥  Mon 28 Sep  13:12:00",
		}, wantRows: 1},
		{name: "the time alone, where a date of two digits leaves too little room for the date", width: 100, shown: Overview, want: []string{
			" SWITCHBOARD    Overview   Accounts   History   Sessions   Runway   Log   tab ⇥" + blanks(13) + "13:12:00",
		}, wantRows: 1},
		{name: "on a phone, the clock on row 0, and the tabs filling a row of their own, touching, without the hint", width: 52, shown: Sessions, want: []string{
			" SWITCHBOARD" + blanks(20) + "Mon 28 Sep  13:12:00",
			" Overview  Accounts  History  Sessions  Runway  Log",
		}, wantRows: 2},
		{name: "on a phone too narrow for the date, the time alone", width: 33, shown: Overview, want: []string{
			" SWITCHBOARD" + blanks(13) + "13:12:00",
			" Overview  Accounts  History  Ses",
		}, wantRows: 2},
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

func TestTheTitleRowsInks(t *testing.T) {
	tests := []struct {
		name  string
		width int
		x, y  int
		want  ink
	}{
		{name: "the badge", width: 160, x: 0, want: ink{token: theme.Canvas, on: hue{token: theme.AccentPrimary}, bold: true}},
		{name: "a tab not shown", width: 160, x: 15, want: tabInk},
		{name: "the tab shown", width: 160, x: 47, want: shownTabInk},
		{name: "the hint", width: 160, x: 74, want: faintInk},
		{name: "the date", width: 160, x: 140, want: mutedInk},
		{name: "the time, bold", width: 160, x: 159, want: strongInk},
		{name: "a phone's tab shown", width: 52, x: 30, y: 1, want: shownTabInk},
		{name: "a phone's time", width: 52, x: 51, want: strongInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 2, View: Sessions}
			c := newCanvas(f.Width, f.Height)
			f.title(c, now)
			if got := c.at(tt.x, tt.y).ink; got != tt.want {
				t.Errorf("column %d of row %d is in %+v, want %+v", tt.x, tt.y, got, tt.want)
			}
		})
	}
	if shownTabInk.on != (hue{token: theme.BgSelection}) || strongInk != (ink{token: theme.TextSecondary, bold: true}) {
		t.Errorf("the tab shown is on %v and the time in %+v, want bg.selection, and bold text.secondary", shownTabInk.on, strongInk)
	}
}

// blanks is n blank cells.
func blanks(n int) string {
	return strings.Repeat(" ", n)
}
