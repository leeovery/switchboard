package dashboard

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// accountsFramed is an account's row of Accounts' frames, this week, on Max
// 20x at $46 a week where its plan is known, or every account's, id "", on
// three at $138: the sessions moved onto it and off it, its limits, its
// minutes at its cap and at a limit, or none read, what its week left at its
// last reset and the day it ran out, its worth and its ratio, and its
// verdict.
type accountsFramed struct {
	id, label      string
	on, off        int
	limits         map[string]int
	atCap, atLimit int
	left           *float64
	ranOut         string
	worth, ratio   float64
	verdict        string
	weeks, limited int
	leftOnAverage  *float64
	unknownMinutes bool
	planKnown      bool
}

// row is the account's row as the views give it.
func (f accountsFramed) row() views.AccountsRow {
	all := f.id == ""
	totals := views.AccountTotals{
		Account: f.id, All: all, MovedOn: f.on, MovedOff: f.off, Limits: f.limits,
		MinutesAtCap: new(f.atCap), MinutesAtLimit: new(f.atLimit), Amount: usd(f.worth),
	}
	if f.unknownMinutes {
		totals.MinutesAtCap, totals.MinutesAtLimit = nil, nil
	}
	row := views.AccountsRow{
		Label: f.label, Totals: totals, Plan: views.PlanCost{Account: f.id, All: all}, WeekLeft: f.left, RanOut: f.ranOut,
		Verdict: views.AccountVerdict{Account: f.id, All: all, Verdict: f.verdict, Weeks: f.weeks, Limits: f.limited, LeftAtReset: f.leftOnAverage},
	}
	if f.planKnown {
		price, cost := usd(200), usd(46)
		if all {
			price, cost = usd(600), usd(138)
		} else {
			row.Plan.Plan, row.PlanName = "max20x", "Max 20x"
		}
		row.Plan.Price, row.Plan.Against = price, views.Against{Cost: cost, Ratio: new(f.ratio)}
	}
	return row
}

// The rows of Accounts' frames, as Compare draws them on Thursday 1
// October: work, personal and side, and every account's.
var (
	framedWork = accountsFramed{
		id: "work", off: 3, limits: map[string]int{"5h": 2}, atCap: 150, atLimit: 40, left: new(0.0), ranOut: "2026-09-25",
		worth: 184, ratio: 4.0, verdict: views.HitsItsLimit, weeks: 7, limited: 3, leftOnAverage: new(0.09), planKnown: true,
	}
	framedPersonal = accountsFramed{
		id: "personal", off: 1, left: new(0.15),
		worth: 212, ratio: 4.6, verdict: views.EndsWithRoom, weeks: 7, leftOnAverage: new(0.27), planKnown: true,
	}
	framedSide = accountsFramed{
		id: "side", on: 4, limits: map[string]int{"5h": 1}, atCap: 52, left: new(0.40),
		worth: 239, ratio: 5.2, verdict: views.PlentyToSpare, weeks: 7, leftOnAverage: new(0.59), planKnown: true,
	}
	framedAll = accountsFramed{
		on: 4, limits: map[string]int{"5h": 3}, atCap: 202, atLimit: 40,
		worth: 635, ratio: 4.6, verdict: views.RoomToSpare, leftOnAverage: new(0.30), planKnown: true,
	}
)

// framedPage is Accounts' page of the frames, this week, of the rows given,
// the last every account's.
func framedPage(rows ...accountsFramed) views.AccountsPage {
	page := views.AccountsPage{Period: views.AccountsThisWeek}
	for _, f := range rows[:len(rows)-1] {
		page.Rows = append(page.Rows, f.row())
	}
	page.All = rows[len(rows)-1].row()
	return page
}

// figureInk is the ink of an account's figures in Compare, every account's
// bold.
func figureInk(row views.AccountsRow) ink {
	if row.Totals.All {
		return strongInk
	}
	return secondaryInk
}

