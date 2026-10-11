package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// referenceDir is where milestone 7's signed-off frames are kept, each by its
// stem, as the text and ANSI of the generator that drew it and a PNG.
const referenceDir = "../../testdata/vhs/reference"

// owed are the frames no fixture draws yet. Each stage that builds a view
// adds its fixtures, named after the frames they draw, and takes their stems
// from here; by stage 14 none is owed.
var owed = []string{
	"accounts-compare-160", "accounts-compare-bad-hour-160", "accounts-compare-month-160", "accounts-compare-ten-160",
	"accounts-detail-all-160", "accounts-detail-limit-160", "accounts-detail-one-160", "accounts-detail-sessions-160",
	"accounts-detail-work-160", "accounts-phone-compare-52", "accounts-phone-detail-52",
	"history-days-160", "history-days-ten-160", "history-phone-year-52", "history-tokens-160", "history-tokens-models-160",
	"history-weeks-160", "history-weeks-columns-160", "history-weeks-ten-160", "history-year-160", "history-year-opus-160",
	"log-event-details-160", "log-events-160", "log-events-filter-160", "log-not-live-160", "log-request-details-160",
	"log-requests-160", "log-requests-filter-160", "log-requests-filtered-160", "log-requests-phone-52",
	"overview-bad-hour-160", "overview-by-account-160", "overview-default-160", "overview-default-216",
	"overview-everything-160x39", "overview-everything-216x106", "overview-phone-52", "overview-pinned-160",
	"overview-sessions-216", "overview-sessions-graphs-216", "overview-six-accounts-160", "overview-ten-accounts-160",
	"overview-usage-alone-160",
	"routing-auto-160", "routing-limit-160", "routing-move-160", "routing-session-160", "routing-two-160",
	"runway-5-hour-160", "runway-phone-52", "runway-stepped-back-160", "runway-ten-accounts-160", "runway-week-160",
	"session-ended-160", "session-moved-160", "session-phone-52", "session-quiet-160", "session-resumed-160",
	"session-turn-opened-160",
	"sessions-list-160", "sessions-list-phone-52", "sessions-moved-160", "sessions-one-account-160", "sessions-phone-52",
	"sessions-pinned-160", "sessions-refused-160", "sessions-switchboard-160", "sessions-ten-accounts-160",
}

// passing is cells a frame drew otherwise than the design says, which the
// test passes over: their span, and what the frame's .txt reads there, from
// its first glyph that isn't blank.
type passing struct {
	span
	reads string
}

// wholeRow passes over row n, which reads as given.
func wholeRow(n int, reads string) passing {
	return passing{span{row: n}, reads}
}

// cellsOf passes over row n's cells from and to, which read as given.
func cellsOf(n, from, to int, reads string) passing {
	return passing{span{row: n, from: from, to: to}, reads}
}

// How the rows passed over whole start, in the frames that drew them before
// the keys were swept.
const (
	// overviewKeys starts an Overview's key line.
	overviewKeys = "tab views     u usage"
	// accountsKeys starts the key line of Accounts wider than a phone.
	accountsKeys = "tab views   [ ] view     p period"
	// historyKeys starts the key line of History wider than a phone.
	historyKeys = "tab views   [ ] view   p period"
	// pinnedRouting starts ROUTING's heading under a pin, which ends with
	// its key back to auto, 0.
	pinnedRouting = "ROUTING   pinned to"
)

// period passes over the period on a heading row 160 wide, its key, p, at
// column p: the period and its key, with the rule's last two cells, as the
// period between its keys, < and >, is two cells wider.
func period(p int, reads string) passing {
	return cellsOf(5, p-4, 160, "──  p "+reads)
}

// phonePeriod passes over a phone's period, the row under its heading.
func phonePeriod(reads string) passing {
	return wholeRow(7, "p "+reads)
}

// How usage's frames word side's week, at its allowance, where it runs out
// first, at its average pace: in full, and short.
const (
	sidesWeek      = "6%/day till Mon 10:00"
	sidesWeekShort = "6%/day · Mon"
)

