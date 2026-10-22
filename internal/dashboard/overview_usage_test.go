package dashboard

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// usageAccount is an account read a minute before routedAt, labelled as
// its id: its 5-hour window used as given, resetting at session, and its week
// and Fable's week used as given, both resetting at week.
func usageAccount(id string, used float64, session time.Time, weekUsed, fableUsed float64, week time.Time) status.Account {
	return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: routedAt.Add(-time.Minute), Windows: []quota.Window{
		{Key: "5h", Label: "Session", Utilization: used, ResetsAt: session},
		{Key: "7d", Label: "Week", Utilization: weekUsed, ResetsAt: week},
		{Key: "7d_oi", Label: "Fable week", Utilization: fableUsed, ResetsAt: week},
	}}
}

// Every live view lists each model's own window, as Fable's week, only while
// w shows them, as the moment's Listed has it: USAGE, by window and by
// account; GRAPHS; Accounts' Detail; Sessions' Switchboard; and a session's
// page. One nearing its limit shows all the same.
func TestEveryLiveViewListsAModelsWindowWhileWShowsThemOrItNearsItsLimit(t *testing.T) {
	graphsAlone := frameOf(160, 40)
	graphsAlone.Sections = Sections{}.Toggled(SectionUsage).Toggled(SectionGraphs)
	byAccount := frameOf(160, 40)
	byAccount.By = ByAccount
	nearing := func(d *status.Document) { d.Accounts[2].Windows[2].Utilization = 0.95 }
	tests := []struct {
		name  string
		f     Frame
		doc   status.Document
		now   time.Time
		fable string
	}{
		{name: "USAGE by window", f: frameOf(160, 40), doc: usageWorld(), now: routedAt, fable: "FABLE WEEK"},
		{name: "USAGE by account", f: byAccount, doc: usageWorld(), now: routedAt, fable: "Fable wk"},
		{name: "GRAPHS", f: graphsAlone, doc: usageWorld(), now: routedAt, fable: "FABLE WEEK"},
		{name: "Accounts' Detail", f: detailFrame(Look{}, framedAccounts, "side", "", nil, nil), doc: usageWorld(), now: routedAt, fable: "Fable wk"},
		{name: "Sessions' Switchboard", f: sceneFrame(160, 37, Look{}), doc: usageWorld(), now: routedAt, fable: "Fable wk"},
		{name: "a session's page", f: pageFrame(t, resumed(), 160, 37), doc: pageDoc(), now: sessionNow, fable: "fable week"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, models := range []bool{false, true} {
				tt.f.Models = models
				text := strings.Join(textOf(tt.f.Draw(tt.doc, tt.now)), "\n")
				if strings.Contains(text, tt.fable) != models {
					t.Errorf("with models %t, it says %q %t, want %t:\n%s", models, tt.fable, !models, models, text)
				}
			}
		})
	}
	t.Run("nearing its limit", func(t *testing.T) {
		f := frameOf(160, 40)
		f.Models = false
		if text := strings.Join(textOf(f.Draw(usageWorld(nearing), routedAt)), "\n"); !strings.Contains(text, "FABLE WEEK") {
			t.Errorf("side's Fable week at 95%%, models hidden, USAGE reads\n%s\nwant it listed", text)
		}
	})
}

// usageWorld is the world the Overview's frames are drawn of, as change
// leaves it, with what's worked out of it: work, capped at 95%, at its cap in
// its 5-hour window; personal, the primary, its week running out ~Sat 06:00
// at its recent rate; and side, where new sessions go.
func usageWorld(change ...func(*status.Document)) status.Document {
	work := usageAccount("work", 0.95, routedClock(1, 17, 10), 0.34, 0.12, routedClock(4, 21, 0))
	work.Reserve = 0.05
	personal := usageAccount("personal", 0.39, routedClock(1, 16, 30), 0.66, 0, routedClock(4, 2, 0))
	personal.Primary = true
	runsOut := time.Date(2026, 10, 3, 6, 0, 30, 0, time.UTC).Sub(routedAt).Hours()
	personal.Rates = []status.Rate{{Window: "7d", Rate: 0.34 / runsOut, Since: routedAt.Add(-30 * time.Minute)}}
	side := usageAccount("side", 0.13, routedClock(1, 18, 50), 0.78, 0.04, routedClock(5, 10, 0))
	doc := status.Document{
		GeneratedAt: routedAt, Source: status.SourceRouter, Best: "side", Primary: "personal",
		Router: status.Health{Healthy: true}, Sessions: 6, Accounts: []status.Account{work, personal, side},
	}
	for _, c := range change {
		c(&doc)
	}
	return doc.WorkedOut(claudeLike)
}

// usagePinned has new sessions pinned to the accounts with the ids given,
// since 14:02, the first where they go.
func usagePinned(ids ...string) func(*status.Document) {
	return func(d *status.Document) {
		d.Pin, d.Best = status.Pin{Accounts: ids, Since: routedClock(1, 14, 2)}, ids[0]
	}
}

// usageBody is USAGE of doc at the moment given as f draws its body in rows
// rows from row 0, as text, and the canvas it's drawn on.
func usageBody(f Frame, doc status.Document, at time.Time, rows int) ([]string, *canvas) {
	c := newCanvas(f.Width, rows)
	f.usageSection(doc, at).draw(c, bounds{width: f.Width, rows: rows})
	return textOf(c.rows(Look{})), c
}

// manyRead is the router's document of n accounts, a1 to an, each read a
// minute before now, new sessions going to a1.
func manyRead(n int) status.Document {
	accounts := make([]status.Account, n)
	for i := range accounts {
		accounts[i] = readAccount(fmt.Sprintf("a%d", i+1), sessionOf(0.1, time.Hour), weekOf(0.2, 3*quota.Day))
	}
	return routerDoc("a1", 0, accounts...)
}

// startsAt fails the test where any of the rows given doesn't start as want
// has it, by its index, or where want has it "", isn't blank.
func startsAt(t *testing.T, rows []string, want map[int]string) {
	t.Helper()
	for row, text := range want {
		if row >= len(rows) || !strings.HasPrefix(rows[row], text) || text == "" && rows[row] != "" {
			t.Errorf("row %d reads %q, want it to start %q", row, rows[min(row, len(rows)-1)], text)
		}
	}
}