// figureAt is a figure of a frame's: the parts drawn from column x along row
// y, cut to width.
type figureAt struct {
	x, y, width int
	parts       []part
}

// heldToFigures draws each figure as fitParts cuts it, then holds the cells
// from each to the frame's right edge to the frame's, as nord draws it.
func heldToFigures(t *testing.T, stem string, width, height int, figures []figureAt) {
	t.Helper()
	nord := Screen(builtin(t, "nord"))
	c := newCanvas(width, height)
	for _, f := range figures {
		c.line(f.x, f.y, fitParts(f.width, f.parts...))
	}
	drawn, frame := screenOf(t, c.rows(nord)), reference(t, stem)
	for _, f := range figures {
		heldTo(t, stem, frame, drawn, region{top: f.y, bottom: f.y, left: f.x, right: f.x + f.width - 1}, shadeOf(nord.Canvas()), nil, nil)
	}
}

// Compare's figures are its frame's, cell for cell: ACTIVITY's moves, LIMITS'
// limits, time at a cap or limit and left at the last reset, and VALUE's
// plan, worth and ratio against the plan's price, every account's bold.
func TestComparesFiguresAreItsFramesCells(t *testing.T) {
	page := framedPage(framedWork, framedPersonal, framedSide, framedAll)
	var figures []figureAt
	for i, row := range append(slices.Clone(page.Rows), page.All) {
		x, k := 26+33*i, figureInk(row)
		figures = append(figures,
			figureAt{x, 22, 32, movedParts(row.Totals, k)},
			figureAt{x, 25, 32, limitsParts(row.Totals, status.NameOfKey, k)},
			figureAt{x, 26, 32, atCapParts(row.Totals, k)},
			figureAt{x, 27, 32, leftParts(row, k)},
			figureAt{x, 30, 32, planParts(page, row, k)},
			figureAt{x, 32, 32, worthParts(row, k)},
			figureAt{x, 33, 32, againstParts(page, row)},
		)
	}
	heldToFigures(t, "accounts-compare-160", 160, 37, figures)
}

// A phone's Compare has the short forms: a 5-hour window's limits as 5h,
// what was left at the last reset without the day it ran out, the plan by
// its price alone, and the ratio without the price.
func TestAPhonesCompareHasTheShortFormsOfItsFrame(t *testing.T) {
	page := framedPage(framedWork, framedPersonal, framedSide, framedAll)
	var figures []figureAt
	for i, row := range append(slices.Clone(page.Rows), page.All) {
		k := figureInk(row)
		figures = append(figures,
			figureAt{35, 29 + i, 17, movedParts(row.Totals, k)},
			figureAt{13, 35 + i, 13, limitsParts(row.Totals, shortWindows(status.NameOfKey), k)},
			figureAt{26, 35 + i, 10, atCapParts(row.Totals, k)},
			figureAt{36, 35 + i, 16, leftShort(row, k)},
			figureAt{13, 41 + i, 13, planPriced(row, k)},
			figureAt{26, 41 + i, 10, worthParts(row, k)},
			figureAt{36, 41 + i, 16, againstShort(row)},
		)
	}
	heldToFigures(t, "accounts-phone-compare-52", 52, 48, figures)
}

