package main

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

func TestScoresFromTheFirstHourTwoDaysAfterTheFirstReading(t *testing.T) {
	// Work's week, resetting 150 hours in, is read every hour from the first
	// reading to the last, and forecast short at each hour.
	tests := []struct {
		name        string
		first, last time.Time
		// from, scored and to are the first hour played, the first scored and
		// the last played; scoredAny, whether any is scored.
		from, scored, to time.Time
		scoredAny        bool
	}{
		{name: "first read on the hour", first: begins, last: hoursIn(60), from: hoursIn(1), scored: hoursIn(48), to: hoursIn(60), scoredAny: true},
		{name: "first read past the hour", first: begins.Add(20 * time.Minute), last: hoursIn(60).Add(20 * time.Minute), from: hoursIn(1), scored: hoursIn(49), to: hoursIn(60), scoredAny: true},
		{name: "last read two days in", first: begins, last: hoursIn(48), from: hoursIn(1), scored: hoursIn(48), to: hoursIn(48), scoredAny: true},
		{name: "last read a minute short of two days in", first: begins, last: hoursIn(48).Add(-time.Minute), from: hoursIn(1), scored: hoursIn(48), to: hoursIn(47)},
	}
	configured := config.Accounts{{ID: "work"}}
	policy := claude.Policy()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read := hourly("work", tt.first, int(tt.last.Sub(tt.first)/time.Hour)+1, 0.1, hoursIn(150))
			if last := read[len(read)-1].At; last.Before(tt.last) {
				read = append(read, weekRead("work", tt.last, 0.2, hoursIn(150)))
			}
			pl := playBack(configured, policy, history{readings: read, lines: slices.Values([]ledger.Held(nil))})

			if !pl.from.Equal(tt.from) || !pl.scored.Equal(tt.scored) || !pl.to.Equal(tt.to) {
				t.Errorf("played from %v, scored from %v, to %v; want from %v, scored from %v, to %v", pl.from, pl.scored, pl.to, tt.from, tt.scored, tt.to)
			}
			for name, forecasts := range map[string][]forecast{"the router's": pl.forecasts, "the baseline's": pl.baseline} {
				if got := hoursOf(forecasts); !slices.Equal(got, hoursFrom(tt.scored, tt.to)) {
					t.Errorf("%s forecasts were made at %v, want each hour from %v to %v", name, got, tt.scored, tt.to)
				}
			}
			if got := reportOf(configured, pl, outcomesOf(configured, policy.Perishable, read)).Scored; got != tt.scoredAny {
				t.Errorf("reported scored %v, want %v", got, tt.scoredAny)
			}
		})
	}
}

// hoursOf are the hours forecasts were made at, in the order made.
func hoursOf(forecasts []forecast) []time.Time {
	hours := make([]time.Time, 0, len(forecasts))
	for _, f := range forecasts {
		hours = append(hours, f.at)
	}
	return hours
}

// hoursFrom are the hours from from to to: none where from is after to.
func hoursFrom(from, to time.Time) []time.Time {
	hours := []time.Time{}
	for hour := from; !hour.After(to); hour = hour.Add(time.Hour) {
		hours = append(hours, hour)
	}
	return hours
}

func TestHourFromIsTheFirstHourAtATimeOrAfterIt(t *testing.T) {
	tests := []struct {
		at, want time.Time
	}{
		{at: begins, want: begins},
		{at: begins.Add(time.Nanosecond), want: hoursIn(1)},
		{at: hoursIn(1).Add(-time.Nanosecond), want: hoursIn(1)},
	}
	for _, tt := range tests {
		if got := hourFrom(tt.at); !got.Equal(tt.want) {
			t.Errorf("hourFrom(%v) = %v, want %v", tt.at, got, tt.want)
		}
	}
}
