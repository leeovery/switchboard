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
		{{Key: "tab", Does: "views"}}, {{Key: "< >", Does: "window"}}, {{Key: "p", Does: "pin"}}, {{Key: "?", Does: "help"}, {Key: "q", Does: "quit"}},
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
			case OnStep:
				want = map[int]string{-1: "<", 1: ">"}[tt.Step]
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
			t.Errorf("at %d columns, the tabs' targets are %v, want every tab's", f.Width, tabs)
		}
		if want := []string{"tab", "< >", "?"}; !slices.Equal(keys, want) {
			t.Errorf("at %d columns, the key line's targets are %q, want %q: never p pin, as no click opens the routing card, nor q quit", f.Width, keys, want)
		}
	}
}

func TestAPhonesTabsAreTargetsOnTheRowsTheyreDrawnOn(t *testing.T) {
	tests := []struct {
		width int
		// rows are the rows each tab is on, in the tabs' order.
		rows []int
	}{
		{width: 60, rows: []int{1, 1, 1, 1, 1, 1, 1}},
		{width: 52, rows: []int{1, 1, 1, 2, 2, 2, 2}},
		{width: 31, rows: []int{1, 1, 1, 2, 2, 2, 2}},
		{width: 30, rows: []int{1, 1, 2, 2, 3, 3, 3}},
		{width: 21, rows: []int{1, 1, 2, 2, 3, 3, 3}},
		{width: 20, rows: []int{1, 2, 2, 3, 3, 4, 4}},
	}
	for _, tt := range tests {
		f := clickable(tt.width, 33, Overview)
		rows := textOf(f.Draw(threeRouted(), now))
		var on []int
		for _, target := range f.Targets(threeRouted(), now) {
			if target.On != OnTab {
				continue
			}
			on = append(on, target.Y)
			if got, want := under(rows, target), " "+target.Tab.Title()+" "; got != want {
				t.Errorf("at %d columns, %s's target is over %q, want %q", tt.width, target.Tab, got, want)
			}
		}
		if !slices.Equal(on, tt.rows) {
			t.Errorf("at %d columns, the tabs are targets on rows %v, want %v", tt.width, on, tt.rows)
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