// Detail's THIS WEEK and verdict are its frames', cell for cell: the limits
// by window, or every account's how many; the plan by its name, or every
// account's by its price; the worth with its ratio; and the verdict, bold in
// its colour, then its record in text.muted, every account's naming the
// accounts that end weeks with the most left and the least.
func TestDetailsThisWeekAndVerdictAreItsFramesCells(t *testing.T) {
	compared := framedPage(framedWork, framedPersonal, framedSide, framedAll)
	limited := framedPersonal
	limited.limits = map[string]int{"5h": 1}
	tests := []struct {
		stem   string
		page   views.AccountsPage
		row    views.AccountsRow
		x, cx  int
		height int
	}{
		{stem: "accounts-detail-work-160", page: compared, row: compared.Rows[0], x: 118, cx: 24},
		{stem: "accounts-detail-all-160", page: compared, row: compared.All, x: 118, cx: 24},
		{stem: "accounts-detail-limit-160", page: compared, row: limited.row(), x: 118, cx: 24},
		{stem: "accounts-detail-sessions-160", page: compared, row: compared.Rows[2], x: 118, cx: 24},
		{stem: "accounts-detail-one-160", page: framedPage(framedWork, framedAll), row: framedWork.row(), x: 110},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			heldToFigures(t, tt.stem, 160, 37, []figureAt{
				{tt.x, 10, 160 - tt.x, limitsParts(tt.row.Totals, status.NameOfKey, secondaryInk)},
				{tt.x, 11, 160 - tt.x, planNamed(tt.row, secondaryInk)},
				{tt.x, 12, 160 - tt.x, worthAgainst(tt.row, secondaryInk)},
				{tt.cx, 25, 160 - tt.cx, verdictParts(tt.page, tt.row)},
			})
		})
	}
}

// A phone's Detail has THIS WEEK in two columns, the worth and its ratio
// apart, and the verdict over two rows, its record without ", on average".
func TestAPhonesDetailHasTheShortFormsOfItsFrame(t *testing.T) {
	row := framedWork.row()
	verdict := verdictShort(row)
	heldToFigures(t, "accounts-phone-detail-52", 52, 41, []figureAt{
		{10, 20, 16, limitsParts(row.Totals, status.NameOfKey, secondaryInk)},
		{10, 21, 16, planNamed(row, secondaryInk)},
		{36, 19, 16, worthParts(row, secondaryInk)},
		{36, 21, 16, againstShort(row)},
		{0, 31, 52, verdict[0]},
		{0, 32, 52, verdict[1]},
	})
}

