package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestUSAGEScrollsAnAccountsRowsAtATimeAPageBeingThoseBetweenItsScrollLines(t *testing.T) {
	tests := []struct {
		name  string
		width int
		by    By
		n     int
		rows  int
		want  Scrolling
	}{
		{name: "fitting, nowhere", width: 160, n: 10, rows: 25},
		{name: "the matrix", width: 160, n: 10, rows: 15, want: Scrolling{Most: 6, Page: 4}},
		{name: "stacked, under 150 columns", width: 120, n: 6, rows: 25, want: Scrolling{Most: 3, Page: 2}},
		{name: "by account", width: 160, by: ByAccount, n: 6, rows: 20, want: Scrolling{Most: 4, Page: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By = tt.by
			if got := f.overviewBlock(manyRead(tt.n), now).scrolling(tt.rows); got != tt.want {
				t.Errorf("in %d rows, %d accounts scroll %+v, want %+v", tt.rows, tt.n, got, tt.want)
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
		shown  Sections
		doc    status.Document
		scroll int
		rows   int
		across int
		want   []Target
	}{
		{
			name: "the matrix: a row an account, none for the pool", width: 160, doc: threeRouted(), rows: 11, across: 160,
			want: slices.Concat(rowsOf("work", 2), rowsOf("personal", 4), rowsOf("side", 6)),
		},
		{
			name: "stacked: a row an account in each window", width: 120, doc: threeRouted(), rows: 25, across: 120,
			want: slices.Concat(rowsOf("work", 2), rowsOf("personal", 4), rowsOf("side", 6), rowsOf("work", 13), rowsOf("personal", 15), rowsOf("side", 17)),
		},
		{
			name: "by account: its block's rows, to where ROUTING's start beside them", width: 160, by: ByAccount, doc: threeRouted(), rows: 25, across: 104,
			want: slices.Concat(rowsOf("work", 0, 1, 2), rowsOf("personal", 5, 6, 7), rowsOf("side", 10, 11, 12)),
		},
		{
			name: "by account, ROUTING hidden: across the frame", width: 160, by: ByAccount, shown: Sections{}.Toggled(SectionRouting), doc: threeRouted(), rows: 25, across: 160,
			want: slices.Concat(rowsOf("work", 0, 1, 2), rowsOf("personal", 5, 6, 7), rowsOf("side", 10, 11, 12)),
		},
		{
			name: "scrolled: those shown alone", width: 160, doc: manyRead(10), scroll: 3, rows: 15, across: 160,
			want: slices.Concat(rowsOf("a4", 3), rowsOf("a5", 5), rowsOf("a6", 7), rowsOf("a7", 9)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.By, f.Sections, f.Scroll = tt.by, tt.shown, tt.scroll
			for i := range tt.want {
				tt.want[i].Width = tt.across
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
	inks := []ink{faintInk, dimInk, strongInk, dimInk, faintInk, dimInk, strongInk, dimInk}
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
		{name: "in the printout, its number alone", row: newUsageBlock(doc, printNow, 160, Look{}, ByWindow, nil).rows[1], number: faintInk, label: titleInk, marks: primaryInk},
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
		{name: "in the printout", block: newUsageBlock(doc, printNow, 160, Look{}, ByWindow, nil)},
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

func TestAWindowsWordsTurningToOutAreCuedWhileItsAccountsChangeIs(t *testing.T) {
	f := frameOf(160, 60)
	f.Changed = map[string]float64{"work": 0.25, "side": 0.5}
	_, c := usageBody(f, threeRouted(), now, 11)
	cue := hue{token: theme.BgAttention, fade: 0.25}
	tests := []struct {
		name     string
		x, y, to int
		cued     bool
	}{
		{name: "work's 5-hour, out ~14:42", x: 63, y: 2, to: 73, cued: true},
		{name: "work's week, its allowance", x: 137, y: 2, to: 159},
		{name: "personal's week, out, its account unchanged", x: 137, y: 4, to: 151},
		{name: "side's 5-hour, changed, but not out", x: 63, y: 6, to: 79},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for x := tt.x; x < tt.to; x++ {
				if got := c.at(x, tt.y).ink.on; (got == cue) != tt.cued || (!tt.cued && got != (hue{})) {
					t.Errorf("cell %d of row %d is on %+v, cued %v", x, tt.y, got, tt.cued)
				}
			}
			if before := c.at(tt.x-1, tt.y).ink.on; before != (hue{}) {
				t.Errorf("the cell before the words is on %+v, want the canvas", before)
			}
		})
	}
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
