package dashboard

import (
	"fmt"
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

// usageBadHour is the bad hour's world: new sessions pinned to personal and
// side, side where they go; work's token refused, its last reading at 14:20;
// and personal at its 5-hour limit till its window resets at 15:54.
func usageBadHour(d *status.Document) {
	usagePinned("personal", "side")(d)
	d.Best = "side"
	account("work", func(a *status.Account) {
		a.FetchedAt, a.Refused = routedClock(1, 14, 20), status.Refusal{Until: routedAt.Add(time.Hour), Status: 401}
	})(d)
	account("personal", func(a *status.Account) {
		a.Windows[0] = quota.Window{Key: "5h", Label: "Session", Utilization: 1, Status: quota.StatusRejected, ResetsAt: routedClock(1, 15, 54)}
		a.Limit = status.Limit{Windows: []string{"5h"}, Until: routedClock(1, 15, 54)}
	})(d)
}

// usageDrawn is the cells of USAGE of doc as f draws its section's body in
// the rows given, from row top, on a canvas the frame's size, nothing else
// drawn on it.
func usageDrawn(t *testing.T, f Frame, doc status.Document, top, rows int) [][]seen {
	t.Helper()
	c := newCanvas(f.Width, f.Height)
	f.usageSection(doc, routedAt).draw(c, bounds{y: top, width: f.Width, rows: rows})
	return screenOf(t, c.rows(f.Look))
}

// The cells of USAGE's frames passed over, as the frames draw them otherwise
// than the design or than the world.
var (
	// evenPace160, evenPace216 and evenPaceByAccount are where the frames
	// place even pace's mark as if each window's pace were a whole percent,
	// where the design's rule, min(round(pace × bar), bar − 1), puts it a
	// cell on or back: work's week and Fable week at 16 and 48 cells, and,
	// at 170, its 5-hour and side's too.
	evenPace160       = []spot{{8, 72, 73}, {8, 120, 121}}
	evenPace216       = []spot{{8, 102, 103}, {12, 45, 46}, {19, 106, 107}, {30, 106, 107}}
	evenPaceByAccount = []spot{{8, 51, 52}, {10, 51, 52}}
	// sidesWeek is where the frames give side's week its allowance, 6%/day
	// till Mon 10:00, where it runs out ~Fri 12:20 at its average pace.
	sidesWeek = map[string]spot{
		"160": {12, 87, 107}, "216": {12, 125, 145}, "216 big": {23, 193, 213}, "by account": {22, 81, 101}, "phone": {24, 37, 48},
	}
	// personalsLimit is where the bad hour's frame has personal's 5-hour
	// reset at 16:30, as the auto world's, its even pace and the pool's a
	// cell back, where its world's resets at 15:54, as its limit lifts.
	personalsLimit = []spot{{10, 26, 28}, {14, 22, 23}}
)

func TestUSAGEIsDrawnAsItsFrames(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	graphed := Sections{}.Toggled(SectionGraphs).Toggled(SectionSessions)
	wide := func(top, rows, width int) []region {
		return []region{{top: top, bottom: top + rows - 1, right: width - 1}}
	}
	tests := []struct {
		stem          string
		width, height int
		doc           status.Document
		shown         Sections
		by            By
		top, rows     int
		in            []region
		whole         []spot
	}{
		{stem: "overview-default-160", width: 160, height: 37, doc: usageWorld(), top: 6, rows: 11, whole: slices.Concat(evenPace160, []spot{sidesWeek["160"]})},
		{stem: "overview-pinned-160", width: 160, height: 37, doc: usageWorld(usagePinned("personal")), top: 6, rows: 11, whole: slices.Concat(evenPace160, []spot{sidesWeek["160"]})},
		{stem: "overview-bad-hour-160", width: 160, height: 37, doc: usageWorld(usageBadHour), top: 6, rows: 11, whole: slices.Concat(evenPace160, []spot{sidesWeek["160"]}, personalsLimit)},
		{stem: "overview-everything-160x39", width: 160, height: 39, doc: usageWorld(), shown: graphed, top: 6, rows: 11, whole: slices.Concat(evenPace160, []spot{sidesWeek["160"]})},
		{stem: "overview-sessions-graphs-216", width: 216, height: 70, doc: usageWorld(), shown: graphed, top: 6, rows: 11, whole: []spot{sidesWeek["216"]}},
		{stem: "overview-default-216", width: 216, height: 70, doc: usageWorld(), top: 6, rows: 33, whole: slices.Concat(evenPace216, []spot{sidesWeek["216 big"]})},
		{stem: "overview-everything-216x106", width: 216, height: 106, doc: usageWorld(), shown: graphed, top: 6, rows: 33, whole: slices.Concat(evenPace216, []spot{sidesWeek["216 big"]})},
		{
			stem: "overview-by-account-160", width: 160, height: 42, doc: usageWorld(), by: ByAccount, top: 6, rows: 28,
			// USAGE's blocks end before ROUTING's beside them, but for the
			// pool's rule across both and the key line.
			in:    []region{{top: 6, bottom: 31, right: 103}, {top: 26, bottom: 26, right: 159}, {top: 32, bottom: 33, right: 159}},
			whole: slices.Concat(evenPaceByAccount, []spot{sidesWeek["by account"]}),
		},
		{stem: "overview-phone-52", width: 52, height: 66, doc: usageWorld(), top: 7, rows: 34, whole: []spot{sidesWeek["phone"]}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, Look: nord, View: Overview, Policy: claudeLike, Sections: tt.shown, By: tt.by, Focus: "work"}
			drawn := usageDrawn(t, f, tt.doc, tt.top, tt.rows)
			in := tt.in
			if in == nil {
				in = wide(tt.top, tt.rows, tt.width)
			}
			for _, r := range in {
				heldTo(t, tt.stem, reference(t, tt.stem), drawn, r, shadeOf(nord.Canvas()), tt.whole, nil)
			}
		})
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
		accounts[i] = readAccount(fmt.Sprintf("a%d", i+1), sessionOf(0.1, time.Hour), weekOf(0.2, 3*day))
	}
	return routerDoc("a1", 0, accounts...)
}

