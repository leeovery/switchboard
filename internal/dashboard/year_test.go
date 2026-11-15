package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// ratioOf is worth over cost, as Against gives it.
func ratioOf(worth, cost float64) *float64 {
	r := worth / cost
	return &r
}

// framedMonth is a BY MONTH row as a frame writes it: its month, as views
// names it, its requests and worth, and of all use, its limits, or of a
// version, its share; the month so far marked so, and From where it's
// counted from a day part-way through.
type framedMonth struct {
	month    string
	requests int
	worth    float64
	limits   int
	share    float64
	soFar    bool
	from     string
}

// The frames' world: three accounts on Ultra, $600 a month, the months'
// names over the first week holding four of their days, two cells a week
// from the grid's first column, from Monday 29 September 2025; and the
// frames' day, Thursday 1 October 2026, whose month, October, is the current
// one, where the frames light September.
var (
	framesFirstWeek = time.Date(2025, 9, 29, 0, 0, 0, 0, time.UTC)
	framesLabels    = []views.MonthLabel{
		{Month: "2025-10", Week: 0}, {Month: "2025-11", Week: 5}, {Month: "2025-12", Week: 9}, {Month: "2026-01", Week: 13},
		{Month: "2026-02", Week: 18}, {Month: "2026-03", Week: 22}, {Month: "2026-04", Week: 26}, {Month: "2026-05", Week: 31},
		{Month: "2026-06", Week: 35}, {Month: "2026-07", Week: 39}, {Month: "2026-08", Week: 44}, {Month: "2026-09", Week: 48},
		{Month: "2026-10", Week: 52, Current: true},
	}
	framesMonths = []framedMonth{
		{month: "2025-10", requests: 10148, worth: 1180, limits: 23}, {month: "2025-11", requests: 11524, worth: 1340, limits: 26},
		{month: "2025-12", requests: 10406, worth: 1210, limits: 23}, {month: "2026-01", requests: 13416, worth: 1560, limits: 30},
		{month: "2026-02", requests: 14792, worth: 1720, limits: 33}, {month: "2026-03", requests: 17028, worth: 1980, limits: 38},
		{month: "2026-04", requests: 18490, worth: 2150, limits: 41}, {month: "2026-05", requests: 20038, worth: 2330, limits: 45},
		{month: "2026-06", requests: 21242, worth: 2470, limits: 48}, {month: "2026-07", requests: 23134, worth: 2690, limits: 52},
		{month: "2026-08", requests: 24596, worth: 2860, limits: 55}, {month: "2026-09", requests: 26832, worth: 3120, limits: 60},
		{month: "2026-10", requests: 1548, worth: 180, limits: 2, soFar: true},
	}
	opusMonths = []framedMonth{
		{month: "2026-06", requests: 3120, worth: 380, share: 0.15, from: "2026-06-22"}, {month: "2026-07", requests: 12940, worth: 1520, share: 0.57},
		{month: "2026-08", requests: 14860, worth: 1730, share: 0.60}, {month: "2026-09", requests: 17320, worth: 2010, share: 0.64},
		{month: "2026-10", requests: 640, worth: 120, share: 0.67, soFar: true},
	}
)

// framesNumbers are LAST 12 MONTHS' numbers as the frames give them.
var framesNumbers = views.YearNumbers{
	Requests: 236410, Sessions: 1688, Limits: map[string]int{"5h": 402}, Amount: usd(27480),
	Cost: usd(7200), Ratio: ratioOf(27480, 7200), BusiestDay: "2026-09-22", LongestRun: 58,
}

// monthOf is the month as BY MONTH's row of it, its worth split by family as
// the frame's bar along row y from column x is, a family's worth its cells':
// all Opus where the frame draws no bar there. Of all use, its plans cost
// $600, and its worth is against that but in the month so far; of a
// version, it has its share.
func monthOf(m framedMonth, frame [][]seen, families map[shade]string, x, y int) views.MonthRow {
	cells := make(map[string]int)
	for col := x; seenAt(frame, col, y).glyph == '▅'; col++ {
		cells[families[seenAt(frame, col, y).fg]]++
	}
	drawn := 0
	for _, n := range cells {
		drawn += n
	}
	month := views.Month{Month: m.month, Requests: m.requests, Amount: usd(m.worth)}
	for _, family := range []string{"opus", "sonnet", "fable", "haiku"} {
		if n := cells[family]; n > 0 {
			month.ByFamily = append(month.ByFamily, views.FamilyWorth{Family: family, Amount: usd(m.worth * float64(n) / float64(drawn))})
		}
	}
	if drawn == 0 {
		month.ByFamily = []views.FamilyWorth{{Family: "opus", Worth: month.Worth}}
	}
	if m.share > 0 {
		return views.MonthRow{Month: month, Share: &m.share, From: m.from, SoFar: m.soFar}
	}
	month.Limits, month.Against = map[string]int{"5h": m.limits}, views.Against{Cost: usd(600)}
	if !m.soFar {
		month.Ratio = ratioOf(m.worth, 600)
	}
	return views.MonthRow{Month: month, From: m.from, SoFar: m.soFar}
}

// yearFrame is a frame of Trends, YEAR and BY MONTH shown, in nord, of a
// terminal the size given, YEAR shown as asked, with the page given, the
// last 12 months read, its windows named as claude names them.
func yearFrame(t *testing.T, width, height int, ask views.YearAsk, page views.YearPage) Frame {
	t.Helper()
	f := graphsFrame(width, height, GraphsSectionsNamed([]string{"year", "months"}))
	f.Look = Screen(builtin(t, "nord"))
	f.Graphs.Read, f.Graphs.Split, f.Graphs.Year = true, ask, &page
	f.WindowName = func(key string) string {
		return map[string]string{"5h": "5-hour", "7d": "Week", "7d_oi": "Fable week"}[key]
	}
	return f
}

// shownYear is the Year the frame's Trends draws, failing the test where it
// draws none.
func shownYear(t *testing.T, f Frame) yearDrawn {
	t.Helper()
	y, ok := f.graphsYear()
	if !ok {
		t.Fatal("Trends draws no Year")
	}
	return y
}

// drawnYear draws the frame's YEAR and BY MONTH, as Trends lays them out,
// on a canvas of their own: YEAR's days from row 0, and BY MONTH's rows and
// the bars' key under them from the row it returns, a blank row under the
// days.
func drawnYear(t *testing.T, f Frame) (*canvas, int) {
	t.Helper()
	y := shownYear(t, f)
	months := y.daysRows() + 1
	c := newCanvas(f.Width, months+y.height())
	y.days(c, 0)
	y.months(c, months)
	y.dots(c, y.form.barX, months+y.dotsRow())
	return c, months
}

// yearText is the frame's Year, as drawnYear draws it, as text.
func yearText(t *testing.T, f Frame) []string {
	t.Helper()
	c, _ := drawnYear(t, f)
	return textOf(c.rows(Look{}))
}

