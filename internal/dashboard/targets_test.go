package dashboard

import (
	"slices"
	"strings"
	"testing"
)

// clickable is a frame of a terminal the size given, showing the view given,
// its key line listing tab views, a key of the page's, p pin, ? help and q
// quit.
func clickable(width, height int, v View) Frame {
	f := frameOf(width, height)
	f.View = v
	f.KeyLine = KeyLine{Groups: [][]Key{
		{{Key: "tab", Does: "views"}}, {{Key: "w", Does: "window: day"}}, {{Key: "p", Does: "pin"}}, {{Key: "?", Does: "help"}, {Key: "q", Does: "quit"}},
	}, Lead: 3}
	return f
}

// under is what the rows drawn have under the target: the glyphs of its
// cells.
func under(rows []string, t Target) string {
	row := []rune(rows[t.Y] + strings.Repeat(" ", t.X+t.Width))
	return string(row[t.X : t.X+t.Width])
}

func TestEachTargetSitsOverWhatItsDrawingPutThere(t *testing.T) {
	for _, f := range []Frame{clickable(160, 40, Overview), clickable(160, 40, Runway), clickable(52, 33, Sessions), clickable(99, 33, Overview)} {
		rows := textOf(f.Draw(threeRouted(), now))
		var tabs []View
		var keys []string
		for _, tt := range f.Targets(threeRouted(), now) {
			var want string
			switch tt.On {
			case OnTab:
				tabs, want = append(tabs, tt.Tab), " "+tt.Tab.Title()+" "
			case OnSubTab:
				want = " " + f.heading().subTabs[tt.SubTab] + " "
			case OnKey:
				i := slices.IndexFunc(f.keysAt(), func(k keyAt) bool { return k.key.Key == tt.Key })
				keys, want = append(keys, tt.Key), f.keysAt()[i].key.words().plain()
			case OnSection:
				if heading := under(rows, tt); strings.HasPrefix(heading, tt.Section.heading()+" ") && strings.HasSuffix(heading, "─ "+tt.Section.Key()) {
					want = heading
				}
			case OnAccount:
				named := func(y int) bool { return y >= 0 && strings.Contains(rows[y], " "+tt.Account+" ") }
				if row := under(rows, tt); named(tt.Y) || named(tt.Y-1) && strings.HasSuffix(row, "/min") {
					want = row
				}
			}
			if got := under(rows, tt); got != want || want == "" {
				t.Errorf("at %d columns on %s, target %+v is over %q, want %q", f.Width, f.View, tt, got, want)
			}
		}
		if !slices.Equal(tabs, Views()) {
			t.Errorf("at %d columns, the tabs' targets are %v, want the six tabs'", f.Width, tabs)
		}
		if want := []string{"tab", "w", "?"}; !slices.Equal(keys, want) {
			t.Errorf("at %d columns, the key line's targets are %q, want %q: never p pin, as no click opens the routing card, nor q quit", f.Width, keys, want)
		}
	}
}

func TestAPhonesTabsAreTargetsOnTheRowOfTheirOwn(t *testing.T) {
	for _, tt := range clickable(52, 33, Overview).Targets(threeRouted(), now) {
		if tt.On == OnTab && tt.Y != 1 {
			t.Errorf("on a phone, %s's tab is a target on row %d, want row 2", tt.Tab, tt.Y+1)
		}
	}
}

func TestTheKeyLinesKeysAreNoTargetsWhereTheyArentDrawn(t *testing.T) {
	noted := clickable(160, 40, Overview)
	noted.KeyLine.Note = "the router isn't answering"
	tests := map[string]Frame{
		"while a note takes the line's place":  noted,
		"where the line has no row of its own": clickable(160, 1, Overview),
	}
	for name, f := range tests {
		if targets := f.Targets(threeRouted(), now); slices.ContainsFunc(targets, func(t Target) bool { return t.On == OnKey }) {
			t.Errorf("%s, the targets are %+v, want no key", name, targets)
		}
	}
}
