package dashboard

import (
	"strings"
	"testing"
)

func TestEachTabIsAClickTargetWhereItsDrawn(t *testing.T) {
	for _, size := range [][2]int{{160, 40}, {52, 33}} {
		f := frameOf(size[0], size[1])
		rows := f.Draw(threeRouted(), now)
		targets := f.Targets()
		if len(targets) != len(Views()) {
			t.Fatalf("at %d columns, %d targets, want the six tabs'", size[0], len(targets))
		}
		for i, v := range Views() {
			tt := targets[i]
			row := []rune(rows[tt.Y] + strings.Repeat(" ", size[0]))
			if tt.On != OnTab || tt.Tab != v || tt.X+tt.Width > len(row) || string(row[tt.X:tt.X+tt.Width]) != " "+v.Title()+" " {
				t.Errorf("at %d columns, target %d is %+v over %q, want %s's tab where it's drawn", size[0], i+1, tt, rows[tt.Y], v.Title())
			}
		}
	}
}
