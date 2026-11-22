package views_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// framesNow is the clock the signed-off frames of stepping back are drawn
// at: Thursday 1 October 2026, 14:42.
var framesNow = time.Date(2026, 10, 1, 14, 42, 0, 0, time.Local)

// civilDate is the day with the given date, as dayfile.Civil gives one.
func civilDate(date string) time.Time {
	day, ok := dayfile.Civil(date)
	if !ok {
		panic("not a date: " + date)
	}
	return day
}

// press steps the period by the key given at now, weeks starting on starts,
// the work begun at began: < back, > on, - shorter and + longer.
func press(p views.Period, key rune, now time.Time, starts time.Weekday, began time.Time) (views.Period, bool) {
	switch key {
	case '<':
		return p.Back(now, starts, began)
	case '>':
		return p.On(now, starts)
	case '-':
		return p.Shorter(now, starts)
	case '+':
		return p.Longer(now, starts)
	}
	panic("not a key: " + string(key))
}

// pressed is the period of the length given, now, stepped by each key in
// turn at now, as press has it, failing the test where one doesn't step it.
func pressed(t *testing.T, length views.Length, keys string, now time.Time, starts time.Weekday, began time.Time) views.Period {
	t.Helper()
	p := length.Now()
	for i, key := range keys {
		next, ok := press(p, key, now, starts, began)
		if !ok {
			t.Fatalf("%s pressed %q at %v: %q doesn't step", length.Name(), keys, now, keys[:i+1])
		}
		p = next
	}
	return p
}

// dialSaid is what the dial says of a period: its name, title and words
// within it, and its span's dates.
type dialSaid struct {
	name, title, within string
	span                views.SpanOf
}

// dialSaidOf is what the dial says of the period at now, weeks starting on
// starts, the work begun at began.
func dialSaidOf(p views.Period, now time.Time, starts time.Weekday, began time.Time) dialSaid {
	d := views.DialOf(p, now, starts, began)
	return dialSaid{name: d.Name(), title: d.Title(), within: d.Within(), span: p.SpanOn(now, starts)}
}

// Stepping back moves each length's last day back a length at a time, and
// names the period as the signed-off dial does: at Thursday 1 October 2026,
// weeks starting on Monday, today, yesterday, then Tue 29 Sep; this week,
// last week, then the week of its first day; and the longer lengths to
// their last day, 12 months with its year, as it isn't now's.
func TestEachLengthStepsBackAndIsNamedAsTheFramesNameIt(t *testing.T) {
	tests := []struct {
		length              views.Length
		keys                string
		name, title, within string
		first, last         string
		weeksBack           int
	}{
		{views.LengthToday, "", "today", "TODAY", "today", "2026-10-01", "2026-10-01", 0},
		{views.LengthToday, "<", "yesterday", "YESTERDAY", "yesterday", "2026-09-30", "2026-09-30", 0},
		{views.LengthToday, "<<", "Tue 29 Sep", "TUE 29 SEP", "on Tue 29 Sep", "2026-09-29", "2026-09-29", 0},
		{views.LengthThisWeek, "", "this week", "THIS WEEK", "this week", "2026-09-28", "2026-10-04", 0},
		{views.LengthThisWeek, "<", "last week", "LAST WEEK", "last week", "2026-09-21", "2026-09-27", 1},
		{views.LengthThisWeek, "<<", "week of 14 Sep", "WEEK OF 14 SEP", "in the week of 14 Sep", "2026-09-14", "2026-09-20", 2},
		{views.LengthThirtyDays, "", "30 days", "LAST 30 DAYS", "in the last 30 days", "2026-09-02", "2026-10-01", 0},
		{views.LengthThirtyDays, "<", "30 days to 1 Sep", "30 DAYS TO 1 SEP", "in the 30 days to 1 Sep", "2026-08-03", "2026-09-01", 4},
		{views.LengthThirtyDays, "<<", "30 days to 2 Aug", "30 DAYS TO 2 AUG", "in the 30 days to 2 Aug", "2026-07-04", "2026-08-02", 9},
		{views.LengthTwelveWeeks, "", "12 weeks", "LAST 12 WEEKS", "in the last 12 weeks", "2026-07-13", "2026-10-04", 0},
		{views.LengthTwelveWeeks, "<", "12 weeks to 12 Jul", "12 WEEKS TO 12 JUL", "in the 12 weeks to 12 Jul", "2026-04-20", "2026-07-12", 12},
		{views.LengthTwelveWeeks, "<<", "12 weeks to 19 Apr", "12 WEEKS TO 19 APR", "in the 12 weeks to 19 Apr", "2026-01-26", "2026-04-19", 24},
		{views.LengthTwelveMonths, "", "12 months", "LAST 12 MONTHS", "in the last 12 months", "2025-10-02", "2026-10-01", 0},
		{views.LengthTwelveMonths, "<", "12 months to 1 Oct 2025", "12 MONTHS TO 1 OCT 2025", "in the 12 months to 1 Oct 2025", "2024-10-02", "2025-10-01", 52},
		{views.LengthTwelveMonths, "<<", "12 months to 1 Oct 2024", "12 MONTHS TO 1 OCT 2024", "in the 12 months to 1 Oct 2024", "2023-10-02", "2024-10-01", 104},
	}
	for _, tt := range tests {
		t.Run(tt.length.Name()+" "+tt.keys, func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, framesNow, time.Monday, time.Time{})
			gone := tt.keys != ""
			want := dialSaid{name: tt.name, title: tt.title, within: tt.within, span: views.SpanOf{First: tt.first, Last: tt.last, Gone: gone, WeeksBack: tt.weeksBack}}
			if got := dialSaidOf(p, framesNow, time.Monday, time.Time{}); got != want {
				t.Errorf("%s pressed %q is %+v, want %+v", tt.length.Name(), tt.keys, got, want)
			}
			if d := views.DialOf(p, framesNow, time.Monday, time.Time{}); d.Gone() != gone || d.AtNow() == gone {
				t.Errorf("%s pressed %q is gone %v, at now %v, want gone %v", tt.length.Name(), tt.keys, d.Gone(), d.AtNow(), gone)
			}
		})
	}
}

