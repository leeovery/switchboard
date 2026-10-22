package capture

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// usageFalls are where the accounts USAGE shows fall as the terminal grows a
// row taller: a fixture, grouped as it is or the other way, as b turns it,
// at sizes, as "138x19", each the height it falls at.
type usageFalls struct {
	fixture string
	by      string
	at      []string
}

// The groupings usageFalls name.
const (
	byWindow  = "by window"
	byAccount = "by account"
)

// knownFalls are where the accounts USAGE shows fall today, each S12b's to
// clear (B6): out of USAGE scrolled whole, a row at a time, whose rows show
// more accounts than it shows a row taller, scrolled an account at a time,
// or scrolled whole with its key held at the foot. The list only shrinks: a
// fall that no longer happens fails the test.
var knownFalls = slices.Concat(fallsAtBase, fallsMoved)

// fallsAtBase are the falls where they are before USAGE gives way at short
// heights, at 6a7a2091.
var fallsAtBase = []usageFalls{
	{fixture: "long-names-overview", by: byAccount, at: []string{"52x29", "73x29", "108x27", "160x24", "216x24"}},
	{fixture: "overview-108x44", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-138x15", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-73x61", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-bad-hour-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-burn-down-160x39", by: byAccount, at: []string{"52x29", "73x29", "108x27", "160x24", "216x24"}},
	{fixture: "overview-by-account-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-by-account-burn-down-220x50", by: byAccount, at: []string{"52x32", "73x32", "108x30", "138x28", "160x27", "216x27"}},
	{fixture: "overview-by-account-graphs-140", by: byAccount, at: []string{"52x29", "73x29", "108x28", "138x26", "160x24", "216x24"}},
	{fixture: "overview-by-account-graphs-160", by: byAccount, at: []string{"52x28", "73x28", "108x26", "160x23", "216x23"}},
	{fixture: "overview-default-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-default-216", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-everything-160x39", by: byAccount, at: []string{"52x29", "73x29", "108x27", "160x24", "216x24"}},
	{fixture: "overview-everything-216x106", by: byAccount, at: []string{"52x29", "73x29", "108x27", "160x24", "216x24"}},
	{fixture: "overview-model-nearing-160", by: byAccount, at: []string{"52x31", "73x31", "108x29", "138x27", "160x25", "216x25"}},
	{fixture: "overview-models-160", by: byAccount, at: []string{"52x31", "73x31", "108x29", "138x27", "160x25", "216x25"}},
	{fixture: "overview-phone-52", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-phone-burn-down-52", by: byAccount, at: []string{"52x29", "73x29", "108x28", "138x26", "160x24", "216x24"}},
	{fixture: "overview-pinned-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "overview-pool-220x50", by: byAccount, at: []string{"52x32", "73x32", "108x30", "138x28", "160x27", "216x27"}},
	{fixture: "overview-pool-burn-down-160", by: byAccount, at: []string{"52x28", "73x28", "108x26", "160x23", "216x23"}},
	{fixture: "overview-sessions-216", by: byAccount, at: []string{"52x28", "73x28", "108x26", "160x22", "216x22"}},
	{fixture: "overview-sessions-graphs-216", by: byAccount, at: []string{"52x29", "73x29", "108x27", "160x24", "216x24"}},
	{fixture: "overview-six-accounts-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "138x23", "160x21", "216x21"}},
	{fixture: "overview-six-running-173x53", by: byAccount, at: []string{"52x29", "73x29", "108x27", "138x25", "160x24", "216x24"}},
	{fixture: "overview-ten-accounts-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "138x23", "160x21", "216x21"}},
	{fixture: "overview-ten-accounts-160", by: byWindow, at: []string{"52x31", "73x31", "108x29", "138x29"}},
	{fixture: "overview-usage-alone-160", by: byAccount, at: []string{"52x23", "73x23", "108x22", "160x19", "216x19"}},
	{fixture: "page-help-scrolled", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "page-not-answering", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "page-routing", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "page-routing-160x16", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "page-routing-phone", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "routing-auto-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "routing-limit-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "routing-move-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
	{fixture: "routing-two-160", by: byAccount, at: []string{"52x27", "73x27", "108x25", "160x21", "216x21"}},
}

// fallsMoved are falls of the steps' own scrolling, where 6a7a2091 has
// none: the step that shows the most in its rows scrolls otherwise a row
// taller, an account at a time where it scrolled whole, and shows fewer, so
// the most any step shows falls. At 138 by account, four rows shorter than
// 6a7a2091's falls; and of ten accounts by window. USAGE scrolled whole at
// every height, as S12b has it, clears them.
var fallsMoved = []usageFalls{
	{fixture: "long-names-overview", by: byAccount, at: []string{"138x21"}},
	{fixture: "overview-108x44", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-138x15", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-73x61", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-bad-hour-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-burn-down-160x39", by: byAccount, at: []string{"138x21"}},
	{fixture: "overview-by-account-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-by-account-burn-down-220x50", by: byAccount, at: []string{"138x24"}},
	{fixture: "overview-by-account-graphs-140", by: byAccount, at: []string{"138x22"}},
	{fixture: "overview-by-account-graphs-160", by: byAccount, at: []string{"138x20"}},
	{fixture: "overview-default-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-default-216", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-everything-160x39", by: byAccount, at: []string{"138x21"}},
	{fixture: "overview-everything-216x106", by: byAccount, at: []string{"138x21"}},
	{fixture: "overview-model-nearing-160", by: byAccount, at: []string{"138x23"}},
	{fixture: "overview-models-160", by: byAccount, at: []string{"138x23"}},
	{fixture: "overview-phone-52", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-phone-burn-down-52", by: byAccount, at: []string{"138x22"}},
	{fixture: "overview-pinned-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-pool-220x50", by: byAccount, at: []string{"138x24"}},
	{fixture: "overview-pool-burn-down-160", by: byAccount, at: []string{"138x20"}},
	{fixture: "overview-sessions-216", by: byAccount, at: []string{"138x20"}},
	{fixture: "overview-sessions-graphs-216", by: byAccount, at: []string{"138x21"}},
	{fixture: "overview-six-accounts-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-six-running-173x53", by: byAccount, at: []string{"138x21"}},
	{fixture: "overview-ten-accounts-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "overview-ten-accounts-160", by: byWindow, at: []string{"52x25", "73x25", "108x23", "138x23"}},
	{fixture: "overview-usage-alone-160", by: byAccount, at: []string{"138x16"}},
	{fixture: "page-help-scrolled", by: byAccount, at: []string{"138x19"}},
	{fixture: "page-not-answering", by: byAccount, at: []string{"138x19"}},
	{fixture: "page-routing", by: byAccount, at: []string{"138x19"}},
	{fixture: "page-routing-160x16", by: byAccount, at: []string{"138x19"}},
	{fixture: "page-routing-phone", by: byAccount, at: []string{"138x19"}},
	{fixture: "routing-auto-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "routing-limit-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "routing-move-160", by: byAccount, at: []string{"138x19"}},
	{fixture: "routing-two-160", by: byAccount, at: []string{"138x19"}},
}

// usageWidths are the widths the accounts USAGE shows are counted at: the
// phone's, the iPad's, the landscape phone's, and the desktops'.
var usageWidths = []int{52, 73, 108, 138, 160, 216}

// Every Overview fixture, by window and by account, at each of usageWidths,
// from 10 rows to 60: the accounts USAGE shows never fall as the terminal
// grows a row taller, but where knownFalls has them, each out of USAGE
// scrolled whole.
func TestTheAccountsUSAGEShowsNeverFallAsTheTerminalGrows(t *testing.T) {
	var mu sync.Mutex
	fell := make(map[string]bool)
	t.Run("fixtures", func(t *testing.T) {
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
							n, shown := usageShown(is, names), usageShown(was, names)
							if n >= shown {
								was = is
								continue
							}
							at := sizeName(watch.Size{Width: width, Height: height})
							mu.Lock()
							fell[fallName(f.Name, by, at)] = true
							mu.Unlock()
							switch {
							case !fallKnown(f.Name, by, at):
								t.Errorf("%s at %s, USAGE shows %d accounts, fewer than the %d a row shorter", by, at, n, shown)
							case !scrolledWhole(was):
								t.Errorf("%s at %s, USAGE shows %d accounts, fewer than the %d a row shorter, not out of scrolled whole", by, at, n, shown)
							}
							was = is
						}
					}
				}
			})
		}
	})
	for _, k := range knownFalls {
		for _, at := range k.at {
			if !fell[fallName(k.fixture, k.by, at)] {
				t.Errorf("%s %s at %s no longer shows fewer accounts than a row shorter: delete it from knownFalls", k.fixture, k.by, at)
			}
		}
	}
}

// fallName names a fall of the fixture named, grouped by by, at a size.
func fallName(fixture, by, at string) string {
	return fixture + " " + by + " at " + at
}

// fallKnown reports whether knownFalls has the fixture named fall, grouped
// by by, at the size named.
func fallKnown(fixture, by, at string) bool {
	return slices.ContainsFunc(knownFalls, func(k usageFalls) bool {
		return k.fixture == fixture && k.by == by && slices.Contains(k.at, at)
	})
}

// Each of knownFalls names a fixture the sweep draws and a grouping, and
// sizes at the widths counted, each fall once.
func TestEveryKnownFallIsOfAFixtureAtAWidthCounted(t *testing.T) {
	swept := make(map[string]bool)
	for _, f := range sweptFixtures(t) {
		swept[f.Name] = true
	}
	named := make(map[string]bool)
	for _, k := range knownFalls {
		if !swept[k.fixture] || (k.by != byWindow && k.by != byAccount) {
			t.Errorf("knownFalls name %s %s, want a fixture the sweep draws, by window or by account", k.fixture, k.by)
		}
		for _, at := range k.at {
			var width, height int
			if _, err := fmt.Sscanf(at, "%dx%d", &width, &height); err != nil || !slices.Contains(usageWidths, width) || height < 11 || height > 60 {
				t.Errorf("knownFalls name %s at %s, want a width counted and a height from 11 to 60", k.fixture, at)
			}
			if name := fallName(k.fixture, k.by, at); named[name] {
				t.Errorf("knownFalls name %s twice", name)
			} else {
				named[name] = true
			}
		}
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

// scrolledWhole reports whether USAGE's rows are scrolled whole, a row at a
// time, as its rows out of view below say.
func scrolledWhole(rows []string) bool {
	return slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, " more below") })
}
