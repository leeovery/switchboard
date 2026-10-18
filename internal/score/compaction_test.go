package score_test

import (
	"math"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

func TestLeftAtReset(t *testing.T) {
	tests := []struct {
		name      string
		candidate score.Candidate
		want      float64
		wantOK    bool
	}{
		{name: "quiet: all its room", candidate: weekly("side", 0.5, 2*day, 0), want: 0.5, wantOK: true},
		{name: "at its rate, short of its limit", candidate: weekly("side", 0.5, 2*day, 0.005), want: 0.26, wantOK: true},
		{name: "its reserve kept back", candidate: reserving(weekly("side", 0.5, 2*day, 0.005), 0.1), want: 0.16, wantOK: true},
		{name: "past its reserve by its reset: none", candidate: reserving(weekly("side", 0.5, 2*day, 0.009), 0.1), wantOK: true},
		{name: "running out before its reset: none", candidate: weekly("side", 0.5, 2*day, 0.02), wantOK: true},
		{name: "refused: none", candidate: withWeek("side", refused(window("7d", 0.5, 2*day)), 0), wantOK: true},
		{name: "no perishable window", candidate: score.Candidate{ID: "side", Windows: []quota.Window{window("5h", 0.2, time.Hour)}}},
		{name: "reset unknown", candidate: withWeek("side", quota.Window{Key: "7d", Utilization: 0.5}, 0)},
		{name: "refused, reset unknown", candidate: withWeek("side", refused(quota.Window{Key: "7d", Utilization: 0.5}), 0)},
		{name: "reset since it was read", candidate: weekly("side", 0.5, -time.Hour, 0)},
		{name: "its rate unknown, never taken as none", candidate: unrated(weekly("side", 0.1, 18*time.Hour, 0))},
		{name: "refused, its rate unknown", candidate: unrated(withWeek("side", refused(window("7d", 0.5, 2*day)), 0))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := policy.LeftAtReset(tt.candidate, now)
			if math.Abs(got-tt.want) > 1e-9 || ok != tt.wantOK {
				t.Errorf("LeftAtReset(%+v) = %v, %v, want %v, %v", tt.candidate, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestAtCompaction(t *testing.T) {
	// work is the account the session is on, its week resetting in five
	// days.
	work := weekly("work", 0.5, 5*day, 0.004)
	tests := []struct {
		name   string
		own    score.Candidate
		choice score.Candidate
		want   score.Move
	}{
		{
			name:   "resets sooner, with room at risk: moves",
			own:    work,
			choice: weekly("side", 0.5, 2*day, 0.005),
			want:   score.Move{Verdict: score.Moves, To: "side", Left: 0.26},
		},
		{
			name:   "resets sooner, just over 2% at risk: moves",
			own:    work,
			choice: weekly("side", 0.97, 2*day, 0),
			want:   score.Move{Verdict: score.Moves, To: "side", Left: 0.03},
		},
		{
			name:   "resets sooner, its rate unknown: stays",
			own:    work,
			choice: unrated(weekly("side", 0.1, 18*time.Hour, 0)),
			want:   score.Move{Verdict: score.StaysUnknown, To: "side"},
		},
		{
			name:   "resets later, its rate unknown: stays, as it resets later",
			own:    work,
			choice: unrated(weekly("side", 0.5, 6*day, 0)),
			want:   score.Move{Verdict: score.StaysResetsLater, To: "side"},
		},
		{
			name:   "the same account: stays",
			own:    work,
			choice: work,
			want:   score.Move{Verdict: score.StaysSame, To: "work"},
		},
		{
			name:   "resets later: stays",
			own:    work,
			choice: weekly("side", 0.5, 6*day, 0),
			want:   score.Move{Verdict: score.StaysResetsLater, To: "side", Left: 0.5},
		},
		{
			name:   "resets at the same time: stays",
			own:    work,
			choice: weekly("side", 0.5, 5*day, 0),
			want:   score.Move{Verdict: score.StaysResetsLater, To: "side", Left: 0.5},
		},
		{
			name:   "2% at risk exactly: stays",
			own:    work,
			choice: weekly("side", 0.98, 2*day, 0),
			want:   score.Move{Verdict: score.StaysNotAtRisk, To: "side", Left: 0.02},
		},
		{
			name:   "used at a rate that leaves none: stays",
			own:    work,
			choice: weekly("side", 0.5, 2*day, 0.02),
			want:   score.Move{Verdict: score.StaysNotAtRisk, To: "side"},
		},
		{
			name:   "its room at risk only past its reserve: stays",
			own:    work,
			choice: reserving(weekly("side", 0.9, 2*day, 0), 0.1),
			want:   score.Move{Verdict: score.StaysNotAtRisk, To: "side"},
		},
		{
			name:   "its reserve spent, as a pin spends it: moves",
			own:    work,
			choice: weekly("side", 0.9, 2*day, 0),
			want:   score.Move{Verdict: score.Moves, To: "side", Left: 0.1},
		},
		{
			name:   "the session's account's reset unknown: stays",
			own:    withWeek("work", quota.Window{Key: "7d", Utilization: 0.5}, 0),
			choice: weekly("side", 0.5, 2*day, 0),
			want:   score.Move{Verdict: score.StaysUnknown, To: "side", Left: 0.5},
		},
		{
			name:   "the session's account's week unread: stays",
			own:    score.Candidate{ID: "work"},
			choice: weekly("side", 0.5, 2*day, 0),
			want:   score.Move{Verdict: score.StaysUnknown, To: "side", Left: 0.5},
		},
		{
			name:   "the session's account's week reset since it was read: stays",
			own:    weekly("work", 0.5, -time.Hour, 0),
			choice: weekly("side", 0.5, 2*day, 0),
			want:   score.Move{Verdict: score.StaysUnknown, To: "side", Left: 0.5},
		},
		{
			name:   "the choice's reset unknown: stays",
			own:    work,
			choice: withWeek("side", quota.Window{Key: "7d", Utilization: 0.5}, 0),
			want:   score.Move{Verdict: score.StaysUnknown, To: "side"},
		},
		{
			name:   "the choice's week reset since it was read: stays",
			own:    work,
			choice: weekly("side", 0.5, -time.Hour, 0.01),
			want:   score.Move{Verdict: score.StaysUnknown, To: "side"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.AtCompaction(tt.own, tt.choice, now)
			if got.Verdict != tt.want.Verdict || got.To != tt.want.To || math.Abs(got.Left-tt.want.Left) > 1e-9 {
				t.Errorf("AtCompaction(%+v, %+v) = %+v, want %+v", tt.own, tt.choice, got, tt.want)
			}
		})
	}
}

// weekly returns a candidate whose week is at utilization, resets after
// resetsIn, and has been used lately at rate, a share of it an hour.
func weekly(id string, utilization float64, resetsIn time.Duration, rate float64) score.Candidate {
	return withWeek(id, window("7d", utilization, resetsIn), rate)
}

// withWeek returns a candidate whose session has used a fifth of it, its week
// as given, used lately at rate.
func withWeek(id string, w quota.Window, rate float64) score.Candidate {
	return score.Candidate{ID: id, Windows: []quota.Window{window("5h", 0.2, 3*time.Hour), w}, PerishableRate: rate, PerishableKnown: true}
}

// unrated is c, how fast its week has been used lately not known.
func unrated(c score.Candidate) score.Candidate {
	c.PerishableRate, c.PerishableKnown = 0, false
	return c
}