// passedOver is, by stem, the rows and cells each frame drew otherwise than
// the design says, which testdata/vhs/README.md lists, with what the
// dashboard draws in their place.
var passedOver = map[string][]passing{
	"usage-160":             {cellsOf(9, 88, 108, sidesWeek)},
	"usage-120":             {cellsOf(20, 98, 118, sidesWeek)},
	"usage-80":              {cellsOf(22, 58, 78, sidesWeek)},
	"usage-52":              {cellsOf(22, 38, 49, sidesWeekShort)},
	"usage-by-account-160":  {cellsOf(20, 138, 158, sidesWeek), wholeRow(31, "┃ even pace")},
	"usage-by-account-52":   {cellsOf(25, 38, 49, sidesWeekShort)},
	"usage-one-account-160": {cellsOf(5, 8, 10, "▲")},

	"overview-bad-hour-160":        {wholeRow(19, pinnedRouting), wholeRow(37, overviewKeys)},
	"overview-by-account-160":      {wholeRow(42, overviewKeys)},
	"overview-default-160":         {wholeRow(37, overviewKeys)},
	"overview-default-216":         {wholeRow(70, overviewKeys)},
	"overview-everything-160x39":   {wholeRow(39, overviewKeys)},
	"overview-everything-216x106":  {wholeRow(106, overviewKeys)},
	"overview-pinned-160":          {wholeRow(19, pinnedRouting), wholeRow(37, overviewKeys)},
	"overview-sessions-216":        {wholeRow(70, overviewKeys)},
	"overview-sessions-graphs-216": {wholeRow(70, overviewKeys)},
	"overview-six-accounts-160":    {wholeRow(41, overviewKeys)},
	"overview-ten-accounts-160":    {wholeRow(53, overviewKeys)},
	"overview-usage-alone-160":     {wholeRow(37, overviewKeys)},

	"accounts-compare-160":          {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-compare-bad-hour-160": {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-compare-month-160":    {period(152, "30 days"), wholeRow(37, accountsKeys)},
	"accounts-compare-ten-160":      {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-detail-all-160":       {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-detail-limit-160":     {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-detail-one-160":       {period(150, "this week"), cellsOf(28, 20, 20, "●"), wholeRow(37, accountsKeys)},
	"accounts-detail-sessions-160":  {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-detail-work-160":      {period(150, "this week"), wholeRow(37, accountsKeys)},
	"accounts-phone-compare-52":     {phonePeriod("this week")},
	"accounts-phone-detail-52":      {phonePeriod("this week")},

	"history-days-160":          {period(148, "eight weeks"), wholeRow(37, historyKeys)},
	"history-days-ten-160":      {period(148, "eight weeks"), wholeRow(37, historyKeys)},
	"history-phone-year-52":     {phonePeriod("last 12 months")},
	"history-tokens-160":        {period(152, "by week"), wholeRow(37, historyKeys)},
	"history-tokens-models-160": {period(152, "by week"), wholeRow(37, historyKeys)},
	"history-weeks-160": {
		period(148, "eight weeks"),
		cellsOf(15, 137, 137, "6"), cellsOf(16, 138, 138, "5"), cellsOf(17, 139, 139, "3"),
		wholeRow(37, historyKeys),
	},
	"history-weeks-columns-160": {
		period(148, "eight weeks"),
		cellsOf(15, 114, 114, "6"), cellsOf(21, 114, 114, "5"), cellsOf(27, 114, 114, "3"),
		wholeRow(37, historyKeys),
	},
	"history-weeks-ten-160": {
		period(148, "eight weeks"),
		cellsOf(8, 18, 18, "4"), cellsOf(8, 96, 96, "4"), cellsOf(8, 150, 150, "6"),
		cellsOf(9, 36, 42, "5% used"),
		cellsOf(13, 92, 92, "4"),
		cellsOf(19, 93, 99, "5% used"),
		cellsOf(22, 96, 114, "80% left at a reset"),
		wholeRow(37, historyKeys),
	},
	"history-year-160":      {period(145, "last 12 months"), wholeRow(37, historyKeys)},
	"history-year-opus-160": {period(145, "last 12 months"), wholeRow(37, historyKeys)},

	"session-phone-52":         {wholeRow(32, "esc back   ↑↓ turn   enter open   ? keys")},
	"session-turn-opened-160":  {wholeRow(37, "esc close    ↑↓ request  p pin")},
	"sessions-one-account-160": {wholeRow(37, "tab views   [ ] view    ↑↓ session  enter its page    p pin")},
	"runway-phone-52":          {wholeRow(33, "< > window   ←→ day   ? keys")},
}

func TestReferenceKeepsEachFrameAsTextANSIAndPNG(t *testing.T) {
	entries, err := os.ReadDir(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string][]string{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		stem := strings.TrimSuffix(e.Name(), ext)
		kinds[stem] = append(kinds[stem], ext)
	}
	for stem, exts := range kinds {
		slices.Sort(exts)
		if want := []string{".ansi", ".png", ".txt"}; !slices.Equal(exts, want) {
			t.Errorf("reference/ keeps %s as %v, want %v", stem, exts, want)
		}
	}
}

func TestEveryFrameIsDrawnByAFixtureOrOwed(t *testing.T) {
	stems, names := stems(t), Names()
	for _, stem := range stems {
		drawn, isOwed := slices.Contains(names, stem), slices.Contains(owed, stem)
		switch {
		case drawn && isOwed:
			t.Errorf("%s is drawn by a fixture, and still owed one: take it from owed", stem)
		case !drawn && !isOwed:
			t.Errorf("no fixture draws %s: name one after it, or owe it", stem)
		}
	}
	for _, stem := range owed {
		if !slices.Contains(stems, stem) {
			t.Errorf("owed %s, which reference/ doesn't keep", stem)
		}
	}
	if compacted := slices.Compact(slices.Sorted(slices.Values(owed))); len(compacted) != len(owed) {
		t.Errorf("owed names a frame twice")
	}
}

func TestEveryFramesANSIParsesToItsText(t *testing.T) {
	for _, stem := range stems(t) {
		t.Run(stem, func(t *testing.T) {
			g, text := reference(t, stem)
			if got := g.text(); !slices.Equal(got, text) {
				t.Errorf("its ANSI reads\n%s\nwant its .txt\n%s", strings.Join(got, "\n"), strings.Join(text, "\n"))
			}
			width, height := sizeOf(t, stem)
			if height != 0 && len(g) != height {
				t.Errorf("it's %d rows, want %d", len(g), height)
			}
			for r, row := range g {
				if len(row) != width {
					t.Errorf("row %d is %d columns, want %d", r+1, len(row), width)
				}
				for c, cell := range row {
					if !cell.fg.painted || !cell.bg.painted {
						t.Errorf("row %d, column %d isn't painted: %s", r+1, c+1, cell)
					}
				}
			}
		})
	}
}

func TestEveryFixtureNamedAfterAFrameDrawsItCellForCell(t *testing.T) {
	utc := fixturesIn(time.UTC)
	canvas := rgb(terminalBackground.R, terminalBackground.G, terminalBackground.B)
	names := Names()
	for _, stem := range stems(t) {
		if !slices.Contains(names, stem) {
			continue
		}
		t.Run(stem, func(t *testing.T) {
			frame, _ := reference(t, stem)
			// A fixture over records is drawn in time.Local whatever the zone
			// asked, as fixtures builds it.
			drawn, err := parse(frameOf(t, utc, stem))
			if err != nil {
				t.Fatalf("parsing what %s draws: %v", stem, err)
			}
			diffs, err := compare(frame, drawn, canvas, spans(passedOver[stem]))
			if err != nil {
				t.Fatalf("%s: %v", stem, err)
			}
			if len(diffs) > 0 {
				t.Error(describe(stem, diffs))
			}
		})
	}
}

func TestEachCellPassedOverIsInItsFrame(t *testing.T) {
	stems := stems(t)
	for stem, passings := range passedOver {
		if !slices.Contains(stems, stem) {
			t.Errorf("passes over cells of %s, which reference/ doesn't keep", stem)
			continue
		}
		g, _ := reference(t, stem)
		for _, p := range passings {
			if !within(p.span, g) {
				t.Errorf("%s: %v is out of its frame, %d rows by %d columns", stem, p.span, len(g), len(g[0]))
				continue
			}
			if got := strings.TrimSpace(textOf(g, p.span)); p.reads == "" || !strings.HasPrefix(got, p.reads) {
				t.Errorf("%s: %v reads %q, want it to start %q, which isn't blank", stem, p.span, got, p.reads)
			}
		}
	}
}

func TestTheReadmeListsEveryFramePassedOver(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(referenceDir, "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for stem := range passedOver {
		if !strings.Contains(string(readme), "`"+stem+"`") {
			t.Errorf("testdata/vhs/README.md doesn't list %s, whose cells the test passes over", stem)
		}
	}
}

// stems are the frames reference/ keeps, by the stems of their .txt files,
// sorted.
func stems(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(referenceDir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("reference/ keeps no frames")
	}
	var stems []string
	for _, p := range paths {
		stems = append(stems, strings.TrimSuffix(filepath.Base(p), ".txt"))
	}
	return stems
}

// reference is the frame kept by the stem given, as its ANSI's cells, and its
// .txt's lines.
func reference(t *testing.T, stem string) (grid, []string) {
	t.Helper()
	ansi, err := os.ReadFile(filepath.Join(referenceDir, stem+".ansi"))
	if err != nil {
		t.Fatal(err)
	}
	txt, err := os.ReadFile(filepath.Join(referenceDir, stem+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := parse(string(ansi))
	if err != nil {
		t.Fatalf("parsing %s.ansi: %v", stem, err)
	}
	return g, strings.Split(strings.TrimSuffix(string(txt), "\n"), "\n")
}

// sizeOf is the size a frame's stem ends with: its width, as in -160, and its
// height where it's given too, as in -160x39, else 0.
func sizeOf(t *testing.T, stem string) (width, height int) {
	t.Helper()
	size := stem[strings.LastIndex(stem, "-")+1:]
	if _, err := fmt.Sscanf(size, "%dx%d", &width, &height); err == nil {
		return width, height
	}
	if _, err := fmt.Sscanf(size, "%d", &width); err != nil {
		t.Fatalf("%s ends with no size: %v", stem, err)
	}
	return width, 0
}

// spans are the spans the passings given pass over.
func spans(passings []passing) []span {
	var s []span
	for _, p := range passings {
		s = append(s, p.span)
	}
	return s
}

// within reports whether s is a span of g's cells: its row one of g's, and
// its columns, where it has them, in order within the row.
func within(s span, g grid) bool {
	if s.row < 1 || s.row > len(g) {
		return false
	}
	return s.from == 0 || 1 <= s.from && s.from <= s.to && s.to <= len(g[s.row-1])
}

// textOf is the glyphs of g the span covers.
func textOf(g grid, s span) string {
	var b strings.Builder
	for c, cell := range g[s.row-1] {
		if s.covers(s.row, c+1) {
			b.WriteRune(cell.glyph)
		}
	}
	return b.String()
}
