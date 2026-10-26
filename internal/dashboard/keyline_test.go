package dashboard

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// keyed is a frame of a terminal width cells wide and 10 rows tall, as text
// alone, its key line l.
func keyed(width int, l KeyLine) Frame {
	f := frameOf(width, 10)
	f.KeyLine = l
	return f
}

// lastRow is the last row of f, drawn.
func lastRow(f Frame) string {
	rows := f.Draw(threeRouted(), now)
	return rows[len(rows)-1]
}

// Keys as the design words them.
var (
	tabViews = Key{Key: "tab", Does: "views"}
	subTabs  = Key{Key: "[ ]", Does: "view"}
	pin      = Key{Key: "p", Does: "pin"}
	helpQuit = []Key{{Key: "?", Does: "help"}, {Key: "q", Does: "quit"}}
)

// groups are keys in their groups as a page might list them: tab views, the
// page's own, p pin, then ? help and q quit.
var groups = [][]Key{
	{tabViews},
	{{Key: "w", Does: "window: auto"}, {Key: "←→", Does: "focus"}},
	{pin},
	helpQuit,
}

func TestTheKeyLineSetsItsKeysApartFromColumnZero(t *testing.T) {
	tests := []struct {
		name        string
		groups      [][]Key
		lead, after int
		want        string
	}{
		{name: "most pages, three cells after tab views", groups: groups, lead: 3, want: "tab views   w window: auto  ←→ focus    p pin    ? help  q quit"},
		{name: "the Overview, five", groups: groups, lead: 5, want: "tab views     w window: auto  ←→ focus    p pin    ? help  q quit"},
		{name: "a group with no keys, no room for it", groups: [][]Key{{tabViews}, nil, {pin}, nil, helpQuit}, lead: 3, want: "tab views   p pin    ? help  q quit"},
		{name: "the Overview's, five after every group", groups: groups, lead: 5, after: 5, want: "tab views     w window: auto  ←→ focus     p pin     ? help  q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastRow(keyed(160, KeyLine{Groups: tt.groups, Lead: tt.lead, After: tt.after})); got != tt.want {
				t.Errorf("the last row = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheKeyLineListsTheKeysThatFitWhole(t *testing.T) {
	if got, want := lastRow(keyed(34, KeyLine{Groups: groups, Lead: 3})), "tab views   w window: auto"; got != want {
		t.Errorf("at 34 columns, the last row = %q, want %q: ←→ focus doesn't fit whole", got, want)
	}
	if got, want := lastRow(keyed(33, KeyLine{Groups: [][]Key{{tabViews}, {{Key: "w", Does: "window: auto"}}, {{Key: "x", Does: "y"}}}, Lead: 3})), "tab views   w window: auto    x y"; got != want {
		t.Errorf("at 33 columns, the last row = %q, want %q: x y ends at the last column", got, want)
	}
}

func TestAKeysWordsAreDrawnAsItStands(t *testing.T) {
	tests := []struct {
		key  Key
		want line
	}{
		{key: pin, want: line{{"p", keyInk}, {" pin", mutedInk}}},
		{key: Key{Key: "p", Does: "pin", Off: true}, want: line{{"p pin", faintInk}}},
		{key: Key{Key: "s", Does: "sessions", Hidden: true}, want: line{{"s", keyInk}, {" sessions", faintInk}}},
		{key: Key{Key: "b", Does: "by", State: "window"}, want: line{{"b", keyInk}, {" by: ", mutedInk}, {"window", secondaryInk}}},
		{key: Key{Key: "b", Does: "by", State: "window", Off: true}, want: line{{"b by: window", faintInk}}},
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
		{width: 160, want: "the router isn't answering"},
		{width: 20, want: "the router isn't an…"},
	}
	for _, tt := range tests {
		if got := lastRow(keyed(tt.width, KeyLine{Groups: groups, Lead: 3, Note: "the router isn't answering"})); got != tt.want {
			t.Errorf("at %d columns, the last row = %q, want %q", tt.width, got, tt.want)
		}
	}
}

func TestAPhonesKeyLineKeepsTheKeysItEndsWith(t *testing.T) {
	page := []Key{{Key: "↑↓", Does: "session"}, {Key: "enter", Does: "open"}, subTabs}
	l := KeyLine{Groups: [][]Key{{tabViews}, {pin}, helpQuit}, Lead: 3, Phone: page, PhoneHelp: "keys"}
	tests := []struct {
		name  string
		width int
		want  string
	}{
		{name: "tab views leads it while it fits, a phone under 100 columns", width: 99, want: "tab views   ↑↓ session   enter open   [ ] view   ? keys   q quit"},
		{name: "tab views is the first to go", width: 63, want: "↑↓ session   enter open   [ ] view   ? keys   q quit"},
		{name: "then the page's keys, from the last", width: 51, want: "↑↓ session   enter open   ? keys   q quit"},
		{name: "? keys and q quit always end it", width: 20, want: "? keys   q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastRow(keyed(tt.width, l)); got != tt.want {
				t.Errorf("at %d columns, the key line reads %q, want %q", tt.width, got, tt.want)
			}
		})
	}
	if got, want := lastRow(keyed(100, l)), "tab views   p pin    ? help  q quit"; got != want {
		t.Errorf("at 100 columns, wider than a phone, the key line reads %q, want %q", got, want)
	}
}

func TestAWideKeyLineThatDoesntFitWholeIsLaidOutAsAPhones(t *testing.T) {
	toggles := []Key{
		{Key: "u", Does: "usage"}, {Key: "r", Does: "routing"}, {Key: "s", Does: "sessions"},
		{Key: "c", Does: "coming up"}, {Key: "l", Does: "lately"}, {Key: "b", Does: "by", State: "window"},
	}
	l := KeyLine{Groups: [][]Key{{tabViews}, toggles, {{Key: "↑↓", Does: "scroll"}, pin, helpQuit[0], helpQuit[1]}}, Lead: 5, After: 5, PhoneHelp: "keys and toggles"}
	wide := "tab views     u usage  r routing  s sessions  c coming up  l lately  b by: window     ↑↓ scroll  p pin  ? help  q quit"
	tests := []struct {
		name  string
		width int
		want  string
	}{
		{name: "wide enough for it whole", width: ansi.StringWidth(wide), want: wide},
		{name: "a cell short of it, wider than a phone", width: ansi.StringWidth(wide) - 1, want: "tab views   ? keys and toggles   q quit"},
		{name: "a phone's width", width: phoneUnder - 1, want: "tab views   ? keys and toggles   q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastRow(keyed(tt.width, l)); got != tt.want {
				t.Errorf("at %d columns, the key line reads %q, want %q", tt.width, got, tt.want)
			}
		})
	}
}

func TestAPhonesKeyLineWithoutItsHelpIsLaidOutAsAWiderOnes(t *testing.T) {
	picker := KeyLine{Groups: [][]Key{{{Key: "↑↓", Does: "preview"}, {Key: "enter", Does: "set"}, {Key: "esc", Does: "put back"}}}, Lead: 3}
	if got, want := lastRow(keyed(52, picker)), "↑↓ preview  enter set  esc put back"; got != want {
		t.Errorf("on a phone, the theme picker's key line reads %q, want %q", got, want)
	}
}

func TestEveryKeyIsWhereTheKeyLineDrawsIt(t *testing.T) {
	for _, f := range []Frame{
		keyed(160, KeyLine{Groups: groups, Lead: 3}),
		keyed(52, KeyLine{Groups: groups, Lead: 3, Phone: []Key{subTabs}, PhoneHelp: "keys"}),
	} {
		row := []rune(lastRow(f))
		for _, k := range f.keysAt() {
			words := k.key.words().plain()
			if got := string(row[k.x:min(k.x+len([]rune(words)), len(row))]); got != words {
				t.Errorf("at %d columns, %q is drawn %q at column %d, where keysAt puts it", f.Width, words, got, k.x)
			}
		}
	}
}
