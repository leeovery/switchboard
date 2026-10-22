package capture

import (
	"slices"
	"strings"
	"testing"
)

// compareCutsWorlds are the fixtures of Compare whose figures run widest:
// over a month, with a plan not known, and with ten accounts.
var compareCutsWorlds = []string{"accounts-compare-month-160", "accounts-compare-plan-unknown-160", "accounts-compare-ten-160"}

// Compare drawn wide cuts no figure under its groups, at any width from 40
// columns to 220, over every period, in each of the worlds whose figures run
// widest: each value takes a form its column holds whole.
func TestCompareDrawnWideCutsNoFigure(t *testing.T) {
	for _, name := range compareCutsWorlds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := fixture(t, name)
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			for _, k := range keysOf("<", "<", "<") {
				m = deliver(m, k)
			}
			for period := range 4 {
				if period > 0 {
					m = deliver(m, keysOf(">")[0])
				}
				for width := 40; width <= 220; width++ {
					rows := plainRows(m, width, tallest)
					if !slices.ContainsFunc(rows, func(row string) bool { return strings.Contains(row, "0 all accounts") }) {
						continue
					}
					groups := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "ACTIVITY") })
					if groups < 0 {
						t.Fatalf("period %d at %d columns: no ACTIVITY in\n%s", period, width, strings.Join(rows, "\n"))
					}
					for _, row := range rows[groups:] {
						if strings.Contains(row, "…") {
							t.Errorf("period %d at %d columns, a figure is cut:\n%s", period, width, row)
						}
					}
				}
			}
		})
	}
}
