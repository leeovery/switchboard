package capture

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// compareCutsWorlds are the fixtures of Compare whose figures run widest:
// over a month, with a plan not known, and with ten accounts.
var compareCutsWorlds = []string{"accounts-compare-month-160", "accounts-compare-plan-unknown-160", "accounts-compare-ten-160"}

// Compare drawn wide cuts no figure under its groups, at any width from 40
// columns to 220, over every period, in each of the worlds whose figures run
// widest: each value takes a form its column holds whole.
func TestCompareDrawnWideCutsNoFigure(t *testing.T) {
	t.Parallel()
	for _, name := range compareCutsWorlds {
		for period := range comparePeriods {
			t.Run(fmt.Sprintf("%s, period %d", name, period), func(t *testing.T) {
				t.Parallel()
				m := compareOver(t, name, period)
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
			})
		}
	}
}

// comparePeriods is how many periods Compare's cuts are held over: from the
// earliest, three back with <, to the latest.
const comparePeriods = 4

// compareOver is Compare of the fixture named, with nothing open over it,
// moved back three periods with <, then on period of them with >.
func compareOver(t *testing.T, name string, period int) watch.Model {
	t.Helper()
	f := fixture(t, name)
	m := withNothingOver(t, f.settle(f.Size), f.Size)
	for _, k := range keysOf("<", "<", "<") {
		m = deliver(m, k)
	}
	for range period {
		m = deliver(m, keysOf(">")[0])
	}
	return m
}

// turnedCutsWorlds are the fixtures whose worlds Compare turns in: three
// accounts, over a month, with a plan not known, with ten accounts, and in
// a bad hour, and with tokens' years ending.
var turnedCutsWorlds = []string{
	"accounts-phone-compare-52", "accounts-compare-160", "accounts-compare-month-160",
	"accounts-compare-plan-unknown-160", "accounts-compare-ten-160", "accounts-compare-bad-hour-160",
	"accounts-compare-token-160",
}

// turnedShare is a largest version's share ending BY MODEL's row after its
// name cut short, as a name may be and its share never.
var turnedShare = regexp.MustCompile(`…( \d+%)$`)

// turnedTall is a height every row of turned Compare shows at, ten
// accounts' among them, so none is scrolled out of view.
const turnedTall = 120

// Compare turned cuts no figure at any width from 40 columns to where it
// turns wide, over every period, in each world it's drawn in: past its
// names' column, from WEEK USED to VALUE's last row, nothing is cut but a
// largest version's name, and never its share.
func TestCompareTurnedCutsNoFigure(t *testing.T) {
	t.Parallel()
	for _, name := range turnedCutsWorlds {
		for period := range comparePeriods {
			t.Run(fmt.Sprintf("%s, period %d", name, period), func(t *testing.T) {
				t.Parallel()
				m := compareOver(t, name, period)
				width := 40
				for ; ; width++ {
					rows := plainRows(m, width, turnedTall)
					if slices.ContainsFunc(rows, func(row string) bool { return strings.Contains(row, "0 all accounts") }) {
						break
					}
					for _, cut := range turnedCuts(t, rows) {
						t.Errorf("period %d at %d columns, a figure is cut:\n%s", period, width, cut)
					}
				}
				if width == 40 {
					t.Fatalf("period %d: Compare is wide at 40 columns, never turned", period)
				}
			})
		}
	}
}

// turnedCuts are the rows of turned Compare that cut a figure past its
// names' column, where its bars and its groups' first columns start.
func turnedCuts(t *testing.T, rows []string) []string {
	t.Helper()
	index := func(prefix string) int {
		i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, prefix) })
		if i < 0 {
			t.Fatalf("no %s in\n%s", prefix, strings.Join(rows, "\n"))
		}
		return i
	}
	week, byModel, activity, value := index("WEEK USED"), index("BY MODEL"), index("ACTIVITY"), index("VALUE")
	accounts := slices.IndexFunc(rows[activity:], func(row string) bool { return strings.TrimSpace(row) == "" }) - 1
	at := strings.Index(rows[activity], "reqs")
	var cut []string
	for y := week; y <= value+accounts; y++ {
		past := strings.TrimRight(ansi.Cut(rows[y], at, ansi.StringWidth(rows[y])), " ")
		if y > byModel && y < activity {
			past = turnedShare.ReplaceAllString(past, "$1")
		}
		if strings.Contains(past, "…") {
			cut = append(cut, rows[y])
		}
	}
	return cut
}
