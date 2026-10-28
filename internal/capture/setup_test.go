package capture

import (
	"strings"
	"testing"

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
// least: at its least and larger, it fills the terminal, its chrome, the
// title row, the status line, the rows round them and the key line on its
// last row, read as those of the same screen drawn as tall as anything
// fits, so no page draws on them; smaller either way, it says only to make
// the terminal larger.
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
					for y := range 4 {
						if rows[y] != tall[y] {
							t.Errorf("at %d×%d, row %d reads\n%q\nwant it as at %d rows\n%q", width, height, y+1, rows[y], tallest, tall[y])
						}
					}
					if key := rows[height-1]; key == "" || key != tall[tallest-1] {
						t.Errorf("at %d×%d, the last row reads\n%q\nwant the key line, as at %d rows\n%q", width, height, key, tallest, tall[tallest-1])
					}
				}
			}
		})
	}
	if swept == 0 {
		t.Fatal("no fixture draws setup's screen: the rule above holds nothing")
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
