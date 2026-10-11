package dashboard

import (
	"slices"
	"testing"
)

// keyed is a frame of a terminal width cells wide and 10 rows tall, as text
// alone, its key line l.
func keyed(width int, l KeyLine) Frame {
	f := frameOf(width, 10)
	f.KeyLine = l
	return f
}

// groups are the Overview's keys, in their groups: tab views, the page's
// own, p pin, then ? help and q quit.
var groups = [][]Key{
	{{Key: "tab", Does: "views"}},
	{{Key: "w", Does: "window: auto"}, {Key: "←→", Does: "focus"}},
	{{Key: "p", Does: "pin"}},
	{{Key: "?", Does: "help"}, {Key: "q", Does: "quit"}},
}

func TestTheKeyLineSetsItsKeysApart(t *testing.T) {
	tests := []struct {
		name string
		lead int
		want string
	}{
		{name: "most pages, three cells after tab views", lead: 3, want: " tab views   w window: auto  ←→ focus    p pin    ? help  q quit"},
		{name: "the Overview, five", lead: 5, want: " tab views     w window: auto  ←→ focus    p pin    ? help  q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := keyed(160, KeyLine{Groups: groups, Lead: tt.lead}).Draw(threeRouted(), now)
			if got := rows[9]; got != tt.want {
				t.Errorf("the last row = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheKeyLineListsTheKeysThatFitWhole(t *testing.T) {
	rows := keyed(35, KeyLine{Groups: groups, Lead: 3}).Draw(threeRouted(), now)
	if got, want := rows[9], " tab views   w window: auto"; got != want {
		t.Errorf("at 35 columns, the last row = %q, want %q: ←→ focus doesn't fit whole", got, want)
	}
}

func TestAKeyThatCantActIsDrawnFaint(t *testing.T) {
	tests := []struct {
		key  Key
		want line
	}{
		{key: Key{Key: "p", Does: "pin"}, want: line{{"p", keyInk}, {" pin", mutedInk}}},
		{key: Key{Key: "p", Does: "pin", Off: true}, want: line{{"p pin", faintInk}}},
	}
	for _, tt := range tests {
		if got := tt.key.words(); !slices.Equal(got, tt.want) {
			t.Errorf("%+v is drawn %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestANoteTakesTheKeyLinesPlace(t *testing.T) {
	tests := []struct {
		width int
		want  string
	}{
		{width: 160, want: " the router isn't answering"},
		{width: 20, want: " the router isn't…"},
	}
	for _, tt := range tests {
		rows := keyed(tt.width, KeyLine{Groups: groups, Lead: 3, Note: "the router isn't answering"}).Draw(threeRouted(), now)
		if got := rows[9]; got != tt.want {
			t.Errorf("at %d columns, the last row = %q, want %q", tt.width, got, tt.want)
		}
	}
}
