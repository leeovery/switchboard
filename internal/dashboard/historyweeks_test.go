package dashboard

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// eightWeeks are the first days of the eight weeks the Weeks frames draw,
// to the week of Thu 1 Oct 2026.
var eightWeeks = []string{"2026-08-10", "2026-08-17", "2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28"}

// weeksRow is an account's row of Weeks, or every account's where id is "":
// its peaks, as percentages, the last this week's so far, and its verdict
// over the last 7 weeks, what it left at a reset on average, as a
// percentage, and how many weeks the rest would have been short without
// it, where that's 0 or more.
func weeksRow(id string, peaks []int, verdict string, limits, left, withoutIt int) views.WeeksRow {
	row := views.WeeksRow{Account: id, All: id == "", Verdict: verdict, Weeks: 7, Limits: limits}
	if left >= 0 {
		row.LeftAtReset = new(float64(left) / 100)
	}
	if withoutIt >= 0 {
		row.WithoutIt = new(withoutIt)
	}
	for i, p := range peaks {
		row.Peaks = append(row.Peaks, views.WeekPeak{Share: new(float64(p) / 100), Open: i == len(peaks)-1})
	}
	return row
}

// primary is the row as the primary account's.
func primary(row views.WeeksRow) views.WeeksRow {
	row.Primary = true
	return row
}

// used is the row as an account that used the share given of its weeks, as
// a percentage, on average.
func used(row views.WeeksRow, share int) views.WeeksRow {
	row.Used = new(float64(share) / 100)
	return row
}

// threeWeeks is Weeks' page of the auto world, as history-weeks-160 and
// history-weeks-columns-160 draw it: three is about right, work alone short
// on 3 of the last 7 weeks, short on 6 with one fewer and none with one
// more.
func threeWeeks() views.WeeksPage {
	return views.WeeksPage{
		Weeks: eightWeeks, Window: "7d", ShortEach: []string{"work"},
		Capacity: views.Capacity{Weeks: 7, Short: 3, WithOneFewer: new(6), Verdict: views.AboutRight},
		Accounts: []views.WeeksRow{
			weeksRow("work", []int{72, 88, 100, 94, 100, 81, 100, 34}, views.HitsItsLimit, 3, 9, 6),
			primary(weeksRow("personal", []int{64, 70, 58, 77, 69, 90, 85, 66}, views.EndsWithRoom, 0, 27, 5)),
			weeksRow("side", []int{22, 35, 41, 30, 52, 47, 60, 78}, views.PlentyToSpare, 0, 59, 3),
		},
		All: weeksRow("", []int{53, 64, 66, 67, 74, 73, 82, 59}, views.RoomToSpare, 3, 45, -1),
	}
}

// tenWeeks is Weeks' page of the world of ten accounts, as
// history-weeks-ten-160 draws it: ten is one more than you need, spare the
// one to drop.
func tenWeeks() views.WeeksPage {
	price := ledger.Picodollars(100_000_000_000_000)
	return views.WeeksPage{
		Weeks: eightWeeks, Window: "7d", ShortEach: []string{"work", "team"}, SameWeeks: true, NoneShortWithout: true, DropPrice: &price,
		Capacity: views.Capacity{Weeks: 7, Short: 4, WithOneFewer: new(4), WithTwoFewer: new(6), Drop: "spare", Verdict: views.OneMoreThanYouNeed},
		Accounts: []views.WeeksRow{
			weeksRow("work", []int{72, 88, 100, 94, 100, 81, 100, 34}, views.HitsItsLimit, 3, 9, 4),
			primary(weeksRow("personal", []int{64, 70, 58, 77, 69, 90, 85, 66}, views.EndsWithRoom, 0, 27, 4)),
			weeksRow("side", []int{22, 35, 41, 30, 52, 47, 60, 78}, views.PlentyToSpare, 0, 59, 4),
			weeksRow("client", []int{40, 52, 48, 61, 55, 58, 63, 47}, views.PlentyToSpare, 0, 46, 4),
			used(weeksRow("spare", []int{5, 8, 3, 10, 6, 4, 9, 5}, views.CouldGo, 0, 95, 4), 5),
			weeksRow("lab", []int{60, 72, 70, 66, 81, 75, 79, 58}, views.EndsWithRoom, 0, 28, 4),
			weeksRow("team", []int{80, 92, 100, 88, 100, 95, 100, 83}, views.HitsItsLimit, 3, 6, 4),
			weeksRow("extra", []int{15, 22, 18, 30, 25, 19, 28, 12}, views.BarelyUsed, 0, 80, 4),
			weeksRow("ops", []int{30, 35, 28, 41, 33, 38, 36, 29}, views.PlentyToSpare, 0, 66, 4),
			weeksRow("studio", []int{45, 50, 62, 48, 55, 58, 52, 41}, views.PlentyToSpare, 0, 47, 4),
		},
		All: weeksRow("", []int{43, 52, 53, 54, 58, 56, 61, 45}, views.RoomToSpare, 4, 50, -1),
	}
}