// monthsDrawn draws BY MONTH's rows and the bars' key under them from row
// 0, on a canvas of the Year's height.
func monthsDrawn(y yearDrawn) *canvas {
	c := newCanvas(y.Width, y.height())
	y.months(c, 0)
	y.dots(c, y.form.barX, y.dotsRow())
	return c
}

// plainYear is the Year's page of the frames' numbers and months, its grid
// every day at level 2 but this week's to come, the families the frames'.
func plainYear() views.YearPage {
	page := views.YearPage{
		Weeks: make([]views.GridWeek, 53), Months: framesLabels, KeyCuts: []int{450, 750, 1000}, Numbers: framesNumbers,
		Families: []string{"opus", "sonnet", "fable", "haiku"},
	}
	for w := range page.Weeks {
		days := make([]views.GridDay, 7)
		for i := range days {
			days[i] = views.GridDay{Day: framesFirstWeek.AddDate(0, 0, 7*w+i).Format(time.DateOnly), Level: 2, LeftOut: w == 52 && i > 3}
		}
		page.Weeks[w] = views.GridWeek{First: days[0].Day, Days: days}
	}
	for _, m := range framesMonths {
		page.ByMonth = append(page.ByMonth, monthOf(m, nil, nil, 0, 0))
	}
	return page
}

// opusYear is plainYear filtered to Opus 5.5, its numbers and months the
// frame's.
func opusYear() views.YearPage {
	page := plainYear()
	page.Version, _ = ledger.Pricing.Version("claude-opus-5-5")
	page.Families, page.ByMonth = nil, nil
	page.Numbers = views.YearNumbers{Requests: 48880, Sessions: 352, Amount: usd(5760), Share: new(0.21), FirstUsed: "2026-06-22", BusiestDay: "2026-09-22"}
	for _, m := range opusMonths {
		page.ByMonth = append(page.ByMonth, monthOf(m, nil, nil, 0, 0))
	}
	return page
}

// numbersBesideX is the column the numbers are at beside the grid, as the
// design lays them out: six cells after the 54 weeks a year's span can
// hold; and besideFrom the width they're beside it from, their labels and
// limits reckoned at two windows of four figures, "week ×9,999 · 5-hour
// ×9,999", after it.
const (
	numbersBesideX = 128
	besideFrom     = numbersBesideX + yearLabelCells + 27
)

// yearOf is the Year the frame shows, laid out in the form given.
func yearOf(f Frame, form yearForm) yearDrawn {
	return yearDrawn{Frame: f, page: *f.Graphs.Year, ask: f.Graphs.Split, alone: f.Graphs.yearShown(), form: form}.laidOut()
}

// Today's month is the current one, its name over the grid in
// text.secondary, the rest faint, and its row of BY MONTH bold: wide, the
// whole year from column 14; in the narrow form, the latest 23 weeks from
// column 4; and BY MONTH's every month, a row each under its heads.
func TestTheCurrentMonthIsLitOverTheGridAndBoldInByMonth(t *testing.T) {
	tests := []struct {
		width                      int
		october, september, name   int
		septembersRow, octobersRow int
	}{
		{width: besideFrom, october: 118, september: 110, name: byMonthNameX, septembersRow: 12, octobersRow: 13},
		{width: 52, october: 48, september: 40, name: byMonthNarrowNameX, septembersRow: 12, octobersRow: 13},
	}
	for _, tt := range tests {
		f := yearFrame(t, tt.width, 60, views.YearAsk{}, plainYear())
		c, months := drawnYear(t, f)
		inks := []struct {
			what string
			x, y int
			want ink
		}{
			{"October over the grid", tt.october, 0, secondaryInk},
			{"September over the grid", tt.september, 0, faintInk},
			{"October in BY MONTH", tt.name, months + tt.octobersRow, strongInk},
			{"September in BY MONTH", tt.name, months + tt.septembersRow, secondaryInk},
		}
		for _, k := range inks {
			if got := c.at(k.x, k.y).ink; got != k.want {
				t.Errorf("at %d columns, %s is in %+v, want %+v", tt.width, k.what, got, k.want)
			}
		}
	}
}

// Where the look draws no shades of its own, as the terminal theme and
// NO_COLOR don't, a day is the glyph whose density tells its level, in the
// grid and in the key.
func TestWithoutShadesADaysLevelIsToldByItsGlyph(t *testing.T) {
	page := plainYear()
	for w := range page.Weeks {
		for i := range page.Weeks[w].Days {
			page.Weeks[w].Days[i].Level = i % shadeLevels
		}
	}
	for _, look := range []Look{Screen(builtin(t, "terminal")), NoColour()} {
		f := yearFrame(t, besideFrom, 37, views.YearAsk{}, page)
		f.Look = look
		rows := yearText(t, f)
		if got, want := []rune(rows[1])[yearGridX], '·'; got != want {
			t.Errorf("a day at level 0 is %q, want %q", got, want)
		}
		if got, want := []rune(rows[5])[yearGridX], '█'; got != want {
			t.Errorf("a day at level 4 is %q, want %q", got, want)
		}
		if got, want := rows[yearKeyRow], textsAt(besideFrom, map[int]string{yearGridX: "· none   ░ under 450   ▒ 450–750   ▓ 750–1,000   █ over 1,000  requests a day"}); got != want {
			t.Errorf("the key reads\n%q\nwant\n%q", got, want)
		}
	}
}

// Each figure a look can change is numbered as the Year's, by its label,
// after the version shown, of the account shown: LAST 12 MONTHS' all but
// what the plans cost, and BY MONTH's month so far's, but its against,
// which is a dash until it ends.
func TestTheYearsFiguresAreNumbered(t *testing.T) {
	year := func(account, of string) Number { return Number{Kind: NumberYear, Account: account, Of: of} }
	tests := []struct {
		name string
		ask  views.YearAsk
		page views.YearPage
		want map[Number]string
	}{
		{
			name: "all use", page: plainYear(),
			want: map[Number]string{
				year("", "requests"): "236,410", year("", "sessions"): "1,688", year("", "limits hit"): "5-hour ×402", year("", "worth"): "$27,480",
				year("", "against it"): "3.8×", year("", "busiest day"): "Tue 22 Sep", year("", "longest run"): "58 days",
				year("", "2026-10 requests"): "1,548", year("", "2026-10 worth"): "$180", year("", "2026-10 limits"): "2",
			},
		},
		{
			name: "an account's", ask: views.YearAsk{Account: "work"}, page: plainYear(),
			want: map[Number]string{
				year("work", "requests"): "236,410", year("work", "sessions"): "1,688", year("work", "limits hit"): "5-hour ×402",
				year("work", "worth"): "$27,480", year("work", "against it"): "3.8×", year("work", "busiest day"): "Tue 22 Sep",
				year("work", "longest run"): "58 days", year("work", "2026-10 requests"): "1,548", year("work", "2026-10 worth"): "$180",
				year("work", "2026-10 limits"): "2",
			},
		},
		{
			name: "a version's", ask: views.YearAsk{Version: "Opus 5.5"}, page: opusYear(),
			want: map[Number]string{
				year("", "Opus 5.5 requests"): "48,880", year("", "Opus 5.5 sessions"): "352", year("", "Opus 5.5 worth"): "$5,760",
				year("", "Opus 5.5 of all use"): "21%", year("", "Opus 5.5 first used"): "Mon 22 Jun", year("", "Opus 5.5 busiest day"): "Tue 22 Sep",
				year("", "Opus 5.5 2026-10 requests"): "640", year("", "Opus 5.5 2026-10 worth"): "$120", year("", "Opus 5.5 2026-10 share"): "67%",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := yearFrame(t, besideFrom, 37, tt.ask, tt.page)
			got := make(map[Number]string)
			for n, drawn := range f.Numbers(threeRouted(), now) {
				if n.Kind == NumberYear {
					got[n] = drawn
				}
			}
			for n, want := range tt.want {
				if got[n] != want {
					t.Errorf("%+v reads %q, want %q", n, got[n], want)
				}
			}
			for n, drawn := range got {
				if _, ok := tt.want[n]; !ok {
					t.Errorf("%+v, %q, is numbered, want it not", n, drawn)
				}
			}
		})
	}
}