// startsAt fails the test where any of the rows given doesn't start as want
// has it, by its index.
func startsAt(t *testing.T, rows []string, want map[int]string) {
	t.Helper()
	for row, text := range want {
		if row >= len(rows) || !strings.HasPrefix(rows[row], text) {
			t.Errorf("row %d reads %q, want it to start %q", row, rows[min(row, len(rows)-1)], text)
		}
	}
}

func TestUSAGEScrollsItsAccountsWithinItsRowsItsHeadingsPoolAndKeyLineHeld(t *testing.T) {
	f := frameOf(160, 40)
	doc := manyRead(10)
	key := strings.Repeat(" ", 14) + "┃ even pace"
	tests := []struct {
		name   string
		scroll int
		want   map[int]string
	}{
		{
			name: "at the top, ▼ under the last row shown", scroll: 0,
			want: map[int]string{2: " 1 a1 ▲", 4: " 2 a2", 6: " 3 a3", 8: " 4 a4", 9: "▼ 6 more accounts", 10: "────", 11: " Σ pool", 13: key},
		},
		{
			name: "part way, ▲ over the first row shown", scroll: 3,
			want: map[int]string{2: "▲ 3 more accounts", 3: " 4 a4", 5: " 5 a5", 7: " 6 a6", 9: " 7 a7", 10: "▼ 3 more accounts", 11: "────", 12: " Σ pool", 14: key},
		},
		{
			name: "past the end, no further than shows the last", scroll: 99,
			want: map[int]string{2: "▲ 6 more accounts", 3: " 7 a7", 5: " 8 a8", 7: " 9 a9", 9: "10 a10", 10: "────", 11: " Σ pool", 13: key},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Scroll = tt.scroll
			rows, c := usageBody(f, doc, now, 15)
			if !strings.Contains(rows[0], "5-HOUR") || !strings.Contains(rows[0], "WEEK") || rows[1] != "" {
				t.Errorf("the first rows read %q, want the windows' headings, held, and a blank row", rows[:2])
			}
			startsAt(t, rows, tt.want)
			for y, row := range rows {
				if strings.HasPrefix(row, "▲") || strings.HasPrefix(row, "▼") {
					if k := c.at(0, y).ink; k != dimInk {
						t.Errorf("row %d's scroll line is drawn in %+v, want text.subtle", y, k)
					}
				}
			}
		})
	}
}

