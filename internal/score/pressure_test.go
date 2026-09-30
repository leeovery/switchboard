package score_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

func TestPaceOf(t *testing.T) {
	// The session began two hours ago, and is 40% used: 20% an hour since it
	// started.
	running := session(0.4, 2*time.Hour)
	tests := []struct {
		name     string
		window   quota.Window
		readings []score.Reading
		want     score.Pace
		wantOK   bool
	}{
		{
			name:     "its recent rate",
			window:   running,
			readings: []score.Reading{readAt(40*time.Minute, 0.3), readAt(0, 0.4)},
			want:     score.Pace{Rate: 0.2, Recent: true}, wantOK: true,
		},
		{
			name:     "no reading 10 minutes back, the use since it started",
			window:   running,
			readings: []score.Reading{readAt(9*time.Minute, 0.38), readAt(0, 0.4)},
			want:     score.Pace{Rate: 0.2}, wantOK: true,
		},
		{
			name:   "no readings, the use since it started",
			window: running,
			want:   score.Pace{Rate: 0.2}, wantOK: true,
		},
		{
			name:     "a reading 10 minutes back, before 5% of the window has passed",
			window:   session(0.03, 12*time.Minute),
			readings: []score.Reading{readAt(10*time.Minute, 0.01), readAt(0, 0.03)},
			want:     score.Pace{Rate: 0.12, Recent: true}, wantOK: true,
		},
		{
			name:   "no readings before 5% of the window has passed",
			window: session(0.03, 14*time.Minute),
		},
		{
			name:     "reset since it was read",
			window:   session(0.4, 6*time.Hour),
			readings: []score.Reading{readAt(20*time.Minute, 0.3), readAt(0, 0.4)},
		},
		{
			name:   "reset unknown",
			window: quota.Window{Key: "5h", Utilization: 0.4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := score.PaceOf(tt.window, tt.readings, now)
			if math.Abs(got.Rate-tt.want.Rate) > 1e-9 || got.Recent != tt.want.Recent || ok != tt.wantOK {
				t.Errorf("PaceOf() = %+v, %v, want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRecentRate(t *testing.T) {
	// The week began three days ago, and is 40% used.
	running := week(0.4, 3*24*time.Hour)
	tests := []struct {
		name     string
		readings []score.Reading
		want     float64
		wantOK   bool
	}{
		{
			name:     "the rise over the half hour from the reading before it",
			readings: []score.Reading{readAt(45*time.Minute, 0.34), readAt(20*time.Minute, 0.37), readAt(0, 0.4)},
			want:     0.12, wantOK: true,
		},
		{
			name:     "the rise from a reading at the half hour's start exactly",
			readings: []score.Reading{readAt(30*time.Minute, 0.37), readAt(0, 0.4)},
			want:     0.06, wantOK: true,
		},
		{
			name:     "no reading since the half hour started, quiet",
			readings: []score.Reading{readAt(2*time.Hour, 0.3), readAt(45*time.Minute, 0.4)},
			want:     0, wantOK: true,
		},
		{
			name:     "read just now, unchanged since before the half hour, quiet",
			readings: []score.Reading{readAt(45*time.Minute, 0.4), readAt(0, 0.4)},
			want:     0, wantOK: true,
		},
		{
			name:     "no reading before the half hour, from the first, 20 minutes back",
			readings: []score.Reading{readAt(20*time.Minute, 0.36), readAt(0, 0.4)},
			want:     0.12, wantOK: true,
		},
		{
			name:     "no reading before the half hour, from the first, 10 minutes back",
			readings: []score.Reading{readAt(10*time.Minute, 0.38), readAt(0, 0.4)},
			want:     0.12, wantOK: true,
		},
		{
			name:     "no reading before the half hour, the first under 10 minutes back",
			readings: []score.Reading{readAt(9*time.Minute, 0.39), readAt(0, 0.4)},
		},
		{
			name:     "a burst of 10 minutes as it ends",
			readings: []score.Reading{readAt(40*time.Minute, 0.3), readAt(10*time.Minute, 0.3), readAt(0, 0.4)},
			want:     0.2, wantOK: true,
		},
		{
			name:     "the same burst, quiet for 20 minutes since",
			readings: []score.Reading{readAt(60*time.Minute, 0.3), readAt(30*time.Minute, 0.3), readAt(20*time.Minute, 0.4)},
			want:     0.2, wantOK: true,
		},
		{
			name:     "the same burst, quiet for 40 minutes since, past",
			readings: []score.Reading{readAt(80*time.Minute, 0.3), readAt(50*time.Minute, 0.3), readAt(40*time.Minute, 0.4)},
			want:     0, wantOK: true,
		},
		{
			name: "steady use, read every 5 minutes",
			readings: []score.Reading{
				readAt(35*time.Minute, 0.335), readAt(30*time.Minute, 0.34), readAt(25*time.Minute, 0.35), readAt(20*time.Minute, 0.36),
				readAt(15*time.Minute, 0.37), readAt(10*time.Minute, 0.38), readAt(5*time.Minute, 0.39), readAt(0, 0.4),
			},
			want: 0.12, wantOK: true,
		},
		{
			name:     "a dip reads no negative rate",
			readings: []score.Reading{readAt(45*time.Minute, 0.41), readAt(0, 0.4)},
			want:     0, wantOK: true,
		},
		{
			name: "no readings",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := score.RecentRate(running, tt.readings, now)
			if math.Abs(got-tt.want) > 1e-9 || ok != tt.wantOK {
				t.Errorf("RecentRate() = %v, %v, want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
	if got, ok := score.RecentRate(week(0.4, 8*24*time.Hour), []score.Reading{readAt(45*time.Minute, 0.37), readAt(0, 0.4)}, now); ok {
		t.Errorf("RecentRate() of a week reset since it was read = %v, true, want false", got)
	}
}

func TestLately(t *testing.T) {
	before, at, since, last := readAt(45*time.Minute, 0.3), readAt(30*time.Minute, 0.31), readAt(20*time.Minute, 0.32), readAt(0, 0.33)
	tests := []struct {
		name     string
		readings []score.Reading
		want     []score.Reading
	}{
		{name: "the last before the half hour and those since", readings: []score.Reading{before, at, since, last}, want: []score.Reading{at, since, last}},
		{name: "those since, with none before", readings: []score.Reading{since, last}, want: []score.Reading{since, last}},
		{name: "the last before, with none since", readings: []score.Reading{before, at}, want: []score.Reading{at}},
		{name: "none", readings: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.Lately(tt.readings, now); !slices.Equal(got, tt.want) {
				t.Errorf("Lately() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPressureOf(t *testing.T) {
	// The session is 40% used, and resets in 3 hours.
	running := []quota.Window{window("5h", 0.4, 3*time.Hour), window("7d", 0.5, 48*time.Hour)}
	tests := []struct {
		name       string
		windows    []quota.Window
		reserve    float64
		rate       float64
		wantOutIn  time.Duration
		wantUnder  bool
		wantSilent bool
	}{
		{name: "runs out before its reset", windows: running, rate: 0.25, wantOutIn: 144 * time.Minute, wantUnder: true},
		{name: "runs out after its reset", windows: running, rate: 0.15, wantOutIn: 4 * time.Hour},
		{name: "runs out at its reset", windows: running, rate: 0.2, wantOutIn: 3 * time.Hour},
		{name: "reaches its reserve before its reset", windows: running, reserve: 0.1, rate: 0.18, wantOutIn: 2*time.Hour + 46*time.Minute + 40*time.Second, wantUnder: true},
		{name: "at the same rate, runs out after its reset without a reserve", windows: running, rate: 0.18, wantOutIn: 3*time.Hour + 20*time.Minute},
		{name: "used at no rate", windows: running, wantSilent: true},
		{name: "at its reserve already", windows: []quota.Window{window("5h", 0.9, 3*time.Hour)}, reserve: 0.1, rate: 0.5, wantSilent: true},
		{name: "used up", windows: []quota.Window{window("5h", 1, 3*time.Hour)}, rate: 0.5, wantSilent: true},
		{name: "refused", windows: []quota.Window{refused(window("5h", 0.4, 3*time.Hour))}, rate: 0.5, wantSilent: true},
		{name: "reset since it was read", windows: []quota.Window{window("5h", 0.4, -time.Minute)}, rate: 0.5, wantSilent: true},
		{name: "reset unknown", windows: []quota.Window{{Key: "5h", Utilization: 0.4}}, rate: 0.5, wantSilent: true},
		{name: "without the window", windows: []quota.Window{window("7d", 0.5, 48*time.Hour)}, rate: 0.5, wantSilent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := score.Pressure{Rate: tt.rate, RunsOut: now.Add(tt.wantOutIn), Under: tt.wantUnder}
			if tt.wantSilent {
				want = score.Pressure{}
			}
			c := score.Candidate{ID: "a", Windows: tt.windows, Reserve: tt.reserve, Rate: tt.rate}
			if got := policy.PressureOf(c, now); got.Rate != want.Rate || !got.RunsOut.Equal(want.RunsOut) || got.Under != want.Under {
				t.Errorf("PressureOf() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestPickSetsAsideAccountsUnderPressure(t *testing.T) {
	// Each account's session is 40% used and resets in 3 hours: at 25% an
	// hour it runs out before then, and at 15% it doesn't. Soon's week
	// resets in a day and later's in five, so soon's quota needs using first.
	soon := func(id string) score.Candidate { return account(id, 0.4, 0.5, 24*time.Hour) }
	later := func(id string) score.Candidate { return account(id, 0.4, 0.5, 5*24*time.Hour) }
	at := func(c score.Candidate, rate float64) score.Candidate {
		c.Rate = rate
		return c
	}
	// Near is the band the tiebreak decides in: a's week scores highest, b's
	// within 0.8 of it, and c's within 0.8 of b's but not of a's; c's session
	// resets soonest, then b's, then a's.
	near := func(id string, week float64, sessionLeft time.Duration) score.Candidate {
		return score.Candidate{ID: id, Windows: []quota.Window{window("5h", 0.1, sessionLeft), window("7d", week, 50*time.Hour)}}
	}
	tests := []struct {
		name       string
		candidates []score.Candidate
		preferred  string
		want       score.Choice
	}{
		{
			name:       "the best under pressure, passed over for the next",
			candidates: []score.Candidate{at(soon("a"), 0.25), later("b")},
			want:       score.Choice{ID: "b", PassedOver: "a"},
		},
		{
			name:       "the best at a rate that runs it out after its reset",
			candidates: []score.Candidate{at(soon("a"), 0.15), later("b")},
			want:       score.Choice{ID: "a"},
		},
		{
			name:       "another under pressure, which changes nothing",
			candidates: []score.Candidate{soon("a"), at(later("b"), 0.25)},
			want:       score.Choice{ID: "a"},
		},
		{
			name:       "every one under pressure, as though none were",
			candidates: []score.Candidate{at(soon("a"), 0.25), at(later("b"), 0.5)},
			want:       score.Choice{ID: "a"},
		},
		{
			name:       "the best under pressure at its reserve",
			candidates: []score.Candidate{at(reserving(soon("a"), 0.1), 0.18), later("b")},
			want:       score.Choice{ID: "b", PassedOver: "a"},
		},
		{
			name:       "the best at the same rate without a reserve",
			candidates: []score.Candidate{at(soon("a"), 0.18), later("b")},
			want:       score.Choice{ID: "a"},
		},
		{
			name:       "the preferred account under pressure, left for one it would have kept against",
			candidates: []score.Candidate{at(account("a", 0.4, 0.5, 50*time.Hour), 0.25), account("b", 0.4, 0.45, 50*time.Hour)},
			preferred:  "a",
			want:       score.Choice{ID: "b", PassedOver: "a"},
		},
		{
			name:       "the preferred account kept, as the one well ahead of it is under pressure",
			candidates: []score.Candidate{account("a", 0.4, 0.45, 50*time.Hour), at(account("b", 0.4, 0.3, 50*time.Hour), 0.25)},
			preferred:  "a",
			want:       score.Choice{ID: "a", PassedOver: "b"},
		},
		{
			name: "the highest under pressure, setting aside the band it set",
			candidates: []score.Candidate{
				at(near("a", 0.3, 4*time.Hour), 0.5),
				near("b", 0.43, 3*time.Hour),
				near("c", 0.54, time.Hour),
			},
			want: score.Choice{ID: "c", PassedOver: "a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The account passed over stands under pressure as its candidate
			// does, at the room it gives, its reserve included.
			want := tt.want
			if i := slices.IndexFunc(tt.candidates, func(c score.Candidate) bool { return c.ID == want.PassedOver }); i >= 0 {
				want.Pressure = policy.PressureOf(tt.candidates[i], now)
			}
			if got, ok := policy.Pick(tt.candidates, policy.IsShared, tt.preferred, now); got != want || !ok {
				t.Errorf("Pick() = %+v, %v, want %+v, true", got, ok, want)
			}
		})
	}
}

func TestProjectAt(t *testing.T) {
	// The session began two hours ago, and is 40% used.
	running := session(0.4, 2*time.Hour)
	tests := []struct {
		name   string
		window quota.Window
		rate   float64
		want   score.Projection
	}{
		{name: "on pace at the rate", window: running, rate: 0.1, want: score.Projection{Kind: score.OnPace, AtReset: 0.7}},
		{name: "runs out at the rate", window: running, rate: 0.3, want: score.Projection{Kind: score.RunsOut, At: now.Add(2 * time.Hour)}},
		{name: "used at no rate, stays as it is", window: running, want: score.Projection{Kind: score.OnPace, AtReset: 0.4}},
		{name: "before 5% has passed, at the rate", window: session(0.02, 5*time.Minute), rate: 0.35, want: score.Projection{Kind: score.RunsOut, At: now.Add(2*time.Hour + 48*time.Minute)}},
		{name: "used up", window: session(1, 2*time.Hour), rate: 0.1, want: score.Projection{Kind: score.Exhausted, At: now.Add(3 * time.Hour)}},
		{name: "reset since it was read", window: session(0.4, 6*time.Hour), rate: 0.1, want: score.Projection{}},
		{name: "reset unknown", window: quota.Window{Key: "5h", Utilization: 0.4}, rate: 0.1, want: score.Projection{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.ProjectAt(tt.window, tt.rate, now); !sameProjection(got, tt.want) {
				t.Errorf("ProjectAt() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// readAt returns a reading at utilization, taken ago before now.
func readAt(ago time.Duration, utilization float64) score.Reading {
	return score.Reading{At: now.Add(-ago), Utilization: utilization}
}