// The heading's readout says the period's range, which - and + change
// alone, and when it is, which < and > change alone: at now today, this
// week, or now; one back yesterday, last week, or to its last day; further
// back a day's date, a week's days, or to its last day; a date with its
// year where it isn't now's, and a week's days across months and years. -
// stops at the shortest range and + at the longest.
func TestTheReadoutSaysTheRangeAndWhen(t *testing.T) {
	midOctober := time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local)
	midJanuary := time.Date(2026, 1, 14, 9, 0, 0, 0, time.Local)
	tests := []struct {
		name              string
		length            views.Length
		keys              string
		now               time.Time
		ranged, when      string
		shortest, longest bool
	}{
		{"today", views.LengthToday, "", framesNow, "day", "today", true, false},
		{"yesterday", views.LengthToday, "<", framesNow, "day", "yesterday", true, false},
		{"a day further back", views.LengthToday, "<<", framesNow, "day", "Tue 29 Sep", true, false},
		{"a day in another year", views.LengthToday, strings.Repeat("<", 274), framesNow, "day", "Wed 31 Dec 2025", true, false},
		{"this week", views.LengthThisWeek, "", framesNow, "week", "this week", false, false},
		{"last week", views.LengthThisWeek, "<", framesNow, "week", "last week", false, false},
		{"a week further back", views.LengthThisWeek, "<<", framesNow, "week", "14–20 Sep", false, false},
		{"a week across months", views.LengthThisWeek, "<<", midOctober, "week", "28 Sep–4 Oct", false, false},
		{"a week across years", views.LengthThisWeek, "<<", midJanuary, "week", "29 Dec 2025–4 Jan", false, false},
		{"30 days at now", views.LengthThirtyDays, "", framesNow, "30 days", "now", false, false},
		{"30 days one back", views.LengthThirtyDays, "<", framesNow, "30 days", "to 1 Sep", false, false},
		{"30 days further back", views.LengthThirtyDays, "<<", framesNow, "30 days", "to 2 Aug", false, false},
		{"12 weeks at now", views.LengthTwelveWeeks, "", framesNow, "12 weeks", "now", false, false},
		{"12 weeks one back", views.LengthTwelveWeeks, "<", framesNow, "12 weeks", "to 12 Jul", false, false},
		{"12 weeks further back", views.LengthTwelveWeeks, "<<", framesNow, "12 weeks", "to 19 Apr", false, false},
		{"12 months at now", views.LengthTwelveMonths, "", framesNow, "12 months", "now", false, true},
		{"12 months one back", views.LengthTwelveMonths, "<", framesNow, "12 months", "to 1 Oct 2025", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := views.DialOf(pressed(t, tt.length, tt.keys, tt.now, time.Monday, time.Time{}), tt.now, time.Monday, time.Time{})
			if d.Range() != tt.ranged || d.When() != tt.when || d.Shortest() != tt.shortest || d.Longest() != tt.longest {
				t.Errorf("%s pressed %q at %v reads - %s +  < %s >, shortest %v, longest %v; want - %s +  < %s >, shortest %v, longest %v",
					tt.length.Name(), tt.keys, tt.now, d.Range(), d.When(), d.Shortest(), d.Longest(), tt.ranged, tt.when, tt.shortest, tt.longest)
			}
		})
	}
}

