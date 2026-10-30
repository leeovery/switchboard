package capture

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// Setup's least, as dashboard.Setup draws it: its title row's name and tabs
// whole, 95 columns, and its chrome round its page's least rows, 25.
const (
	setupLeastWidth  = 95
	setupLeastHeight = 25
	makeItLarger     = "Make the terminal larger."
)

// Every fixture of setup's screen, at a grid of sizes either side of its
// least: at its least and larger, it fills the terminal, no row wider than
// it, its chrome, the title row, the status line, the rows round them and
// the key line on its last row, read as those of the same screen drawn as
// tall as anything fits, so no page draws on them, and its page's heading
// on its row; and where the page has a row chosen, it shows it; but where a
// card is open over the page, the card whole, placed by the dashboard
// Card's rule, and the key line empty, as the card's keys are its own, the
// rows down to the heading held but for the cells under the card; smaller
// either way, it says only to make the terminal larger.
func TestSetupsScreenIsWholeOrSaysToMakeTheTerminalLarger(t *testing.T) {
	widths := []int{30, 52, 80, 94, 95, 96, 120, 160, 230}
	heights := []int{8, 16, 24, 25, 26, 37, tallest}
	var swept int
	for _, name := range Names() {
		f := fixture(t, name)
		if f.setup == nil {
			continue
		}
		swept++
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, width := range widths {
				tall := setupRows(t, f, width, tallest)
				for _, height := range heights {
					rows := setupRows(t, f, width, height)
					if width < setupLeastWidth || height < setupLeastHeight {
						if rows[0] != makeItLarger || strings.Join(rows[1:], "") != "" {
							t.Errorf("at %d×%d, the screen reads\n%s\nwant %q alone", width, height, strings.Join(rows, "\n"), makeItLarger)
						}
						continue
					}
					var card *setupCard
					if f.setup.card != "" {
						found, wrong := cardOn(rows, height, f.setup.card, f.setup.cardKeys)
						if wrong != "" {
							t.Errorf("at %d×%d, %s:\n%s", width, height, wrong, strings.Join(rows, "\n"))
							continue
						}
						card = &found
					}
					for y := range setupHeadingRow + 1 {
						got, want := rows[y], tall[y]
						if card != nil && y >= card.top {
							got, want = card.outside(got), card.outside(want)
						}
						if got != want {
							t.Errorf("at %d×%d, row %d reads\n%q\nwant it as at %d rows\n%q", width, height, y+1, rows[y], tallest, tall[y])
						}
					}
					for y, row := range rows {
						if cells := ansi.StringWidth(row); cells > width {
							t.Errorf("at %d×%d, row %d is %d cells wide:\n%q", width, height, y+1, cells, row)
						}
					}
					if card != nil {
						continue
					}
					if key := rows[height-1]; key == "" || key != tall[tallest-1] {
						t.Errorf("at %d×%d, the last row reads\n%q\nwant the key line, as at %d rows\n%q", width, height, key, tallest, tall[tallest-1])
					}
					if chosen := chosenRow(tall); chosen != "" && chosenRow(rows) != chosen {
						t.Errorf("at %d×%d, the page reads\n%s\nwant its row chosen in view, %q", width, height, strings.Join(rows, "\n"), chosen)
					}
				}
			}
		})
	}
	if swept == 0 {
		t.Fatal("no fixture draws setup's screen: the rule above holds nothing")
	}
}

// setupCard is a card drawn over setup's screen, as cardOn finds it: the
// rows of its top edge and its bottom edge, and the cells it takes across,
// from x, w wide.
type setupCard struct {
	top, bottom, x, w int
}

// cardOn finds the card titled title, its keys keys, drawn over setup's
// screen, height rows tall, as rows, and says what's wrong with it, ""
// where nothing is: its top edge where the dashboard Card's one rule places
// it, centred down the screen, and so under the status line; its last row
// but one blanks, then its keys, then the two blanks to its line, as the
// dashboard Card right-aligns them; its bottom edge over the key line; and
// the key line empty, as its keys are its own.
func cardOn(rows []string, height int, title, keys string) (setupCard, string) {
	top := slices.IndexFunc(rows, func(row string) bool { return strings.Contains(row, "╭─ "+title+" ─") })
	if top < 0 {
		return setupCard{}, "the card " + title + " isn't drawn"
	}
	bottom := top + slices.IndexFunc(rows[top:], func(row string) bool { return strings.Contains(row, "╰─") })
	left, right := strings.Index(rows[top], "╭"), strings.Index(rows[top], "╮")
	if bottom < top+2 || right < 0 {
		return setupCard{}, "the card " + title + " isn't whole"
	}
	c := setupCard{top: top, bottom: bottom, x: ansi.StringWidth(rows[top][:left])}
	c.w = ansi.StringWidth(rows[top][:right]) + 1 - c.x
	switch placed := max((height-(bottom-top+1))/2, 0); {
	case top != placed:
		return c, fmt.Sprintf("the card %s's top edge is on row %d, want it on row %d, as the dashboard Card's rule places it", title, top+1, placed+1)
	case top <= setupStatusRow || bottom >= height-1:
		return c, "the card " + title + " isn't between the status line and the key line"
	case ansi.Cut(rows[bottom-1], c.x+1, c.x+c.w-1) != strings.Repeat(" ", c.w-4-ansi.StringWidth(keys))+keys+"  ":
		return c, fmt.Sprintf("the card's last row but one reads %q, want its keys, %q, right-aligned two cells in from its line", rows[bottom-1], keys)
	case strings.TrimSpace(rows[height-1]) != "":
		return c, fmt.Sprintf("the key line reads %q, want it empty while the card's open", rows[height-1])
	}
	return c, ""
}

