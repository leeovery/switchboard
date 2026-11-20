package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// detailWidths are the widths Detail's rules over a past period hold at:
// stacked at 52, 73 and 104, wide at 138, 160 and 173.
var detailWidths = []int{52, 73, 104, 138, 160, 173}

// backFrom is the period of the length given at now, stepped back as many
// times as given, as < steps it, weeks starting on Monday.
func backFrom(t *testing.T, length views.Length, now time.Time, back int) views.Period {
	t.Helper()
	period := length.Now()
	for range back {
		var ok bool
		if period, ok = period.Back(now, time.Monday, time.Time{}); !ok {
			t.Fatalf("%s steps back to nothing", length.Name())
		}
	}
	return period
}

// detailPeriod is a period Detail is drawn over at routedAt, Thursday 1
// October, and its page over it.
type detailPeriod struct {
	name   string
	period views.Period
	page   *views.AccountsPage
}

// detailPeriods are this week, at now; Tue 29 Sep, gone, its weeks ending at
// this week, still running; and last week, gone, its weeks ending before
// this one: each with its page as the views give it, as detailsPage and
// goneDetail have them.
func detailPeriods(t *testing.T) []detailPeriod {
	tue, lastWeek := backFrom(t, views.LengthToday, routedAt, 2), backFrom(t, views.LengthThisWeek, routedAt, 1)
	return []detailPeriod{
		{name: "this week", period: views.LengthThisWeek.Now(), page: worthBy(detailsPage(t), weekByDay([]float64{40, 10, 30, 20}))},
		{name: "Tue 29 Sep", period: tue, page: goneDetail(t, tue, true)},
		{name: "last week", period: lastWeek, page: goneDetail(t, lastWeek, false)},
	}
}

// goneDetail is Detail's page of its frames over the period gone given, as
// the views give it: its weeks named by Sunday 27 September, the last day of
// their last whole week; where they end at this week, as thisWeek says, as
// Tue 29 Sep's do, this week's days to come and its peak so far as now's;
// else every week whole and a week earlier, none to come; and its worth's
// buckets gone, none now's.
func goneDetail(t *testing.T, period views.Period, thisWeek bool) *views.AccountsPage {
	page := detailsPage(t)
	page.Period, page.ThisWeek, page.WholeTo = period, thisWeek, time.Date(2026, time.September, 27, 0, 0, 0, 0, time.UTC)
	gone := weekByDay([]float64{40, 10, 30, 20, 50, 5, 15})
	gone.Gone = true
	worthBy(page, gone)
	if thisWeek {
		return page
	}
	page.ToCome = 0
	for i, week := range page.Weeks {
		page.Weeks[i] = civilOf(t, week).AddDate(0, 0, -7).Format(time.DateOnly)
	}
	for _, row := range detailRows(page) {
		row.Peaks = slices.Clone(row.Peaks)
		row.Peaks[len(row.Peaks)-1].Open = false
	}
	return page
}

// worthBy is the page with each row's worth charted as given.
func worthBy(page *views.AccountsPage, by views.WorthBy) *views.AccountsPage {
	for _, row := range detailRows(page) {
		row.WorthBy = views.WorthCharts{by}
	}
	return page
}

// detailRows are the page's rows: each account's, then all accounts'.
func detailRows(page *views.AccountsPage) []*views.AccountsRow {
	rows := []*views.AccountsRow{&page.All}
	for i := range page.Rows {
		rows = append(rows, &page.Rows[i])
	}
	return rows
}

// civilOf is the day of the date given, as dayfile.Civil gives it.
func civilOf(t *testing.T, date string) time.Time {
	t.Helper()
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		t.Fatal(err)
	}
	return day
}

// pastDrawn is Detail as its form draws it whole: the frame and Detail of its
// moment, laid out as detailIn lays it; its canvas, wide its panel with the
// list beside it, stacked the stack; its rows as text; and whether it's
// wide.
type pastDrawn struct {
	f    Frame
	d    detailDrawn
	c    *canvas
	rows []string
	wide bool
}

// drawnOver is Detail of doc at routedAt, work chosen, width cells wide,
// over the period given, its dial as at routedAt, as pastDrawn has it, the
// frame changed as given.
func drawnOver(width int, dp detailPeriod, doc status.Document, change ...func(*Frame)) pastDrawn {
	f := detailFrame(Look{}, framedAccounts, "work", "", dp.page, nil)
	f.Width, f.Period, f.Dial = width, dp.period, views.DialOf(dp.period, routedAt, time.Monday, time.Time{})
	for _, ch := range change {
		ch(&f)
	}
	return detailDrawnAt(f, doc, routedAt)
}

