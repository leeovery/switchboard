package capture

import (
	"cmp"
	"regexp"
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
					sized := deliver(m, tea.WindowSizeMsg{Width: width, Height: height})
					rows := plainOf(sized.View().Content, height)
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
					scrolled := deliver(sized, keysOf("j")[0])
					if rows := plainOf(scrolled.View().Content, height); !showsAnAccountItCounts(rows, names) {
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
	return plainOf(m.View().Content, height)
}

// plainOf are the rows of a frame's ANSI as plain text, as many as it's
// tall.
func plainOf(content string, height int) []string {
	rows := strings.Split(ansi.Strip(content), "\n")
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}

// The sizes the scroll sweep draws every fixture at: the widths either side
// of those a layout changes at, and heights above the floor, where every
// view has rows to page through.
var (
	pagedWidths  = []int{52, 80, 120, 160}
	pagedHeights = []int{14, 16, 18, 20, 22, 24, 26, 30, 36}
)

// Every fixture's view, at a grid of sizes above the floor, paged up from
// where it's drawn as far as it goes, then down: no window or section
// heading is drawn without its first row; and where it pages, no line the
// paging changes says more lies beyond either end. History's views, the
// Overview's USAGE, Accounts' views and a phone's Switchboard, once they
// have two rows to page, reach every row they draw as tall as everything
// fits, paged down from the top, and up from the foot, so the frame cuts
// nothing short that they can't scroll to; and say on their first page that
// more lies below, where they page, but the Year, as its frames draw it. The
// Log's tables are passed over: they hold no heading, say where they are on their own status line, and paging a day's
// lines at every size would take the sweep's time many times over.
func TestNoViewCutsWhatItCantScrollTo(t *testing.T) {
	for _, name := range Names() {
		f := fixture(t, name)
		if f.Printout || strings.HasPrefix(name, "log-") || strings.HasPrefix(name, "page-log-") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			for _, width := range pagedWidths {
				reached, tall := reachOf(name, width), plainRows(m, width, tallest)
				for _, height := range pagedHeights {
					pages := pagesOf(m, width, height)
					for _, p := range pages {
						if y := headingAlone(p, chromeOf(width)); y >= 0 {
							t.Errorf("at %d×%d, row %d is a heading without its first row:\n%s", width, height, y+1, strings.Join(p.rows, "\n"))
						}
					}
					if beyond := saidBeyond(pages); beyond != "" {
						t.Errorf("at %d×%d, paged as far as it goes, the view says %q:\n%s", width, height, beyond, strings.Join(pages[len(pages)-1].rows, "\n"))
					}
					if reached == nil || len(reached(pages[0].rows)) < 2 {
						continue
					}
					if first := pages[0]; len(pages) > 1 && !strings.Contains(name, "-year") && !saysMore(first.rows, "▼") {
						t.Errorf("at %d×%d, the view pages, but its first page doesn't say more lies below:\n%s", width, height, strings.Join(first.rows, "\n"))
					}
					if missing := unreached(reached, pages, tall, width); len(missing) > 0 {
						t.Errorf("at %d×%d, paged as far as it goes, the view never shows %d of the rows it draws at %d rows:\n%s", width, height, len(missing), tallest, strings.Join(missing, "\n"))
					}
					if missing := unreached(reached, pagesUp(m, width, height), tall, width); len(missing) > 0 {
						t.Errorf("at %d×%d, paged up from as far down as it goes, the view never shows %d of the rows it draws at %d rows:\n%s", width, height, len(missing), tallest, strings.Join(missing, "\n"))
					}
				}
			}
		})
	}
}

// saidBeyond is what a line the paging changes says lies beyond either end
// of the pages: ▼ and how many more on the last, or ▲ on the first, that
// the other end doesn't say alike, as a line saying what's out of view of
// something the pages don't scroll, as Detail's list, says it at both; ""
// where none does.
func saidBeyond(pages []paged) string {
	first, last := pages[0].rows, pages[len(pages)-1].rows
	for _, end := range []struct {
		rows, other []string
		mark        string
	}{{rows: last, other: first, mark: "▼"}, {rows: first, other: last, mark: "▲"}} {
		for _, said := range outLine.FindAllString(strings.Join(end.rows, "\n"), -1) {
			if strings.HasPrefix(said, end.mark) && !strings.Contains(strings.Join(end.other, "\n"), said) {
				return said
			}
		}
	}
	return ""
}

// paged is a page of a frame as the screen draws it, and its rows as plain
// text.
type paged struct {
	content string
	rows    []string
}

// mostPages is the furthest the sweep pages a view either way.
const mostPages = 400

