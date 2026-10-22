package capture

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// The groupings USAGE is counted in.
const (
	byWindow  = "by window"
	byAccount = "by account"
)

// usageWidths are the widths the accounts USAGE shows are counted at: the
// phone's, the iPad's, the landscape phone's, and the desktops'.
var usageWidths = []int{52, 73, 108, 138, 160, 216}

// Every Overview fixture, by window and by account, at each of usageWidths,
// from 10 rows to 60: the accounts USAGE shows never fall as the terminal
// grows a row taller, as it scrolls whole, a row at a time, whatever step it
// takes.
func TestTheAccountsUSAGEShowsNeverFallAsTheTerminalGrows(t *testing.T) {
	for _, f := range sweptFixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			t.Parallel()
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			if viewOf(t, m, f.Size) != "Overview" {
				t.Skip("not the Overview")
			}
			names := accountNames(f)
			for _, turned := range []bool{false, true} {
				g, by := m, groupedBy(f, turned)
				if turned {
					g = deliver(m, tea.KeyPressMsg{Code: 'b', Text: "b"})
				}
				for _, width := range usageWidths {
					var was []string
					for height := 10; height <= 60; height++ {
						is := usageRows(plainRows(g, width, height)[:height])
						if n, shown := usageShown(is, names), usageShown(was, names); n < shown {
							t.Errorf("%s at %s, USAGE shows %d accounts, fewer than the %d a row shorter", by, sizeName(watch.Size{Width: width, Height: height}), n, shown)
						}
						was = is
					}
				}
			}
		})
	}
}

// groupedBy is how the fixture's Overview groups USAGE: by account where its
// keys press b an odd number of times, else by window; the other way where
// it's turned.
func groupedBy(f Fixture, turned bool) string {
	presses := 0
	for _, k := range f.keys {
		if k.Text == "b" {
			presses++
		}
	}
	if (presses%2 == 1) != turned {
		return byAccount
	}
	return byWindow
}

// sectionHeading matches the heading of a section of the Overview's under
// USAGE.
var sectionHeading = regexp.MustCompile(`^(GRAPHS|ROUTING|SESSIONS|COMING UP|LATELY)( |$)`)

// usageRows are USAGE's rows of the Overview's: from under its heading to
// the next section's.
func usageRows(rows []string) []string {
	at := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "USAGE ") })
	if at < 0 {
		return nil
	}
	rows = rows[at+1:]
	if end := slices.IndexFunc(rows, sectionHeading.MatchString); end >= 0 {
		rows = rows[:end]
	}
	return rows
}

// usageShown is how many of the accounts named USAGE's rows show: those
// leading a numbered row.
func usageShown(rows, names []string) int {
	var shown []string
	for _, row := range rows {
		m := numbered.FindStringSubmatch(row)
		if m == nil {
			continue
		}
		if i := slices.IndexFunc(names, func(name string) bool { return leadsWith(m[1], name) }); i >= 0 && !slices.Contains(shown, names[i]) {
			shown = append(shown, names[i])
		}
	}
	return len(shown)
}