// Each of Accounts' figures says none as the design does: a dash, or none
// of the limits, in text.faint, bold in Compare's every account; and each
// says what its frames don't show as its rule has it.
func TestAccountsFiguresSayEachForm(t *testing.T) {
	faintBold := faintInk
	faintBold.bold = true
	k := secondaryInk
	both := framedWork
	both.on, both.off = 2, 1
	twoWindows := framedWork
	twoWindows.limits = map[string]int{"7d": 1, "5h": 2}
	unread := framedWork
	unread.unknownMinutes = true
	idle := accountsFramed{id: "spare", left: new(0.15), ranOut: "2026-09-25"}
	twoOfThree := framedPage(framedWork, framedSide, accountsFramed{id: "spare", worth: 12}, framedAll)
	fable := func(key string) string { return map[string]string{"7d_oi": "Fable week"}[key] }
	tests := []struct {
		name  string
		parts []part
		want  line
	}{
		{name: "moves on and off", parts: movedParts(both.row().Totals, k), want: line{{"2 in · 1 out", k}}},
		{name: "no moves", parts: movedParts(idle.row().Totals, k), want: line{{"—", faintInk}}},
		{name: "no moves of every account's", parts: movedParts(accountsFramed{}.row().Totals, strongInk), want: line{{"—", faintBold}}},
		{name: "every account's moves", parts: movedParts(framedAll.row().Totals, strongInk), want: line{{"4 moves", strongInk}}},
		{name: "limits in two windows, the shortest first", parts: limitsParts(twoWindows.row().Totals, status.NameOfKey, k),
			want: line{{"5-hour ×2", errorInk}, {" · ", faintInk}, {"week ×1", errorInk}}},
		{name: "a model's own week's limits, short", parts: limitsParts(views.AccountTotals{Limits: map[string]int{"7d_oi": 1}}, shortWindows(fable), k),
			want: line{{"Fable wk ×1", errorInk}}},
		{name: "no limits", parts: limitsParts(idle.row().Totals, status.NameOfKey, k), want: line{{"none", faintInk}}},
		{name: "no limits of every account's", parts: limitsParts(accountsFramed{}.row().Totals, status.NameOfKey, strongInk), want: line{{"none", faintBold}}},
		{name: "no time at a cap or a limit", parts: atCapParts(idle.row().Totals, k), want: line{{"—", faintInk}}},
		{name: "time at a cap or a limit never read", parts: atCapParts(unread.row().Totals, k), want: line{{"—", faintInk}}},
		{name: "left at a reset, not run out", parts: leftParts(framedPersonal.row(), k), want: line{{"15%", k}}},
		{name: "left at a reset, run out", parts: leftParts(framedWork.row(), k), want: line{{"0% (limit Fri)", k}}},
		{name: "no week ended", parts: leftParts(framedWork.withoutWeek().row(), k), want: line{{"—", faintInk}}},
		{name: "every account's left at a reset", parts: leftParts(framedAll.row(), strongInk), want: line{{"—", faintBold}}},
		{name: "a plan not known", parts: planParts(twoOfThree, twoOfThree.Rows[2], k), want: line{{"—", faintInk}}},
		{name: "every account's plans, some not known", parts: planParts(twoOfThree, twoOfThree.All, strongInk),
			want: line{{"$600/mo", strongInk}, {" · 2 of 3 plans", dimInk}}},
		{name: "a plan not known, by its name", parts: planNamed(twoOfThree.Rows[2], k), want: line{{"—", faintInk}}},
		{name: "a plan not known, by its price", parts: planPriced(twoOfThree.Rows[2], k), want: line{{"—", faintInk}}},
		{name: "no ratio against a plan not known", parts: againstParts(twoOfThree, twoOfThree.Rows[2]), want: line{{"—", faintInk}}},
		{name: "no ratio, short", parts: againstShort(twoOfThree.Rows[2]), want: line{{"—", faintInk}}},
		{name: "a worth without a plan known", parts: worthAgainst(twoOfThree.Rows[2], k), want: line{{"$12", k}}},
		{name: "a ratio under 1×", parts: againstParts(twoOfThree, accountsFramed{id: "spare", worth: 20, ratio: 0.4, planKnown: true}.row()),
			want: line{{"0.4×", alertInk}, {" its $46 week", dimInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitParts(200, tt.parts...).inked(); !slices.Equal(got, tt.want) {
				t.Errorf("said %+v, want %+v", got, tt.want)
			}
		})
	}
}

// withoutWeek is the row with none of its weeks ended in the six weeks.
func (f accountsFramed) withoutWeek() accountsFramed {
	f.left, f.ranOut = nil, ""
	return f
}

// The verdict says not enough weeks yet where no week can be judged; and
// every account's names the one account whose weeks are known alone, and
// none where none's are; little to spare together in accent.attention.
func TestTheVerdictSaysEachForm(t *testing.T) {
	unjudged := func(f accountsFramed) accountsFramed {
		f.verdict, f.weeks, f.limited, f.leftOnAverage = "", 0, 0, nil
		return f
	}
	little := framedAll
	little.verdict, little.leftOnAverage = views.LittleToSpare, new(0.12)
	oneKnown := framedPage(framedWork, unjudged(framedSide), framedAll)
	byLabel := func(f accountsFramed, id string) accountsFramed {
		f.id, f.label = id, f.id
		return f
	}
	labelled := framedPage(byLabel(framedWork, "acct-1"), byLabel(framedSide, "acct-2"), framedAll)
	noneKnown := framedPage(unjudged(framedWork), unjudged(framedSide), framedAll)
	notEnough := line{{views.NotEnoughWeeks, dimInk}}
	tests := []struct {
		name  string
		parts []part
		want  line
	}{
		{name: "an account with no week judged", parts: verdictParts(noneKnown, noneKnown.Rows[0]), want: notEnough},
		{name: "every account with no week judged", parts: verdictParts(noneKnown, unjudged(framedAll).row()), want: notEnough},
		{name: "every account, one account's weeks known", parts: verdictParts(oneKnown, oneKnown.All),
			want: line{{views.RoomToSpare, boldPositiveInk}, {"   30% left at a reset, on average · work ends weeks with 9% left", mutedInk}}},
		{name: "every account, its accounts labelled apart from their ids", parts: verdictParts(labelled, labelled.All),
			want: line{{views.RoomToSpare, boldPositiveInk}, {"   30% left at a reset, on average · side ends weeks with 59% left, work with 9%", mutedInk}}},
		{name: "every account, none's weeks known", parts: verdictParts(noneKnown, noneKnown.All),
			want: line{{views.RoomToSpare, boldPositiveInk}, {"   30% left at a reset, on average", mutedInk}}},
		{name: "every account, little to spare", parts: verdictParts(noneKnown, little.row()),
			want: line{{views.LittleToSpare, alertInk}, {"   12% left at a reset, on average", mutedInk}}},
		{name: "an account's verdict without a share left", parts: verdictParts(oneKnown, accountsFramed{id: "work", verdict: views.CouldGo, weeks: 7}.row()),
			want: line{{views.CouldGo, alertInk}, {"   limit hit 0 of the last 7 weeks", mutedInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitParts(200, tt.parts...).inked(); !slices.Equal(got, tt.want) {
				t.Errorf("said %+v, want %+v", got, tt.want)
			}
		})
	}
	short := func(row views.AccountsRow) []line {
		var got []line
		for _, parts := range verdictShort(row) {
			got = append(got, fitParts(52, parts...).inked())
		}
		return got
	}
	if got, want := short(framedAll.row()), []line{{{views.RoomToSpare, boldPositiveInk}}, {{"30% left at a reset", mutedInk}}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("every account's short verdict is %+v, want %+v", got, want)
	}
	if got, want := short(unjudged(framedWork).row()), []line{notEnough}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("an account's short verdict with no week judged is %+v, want %+v", got, want)
	}
}

// extraOf is an account with extra usage given as the status given: off,
// rejected; on, any other; and not read, none given.
func extraOf(id string, given quota.Status) status.AccountNow {
	return status.AccountNow{ID: id, Extra: quota.ExtraUsage{Status: given}}
}

// Extra usage is its frames', cell for cell: each account's off in Compare's
// VALUE, and every account's, bold, off on all, its column running to the
// frame's edge; and in Detail's THIS WEEK and a phone's Detail, of the
// account chosen or every account, off.
func TestExtraUsageIsItsFramesCells(t *testing.T) {
	off := quota.StatusRejected
	three := []status.AccountNow{extraOf("work", off), extraOf("personal", off), extraOf("side", off)}
	ten := slices.Repeat(three[:1], 10)
	compared := func(accounts []status.AccountNow, shown, colw int) []figureAt {
		var figures []figureAt
		for i, a := range accounts[:shown] {
			figures = append(figures, figureAt{26 + colw*i, 31, colw, extraParts(a, secondaryInk)})
		}
		x := 26 + colw*shown
		return append(figures, figureAt{x, 31, 160 - x, extraOfAll(status.ExtraOfAll(accounts), strongInk)})
	}
	thisWeek := func(x int, accounts ...status.AccountNow) []figureAt {
		return []figureAt{{x, 13, 160 - x, extraShort(status.ExtraOfAll(accounts))}}
	}
	tests := []struct {
		stem          string
		width, height int
		figures       []figureAt
	}{
		{stem: "accounts-compare-160", width: 160, height: 37, figures: compared(three, 3, 33)},
		{stem: "accounts-compare-ten-160", width: 160, height: 37, figures: compared(ten, 6, 19)},
		{stem: "accounts-detail-all-160", width: 160, height: 37, figures: thisWeek(118, three...)},
		{stem: "accounts-detail-work-160", width: 160, height: 37, figures: thisWeek(118, three[0])},
		{stem: "accounts-detail-one-160", width: 160, height: 37, figures: thisWeek(110, three[0])},
		{stem: "accounts-phone-detail-52", width: 52, height: 41, figures: []figureAt{{36, 20, 16, extraShort(status.ExtraOfAll(three[:1]))}}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			heldToFigures(t, tt.stem, tt.width, tt.height, tt.figures)
		})
	}
}