// weeksFrame is a frame showing Weeks' page, drawn in the shape given, of a
// terminal the size given, in the look given.
func weeksFrame(page views.WeeksPage, shape WeeksShape, width, height int, look Look) Frame {
	f := frameOf(width, height)
	f.View, f.Look = History, look
	f.History = HistoryPage{Shown: HistoryWeeks, WeeksShape: shape, Read: true, Weeks: &page}
	return f
}

// weeksText is f's view of Weeks drawn as text alone, a row each from the
// view's top, without the blanks at their ends.
func weeksText(f Frame) []string {
	return textOf(historyDrawn(f).rows(Look{}))[f.top():]
}

// shortOf is Weeks' page of the auto world as short as given: now, with one
// fewer and with one more, of 7 weeks.
func shortOf(short, withOneFewer, withOneMore int, verdict string) views.WeeksPage {
	p := threeWeeks()
	p.Capacity.Short, p.Capacity.WithOneFewer, p.Capacity.WithOneMore, p.Capacity.Verdict = short, new(withOneFewer), withOneMore, verdict
	if short == 0 {
		p.ShortEach = nil
	}
	return p
}

func TestWeeksHeadlineIsBoldInItsVerdictsColourItsCountAWordToTen(t *testing.T) {
	positive, attention := ink{token: theme.StatePositive, bold: true}, ink{token: theme.AccentAttention, bold: true}
	eleven := threeWeeks()
	for range 8 {
		eleven.Accounts = append(eleven.Accounts, weeksRow("more", slices.Repeat([]int{50}, 8), views.PlentyToSpare, 0, 50, 0))
	}
	tests := []struct {
		name string
		page views.WeeksPage
		want string
		ink  ink
	}{
		{name: "about right", page: threeWeeks(), want: "three is about right", ink: positive},
		{name: "one more than you need", page: tenWeeks(), want: "ten is one more than you need", ink: positive},
		{name: "one too few", page: shortOf(5, 6, 1, views.OneTooFew), want: "three is one too few", ink: attention},
		{name: "too few", page: shortOf(7, 7, 4, views.TooFew), want: "three is too few", ink: attention},
		{name: "eleven accounts", page: eleven, want: "11 is about right", ink: positive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := weeksFrame(tt.page, WeeksTable, 160, 37, Look{})
			c := historyDrawn(f)
			if got := weeksText(f)[0]; got != tt.want {
				t.Errorf("the headline reads %q, want %q", got, tt.want)
			}
			if got := c.at(0, f.top()).ink; got != tt.ink {
				t.Errorf("the headline is in %+v, want %+v", got, tt.ink)
			}
		})
	}
}

func TestWeeksVerdictSaysHowShortEachReplayIsTheWorstBold(t *testing.T) {
	tests := []struct {
		name string
		page views.WeeksPage
		// want are the verdict's second and third rows; worst the counts in
		// bold accent.attention, and muted those in text.muted.
		want         [2]string
		worst, muted []string
	}{
		{
			name: "too few: one more would still leave 4 of 7 short", page: shortOf(7, 7, 4, views.TooFew),
			want: [2]string{
				"as now: short on 7 of the last 7 weeks, each time work alone   ·   with one fewer: short on 7 of 7   ·   with one more: short on 4 of 7",
				"an estimate, from replaying these weeks with one account fewer or more",
			},
			worst: []string{"short on 7 of the last 7 weeks", "short on 7 of 7"}, muted: []string{"short on 4 of 7"},
		},
		{
			name: "the same weeks, and two fewer in one more's place", page: tenWeeks(),
			want: [2]string{
				"as now: short on 4 of the last 7 weeks, work and team each time   ·   with one fewer: short on 4 of 7, the same weeks   ·   with two fewer: short on 6 of 7",
				"spare is the one to drop: $100/mo, 5% used on average, and nothing ran short without it   an estimate, from replaying these weeks",
			},
			worst: []string{"short on 6 of 7"}, muted: []string{"short on 4 of the last 7 weeks", "short on 4 of 7, the same weeks"},
		},
		{
			name: "none short", page: shortOf(0, 0, 0, views.OneMoreThanYouNeed),
			want: [2]string{
				"as now: short on none of the last 7 weeks   ·   with one fewer: short on none   ·   with one more: short on none",
				"an estimate, from replaying these weeks with one account fewer or more",
			},
			muted: []string{"short on none of the last 7 weeks", "short on none"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := weeksFrame(tt.page, WeeksTable, 160, 37, Look{})
			if got := weeksText(f)[1:3]; got[0] != tt.want[0] || got[1] != tt.want[1] {
				t.Errorf("the verdict reads\n%q\n%q\nwant\n%q\n%q", got[0], got[1], tt.want[0], tt.want[1])
			}
			row := historyDrawn(f).cells[f.top()+1]
			for _, want := range []struct {
				counts []string
				ink    ink
			}{{tt.worst, alertInk}, {tt.muted, mutedInk}} {
				for _, count := range want.counts {
					if got := inkOf(row, count); got != want.ink {
						t.Errorf("%q is in %+v, want %+v", count, got, want.ink)
					}
				}
			}
		})
	}
}