// - and + change the length and keep the period's last day: a week length
// takes the week holding it, 12 weeks the 12 to that week; one reaching
// today is now, named as now.
func TestMinusAndPlusKeepThePeriodsLastDay(t *testing.T) {
	tests := []struct {
		length      views.Length
		keys        string
		name        string
		first, last string
	}{
		{views.LengthThisWeek, "<+", "30 days to 27 Sep", "2026-08-29", "2026-09-27"},
		{views.LengthThisWeek, "<-", "Sun 27 Sep", "2026-09-27", "2026-09-27"},
		{views.LengthThirtyDays, "<-", "week of 31 Aug", "2026-08-31", "2026-09-06"},
		{views.LengthThirtyDays, "<+", "12 weeks to 6 Sep", "2026-06-15", "2026-09-06"},
		{views.LengthThirtyDays, "<++", "12 months to 6 Sep", "2025-09-07", "2026-09-06"},
		{views.LengthTwelveWeeks, "<-", "30 days to 12 Jul", "2026-06-13", "2026-07-12"},
		{views.LengthTwelveMonths, "<-", "12 weeks to 5 Oct 2025", "2025-07-14", "2025-10-05"},
		{views.LengthToday, "<<<<+", "last week", "2026-09-21", "2026-09-27"},
		{views.LengthToday, "<+", "this week", "2026-09-28", "2026-10-04"},
		{views.LengthToday, "<<+", "this week", "2026-09-28", "2026-10-04"},
		{views.LengthToday, "<+-", "today", "2026-10-01", "2026-10-01"},
		{views.LengthThisWeek, "-", "today", "2026-10-01", "2026-10-01"},
		{views.LengthThirtyDays, "+", "12 weeks", "2026-07-13", "2026-10-04"},
		{views.LengthTwelveWeeks, "+", "12 months", "2025-10-02", "2026-10-01"},
	}
	for _, tt := range tests {
		t.Run(tt.length.Name()+" "+tt.keys, func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, framesNow, time.Monday, time.Time{})
			got := dialSaidOf(p, framesNow, time.Monday, time.Time{})
			if got.name != tt.name || got.span.First != tt.first || got.span.Last != tt.last {
				t.Errorf("%s pressed %q is %q, %s to %s, want %q, %s to %s", tt.length.Name(), tt.keys, got.name, got.span.First, got.span.Last, tt.name, tt.first, tt.last)
			}
			if now := !got.span.Gone; now != (tt.name == p.Length().Name()) {
				t.Errorf("%s pressed %q, named %q, is at now %v", tt.length.Name(), tt.keys, got.name, now)
			}
		})
	}
}

// - refuses at today, the shortest, and + at the 12 months, the longest,
// at now or gone, the period staying as it was.
func TestMinusAndPlusStopAtTheEnds(t *testing.T) {
	tests := []struct {
		length views.Length
		keys   string
		key    rune
	}{
		{views.LengthToday, "", '-'},
		{views.LengthToday, "<", '-'},
		{views.LengthTwelveMonths, "", '+'},
		{views.LengthTwelveMonths, "<", '+'},
	}
	for _, tt := range tests {
		t.Run(tt.length.Name()+" "+tt.keys+string(tt.key), func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, framesNow, time.Monday, time.Time{})
			if got, ok := press(p, tt.key, framesNow, time.Monday, time.Time{}); ok || got != p {
				t.Errorf("%s pressed %q then %q steps to %v, %v; want it to stay", tt.length.Name(), tt.keys, tt.key, got, ok)
			}
		})
	}
}

// > steps on a length at a time and stops at now: from one back, now; from
// two back, one back; at now it refuses, the period staying now.
func TestOnStepsBackToNowAndStopsThere(t *testing.T) {
	for _, length := range views.Lengths() {
		t.Run(length.Name(), func(t *testing.T) {
			if got := pressed(t, length, "<>", framesNow, time.Monday, time.Time{}); got != length.Now() {
				t.Errorf("%s pressed \"<>\" is %+v, want now", length.Name(), dialSaidOf(got, framesNow, time.Monday, time.Time{}))
			}
			if got, want := pressed(t, length, "<<>", framesNow, time.Monday, time.Time{}), pressed(t, length, "<", framesNow, time.Monday, time.Time{}); got != want {
				t.Errorf("%s pressed \"<<>\" is %+v, want one back, %+v", length.Name(), dialSaidOf(got, framesNow, time.Monday, time.Time{}), dialSaidOf(want, framesNow, time.Monday, time.Time{}))
			}
			if got, ok := length.Now().On(framesNow, time.Monday); ok || got != length.Now() {
				t.Errorf("%s at now steps on to %+v", length.Name(), dialSaidOf(got, framesNow, time.Monday, time.Time{}))
			}
		})
	}
}

// Weeks starting on Sunday end on Saturday: a week length steps back by
// them, and - and + keep their Saturday.
func TestWeeksStartingOnSundayStepByTheirWeeks(t *testing.T) {
	tests := []struct {
		length      views.Length
		keys        string
		name        string
		first, last string
	}{
		{views.LengthThisWeek, "", "this week", "2026-09-27", "2026-10-03"},
		{views.LengthThisWeek, "<", "last week", "2026-09-20", "2026-09-26"},
		{views.LengthThisWeek, "<<", "week of 13 Sep", "2026-09-13", "2026-09-19"},
		{views.LengthTwelveWeeks, "", "12 weeks", "2026-07-12", "2026-10-03"},
		{views.LengthTwelveWeeks, "<", "12 weeks to 11 Jul", "2026-04-19", "2026-07-11"},
		{views.LengthThisWeek, "<+", "30 days to 26 Sep", "2026-08-28", "2026-09-26"},
		{views.LengthThisWeek, "<-", "Sat 26 Sep", "2026-09-26", "2026-09-26"},
		{views.LengthThirtyDays, "<-", "week of 30 Aug", "2026-08-30", "2026-09-05"},
	}
	for _, tt := range tests {
		t.Run(tt.length.Name()+" "+tt.keys, func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, framesNow, time.Sunday, time.Time{})
			got := dialSaidOf(p, framesNow, time.Sunday, time.Time{})
			if got.name != tt.name || got.span.First != tt.first || got.span.Last != tt.last {
				t.Errorf("%s pressed %q is %q, %s to %s, want %q, %s to %s", tt.length.Name(), tt.keys, got.name, got.span.First, got.span.Last, tt.name, tt.first, tt.last)
			}
		})
	}
}