// detailDrawnAt is Detail in the frame f of doc at now, as pastDrawn has it.
func detailDrawnAt(f Frame, doc status.Document, now time.Time) pastDrawn {
	l, wide := f.detailIn(f.Moment(doc, now))
	d := f.detailDrawn(doc, now, l)
	c := stacked{d}.whole().c
	if wide {
		c = d.panel()
		d.list(c, 0, wholeRows)
	}
	return pastDrawn{f: f, d: d, c: c, rows: textOf(c.rows(Look{})), wide: wide}
}

// head is the text of how Detail heads the account: wide, its head, from the
// panel's column; stacked, the row under the picker.
func (p pastDrawn) head() string {
	if !p.wide {
		return p.rows[1]
	}
	return strings.TrimSpace(cellsFrom(p.rows[0], p.d.l.panel))
}

// cellsFrom is the row from column x on.
func cellsFrom(row string, x int) string {
	runes := []rune(row)
	return string(runes[min(x, len(runes)):])
}

// cellsOf is the row's cells from column x to column end.
func cellsOf(row string, x, end int) string {
	runes := []rune(row)
	runes = append(runes, []rune(strings.Repeat(" ", max(end-len(runes), 0)))...)
	return string(runes[x:end])
}

// pastWorld is the world Detail is drawn of over a past period: the auto
// world, work's token's year up in 21 days, and what befell its accounts.
func pastWorld() status.Document {
	return usageWorld(extraOff, tokenLeft("work", 21), happened(onAuto()...))
}

// In a past period what tells the moment leaves Detail, wide and stacked:
// the state the head says, the token's line, the list's states' marks,
// their cells kept blank, and USAGE; THIS WEEK, titled by the dial, heads on
// BY MODEL's row, wide, and under the picker, stacked. Where its wide form
// starts holds as at now, so a step in time never turns it stacked or wide.
// At now each shows.
func TestAPastDetailsMomentLeaves(t *testing.T) {
	doc := pastWorld()
	periods := detailPeriods(t)
	for _, width := range detailWidths {
		wideAtNow := drawnOver(width, periods[0], doc).wide
		for _, dp := range periods {
			t.Run(fmt.Sprintf("%d %s", width, dp.name), func(t *testing.T) {
				p := drawnOver(width, dp, doc)
				gone, all := p.f.Dial.Gone(), strings.Join(p.rows, "\n")
				if p.wide != wideAtNow {
					t.Errorf("drawn wide %v, want %v, as at now", p.wide, wideAtNow)
				}
				for _, leaves := range []string{"USAGE", "token expires", "at its cap"} {
					if shows := rowWith(p.rows, leaves) >= 0; shows == gone {
						t.Errorf("%q shows: %v, want %v, in\n%s", leaves, shows, !gone, all)
					}
				}
				if named := p.head() == "work" || p.head() == ""; named != gone {
					t.Errorf("the head reads %q; the name alone: %v, want %v", p.head(), named, gone)
				}
				title := p.f.Dial.Title()
				headsWith := "USAGE"
				if gone {
					headsWith = "BY MODEL"
				}
				if y := rowWith(p.rows, title); y < 0 || p.wide && !strings.HasPrefix(cellsFrom(p.rows[y], p.d.l.panel), headsWith) {
					t.Errorf("%s heads on row %d, want %s's, in\n%s", title, y, headsWith, all)
				}
				if !p.wide {
					return
				}
				for i := range framedAccounts {
					y := (i + 1) * listApart
					if marked := p.c.at(p.d.l.dotAt(), y).glyph != " "; marked == gone {
						t.Errorf("the list's row %d reads %q; marked %v, want %v", y, p.rows[y], marked, !gone)
					}
				}
			})
		}
	}
}

// Wide, in a past period, THIS WEEK heads on BY MODEL's row at its column,
// ending at the weeks' edge, and BY MODEL's bar and its versions end a cell
// before it; the rule sits two rows under the lower of BY MODEL's last row
// and THIS WEEK's.
func TestAPastDetailsFiguresStandBesideBYMODEL(t *testing.T) {
	doc := pastWorld()
	for _, width := range detailWidths {
		for _, dp := range detailPeriods(t)[1:] {
			p := drawnOver(width, dp, doc)
			if !p.wide {
				continue
			}
			t.Run(fmt.Sprintf("%d %s", width, dp.name), func(t *testing.T) {
				l := p.d.l
				y, title := rowWith(p.rows, "BY MODEL"), p.f.Dial.Title()
				if got := columnOf(p.rows[y], title); got != l.thisWeekAt() {
					t.Errorf("%s is at column %d, want THIS WEEK's, %d, on BY MODEL's row %q", title, got, l.thisWeekAt(), p.rows[y])
				}
				end := l.thisWeekAt() - 1
				if bar := cellsOf(p.rows[y], l.cx(), end+1); bar != strings.Repeat("█", end-l.cx())+" " {
					t.Errorf("BY MODEL's bar reads %q, want it from cx to a cell before THIS WEEK", bar)
				}
				versions := lastRowWith(p.rows, y+1, l.cx(), end)
				if versions <= y {
					t.Fatalf("no versions under BY MODEL's bar in\n%s", strings.Join(p.rows, "\n"))
				}
				for row := y + 1; row <= versions; row++ {
					if gap := cellsOf(p.rows[row], end, end+1); gap != " " {
						t.Errorf("BY MODEL's versions on row %d run to THIS WEEK: %q", row, p.rows[row])
					}
				}
				figures := lastRowWith(p.rows, y, l.thisWeekAt(), l.edge())
				if got, want := rowWith(p.rows, "┄┄ "), max(versions, figures)+2; got != want {
					t.Errorf("the rule is on row %d, want two under BY MODEL's last, %d, and THIS WEEK's, %d", got, versions, figures)
				}
			})
		}
	}
}

