package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
)

// Runway's past is judged by the readings history's readings, then those the
// watch read of the documents it read itself, the history holding none of
// them, all of them oldest first: each in its place by when it was read,
// those read at one time in the order given, the history's first.
func TestRunwaysReadingsAreTheHistorysAndThoseTheWatchReadInTheOrderRead(t *testing.T) {
	carried := reading("work", "5h", "2026-10-06", "22:00", 0.1)
	filed := []readings.Reading{carried, reading("work", "5h", "2026-10-07", "09:00", 0.2), reading("side", "5h", "2026-10-07", "11:00", 0.3)}
	seen := []readings.Reading{reading("work", "5h", "2026-10-07", "10:00", 0.25), reading("work", "7d", "2026-10-07", "11:00", 0.5), reading("work", "5h", "2026-10-07", "12:00", 0.4)}
	tests := []struct {
		name         string
		filed, seen  []readings.Reading
		want         []readings.Reading
		filedAsGiven bool
	}{
		{name: "none seen", filed: filed, want: filed, filedAsGiven: true},
		{name: "none filed", seen: seen, want: seen},
		{name: "both", filed: filed, seen: seen, want: []readings.Reading{carried, filed[1], seen[0], filed[2], seen[1], seen[2]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := views.RunwayReadings(tt.filed, tt.seen)
			if !slices.Equal(got, tt.want) {
				t.Errorf("RunwayReadings() = %+v, want %+v", got, tt.want)
			}
			if tt.filedAsGiven && len(got) > 0 && &got[0] != &tt.filed[0] {
				t.Error("RunwayReadings() copied the history's readings, with none seen, want them as given, for a look to hold its past by them")
			}
		})
	}
	if was := slices.Clone(filed); views.RunwayReadings(filed, seen) != nil && !slices.Equal(filed, was) {
		t.Errorf("RunwayReadings() changed the history's readings given to %+v, want them as they were", filed)
	}
}

// The summaries a Past holds of a span of days are those of its days from
// the first to the last, oldest first, as the ledger gave them: none of a
// span it holds no day of.
func TestAPastsSummariesOfASpanAreItsDaysFromTheFirstToTheLast(t *testing.T) {
	past := views.NewPast(input(overTheYear), overTheYear)
	tests := []struct {
		name        string
		first, last time.Time
		want        []string
	}{
		{name: "a day", first: civil(2026, time.October, 6), last: civil(2026, time.October, 6), want: []string{"2026-10-06"}},
		{name: "a week to today", first: civil(2026, time.September, 30), last: civil(2026, time.October, 7), want: summariesBetween(overTheYear, "2026-09-30", "2026-10-07")},
		{name: "before the days held", first: civil(2024, time.January, 1), last: civil(2024, time.January, 7)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, s := range past.Summaries(tt.first, tt.last) {
				got = append(got, s.Day)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Summaries(%s, %s) are of %q, want %q", tt.first.Format(time.DateOnly), tt.last.Format(time.DateOnly), got, tt.want)
			}
		})
	}
	if got := past.Summaries(civil(2026, time.October, 6), civil(2026, time.October, 6)); len(got) != 1 || !sameSummary(got[0], summaryOn(overTheYear, "2026-10-06")) {
		t.Errorf("Summaries(6 October) = %+v, want the ledger's summary of it as given", got)
	}
}

// summariesBetween are the dates of the days given from first to last.
func summariesBetween(days []ledger.Summary, first, last string) []string {
	var dates []string
	for _, d := range days {
		if d.Day >= first && d.Day <= last {
			dates = append(dates, d.Day)
		}
	}
	return dates
}

// summaryOn is the day of those given with the given date.
func summaryOn(days []ledger.Summary, date string) ledger.Summary {
	return days[slices.IndexFunc(days, func(d ledger.Summary) bool { return d.Day == date })]
}

// sameSummary reports whether a and b are the one day's summary, its
// accounts' days alike.
func sameSummary(a, b ledger.Summary) bool {
	return a.Day == b.Day && a.Lines == b.Lines && slices.EqualFunc(a.Accounts, b.Accounts, func(x, y ledger.AccountDay) bool {
		return x.Account == y.Account && len(x.Models) == len(y.Models) && x.Sessions == y.Sessions
	})
}