// Across a year's end, a date carries its year where it isn't now's: a
// week by its first day's, the rest by their last day's.
func TestADateCarriesItsYearOnlyWhereItIsntNows(t *testing.T) {
	now := time.Date(2027, 1, 11, 9, 30, 0, 0, time.Local)
	tests := []struct {
		length       views.Length
		keys         string
		name, within string
		first, last  string
	}{
		{views.LengthToday, "<<<<<<<<<<", "Fri 1 Jan", "on Fri 1 Jan", "2027-01-01", "2027-01-01"},
		{views.LengthToday, "<<<<<<<<<<<", "Thu 31 Dec 2026", "on Thu 31 Dec 2026", "2026-12-31", "2026-12-31"},
		{views.LengthThisWeek, "<", "last week", "last week", "2027-01-04", "2027-01-10"},
		{views.LengthThisWeek, "<<", "week of 28 Dec 2026", "in the week of 28 Dec 2026", "2026-12-28", "2027-01-03"},
		{views.LengthThirtyDays, "<", "30 days to 12 Dec 2026", "in the 30 days to 12 Dec 2026", "2026-11-13", "2026-12-12"},
		{views.LengthTwelveWeeks, "<", "12 weeks to 25 Oct 2026", "in the 12 weeks to 25 Oct 2026", "2026-08-03", "2026-10-25"},
		{views.LengthTwelveMonths, "<", "12 months to 11 Jan 2026", "in the 12 months to 11 Jan 2026", "2025-01-12", "2026-01-11"},
	}
	for _, tt := range tests {
		t.Run(tt.length.Name()+" "+tt.keys, func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, now, time.Monday, time.Time{})
			got := dialSaidOf(p, now, time.Monday, time.Time{})
			want := dialSaid{name: tt.name, title: strings.ToUpper(tt.name), within: tt.within, span: got.span}
			if got != want || got.span.First != tt.first || got.span.Last != tt.last {
				t.Errorf("%s pressed %q at %v is %+v, want %+v, %s to %s", tt.length.Name(), tt.keys, now, got, want, tt.first, tt.last)
			}
		})
	}
}

// 12 months step back a year, to the same date, a 29 February's to the
// 28th, as AddDate's 1 March would leave a day out; > steps to the 12 months
// beginning the day after, so from now it always comes back to now.
func TestTwelveMonthsStepByAYearTheTwentyNinthToTheTwentyEighth(t *testing.T) {
	tests := []struct {
		name        string
		now         time.Time
		keys        string
		first, last string
		gone        bool
	}{
		{"now on a 29 February", time.Date(2028, 2, 29, 12, 0, 0, 0, time.Local), "", "2027-03-01", "2028-02-29", false},
		{"back from a 29 February", time.Date(2028, 2, 29, 12, 0, 0, 0, time.Local), "<", "2026-03-01", "2027-02-28", true},
		{"twice back from a 29 February", time.Date(2028, 2, 29, 12, 0, 0, 0, time.Local), "<<", "2025-03-01", "2026-02-28", true},
		{"back and on to a 29 February", time.Date(2028, 2, 29, 12, 0, 0, 0, time.Local), "<>", "2027-03-01", "2028-02-29", false},
		{"back from a 28 February after a 29th", time.Date(2029, 2, 28, 12, 0, 0, 0, time.Local), "<", "2027-03-01", "2028-02-28", true},
		{"twice back from a 28 February after a 29th", time.Date(2029, 2, 28, 12, 0, 0, 0, time.Local), "<<", "2026-03-01", "2027-02-28", true},
		{"on from the 28th before a 29 February, to the 12 months after it", time.Date(2029, 2, 28, 12, 0, 0, 0, time.Local), "<<>", "2027-03-01", "2028-02-29", true},
		{"on from a 28 February to the 12 months after it", time.Date(2030, 2, 28, 12, 0, 0, 0, time.Local), "<<<>", "2027-03-01", "2028-02-29", true},
		{"on from a 29 February, to the 12 months beginning on it", time.Date(2030, 2, 28, 12, 0, 0, 0, time.Local), "<<<>>", "2028-02-29", "2029-02-28", true},
		{"back from 1 March after a 29 February", time.Date(2028, 3, 1, 12, 0, 0, 0, time.Local), "<", "2026-03-02", "2027-03-01", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pressed(t, views.LengthTwelveMonths, tt.keys, tt.now, time.Monday, time.Time{})
			if got, want := p.SpanOn(tt.now, time.Monday), (views.SpanOf{First: tt.first, Last: tt.last, Gone: tt.gone}); got.First != want.First || got.Last != want.Last || got.Gone != want.Gone {
				t.Errorf("12 months pressed %q at %v span %+v, want %+v", tt.keys, tt.now, got, want)
			}
		})
	}
}