// pagesOf are the pages of m drawn at the size given, from the top: paged up
// from where it's drawn with PgUp as far as it goes, then down with PgDn.
func pagesOf(m watch.Model, width, height int) []paged {
	m = deliver(m, tea.WindowSizeMsg{Width: width, Height: height})
	up := pagedBy(m, height, tea.KeyPgUp)
	slices.Reverse(up)
	return append(up, pagedBy(m, height, tea.KeyPgDown)[1:]...)
}

// pagesUp are the pages of m drawn at the size given, paged up with PgUp
// from as far down as PgDn takes it.
func pagesUp(m watch.Model, width, height int) []paged {
	m = deliver(m, tea.WindowSizeMsg{Width: width, Height: height})
	for range mostPages {
		next := deliver(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
		if next.View().Content == m.View().Content {
			break
		}
		m = next
	}
	return pagedBy(m, height, tea.KeyPgUp)
}

// pagedBy are the pages of m, drawn height rows tall, as the key given pages
// it from where it's drawn, till a press of it changes nothing.
func pagedBy(m watch.Model, height int, key rune) []paged {
	content := m.View().Content
	pages := []paged{{content: content, rows: plainOf(content, height)}}
	for range mostPages {
		m = deliver(m, tea.KeyPressMsg{Code: key})
		if content = m.View().Content; content == pages[len(pages)-1].content {
			break
		}
		pages = append(pages, paged{content: content, rows: plainOf(content, height)})
	}
	return pages
}

// chromeOf is how many rows the title, the status line and the rows between
// take at the width given, the view's heading on the row after the next.
func chromeOf(width int) int {
	if dashboard.Phone(width) {
		return 4
	}
	return 3
}

var (
	// headingText is a window's or a section's heading, as the views draw
	// them: words in capitals, as 5-HOUR, BY ACCOUNT, or the matrix's heads,
	// and a note after them, as "share of use".
	headingText = regexp.MustCompile(`^[A-Z0-9-]*[A-Z]{2}[A-Z0-9-]*( +[A-Z0-9][A-Z0-9-]*)*( {2,}[a-z(].*)?$`)
	// ruledText is a heading in words ruled to the view's edge, as the
	// List's ended today, or Detail's the last six weeks, ruled from its
	// left too.
	ruledText = regexp.MustCompile(`^(┄+ )?[a-z][a-z ]*[a-z] +(─+|┄+)$`)
	// outLine is a line saying how much of a view is out of view.
	outLine = regexp.MustCompile(`[▲▼] [0-9,]+ more`)
)

// panelOf is the part of a row a view scrolls, where something held beside
// it is ruled off, as Detail's list is: what's after the rule.
func panelOf(row string) string {
	if _, after, found := strings.Cut(row, "│"); found {
		return panelOf(after)
	}
	return row
}

// headingAlone is the row of the page drawing a heading, in capitals or
// ruled, with nothing under it before what ends its view: a line saying
// what's out of view, a section's rule, USAGE's key line, the List's today
// line, or the key line; -1 where none does. A heading on its view's first
// row has no row over it to end a page on, and one drawn on a bar, as BY
// MODEL's is, heads the bar.
func headingAlone(p paged, chrome int) int {
	for y := chrome + 2; y < len(p.rows)-1; y++ {
		text := strings.TrimSpace(panelOf(p.rows[y]))
		if !ruledText.MatchString(text) && (!headingText.MatchString(text) || strings.Contains(text, "─")) {
			continue
		}
		if viewsFirst(p.rows, y, chrome) || headsARow(p.rows, y) || onABar(p, y) {
			continue
		}
		return y
	}
	return -1
}

// viewsFirst reports whether row y is the first its view draws on: nothing
// over it but blank rows, a line saying what's out of view above, or a
// section's rule, up to the view's heading.
func viewsFirst(rows []string, y, chrome int) bool {
	for r := y - 1; r > chrome+1; r-- {
		text := strings.TrimSpace(panelOf(rows[r]))
		if strings.Contains(text, "─") || outLine.MatchString(text) {
			return true
		}
		if text != "" {
			return false
		}
	}
	return true
}

// headsARow reports whether a row under row y is drawn on before what ends
// its view, as headingAlone has it: the row a line saying what lies below is
// on, a section's rule, USAGE's key line, the List's today line, or the key
// line.
func headsARow(rows []string, y int) bool {
	for _, row := range rows[y+1 : len(rows)-1] {
		text := strings.TrimSpace(panelOf(row))
		switch {
		case text == "":
			continue
		case strings.Contains(outLine.FindString(text), "▼"), strings.Contains(text, "─"), strings.HasPrefix(text, "┃ even pace"), strings.HasPrefix(text, "today  "):
			return false
		}
		return true
	}
	return false
}

// onABar reports whether row y draws on a ground of its own anywhere, as a
// bar does.
func onABar(p paged, y int) bool {
	g, err := parse(p.content)
	if err != nil || y >= len(g) {
		return false
	}
	grounds := make(map[colour]bool)
	for _, c := range g[y] {
		grounds[c.bg] = true
	}
	return len(grounds) > 1
}

// saysMore reports whether a row says how much is out of view the way the
// mark given points: ▲ above, ▼ below.
func saysMore(rows []string, mark string) bool {
	return slices.ContainsFunc(rows, func(row string) bool {
		return slices.ContainsFunc(outLine.FindAllString(row, -1), func(said string) bool { return strings.HasPrefix(said, mark) })
	})
}

// reachOf is what of the fixture's view, by name, at the width given, the
// sweep holds paged to reach every row of, from its rows as drawn: History's
// views, from the row under the view's heading, but for their footnote;
// the Overview's USAGE, from its heading to the next section's, but for its
// key line, where nothing is drawn beside it; Accounts' views, but for
// Detail's list; a phone's Switchboard in lanes, from the row under the
// view's heading to the foot, which gives way where they scroll. Elsewhere
// it's nil: the rest of the views fold and drop what they show by the room
// they have, as the design lays them out.
func reachOf(name string, width int) func(rows []string) []string {
	chrome := chromeOf(width)
	switch {
	case strings.HasPrefix(name, "history-"):
		return func(rows []string) []string {
			view := rows[chrome+2 : len(rows)-1]
			if i := slices.IndexFunc(view, func(r string) bool { return strings.HasPrefix(r, "worth: ") }); i >= 0 {
				view = view[:i]
			}
			return shownRows(view)
		}
	case strings.HasPrefix(name, "overview-") && width < 150:
		return func(rows []string) []string {
			from := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "USAGE ") })
			if from < 0 {
				return nil
			}
			view := rows[from+1:]
			if to := slices.IndexFunc(view, func(r string) bool { return strings.Contains(r, "─") || strings.HasPrefix(r, "┃ even pace") }); to >= 0 {
				view = view[:to]
			}
			return shownRows(view)
		}
	case strings.HasPrefix(name, "accounts-"):
		return func(rows []string) []string {
			view := make([]string, 0, len(rows))
			for _, r := range rows[chrome+2 : len(rows)-1] {
				view = append(view, panelOf(r))
			}
			return shownRows(view)
		}
	case strings.HasPrefix(name, "sessions-") && !strings.Contains(name, "-list") && dashboard.Phone(width):
		return func(rows []string) []string {
			view := rows[chrome+2 : len(rows)-1]
			if to := slices.IndexFunc(view, func(r string) bool { return strings.HasPrefix(r, "┃ even pace") }); to >= 0 {
				view = view[:to]
			}
			return lanesRead(shownRows(view))
		}
	}
	return nil
}