// Ten accounts in the matrix, 25 rows laid out whole, its key line among
// them, scroll whole in 15, a row at a time: the windows' heading on row 0,
// then a1 to a10 two rows apart from row 2, the pool's rule on 21 and the
// pool on 22, the key line on 24; a page of 14 over the row saying what's
// out of view. Scrolled into the accounts' rows, the heading is kept on the
// first row, in place of the row under it there.
func TestUSAGEScrollsWholeARowAtATimeItsWindowsHeadingKept(t *testing.T) {
	f := frameOf(160, 40)
	doc := manyRead(10)
	key := strings.Repeat(" ", 14) + "┃ even pace"
	tests := []struct {
		name   string
		scroll int
		want   map[int]string
	}{
		{
			name: "at the top, the rows below said", scroll: 0,
			want: map[int]string{2: " 1 a1 ▲", 4: " 2 a2", 12: " 6 a6", 13: "", 14: "▼ 11 more below"},
		},
		{
			name: "a row down, the blank under the heading under it", scroll: 1,
			want: map[int]string{1: " 1 a1 ▲", 3: " 2 a2", 13: " 7 a7", 14: "▲ 1 more above · ▼ 10 more below"},
		},
		{
			name: "four rows down, a2 under the heading", scroll: 4,
			want: map[int]string{1: "", 2: " 3 a3", 4: " 4 a4", 12: " 8 a8", 14: "▲ 4 more above · ▼ 7 more below"},
		},
		{
			name: "past the end, no further than shows the key line", scroll: 99,
			want: map[int]string{1: " 6 a6", 9: "10 a10", 10: "────", 11: " Σ pool", 12: "", 13: key, 14: "▲ 11 more above"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Scroll = tt.scroll
			rows, c := usageBody(f, doc, now, 15)
			if !strings.Contains(rows[0], "5-HOUR") || !strings.Contains(rows[0], "WEEK") || strings.Contains(rows[0], "%") {
				t.Errorf("the first row reads %q, want the windows' heading alone", rows[0])
			}
			startsAt(t, rows, tt.want)
			if k := c.at(0, 14).ink; k != dimInk {
				t.Errorf("the row saying what's out of view is drawn in %+v, want text.subtle", k)
			}
		})
	}
}

// Stacked at 120 columns, the usage world's three windows are 32 rows laid
// out whole: the week's heading on row 0, its accounts on 2, 4 and 6, its
// pool on 8; the 5-hour's heading on 10, its accounts on 12, 14 and 16; Fable
// week's on 20, its accounts on 22, 24 and 26, its pool on 28; the key line
// on 30 and 31. In 8 rows, each window's heading is kept on the first row
// while the scroll is among its rows, and hands it to the next window's as
// the scroll passes into them; the blank row between two windows is
// neither's.
func TestStackedUSAGEKeepsEachWindowsHeadingOverItsRowsAndHandsItOn(t *testing.T) {
	f := frameOf(120, 60)
	tests := []struct {
		scroll int
		want   map[int]string
	}{
		{scroll: 0, want: map[int]string{0: "WEEK", 2: " 1 work"}},
		{scroll: 3, want: map[int]string{0: "WEEK", 1: " 2 personal", 3: " 3 side", 5: " Σ pool"}},
		{scroll: 9, want: map[int]string{0: "", 1: "5-HOUR", 3: " 1 work"}},
		{scroll: 13, want: map[int]string{0: "5-HOUR", 1: " 2 personal"}},
		{scroll: 23, want: map[int]string{0: "FABLE WEEK", 1: " 2 personal", 5: " Σ pool"}},
		{scroll: 99, want: map[int]string{0: "FABLE WEEK", 1: " 3 side", 4: "", 5: "┃ even pace", 7: "▲ 25 more above"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("scrolled %d", tt.scroll), func(t *testing.T) {
			f.Scroll = tt.scroll
			rows, _ := usageBody(f, usageWorld(), now, 8)
			startsAt(t, rows, tt.want)
			if t.Failed() {
				t.Logf("USAGE reads\n%s", strings.Join(rows, "\n"))
			}
		})
	}
}

// USAGE scrolls a row at a time, as far as shows its last row, the key
// line's, over the row saying what's out of view; a page being the rows from
// the offset to the last shown, less those a page leaves off its foot so as
// not to end on a heading without the first row under it, and a row less
// where a heading kept a page on would take the place of the first row
// left out, so a page down passes over none.
func TestUSAGEScrollsARowAtATimeAPageBeingTheRowsShownAtItsOffset(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		by     By
		n      int
		rows   int
		scroll int
		want   Scrolling
	}{
		{name: "fitting, nowhere", width: 160, n: 10, rows: 25},
		{name: "the matrix at the top, its heading kept a page on", width: 160, n: 10, rows: 9, want: Scrolling{Most: 17, Page: 7}},
		{name: "the matrix, its heading kept here and a page on", width: 160, n: 10, rows: 9, scroll: 1, want: Scrolling{Most: 17, Page: 7}},
		{name: "the matrix a page from its foot, which a page on goes no further than", width: 160, n: 10, rows: 15, want: Scrolling{Most: 11, Page: 14}},
		{name: "the matrix scrolled past the foot, as the frame now has it", width: 160, n: 10, rows: 9, scroll: 30, want: Scrolling{Most: 17, Page: 8}},
		{name: "by account, no heading kept", width: 160, by: ByAccount, n: 6, rows: 17, scroll: 1, want: Scrolling{Most: 13, Page: 16}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By = tt.by
			if got := f.usageSection(manyRead(tt.n), now).scrolling(tt.rows, tt.scroll); got != tt.want {
				t.Errorf("in %d rows, scrolled %d down, %d accounts scroll %+v, want %+v", tt.rows, tt.scroll, tt.n, got, tt.want)
			}
		})
	}
}

// workAlone is the usage world's work alone: one account, its three
// windows.
func workAlone(d *status.Document) {
	d.Accounts, d.Best, d.Primary, d.Sessions = d.Accounts[:1], "work", "", 2
}

// usageWhole is USAGE's block of doc as f lays it out whole, every account
// in view, its key line after a blank row, as text.
func usageWhole(f Frame, doc status.Document) []string {
	b := f.overviewBlock(doc, now)
	c := growing(f.Width)
	b.key(c, b.draw(c, 0)+1)
	return textOf(c.rows(Look{}))
}

// rowsOut is what a line saying how many rows are out of view says of above
// and below, as USAGE says it: "▲ 3 more above · ▼ 9 more below".
func rowsOut(above, below int) string {
	var said []string
	if above > 0 {
		said = append(said, fmt.Sprintf("▲ %d more above", above))
	}
	if below > 0 {
		said = append(said, fmt.Sprintf("▼ %d more below", below))
	}
	return strings.Join(said, " · ")
}

// usageHeading matches a row of USAGE laid out whole that heads the rows
// under it: a window's heading, stacked; the windows' headings over the
// matrix; a block's name on a row of its own, by account; or the pool's rule.
// windowHeading matches the windows' alone.
var (
	usageHeading  = regexp.MustCompile(`^([A-Z0-9 -]+|\s+5-HOUR\s.*|\s?(\d+|Σ) \S+|─+)$`)
	windowHeading = regexp.MustCompile(`^([A-Z0-9 -]+|\s+5-HOUR\s.*)$`)
)

// keptOver is the heading of the window whose rows row at of whole is among,
// below the heading's own: the last window's heading over it, where no
// blank row between windows comes between them.
func keptOver(whole []string, at int) string {
	for j := at - 1; j >= 0; j-- {
		if windowHeading.MatchString(whole[j]) {
			return whole[j]
		}
	}
	return ""
}