// inkOf is the ink of the first cell of the first run of cells along row
// that reads text, the zero ink where none does.
func inkOf(row []cell, text string) ink {
	glyphs := make([]string, len(row))
	for i, c := range row {
		glyphs[i] = c.glyph
	}
	before, _, found := strings.Cut(strings.Join(glyphs, ""), text)
	if !found {
		return ink{}
	}
	return row[len([]rune(before))].ink
}

func TestWeeksWithOneAccountHasNoRowForEveryAccountAndSaysWhetherASecondWouldHaveClearedTheWeeks(t *testing.T) {
	tests := []struct {
		name               string
		short, withOneMore int
		want               string
	}{
		{name: "a second would clear them", short: 3, want: "as now: short on 3 of the last 7 weeks, each time work alone   ·   a second would have cleared them"},
		{name: "a second would leave some", short: 5, withOneMore: 2, want: "as now: short on 5 of the last 7 weeks, each time work alone   ·   with a second: short on 2 of 7"},
		{name: "none short", want: "as now: short on none of the last 7 weeks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := threeWeeks()
			page.Accounts, page.Capacity = page.Accounts[:1], views.Capacity{Weeks: 7, Short: tt.short, WithOneMore: tt.withOneMore, Verdict: views.AboutRight}
			if tt.short == 0 {
				page.ShortEach = nil
			}
			page.Accounts[0].WithoutIt = nil
			f := weeksFrame(page, WeeksTable, 160, 37, Look{})
			rows := weeksText(f)
			if rows[0] != "one is about right" || rows[1] != tt.want {
				t.Errorf("the verdict reads %q and %q, want one is about right, and %q", rows[0], rows[1], tt.want)
			}
			if rows[weeksAllRow] != "" || !strings.HasPrefix(rows[weeksFirstRow], "work ") || strings.Contains(rows[weeksFirstRow], "without it") {
				t.Errorf("the table reads\n%s\nwant no row for every account, and work's without what the rest would do without it", strings.Join(rows[:weeksFirstRow+1], "\n"))
			}
		})
	}
}

func TestWeeksSaysWhyTheOneToDropIsAsFarAsItsKnown(t *testing.T) {
	price := ledger.Picodollars(100_000_000_000_000)
	tests := []struct {
		name             string
		price            *ledger.Picodollars
		used             bool
		noneShortWithout bool
		want             string
	}{
		{name: "all of it", price: &price, used: true, noneShortWithout: true, want: "spare is the one to drop: $100/mo, 5% used on average, and nothing ran short without it"},
		{name: "on no plan", used: true, noneShortWithout: true, want: "spare is the one to drop: 5% used on average, and nothing ran short without it"},
		{name: "others short without it", price: &price, used: true, want: "spare is the one to drop: $100/mo, 5% used on average"},
		{name: "nothing but that", noneShortWithout: true, want: "spare is the one to drop: nothing ran short without it"},
		{name: "nothing known", want: "spare is the one to drop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := tenWeeks()
			page.DropPrice, page.NoneShortWithout = tt.price, tt.noneShortWithout
			if !tt.used {
				page.Accounts[4].Used = nil
			}
			if got := fitParts(200, (weeksDrawn{page: page}).dropped()...).inked(); !slices.Equal(got, line{{tt.want, mutedInk}}) {
				t.Errorf("the line naming the one to drop is %+v, want %q in text.muted", got, tt.want)
			}
		})
	}
}