func TestUSAGEScrollsAnAccountsRowsAtATimeAPageBeingThoseShownAtItsOffset(t *testing.T) {
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
		{name: "the matrix", width: 160, n: 10, rows: 15, want: Scrolling{Most: 6, Page: 4}},
		{name: "the matrix at the top, no ▲ over it", width: 160, n: 10, rows: 14, want: Scrolling{Most: 6, Page: 4}},
		{name: "the matrix between ▲ and ▼", width: 160, n: 10, rows: 14, scroll: 1, want: Scrolling{Most: 6, Page: 3}},
		{name: "the matrix at the foot, no ▼ under it", width: 160, n: 10, rows: 14, scroll: 6, want: Scrolling{Most: 6, Page: 4}},
		{name: "the matrix scrolled past the foot, as the frame now has it", width: 160, n: 10, rows: 14, scroll: 9, want: Scrolling{Most: 6, Page: 4}},
		{name: "stacked, under 150 columns, at the top", width: 120, n: 6, rows: 25, want: Scrolling{Most: 3, Page: 3}},
		{name: "stacked, under 150 columns, between ▲ and ▼", width: 120, n: 6, rows: 25, scroll: 1, want: Scrolling{Most: 3, Page: 2}},
		{name: "by account", width: 160, by: ByAccount, n: 6, rows: 20, want: Scrolling{Most: 4, Page: 2}},
		{name: "by account at the top", width: 160, by: ByAccount, n: 6, rows: 21, want: Scrolling{Most: 3, Page: 3}},
		{name: "by account between ▲ and ▼", width: 160, by: ByAccount, n: 6, rows: 21, scroll: 1, want: Scrolling{Most: 3, Page: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By, f.Scroll = tt.by, tt.scroll
			if got := f.usageSection(manyRead(tt.n), now).scrolling(bounds{width: tt.width, rows: tt.rows}); got != tt.want {
				t.Errorf("in %d rows, scrolled %d down, %d accounts scroll %+v, want %+v", tt.rows, tt.scroll, tt.n, got, tt.want)
			}
		})
	}
}

func TestRevealingAnAccountInUSAGEScrollsOnlyWhereItIsntShownAndThenTillItIs(t *testing.T) {
	accounts := make([]status.Account, 8)
	for i := range accounts {
		accounts[i] = readAccount(fmt.Sprintf("a%d", i+1), sessionOf(0.1, time.Hour), weekOf(0.2, 3*day), windowOf("7d_opus", "Opus", 0.3, 3*day))
	}
	doc := routerDoc("a1", 0, accounts...)
	// Stacked, under 150 columns, three windows: at the top, with no ▲ over
	// each, a1 to a4 show; between ▲ and ▼, three; at the foot, from a5, four.
	f := frameOf(120, 54)
	if s := f.Scrolling(doc, now); s != (Scrolling{Most: 4, Page: 4}) {
		t.Fatalf("USAGE scrolls %+v, want 4 accounts at the top, as far as 4", s)
	}
	tests := []struct {
		name   string
		scroll int
		id     string
		want   int
	}{
		{name: "the fourth, shown at the top: where it is", id: "a4", want: 0},
		{name: "the fifth, below the top: till it shows, between ▲ and ▼", id: "a5", want: 2},
		{name: "the last: to the foot", id: "a8", want: 4},
		{name: "below the page between ▲ and ▼: till it's the last shown", scroll: 1, id: "a6", want: 3},
		{name: "above the page: till it's the first shown", scroll: 3, id: "a2", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Scroll = tt.scroll
			if got := f.Revealing(doc, now, tt.id); got != tt.want {
				t.Errorf("scrolled %d down, revealing %s scrolls to %d, want %d", tt.scroll, tt.id, got, tt.want)
			}
		})
	}
}