// USAGE laid out whole, its key line among its rows, scrolls a row at a
// time in every layout, as far as shows its last row over the row saying
// what's out of view: each row of the page the block's, from the offset,
// but the first where a window's heading is kept there, its own window's;
// the row under them saying how many rows are out of view, above and below;
// no page ending on a heading without the first row under it; and paged
// through, every row of it shows.
func TestUSAGEScrollsWholeARowAtATimeInEveryLayout(t *testing.T) {
	tests := []struct {
		name  string
		width int
		by    By
		doc   status.Document
		rows  int
	}{
		{name: "one account's three windows, stacked under 150 columns", width: 80, doc: usageWorld(workAlone), rows: 11},
		{name: "three accounts' three windows, stacked", width: 120, doc: usageWorld(), rows: 14},
		{name: "ten accounts in the matrix", width: 160, doc: manyRead(10), rows: 7},
		{name: "by account under 110 columns, each block's name on a row of its own", width: 80, by: ByAccount, doc: usageWorld(), rows: 12},
		{name: "by account from 150 columns, ROUTING beside", width: 160, by: ByAccount, doc: usageWorld(), rows: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By = tt.by
			usage := f.usageSection(tt.doc, now)
			whole, page := usageWhole(f, tt.doc), tt.rows-1
			most := usage.scrolling(tt.rows, 0).Most
			if most != len(whole)-page {
				t.Fatalf("in %d rows, USAGE scrolls %d rows at most, want %d, as far as shows the last of its %d", tt.rows, most, len(whole)-page, len(whole))
			}
			paged := f.overviewBlock(tt.doc, now).views().paged(tt.rows)
			reached := make([]bool, len(whole))
			for scroll := 0; scroll <= most; scroll++ {
				f.Scroll = scroll
				rows, _ := usageBody(f, tt.doc, now, tt.rows)
				kept := 0
				if row := whole[scroll]; rows[0] != row && (row == "" || !strings.HasPrefix(rows[0], row)) {
					kept = 1
					if want := keptOver(whole, scroll); rows[0] != want {
						t.Errorf("scrolled %d, the first row reads %q, want the block's %q, or the heading of its window, %q", scroll, rows[0], whole[scroll], want)
					}
				}
				_, to := paged.shown(scroll)
				shown := to - scroll
				if says, want := rows[page], rowsOut(scroll, len(whole)-scroll-shown); strings.TrimSpace(says) != want {
					t.Errorf("scrolled %d, the last row reads %q, want %q", scroll, says, want)
				}
				for i := kept; i < shown; i++ {
					if row := whole[scroll+i]; !strings.HasPrefix(rows[i], row) {
						t.Errorf("scrolled %d, the page's row %d reads %q, want it to start %q, the block's row %d", scroll, i, rows[i], row, scroll+i)
					}
					reached[scroll+i] = true
				}
				end := scroll + shown - 1
				for end > scroll && whole[end] == "" {
					end--
				}
				if end > scroll && usageHeading.MatchString(whole[end]) {
					t.Errorf("scrolled %d, the page ends on %q, a heading without the first row under it", scroll, whole[end])
				}
			}
			for i, row := range whole {
				if !reached[i] {
					t.Errorf("paged through, USAGE's row %d never reads %q", i, row)
				}
			}
		})
	}
}

// The Overview scrolls a row at a time wherever its USAGE scrolls, a turn of
// the wheel scrolling several: as at 80×24 with one account of three windows.
func TestTheOverviewScrollsARowAtATime(t *testing.T) {
	f := frameOf(80, 24)
	if doc := usageWorld(workAlone); f.Scrolling(doc, now).Most == 0 || !f.ScrollsRows(doc, now) {
		t.Errorf("at 80×24, the Overview scrolls %+v, a row at a time: %t; want it to scroll, a row at a time", f.Scrolling(doc, now), f.ScrollsRows(doc, now))
	}
}

// Scrolled, USAGE's targets are the rows of each account's shown, where
// they're drawn; and ROUTING beside its blocks follows them, drawn on none of
// the rows past those shown. By account from 150 columns, the blocks are
// five rows, a blank row apart: scrolled 4 rows, work's last shows on the
// page's first row, personal's from its third, and side's first on the
// page's last, over the row saying what's out of view.
func TestUSAGEScrolledTargetsAndHasROUTINGBesideTheRowsItShows(t *testing.T) {
	f := frameOf(160, 60)
	f.By, f.Scroll = ByAccount, 4
	usage := f.usageSection(usageWorld(), now)
	body := bounds{y: 6, width: 160, rows: 10}
	want := []Target{{Y: 6, Width: 160, On: OnAccount, Account: "work"}}
	for y := 2; y < 7; y++ {
		want = append(want, Target{Y: 6 + y, Width: 160, On: OnAccount, Account: "personal"})
	}
	want = append(want, Target{Y: 6 + 8, Width: 160, On: OnAccount, Account: "side"})
	if got := usage.targets(body); !slices.Equal(got, want) {
		t.Errorf("scrolled 4 rows, USAGE's targets are\n%+v\nwant\n%+v", got, want)
	}
	rows, _ := usageBody(f, usageWorld(), now, 10)
	at := f.overviewBlock(usageWorld(), now).besideAt()
	beside := func(y int) string { return strings.TrimSpace(rows[y][min(len(rows[y]), at):]) }
	for _, y := range []int{2, 8} {
		if beside(y) == "" {
			t.Errorf("scrolled 4 rows, a block's first row reads %q, want ROUTING's state beside it", rows[y])
		}
	}
	if beside(9) != "" {
		t.Errorf("scrolled 4 rows, the row saying what's out of view reads %q, want nothing beside it", rows[9])
	}
}

// Scrolled, revealing an account scrolls nowhere while any of its rows
// shows, under a heading kept; else no further than shows its nearest row,
// below the page or above it. Stacked at 120 columns in 8 rows, seven show
// at once over the row saying what's out of view: the week's heading on
// row 0 and its accounts on 2, 4 and 6; the 5-hour's heading on 10, and its
// accounts on 12, 14 and 16; Fable week's on 20, and 22, 24 and 26.
func TestRevealingAnAccountInUSAGEScrollsNoFurtherThanShowsARowOfIt(t *testing.T) {
	usage := frameOf(120, 60).usageSection(usageWorld(), now)
	tests := []struct {
		name   string
		scroll int
		id     string
		want   int
	}{
		{name: "shown at the top: where it is", id: "work", want: 0},
		{name: "below the page: till it's the page's last", scroll: 9, id: "side", want: 10},
		{name: "a row of it the first under the heading kept: where it is", scroll: 11, id: "work", want: 11},
		{name: "a row of it under the heading kept: up a row, till it shows under it", scroll: 12, id: "work", want: 11},
		{name: "nearer below than above: down till it shows", scroll: 4, id: "work", want: 6},
		{name: "nearer above than below: up till it shows under the heading kept", scroll: 13, id: "work", want: 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usage.revealing(8, tt.scroll, tt.id); got != tt.want {
				t.Errorf("scrolled %d down, revealing %s scrolls to %d, want %d", tt.scroll, tt.id, got, tt.want)
			}
		})
	}
}

