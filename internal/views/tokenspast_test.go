package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// usedBackFromTheFrames are the days from Tuesday 1 October 2024 to the
// frames' today, Thursday 1 October 2026, work's use on each day given: the
// first and last days of a past period one step back from today, and the
// days either side of it, for each length but today, with a day between.
var usedBackFromTheFrames = daysTo("2024-10-01", framesNow,
	usedOn("2024-10-01"), usedOn("2024-10-02"), usedOn("2025-03-15"), usedOn("2025-10-01"), usedOn("2025-10-02"),
	usedOn("2026-04-19"), usedOn("2026-04-20"), usedOn("2026-05-20"), usedOn("2026-07-12"), usedOn("2026-07-13"),
	usedOn("2026-08-02"), usedOn("2026-08-03"), usedOn("2026-08-20"), usedOn("2026-09-01"), usedOn("2026-09-02"),
	usedOn("2026-09-20"), usedOn("2026-09-21"), usedOn("2026-09-27"), usedOn("2026-09-28"),
)

// usedOn is the day with the given date, work having used Claude Opus 5.5
// on it.
func usedOn(date string) ledger.Summary {
	return day(date, account("work", sent("claude-opus-5-5", 1, inputUsage)))
}

// tokensRowsSaid are a Tokens page's rows as a test reads them: each row's
// first day, or month, what its plans cost and whether it's so far.
type tokensRowsSaid struct {
	first []string
	costs []string
	soFar []bool
}

// tokensRowsSaidOf are the page's rows as a test reads them.
func tokensRowsSaidOf(t *testing.T, page views.TokensPage) tokensRowsSaid {
	t.Helper()
	var said tokensRowsSaid
	for _, p := range page.Periods {
		said.first, said.costs, said.soFar = append(said.first, p.First), append(said.costs, jsonOf(t, p.Cost)), append(said.soFar, p.SoFar)
	}
	return said
}

// A past period's rows run over its own days, stopping at its last: a day
// each over 30 days and last week, a week over 12 weeks, whole weeks to its
// last, and a month over 12 months, each priced over its period as now
// prices it, work's $200 and side's $20. A past 12 months' oldest month is
// priced by its days from its first, 30 of October 2024, and its last by its
// days to its last, 1 of October 2025, never whole. Its total begins on its
// first day, and nothing in it is so far, as it holds no today.
func TestTokensOverAPastPeriodAreItsOwnRowsNoneSoFar(t *testing.T) {
	aDay, aWeek := "7.232876712328", "50.769230769231"
	tests := []struct {
		length views.Length
		name   string
		first  []string
		costs  []string
		total  string
	}{
		{
			length: views.LengthThisWeek, name: "last week",
			first: []string{"2026-09-27", "2026-09-21"}, costs: []string{aDay, aDay}, total: "2026-09-21",
		},
		{
			length: views.LengthThirtyDays, name: "30 days to 1 Sep",
			first: []string{"2026-09-01", "2026-08-20", "2026-08-03"}, costs: []string{aDay, aDay, aDay}, total: "2026-08-03",
		},
		{
			length: views.LengthTwelveWeeks, name: "12 weeks to 12 Jul",
			first: []string{"2026-07-06", "2026-05-18", "2026-04-20"}, costs: []string{aWeek, aWeek, aWeek}, total: "2026-04-20",
		},
		{
			length: views.LengthTwelveMonths, name: "12 months to 1 Oct 2025",
			first: []string{"2025-10", "2025-03", "2024-10"}, costs: []string{aDay, "220", "216.986301369863"}, total: "2024-10-02",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pressed(t, tt.length, "<", dayfile.CivilOf(framesNow), time.Monday, time.Time{})
			if got := views.DialOf(p, framesNow, time.Monday, time.Time{}).Name(); got != tt.name {
				t.Fatalf("%s one back is %q, want %q", tt.length.Name(), got, tt.name)
			}
			in := input(usedBackFromTheFrames)
			in.Now, in.From = framesNow, on("2024-10-01")
			page := views.NewPast(in, usedBackFromTheFrames).TokensOver(p)
			got := tokensRowsSaidOf(t, page)
			if !slices.Equal(got.first, tt.first) || !slices.Equal(got.costs, tt.costs) {
				t.Errorf("%s's rows are %q, costing %v; want %q, costing %v", tt.name, got.first, got.costs, tt.first, tt.costs)
			}
			if slices.Contains(got.soFar, true) {
				t.Errorf("%s's rows are so far %v, want none", tt.name, got.soFar)
			}
			if page.Total == nil || page.Total.First != tt.total || page.Total.SoFar {
				t.Errorf("%s's total is %+v, want the period from %s, not so far", tt.name, page.Total, tt.total)
			}
		})
	}
}

// A past period whose last row is the month today is in never has it so
// far: on Wednesday 7 October, last week's Sunday, 4 October, lengthened to
// 12 months, is the 12 months to 4 Oct, its October priced by its 4 days to
// then, not whole.
func TestTokensOverAPastPeriodEndingInTodaysMonthHasNothingSoFar(t *testing.T) {
	p := pressed(t, views.LengthThisWeek, "<+++", dayfile.CivilOf(now), time.Monday, time.Time{})
	if got, want := views.DialOf(p, now, time.Monday, time.Time{}).Name(), "12 months to 4 Oct"; got != want {
		t.Fatalf("last week lengthened to 12 months is %q, want %q", got, want)
	}
	page := views.NewPast(input(usedAcrossTwoMonths), usedAcrossTwoMonths).TokensOver(p)
	got := tokensRowsSaidOf(t, page)
	want := tokensRowsSaid{first: []string{"2026-10", "2026-09"}, costs: []string{"28.931506849315", "220"}, soFar: []bool{false, false}}
	if !slices.Equal(got.first, want.first) || !slices.Equal(got.costs, want.costs) || !slices.Equal(got.soFar, want.soFar) {
		t.Errorf("the rows are %q, costing %v, so far %v; want %q, costing %v, so far %v", got.first, got.costs, got.soFar, want.first, want.costs, want.soFar)
	}
	if page.Total == nil || page.Total.SoFar {
		t.Errorf("the total is %+v, want one not so far", page.Total)
	}
}