// lastRowWith is the last row from row from on, each from it holding text
// between columns x and end, from - 1 where from holds none.
func lastRowWith(rows []string, from, x, end int) int {
	last := from - 1
	for y := from; y < len(rows) && strings.TrimSpace(cellsOf(rows[y], x, end)) != ""; y++ {
		last = y
	}
	return last
}

// A past period's weeks are said by where they end, wide and stacked, as the
// views word them: the rule, wide, "the last eight weeks" where they end at
// this week, a past day of it as Tue 29 Sep among them, else by the last day
// of the last, "the eight weeks to Sun 27 Sep"; and the facts, in every
// period gone, by the last day of the last whole week, "limit hit 0 of the 2
// weeks to Sun 27 Sep", in whichever form fits, as at now they're "of the
// last 2 weeks".
func TestAPastDetailsWeeksAreSaidByWhereTheyEnd(t *testing.T) {
	am := fewerWeeksMoment(t)
	tests := []struct {
		name   string
		period views.Period
		ruleTo bool
	}{
		{name: "this week", period: views.LengthThisWeek.Now()},
		{name: "Tue 29 Sep", period: backFrom(t, views.LengthToday, am.Now, 2)},
		{name: "last week", period: backFrom(t, views.LengthThisWeek, am.Now, 1), ruleTo: true},
	}
	for _, width := range detailWidths {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%d %s", width, tt.name), func(t *testing.T) {
				f := Frame{
					Width: width, Height: wholeRows, View: Accounts, Policy: claude.Policy(), Period: tt.period,
					Dial:     views.DialOf(tt.period, am.Now, am.WeekStarts, time.Time{}),
					Accounts: AccountsShown{Shown: AccountsDetail, Accounts: am.Accounts, Chosen: am.Accounts[0].ID, Page: momentPage(am, tt.period)},
				}
				p := detailDrawnAt(f, am.Doc, am.Now)
				gone := f.Dial.Gone()
				y := rowWith(p.rows, "limit hit")
				if y < 0 {
					t.Fatalf("no facts drawn in\n%s", strings.Join(p.rows, "\n"))
				}
				facts := strings.TrimSpace(p.rows[y])
				if named := strings.Contains(facts, " weeks to Sun 27 Sep"); named != gone || gone && strings.Contains(facts, "of the last") {
					t.Errorf("the facts read %q; named by Sun 27 Sep: %v, want %v", facts, named, gone)
				}
				if !p.wide {
					return
				}
				weeks := prose.Number(p.d.weeksShown())
				want := "┄┄ the last " + weeks + " weeks ┄"
				if tt.ruleTo {
					want = "┄┄ the " + weeks + " weeks to Sun 27 Sep ┄"
				}
				if rowWith(p.rows, want) < 0 {
					t.Errorf("no rule reading %q in\n%s", want, strings.Join(p.rows, "\n"))
				}
			})
		}
	}
}

// fewerWeeksMoment is Thursday 1 October, the ledger begun on Monday 14
// September, work's weeks and side's ended in two whole weeks alone.
func fewerWeeksMoment(t *testing.T) viewstest.AccountsMoment {
	for _, am := range viewstest.AccountsMoments() {
		if am.Name == "fewer than seven weeks recorded" {
			return am
		}
	}
	t.Fatal("no moment of fewer than seven weeks recorded")
	return viewstest.AccountsMoment{}
}