// A period with no requests draws each of its numbers as a faint dash, and
// its key no shade but level 0's.
func TestAnEmptyYearsNumbersAreDashes(t *testing.T) {
	page := plainYear()
	page.Numbers, page.KeyCuts = views.YearNumbers{Cost: usd(7200)}, nil
	f := yearFrame(t, besideFrom, 37, views.YearAsk{}, page)
	c, _ := drawnYear(t, f)
	rows := textOf(c.rows(Look{}))
	for i, label := range []string{"requests", "sessions", "limits hit", "worth", "plans cost", "against it", "busiest day", "longest run"} {
		if got, want := rows[1+i], textsAt(besideFrom, map[int]string{numbersBesideX: padded(label, yearLabelCells) + "—"}); !strings.HasSuffix(got, want[numbersBesideX:]) {
			t.Errorf("%s reads %q, want a dash", label, got)
		}
		if got := c.at(numbersBesideX+yearLabelCells, 1+i).ink; got != faintInk {
			t.Errorf("%s's dash is in %+v, want text.faint", label, got)
		}
	}
	if got, want := rows[yearKeyRow], textsAt(besideFrom, map[int]string{yearGridX: "■ none  requests a day"}); got != want {
		t.Errorf("the key reads %q, want %q", got, want)
	}
}

// The numbers are headed by their period, the last 12 months, after the use
// shown alone; and the key's unit names it.
func TestTheNumbersAndTheKeyNameWhatsShown(t *testing.T) {
	tests := []struct {
		ask           views.YearAsk
		heading, unit string
	}{
		{ask: views.YearAsk{}, heading: "LAST 12 MONTHS", unit: "requests a day"},
		{ask: views.YearAsk{Version: "Opus 5.5"}, heading: "OPUS 5.5 · 12 MONTHS", unit: "Opus 5.5 requests a day"},
		{ask: views.YearAsk{Account: "work"}, heading: "WORK · 12 MONTHS", unit: "work requests a day"},
	}
	for _, tt := range tests {
		f := yearFrame(t, besideFrom, 37, tt.ask, plainYear())
		rows := yearText(t, f)
		if got := strings.TrimSpace(string([]rune(rows[0])[numbersBesideX:])); got != tt.heading {
			t.Errorf("%+v: the numbers are headed %q, want %q", tt.ask, got, tt.heading)
		}
		if got := rows[yearKeyRow]; !strings.HasSuffix(got, "  "+tt.unit) {
			t.Errorf("%+v: the key reads %q, want it to end %q", tt.ask, got, tt.unit)
		}
	}
}

// An account shown alone is named by its label, not its id, wherever the
// Year names it: the key line's split, the numbers' heading and the key.
func TestTheYearNamesAnAccountShownAloneByItsLabel(t *testing.T) {
	f := yearFrame(t, besideFrom, 37, views.YearAsk{Account: "acct-2"}, plainYear())
	f.Graphs.Accounts = config.Accounts{{ID: "work", Label: "Work"}, {ID: "acct-2", Label: "Studio"}}
	rows := yearText(t, f)

	if got := f.Graphs.SplitShown(); got != "Studio" {
		t.Errorf("v names %q, want Studio", got)
	}
	if got := strings.TrimSpace(string([]rune(rows[0])[numbersBesideX:])); got != "STUDIO · 12 MONTHS" {
		t.Errorf("the numbers are headed %q, want %q", got, "STUDIO · 12 MONTHS")
	}
	if got := rows[yearKeyRow]; !strings.HasSuffix(got, "  Studio requests a day") {
		t.Errorf("the key reads %q, want it to end %q", got, "Studio requests a day")
	}
	if text := strings.Join(textOf(f.page(threeRouted(), now).rows(Look{})), "\n"); strings.Contains(strings.ToLower(text), "acct-2") {
		t.Errorf("Trends names the account by its id somewhere:\n%s", text)
	}
}

// A month's bar is its worth against the largest month's, 1 cell at least
// and the most at most, a half rounded to the even, as the frames have it;
// split among its families by their worth, each run ending where its share
// and those before it round to, so they fill it.
func TestAMonthsBarIsItsWorthSplitByFamily(t *testing.T) {
	cells := []struct {
		worth, largest float64
		most, want     int
	}{
		{worth: 3120, largest: 3120, most: 30, want: 30},
		{worth: 2860, largest: 3120, most: 30, want: 28},
		{worth: 2860, largest: 3120, most: 18, want: 16},
		{worth: 1, largest: 3120, most: 30, want: 1},
		{worth: 0, largest: 3120, most: 30, want: 0},
	}
	for _, tt := range cells {
		if got := barUnits(*usd(tt.worth), *usd(tt.largest), tt.most); got != tt.want {
			t.Errorf("barUnits($%v, $%v, %d) = %d, want %d", tt.worth, tt.largest, tt.most, got, tt.want)
		}
	}
	families := []views.FamilyWorth{
		{Family: "opus", Amount: usd(322)}, {Family: "sonnet", Amount: usd(751)},
		{Family: "fable", Amount: usd(1)}, {Family: "haiku", Amount: usd(107)},
	}
	want := line{{"▅▅▅", familyHue("opus").ink()}, {"▅▅▅▅▅▅▅", familyHue("sonnet").ink()}, {"▅", familyHue("haiku").ink()}}
	if got := familySplit(families, 11); !slices.Equal(got, want) {
		t.Errorf("the bar is %+v, want %+v, Fable too little for a cell", got, want)
	}
}