func TestEachAccountsRowsInUSAGEMoveTheFocusThere(t *testing.T) {
	rowsOf := func(id string, ys ...int) []Target {
		var targets []Target
		for _, y := range ys {
			targets = append(targets, Target{Y: 6 + y, On: OnAccount, Account: id})
		}
		return targets
	}
	tests := []struct {
		name   string
		width  int
		by     By
		doc    status.Document
		scroll int
		rows   int
		want   []Target
	}{
		{
			name: "the matrix: a row an account, none for the pool", width: 160, doc: threeRouted(), rows: 11,
			want: slices.Concat(rowsOf("work", 2), rowsOf("personal", 4), rowsOf("side", 6)),
		},
		{
			name: "stacked: a row an account in each window", width: 120, doc: threeRouted(), rows: 25,
			want: slices.Concat(rowsOf("work", 2), rowsOf("personal", 4), rowsOf("side", 6), rowsOf("work", 12), rowsOf("personal", 14), rowsOf("side", 16)),
		},
		{
			name: "by account: its block's rows, across ROUTING's beside them", width: 160, by: ByAccount, doc: threeRouted(), rows: 25,
			want: slices.Concat(rowsOf("work", 0, 1, 2), rowsOf("personal", 4, 5, 6), rowsOf("side", 8, 9, 10)),
		},
		{
			name: "scrolled: those shown alone, none under the heading kept", width: 160, doc: manyRead(10), scroll: 4, rows: 15,
			want: slices.Concat(rowsOf("a3", 2), rowsOf("a4", 4), rowsOf("a5", 6), rowsOf("a6", 8), rowsOf("a7", 10), rowsOf("a8", 12)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By, f.Scroll = tt.by, tt.scroll
			for i := range tt.want {
				tt.want[i].Width = tt.width
			}
			if got := f.usageSection(tt.doc, now).targets(bounds{y: 6, width: tt.width, rows: tt.rows}); !slices.Equal(got, tt.want) {
				t.Errorf("USAGE's targets are\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestUSAGEHasItsBigBarsWhereTheyFitByWindowFrom150Columns(t *testing.T) {
	tests := []struct {
		name  string
		width int
		by    By
		rows  int
		big   bool
	}{
		{name: "rows for the big bars", width: 160, rows: 21, big: true},
		{name: "a row short of them: the compact matrix", width: 160, rows: 20},
		{name: "under 150 columns: usage's stacked layout, no bigger", width: 149, rows: 40},
		{name: "by account: its blocks, no bigger", width: 160, by: ByAccount, rows: 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By = tt.by
			rows, _ := usageBody(f, threeRouted(), now, tt.rows)
			if big := rows[0] == "WEEK" && strings.HasPrefix(rows[2], " 1 work") && tt.width >= matrixWidth; big != tt.big {
				t.Errorf("in %d rows, USAGE's big bars show: %v, want %v; its first rows read %q", tt.rows, big, tt.big, rows[:3])
			}
		})
	}
	if got, want := frameOf(160, 60).usageSection(threeRouted(), now).most, 23; got != want {
		t.Errorf("USAGE's most is %d rows, want %d: its heading and the big bars", got, want)
	}
}

// On the Overview, as in the printout, USAGE's blocks are a blank row apart,
// by window and by account, at every width, as its key line is from the
// last; but two where GRAPHS' charts sit beside its blocks by account, each
// chart's axis on the first, beside the cells of USAGE's own; and where the
// charts stack beside each block, or with USAGE at its most, as many more as
// give each chart 2 rows, or 5, as under the last block before the key line.
func TestUSAGEsBlocksAreABlankRowApartButTwoBesideGRAPHSCharts(t *testing.T) {
	at := func(width int, by By) Frame {
		f := frameOf(width, 60)
		f.By = by
		return f
	}
	tests := []struct {
		name        string
		f           Frame
		least, most bool
		stacked     bool
		apart, key  int
	}{
		{name: "the big bars", f: at(160, ByWindow), apart: 1, key: 1},
		{name: "stacked at 120", f: at(120, ByWindow), apart: 1, key: 1},
		{name: "stacked at 80", f: at(80, ByWindow), apart: 1, key: 1},
		{name: "stacked on a phone", f: at(52, ByWindow), apart: 1, key: 1},
		{name: "by account, ROUTING beside", f: at(160, ByAccount), apart: 1, key: 1},
		{name: "by account at 120", f: at(120, ByAccount), apart: 1, key: 1},
		{name: "by account on a phone", f: at(52, ByAccount), apart: 1, key: 1},
		{name: "by account, GRAPHS' charts beside, at USAGE's least", f: besideOf(220), least: true, apart: 2, key: 1},
		{name: "by account, GRAPHS' charts beside, at USAGE's most", f: besideOf(220), most: true, apart: 3, key: 2},
		{name: "by account at 160, GRAPHS' charts beside, at USAGE's least, side by side as before", f: besideOf(160), least: true, apart: 2, key: 1},
		{name: "by account, GRAPHS' charts stacked beside", f: besideOf(160), stacked: true, apart: 8, key: 7},
		{name: "by account, GRAPHS' charts stacked beside, at USAGE's most", f: besideOf(160), most: true, apart: 17, key: 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			starts, cells := startsWindow, tt.f.Width
			if tt.f.By == ByAccount {
				starts = startsAccount
			}
			if tt.f.Width >= matrixWidth && tt.f.By == ByAccount {
				cells = tt.f.overviewBlock(usageWorld(), now).besideAt()
			}
			height, usage := 60, tt.f.usageSection(usageWorld(), now)
			switch {
			case tt.least:
				height = usage.least - sectionHead
			case tt.most:
				height = usage.most - sectionHead
			case tt.stacked:
				height = usage.between[0] - sectionHead
			}
			rows, _ := usageBody(tt.f, usageWorld(), now, height)
			blocks, key := gapsOver(rows, cells, starts)
			if want := slices.Repeat([]int{tt.apart}, len(blocks)); len(blocks) < 2 || !slices.Equal(blocks, want) || key != tt.key {
				t.Errorf("the blank rows over each block but the first are %v, and over the key line %d; want %d over each, and %d:\n%s", blocks, key, tt.apart, tt.key, strings.Join(rows, "\n"))
			}
		})
	}
}

// While GRAPHS shows, the matrix lights the focused account's whole name
// column, its number, name and marks, as wide as USAGE fits the column to the
// names, but the gap before the bars: 12 cells of the 14 the sample's names
// fit in, and 22 of the 24 at most a long name takes, cut to fit.
func TestTheFocusedAccountIsPickedOutInTheMatrixWhileGRAPHSShows(t *testing.T) {
	graphed := Sections{}.Toggled(SectionGraphs)
	tests := []struct {
		name   string
		shown  Sections
		by     By
		rows   int
		label  string
		picked int
	}{
		{name: "the matrix, GRAPHS shown", shown: graphed, rows: 11, picked: 12},
		{name: "the matrix, a name of 13 cells", shown: graphed, rows: 11, label: "personal-plus", picked: 17},
		{name: "the matrix, a name longer than the column holds", shown: graphed, rows: 11, label: "personal-and-much-longer", picked: 22},
		{name: "GRAPHS hidden", rows: 11},
		{name: "the big bars", shown: graphed, rows: 22},
		{name: "by account", shown: graphed, by: ByAccount, rows: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 60)
			f.Sections, f.By, f.Focus = tt.shown, tt.by, "personal"
			doc := threeRouted()
			doc.Accounts[1].Label = tt.label
			_, c := usageBody(f, doc, now, tt.rows)
			selected := hue{token: theme.BgSelection}
			for y := range c.height() {
				for x := range f.Width {
					want := y == 4 && x < tt.picked
					if got := c.at(x, y).ink.on == selected; got != want {
						t.Errorf("cell %d of row %d is on bg.selection: %v, want %v", x, y, got, want)
					}
				}
			}
		})
	}
}

// The marks' words take their long form where it fits the key line, else
// their short, as firstFit has them, never by the phone: ▲ new sessions go
// here on a phone of 73, and ▲ new at 52, where the cap's part leaves the
// long no room, but not where there's no cap.
func TestUSAGEsKeyLineNamesTheMarksTheAccountsCarry(t *testing.T) {
	pace := "┃ even pace: where you would be now, used evenly"
	noPrimary := func(d *status.Document) {
		for i := range d.Accounts {
			d.Accounts[i].Primary = false
		}
	}
	tests := []struct {
		name  string
		doc   status.Document
		width int
		want  []string
	}{
		{name: "from 150 columns, four cells apart after the cap's", doc: trio(), width: 160, want: []string{pace + "    ╎ cap: work at 95%    ◆ primary    ▲ new sessions go here"}},
		{name: "narrower, on the second line", doc: trio(), width: 120, want: []string{pace, "╎ cap: work at 95%    ◆ primary    ▲ new sessions go here"}},
		{name: "at 100, four cells apart", doc: trio(), width: 100, want: []string{pace, "╎ cap: work at 95%    ◆ primary    ▲ new sessions go here"}},
		{name: "at 99, three cells apart, the long form fitting", doc: trio(), width: 99, want: []string{pace, "╎ cap: work at 95%   ◆ primary   ▲ new sessions go here"}},
		{name: "at 73, on a phone, the long form fitting", doc: trio(), width: 73, want: []string{pace, "╎ cap: work at 95%   ◆ primary   ▲ new sessions go here"}},
		{name: "at 55, the long form just fitting", doc: trio(), width: 55, want: []string{pace, "╎ cap: work at 95%   ◆ primary   ▲ new sessions go here"}},
		{name: "at 54, a cell short of the long form: ▲ new", doc: trio(), width: 54, want: []string{pace, "╎ cap: work at 95%   ◆ primary   ▲ new"}},
		{name: "at 52, ▲ new", doc: trio(), width: 52, want: []string{pace, "╎ cap: work at 95%   ◆ primary   ▲ new"}},
		{name: "at 52 with no cap, the long form fitting", doc: trio(account("work", func(a *status.Account) { a.Reserve = 0 })), width: 52, want: []string{pace, "◆ primary   ▲ new sessions go here"}},
		{name: "no primary, no ◆", doc: trio(noPrimary), width: 120, want: []string{pace, "╎ cap: work at 95%    ▲ new sessions go here"}},
		{name: "no cap, the marks alone", doc: trio(account("work", func(a *status.Account) { a.Reserve = 0 })), width: 120, want: []string{pace, "◆ primary    ▲ new sessions go here"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			b := f.overviewBlock(tt.doc, printNow)
			c := growing(tt.width)
			b.key(c, 0)
			if got := textOf(c.rows(Look{})); !slices.Equal(trimmedLeft(got), tt.want) {
				t.Errorf("the key line reads %q, want %q", got, tt.want)
			}
			for _, marks := range keyMarks(namesOf(f.Moment(tt.doc, printNow))) {
				if best := marks[len(marks)-1]; best[0].ink != accentInk || best[1].ink != mutedInk {
					t.Errorf("▲'s part is drawn %+v, want ▲ in accent.mode and its words in text.muted", best)
				}
			}
		})
	}
}

// trimmedLeft is the rows given without the blanks at their starts.
func trimmedLeft(rows []string) []string {
	trimmed := make([]string, len(rows))
	for i, row := range rows {
		trimmed[i] = strings.TrimLeft(row, " ")
	}
	return trimmed
}

func TestOneAccountsUSAGESaysTheRoomLeftWhereThePoolWouldSayIt(t *testing.T) {
	one := func(reserve float64) status.Document {
		a := readAccount("work", sessionOf(0.6, 2*time.Hour), weekOf(0.4, 3*quota.Day))
		a.Reserve = reserve
		return routerDoc("work", 0, a)
	}
	tests := []struct {
		name  string
		doc   status.Document
		width int
		want  string
	}{
		{name: "one account, the week first", doc: one(0), width: 160, want: "room left  week 0.6 of 1  ·  5h 0.4 of 1"},
		{name: "to its cap, as the pool counts its room", doc: one(0.2), width: 160, want: "room left  week 0.4 of 1  ·  5h 0.2 of 1"},
		{name: "on a phone too", doc: one(0), width: 52, want: "room left  week 0.6 of 1  ·  5h 0.4 of 1"},
		{name: "several accounts, whose pool says it", doc: threeRouted(), width: 160},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := frameOf(tt.width, 60).usageSection(tt.doc, now).description()
			if got.plain() != tt.want {
				t.Errorf("USAGE's description reads %q, want %q", got.plain(), tt.want)
			}
		})
	}
	room := func(key string) ink { return strongInk.numbered(Number{Kind: NumberRoom, Account: "work", Of: key}) }
	inks := []ink{faintInk, dimInk, room("7d"), dimInk, faintInk, faintInk, dimInk, room("5h"), dimInk}
	got := frameOf(160, 60).usageSection(one(0), now).description()
	if len(got) != len(inks) {
		t.Fatalf("USAGE's description is %+v, want %d spans", got, len(inks))
	}
	for i, s := range got {
		if s.ink != inks[i] {
			t.Errorf("%q is drawn in %+v, want %+v", s.text, s.ink, inks[i])
		}
	}
}