func TestStackedEachWindowScrollsAlike(t *testing.T) {
	f := frameOf(120, 60)
	f.Scroll = 1
	rows, _ := usageBody(f, manyRead(6), now, 25)
	for _, line := range []string{" ▲ 1 more account", " ▼ 3 more accounts"} {
		if first := slices.Index(rows, line); first < 0 || slices.Index(rows[first+1:], line) < 0 {
			t.Errorf("the windows read\n%s\nwant %q in each", strings.Join(rows, "\n"), line)
		}
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
			want: slices.Concat(rowsOf("work", 2), rowsOf("personal", 4), rowsOf("side", 6), rowsOf("work", 13), rowsOf("personal", 15), rowsOf("side", 17)),
		},
		{
			name: "by account: its block's rows, across ROUTING's beside them", width: 160, by: ByAccount, doc: threeRouted(), rows: 25,
			want: slices.Concat(rowsOf("work", 0, 1, 2), rowsOf("personal", 5, 6, 7), rowsOf("side", 10, 11, 12)),
		},
		{
			name: "scrolled: those shown alone", width: 160, doc: manyRead(10), scroll: 3, rows: 15,
			want: slices.Concat(rowsOf("a4", 3), rowsOf("a5", 5), rowsOf("a6", 7), rowsOf("a7", 9)),
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
		{name: "rows for the big bars", width: 160, rows: 22, big: true},
		{name: "a row short of them: the compact matrix", width: 160, rows: 21},
		{name: "under 150 columns: usage's stacked layout, no bigger", width: 149, rows: 40},
		{name: "by account: its blocks, no bigger", width: 160, by: ByAccount, rows: 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By = tt.by
			rows, _ := usageBody(f, threeRouted(), now, tt.rows)
			if big := rows[0] == "5-HOUR" && strings.HasPrefix(rows[2], " 1 work") && tt.width >= matrixWidth; big != tt.big {
				t.Errorf("in %d rows, USAGE's big bars show: %v, want %v; its first rows read %q", tt.rows, big, tt.big, rows[:3])
			}
		})
	}
	if got, want := frameOf(160, 60).usageSection(threeRouted(), now).most, 24; got != want {
		t.Errorf("USAGE's most is %d rows, want %d: its heading and the big bars", got, want)
	}
}