// After a part month's bar: from the day it's counted from, or so far; and
// filtered to a version, a faint row before its months for those before it
// was used, naming the version of its family used then.
func TestByMonthsNotesAndTheRowBefore(t *testing.T) {
	notes := []struct {
		row  views.MonthRow
		want string
	}{
		{row: views.MonthRow{From: "2026-03-16"}, want: "from the 16th"},
		{row: views.MonthRow{From: "2026-06-22", SoFar: true}, want: "from the 22nd"},
		{row: views.MonthRow{SoFar: true}, want: "so far"},
		{row: views.MonthRow{}, want: ""},
	}
	for _, tt := range notes {
		if got := fitParts(80, monthNote(tt.row)...).plain(); got != tt.want {
			t.Errorf("monthNote(%+v) = %q, want %q", tt.row, got, tt.want)
		}
	}
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 23: "23rd", 31: "31st"} {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
	before := []struct {
		before views.MonthsBefore
		want   string
	}{
		{before: views.MonthsBefore{From: "2025-10", To: "2026-05", Then: "Opus 5"}, want: "  Oct 2025 – May          none: Opus 5 then"},
		{before: views.MonthsBefore{From: "2026-03", To: "2026-03"}, want: "  Mar 2026                none"},
	}
	for _, tt := range before {
		c := newCanvas(80, 1)
		monthsBefore(c, 0, tt.before, 80)
		if got := textOf(c.rows(Look{}))[0]; got != tt.want {
			t.Errorf("the row before reads %q, want %q", got, tt.want)
		}
		if got := c.at(byMonthWorthX, 0).ink; got != faintInk {
			t.Errorf("the row before is in %+v, want text.faint", got)
		}
	}
}

// The Year's height, as it's laid out, is the rows BY MONTH draws: wide, to
// the bars' key two rows under its last month, or to that month where there
// are no bars to key, as filtered to a version, with the row before its
// months and without; and in the narrow form, to the bars' key under every
// month, on two rows.
func TestTheYearsHeightIsTheRowsItDraws(t *testing.T) {
	unpriced := plainYear()
	unpriced.Families = nil
	before := opusYear()
	before.Before = &views.MonthsBefore{From: "2025-10", To: "2026-05", Then: "Opus 5"}
	opus := views.YearAsk{Version: "Opus 5.5"}
	tests := []struct {
		name  string
		width int
		ask   views.YearAsk
		page  views.YearPage
		want  int
	}{
		{name: "all use", width: besideFrom, page: plainYear(), want: 16},
		{name: "no bars to key", width: besideFrom, page: unpriced, want: 14},
		{name: "a version", width: besideFrom, ask: opus, page: opusYear(), want: 6},
		{name: "a version, with the months before it", width: besideFrom, ask: opus, page: before, want: 7},
		{name: "narrow, its bars' key on two rows", width: 52, page: plainYear(), want: 17},
	}
	for _, tt := range tests {
		f := yearFrame(t, tt.width, 60, tt.ask, tt.page)
		drawn := growing(f.Width)
		y := shownYear(t, f)
		y.months(drawn, 0)
		y.dots(drawn, y.form.barX, y.dotsRow())
		if got := y.height(); got != drawn.height() || got != tt.want {
			t.Errorf("%s: the Year's height is %d, and it draws %d rows; want %d", tt.name, got, drawn.height(), tt.want)
		}
	}
}

// The Year's narrow key writes a thousand as k.
func TestTheYearsNarrowKeyWritesAThousandAsK(t *testing.T) {
	for n, want := range map[int]string{450: "450", 1000: "1k", 1250: "1.25k", 12000: "12k"} {
		if got := keyNarrow.number(n); got != want {
			t.Errorf("the narrow key's number(%d) = %q, want %q", n, got, want)
		}
	}
}

// A figure in BY MONTH is drawn whole, a cell short of the next column, or
// of the bars, or where it doesn't fit, as an ellipsis alone, as one cut
// part-way would read as another, money and counts alike: in the narrow form, a
// month worth $1,234,567; wide, a month of 1,234,567,890 requests.
func TestByMonthsFiguresAreDrawnWholeOrAsAnEllipsis(t *testing.T) {
	tests := []struct {
		name  string
		width int
		month func(m *views.MonthRow)
		want  string
	}{
		{
			name: "worth, narrow", width: 52,
			month: func(m *views.MonthRow) { m.Amount, m.Ratio = usd(1_234_567), new(0.4) },
			want:  " Sep        …         0.4×",
		},
		{
			name: "requests, wide", width: 160,
			month: func(m *views.MonthRow) { m.Requests = 1_234_567_890 },
			want:  "  Sep         …           $3,120",
		},
	}
	for _, tt := range tests {
		page := plainYear()
		tt.month(&page.ByMonth[len(page.ByMonth)-2])
		rows := yearText(t, yearFrame(t, tt.width, 60, views.YearAsk{}, page))
		i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(strings.TrimSpace(row), "Sep ") })
		if i < 0 || !strings.HasPrefix(rows[i], tt.want) {
			t.Errorf("%s, September's row reads %q, want it to start %q", tt.name, rows[max(i, 0)], tt.want)
		}
	}
}

// A figure of the month so far drawn as an ellipsis is no number, so its
// changing brightens nothing: in the narrow form, a worth of $1,234,567, or one
// changed to it from $180.
func TestAMonthSoFarsFigureDrawnAsAnEllipsisBrightensNothing(t *testing.T) {
	worth := Number{Kind: NumberYear, Of: "2026-10 worth"}
	tests := []struct {
		amount float64
		want   string
	}{
		{amount: 180, want: "$180"},
		{amount: 1_234_567},
		{amount: 2_345_678},
	}
	for _, tt := range tests {
		page := plainYear()
		page.ByMonth[len(page.ByMonth)-1].Amount = usd(tt.amount)
		if got := yearFrame(t, 52, 60, views.YearAsk{}, page).Numbers(threeRouted(), now)[worth]; got != tt.want {
			t.Errorf("the month so far worth $%.0f is numbered %q, want %q", tt.amount, got, tt.want)
		}
	}
}

// The grid shows the latest weeks that fit, in either form, the whole year
// at most: as many as end, with their months' names over them, short of the
// frame's margin, as 33 do at 73 columns, where 34 would put Oct in it. The
// weeks it leaves off are the earliest, each week it shows in the column the
// latest weeks' have at 52 columns, less the weeks left off.
func TestTheGridShowsTheLatestWeeksThatFit(t *testing.T) {
	tests := []struct {
		width, weeks int
		narrow       bool
	}{
		{width: 40, weeks: 17, narrow: true},
		{width: 52, weeks: 23, narrow: true},
		{width: 73, weeks: 33, narrow: true},
		{width: 108, weeks: 46},
		{width: 120, weeks: 52},
		{width: 122, weeks: 53},
		{width: 220, weeks: 53},
	}
	for _, tt := range tests {
		f := yearFrame(t, tt.width, 60, views.YearAsk{}, plainYear())
		y := shownYear(t, f)
		if y.form.narrow != tt.narrow || y.laid.weeks != tt.weeks {
			t.Errorf("at %d columns, the grid shows %d weeks, its form narrow %t; want %d, %t", tt.width, y.laid.weeks, y.form.narrow, tt.weeks, tt.narrow)
		}
		rows := yearText(t, f)
		for i := range 8 {
			if row := rows[i]; ansi.StringWidth(row) > f.edge() {
				t.Errorf("at %d columns, the grid's row %d runs into the margin:\n%q", tt.width, i, row)
			}
		}
	}
	drawn := func(width int) *canvas {
		c, _ := drawnYear(t, yearFrame(t, width, 60, views.YearAsk{}, plainYear()))
		return c
	}
	at52, at40 := drawn(52), drawn(40)
	const leftOff = 2 * (23 - 17)
	for row := 1; row <= 7; row++ {
		for x := yearNarrowGridX; x < 40; x++ {
			if got, want := *at40.at(x, row), *at52.at(x+leftOff, row); got != want {
				t.Errorf("at 40 columns, row %d, column %d is %+v, want %+v, as 52 columns draw the week 6 later", row, x, got, want)
			}
		}
	}
}