// Periods tile both ways: every step back ends the day before the period it
// steps from begins, and every step on begins the day after it ends, but
// that no 12 months begin on the 1 March after a 29 February, so those after
// one begin on it; and stepping back then on as many times comes back to
// now, on every day of three years, a 29 February among them, weeks starting
// on any day.
func TestStepsTileAndStepBackToNow(t *testing.T) {
	for day := civilDate("2027-01-01"); day.Year() < 2030; day = day.AddDate(0, 0, 1) {
		now := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.Local)
		for _, starts := range []time.Weekday{time.Monday, time.Sunday, time.Wednesday} {
			for _, length := range views.Lengths() {
				p := length.Now()
				for back := 1; back <= 3; back++ {
					was := p.SpanOn(now, starts)
					next, ok := p.Back(now, starts, time.Time{})
					if got := next.SpanOn(now, starts); !ok || !civilDate(got.Last).AddDate(0, 0, 1).Equal(civilDate(was.First)) {
						t.Fatalf("%s back %d on %s from %s: %s to %s, %v; want it to end the day before %s",
							length.Name(), back, day.Format(time.DateOnly), starts, got.First, got.Last, ok, was.First)
					}
					p = next
				}
				for on := 1; on <= 3; on++ {
					was := p.SpanOn(now, starts)
					next, ok := p.On(now, starts)
					if got := next.SpanOn(now, starts); !ok || got.First != beginsAfter(length, was.Last) {
						t.Fatalf("%s on %d on %s from %s: %s to %s, %v; want it to begin %s",
							length.Name(), on, day.Format(time.DateOnly), starts, got.First, got.Last, ok, beginsAfter(length, was.Last))
					}
					p = next
				}
				if p != length.Now() {
					t.Fatalf("%s back 3 then on 3 on %s from %s is %+v, want now", length.Name(), day.Format(time.DateOnly), starts, p.SpanOn(now, starts))
				}
			}
		}
	}
}

// beginsAfter is the date the period of the length a step on from one
// ending on the date last begins on: the day after; but of 12 months after
// a 29 February, that 29th, as none begin on the 1 March after one.
func beginsAfter(length views.Length, last string) string {
	if length == views.LengthTwelveMonths && strings.HasSuffix(last, "-02-29") {
		return last
	}
	return civilDate(last).AddDate(0, 0, 1).Format(time.DateOnly)
}

// < stops at the period holding the day the work began: it refuses where
// the period begins on or before that day, the period staying, and the
// dial says it's the earliest; where that day isn't known, it steps freely.
func TestBackStopsAtThePeriodHoldingTheFirstDayOfWork(t *testing.T) {
	tests := []struct {
		name   string
		length views.Length
		began  time.Time
		keys   string
		last   string
	}{
		{"a week, the work begun mid-week", views.LengthThisWeek, on("2026-09-15").Add(10 * time.Hour), "<<", "2026-09-20"},
		{"a week, the work begun on its first day", views.LengthThisWeek, on("2026-09-14"), "<<", "2026-09-20"},
		{"a week, the work begun the day before it", views.LengthThisWeek, on("2026-09-13"), "<<<", "2026-09-13"},
		{"a day", views.LengthToday, on("2026-09-29"), "<<", "2026-09-29"},
		{"30 days, the work begun in them", views.LengthThirtyDays, on("2026-09-15"), "", "2026-10-01"},
		{"12 months, the work begun in them", views.LengthTwelveMonths, on("2026-03-02"), "", "2026-10-01"},
		{"12 months, the work begun the 12 months before", views.LengthTwelveMonths, on("2025-03-02"), "<", "2025-10-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, framesNow, time.Monday, tt.began)
			for i, key := range tt.keys {
				before := pressed(t, tt.length, tt.keys[:i], framesNow, time.Monday, tt.began)
				if views.DialOf(before, framesNow, time.Monday, tt.began).Earliest() {
					t.Errorf("%s pressed %q, the work begun %v, is the earliest, though %q steps back", tt.length.Name(), tt.keys[:i], tt.began, string(key))
				}
			}
			if got := p.SpanOn(framesNow, time.Monday).Last; got != tt.last {
				t.Errorf("%s pressed %q ends %s, want %s", tt.length.Name(), tt.keys, got, tt.last)
			}
			if got, ok := p.Back(framesNow, time.Monday, tt.began); ok || got != p {
				t.Errorf("%s pressed %q, the work begun %v, steps back to %+v", tt.length.Name(), tt.keys, tt.began, got.SpanOn(framesNow, time.Monday))
			}
			if !views.DialOf(p, framesNow, time.Monday, tt.began).Earliest() {
				t.Errorf("%s pressed %q, the work begun %v, isn't the earliest", tt.length.Name(), tt.keys, tt.began)
			}
		})
	}
}

// A period lying wholly before the day the work began, as a change of pool
// can leave one held, is at the floor too: < refuses, giving it back, and the
// dial says it's the earliest.
func TestAPeriodBeforeTheFirstDayOfWorkIsAtTheFloor(t *testing.T) {
	began := on("2026-09-25")
	p := pressed(t, views.LengthThisWeek, "<<", framesNow, time.Monday, time.Time{})
	if got, ok := p.Back(framesNow, time.Monday, began); ok || got != p {
		t.Errorf("the week of 14 Sep, the work begun %v, steps back to %+v", began, got.SpanOn(framesNow, time.Monday))
	}
	if !views.DialOf(p, framesNow, time.Monday, began).Earliest() {
		t.Errorf("the week of 14 Sep, the work begun %v, isn't the earliest", began)
	}
}

