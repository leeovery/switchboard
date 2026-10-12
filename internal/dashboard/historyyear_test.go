package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// usd is an amount of dollars, as the ledger counts money.
func usd(dollars float64) *ledger.Picodollars {
	p := ledger.Picodollars(dollars * float64(aDollar))
	return &p
}

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

// The frames' world: three accounts on Max 20x, $600 a month, the months'
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

// framedYear is the Year's page a frame of it draws, as the design holds it,
// in the look given: its grid, 53 weeks from the frames' first, the last
// shown of them from column x and row top, each day that shows at the level
// whose shade the frame draws it in, as square gives each level's, this
// week's days to come left out; and BY MONTH's rows, each split by family
// as the frame's bar is, from column barX and the row first.
func framedYear(t *testing.T, look Look, stem string, square func(level int) ink, x, top, shown int, months []framedMonth, barX, first int) views.YearPage {
	t.Helper()
	frame := reference(t, stem)
	shades := make(map[shade]int)
	for level := range shadeLevels {
		shades[shadeOf(look.colour(square(level)))] = level
	}
	shownFrom := 53 - shown
	page := views.YearPage{Weeks: make([]views.GridWeek, 53), Months: framesLabels}
	for w := range page.Weeks {
		days := make([]views.GridDay, 7)
		for i := range days {
			days[i].Day = framesFirstWeek.AddDate(0, 0, 7*w+i).Format(time.DateOnly)
			days[i].LeftOut = w == 52 && i > 3
			if w >= shownFrom && !days[i].LeftOut {
				days[i].Level = shades[seenAt(frame, x+2*(w-shownFrom), top+1+i).fg]
			}
		}
		page.Weeks[w] = views.GridWeek{First: days[0].Day, Days: days}
	}
	families := make(map[shade]string)
	for _, family := range []string{"opus", "sonnet", "fable", "haiku"} {
		families[shadeOf(look.colour(familyHue(family).ink()))] = family
	}
	for i, m := range months {
		page.ByMonth = append(page.ByMonth, monthOf(m, frame, families, barX, first+i))
	}
	return page
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

// yearFrame is a frame of the Year shown, in nord, of a terminal the size
// given, asked for as given, with the page given, its windows named as
// claude names them.
func yearFrame(t *testing.T, width, height int, ask views.YearAsk, page views.YearPage) Frame {
	t.Helper()
	f := frameOf(width, height)
	f.View, f.Look = History, Screen(builtin(t, "nord"))
	f.History = HistoryPage{Read: true, YearAsk: ask, Year: &page}
	f.WindowName = func(key string) string { return map[string]string{"5h": "5-hour", "7d": "Week"}[key] }
	return f
}

// drawnYear is the Year as the frame draws it alone, from its view's top, as
// a terminal shows it.
func drawnYear(t *testing.T, f Frame) [][]seen {
	t.Helper()
	c := newCanvas(f.Width, f.Height)
	f.historyYear(c, f.top())
	return screenOf(t, c.rows(f.Look))
}

// The Year is drawn as its frames are, cell for cell, but where they light
// September, the frames' month before, where the design lights today's,
// October: its name over the grid and bold in BY MONTH.
func TestTheYearIsDrawnAsItsFrames(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	opus, _ := ledger.Pricing.Version("claude-opus-5-5")
	steps := func(level int) ink { return stepInk(versionHue(opus), level) }
	tests := []struct {
		stem          string
		width, height int
		ask           views.YearAsk
		page          func() views.YearPage
		in            region
		current       []spot
	}{
		{
			stem: "history-year-160", width: 160, height: 37,
			page: func() views.YearPage {
				page := framedYear(t, nord, "history-year-160", dayInk, yearGridX, 6, 53, framesMonths, byMonthBarX, 19)
				page.KeyCuts, page.Numbers, page.Families = []int{450, 750, 1000}, framesNumbers, []string{"opus", "sonnet", "fable", "haiku"}
				return page
			},
			in:      region{top: 6, bottom: 34, left: 0, right: 159},
			current: []spot{{row: 6, first: 110, last: 120}, {row: 30, first: 2, last: 4}, {row: 31, first: 2, last: 4}},
		},
		{
			stem: "history-year-opus-160", width: 160, height: 37, ask: views.YearAsk{Version: "Opus 5.5"},
			page: func() views.YearPage {
				page := framedYear(t, nord, "history-year-opus-160", steps, yearGridX, 6, 53, opusMonths, byMonthBarX, 20)
				page.KeyCuts, page.Version = []int{400, 600, 700}, opus
				page.Numbers = views.YearNumbers{
					Requests: 48880, Sessions: 352, Amount: usd(5760), Share: new(0.21), FirstUsed: "2026-06-22", BusiestDay: "2026-09-22",
				}
				page.Before = &views.MonthsBefore{From: "2025-10", To: "2026-05", Then: "Opus 5"}
				return page
			},
			in:      region{top: 6, bottom: 34, left: 0, right: 159},
			current: []spot{{row: 6, first: 110, last: 120}, {row: 23, first: 2, last: 4}, {row: 24, first: 2, last: 4}},
		},
		{
			stem: "history-phone-year-52", width: 52, height: 35,
			page: func() views.YearPage {
				page := framedYear(t, nord, "history-phone-year-52", dayInk, yearPhoneGridX, 8, yearPhoneWeeks, framesMonths, byMonthPhoneBarX, 25-7)
				page.KeyCuts, page.Numbers = []int{450, 750, 1000}, framesNumbers
				return page
			},
			in:      region{top: 8, bottom: 33, left: 0, right: 51},
			current: []spot{{row: 8, first: 38, last: 48}, {row: 29, first: 1, last: 3}, {row: 30, first: 1, last: 3}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := yearFrame(t, tt.width, tt.height, tt.ask, tt.page())
			heldTo(t, tt.stem, reference(t, tt.stem), drawnYear(t, f), tt.in, shadeOf(nord.Canvas()), tt.current, nil)
		})
	}
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

// yearCanvas is the Year of the frame drawn alone, from its view's top.
func yearCanvas(f Frame) *canvas {
	c := newCanvas(f.Width, f.Height)
	f.historyYear(c, f.top())
	return c
}

// Today's month is the current one, its name over the grid in
// text.secondary, the rest faint, and its row of BY MONTH bold.
func TestTheCurrentMonthIsLitOverTheGridAndBoldInByMonth(t *testing.T) {
	tests := []struct {
		width                      int
		october, september, name   int
		septembersRow, octobersRow int
	}{
		{width: 160, october: 118, september: 110, name: byMonthNameX, septembersRow: yearByMonthRow + 2 + 11, octobersRow: yearByMonthRow + 2 + 12},
		{width: 52, october: 46, september: 38, name: byMonthPhoneNameX, septembersRow: yearPhoneByMonthRow + 5, octobersRow: yearPhoneByMonthRow + 6},
	}
	for _, tt := range tests {
		f := yearFrame(t, tt.width, 37, views.YearAsk{}, plainYear())
		c, top := yearCanvas(f), f.top()
		inks := []struct {
			what string
			x, y int
			want ink
		}{
			{"October over the grid", tt.october, top, secondaryInk},
			{"September over the grid", tt.september, top, faintInk},
			{"October in BY MONTH", tt.name, top + tt.octobersRow, strongInk},
			{"September in BY MONTH", tt.name, top + tt.septembersRow, secondaryInk},
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
		f := yearFrame(t, 160, 37, views.YearAsk{}, page)
		f.Look = look
		rows, top := textOf(yearCanvas(f).rows(Look{})), f.top()
		if got, want := []rune(rows[top+1])[yearGridX], '·'; got != want {
			t.Errorf("a day at level 0 is %q, want %q", got, want)
		}
		if got, want := []rune(rows[top+5])[yearGridX], '█'; got != want {
			t.Errorf("a day at level 4 is %q, want %q", got, want)
		}
		if got, want := rows[top+yearKeyRow], textsAt(160, map[int]string{yearGridX: "· none   ░ under 450   ▒ 450–750   ▓ 750–1,000   █ over 1,000  requests a day"}); got != want {
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
			f := yearFrame(t, 160, 37, tt.ask, tt.page)
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
	f := yearFrame(t, 160, 37, views.YearAsk{}, page)
	c, top := yearCanvas(f), f.top()
	rows := textOf(c.rows(Look{}))
	for i, label := range []string{"requests", "sessions", "limits hit", "worth", "plans cost", "against it", "busiest day", "longest run"} {
		if got, want := rows[top+1+i], textsAt(160, map[int]string{yearNumbersX: padded(label, yearLabelCells) + "—"}); !strings.HasSuffix(got, want[yearNumbersX:]) {
			t.Errorf("%s reads %q, want a dash", label, got)
		}
		if got := c.at(yearNumbersX+yearLabelCells, top+1+i).ink; got != faintInk {
			t.Errorf("%s's dash is in %+v, want text.faint", label, got)
		}
	}
	if got, want := rows[top+yearKeyRow], textsAt(160, map[int]string{yearGridX: "■ none  requests a day"}); got != want {
		t.Errorf("the key reads %q, want %q", got, want)
	}
}

// The numbers are headed by their period, the last 12 months or a calendar
// year, after the use shown alone; and the key's unit names it.
func TestTheNumbersAndTheKeyNameWhatsShown(t *testing.T) {
	tests := []struct {
		ask           views.YearAsk
		heading, unit string
	}{
		{ask: views.YearAsk{}, heading: "LAST 12 MONTHS", unit: "requests a day"},
		{ask: views.YearAsk{Year: 2025}, heading: "2025", unit: "requests a day"},
		{ask: views.YearAsk{Version: "Opus 5.5"}, heading: "OPUS 5.5 · 12 MONTHS", unit: "Opus 5.5 requests a day"},
		{ask: views.YearAsk{Year: 2025, Version: "Opus 5.5"}, heading: "OPUS 5.5 · 2025", unit: "Opus 5.5 requests a day"},
		{ask: views.YearAsk{Account: "work"}, heading: "WORK · 12 MONTHS", unit: "work requests a day"},
	}
	for _, tt := range tests {
		f := yearFrame(t, 160, 37, tt.ask, plainYear())
		rows, top := textOf(yearCanvas(f).rows(Look{})), f.top()
		if got := strings.TrimSpace(string([]rune(rows[top])[yearNumbersX:])); got != tt.heading {
			t.Errorf("%+v: the numbers are headed %q, want %q", tt.ask, got, tt.heading)
		}
		if got := rows[top+yearKeyRow]; !strings.HasSuffix(got, "  "+tt.unit) {
			t.Errorf("%+v: the key reads %q, want it to end %q", tt.ask, got, tt.unit)
		}
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
		if got := barCells(*usd(tt.worth), *usd(tt.largest), tt.most); got != tt.want {
			t.Errorf("barCells($%v, $%v, %d) = %d, want %d", tt.worth, tt.largest, tt.most, got, tt.want)
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
		if got := monthNote(tt.row); got != tt.want {
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
		monthsBefore(c, 0, tt.before)
		if got := textOf(c.rows(Look{}))[0]; got != tt.want {
			t.Errorf("the row before reads %q, want %q", got, tt.want)
		}
		if got := c.at(byMonthWorthX, 0).ink; got != faintInk {
			t.Errorf("the row before is in %+v, want text.faint", got)
		}
	}
}

// Against its price is bold, in state.positive at 1× or more as it's
// written, and accent.attention under; a dash where there's none. A phone's
// key writes a thousand as k.
func TestAgainstItsPriceIsColouredAsItsWritten(t *testing.T) {
	tests := []struct {
		ratio *float64
		want  line
	}{
		{ratio: new(3.8166), want: line{{"3.8×", boldPositiveInk}}},
		{ratio: new(0.96), want: line{{"1.0×", boldPositiveInk}}},
		{ratio: new(0.94), want: line{{"0.9×", alertInk}}},
		{want: line{noFigure}},
	}
	for _, tt := range tests {
		if got := againstSaid(tt.ratio); !slices.Equal(got, tt.want) {
			t.Errorf("againstSaid(%v) = %+v, want %+v", tt.ratio, got, tt.want)
		}
	}
	for n, want := range map[int]string{450: "450", 1000: "1k", 1250: "1.25k", 12000: "12k"} {
		if got := kilo(n); got != want {
			t.Errorf("kilo(%d) = %q, want %q", n, got, want)
		}
	}
}

// A worth is written to the dollar, its thousands set apart: a faint +
// after it where part is unpriced, unpriced where all is, and a dash for
// none.
func TestAWorthIsWrittenToTheDollar(t *testing.T) {
	tests := []struct {
		worth views.Worth
		want  line
	}{
		{worth: views.Worth{Amount: usd(27479.5)}, want: line{{"$27,480", secondaryInk}}},
		{worth: views.Worth{Amount: usd(184), Unpriced: []string{"claude-opus-9"}}, want: line{{"$184", secondaryInk}, {"+", faintInk}}},
		{worth: views.Worth{Unpriced: []string{"claude-opus-9"}}, want: line{{"unpriced", secondaryInk}}},
		{worth: views.Worth{}, want: line{noFigure}},
	}
	for _, tt := range tests {
		if got := worthSaid(tt.worth); !slices.Equal(got, tt.want) {
			t.Errorf("worthSaid(%+v) = %+v, want %+v", tt.worth, got, tt.want)
		}
	}
}
