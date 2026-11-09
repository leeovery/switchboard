package main

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

func TestScoresFromTheLaterOfTwoDaysInAndEveryAccountsWeekRead(t *testing.T) {
	// Work's week, resetting 150 hours in, is read every hour from the first
	// reading to the last, and forecast short at each hour, unless work's
	// 5-hour window is read instead, as workKey says; side, where it's
	// configured, and any account not configured, are read as given.
	sideFrom := hoursIn(60).Add(20 * time.Minute)
	sideSession := []readings.Reading{{At: sideFrom, Account: "side", Key: "5h", Utilization: 0.3, ResetsAt: hoursIn(64)}}
	tests := []struct {
		name        string
		first, last time.Time
		workKey     string
		// sided is set where side is configured, and side are its readings
		// and those of accounts not configured.
		sided bool
		side  []readings.Reading
		// from, scored and to are the first hour played, the first scored, none
		// where none is, and the last played; scoredAny, whether any is
		// scored; unread, the accounts whose week the history never reads.
		from, scored, to time.Time
		scoredAny        bool
		unread           []string
	}{
		{name: "first read on the hour", first: begins, last: hoursIn(60), from: hoursIn(1), scored: hoursIn(48), to: hoursIn(60), scoredAny: true},
		{name: "first read past the hour", first: begins.Add(20 * time.Minute), last: hoursIn(60).Add(20 * time.Minute), from: hoursIn(1), scored: hoursIn(49), to: hoursIn(60), scoredAny: true},
		{name: "last read two days in", first: begins, last: hoursIn(48), from: hoursIn(1), scored: hoursIn(48), to: hoursIn(48), scoredAny: true},
		{name: "last read a minute short of two days in", first: begins, last: hoursIn(48).Add(-time.Minute), from: hoursIn(1), scored: hoursIn(48), to: hoursIn(47)},
		{
			name: "side's week first read past two days in", first: begins, last: hoursIn(70), sided: true, side: hourly("side", sideFrom, 10, 0.3, hoursIn(160)),
			from: hoursIn(1), scored: hoursIn(61), to: hoursIn(70), scoredAny: true,
		},
		{
			name: "side read only its 5-hour window", first: begins, last: hoursIn(70), sided: true, side: sideSession,
			from: hoursIn(1), to: hoursIn(70), unread: []string{"side"},
		},
		{name: "side never read", first: begins, last: hoursIn(70), sided: true, from: hoursIn(1), to: hoursIn(70), unread: []string{"side"}},
		{
			name: "work read only its 5-hour window, side never read", first: begins, last: hoursIn(70), workKey: "5h", sided: true,
			from: hoursIn(1), to: hoursIn(70), unread: []string{"work", "side"},
		},
		{
			name: "an account not configured first read past two days in", first: begins, last: hoursIn(70), side: hourly("other", sideFrom, 10, 0.3, hoursIn(160)),
			from: hoursIn(1), scored: hoursIn(48), to: hoursIn(70), scoredAny: true,
		},
	}
	policy := claude.Policy()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configured := config.Accounts{{ID: "work"}}
			if tt.sided {
				configured = append(configured, config.Account{ID: "side"})
			}
			read := hourly("work", tt.first, int(tt.last.Sub(tt.first)/time.Hour)+1, 0.1, hoursIn(150))
			if last := read[len(read)-1].At; last.Before(tt.last) {
				read = append(read, weekRead("work", tt.last, 0.2, hoursIn(150)))
			}
			for i := range read {
				read[i].Key = cmp.Or(tt.workKey, read[i].Key)
			}
			read = slices.SortedStableFunc(slices.Values(slices.Concat(read, tt.side)), func(a, b readings.Reading) int { return a.At.Compare(b.At) })
			pl := playBack(configured, policy, history{readings: read, lines: slices.Values([]ledger.Held(nil))})

			scored, _ := pl.scoredFrom()
			if !pl.from.Equal(tt.from) || !scored.Equal(tt.scored) || !pl.to.Equal(tt.to) {
				t.Errorf("played from %v, scored from %v, to %v; want from %v, scored from %v, to %v", pl.from, scored, pl.to, tt.from, tt.scored, tt.to)
			}
			want := []time.Time{}
			if !tt.scored.IsZero() {
				want = hoursFrom(tt.scored, tt.to)
			}
			for name, forecasts := range map[string][]forecast{"the router's": pl.forecasts, "the baseline's": pl.baseline} {
				if got := hoursOf(forecasts, "work"); !slices.Equal(got, want) {
					t.Errorf("%s forecasts of work were made at %v, want %v", name, got, want)
				}
			}
			r := reportOf(configured, pl, outcomesOf(configured, policy.Perishable, read))
			if r.Scored != tt.scoredAny || !slices.Equal(r.Unread, tt.unread) {
				t.Errorf("reported scored %v, unread %v; want %v, %v", r.Scored, r.Unread, tt.scoredAny, tt.unread)
			}
		})
	}
}

// hoursOf are the hours the forecasts of the account with the given id were
// made at, among forecasts, in the order made.
func hoursOf(forecasts []forecast, id string) []time.Time {
	hours := []time.Time{}
	for _, f := range forecasts {
		if f.account == id {
			hours = append(hours, f.at)
		}
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