// Where the day the work began isn't known, < steps back freely, more than
// a year, and the dial never says it's the earliest.
func TestBackStepsFreelyWhereTheFirstDayOfWorkIsntKnown(t *testing.T) {
	p := views.LengthThisWeek.Now()
	for back := range 60 {
		if views.DialOf(p, framesNow, time.Monday, time.Time{}).Earliest() {
			t.Fatalf("this week %d back is the earliest", back)
		}
		next, ok := p.Back(framesNow, time.Monday, time.Time{})
		if !ok {
			t.Fatalf("this week %d back doesn't step back", back)
		}
		p = next
	}
	if got := p.SpanOn(framesNow, time.Monday).Last; got != "2025-08-10" {
		t.Errorf("this week 60 back ends %s, want 2025-08-10", got)
	}
}

// A period gone keeps its dates as the clock moves on: yesterday's day is
// named by its date the day after. Where the clock goes back to the period
// or before it, it's now, named as now, and steps from now.
func TestAPeriodGoneKeepsItsDatesAndIsNowOnceItReachesToday(t *testing.T) {
	tests := []struct {
		name      string
		length    views.Length
		keys      string
		now       time.Time
		said      string
		first     string
		last      string
		weeksBack int
		gone      bool
	}{
		{"yesterday, a day on", views.LengthToday, "<", time.Date(2026, 10, 2, 0, 5, 0, 0, time.Local), "Wed 30 Sep", "2026-09-30", "2026-09-30", 0, true},
		{"last week, a week on", views.LengthThisWeek, "<", time.Date(2026, 10, 5, 9, 0, 0, 0, time.Local), "week of 21 Sep", "2026-09-21", "2026-09-27", 2, true},
		{"yesterday, the clock back to it", views.LengthToday, "<", time.Date(2026, 9, 30, 10, 0, 0, 0, time.Local), "today", "2026-09-30", "2026-09-30", 0, false},
		{"last week, the clock back into it", views.LengthThisWeek, "<", time.Date(2026, 9, 25, 10, 0, 0, 0, time.Local), "this week", "2026-09-21", "2026-09-27", 0, false},
		{"30 days to 1 Sep, the clock back before their last day", views.LengthThirtyDays, "<", time.Date(2026, 8, 20, 10, 0, 0, 0, time.Local), "30 days", "2026-07-22", "2026-08-20", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, framesNow, time.Monday, time.Time{})
			got := dialSaidOf(p, tt.now, time.Monday, time.Time{})
			want := views.SpanOf{First: tt.first, Last: tt.last, Gone: tt.gone, WeeksBack: tt.weeksBack}
			if got.name != tt.said || got.span != want {
				t.Errorf("%s pressed %q at %v is %q, %+v, want %q, %+v", tt.length.Name(), tt.keys, tt.now, got.name, got.span, tt.said, want)
			}
			if _, ok := p.On(tt.now, time.Monday); ok != tt.gone {
				t.Errorf("%s pressed %q at %v steps on %v, want %v", tt.length.Name(), tt.keys, tt.now, ok, tt.gone)
			}
			if tt.gone {
				return
			}
			if back, _ := p.Back(tt.now, time.Monday, time.Time{}); back != pressed(t, tt.length, "<", tt.now, time.Monday, time.Time{}) {
				t.Errorf("%s pressed %q at %v steps back to %+v, want now's one back", tt.length.Name(), tt.keys, tt.now, back.SpanOn(tt.now, time.Monday))
			}
			// Each step refused gives the period now, never the one held.
			if got, _ := p.On(tt.now, time.Monday); got != tt.length.Now() {
				t.Errorf("%s pressed %q at %v, refusing >, gives %+v, want now", tt.length.Name(), tt.keys, tt.now, got)
			}
			if got, ok := p.Back(tt.now, time.Monday, tt.now); ok || got != tt.length.Now() {
				t.Errorf("%s pressed %q at %v, refusing < at the floor, gives %+v, %v, want now", tt.length.Name(), tt.keys, tt.now, got, ok)
			}
		})
	}
}

// A period gone begins where a read of its days begins: at the start of its
// first local day.
func TestAPeriodGoneBeginsAtItsFirstDaysStart(t *testing.T) {
	tests := []struct {
		length views.Length
		keys   string
		want   time.Time
	}{
		{views.LengthToday, "<", on("2026-09-30")},
		{views.LengthThisWeek, "<", on("2026-09-21")},
		{views.LengthThirtyDays, "<", on("2026-08-03")},
		{views.LengthTwelveMonths, "<", on("2024-10-02")},
	}
	for _, tt := range tests {
		t.Run(tt.length.Name()+" "+tt.keys, func(t *testing.T) {
			if got := pressed(t, tt.length, tt.keys, framesNow, time.Monday, time.Time{}).From(framesNow, time.Monday); !got.Equal(tt.want) {
				t.Errorf("%s pressed %q begins %v, want %v", tt.length.Name(), tt.keys, got, tt.want)
			}
		})
	}
}