// USAGE's heading says the room left wherever it has room for it, on a phone
// too: whole from 52 columns, its 40 cells between the name's three blanks
// and the rule's least and key; narrower, its windows give way whole from
// the last, never cut.
func TestOneAccountsUSAGEHeadingSaysTheRoomLeftWhereItHasRoom(t *testing.T) {
	a := readAccount("work", sessionOf(0.6, 2*time.Hour), weekOf(0.4, 3*quota.Day))
	doc := routerDoc("work", 0, a)
	tests := []struct {
		width int
		want  string
	}{
		{width: 160, want: "USAGE   room left  week 0.6 of 1  ·  5h 0.4 of 1 ─"},
		{width: 73, want: "USAGE   room left  week 0.6 of 1  ·  5h 0.4 of 1 ─"},
		{width: 52, want: "USAGE   room left  week 0.6 of 1  ·  5h 0.4 of 1 ─ u"},
		{width: 51, want: "USAGE   room left  week 0.6 of 1 ─"},
		{width: 36, want: "USAGE   room left  week 0.6 of 1 ─ u"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			f := frameOf(tt.width, 60)
			c := newCanvas(tt.width, 1)
			f.sectionHeading(c, planned{section: f.usageSection(doc, now), head: bounds{width: tt.width, rows: 1}})
			if got := textOf(c.rows(Look{}))[0]; !strings.HasPrefix(got, tt.want) {
				t.Errorf("USAGE's heading reads %q, want it to start %q", got, tt.want)
			}
		})
	}
}