func TestWeeksTablesVerdictsSayEachAccountsWeeks(t *testing.T) {
	positive := ink{token: theme.StatePositive, bold: true}
	plenty := weeksRow("side", nil, views.PlentyToSpare, 0, 59, 3)
	tests := []struct {
		name string
		row  views.WeeksRow
		many bool
		want line
	}{
		{name: "few accounts", row: plenty, want: line{{"plenty to spare most weeks", positive}, {"  59% left", dimInk}, {"  without it: short 3 of 7", dimInk}}},
		{name: "many accounts", row: plenty, many: true, want: line{{"plenty to spare most weeks", positive}, {"   59% left at a reset", dimInk}}},
		{
			name: "hits its limit", row: weeksRow("work", nil, views.HitsItsLimit, 3, 9, 0),
			want: line{{"hits its limit most weeks", exhaustedInk}, {"  9% left", dimInk}, {"  without it: short 0 of 7", dimInk}},
		},
		{name: "ends with room", row: weeksRow("lab", nil, views.EndsWithRoom, 0, 28, -1), many: true, want: line{{"ends most weeks with room", strongInk}, {"   28% left at a reset", dimInk}}},
		{name: "could go", row: used(weeksRow("spare", nil, views.CouldGo, 0, 95, 4), 5), many: true, want: line{{"could go: ", alertInk}, {"5% used, no limit ever", mutedInk}}},
		{name: "barely used", row: weeksRow("extra", nil, views.BarelyUsed, 0, 80, 4), want: line{{"barely used: ", alertInk}, {"80% left at a reset", mutedInk}}},
		{name: "no week known", row: weeksRow("new", nil, "", 0, -1, 4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := threeWeeks()
			if tt.many {
				page = tenWeeks()
			}
			if got := fitParts(200, (weeksDrawn{page: page}).tableVerdict(tt.row)...).inked(); !slices.Equal(got, tt.want.inked()) {
				t.Errorf("the verdict is %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWeeksPeaksAreColouredByLevelThisWeeksSoFarBold(t *testing.T) {
	peak := func(share float64, open bool) views.WeekPeak { return views.WeekPeak{Share: new(share), Open: open} }
	tests := []struct {
		name string
		peak views.WeekPeak
		want line
	}{
		{name: "under 70%", peak: peak(0.69, false), want: line{{"69%", ink{token: theme.VizRamp1}}}},
		{name: "from 70%", peak: peak(0.70, false), want: line{{"70%", ink{token: theme.VizRamp2}}}},
		{name: "from 90%, as shown", peak: peak(0.895, false), want: line{{"90%", ink{token: theme.VizRamp4}}}},
		{name: "at its limit", peak: peak(1, false), want: line{{"100%", exhaustedInk}}},
		{name: "not known", peak: views.WeekPeak{Open: true}, want: line{{"—", faintInk}}},
	}
	w := weeksDrawn{page: threeWeeks()}
	for _, tt := range tests {
		if got := fitParts(peakCells, w.peak(w.page.Accounts[0], tt.peak)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: the peak is %+v, want %+v", tt.name, got, tt.want)
		}
	}
	if so := w.peak(w.page.Accounts[0], peak(0.34, true)).said; len(so) != 1 || so[0].text != "34%…" || so[0].ink.token != theme.TextSecondary || !so[0].ink.bold {
		t.Errorf("this week's peak so far is %+v, want 34%%… in bold text.secondary", so)
	}
}

func TestWeeksThisWeeksPeaksBrightenAsTheyChange(t *testing.T) {
	want := map[Number]string{
		{Kind: NumberUsed, Account: "work", Of: "7d"}:     "34%…",
		{Kind: NumberUsed, Account: "personal", Of: "7d"}: "66%…",
		{Kind: NumberUsed, Account: "side", Of: "7d"}:     "78%…",
		{Kind: NumberUsed, Of: "7d"}:                      "59%…",
	}
	for _, shape := range []WeeksShape{WeeksTable, WeeksColumns} {
		if got := historyDrawn(weeksFrame(threeWeeks(), shape, 160, 37, Look{})).numbers(); !maps.Equal(got, want) {
			t.Errorf("shape %d draws the numbers %v, want this week's peaks so far alone, %v", shape, got, want)
		}
	}
}

func TestWeeksSaysThereArentEnoughWeeksTillAWholeOneIsJudged(t *testing.T) {
	f := weeksFrame(views.WeeksPage{Weeks: eightWeeks, Accounts: threeWeeks().Accounts}, WeeksTable, 160, 37, Look{})
	if rows := weeksText(f); rows[0] != "not enough weeks yet" || slices.ContainsFunc(rows[1:], func(r string) bool { return r != "" }) {
		t.Errorf("Weeks of no whole week reads\n%s\nwant not enough weeks yet alone", strings.Join(rows, "\n"))
	}
	if got := historyDrawn(f).at(0, f.top()).ink; got != dimInk {
		t.Errorf("not enough weeks yet is in %+v, want text.subtle", got)
	}
	f.History.Weeks = nil
	if rows := weeksText(f); slices.ContainsFunc(rows, func(r string) bool { return r != "" }) {
		t.Errorf("Weeks before its page is built reads\n%s\nwant nothing", strings.Join(rows, "\n"))
	}
}

func TestWeeksLegendKeysEachMarkAsItsDrawn(t *testing.T) {
	tests := []struct {
		shape WeeksShape
		want  string
		this  ink
	}{
		{shape: WeeksTable, want: "each week at its peak   ·   100% it hit its limit   ·   34%… this week, so far", this: strongInk},
		{shape: WeeksColumns, want: "short: a week some account hit its limit   ·   each week at its peak   ·   100% it hit its limit   ·   34%… this week, so far", this: secondaryInk},
	}
	for _, tt := range tests {
		f := weeksFrame(threeWeeks(), tt.shape, 160, 37, Look{})
		c := historyDrawn(f)
		foot := f.top() + f.viewRows() - 1
		if got := textOf(c.rows(Look{}))[foot]; got != tt.want {
			t.Errorf("shape %d: the legend on row %d reads %q, want %q", tt.shape, foot+1, got, tt.want)
		}
		if this, limit := inkOf(c.cells[foot], "34%…"), inkOf(c.cells[foot], "100%"); this != tt.this || limit != exhaustedInk {
			t.Errorf("shape %d: the legend's 34%%… is in %+v and 100%% in %+v, want %+v and %+v", tt.shape, this, limit, tt.this, exhaustedInk)
		}
	}
}

// manyWeeks is the auto world's page with n accounts, a1, a2 and on.
func manyWeeks(n int) views.WeeksPage {
	page := threeWeeks()
	page.Accounts = nil
	for i := range n {
		page.Accounts = append(page.Accounts, weeksRow("a"+strconv.Itoa(i+1), slices.Repeat([]int{50}, 8), views.PlentyToSpare, 0, 50, 0))
	}
	return page
}

func TestWeeksScrollsAccountsPastWhatFits(t *testing.T) {
	const scroll = "j/k or the wheel to scroll them"
	tests := []struct {
		name   string
		shape  WeeksShape
		scroll int
		want   Scrolling
		// first is the first account shown, on the view's row given, and
		// out what the row over the legend says is out of view, above and
		// below.
		first, at int
		out       string
	}{
		{name: "the table", want: Scrolling{Most: 6, Page: 19}, first: 1, at: weeksFirstRow, out: "▼ 6 more accounts below   ·   " + scroll},
		{
			name: "the table scrolled", scroll: 4, want: Scrolling{Most: 6, Page: 19}, first: 5, at: weeksFirstRow,
			out: "▲ 4 more accounts above   ·   ▼ 2 more accounts below   ·   " + scroll,
		},
		{
			name: "the table scrolled to its end, and past", scroll: 9, want: Scrolling{Most: 6, Page: 19}, first: 7, at: weeksFirstRow,
			out: "▲ 6 more accounts above   ·   " + scroll,
		},
		{
			name: "columns", shape: WeeksColumns, scroll: 1, want: Scrolling{Most: 22, Page: 3}, first: 2, at: blockRows,
			out: "▲ 1 more account above   ·   ▼ 21 more accounts below   ·   " + scroll,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := weeksFrame(manyWeeks(25), tt.shape, 160, 37, Look{})
			f.Scroll = tt.scroll
			if got := f.weeksScrolling(f.Scroll); got != tt.want {
				t.Errorf("Weeks scrolls %+v, want %+v", got, tt.want)
			}
			rows := weeksText(f)
			if first := "a" + strconv.Itoa(tt.first) + " "; !strings.HasPrefix(rows[tt.at], first) {
				t.Errorf("row %d reads %q, want account %s first", tt.at, rows[tt.at], first)
			}
			if got := rows[f.viewRows()-2]; got != tt.out {
				t.Errorf("the row over the legend reads %q, want %q", got, tt.out)
			}
		})
	}
	if s := weeksFrame(threeWeeks(), WeeksTable, 160, 37, Look{}).weeksScrolling(0); s != (Scrolling{Page: 19}) {
		t.Errorf("Weeks of three accounts scrolls %+v, want none, 19 to a page", s)
	}
}

// Shorter than History's frames, or where it would show none of its
// accounts, Weeks is drawn whole, every account, and scrolls by rows over its
// legend, which keeps the view's last row, as a phone's form has none: the
// row after the page, the one over the legend, or a phone's last, says how
// many rows are out of view, above and below, and no line counts accounts it
// doesn't show; and the last row shows scrolled to the end.
func TestWeeksScrollsWholeWhereItsFramesDontFit(t *testing.T) {
	tests := []struct {
		name          string
		page          views.WeeksPage
		shape         WeeksShape
		width, height int
		want          Scrolling
		// legend is how the view's last row starts, "" where it has none;
		// last how the last row over it starts, scrolled to the end; and
		// below and above what the row after the page says at the top and
		// at the end, "" where it says nothing.
		legend, last, below, above string
	}{
		{name: "the table, a row short of the frames", page: tenWeeks(), width: 160, height: 36, want: Scrolling{Page: 26}, legend: "each week", last: ""},
		{
			name: "the table at 160×20", page: tenWeeks(), width: 160, height: 20, want: Scrolling{Most: 8, Page: 10}, legend: "each week", last: "studio ",
			below: "▼ 8 more below   ·   j/k to scroll", above: "▲ 8 more above   ·   j/k to scroll",
		},
		{
			name: "columns at 160×30", page: threeWeeks(), shape: WeeksColumns, width: 160, height: 30, want: Scrolling{Most: 3, Page: 20}, legend: "short: ", last: "              10 Aug",
			below: "▼ 3 more below   ·   j/k to scroll", above: "▲ 3 more above   ·   j/k to scroll",
		},
		{
			name: "a phone at 52×24, the line on its last row", page: tenWeeks(), width: 52, height: 24, want: Scrolling{Most: 10, Page: 13}, last: "studio ",
			below: "▼ 10 more below   ·   j/k to scroll", above: "▲ 10 more above   ·   j/k to scroll",
		},
		{
			name: "a narrow phone as tall as the frames", page: tenWeeks(), width: 12, height: 37, want: Scrolling{Most: 22, Page: 26}, last: "studio",
			below: "▼ 22 more…", above: "▲ 22 more…",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := weeksFrame(tt.page, tt.shape, tt.width, tt.height, Look{})
			if got := f.weeksScrolling(f.Scroll); got != tt.want || !f.ScrollsRows(status.Document{}, now) {
				t.Errorf("it scrolls %+v, a row at a time: %t; want %+v, a row at a time", got, f.ScrollsRows(status.Document{}, now), tt.want)
			}
			for _, at := range []struct {
				scroll int
				says   string
			}{{0, tt.below}, {tt.want.Most, tt.above}} {
				f.Scroll = at.scroll
				view := weeksText(f)[:f.viewRows()]
				if foot := view[len(view)-1]; tt.legend != "" && !strings.HasPrefix(foot, tt.legend) {
					t.Errorf("scrolled %d, the view's last row reads %q, want it to start %q", at.scroll, foot, tt.legend)
				}
				if says := view[tt.want.Page]; says != at.says {
					t.Errorf("scrolled %d, the row after the page reads %q, want %q", at.scroll, says, at.says)
				}
				if slices.ContainsFunc(view, func(row string) bool { return strings.Contains(row, "more account") }) {
					t.Errorf("scrolled %d, the view reads\n%s\nwant no line counting accounts out of view", at.scroll, strings.Join(view, "\n"))
				}
				if at.scroll == tt.want.Most && !strings.HasPrefix(view[tt.want.Page-1], tt.last) {
					t.Errorf("scrolled to the end, the last row over the legend reads %q, want it to start %q", view[tt.want.Page-1], tt.last)
				}
			}
		})
	}
	if f := weeksFrame(tenWeeks(), WeeksTable, 160, 37, Look{}); f.ScrollsRows(status.Document{}, now) {
		t.Error("ten accounts as tall as the frames scroll a row at a time, want an account at a time")
	}
}

func TestWeeksColumnsFillFromTheBottomTheirTopCellInEighths(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		name string
		look Look
		peak views.WeekPeak
		// want are the column's rows, top first, as text, and under its
		// peak.
		want  [columnRows]string
		under string
	}{
		{name: "72%: two rows whole and an eighth", look: nord, peak: views.WeekPeak{Share: new(0.72)}, want: [columnRows]string{"▁▁▁▁▁"}, under: " 72%"},
		{name: "a look that can't blend fills in blocks", peak: views.WeekPeak{Share: new(0.72)}, want: [columnRows]string{"▁▁▁▁▁", "█████", "█████"}, under: " 72%"},
		{name: "filled to the whole percent under it", look: nord, peak: views.WeekPeak{Share: new(0.7267)}, want: [columnRows]string{"▂▂▂▂▂"}, under: " 73%"},
		{name: "over its limit", look: nord, peak: views.WeekPeak{Share: new(1.3)}, under: " 130%"},
		{name: "no use", look: nord, peak: views.WeekPeak{Share: new(0.0)}, want: [columnRows]string{2: "▁▁▁▁▁"}, under: "  0%"},
		{name: "not known", look: nord, under: "  —"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const at = 14
			c := newCanvas(at+weekColumnCells, columnRows+1)
			w := weeksDrawn{page: views.WeeksPage{Weeks: eightWeeks[:1]}, edge: at + weekColumnCells, laid: weeksLaid{weeks: []flex.Span{{At: at, Size: weekColumnCells}}}}
			w.block(c, tt.look, 0, views.WeeksRow{Peaks: []views.WeekPeak{tt.peak}})
			rows := textOf(c.rows(Look{}))
			for r, want := range tt.want {
				if got := strings.TrimPrefix(rows[r], strings.Repeat(" ", at)); got != want {
					t.Errorf("row %d of the column reads %q, want %q", r, got, want)
				}
			}
			if got := strings.TrimPrefix(rows[columnRows], strings.Repeat(" ", at)); got != tt.under {
				t.Errorf("under the column reads %q, want %q", got, tt.under)
			}
		})
	}
}

func TestWeeksOnAPhoneIsTheVerdictAndTheTablesWeeksThatFit(t *testing.T) {
	f := weeksFrame(threeWeeks(), WeeksColumns, 52, 35, Look{})
	want := []string{
		"three is about right",
		"as now: short on 3 of the last 7 weeks, each time",
		"work alone",
		"with one fewer: short on 6 of 7",
		"with one more: short on none",
		"an estimate, from replaying these weeks with one",
		"account fewer or more",
		"",
		"               7 Sep  14 Sep  21 Sep  28 Sep",
		"",
		"all accounts    74%     73%     82%    59%…",
		"",
		"work           100%     81%    100%    34%…",
		"personal ◆      69%     90%     85%    66%…",
		"side            52%     47%     60%    78%…",
	}
	rows := weeksText(f)
	if !slices.Equal(rows[:len(want)], want) || slices.ContainsFunc(rows[len(want):], func(r string) bool { return r != "" }) {
		t.Errorf("Weeks on a phone reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
}

// Narrower than the wide form's natural, Weeks' oldest weeks give way so
// each verdict reads whole, a cell after the last week shown; what else is
// beside the weeks, and the capacity verdict's rows, are cut at the edge.
func TestWeeksGivesAWeekWayBeforeItCutsAVerdict(t *testing.T) {
	f := weeksFrame(threeWeeks(), WeeksTable, 120, 37, Look{})
	rows := weeksText(f)
	if got, want := rows[1], "as now: short on 3 of the last 7 weeks, each time work alone   ·   with one fewer: short on 6 of 7   ·   with one more…"; got != want {
		t.Errorf("the verdict's second row reads %q, want %q", got, want)
	}
	if got, want := rows[weeksFirstRow], "work            94%    100%     81%    100%    34%…  hits its limit most weeks  9% left  without it: short 6 of 7"; got != want {
		t.Errorf("work's row reads %q, want %q", got, want)
	}
}

// Where one week leaves the verdict short of its natural, it's cut at the
// edge, never to less than its word; and on a phone, with no verdict beside
// them, the weeks that fit show: seven at 73 columns, all eight from 77.
func TestWeeksShowsTheWeeksThatFit(t *testing.T) {
	tests := []struct {
		name   string
		page   views.WeeksPage
		width  int
		phone  bool
		weeks  int
		reads  string
		labels int
	}{
		{name: "the wide form where it fits, one week, the verdict cut", page: threeWeeks(), width: 79, weeks: 1, labels: 14, reads: "work           34%…  hits its limit most weeks  9% left  without it: short 6…"},
		{name: "a phone a cell narrower, all eight weeks", page: threeWeeks(), width: 78, phone: true, weeks: 8, labels: 14},
		{name: "a phone at 77, all eight", page: threeWeeks(), width: 77, phone: true, weeks: 8, labels: 14},
		{name: "a phone at 76, seven", page: threeWeeks(), width: 76, phone: true, weeks: 7, labels: 14},
		{name: "a phone at 73, seven", page: threeWeeks(), width: 73, phone: true, weeks: 7, labels: 14},
		{name: "a phone at 73, long names whole, six", page: longNamedWeeks(), width: 73, phone: true, weeks: 6, labels: labelMost},
		{name: "a phone at 52, four", page: threeWeeks(), width: 52, phone: true, weeks: 4, labels: 14},
		{name: "a phone at 27, one, the label column at its natural", page: threeWeeks(), width: 27, phone: true, weeks: 1, labels: 14},
		{name: "a phone at 20, one, the label column narrowed", page: threeWeeks(), width: 20, phone: true, weeks: 1, labels: 13},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := weeksFrame(tt.page, WeeksTable, tt.width, 61, Look{})
			w := f.weeksDrawn(tt.page)
			if w.phone != tt.phone || len(w.laid.weeks) != tt.weeks || w.laid.labels.Size != tt.labels {
				t.Errorf("Weeks is drawn on a phone: %t, %d weeks, its labels %d cells; want %t, %d and %d",
					w.phone, len(w.laid.weeks), w.laid.labels.Size, tt.phone, tt.weeks, tt.labels)
			}
			if tt.reads != "" && !slices.Contains(weeksText(f), tt.reads) {
				t.Errorf("Weeks reads\n%s\nwant a row %q", strings.Join(weeksText(f), "\n"), tt.reads)
			}
		})
	}
}

// longNamedWeeks is the auto world's page, its accounts' labels long, the
// widest the label column's most.
func longNamedWeeks() views.WeeksPage {
	page := threeWeeks()
	page.Accounts = slices.Clone(page.Accounts)
	for i, label := range []string{"work-team-plan", "personal-research", "side-hustle-consulting"} {
		page.Accounts[i].Label = label
	}
	return page
}

// weeksForm names the form Weeks is drawn in: its phone form, or wide, in
// columns or as a table.
func weeksFormOf(w weeksDrawn) string {
	switch {
	case w.phone:
		return "phone"
	case w.columns:
		return "columns"
	}
	return "table"
}

// weeksSweep is Weeks drawn of each page, in each shape, at each width from
// 40 to 220: the page's name, the shape, and Weeks as each width draws it,
// the narrowest first.
func weeksSweep() map[string][]weeksDrawn {
	pages := map[string]views.WeeksPage{"three": threeWeeks(), "ten": tenWeeks(), "long names": longNamedWeeks()}
	sweep := map[string][]weeksDrawn{}
	for name, page := range pages {
		for shape, shaped := range map[WeeksShape]string{WeeksTable: "as a table", WeeksColumns: "in columns"} {
			for width := 40; width <= 220; width++ {
				key := name + " " + shaped
				sweep[key] = append(sweep[key], weeksFrame(page, shape, width, 61, Look{}).weeksDrawn(page))
			}
		}
	}
	return sweep
}

// As the width grows, Weeks' phone form gives way to its wide form once, at
// the same width whatever its page, as the form never hangs on the data:
// where its parts' leasts fit and its legend reads whole on a row; and its
// columns, chosen, take their own wide form once more, past their wider
// legend.
func TestWeeksFormSwitchesOnceAsTheWidthGrows(t *testing.T) {
	switches := map[string]int{}
	for name, drawn := range weeksSweep() {
		var forms []string
		for i, w := range drawn {
			form := weeksFormOf(w)
			if i == 0 || form != forms[len(forms)-1] {
				forms = append(forms, form)
			}
			if i > 0 && drawn[i-1].phone && !w.phone {
				switches[strings.SplitN(name, " ", 2)[0]+" "+form] = w.edge + margin
			}
		}
		want := []string{"phone", "table"}
		if strings.HasSuffix(name, "in columns") {
			want = append(want, "columns")
		}
		if !slices.Equal(forms, want) {
			t.Errorf("%s, from 40 columns to 220, takes the forms %v, want %v", name, forms, want)
		}
	}
	if widths := slices.Sorted(maps.Values(switches)); widths[0] != widths[len(widths)-1] {
		t.Errorf("the wide form starts at %v columns, by page, want one width for every page", switches)
	}
}

// Within each form, a wider frame never shows fewer weeks, nor a shorter
// verdict, nor a narrower label column; wherever a week could give way, the
// verdict is at its natural, the widest it's worded, whole; and wherever a
// week could give way, or the verdict narrow, the label column is at its
// natural, so names come before the weeks and the verdict alike.
func TestWeeksWiderNeverShowsLess(t *testing.T) {
	for name, drawn := range weeksSweep() {
		for i, w := range drawn {
			natural := min(labelColumn(w.widestName()+nameGap).Natural, labelMost)
			narrowing := !w.phone && w.laid.verdict.Size > verdictPart(w.columns).Least
			if (len(w.laid.weeks) > 1 || narrowing) && w.laid.labels.Size != natural {
				t.Errorf("%s at %d columns: %d weeks show, the verdict %d cells, its label column %d, want its natural %d",
					name, w.edge+margin, len(w.laid.weeks), w.laid.verdict.Size, w.laid.labels.Size, natural)
			}
			if !w.phone && len(w.laid.weeks) > 1 && w.laid.verdict.Size != widestVerdict(w.columns) {
				t.Errorf("%s at %d columns: %d weeks show, the verdict %d cells, want its natural %d", name, w.edge+margin, len(w.laid.weeks), w.laid.verdict.Size, widestVerdict(w.columns))
			}
			if i == 0 || weeksFormOf(drawn[i-1]) != weeksFormOf(w) {
				continue
			}
			was := drawn[i-1].laid
			if len(w.laid.weeks) < len(was.weeks) || w.laid.verdict.Size < was.verdict.Size || w.laid.labels.Size < was.labels.Size {
				t.Errorf("%s at %d columns shows %d weeks, its verdict %d cells and labels %d, fewer than a cell narrower's %d, %d and %d",
					name, w.edge+margin, len(w.laid.weeks), w.laid.verdict.Size, w.laid.labels.Size, len(was.weeks), was.verdict.Size, was.labels.Size)
			}
		}
	}
}

// An account's verdict reads whole wherever a week can give way, every
// account's, in the table and in columns, its names whole too.
func TestWeeksVerdictsReadWholeWhereAWeekCanGiveWay(t *testing.T) {
	for _, page := range []views.WeeksPage{threeWeeks(), longNamedWeeks()} {
		for _, shape := range []WeeksShape{WeeksTable, WeeksColumns} {
			for width := 40; width <= 220; width++ {
				f := weeksFrame(page, shape, width, 61, Look{})
				w := f.weeksDrawn(page)
				if w.phone || len(w.laid.weeks) < 2 {
					continue
				}
				text := strings.Join(weeksText(f), "\n")
				for _, row := range page.Accounts {
					said := [][]part{w.tableVerdict(row)}
					if w.columns {
						said = w.accountAside(row)
					}
					plains := []string{historyAccount(w.named(row.Account), row.Primary, math.MaxInt).plain()}
					for _, l := range said {
						plains = append(plains, fitParts(math.MaxInt, l...).plain())
					}
					for _, plain := range plains {
						if !strings.Contains(text, plain) {
							t.Errorf("shape %d at %d columns: Weeks reads\n%s\nwant %q whole", shape, width, text, plain)
						}
					}
				}
			}
		}
	}
}

// Narrower than a phone, Weeks' table shows the latest of its four weeks
// that fit short of the edge, this week's among them, its peak whole and
// numbered.
func TestWeeksOnANarrowPhoneShowsTheLatestWeeksThatFit(t *testing.T) {
	f := weeksFrame(threeWeeks(), WeeksTable, 40, 40, NoColour())
	rows := textOf(historyDrawn(f).rows(Look{}))
	if want := "              14 Sep  21 Sep  28 Sep"; !slices.Contains(rows, want) {
		t.Errorf("at 40 columns, Weeks reads\n%s\nwant its heads %q", strings.Join(rows, "\n"), want)
	}
	if got := f.Numbers(threeRouted(), now)[Number{Kind: NumberUsed, Account: "work", Of: "7d"}]; got != "34%…" {
		t.Errorf("at 40 columns, work's week so far is numbered %q, want %q", got, "34%…")
	}
}

// What Weeks says beside its weeks cuts its words, but each figure in it, a
// count, a share or a price, is drawn whole or as an ellipsis, never cut
// part-way: the headline's count, the one to drop's price, an account's
// verdict's share and its record's.
func TestWeeksWordsKeepTheirFiguresWhole(t *testing.T) {
	few, many := weeksDrawn{page: threeWeeks()}, weeksDrawn{page: tenWeeks()}
	plenty := weeksRow("side", nil, views.PlentyToSpare, 0, 59, 3)
	tests := []struct {
		name  string
		said  []part
		width int
		want  string
	}{
		{name: "the headline's count", said: few.headline(), width: 3, want: "…"},
		{name: "the one to drop's price", said: many.dropped(), width: 30, want: "spare is the one to drop: …"},
		{name: "a verdict's share", said: few.tableVerdict(plenty), width: 30, want: "plenty to spare most weeks  …"},
		{name: "a record's share, filling it", said: few.accountAside(few.page.Accounts[0])[1], width: 27, want: "limit hit 3 of 7 weeks · 9%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitParts(tt.width, tt.said...).plain(); got != tt.want {
				t.Errorf("in %d cells, it reads %q, want %q", tt.width, got, tt.want)
			}
		})
	}
}
