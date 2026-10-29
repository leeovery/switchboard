package capture

import (
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
// card is open over the page, the card whole, and the key line empty, as
// the card's keys are its own; smaller either way, it says only to make the
// terminal larger.
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
					for y := range setupHeadingRow + 1 {
						if rows[y] != tall[y] {
							t.Errorf("at %d×%d, row %d reads\n%q\nwant it as at %d rows\n%q", width, height, y+1, rows[y], tallest, tall[y])
						}
					}
					for y, row := range rows {
						if cells := ansi.StringWidth(row); cells > width {
							t.Errorf("at %d×%d, row %d is %d cells wide:\n%q", width, height, y+1, cells, row)
						}
					}
					if f.setup.card != "" {
						checkCardWhole(t, width, height, rows, f.setup.card)
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

// checkCardWhole checks the card titled title is drawn whole over setup's
// screen, drawn width cells wide and height tall, as rows: its top edge,
// titled, under the status line; its keys, on its last row but one; and its
// bottom edge, over the key line, which is empty.
func checkCardWhole(t *testing.T, width, height int, rows []string, title string) {
	t.Helper()
	top := slices.IndexFunc(rows, func(row string) bool { return strings.Contains(row, "╭─ "+title+" ─") })
	bottom := slices.IndexFunc(rows, func(row string) bool { return strings.Contains(row, "╰─") })
	switch {
	case top <= setupStatusRow || bottom <= top+2 || bottom >= height-1:
		t.Errorf("at %d×%d, the card %s isn't whole between the status line and the key line:\n%s", width, height, title, strings.Join(rows, "\n"))
	case !strings.Contains(rows[bottom-1], " esc "):
		t.Errorf("at %d×%d, the card's last row but one reads %q, want its keys", width, height, rows[bottom-1])
	}
	if key := strings.TrimSpace(rows[height-1]); key != "" {
		t.Errorf("at %d×%d, the key line reads %q, want it empty while the card's open", width, height, key)
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