func TestAcrossAResetBetweenReadsOneAccountsRoomLeftAndROUTINGAgree(t *testing.T) {
	// Work is at its cap in its 5-hour, read at now, which resets in an hour.
	a := readAccount("work", sessionOf(0.9, time.Hour), weekOf(0.4, 3*quota.Day))
	a.Reserve = 0.2
	doc := routerDoc("work", 0, a)
	reset := now.Add(time.Hour)
	f := frameOf(160, 60)
	tests := []struct {
		name   string
		now    time.Time
		capped bool
		want   string
	}{
		{name: "a moment before it resets", now: reset.Add(-time.Second), capped: true, want: "room left  week 0.4 of 1  ·  5h 0.0 of 1"},
		{name: "a moment after, before it's read again", now: reset.Add(time.Second), want: "room left  week 0.4 of 1  ·  5h 0.8 of 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if capped := f.routingTableOf(doc, tt.now).accounts[0].standing.Kind == status.RoutingCapped; capped != tt.capped {
				t.Errorf("ROUTING has work capped %t, want %t", capped, tt.capped)
			}
			if got := f.usageSection(doc, tt.now).description().plain(); got != tt.want {
				t.Errorf("USAGE's description reads %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAcrossAResetBetweenReadsThePoolsRoomAndROUTINGAgree(t *testing.T) {
	// Work is at its cap in its 5-hour, read at now, which resets in an hour;
	// side has half its 5-hour left.
	work := readAccount("work", sessionOf(0.9, time.Hour), weekOf(0.4, 3*quota.Day))
	work.Reserve = 0.2
	doc := routerDoc("side", 0, work, readAccount("side", sessionOf(0.5, 3*time.Hour), weekOf(0.3, 3*quota.Day)))
	reset := now.Add(time.Hour)
	f := frameOf(160, 60)
	tests := []struct {
		name   string
		now    time.Time
		capped bool
		room   string
	}{
		{name: "a moment before it resets", now: reset.Add(-time.Second), capped: true, room: "0.5 of 2 left"},
		{name: "a moment after, before it's read again", now: reset.Add(time.Second), room: "1.3 of 2 left"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if capped := f.routingTableOf(doc, tt.now).accounts[0].standing.Kind == status.RoutingCapped; capped != tt.capped {
				t.Errorf("ROUTING has work capped %t, want %t", capped, tt.capped)
			}
			rows, _ := usageBody(f, doc, tt.now, 20)
			fiveHour := slices.Index(rows, "5-HOUR")
			i := slices.IndexFunc(rows[max(fiveHour, 0):], func(r string) bool { return strings.Contains(r, "Σ pool") }) + fiveHour
			if fiveHour < 0 || i < fiveHour || !strings.Contains(rows[i], "  "+tt.room) {
				t.Errorf("USAGE reads\n%s\nwant the pool's 5-hour room %q", strings.Join(rows, "\n"), tt.room)
			}
		})
	}
}

func TestAcrossAResetBetweenReadsUSAGEAndROUTINGCountTheSamePool(t *testing.T) {
	// Pinned to work and side, so personal is out of the pool. Work is at
	// its cap in its 5-hour, read at now, which resets in an hour.
	work := readAccount("work", sessionOf(0.9, time.Hour), weekOf(0.4, 3*quota.Day))
	work.Reserve = 0.2
	doc := routerDoc("side", 0, work, readAccount("personal", sessionOf(0.2, 2*time.Hour), weekOf(0.5, 3*quota.Day)), readAccount("side", sessionOf(0.5, 3*time.Hour), weekOf(0.3, 3*quota.Day)))
	doc.Pin = status.Pin{Accounts: []string{"work", "side"}, Since: now.UTC()}
	doc = doc.WorkedOut(claudeLike)
	reset := now.Add(time.Hour)
	want := []string{"work", "side"}
	for _, at := range []time.Time{reset.Add(-time.Second), reset.Add(time.Second)} {
		f := frameOf(160, 60).Built(doc, at)
		var rows []string
		for _, r := range f.overviewBlock(doc, at).rows {
			if !r.pool && !r.gauges[0].bar.out {
				rows = append(rows, r.id)
			}
		}
		if !slices.Equal(rows, want) {
			t.Errorf("at %s, USAGE draws %q in the pool, want %q", at.Format(time.Kitchen), rows, want)
		}
		body, _ := usageBody(f, doc, at, 20)
		if i := slices.IndexFunc(body, func(r string) bool { return strings.Contains(r, "Σ pool") }); i < 0 || !strings.Contains(body[i], " of 2 left") {
			t.Errorf("at %s, USAGE reads\n%s\nwant its Σ row to count the pool's 2 accounts", at.Format(time.Kitchen), strings.Join(body, "\n"))
		}
		var routing []string
		for _, r := range f.routingTableOf(doc, at).pool {
			routing = append(routing, r.id)
		}
		if !slices.Equal(routing, want) {
			t.Errorf("at %s, ROUTING's pool block counts %q, want %q", at.Format(time.Kitchen), routing, want)
		}
	}
}

func TestAnAccountOutOfThePoolIsNamedFaintOnTheOverviewAlone(t *testing.T) {
	doc := trio(pinnedTo("side"))
	faintLabel := ink{token: theme.TextFaint, bold: true}
	tests := []struct {
		name                 string
		row                  usageRow
		number, label, marks ink
	}{
		{name: "on the Overview", row: frameOf(160, 60).overviewBlock(doc, printNow).rows[1], number: faintInk, label: faintLabel, marks: faintInk},
		{name: "in the printout, its number alone", row: printBlock(doc, 160, Look{}).rows[1], number: faintInk, label: titleInk, marks: primaryInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.row.name; len(got) != 3 || got[0].ink != tt.number || got[1].ink != tt.label || got[2].ink != tt.marks {
				t.Errorf("personal, out of the pool, is named %+v, want its number in %+v, its label in %+v and ◆ in %+v", got, tt.number, tt.label, tt.marks)
			}
		})
	}
}

func TestTheOverviewFadesAnAccountOutOfThePoolWhereThePrintoutDoesnt(t *testing.T) {
	doc := trio(pinnedTo("side"))
	tests := []struct {
		name  string
		block usageBlock
		out   bool
	}{
		{name: "on the Overview", block: frameOf(160, 60).overviewBlock(doc, printNow), out: true},
		{name: "in the printout", block: printBlock(doc, 160, Look{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, row := range tt.block.rows {
				for _, g := range row.gauges {
					faint := g.words.full != nil && g.words.full[0].ink == faintInk
					if out := row.id == "work" || row.id == "personal"; g.bar.out != (out && tt.out) || faint != (out && tt.out) {
						t.Errorf("row %d's window is faded %v, its words faint %v, want both %v", i, g.bar.out, faint, out && tt.out)
					}
				}
			}
		})
	}
}

func TestUSAGEsBarsFillAsFarAsTheyveEasedTheirPercentagesAsRead(t *testing.T) {
	f := frameOf(160, 60)
	f.Eased = map[Ref]float64{{Account: "work", Window: "7d"}: 0}
	rows, _ := usageBody(f, threeRouted(), now, 11)
	if got := colAt(rows[2], 14, 42); strings.TrimSpace(got) != "┃" {
		t.Errorf("work's week bar, easing from nothing, reads %q, want its track and even pace's mark alone", got)
	}
	if got := colAt(rows[2], 14+42+1, 4); got != " 34%" {
		t.Errorf("work's week percentage reads %q, want the reading's, 34%%", got)
	}
}

func TestAWindowsWordsAreCuedWhileTheirTurningToOutIs(t *testing.T) {
	cued := func(look Look) *canvas {
		f := frameOf(160, 60)
		f.Look = look
		f.TurnedOut = map[Ref]float64{{Account: "work", Window: "5h"}: 0.25, {Account: "side", Window: "5h"}: 0.5}
		f.Changed = map[string]float64{"personal": 0.5}
		_, c := usageBody(f, threeRouted(), now, 11)
		return c
	}
	tests := []struct {
		name     string
		x, y, to int
		cued     bool
	}{
		{name: "work's week, its allowance", x: 63, y: 2, to: 85},
		{name: "personal's week, out, but not turned out, its account's state changed", x: 63, y: 4, to: 77},
		{name: "work's 5-hour, turned out ~14:42", x: 137, y: 2, to: 147, cued: true},
		{name: "side's 5-hour, seen to turn out, but not out now", x: 137, y: 6, to: 153},
	}
	t.Run("on bg.attention", func(t *testing.T) {
		c, cue := cued(Screen(builtin(t, "nord"))), hue{token: theme.BgAttention, fade: 0.25}
		for _, tt := range tests {
			for x := tt.x; x < tt.to; x++ {
				if got := c.at(x, tt.y).ink.on; (got == cue) != tt.cued || (!tt.cued && got != (hue{})) {
					t.Errorf("%s: cell %d of row %d is on %+v, cued %v", tt.name, x, tt.y, got, tt.cued)
				}
				if c.at(x, tt.y).ink.bold {
					t.Errorf("%s: cell %d of row %d is bold, want it bold only where the look has no colour", tt.name, x, tt.y)
				}
			}
			if before := c.at(tt.x-1, tt.y).ink.on; before != (hue{}) {
				t.Errorf("%s: the cell before the words is on %+v, want the canvas", tt.name, before)
			}
		}
	})
	t.Run("bold where the look has no colour", func(t *testing.T) {
		c := cued(NoColour())
		for _, tt := range tests {
			for x := tt.x; x < tt.to; x++ {
				if got := c.at(x, tt.y).ink.bold; got != tt.cued {
					t.Errorf("%s: cell %d of row %d is bold %v, want %v", tt.name, x, tt.y, got, tt.cued)
				}
			}
		}
	})
}

func TestByAccountUSAGEsBarsLeaveROUTINGItsRoomBesideThem(t *testing.T) {
	long := func(d *status.Document) { d.Accounts[0].Label = "work-account-1" }
	tests := []struct {
		name                string
		width               int
		doc                 status.Document
		names, bar, routing int
	}{
		{name: "the sample's 16-cell name column: 48 cells, ROUTING from 107", width: 160, doc: trio(), names: 16, bar: 48, routing: 107},
		{name: "a wider name column takes its cells from the bars", width: 160, doc: trio(long), names: 20, bar: 44, routing: 107},
		{name: "under 150 columns, usage's own bars", width: 149, doc: trio(), names: 16, bar: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By = ByAccount
			b := f.overviewBlock(tt.doc, printNow)
			if b.names != tt.names || b.accountBar != tt.bar || (tt.bar > 0 && b.besideAt() != tt.routing) {
				t.Errorf("its name column %d, bars %d and ROUTING at %d; want %d, %d and %d", b.names, b.accountBar, b.besideAt(), tt.names, tt.bar, tt.routing)
			}
			if tt.bar > 0 {
				rows, _ := usageBody(f, tt.doc, printNow, 30)
				at, want := tt.names+windowLabelCells+tt.bar+1, " 34%  19%/day till Sun 21:00"
				if got := colAt(rows[0], at, len([]rune(want))); got != want {
					t.Errorf("work's week reads %q from column %d, want %q, its percentage a cell after its bar", got, at, want)
				}
			}
		})
	}
}

func TestByAccountUSAGEsWordsEndBeforeTheGapBeforeROUTINGBesideThem(t *testing.T) {
	stale := trio(account("work", func(a *status.Account) {
		a.Error, a.FetchedAt = "HTTP 529 · Overloaded", on(30, 12, 42).AddDate(0, -1, 0).UTC()
	}))
	f := frameOf(160, 60)
	f.By = ByAccount
	b := f.overviewBlock(stale, printNow)
	words, gap := b.names+windowLabelCells+b.accountBar+afterBar, b.besideAt()-besideCells
	rows, _ := usageBody(f, stale, printNow, 30)
	if got := strings.TrimRight(colAt(rows[0], words, gap-words), " "); got != "read Wed" {
		t.Errorf("work's stale 5-hour reads %q from column %d, want %q, short, as in full it'd run into the gap before ROUTING", got, words, "read Wed")
	}
	if got := colAt(rows[0], gap, besideCells); got != strings.Repeat(" ", besideCells) {
		t.Errorf("the gap before ROUTING reads %q from column %d, want it blank", got, gap)
	}
}

func TestUSAGELeavesROUTINGItsColumnsBesideItsBlocksWhileItShowsByAccountFrom150Columns(t *testing.T) {
	tests := []struct {
		name  string
		width int
		by    By
		shown Sections
		aside int
	}{
		{name: "by account from 150 columns: from hx", width: 160, by: ByAccount, aside: 107},
		{name: "by window", width: 160},
		{name: "by account under 150 columns", width: 149, by: ByAccount},
		{name: "by account, ROUTING hidden", width: 160, by: ByAccount, shown: Sections{}.Toggled(SectionRouting)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By, f.Sections = tt.by, tt.shown
			if got := f.usageSection(usageWorld(), routedAt).aside; got != tt.aside {
				t.Errorf("USAGE leaves ROUTING the columns from %d, want %d", got, tt.aside)
			}
		})
	}
}