// yearPages are the Year of each kind of use, with its data at its
// widest: all use, a version, an account, all use with some plans
// unknown, and all use that hit limits in three windows.
func yearPages(t *testing.T) map[string]func(width int) Frame {
	t.Helper()
	unknown := plainYear()
	unknown.Numbers.Plans = views.PlansKnown{Known: 2, Of: 3}
	three := plainYear()
	three.Numbers.Limits = map[string]int{"5h": 4120, "7d": 3, "7d_oi": 2}
	return map[string]func(width int) Frame{
		"all use":            func(width int) Frame { return yearFrame(t, width, 60, views.YearAsk{}, plainYear()) },
		"a version":          func(width int) Frame { return yearFrame(t, width, 60, views.YearAsk{Version: "Opus 5.5"}, opusYear()) },
		"an account":         func(width int) Frame { return yearFrame(t, width, 60, views.YearAsk{Account: "work"}, plainYear()) },
		"some plans unknown": func(width int) Frame { return yearFrame(t, width, 60, views.YearAsk{}, unknown) },
		"three windows hit":  func(width int) Frame { return yearFrame(t, width, 60, views.YearAsk{}, three) },
	}
}

// The Year takes its wide form where its key fits whole, reckoned at cuts
// of four figures: from 99 columns; narrower, the narrow form's. Its numbers are
// beside the grid, at column 128, from one width, whatever's shown:
// besideFrom, their labels and limits of two windows of four figures fitting
// after the 54 weeks a span can hold; narrower, and always in the narrow form,
// they're under the key.
func TestTheYearsArrangementIsWhereItsPartsFit(t *testing.T) {
	for name, frame := range yearPages(t) {
		for _, width := range []int{73, 98, 99, 138, besideFrom - 1, besideFrom, 220} {
			y := shownYear(t, frame(width))
			narrow, beside := width < 99, width >= besideFrom
			if y.form.narrow != narrow || y.laid.beside != beside {
				t.Errorf("%s at %d columns: its narrow form %t, the numbers beside the grid %t; want %t, %t", name, width, y.form.narrow, y.laid.beside, narrow, beside)
			}
			if beside && y.laid.numbersX != numbersBesideX {
				t.Errorf("%s at %d columns: the numbers are beside the grid at column %d, want %d", name, width, y.laid.numbersX, numbersBesideX)
			}
		}
	}
}

// Where the numbers sit never hangs on the data, so v and < > never move BY
// MONTH: at every width, all use, a version, an account, some plans unknown,
// three windows hit and a period without use take the one arrangement.
func TestTheYearsArrangementNeverHangsOnItsData(t *testing.T) {
	unused := plainYear()
	unused.Numbers, unused.KeyCuts = views.YearNumbers{}, nil
	pages := yearPages(t)
	pages["without use"] = func(width int) Frame { return yearFrame(t, width, 60, views.YearAsk{}, unused) }
	for width := 52; width <= 220; width++ {
		arrangements := map[[2]bool][]string{}
		for name, frame := range pages {
			y := shownYear(t, frame(width))
			key := [2]bool{y.form.narrow, y.laid.beside}
			arrangements[key] = append(arrangements[key], name)
		}
		if len(arrangements) > 1 {
			t.Errorf("at %d columns, the Year takes more than one arrangement, by its data: %v", width, arrangements)
		}
	}
}

// The Year's form never hangs on its data: the key is reckoned at cuts of
// four figures and its unit without the use shown, so at every width, a
// period with use, one without, one of cuts of five figures and an
// account's with a long label each take the one form.
func TestTheYearsFormNeverHangsOnItsData(t *testing.T) {
	unused := plainYear()
	unused.Numbers, unused.KeyCuts = views.YearNumbers{}, nil
	busy := plainYear()
	busy.KeyCuts = []int{12_000, 24_000, 48_000}
	for width := 52; width <= 170; width++ {
		long := yearFrame(t, width, 60, views.YearAsk{Account: "acct-2"}, plainYear())
		long.Graphs.Accounts = config.Accounts{{ID: "acct-2", Label: "the-research-team-long-label"}}
		forms := map[bool][]string{}
		for name, f := range map[string]Frame{
			"with use":        yearFrame(t, width, 60, views.YearAsk{}, plainYear()),
			"without use":     yearFrame(t, width, 60, views.YearAsk{}, unused),
			"of busy days":    yearFrame(t, width, 60, views.YearAsk{}, busy),
			"of a long label": long,
		} {
			y := shownYear(t, f)
			forms[y.form.narrow] = append(forms[y.form.narrow], name)
		}
		if len(forms) > 1 {
			t.Errorf("at %d columns, the Year takes more than one form, by its data: %v", width, forms)
		}
	}
}