// A figure of the period is said as its words say it while the period
// records anything, and a dash in text.faint, bold where all accounts' is,
// where it records nothing, as on a fresh install, whatever its words.
func TestAPeriodsFigureIsADashWhereThePeriodRecordsNothing(t *testing.T) {
	said := []part{figureIn("0", strongInk)}
	tests := []struct {
		name     string
		recorded bool
		k        ink
		want     line
	}{
		{name: "recorded", recorded: true, k: strongInk, want: line{{"0", strongInk}}},
		{name: "nothing recorded", k: secondaryInk, want: line{noFigure}},
		{name: "nothing recorded, all accounts'", k: strongInk, want: line{{"—", ink{token: theme.TextFaint, bold: true}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := views.AccountsPage{Recorded: tt.recorded}
			if got := fitParts(20, periodParts(page, said, tt.k)...).inked(); !slices.Equal(got, tt.want) {
				t.Errorf("said %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Extra usage says each form as the design does: an account's on in
// accent.attention, billed past its limit, read so where only its use is
// given; every account's on on all, or on for how many of them, those not
// read counted among them; off where none read is on; and a dash where none
// is read. Detail's short forms say on and off alone.
func TestExtraUsageSaysEachForm(t *testing.T) {
	on, off, unread := quota.StatusAllowed, quota.StatusRejected, quota.Status("")
	used := status.AccountNow{ID: "side", Extra: quota.ExtraUsage{Utilization: new(0.2)}}
	mixed := []status.AccountNow{extraOf("work", on), extraOf("personal", off), extraOf("side", unread)}
	boldMuted, boldWarning, faintBold := mutedInk, warningInk, faintInk
	boldMuted.bold, boldWarning.bold, faintBold.bold = true, true, true
	tests := []struct {
		name  string
		parts []part
		want  line
	}{
		{name: "an account's off", parts: extraParts(extraOf("work", off), secondaryInk), want: line{{"off", mutedInk}}},
		{name: "an account's on", parts: extraParts(extraOf("work", on), secondaryInk), want: line{{"on · billed past its limit", warningInk}}},
		{name: "an account's on, its use alone given", parts: extraParts(used, secondaryInk), want: line{{"on · billed past its limit", warningInk}}},
		{name: "an account's not read", parts: extraParts(extraOf("work", unread), secondaryInk), want: line{{"—", faintInk}}},
		{name: "every account on", parts: extraOfAll(status.ExtraOfAll([]status.AccountNow{extraOf("work", on), used}), strongInk), want: line{{"on on all", boldWarning}}},
		{name: "every account, one of three on", parts: extraOfAll(status.ExtraOfAll(mixed), strongInk), want: line{{"on for 1 of 3", boldWarning}}},
		{name: "every account read off, one not read", parts: extraOfAll(status.ExtraOfAll(mixed[1:]), strongInk), want: line{{"off on all", boldMuted}}},
		{name: "every account not read", parts: extraOfAll(status.ExtraOfAll(mixed[2:]), strongInk), want: line{{"—", faintBold}}},
		{name: "short, an account on", parts: extraShort(status.ExtraOfAll(mixed[:1])), want: line{{"on", warningInk}}},
		{name: "short, every account, one of three on", parts: extraShort(status.ExtraOfAll(mixed)), want: line{{"on for 1 of 3", warningInk}}},
		{name: "short, every account read off", parts: extraShort(status.ExtraOfAll(mixed[1:])), want: line{{"off", mutedInk}}},
		{name: "short, none read", parts: extraShort(status.ExtraOfAll(mixed[2:])), want: line{{"—", faintInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitParts(200, tt.parts...).inked(); !slices.Equal(got, tt.want) {
				t.Errorf("said %+v, want %+v", got, tt.want)
			}
		})
	}
}