func TestROUTINGBesideUSAGEsBlocksFollowsThemAsTheyScroll(t *testing.T) {
	doc := manyRead(10)
	for i := range doc.Accounts {
		doc.Accounts[i].Sessions = i + 1
	}
	f := frameOf(160, 60)
	f.By = ByAccount
	x := f.overviewBlock(doc, now).besideAt()
	// Blocks of three rows, a blank row apart, a1's from row 0, the pool's
	// from 40, scrolled a row at a time in 22 rows.
	tests := []struct {
		scroll int
		want   map[int][2]string
	}{
		{scroll: 2, want: map[int][2]string{2: {"2 a2", "2 sessions"}, 6: {"3 a3", "3 sessions"}, 18: {"6 a6", "6 sessions"}}},
		{scroll: 99, want: map[int][2]string{0: {"7 a7", "7 sessions"}, 12: {"10 a10", "10 sessions"}, 16: {"Σ pool", "55 sessions"}}},
	}
	for _, tt := range tests {
		f.Scroll = tt.scroll
		rows, _ := usageBody(f, doc, now, 22)
		beside := make([]string, len(rows))
		for i, row := range rows {
			beside[i] = strings.TrimRight(colAt(row, x, f.Width-x), " ")
		}
		for y, w := range tt.want {
			if !strings.HasPrefix(strings.TrimLeft(rows[y], " "), w[0]) || beside[y+1] != w[1] {
				t.Errorf("scrolled %d, row %d reads %q, %q beside the row under it; want it to start %q, %q beside", tt.scroll, y, rows[y], beside[y+1], w[0], w[1])
			}
		}
	}
}