// The numbers under the key are in columns of three, four cells apart at
// least and spread to end where the grid ends, each as wide as its widest
// figure whole: wide, with their wide labels, counts,
// money, then the busiest day and the longest run; in the narrow form, with their
// short labels. Where the room is short, a figure takes its shorter forms
// before any whole figure leaves: the second column narrows first, against
// it over the plans known shortened; then the columns past the second
// leave; then the first narrows, its limits leaving off their windows
// whole; then the second leaves; and where even the first's shortest forms
// don't fit, they're an ellipsis. With this world's figures, the narrow form's
// third column fits from 76 columns.
func TestTheNumbersUnderTheKeyAreInColumnsThatFit(t *testing.T) {
	two := map[string]int{"5h": 412, "7d": 3}
	tests := []struct {
		name   string
		width  int
		limits map[string]int
		plans  views.PlansKnown
		want   []string
	}{
		{
			name: "wide, three columns, spread to the grid's end", width: 138, limits: two,
			want: []string{
				"requests     236,410                                   worth        $27,480                     busiest day  Tue 22 Sep",
				"sessions     1,688                                     plans cost   $7,200                      longest run  58 days",
				"limits hit   week ×3 · 5-hour ×412                     against it   3.8×",
			},
		},
		{
			name: "narrow, two columns, limits whole, spread to the grid's end", width: 73, limits: two,
			want: []string{
				"requests  236,410                                  worth     $27,480",
				"sessions  1,688                                    plans     $7,200",
				"limits    week ×3 · 5-hour ×412                    against   3.8×",
			},
		},
		{
			name: "narrow with room for a third", width: 76, limits: two,
			want: []string{
				"requests  236,410                  worth     $27,480    busiest   Tue 22 Sep",
				"sessions  1,688                    plans     $7,200     longest   58 days",
				"limits    week ×3 · 5-hour ×412    against   3.8×",
			},
		},
		{
			name: "narrow, its limits short of room, the 5-hour left off", width: 46, limits: two,
			want: []string{
				"requests  236,410           worth     $27,480",
				"sessions  1,688             plans     $7,200",
				"limits    week ×3           against   3.8×",
			},
		},
		{
			name: "narrow, its limits whole, the second column left", width: 46, limits: map[string]int{"5h": 41_200_000},
			want: []string{
				"requests  236,410",
				"sessions  1,688",
				"limits    5-hour ×41,200,000",
			},
		},
		{
			name: "narrow, its limits too long for even one", width: 24, limits: map[string]int{"5h": 41_200_000},
			want: []string{
				"requests  236,410",
				"sessions  1,688",
				"limits    …",
			},
		},
		{
			name: "plans unknown, against compact before a column leaves", width: 80, limits: two, plans: views.PlansKnown{Known: 2, Of: 3},
			want: []string{
				"requests  236,410                  worth     $27,480        busiest   Tue 22 Sep",
				"sessions  1,688                    plans     $7,200         longest   58 days",
				"limits    week ×3 · 5-hour ×412    against   3.8× · 2/3",
			},
		},
		{
			name: "plans unknown, against its ratio alone before a column leaves", width: 66, limits: map[string]int{"5h": 412}, plans: views.PlansKnown{Known: 2, Of: 3},
			want: []string{
				"requests  236,410        worth     $27,480    busiest   Tue 22 Sep",
				"sessions  1,688          plans     $7,200     longest   58 days",
				"limits    5-hour ×412    against   3.8×",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := plainYear()
			page.Numbers.Limits, page.Numbers.Plans = tt.limits, tt.plans
			f := yearFrame(t, tt.width, 60, views.YearAsk{}, page)
			rows := yearText(t, f)
			first := yearNumbersRow + 1
			if got := rows[first : first+3]; !slices.Equal(got, tt.want) {
				t.Errorf("at %d columns, the numbers read\n%s\nwant\n%s", tt.width, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// BY MONTH's bars take the room after its columns, the largest month's
// ending as far short of the edge as a month's name starts in from the
// left, in either form.
func TestByMonthsBarsTakeTheRoomAfterTheirColumns(t *testing.T) {
	for width, want := range map[int]int{40: 6, 60: 26, 73: 39, 98: 64, 99: 37, 100: 38, 160: 98, 220: 158} {
		y := shownYear(t, yearFrame(t, width, 60, views.YearAsk{}, plainYear()))
		if y.laid.bar != want {
			t.Errorf("at %d columns, BY MONTH's bars take %d cells at most, want %d", width, y.laid.bar, want)
		}
		c := monthsDrawn(y)
		september := y.laid.byMonth.last() - 1
		if got := strings.Count(textOf(c.rows(Look{}))[september], "▅"); got != want {
			t.Errorf("at %d columns, September, the largest month, has a bar of %d cells, want %d", width, got, want)
		}
	}
}

// In the narrow form, BY MONTH shows every month, as wide, the first with its year,
// the month so far's bar followed by so far, and the families' key two rows
// under the last month at the bars' column.
func TestANarrowByMonthShowsEveryMonth(t *testing.T) {
	y := shownYear(t, yearFrame(t, 73, 61, views.YearAsk{}, plainYear()))
	at := y.laid.byMonth
	rows := textOf(monthsDrawn(y).rows(Look{}))
	if len(at.months) != 13 || !strings.HasPrefix(rows[at.first], " Oct 2025 ") {
		t.Errorf("BY MONTH shows %d months, the first reading %q; want 13, from Oct 2025", len(at.months), rows[at.first])
	}
	if got := rows[at.last()]; !strings.HasSuffix(got, "  so far") {
		t.Errorf("the month so far reads %q, want it to end so far", got)
	}
	if got, want := rows[y.dotsRow()], textsAt(byMonthNarrowBarX+37, map[int]string{byMonthNarrowBarX: "● Opus   ● Sonnet   ● Fable   ● Haiku"}); got != want {
		t.Errorf("the families' key reads %q, want %q", got, want)
	}
}

// Nothing the Year draws is left to the canvas to clip: of all use and of a
// version with the months before it, at any width, it draws nothing past
// the frame's width.
func TestTheYearDrawsNothingPastItsEdge(t *testing.T) {
	before := opusYear()
	before.Before = &views.MonthsBefore{From: "2025-10", To: "2026-05", Then: "Opus 5"}
	pages := map[string]func(width int) Frame{
		"all use":   func(width int) Frame { return yearFrame(t, width, 60, views.YearAsk{}, plainYear()) },
		"a version": func(width int) Frame { return yearFrame(t, width, 60, views.YearAsk{Version: "Opus 5.5"}, before) },
	}
	for name, frame := range pages {
		for _, width := range []int{1, 4, 12, 20, 28, 36, 44, 52, 60, 73, 80, 98, 99, 100, 120, 138, 148, 149, 160} {
			f := frame(width)
			y := shownYear(t, f)
			months := y.daysRows() + 1
			c := newCanvas(width+80, months+y.height())
			y.days(c, 0)
			y.months(c, months)
			y.dots(c, y.form.barX, months+y.dotsRow())
			for row, cells := range c.cells {
				if x := slices.IndexFunc(cells[width:], func(here cell) bool { return here != blank }); x >= 0 {
					t.Errorf("%s at %d columns draws %q past its edge, at column %d of row %d:\n%q",
						name, width, cells[width+x].glyph, width+x, row, textOf(c.rows(Look{}))[row])
				}
			}
		}
	}
}

// Beside the grid, a figure longer than reckoned takes its shorter forms,
// never cut part-way: from besideFrom, limits of two windows read whole, and
// of three, their last window left off whole; against it over the plans
// known, short.
func TestAFigureBesideTheGridTakesItsShorterForms(t *testing.T) {
	pages := yearPages(t)
	tests := []struct {
		page, label, want string
	}{
		{page: "all use", label: "limits hit", want: "limits hit   5-hour ×402"},
		{page: "three windows hit", label: "limits hit", want: "limits hit   week ×3 · 5-hour ×4,120"},
		{page: "some plans unknown", label: "against it", want: "against it   3.8× · 2 of 3 plans known"},
	}
	for _, tt := range tests {
		f := pages[tt.page](besideFrom)
		rows := yearText(t, f)
		i := rowWith(rows, tt.label)
		if i < 0 || !strings.HasSuffix(rows[i], tt.want) || ansi.StringWidth(rows[i]) > f.Width {
			t.Errorf("%s at %d columns, %s reads %q, want it to end %q", tt.page, besideFrom, tt.label, rows[max(i, 0)], tt.want)
		}
	}
}

// A part month's bar leaves its note's cells, so a part month never reads as
// a whole one: where it's the largest month, every bar shrinks with it,
// each its month's share; in the narrow form at 73 columns, and wide at 99; and of
// a ledger whose only month is the month so far.
func TestAPartMonthsBarLeavesItsNoteRoom(t *testing.T) {
	largest := plainYear()
	largest.ByMonth[len(largest.ByMonth)-1].Amount = usd(5000)
	alone := plainYear()
	alone.ByMonth = alone.ByMonth[len(alone.ByMonth)-1:]
	tests := []struct {
		name             string
		width            int
		page             views.YearPage
		soFar, september int
	}{
		{name: "the largest, narrow", width: 73, page: largest, soFar: 33, september: 21},
		{name: "the largest, wide", width: 99, page: largest, soFar: 31, september: 19},
		{name: "the only month, narrow", width: 73, page: alone, soFar: 33},
	}
	for _, tt := range tests {
		y := shownYear(t, yearFrame(t, tt.width, 60, views.YearAsk{}, tt.page))
		rows := textOf(monthsDrawn(y).rows(Look{}))
		last := rows[y.laid.byMonth.last()]
		if got := strings.Count(last, "▅"); got != tt.soFar || !strings.HasSuffix(last, "▅  so far") {
			t.Errorf("%s: the month so far reads %q, its bar %d cells; want %d, then so far", tt.name, last, got, tt.soFar)
		}
		if tt.september == 0 {
			continue
		}
		if got := strings.Count(rows[y.laid.byMonth.last()-1], "▅"); got != tt.september {
			t.Errorf("%s: September's bar is %d cells, want %d", tt.name, got, tt.september)
		}
	}
}

// A part month's note leaves where it doesn't fit after its bar with the
// bars at half their most or more, and then the bars keep their most: of
// Opus 5.5 in the narrow form at 48 columns, the bars shrink to 7 cells, half
// their 14, for June's note, from the 22nd; at 47, half their 13 won't hold
// it, so June has no note and September its 13 cells; and a part month that
// isn't the largest, the month so far, keeps its note beside its bar.
func TestAPartMonthsNoteLeavesWhereItDoesntFit(t *testing.T) {
	for width, tt := range map[int]struct {
		noted     bool
		september int
	}{48: {noted: true, september: 7}, 47: {september: 13}} {
		y := shownYear(t, yearFrame(t, width, 60, views.YearAsk{Version: "Opus 5.5"}, opusYear()))
		rows := textOf(monthsDrawn(y).rows(Look{}))
		at := y.laid.byMonth
		june, september, october := rows[at.first], rows[at.last()-1], rows[at.last()]
		if strings.HasSuffix(june, "▅  from the 22nd") != tt.noted || strings.Count(september, "▅") != tt.september {
			t.Errorf("at %d columns, June reads %q and September %q; want June's note %t, September's bar %d cells", width, june, september, tt.noted, tt.september)
		}
		if !strings.HasSuffix(october, "▅  so far") {
			t.Errorf("at %d columns, the month so far reads %q, want its note after its bar", width, october)
		}
	}
}

// A year's span can fall across 54 weeks, as 2028's does with weeks from
// Sunday; beside the grid, all 54 show, and the gap before the numbers
// holds: every row's cells from the last week to the numbers' column are
// blank, and the numbers start there.
func TestA54WeekYearKeepsTheGapBeforeTheNumbers(t *testing.T) {
	page := plainYear()
	week := page.Weeks[0]
	earlier := views.GridWeek{First: framesFirstWeek.AddDate(0, 0, -7).Format(time.DateOnly), Days: week.Days}
	page.Weeks = append([]views.GridWeek{earlier}, page.Weeks...)
	page.Months = slices.Clone(page.Months)
	for i := range page.Months {
		page.Months[i].Week++
	}
	f := yearFrame(t, besideFrom, 60, views.YearAsk{}, page)
	y := shownYear(t, f)
	if y.laid.weeks != 54 || !y.laid.beside {
		t.Fatalf("at %d columns, the grid shows %d weeks, the numbers beside it %t; want 54, true", besideFrom, y.laid.weeks, y.laid.beside)
	}
	rows := yearText(t, f)
	lastWeek := yearGridX + 2*53
	for i := 1; i <= 7; i++ {
		row := []rune(rows[i])
		if gap := string(row[lastWeek+1 : numbersBesideX]); strings.TrimSpace(gap) != "" || row[numbersBesideX] == ' ' {
			t.Errorf("row %d reads %q, want the cells from column %d to %d blank and the numbers from %d", i, string(row), lastWeek+1, numbersBesideX, numbersBesideX)
		}
	}
}

// The numbers' heading cuts the name of the use shown, never its period: an
// account of a label longer than the column beside the grid has. At every
// width from 8 columns, the period follows any name before it whole: a use's
// name down to its ellipsis alone, from 13 columns; LAST whole, from 14.
// Narrower, the period shows alone, whole where it fits, else as an
// ellipsis.
func TestTheNumbersHeadingCutsItsNameNeverItsPeriod(t *testing.T) {
	long := config.Accounts{{ID: "acct-2", Label: "the-research-team-with-a-label-longer-than-its-column"}}
	f := yearFrame(t, besideFrom, 60, views.YearAsk{Account: "acct-2"}, plainYear())
	f.Graphs.Accounts = long
	heading := string([]rune(yearText(t, f)[0])[numbersBesideX:])
	if want := "THE-RESEARCH-TEAM-WITH-A-LA… · 12 MONTHS"; heading != want {
		t.Errorf("the numbers' heading reads %q, want %q", heading, want)
	}
	tests := []struct {
		name          string
		ask           views.YearAsk
		page          func() views.YearPage
		apart, period string
		namedFrom     int
	}{
		{name: "an account of a long label", ask: views.YearAsk{Account: "acct-2"}, page: plainYear, apart: " · ", period: "12 MONTHS", namedFrom: 13},
		{name: "a version", ask: views.YearAsk{Version: "Opus 5.5"}, page: opusYear, apart: " · ", period: "12 MONTHS", namedFrom: 13},
		{name: "all use", ask: views.YearAsk{}, page: plainYear, apart: " ", period: "12 MONTHS", namedFrom: 14},
	}
	for _, tt := range tests {
		for width := 8; width <= 30; width++ {
			f := yearFrame(t, width, 60, tt.ask, tt.page())
			f.Graphs.Accounts = long
			y := shownYear(t, f)
			c := newCanvas(width, 1)
			y.heading(c, 0, 0)
			got := textOf(c.rows(Look{}))[0]
			named := strings.HasSuffix(got, tt.apart+tt.period) && !strings.HasPrefix(got, " ")
			alone := got == tt.period || (ansi.StringWidth(tt.period) > width && got == "…")
			if named != (width >= tt.namedFrom) || !named && !alone {
				t.Errorf("%s at %d columns, the heading reads %q; want a name before%s%s from %d columns, else the period alone", tt.name, width, got, tt.apart, tt.period, tt.namedFrom)
			}
		}
	}
}

// The last column of numbers shown takes the row to the frame's width, so
// its figures take their longest forms that fit: in the narrow form at 60 columns,
// some plans unknown, the third column gone, against it reads "3.8× · 2/3",
// where the second column's own width would leave it "3.8×".
func TestTheLastColumnOfNumbersShownTakesTheRow(t *testing.T) {
	page := plainYear()
	page.Numbers.Limits, page.Numbers.Plans = map[string]int{"5h": 412, "7d": 3}, views.PlansKnown{Known: 2, Of: 3}
	f := yearFrame(t, 60, 60, views.YearAsk{}, page)
	rows := yearText(t, f)
	if got, want := rows[yearNumbersRow+3], "limits    week ×3 · 5-hour ×412    against   3.8× · 2/3"; got != want {
		t.Errorf("the limits row reads %q, want %q", got, want)
	}
}

// Where the families' key doesn't fit on one row from the bars' column, it
// flows onto the rows it takes, a family never cut: in the narrow form at 52
// columns, two rows.
func TestTheFamiliesKeyFlowsOntoTheRowsItTakes(t *testing.T) {
	y := shownYear(t, yearFrame(t, 52, 60, views.YearAsk{}, plainYear()))
	rows := textOf(monthsDrawn(y).rows(Look{}))
	want := []string{
		textsAt(byMonthNarrowBarX+17, map[int]string{byMonthNarrowBarX: "● Opus   ● Sonnet"}),
		textsAt(byMonthNarrowBarX+17, map[int]string{byMonthNarrowBarX: "● Fable   ● Haiku"}),
	}
	if got := rows[y.dotsRow():]; !slices.Equal(got, want) {
		t.Errorf("the families' key reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Every account's against it, over the plans known where some aren't, says
// so in the room it has: in full, short, compact, or the ratio alone, never
// cut; and BY MONTH says it once for the table, after what it shows, wide
// where it fits, never of a version's months, whose worth is against no
// plan.
func TestTheYearsAgainstOverThePlansKnownSaysSoInTheRoomItHas(t *testing.T) {
	someKnown := views.Against{Ratio: new(3.8), Cost: usd(7200), Plans: views.PlansKnown{Known: 2, Of: 3}}
	tests := []struct {
		name    string
		against views.Against
		width   int
		want    string
	}{
		{name: "in full", against: someKnown, width: 60, want: "against it   3.8× · 2 of 3 plans known"},
		{name: "short", against: someKnown, width: 37, want: "against it   3.8× · 2 of 3 plans"},
		{name: "compact", against: someKnown, width: 31, want: "against it   3.8× · 2/3"},
		{name: "the ratio alone", against: someKnown, width: 22, want: "against it   3.8×"},
		{name: "every plan known", against: views.Against{Ratio: new(3.8), Cost: usd(7200), Plans: views.PlansKnown{Known: 3, Of: 3}},
			width: 60, want: "against it   3.8×"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := againstFigure(tt.against).drawn("against it", yearLabelCells, tt.width).plain(); got != tt.want {
				t.Errorf("in %d cells, against it reads %q, want %q", tt.width, got, tt.want)
			}
		})
	}

	heads := []struct {
		name  string
		cells int
		ask   views.YearAsk
		page  func() views.YearPage
		want  string
	}{
		{name: "BY MONTH, wide", cells: 80, page: plainYear,
			want: "worth at API prices; against what the plans cost that month · 2 of 3 plans known"},
		{name: "BY MONTH, short", cells: 79, page: plainYear,
			want: "worth at API prices; against what the plans cost that month · 2 of 3 plans"},
		{name: "BY MONTH, compact", cells: 73, page: plainYear,
			want: "worth at API prices; against what the plans cost that month · 2/3"},
		{name: "BY MONTH, without room", cells: 64, page: plainYear, want: "worth at API prices; against what the plans cost that month"},
		{name: "BY MONTH, without room for what it shows", cells: 58, page: plainYear},
		{name: "a version's months", cells: 160, ask: views.YearAsk{Version: "Opus 5.5"}, page: opusYear,
			want: "Opus 5.5 only: its worth at API prices, and its share of the month"},
	}
	for _, tt := range heads {
		t.Run(tt.name, func(t *testing.T) {
			page := tt.page()
			page.Numbers.Plans = views.PlansKnown{Known: 2, Of: 3}
			f := yearFrame(t, 160, 60, tt.ask, page)
			if got := yearOf(f, yearWide).byMonthSaid(tt.cells).plain(); got != tt.want {
				t.Errorf("in %d cells, BY MONTH's heading says %q, want %q", tt.cells, got, tt.want)
			}
		})
	}
}

// Every figure the Year draws is drawn whole, or where it doesn't fit, as an
// ellipsis, never cut part-way, and is numbered only where it's drawn whole;
// its words cut, as the key's, and a version's name in the heading; and BY
// MONTH's bars shrink to the room there is, each against the largest.
func TestTheYearsFiguresAreDrawnWholeOrAsAnEllipsis(t *testing.T) {
	requests := Number{Kind: NumberYear, Of: "requests"}
	tests := []struct {
		name     string
		width    int
		ask      views.YearAsk
		page     views.YearPage
		lead     string
		want     string
		numbered string
	}{
		{name: "requests, fitting", width: 19, page: plainYear(), lead: "requests", want: "requests  236,410", numbered: "236,410"},
		{name: "requests, a cell short", width: 16, page: plainYear(), lead: "requests", want: "requests  …"},
		{name: "a month's worth, a cell short", width: 17, page: plainYear(), lead: " May", want: " May        …"},
		{name: "a bound in the key", width: 19, page: plainYear(), lead: "■ 0", want: "■ 0 ■ <450 ■ …"},
		{name: "the period after a version, its name cut", width: 16, ask: views.YearAsk{Version: "Opus 5.5"}, page: opusYear(), lead: "OPU", want: "OPU… · 12 MONTHS"},
		{name: "a bar in the room left", width: 40, page: plainYear(), lead: " May", want: " May        $2,330    3.9×      ▅▅▅▅"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := yearFrame(t, tt.width, 60, tt.ask, tt.page)
			rows := yearText(t, f)
			i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, tt.lead) })
			if i < 0 || rows[i] != tt.want {
				t.Errorf("at %d columns, the Year reads\n%s\nwant a row %q", tt.width, strings.Join(rows, "\n"), tt.want)
			}
			if got := f.Numbers(threeRouted(), now)[requests]; tt.lead == "requests" && got != tt.numbered {
				t.Errorf("at %d columns, its requests are numbered %q, want %q", tt.width, got, tt.numbered)
			}
		})
	}
	if got := fitParts(11, monthNote(views.MonthRow{From: "2026-03-16"})...).plain(); got != "from the …" {
		t.Errorf("the note after a part month's bar, in 11 cells, reads %q, want its day whole or an ellipsis, %q", got, "from the …")
	}
	c := newCanvas(12, 1)
	monthsBefore(c, 0, views.MonthsBefore{From: "2025-10", To: "2026-05", Then: "Opus 5"}, 12)
	if got := textOf(c.rows(Look{}))[0]; got != "  …" {
		t.Errorf("the row before, in 12 columns, reads %q, want its months whole or an ellipsis, %q", got, "  …")
	}
}
