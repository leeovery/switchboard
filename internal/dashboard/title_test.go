package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestTheTitleRow(t *testing.T) {
	three := []View{Accounts, "sessions", "runway"}
	tests := []struct {
		name  string
		width int
		views []View
		shown View
		// want are the rows it takes, and wantNext the row it returns.
		want     []string
		wantNext int
	}{
		{name: "the views as tabs, and the hint to move between them", width: 160, views: three, shown: Accounts, want: []string{
			"  SWITCHBOARD    Accounts   sessions   runway   tab ⇥" + blanks(86) + "Mon 28 Sep  13:12:00",
		}, wantNext: 1},
		{name: "one view, with nothing to move to", width: 160, views: []View{Accounts}, shown: Accounts, want: []string{
			"  SWITCHBOARD    Accounts" + blanks(114) + "Mon 28 Sep  13:12:00",
		}, wantNext: 1},
		{name: "on a phone, the tabs on a row of their own", width: 52, views: three, shown: "sessions", want: []string{
			"  SWITCHBOARD" + blanks(33) + "13:12",
			"",
			"  Accounts  sessions  runway" + blanks(18) + "tab ⇥",
		}, wantNext: 3},
		{name: "no views, as printed once, no tabs", width: 160, want: []string{
			"  SWITCHBOARD" + blanks(126) + "Mon 28 Sep  13:12:00",
		}, wantNext: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 4, Views: tt.views, View: tt.shown}
			c := newCanvas(f.Width, f.Height)
			if got := f.title(c, now); got != tt.wantNext {
				t.Errorf("title() = %d, want %d", got, tt.wantNext)
			}
			if rows := c.rows(Look{})[:len(tt.want)]; !slices.Equal(rows, tt.want) {
				t.Errorf("the title reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestTheTitleRowFallsBackToTheTimeAlone(t *testing.T) {
	f := Frame{Width: 100, Height: 1, Views: []View{Accounts, "sessions", "runway", "a-view-with-a-long-name"}, View: Accounts}
	c := newCanvas(f.Width, f.Height)
	f.title(c, now)

	if row := c.rows(Look{})[0]; !strings.HasSuffix(row, " 13:12:00") || strings.Contains(row, "Mon 28 Sep") {
		t.Errorf("the title reads %q, want the time alone, the date not fitting beside the tabs", row)
	}
}

func TestTheShownTabIsPickedOut(t *testing.T) {
	f := Frame{Width: 160, Height: 1, Views: []View{Accounts, "sessions"}, View: "sessions"}
	c := newCanvas(f.Width, f.Height)
	f.title(c, now)

	tests := []struct {
		x    int
		want ink
	}{
		{x: 1, want: ink{token: theme.Canvas, on: theme.AccentPrimary, bold: true}},
		{x: 17, want: tabInk},
		{x: 28, want: shownTabInk},
		{x: 39, want: faintInk},
	}
	for _, tt := range tests {
		if got := c.at(tt.x, 0).ink; got != tt.want {
			t.Errorf("column %d is in %+v, want %+v", tt.x, got, tt.want)
		}
	}
	if shownTabInk.on != theme.BgSelection {
		t.Errorf("the tab shown is on %v, want bg.selection", shownTabInk.on)
	}
}