// The dial's short name is its length's name at now, as its name is, and
// gone, never one that says now (today, this week): a day's own name, or in
// another year its date with its year, the week of a week, and the longer
// lengths' own names. Yesterday keeps its name on 1 January, its year
// another: the year is said only of a date.
func TestShortIsTheLengthsNameNeverOneThatSaysNowGone(t *testing.T) {
	newYear := time.Date(2027, 1, 1, 9, 0, 0, 0, time.Local)
	tests := []struct {
		length views.Length
		keys   string
		want   string
		now    time.Time
	}{
		{views.LengthToday, "", "today", framesNow},
		{views.LengthToday, "<", "yesterday", framesNow},
		{views.LengthToday, "<<", "Tue 29 Sep", framesNow},
		{views.LengthToday, strings.Repeat("<", 274), "31 Dec 2025", framesNow},
		{views.LengthToday, "<", "yesterday", newYear},
		{views.LengthToday, "<<", "30 Dec 2026", newYear},
		{views.LengthThisWeek, "", "this week", framesNow},
		{views.LengthThisWeek, "<", "the week", framesNow},
		{views.LengthThisWeek, "<<", "the week", framesNow},
		{views.LengthThirtyDays, "", "30 days", framesNow},
		{views.LengthThirtyDays, "<", "30 days", framesNow},
		{views.LengthTwelveWeeks, "", "12 weeks", framesNow},
		{views.LengthTwelveWeeks, "<<", "12 weeks", framesNow},
		{views.LengthTwelveMonths, "", "12 months", framesNow},
		{views.LengthTwelveMonths, "<", "12 months", framesNow},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s %d back on %s", tt.length.Name(), len(tt.keys), tt.now.Format(time.DateOnly)), func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, tt.now, time.Monday, time.Time{})
			if got := views.DialOf(p, tt.now, time.Monday, time.Time{}).Short(); got != tt.want {
				t.Errorf("%s pressed %q is short %q, want %q", tt.length.Name(), tt.keys, got, tt.want)
			}
		})
	}
}

// The heading keeps room for the widest readout: at now, the widest of
// every length's at now, which every period at now fits and one fills;
// gone, the widest of every period gone, which every period gone of a
// length, stepped back a hundred times at a later year's start, fits, one
// filling it.
func TestWidestIsTheWidestReadoutAtNowOrGone(t *testing.T) {
	cells := func(ranged, when string) int { return utf8.RuneCountInString(ranged) + utf8.RuneCountInString(when) }
	now := views.DialOf(views.LengthThisWeek.Now(), framesNow, time.Monday, time.Time{})
	gone := views.DialOf(pressed(t, views.LengthThisWeek, "<", framesNow, time.Monday, time.Time{}), framesNow, time.Monday, time.Time{})
	nowRange, nowWhen := now.Widest()
	goneRange, goneWhen := gone.Widest()
	if nowRange != "week" || nowWhen != "this week" {
		t.Errorf("at now the heading keeps room for %q %q, want week, this week", nowRange, nowWhen)
	}
	if goneRange != "week" || cells(goneRange, goneWhen) != 26 {
		t.Errorf("gone the heading keeps room for %q %q, want a week over a year's end, neither year now's, as 29 Dec 2025–4 Jan 2026", goneRange, goneWhen)
	}
	newYear := time.Date(2027, 1, 1, 9, 0, 0, 0, time.Local)
	filled := false
	for _, length := range views.Lengths() {
		d := views.DialOf(length.Now(), newYear, time.Monday, time.Time{})
		if got := cells(d.Range(), d.When()); got > cells(nowRange, nowWhen) {
			t.Errorf("%s at now, %q %q, is %d wide, wider than the room kept", length.Name(), d.Range(), d.When(), got)
		}
		p := length.Now()
		for back := range 100 {
			p, _ = p.Back(newYear, time.Monday, time.Time{})
			d := views.DialOf(p, newYear, time.Monday, time.Time{})
			got := cells(d.Range(), d.When())
			if got > cells(goneRange, goneWhen) {
				t.Errorf("%s %d back, %q %q, is %d wide, wider than the room kept", length.Name(), back+1, d.Range(), d.When(), got)
			}
			filled = filled || got == cells(goneRange, goneWhen)
		}
	}
	if !filled {
		t.Errorf("no period gone fills the room kept for %q %q", goneRange, goneWhen)
	}
}

