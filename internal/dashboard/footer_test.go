package dashboard

import (
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// keys are the keys of a watch reading the router of three accounts, as the
// frames list them, ? and q always.
var keys = []Key{
	{Key: "tab", Does: "views"}, {Key: "w", Does: "window: auto"}, {Key: "←→", Does: "focus"}, {Key: "space", Does: "flip"},
	{Key: "g", Does: "chart"}, {Key: "1-3", Does: "pin"}, {Key: "a", Does: "auto"}, {Key: "m", Does: "move"},
	{Key: "?", Does: "keys", Always: true}, {Key: "q", Does: "quit", Always: true},
}

func TestTheFooterListsAsManyKeysAsFitWhole(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		keys   []Key
		status string
		note   string
		want   string
	}{
		{name: "every key, and how reading goes at the right", width: 160, keys: keys, status: "read 4s ago",
			want: " tab views   w window: auto   ←→ focus   space flip   g chart   1-3 pin   a auto   m move   ? keys   q quit" + blanks(41) + "read 4s ago"},
		{name: "where all don't fit, dropping from the end, but for ? and q", width: 100, keys: keys, status: "read 4s ago",
			want: " tab views   w window: auto   ←→ focus   space flip   g chart   ? keys   q quit" + blanks(9) + "read 4s ago"},
		{name: "on a phone, closer", width: 52, keys: keys, status: "read 4s ago",
			want: " tab views  ? keys  q quit" + blanks(14) + "read 4s ago"},
		{name: "without a status, to the edge", width: 41, keys: keys[5:],
			want: " 1-3 pin  a auto  m move  ? keys  q quit"},
		{name: "those always listed, as many as fit alone", width: 12, keys: keys,
			want: " ? keys"},
		{name: "what a key did, in place of the keys", width: 160, keys: keys, status: "read 0s ago", note: "new sessions go to side",
			want: " new sessions go to side" + blanks(124) + "read 0s ago"},
		{name: "a note cut short to fit", width: 40, keys: keys, status: "read 0s ago", note: "nothing's pinned to move sessions to: pin an account with 1-3",
			want: " nothing's pinned to move…  read 0s ago"},
		{name: "a status too long for the footer, cut short", width: 30, keys: keys, status: "couldn't read usage: connection refused · next 13:14",
			want: " couldn't read usage: connec…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 1, Keys: tt.keys, Status: tt.status, Note: tt.note}
			c := newCanvas(f.Width, f.Height)
			f.footer(c, status.Document{}, 0)
			if got := c.rows(Look{})[0]; got != tt.want {
				t.Errorf("the footer reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestTheFootersInks(t *testing.T) {
	f := Frame{Width: 60, Height: 1, Keys: keys[:1], Status: "read 4s ago"}
	c := newCanvas(f.Width, f.Height)
	f.footer(c, status.Document{}, 0)

	for _, tt := range []struct {
		x    int
		want ink
	}{
		{x: 1, want: keyInk},
		{x: 5, want: mutedInk},
		{x: 55, want: dimInk},
	} {
		if got := c.at(tt.x, 0).ink; got != tt.want {
			t.Errorf("column %d is in %+v, want %+v", tt.x, got, tt.want)
		}
	}
	if keyInk.token != theme.AccentKey || !keyInk.bold {
		t.Errorf("a key is in %+v, want accent.key, bold", keyInk)
	}
}

func TestTheFooterSaysWhichSessionIsPickedOutInPlaceOfHowReadingGoes(t *testing.T) {
	f := Frame{
		Width: 120, Height: 1, Status: "read 4s ago", Focus: "work", Selected: Seat{Session: idD28C, Model: opus},
		Keys: []Key{{Key: "↑↓", Does: "select"}, {Key: "esc", Does: "done"}},
	}
	c := newCanvas(f.Width, f.Height)
	f.footer(c, threeRouted(), 0)

	if got, want := c.rows(Look{})[0], " ↑↓ select   esc done"+blanks(77)+"d28c selected on work"; got != want {
		t.Errorf("the footer reads\n%q\nwant\n%q", got, want)
	}
	for _, tt := range []struct {
		x    int
		want ink
	}{
		{x: 98, want: titleInk},
		{x: 103, want: mutedInk},
		{x: 115, want: strongInk},
	} {
		if got := c.at(tt.x, 0).ink; got != tt.want {
			t.Errorf("column %d is in %+v, want %+v", tt.x, got, tt.want)
		}
	}
}

// blanks are n blank cells.
func blanks(n int) string {
	return spaces(n).text
}