var (
	// laneName is a lane's name row, its account's number first.
	laneName = regexp.MustCompile(`^ ?([0-9]+) `)
	// laneWindows is a lane's windows' row: a window's label a cell in.
	laneWindows = regexp.MustCompile(`^ [^ ]`)
	// shownPercent is a bar's percentage, or — where it shows none.
	shownPercent = regexp.MustCompile(`^([0-9]+%|—)$`)
)

// lanesRead are a phone's Switchboard's rows as the sweep reaches them: each
// account's name row and each session's as drawn; and as the lanes stack
// their windows a row each where rows are free, and side by side where
// they're short, each window a windows' row shows as its account's number,
// its label and its percentage.
func lanesRead(rows []string) []string {
	var read []string
	account := ""
	for _, r := range rows {
		m := laneName.FindStringSubmatch(r)
		if m != nil {
			account = m[1]
		}
		if m != nil || !laneWindows.MatchString(r) {
			read = append(read, r)
			continue
		}
		fields := strings.Fields(r)
		label := fields[0]
		for i, f := range fields[1:] {
			if shownPercent.MatchString(f) {
				read = append(read, account+" "+label+" "+f)
				if i+2 < len(fields) {
					label = fields[i+2]
				}
			}
		}
	}
	return read
}

// shownRows are the rows given drawn on, trimmed at their right, but for
// those saying what's out of view.
func shownRows(rows []string) []string {
	var shown []string
	for _, r := range rows {
		if r = strings.TrimRight(r, " "); r != "" && !outLine.MatchString(r) {
			shown = append(shown, r)
		}
	}
	return shown
}

// unreached are the rows reached reads of tall, the view drawn as tall as
// everything fits, that none of the pages shows, in the order tall draws
// them.
func unreached(reached func([]string) []string, pages []paged, tall []string, width int) []string {
	seen := make(map[string]bool)
	for _, p := range pages {
		for _, r := range reached(p.rows) {
			seen[r] = true
		}
	}
	var missing []string
	for _, r := range reached(tall) {
		if !seen[r] {
			missing = append(missing, ansi.Truncate(r, width, ""))
		}
	}
	return missing
}