func TestROUTINGBesideBlocksOfOneWindowLeavesTheirTracesOff(t *testing.T) {
	doc := routerDoc("a1", 0, readAccount("a1", sessionOf(0.1, time.Hour)), readAccount("a2", sessionOf(0.2, time.Hour)))
	f := frameOf(160, 40)
	f.By, f.Policy.Shared = ByAccount, []string{"5h"}
	x := f.overviewBlock(doc, now).besideAt()
	rows, _ := usageBody(f, doc, now, 10)
	want := map[int]string{0: "● open · new sessions come here", 1: "0 sessions", 2: "● open", 4: "2 open", 5: "0 sessions"}
	for y, w := range want {
		if got := strings.TrimRight(colAt(rows[y], x, f.Width-x), " "); got != w {
			t.Errorf("beside row %d reads %q, want %q", y, got, w)
		}
	}
	if rule := rows[3]; strings.Trim(rule, "─") != "" {
		t.Errorf("the pool's rule reads %q, want it whole, nothing beside the block above over it", rule)
	}
	if key := rows[7]; strings.Contains(key, "/min") {
		t.Errorf("the key line reads %q, want no trace over it", key)
	}
}

func TestTheOverviewShowsEachAccountsRowsUnderTheWindowsEveryModelSharesBeforeAnyIsRead(t *testing.T) {
	doc := unreadOf(threeRouted())
	names := map[By][]string{ByWindow: {"5-HOUR", "WEEK"}, ByAccount: {"5-hour", "Week"}}
	for _, by := range []By{ByWindow, ByAccount} {
		for _, width := range []int{160, 80} {
			t.Run(fmt.Sprintf("by %d at %d columns", by, width), func(t *testing.T) {
				f := frameOf(width, 60)
				f.By, f.WindowName = by, printName
				rows, _ := usageBody(f, doc, now, 40)
				text := strings.Join(rows, "\n")
				for _, want := range append([]string{"1 work", "2 personal", "3 side"}, names[by]...) {
					if !strings.Contains(text, want) {
						t.Errorf("USAGE has no %q:\n%s", want, text)
					}
				}
				if got := strings.Count(text, "—  not read"); got != 6 {
					t.Errorf("USAGE's gauges read \"not read\" %d times, want 6, each account's 5-hour window and week:\n%s", got, text)
				}
				var charts []string
				for _, ch := range f.graphsOf(doc, now).charts {
					charts = append(charts, ch.name)
				}
				if want := []string{"WEEK", "5-HOUR"}; !slices.Equal(charts, want) {
					t.Errorf("GRAPHS charts %q, want %q", charts, want)
				}
			})
		}
	}
}

func TestROUTINGBesideUSAGEDrawsNothingAndHasNoTargetsOfItsOwn(t *testing.T) {
	f := frameOf(160, 40)
	f.By = ByAccount
	s := f.routingSection(threeRouted(), now)
	body := bounds{x: 107, y: 6, width: 53, rows: 22}
	c := newCanvas(f.Width, f.Height)
	s.draw(c.within(body), body)
	if drawn := strings.Join(textOf(c.rows(Look{})), ""); drawn != "" {
		t.Errorf("ROUTING beside USAGE drew %q of its own, want nothing: USAGE draws it beside its blocks", drawn)
	}
	if targets := s.targets(body); targets != nil {
		t.Errorf("ROUTING beside USAGE has targets %+v of its own, want none: USAGE's rows span it", targets)
	}
}

// namedOn are the cells of the first row of c to name an account as given,
// its number, label and marks, from the first.
func namedOn(c *canvas, named string) []cell {
	for y, row := range textOf(c.rows(Look{})) {
		if before, _, found := strings.Cut(row, named); found {
			x := utf8.RuneCountInString(before)
			return c.cells[y][x : x+utf8.RuneCountInString(named)]
		}
	}
	return nil
}

func TestAnAccountOutOfThePoolIsNamedFaintWhereverTheOverviewNamesIt(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	doc.Pin = status.Pin{Accounts: []string{"side"}, Since: routedClock(1, 14, 2)}
	doc = doc.WorkedOut(claudeLike)
	f := Frame{Width: 160, Height: 40, View: Overview, Policy: claudeLike, Sessions: sessions, Stream: stream}
	sections := map[string]section{"ROUTING": f.routingSection(doc, routedAt), "SESSIONS": f.sessionsSection(doc, routedAt)}
	for name, s := range sections {
		t.Run(name, func(t *testing.T) {
			c := newCanvas(f.Width, 12)
			s.draw(c, bounds{width: f.Width, rows: 12})
			out, in := namedOn(c, "2 personal ◆"), namedOn(c, "3 side ▲")
			if out == nil || in == nil {
				t.Fatalf("%s names personal on %v and side on %v, want both named", name, out, in)
			}
			for i, here := range out {
				if here.ink.token != theme.TextFaint {
					t.Errorf("personal, out of the pool, has cell %d of its name in %s, want text.faint", i, here.ink.token)
				}
			}
			if label := in[2].ink.token; label == theme.TextFaint {
				t.Errorf("side, in the pool, is labelled in %s, want it as USAGE names it", label)
			}
		})
	}
}
