package capture

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// The sizes the fit sweep draws every fixture at: widths either side of
// those a layout changes at, and every height from where the page first has
// room for a view to where History stops scrolling whole, then a few
// taller.
var (
	sweptWidths  = []int{20, 40, 52, 80, 99, 100, 120, 160, 230}
	sweptHeights = []int{8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 28, 30, 33, 36, 37, 41}
)

// tallest is the height the fit sweep holds each smaller one to: tall
// enough that every view fits.
const tallest = 60

// Every fixture, with nothing open over its page, at a grid of widths and
// heights: its title rows, its status line and the row between, and its key
// line read as those of the same fixture drawn as tall as anything fits, as
// the height changes none of them, so no view draws on them; and where
// History's Weeks or Days say how many more accounts are out of view, as
// drawn and scrolled a row, they show one at least.
func TestNoViewDrawsOnThePagesChromeOrCountsAccountsItCantShow(t *testing.T) {
	for _, name := range Names() {
		f := fixture(t, name)
		if f.Printout {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			names := accountNames(f)
			for _, width := range sweptWidths {
				tall := plainRows(m, width, tallest)
				chrome := 3
				if dashboard.Phone(width) {
					chrome = 4
				}
				for _, height := range sweptHeights {
					rows := plainRows(m, width, height)
					for y := range chrome {
						if rows[y] != tall[y] {
							t.Errorf("at %d×%d, row %d reads\n%q\nwant it as at %d rows\n%q", width, height, y+1, rows[y], tallest, tall[y])
						}
					}
					if rows[height-1] != tall[tallest-1] {
						t.Errorf("at %d×%d, the key line reads\n%q\nwant it as at %d rows\n%q", width, height, rows[height-1], tallest, tall[tallest-1])
					}
					if !showsAnAccountItCounts(rows, names) {
						t.Errorf("at %d×%d, the view counts accounts out of view and shows none:\n%s", width, height, strings.Join(rows, "\n"))
					}
					scrolled := deliver(m, tea.WindowSizeMsg{Width: width, Height: height}, keysOf("j")[0])
					if rows := plainRows(scrolled, width, height); !showsAnAccountItCounts(rows, names) {
						t.Errorf("at %d×%d, scrolled a row, the view counts accounts out of view and shows none:\n%s", width, height, strings.Join(rows, "\n"))
					}
				}
			}
		})
	}
}

// withNothingOver is m, as drawn at size, with whatever is open over its
// page closed by esc, as a card, help or the theme picker is: the page
// faded behind one draws the title row's name plain, where it's otherwise
// bold.
func withNothingOver(t *testing.T, m watch.Model, size watch.Size) watch.Model {
	t.Helper()
	for range 3 {
		g, err := parse(deliver(m, tea.WindowSizeMsg{Width: size.Width, Height: size.Height}).View().Content)
		if err != nil {
			t.Fatal(err)
		}
		if g[0][1].glyph == 'S' && g[0][1].bold {
			return m
		}
		m = deliver(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	}
	t.Fatal("after esc three times, the page is still faded behind something open over it")
	return m
}

// accountNames are the names the fixture's accounts are drawn by: their
// labels, else their ids.
func accountNames(f Fixture) []string {
	names := make([]string, len(f.accounts))
	for i, a := range f.accounts {
		names[i] = cmp.Or(a.Label, a.ID)
	}
	return names
}

// showsAnAccountItCounts reports whether History's rows that say how many
// more accounts are out of view show an account above that, a row leading
// with one of the names given, as Weeks and Days draw them: true on
// another tab, or where no row says it.
func showsAnAccountItCounts(rows, names []string) bool {
	history := slices.ContainsFunc(rows, func(row string) bool { return strings.HasPrefix(row, "HISTORY ") })
	out := slices.IndexFunc(rows, func(row string) bool { return strings.Contains(row, "more account") })
	if !history || out < 0 {
		return true
	}
	return slices.ContainsFunc(rows[:out], func(row string) bool {
		return slices.ContainsFunc(names, func(name string) bool { return leadsWith(row, name) })
	})
}

// leadsWith reports whether row leads with the account's name given, as
// Weeks and Days draw it: whole, then the end of the row, two spaces, or its
// mark; or cut short to the cells it has, as "w…".
func leadsWith(row, name string) bool {
	if rest, ok := strings.CutPrefix(row, name); ok {
		return rest == "" || strings.HasPrefix(rest, "  ") || strings.HasPrefix(rest, " ◆") || strings.HasPrefix(rest, " ▲")
	}
	first, _, _ := strings.Cut(row, " ")
	cut, ok := strings.CutSuffix(first, "…")
	return ok && cut != "" && strings.HasPrefix(name, cut)
}

// plainRows are the rows of m drawn at the size given, as plain text, as
// many as it's tall.
func plainRows(m watch.Model, width, height int) []string {
	m = deliver(m, tea.WindowSizeMsg{Width: width, Height: height})
	rows := strings.Split(ansi.Strip(m.View().Content), "\n")
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}