// A time in the period is said as Projects' last says it: at now as
// prose.Lately says it; gone, never now, over a day by its time alone, over
// more with its weekday in the period's last seven days, else its date, its
// year where it isn't now's.
func TestATimeInThePeriodIsSaidByHowFarIntoItItFalls(t *testing.T) {
	at := func(date string, hour, minute int) time.Time {
		day := civilDate(date)
		return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.Local)
	}
	tests := []struct {
		name   string
		length views.Length
		keys   string
		t      time.Time
		want   string
	}{
		{"today, at now", views.LengthToday, "", at("2026-10-01", 13, 51), "13:51"},
		{"this week, yesterday, at now", views.LengthThisWeek, "", at("2026-09-30", 21, 40), "yesterday"},
		{"30 days, a week back, at now", views.LengthThirtyDays, "", at("2026-09-21", 21, 40), "21 Sep"},
		{"yesterday", views.LengthToday, "<", at("2026-09-30", 21, 40), "21:40"},
		{"a day gone", views.LengthToday, "<<", at("2026-09-29", 8, 5), "08:05"},
		{"last week, its last day", views.LengthThisWeek, "<", at("2026-09-27", 21, 40), "Sun 21:40"},
		{"last week, its first day", views.LengthThisWeek, "<", at("2026-09-21", 9, 12), "Mon 09:12"},
		{"30 days gone, their last day", views.LengthThirtyDays, "<", at("2026-09-01", 21, 40), "Tue 21:40"},
		{"30 days gone, six days before their last", views.LengthThirtyDays, "<", at("2026-08-26", 10, 0), "Wed 10:00"},
		{"30 days gone, seven days before their last", views.LengthThirtyDays, "<", at("2026-08-25", 10, 0), "25 Aug 10:00"},
		{"30 days gone, further back", views.LengthThirtyDays, "<", at("2026-08-21", 21, 40), "21 Aug 21:40"},
		{"12 months gone, in another year", views.LengthTwelveMonths, "<", at("2024-10-03", 9, 0), "3 Oct 2024 09:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := views.DialOf(pressed(t, tt.length, tt.keys, framesNow, time.Monday, time.Time{}), framesNow, time.Monday, time.Time{})
			if got := d.TimeSaid(tt.t); got != tt.want {
				t.Errorf("%s pressed %q says %v as %q, want %q", tt.length.Name(), tt.keys, tt.t, got, tt.want)
			}
		})
	}
}

// At now, a time in the period is said as Projects' last said it before the
// dial stepped in time, prose.Lately's, whatever the length.
func TestATimeAtNowIsSaidAsLatelySaysIt(t *testing.T) {
	for _, length := range views.Lengths() {
		d := views.DialOf(length.Now(), framesNow, time.Monday, time.Time{})
		for back := range 40 {
			at := framesNow.Add(-time.Duration(back) * 7 * time.Hour)
			if got, want := d.TimeSaid(at), prose.Lately(framesNow, at); got != want {
				t.Errorf("%s at now says %v as %q, want %q", length.Name(), at, got, want)
			}
		}
	}
}

// inZone has the local time zone be loc until the test ends, as a process
// started there has it.
func inZone(t *testing.T, loc *time.Location) {
	t.Helper()
	was := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = was })
}

// A period's days are the local zone's, however now's told: its steps, where
// it begins, its words and the times it tells follow the local day, 10 hours
// west of UTC and 13 east, half an hour after midnight and before it, now
// told in UTC, whose day and year differ there.
func TestAPeriodsDaysAreTheLocalZones(t *testing.T) {
	tests := []struct {
		date   string
		length views.Length
		keys   string
		name   string
		first  string
	}{
		{"2026-10-01", views.LengthToday, "<", "yesterday", "2026-09-30"},
		{"2026-10-01", views.LengthToday, "<<", "Tue 29 Sep", "2026-09-29"},
		{"2026-10-01", views.LengthThisWeek, "<", "last week", "2026-09-21"},
		{"2026-10-01", views.LengthThisWeek, "<>", "this week", "2026-09-28"},
		{"2026-10-01", views.LengthTwelveMonths, "<", "12 months to 1 Oct 2025", "2024-10-02"},
		{"2027-01-01", views.LengthToday, "<<", "Wed 30 Dec 2026", "2026-12-30"},
		{"2027-01-01", views.LengthThisWeek, "<<", "week of 14 Dec 2026", "2026-12-14"},
		{"2027-01-01", views.LengthThirtyDays, "<", "30 days to 2 Dec 2026", "2026-11-03"},
	}
	zones := []*time.Location{time.FixedZone("UTC-10", -10*60*60), time.FixedZone("UTC+13", 13*60*60)}
	for _, zone := range zones {
		for _, clock := range [][2]int{{0, 30}, {23, 30}} {
			for _, tt := range tests {
				day := civilDate(tt.date)
				local := time.Date(day.Year(), day.Month(), day.Day(), clock[0], clock[1], 0, 0, zone)
				t.Run(tt.length.Name()+" "+tt.keys+" at "+local.String(), func(t *testing.T) {
					inZone(t, zone)
					now := local.UTC()
					p := pressed(t, tt.length, tt.keys, now, time.Monday, time.Time{})
					if got := views.DialOf(p, now, time.Monday, time.Time{}).Name(); got != tt.name {
						t.Errorf("%s pressed %q at %v is %q, want %q", tt.length.Name(), tt.keys, local, got, tt.name)
					}
					first := civilDate(tt.first)
					if got, want := p.From(now, time.Monday), time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, zone); !got.Equal(want) {
						t.Errorf("%s pressed %q at %v begins %v, want %v", tt.length.Name(), tt.keys, local, got, want)
					}
				})
			}
			local := time.Date(2026, 10, 1, clock[0], clock[1], 0, 0, zone)
			t.Run("a time last week at "+local.String(), func(t *testing.T) {
				inZone(t, zone)
				now := local.UTC()
				d := views.DialOf(pressed(t, views.LengthThisWeek, "<", now, time.Monday, time.Time{}), now, time.Monday, time.Time{})
				if got := d.TimeSaid(time.Date(2026, 9, 27, 21, 40, 0, 0, zone).UTC()); got != "Sun 21:40" {
					t.Errorf("last week at %v says Sun 27 Sep 21:40 as %q, want \"Sun 21:40\"", local, got)
				}
			})
		}
	}
}