// BY DAY and WORTH mark nothing as now's in a past period, wide and stacked,
// but this week as now does where it shows: the last week's first day is
// bold where it's this week, as it is at now and at Tue 29 Sep, and its days
// to come, where the page has any, "··"; and WORTH's axis names no bucket
// bold in a period gone, as at now it names now's.
func TestAPastDetailMarksNothingAsNowsButThisWeekWhereItShows(t *testing.T) {
	doc := pastWorld()
	for _, width := range detailWidths {
		for _, dp := range detailPeriods(t) {
			t.Run(fmt.Sprintf("%d %s", width, dp.name), func(t *testing.T) {
				p := drawnOver(width, dp, doc)
				days := rowWith(p.rows, "BY DAY")
				last := prose.DayMonth(civilOf(t, dp.page.Weeks[len(dp.page.Weeks)-1]))
				x := columnOf(p.rows[days+1], last)
				if x < 0 {
					t.Fatalf("no %s under BY DAY in\n%s", last, strings.Join(p.rows, "\n"))
				}
				if bold := p.c.at(x, days+1).ink.bold; bold != dp.page.ThisWeek {
					t.Errorf("%s is bold %v, want %v", last, bold, dp.page.ThisWeek)
				}
				toCome := slices.ContainsFunc(cellsAlong(p.c, days), func(c *cell) bool { return c.glyph == "·" && c.ink == faintInk })
				if toCome != (dp.page.ToCome > 0) {
					t.Errorf("BY DAY reads %q; days to come shown %v, want %v", p.rows[days], toCome, dp.page.ToCome > 0)
				}
				axis := rowWith(p.rows, "peak ") - 1
				named := slices.ContainsFunc(cellsAlong(p.c, axis), func(c *cell) bool { return c.glyph != " " && c.ink.bold })
				if gone := p.f.Dial.Gone(); named == gone {
					t.Errorf("WORTH's axis %q names a bucket bold: %v, want %v", p.rows[axis], named, !gone)
				}
			})
		}
	}
}

// cellsAlong are the cells of row y of c.
func cellsAlong(c *canvas, y int) []*cell {
	cells := make([]*cell, c.width)
	for x := range cells {
		cells[x] = c.at(x, y)
	}
	return cells
}

// In a past period LATELY tells the events the watch holds up to the end of
// its last day, wide and stacked, in place of the document's, none picked
// out as fresh, as nothing in the past is, though an id is now's; once they're
// held and none is told, it says it has nothing lately, never how the router
// stands now; and while they're read, it says nothing, the heading saying
// loading…. At now it tells the document's.
func TestAPastDetailsLATELYTellsTheEventsHeld(t *testing.T) {
	doc := pastWorld()
	periods := detailPeriods(t)
	atNow, gone := periods[0], periods[2]
	held := views.Sentence{
		ID: 1, At: time.Date(2026, time.September, 27, 21, 15, 0, 0, time.UTC), Kind: status.EventRoom,
		Full: views.Said{Words: status.Words{status.Plain("open again: its week window reset")}},
	}
	f := detailFrame(Look{}, framedAccounts, "work", "", nil, nil)
	nows := views.AccountLately(f.Moment(doc, routedAt), "work", everyEvent, f.sessionDirs())[0]
	tests := []struct {
		name   string
		dp     detailPeriod
		lately []views.Sentence
		held   bool
		want   string
	}{
		{name: "at now, the document's", dp: atNow, want: nows.Full.String()},
		{name: "gone, the events held", dp: gone, lately: []views.Sentence{held}, held: true, want: "Sun 21:15  ✓ open again: its week window reset"},
		{name: "gone, none held to tell", dp: gone, lately: []views.Sentence{}, held: true, want: nothingLately.plain()},
		{name: "gone, the events read", dp: gone},
	}
	for _, width := range detailWidths {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%d %s", width, tt.name), func(t *testing.T) {
				p := drawnOver(width, tt.dp, doc, func(f *Frame) {
					f.Accounts.Lately, f.Accounts.LatelyHeld = tt.lately, tt.held
					f.Fresh = map[int]float64{held.ID: 0.5, nows.ID: 0.5}
				})
				y, x := rowWith(p.rows, "LATELY"), p.d.l.cx()
				if !p.wide {
					y, x = y+1, 0
				}
				told := ""
				if y < len(p.rows) {
					told = strings.TrimSpace(cellsFrom(p.rows[y], x))
				}
				if tt.want == "" && told != "" || !strings.Contains(told, tt.want) {
					t.Errorf("LATELY reads %q, want %q", told, tt.want)
				}
				if p.f.Dial.Gone() && strings.Contains(strings.Join(p.rows, "\n"), nows.Full.String()) {
					t.Errorf("LATELY tells now's %q in a past period", nows.Full.String())
				}
				if cued := p.c.at(x, y).ink.on.token == theme.BgAttention; cued == p.f.Dial.Gone() && told != "" {
					t.Errorf("LATELY's first line is cued %v, want %v", cued, !p.f.Dial.Gone())
				}
			})
		}
	}
}