func TestTheFocusedAccountIsPickedOutInTheMatrixWhileGRAPHSShows(t *testing.T) {
	graphed := Sections{}.Toggled(SectionGraphs)
	tests := []struct {
		name   string
		shown  Sections
		by     By
		rows   int
		picked bool
	}{
		{name: "the matrix, GRAPHS shown", shown: graphed, rows: 11, picked: true},
		{name: "GRAPHS hidden", rows: 11},
		{name: "the big bars", shown: graphed, rows: 22},
		{name: "by account", shown: graphed, by: ByAccount, rows: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 60)
			f.Sections, f.By, f.Focus = tt.shown, tt.by, "personal"
			_, c := usageBody(f, threeRouted(), now, tt.rows)
			selected := hue{token: theme.BgSelection}
			for y := range c.height() {
				for x := range f.Width {
					want := tt.picked && y == 4 && x < focusCells
					if got := c.at(x, y).ink.on == selected; got != want {
						t.Errorf("cell %d of row %d is on bg.selection: %v, want %v", x, y, got, want)
					}
				}
			}
		})
	}
}

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
		{name: "on a phone, three cells apart, ▲ new", doc: trio(), width: 52, want: []string{pace, "╎ cap: work at 95%   ◆ primary   ▲ new"}},
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
			if marks := keyMarks(namesOf(tt.doc), false); len(marks) > 0 && (marks[len(marks)-1][0].ink != accentInk || marks[len(marks)-1][1].ink != mutedInk) {
				t.Errorf("▲'s part is drawn %+v, want ▲ in accent.mode and its words in text.muted", marks[len(marks)-1])
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
		a := readAccount("work", sessionOf(0.6, 2*time.Hour), weekOf(0.4, 3*day))
		a.Reserve = reserve
		return routerDoc("work", 0, a)
	}
	tests := []struct {
		name  string
		doc   status.Document
		width int
		want  string
	}{
		{name: "one account", doc: one(0), width: 160, want: "room left  5h 0.4 of 1  ·  week 0.6 of 1"},
		{name: "to its cap, as the pool counts its room", doc: one(0.2), width: 160, want: "room left  5h 0.2 of 1  ·  week 0.4 of 1"},
		{name: "on a phone, none", doc: one(0), width: 52},
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
	inks := []ink{faintInk, dimInk, room("5h"), dimInk, faintInk, dimInk, room("7d"), dimInk}
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

func TestAnAccountOutOfThePoolIsNamedFaintOnTheOverviewAlone(t *testing.T) {
	doc := trio(pinnedTo("side"))
	faintLabel := ink{token: theme.TextFaint, bold: true}
	tests := []struct {
		name                 string
		row                  usageRow
		number, label, marks ink
	}{
		{name: "on the Overview", row: frameOf(160, 60).overviewBlock(doc, printNow).rows[1], number: faintInk, label: faintLabel, marks: faintInk},
		{name: "in the printout, its number alone", row: newUsageBlock(doc, printNow, 160, Look{}, ByWindow, usageWindows(doc, nil, printPolicy.Shared, printName)).rows[1], number: faintInk, label: titleInk, marks: primaryInk},
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
		{name: "in the printout", block: newUsageBlock(doc, printNow, 160, Look{}, ByWindow, usageWindows(doc, nil, printPolicy.Shared, printName))},
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
	f.Eased = map[Ref]float64{{Account: "work", Window: "5h"}: 0}
	rows, _ := usageBody(f, threeRouted(), now, 11)
	if got := colAt(rows[2], 14, 42); strings.TrimSpace(got) != "┃" {
		t.Errorf("work's 5-hour bar, easing from nothing, reads %q, want its track and even pace's mark alone", got)
	}
	if got := colAt(rows[2], 14+42+1, 4); got != " 58%" {
		t.Errorf("work's 5-hour percentage reads %q, want the reading's, 58%%", got)
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
		{name: "work's 5-hour, turned out ~14:42", x: 63, y: 2, to: 73, cued: true},
		{name: "work's week, its allowance", x: 137, y: 2, to: 159},
		{name: "personal's week, out, but not turned out, its account's state changed", x: 137, y: 4, to: 151},
		{name: "side's 5-hour, seen to turn out, but not out now", x: 63, y: 6, to: 79},
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
			if b.names != tt.names || b.accountBar != tt.bar || (tt.bar > 0 && b.routingAt() != tt.routing) {
				t.Errorf("its name column %d, bars %d and ROUTING at %d; want %d, %d and %d", b.names, b.accountBar, b.routingAt(), tt.names, tt.bar, tt.routing)
			}
			if tt.bar > 0 {
				rows, _ := usageBody(f, tt.doc, printNow, 30)
				at, want := tt.names+windowLabelCells+tt.bar+1, " 95%  at cap · back 17:10"
				if got := colAt(rows[0], at, len([]rune(want))); got != want {
					t.Errorf("work's 5-hour reads %q from column %d, want %q, its percentage a cell after its bar", got, at, want)
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
	words, gap := b.names+windowLabelCells+b.accountBar+afterBar, b.routingAt()-besideCells
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

func TestROUTINGBesideUSAGEsBlocksIsDrawnAsTheOverviewsFrameIs(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	f := Frame{
		Width: 160, Height: 42, Look: nord, View: Overview, Policy: claudeLike, By: ByAccount,
		Sections: Sections{}.Toggled(SectionGraphs).Toggled(SectionSessions),
		Sessions: sessionsRunning(map[string]int{"personal": 2, "side": 4}),
	}
	frame, drawn := reference(t, "overview-by-account-160"), screenOf(t, f.Draw(usageWorld(), routedAt))
	// The traces and their rates, as the frame's traffic isn't the world's;
	// and work's state, which the frame draws whole to the edge, where the
	// design cuts its words by whole clauses to end two cells short of it.
	whole := append(spanning(107, 159, 10, 17, 24, 31), spot{row: 6, first: 132, last: 159})
	// Row 26 is the pool's rule, which USAGE's blocks draw.
	for _, in := range []region{{top: 4, bottom: 25, left: 107, right: 159}, {top: 27, bottom: 31, left: 107, right: 159}} {
		heldTo(t, "overview-by-account-160", frame, drawn, in, shadeOf(nord.Canvas()), whole, nil)
	}
}

func TestROUTINGBesideUSAGEsBlocksFollowsThemAsTheyScroll(t *testing.T) {
	doc := manyRead(10)
	for i := range doc.Accounts {
		doc.Accounts[i].Sessions = i + 1
	}
	f := frameOf(160, 60)
	f.By, f.Scroll = ByAccount, 2
	x := f.overviewBlock(doc, now).routingAt()
	rows, _ := usageBody(f, doc, now, 22)
	beside := make([]string, len(rows))
	for i, row := range rows {
		beside[i] = strings.TrimRight(colAt(row, x, f.Width-x), " ")
	}
	want := map[int][2]string{1: {"3 a3", "3 sessions"}, 6: {"4 a4", "4 sessions"}, 11: {"5 a5", "5 sessions"}, 17: {"Σ pool", "55 sessions"}}
	for y, w := range want {
		if !strings.HasPrefix(strings.TrimLeft(rows[y], " "), w[0]) || beside[y+1] != w[1] {
			t.Errorf("row %d reads %q, %q beside the row under it; want it to start %q, %q beside", y, rows[y], beside[y+1], w[0], w[1])
		}
	}
}

func TestROUTINGBesideBlocksOfOneWindowLeavesTheirTracesOff(t *testing.T) {
	doc := routerDoc("a1", 0, readAccount("a1", sessionOf(0.1, time.Hour)), readAccount("a2", sessionOf(0.2, time.Hour)))
	f := frameOf(160, 40)
	f.By, f.Policy.Shared = ByAccount, []string{"5h"}
	x := f.overviewBlock(doc, now).routingAt()
	rows, _ := usageBody(f, doc, now, 10)
	want := map[int]string{0: "● open · new sessions come here", 1: "0 sessions", 2: "", 3: "● open", 4: "0 sessions", 6: "2 open", 7: "0 sessions"}
	for y, w := range want {
		if got := strings.TrimRight(colAt(rows[y], x, f.Width-x), " "); got != w {
			t.Errorf("beside row %d reads %q, want %q", y, got, w)
		}
	}
	if rule := rows[5]; strings.Trim(rule, "─") != "" {
		t.Errorf("the pool's rule reads %q, want it whole, no trace over it", rule)
	}
	if key := rows[8]; strings.Contains(key, "/min") {
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
				if want := []string{"5-HOUR", "WEEK"}; !slices.Equal(charts, want) {
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
	doc, sessions, traffic := overviewWorld()
	doc.Pin = status.Pin{Accounts: []string{"side"}, Since: routedClock(1, 14, 2)}
	doc = doc.WorkedOut(claudeLike)
	f := Frame{Width: 160, Height: 40, View: Overview, Policy: claudeLike, Sessions: sessions, Traffic: traffic}
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
