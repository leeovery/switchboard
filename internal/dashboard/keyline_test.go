package dashboard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// referenceRows are the rows of milestone 7's frame named, as its file of
// the extension given has them: its text, "txt", or its ANSI, "ansi".
func referenceRows(t *testing.T, name, ext string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "vhs", "reference", name+"."+ext))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
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
		{key: Key{Key: "g", Does: "graphs", Hidden: true}, want: line{{"g", keyInk}, {" graphs", faintInk}}},
		{key: Key{Key: "b", Does: "by", State: "window"}, want: line{{"b", keyInk}, {" by: ", mutedInk}, {"window", secondaryInk}}},
		{key: Key{Key: "b", Does: "by", State: "window", Off: true}, want: line{{"b by: window", faintInk}}},
	}
	for _, tt := range tests {
		if got := tt.key.words(); !slices.Equal(got, tt.want) {
			t.Errorf("%+v is drawn %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestAKeyThatCantActIsDrawnAsTheFrameDrawsIt(t *testing.T) {
	frame := referenceRows(t, "log-not-live-160", "ansi")
	f := keyed(160, KeyLine{Groups: [][]Key{{tabViews}, {{Key: "p", Does: "pin", Off: true}}}, Lead: 3})
	f.Height, f.Look, f.View = len(frame), Screen(builtin(t, "nord")), Log
	rows := f.Draw(threeRouted(), now)
	if got, want := sgrBefore(rows[len(rows)-1], "p pin"), sgrBefore(frame[len(frame)-1], "p pin"); got == "" || got != want {
		t.Errorf("p pin, off, is drawn in %q, want %q, as log-not-live-160 draws it", got, want)
	}
}

func TestTheOverviewsTogglesAreDrawnAsTheFrameDrawsThem(t *testing.T) {
	toggles := []Key{
		{Key: "u", Does: "usage"}, {Key: "g", Does: "graphs", Hidden: true}, {Key: "r", Does: "routing"},
		{Key: "s", Does: "sessions", Hidden: true}, {Key: "c", Does: "coming up"}, {Key: "l", Does: "lately"},
		{Key: "b", Does: "by", State: "window"},
	}
	text, frame := referenceRows(t, "overview-default-160", "txt"), referenceRows(t, "overview-default-160", "ansi")
	f := keyed(160, KeyLine{Groups: [][]Key{{tabViews}, toggles, {{Key: "↑↓", Does: "focus"}}}, Lead: 5, After: 5})
	f.Height, f.View = len(text), Overview
	if got, want := lastRow(f), text[len(text)-1]; !strings.HasPrefix(want, got) {
		t.Errorf("the key line reads\n%q\nwant the frame's, as far as ↑↓ focus\n%q", got, want)
	}
	f.Look = Screen(builtin(t, "nord"))
	rows := f.Draw(threeRouted(), now)
	for _, words := range []string{"usage", "graphs", "sessions", "by:", "window"} {
		if got, want := sgrBefore(rows[len(rows)-1], words), sgrBefore(frame[len(frame)-1], words); got == "" || got != want {
			t.Errorf("%q is drawn in %q, want %q, as overview-default-160 draws it", words, got, want)
		}
	}
}

// sgrBefore is the parameters of the last SGR sequence before text in the
// row, and any spaces drawn in it before the text, without a reset leading
// them: "" where text isn't there.
func sgrBefore(row, text string) string {
	at := strings.Index(row, text)
	if at < 0 {
		return ""
	}
	from := strings.LastIndex(row[:at], "\x1b[")
	if from < 0 {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSuffix(strings.TrimRight(row[from+2:at], " "), "m"), "0;")
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

func TestTheKeyLinesAreTheFrames(t *testing.T) {
	logs := [][]Key{
		{tabViews}, {subTabs},
		{{Key: "↑↓", Does: "request"}, {Key: "enter", Does: "open"}, {Key: "f", Does: "what it shows"}, {Key: "/", Does: "find"}},
		{pin}, helpQuit,
	}
	sessions := [][]Key{{tabViews}, {subTabs}, {{Key: "↑↓", Does: "session"}, {Key: "enter", Does: "its page"}}, {pin}, helpQuit}
	overview := [][]Key{
		{tabViews},
		{{Key: "u", Does: "usage"}, {Key: "g", Does: "graphs"}, {Key: "r", Does: "routing"}, {Key: "s", Does: "sessions"}, {Key: "c", Does: "coming up"}, {Key: "l", Does: "lately"}, {Key: "b", Does: "by: window"}},
		{{Key: "↑↓", Does: "focus"}, pin, helpQuit[0], helpQuit[1]},
	}
	runway := func(step string) [][]Key { return [][]Key{{tabViews}, runwayKeys(step), {pin}, helpQuit} }
	tests := []struct {
		frame string
		view  View
		line  KeyLine
	}{
		{frame: "runway-5-hour-160", view: Runway, line: KeyLine{Groups: runway("day"), Lead: 3}},
		{frame: "runway-stepped-back-160", view: Runway, line: KeyLine{Groups: runway("day"), Lead: 3}},
		{frame: "runway-ten-accounts-160", view: Runway, line: KeyLine{Groups: runway("day"), Lead: 3}},
		{frame: "runway-week-160", view: Runway, line: KeyLine{Groups: runway("week"), Lead: 3}},
		{frame: "log-requests-160", view: Log, line: KeyLine{Groups: logs, Lead: 3}},
		{frame: "sessions-switchboard-160", view: Sessions, line: KeyLine{Groups: sessions, Lead: 3}},
		{frame: "sessions-list-160", view: Sessions, line: KeyLine{Groups: sessions, Lead: 3}},
		{frame: "log-requests-phone-52", view: Log, line: KeyLine{Groups: logs, Lead: 3, Phone: []Key{{Key: "↑↓", Does: "request"}, {Key: "enter", Does: "open"}, subTabs}, PhoneHelp: "keys"}},
		{frame: "sessions-phone-52", view: Sessions, line: KeyLine{Groups: sessions, Lead: 3, Phone: []Key{{Key: "↑↓", Does: "session"}, {Key: "enter", Does: "open"}, subTabs}, PhoneHelp: "keys"}},
		{frame: "sessions-list-phone-52", view: Sessions, line: KeyLine{Groups: sessions, Lead: 3, Phone: []Key{{Key: "↑↓", Does: "session"}, {Key: "enter", Does: "open"}, subTabs}, PhoneHelp: "keys"}},
		{frame: "accounts-phone-compare-52", view: Accounts, line: KeyLine{Groups: [][]Key{{tabViews}, {subTabs}, helpQuit}, Lead: 3, Phone: []Key{subTabs}, PhoneHelp: "keys"}},
		{frame: "overview-phone-52", view: Overview, line: KeyLine{Groups: overview, Lead: 5, PhoneHelp: "keys and toggles"}},
	}
	for _, tt := range tests {
		t.Run(tt.frame, func(t *testing.T) {
			frame := referenceRows(t, tt.frame, "txt")
			want := frame[len(frame)-1]
			f := frameOf(ansi.StringWidth(frame[0]), len(frame))
			f.View, f.KeyLine = tt.view, tt.line
			if got := lastRow(f); got != want {
				t.Errorf("the key line reads\n%q\nwant the frame's\n%q", got, want)
			}
		})
	}
}

// runwayKeys are Runway's own keys as the design words them: < > window,
// and ←→ stepping by the day or the week.
func runwayKeys(step string) []Key {
	return []Key{{Key: "< >", Does: "window"}, {Key: "←→", Does: step}}
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
		{Key: "u", Does: "usage"}, {Key: "g", Does: "graphs"}, {Key: "r", Does: "routing"}, {Key: "s", Does: "sessions"},
		{Key: "c", Does: "coming up"}, {Key: "l", Does: "lately"}, {Key: "b", Does: "by", State: "window"},
	}
	l := KeyLine{Groups: [][]Key{{tabViews}, toggles, {{Key: "↑↓", Does: "focus"}, pin, helpQuit[0], helpQuit[1]}}, Lead: 5, After: 5, PhoneHelp: "keys and toggles"}
	wide := "tab views     u usage  g graphs  r routing  s sessions  c coming up  l lately  b by: window     ↑↓ focus  p pin  ? help  q quit"
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