// outside is row without the cells the card takes across: those before it,
// and those after it, the blanks at the end trimmed.
func (c setupCard) outside(row string) string {
	return ansi.Cut(row, 0, c.x) + "│" + strings.TrimRight(ansi.Cut(row, c.x+c.w, ansi.StringWidth(row)), " ")
}

// The card check holds a card's place to the dashboard Card's rule: the add
// card's fixture, its card moved a row up at 37 rows, is caught.
func TestTheCardCheckHoldsTheCardsPlace(t *testing.T) {
	f := fixture(t, "setup-add-first-160")
	rows := setupRows(t, f, 160, 37)
	card, wrong := cardOn(rows, 37, f.setup.card, f.setup.cardKeys)
	if wrong != "" {
		t.Fatalf("as drawn, %s", wrong)
	}
	raised := slices.Concat(rows[:card.top-1], rows[card.top:card.bottom+1], rows[card.top-1:card.top], rows[card.bottom+1:])
	if _, wrong := cardOn(raised, 37, f.setup.card, f.setup.cardKeys); !strings.Contains(wrong, "as the dashboard Card's rule places it") {
		t.Errorf("a row up, the card check says %q, want it caught as placed wrong", wrong)
	}
}

// chosenRow is the row of rows that's chosen, as ▌ leads it: "" where none
// is.
func chosenRow(rows []string) string {
	for _, row := range rows {
		if strings.HasPrefix(row, "▌") {
			return row
		}
	}
	return ""
}

// The rows of setup's screen: its status line's; its page's heading's; the
// first under it; and those after its page's last, the blank row and the key
// line.
const (
	setupStatusRow  = 2
	setupHeadingRow = 4
	setupFirstRow   = 6
	setupUnderPage  = 2
)

// The sizes the paging sweep draws setup's screen at: its least, and either
// side of where its pages reflow.
var (
	setupPagedWidths  = []int{95, 120, 160}
	setupPagedHeights = []int{25, 26, 28, 30, 37}
)

// Every fixture of setup's screen, at a grid of sizes from its least, paged
// down from where it opens, a page at a time, as far as it goes: each page
// shows the page's heading on its row; a table's heads are never drawn
// without the row under them, but kept on the page's first row; the line
// saying what's out of view names no keys where the page chooses its rows;
// and between them, the pages show every row the page draws as tall as it
// all fits. A fixture with a card open is passed over: the card takes every
// key, and covers what's under it, which the page's own fixture pages.
func TestSetupsPagesShowEveryRowTheyDraw(t *testing.T) {
	var swept int
	for _, name := range Names() {
		f := fixture(t, name)
		if f.setup == nil || f.setup.card != "" {
			continue
		}
		swept++
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, width := range setupPagedWidths {
				tall := setupRows(t, f, width, tallest)
				for _, height := range setupPagedHeights {
					seen := make(map[string]bool)
					m := f.settleSetup(watch.Size{Width: width, Height: height})
					for range mostPages {
						rows := plainOf(m.View().Content, height)
						checkSetupPage(t, width, height, rows, tall)
						for _, row := range rows[setupFirstRow : height-setupUnderPage] {
							seen[strings.TrimRight(row, " ")] = true
						}
						next := deliver(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
						if next.View().Content == m.View().Content {
							break
						}
						m = next
					}
					for _, row := range tall[setupFirstRow : tallest-setupUnderPage] {
						if row = strings.TrimRight(row, " "); row != "" && !seen[row] {
							t.Errorf("at %d×%d, paged as far as it goes, the page never shows\n%q", width, height, row)
						}
					}
				}
			}
		})
	}
	if swept == 0 {
		t.Fatal("no fixture draws setup's screen: the rule above holds nothing")
	}
}

// checkSetupPage checks a page of setup's screen, drawn width cells wide and
// height tall, as rows, against the screen drawn as tall as it all fits, as
// tall: its heading on its row; a table's heads not on its last row, nor
// over a blank one, but where they're kept on its first; and where it has a
// row chosen, what's out of view said naming no keys.
func checkSetupPage(t *testing.T, width, height int, rows, tall []string) {
	t.Helper()
	if rows[setupHeadingRow] != tall[setupHeadingRow] {
		t.Errorf("at %d×%d, the heading's row reads\n%q\nwant\n%q", width, height, rows[setupHeadingRow], tall[setupHeadingRow])
	}
	last := height - setupUnderPage - 1
	for y := setupFirstRow; y <= last; y++ {
		heads := strings.Contains(rows[y], " ACCOUNT ")
		if heads && y > setupFirstRow && (y+2 > last || strings.TrimSpace(rows[y+2]) == "" || outLine.MatchString(rows[y+2])) {
			t.Errorf("at %d×%d, row %d is the table's heads without the row under them:\n%s", width, height, y+1, strings.Join(rows, "\n"))
		}
		if out := outLine.FindString(rows[y]); out != "" && chosenRow(tall) != "" && strings.Contains(rows[y], "j/k") {
			t.Errorf("at %d×%d, row %d says what's out of view naming keys that choose here:\n%q", width, height, y+1, rows[y])
		}
	}
}

// setupRows are the rows of the fixture's frame on a terminal of the size
// given, as plain text, failing the test unless there are as many as it's
// tall.
func setupRows(t *testing.T, f Fixture, width, height int) []string {
	t.Helper()
	rows := strings.Split(strings.TrimSuffix(Plain(f.Frame(watch.Size{Width: width, Height: height})), "\n"), "\n")
	if len(rows) != height {
		t.Fatalf("at %d×%d, the frame is %d rows, want %d", width, height, len(rows), height)
	}
	return rows
}
