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
	countPlans(&page)
	return page
}

// countPlans counts the plans known of the page's accounts on every
// account's row, as the views count them.
func countPlans(page *views.AccountsPage) {
	page.All.Plan.Plans = views.PlansKnown{Of: len(page.Rows)}
	for _, row := range page.Rows {
		if row.Plan.Plan != "" {
			page.All.Plan.Plans.Known++
		}
	}
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
		{name: "limits in two windows, the week first", parts: limitsParts(twoWindows.row().Totals, status.NameOfKey, k),
			want: line{{"week ×1", errorInk}, {" · ", faintInk}, {"5-hour ×2", errorInk}}},
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
		{name: "a plan not known", parts: planParts(twoOfThree.Rows[2], k), want: line{{"—", faintInk}}},
		{name: "every account's plans, some not known", parts: planParts(twoOfThree.All, strongInk),
			want: line{{"$600/mo", strongInk}, {" · 2 of 3 plans", dimInk}}},
		{name: "a plan not known, by its name", parts: planNamed(twoOfThree.Rows[2], k), want: line{{"—", faintInk}}},
		{name: "a plan not known, by its price", parts: planPriced(twoOfThree.Rows[2], k), want: line{{"—", faintInk}}},
		{name: "no ratio against a plan not known", parts: againstParts(twoOfThree, twoOfThree.Rows[2], 200), want: line{{"—", faintInk}}},
		{name: "every account's ratio, over the plans known", parts: againstParts(twoOfThree, twoOfThree.All, 200),
			want: line{{"4.6×", boldPositiveInk}, {" its $138 week · 2 of 3 plans known", dimInk}}},
		{name: "every account's ratio, over the plans known, short where that doesn't fit", parts: againstParts(twoOfThree, twoOfThree.All, 34),
			want: line{{"4.6×", boldPositiveInk}, {" its $138 week · 2 of 3 plans", dimInk}}},
		{name: "every account's ratio, over the plans known, compact where neither fits", parts: againstParts(twoOfThree, twoOfThree.All, 30),
			want: line{{"4.6×", boldPositiveInk}, {" its $138 week · 2/3", dimInk}}},
		{name: "every account's ratio, over the plans known, the ratio alone where none fits", parts: againstParts(twoOfThree, twoOfThree.All, 23),
			want: line{{"4.6×", boldPositiveInk}, {" its $138 week", dimInk}}},
		{name: "every account's ratio, every plan known", parts: againstParts(framedPage(framedWork, framedAll), framedAll.row(), 200),
			want: line{{"4.6×", boldPositiveInk}, {" its $138 week", dimInk}}},
		{name: "no ratio, short", parts: againstShort(twoOfThree.Rows[2]), want: line{{"—", faintInk}}},
		{name: "no ratio, on a phone", parts: againstOnPhone(twoOfThree.Rows[2]), want: line{{"—", faintInk}}},
		{name: "every account's ratio on a phone, over the plans known", parts: againstOnPhone(twoOfThree.All),
			want: line{{"4.6×", boldPositiveInk}, {" · 2/3", dimInk}}},
		{name: "every account's ratio on a phone, every plan known", parts: againstOnPhone(framedAll.row()),
			want: line{{"4.6×", boldPositiveInk}}},
		{name: "a ratio under 1×", parts: againstParts(twoOfThree, accountsFramed{id: "spare", worth: 20, ratio: 0.4, planKnown: true}.row(), 200),
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
// read counted among them; off on all where every one is read off; and a
// dash where none is read on and one isn't read. Detail's short forms say on
// and off alone.
func TestExtraUsageSaysEachForm(t *testing.T) {
	on, off, unread := quota.StatusAllowed, quota.StatusRejected, quota.Status("")
	used := status.AccountNow{ID: "side", Extra: quota.ExtraUsage{Utilization: new(0.2)}}
	mixed := []status.AccountNow{extraOf("work", on), extraOf("personal", off), extraOf("side", unread)}
	allOff := []status.AccountNow{extraOf("work", off), extraOf("personal", off)}
	boldMuted, boldWarning, faintBold := mutedInk, warningInk, faintInk
	boldMuted.bold, boldWarning.bold, faintBold.bold = true, true, true
	tests := []struct {
		name  string
		parts []part
		want  line
	}{
		{name: "an account's off", parts: extraForms(extraOf("work", off), secondaryInk)[0], want: line{{"off", mutedInk}}},
		{name: "an account's on", parts: extraForms(extraOf("work", on), secondaryInk)[0], want: line{{"on · billed past its limit", warningInk}}},
		{name: "an account's on, its use alone given", parts: extraForms(used, secondaryInk)[0], want: line{{"on · billed past its limit", warningInk}}},
		{name: "an account's not read", parts: extraForms(extraOf("work", unread), secondaryInk)[0], want: line{{"—", faintInk}}},
		{name: "every account on", parts: extraOfAllForms(status.ExtraOfAll([]status.AccountNow{extraOf("work", on), used}), strongInk)[0], want: line{{"on on all", boldWarning}}},
		{name: "every account, one of three on", parts: extraOfAllForms(status.ExtraOfAll(mixed), strongInk)[0], want: line{{"on for 1 of 3", boldWarning}}},
		{name: "every account read off", parts: extraOfAllForms(status.ExtraOfAll(allOff), strongInk)[0], want: line{{"off on all", boldMuted}}},
		{name: "those read off, one not read", parts: extraOfAllForms(status.ExtraOfAll(mixed[1:]), strongInk)[0], want: line{{"—", faintBold}}},
		{name: "every account not read", parts: extraOfAllForms(status.ExtraOfAll(mixed[2:]), strongInk)[0], want: line{{"—", faintBold}}},
		{name: "Compare's shortest, an account on", parts: extraForms(extraOf("work", on), secondaryInk).shortest(), want: line{{"on", warningInk}}},
		{name: "Compare's shortest, every account on", parts: extraOfAllForms(status.ExtraOfAll([]status.AccountNow{extraOf("work", on), used}), strongInk).shortest(), want: line{{"on", boldWarning}}},
		{name: "Compare's shortest, every account read off", parts: extraOfAllForms(status.ExtraOfAll(allOff), strongInk).shortest(), want: line{{"off", boldMuted}}},
		{name: "Compare's shortest, one of three on", parts: extraOfAllForms(status.ExtraOfAll(mixed), strongInk).shortest(), want: line{{"on for 1 of 3", boldWarning}}},
		{name: "short, an account on", parts: extraShort(status.ExtraOfAll(mixed[:1])), want: line{{"on", warningInk}}},
		{name: "short, every account, one of three on", parts: extraShort(status.ExtraOfAll(mixed)), want: line{{"on for 1 of 3", warningInk}}},
		{name: "short, every account read off", parts: extraShort(status.ExtraOfAll(allOff)), want: line{{"off", mutedInk}}},
		{name: "short, those read off, one not read", parts: extraShort(status.ExtraOfAll(mixed[1:])), want: line{{"—", faintInk}}},
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
