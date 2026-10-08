package score_test

import (
	"math"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

func TestElapsed(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		want   float64
		wantOK bool
	}{
		{name: "at the start", window: session(0.2, 0), want: 0, wantOK: true},
		{name: "halfway", window: session(0.2, 150*time.Minute), want: 0.5, wantOK: true},
		{name: "a quarter of a week", window: week(0.2, 42*time.Hour), want: 0.25, wantOK: true},
		{name: "at the reset", window: session(0.2, 5*time.Hour), want: 1, wantOK: true},
		{name: "past the reset", window: session(0.2, 6*time.Hour), want: 1, wantOK: true},
		{name: "before the start", window: session(0.2, -time.Hour), want: 0, wantOK: true},
		{name: "length unknown", window: window("burst", 0.2, time.Hour), wantOK: false},
		{name: "reset unknown", window: quota.Window{Key: "5h", Utilization: 0.2}, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := score.Elapsed(tt.window, now)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("Elapsed() = %v, %v, want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestProject(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		want   score.Projection
	}{
		{name: "on pace", window: session(0.46, 150*time.Minute), want: score.Projection{Kind: score.OnPace, AtReset: 0.92}},
		{name: "on pace, having used nothing", window: session(0, 150*time.Minute), want: score.Projection{Kind: score.OnPace}},
		{name: "runs out", window: session(0.4, time.Hour), want: score.Projection{Kind: score.RunsOut, At: now.Add(90 * time.Minute)}},
		{name: "on pace for its limit, reaching it just as it resets: no run-out", window: session(0.2, time.Hour), want: score.Projection{Kind: score.OnPace, AtReset: 1}},
		{name: "exactly 5% passed", window: session(0.01, 15*time.Minute), want: score.Projection{Kind: score.OnPace, AtReset: 0.2}},
		{name: "just under 5% passed", window: session(0.01, 15*time.Minute-time.Second), want: score.Projection{}},
		{name: "used up exactly", window: session(1, 2*time.Hour), want: score.Projection{Kind: score.Exhausted, At: now.Add(3 * time.Hour)}},
		{name: "over the limit", window: session(1.04, 2*time.Hour), want: score.Projection{Kind: score.Exhausted, At: now.Add(3 * time.Hour)}},
		{name: "refused below the limit", window: refused(session(0.3, 2*time.Hour)), want: score.Projection{Kind: score.Exhausted, At: now.Add(3 * time.Hour)}},
		{name: "exhausted before 5% passed", window: session(1, 5*time.Minute), want: score.Projection{Kind: score.Exhausted, At: now.Add(295 * time.Minute)}},
		{name: "exhausted, back at an unknown time", window: quota.Window{Key: "5h", Utilization: 1}, want: score.Projection{Kind: score.Exhausted}},
		{name: "exhausted, length unknown", window: window("burst", 1, time.Hour), want: score.Projection{Kind: score.Exhausted, At: now.Add(time.Hour)}},
		{name: "reset since it was read", window: session(0.5, 6*time.Hour), want: score.Projection{}},
		{name: "exhausted, but reset since it was read", window: refused(session(1, 6*time.Hour)), want: score.Projection{}},
		{name: "resetting now", window: session(0.5, 5*time.Hour), want: score.Projection{}},
		{name: "reset unknown", window: quota.Window{Key: "5h", Utilization: 0.5}, want: score.Projection{}},
		{name: "length unknown", window: window("burst", 0.5, time.Hour), want: score.Projection{}},
		{name: "not yet begun", window: session(0.5, -time.Hour), want: score.Projection{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.Project(tt.window, now); !sameProjection(got, tt.want) {
				t.Errorf("Project() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestProjectRunsOutAt(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		want   time.Time
	}{
		{name: "40% in an hour leaves 60% for 1h 30m", window: session(0.4, time.Hour), want: time.Date(2026, 9, 28, 14, 42, 0, 0, time.UTC)},
		{name: "75% in 3h leaves 25% for an hour", window: session(0.75, 3*time.Hour), want: time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC)},
		{name: "90% in 30m leaves 10% for 3m 20s", window: session(0.9, 30*time.Minute), want: time.Date(2026, 9, 28, 13, 15, 20, 0, time.UTC)},
		{name: "50% in two days of a week leaves 50% for two more", window: week(0.5, 48*time.Hour), want: time.Date(2026, 9, 30, 13, 12, 0, 0, time.UTC)},
		{name: "25% in a day of a week leaves 75% for three more", window: week(0.25, 24*time.Hour), want: time.Date(2026, 10, 1, 13, 12, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := score.Project(tt.window, now)
			if got.Kind != score.RunsOut || !got.At.Equal(tt.want) {
				t.Errorf("Project() = %+v, want it to run out at %v", got, tt.want)
			}
		})
	}
}

func TestReaches(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		floor  float64
		want   time.Time
		wantOK bool
	}{
		{name: "running out, its limit as it runs out", window: session(0.4, time.Hour), floor: 1, want: now.Add(90 * time.Minute), wantOK: true},
		{name: "running out, a reserve before it", window: session(0.4, time.Hour), floor: 0.9, want: now.Add(75 * time.Minute), wantOK: true},
		{name: "on pace past a reserve", window: session(0.3, 2*time.Hour), floor: 0.6, want: now.Add(2 * time.Hour), wantOK: true},
		{name: "on pace short of a reserve", window: session(0.46, 150*time.Minute), floor: 0.95},
		{name: "on pace, ending at the reserve as it resets", window: session(0.45, 150*time.Minute), floor: 0.9},
		{name: "at its reserve already", window: session(0.92, 4*time.Hour), floor: 0.9},
		{name: "used up", window: session(1, 2*time.Hour), floor: 0.9},
		{name: "saying nothing", window: session(0.5, 6*time.Hour), floor: 0.9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := score.Reaches(tt.window, score.Project(tt.window, now), tt.floor, now)
			if !got.Equal(tt.want) || ok != tt.wantOK {
				t.Errorf("Reaches() = %v, %v, want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestARunOutCountsOnlyBeforeTheReset(t *testing.T) {
	// Half used, at a quarter of the window an hour from now on: its limit in
	// two hours, and three quarters of it, a reserve's floor, in one.
	const rate = 0.25
	tests := []struct {
		name     string
		resetsIn time.Duration
		floor    float64
		want     score.Projection
		// wantAt is when it reaches the floor, as Reaches has it.
		wantAt time.Time
		wantOK bool
	}{
		{name: "its limit a minute before the reset", resetsIn: 121 * time.Minute, floor: 1, want: score.Projection{Kind: score.RunsOut, At: now.Add(2 * time.Hour)}, wantAt: now.Add(2 * time.Hour), wantOK: true},
		{name: "its limit just as it resets", resetsIn: 2 * time.Hour, floor: 1, want: score.Projection{Kind: score.OnPace, AtReset: 1}},
		{name: "its limit a minute after the reset", resetsIn: 119 * time.Minute, floor: 1, want: score.Projection{Kind: score.OnPace, AtReset: 0.5 + rate*119/60}},
		{name: "a reserve a minute before the reset", resetsIn: 61 * time.Minute, floor: 0.75, want: score.Projection{Kind: score.OnPace, AtReset: 0.5 + rate*61/60}, wantAt: now.Add(time.Hour), wantOK: true},
		{name: "a reserve just as it resets", resetsIn: time.Hour, floor: 0.75, want: score.Projection{Kind: score.OnPace, AtReset: 0.75}},
		{name: "a reserve a minute after the reset", resetsIn: 59 * time.Minute, floor: 0.75, want: score.Projection{Kind: score.OnPace, AtReset: 0.5 + rate*59/60}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := window("5h", 0.5, tt.resetsIn)
			p := score.ProjectAt(w, rate, now)
			if !sameProjection(p, tt.want) {
				t.Errorf("ProjectAt() = %+v, want %+v", p, tt.want)
			}
			if got, ok := score.Reaches(w, p, tt.floor, now); !got.Equal(tt.wantAt) || ok != tt.wantOK {
				t.Errorf("Reaches() = %v, %v, want %v, %v", got, ok, tt.wantAt, tt.wantOK)
			}
		})
	}
}

func TestAFloorReachedJustAsTheWindowResetsIsntReachedThoughItsShareRoundsPast(t *testing.T) {
	// At the rate that ends it at 90% as it resets in 22 minutes, 1% used
	// heads a few ulps past 90%, as an experiment found.
	w := window("5h", 0.01, 22*time.Minute)
	p := score.ProjectAt(w, (0.9-0.01)/(22*time.Minute).Hours(), now)
	if p.Kind != score.OnPace || p.AtReset <= 0.9 {
		t.Fatalf("ProjectAt() = %+v, want OnPace a hair past 0.9, the case this is of", p)
	}
	if got, ok := score.Reaches(w, p, 0.9, now); ok {
		t.Errorf("Reaches() = %v, true, want false: it reaches 0.9 as it resets, at %v", got, w.ResetsAt)
	}
}

func TestSooner(t *testing.T) {
	runsOut := func(in time.Duration) score.Projection { return score.Projection{Kind: score.RunsOut, At: now.Add(in)} }
	onPace := func(atReset float64) score.Projection { return score.Projection{Kind: score.OnPace, AtReset: atReset} }
	tests := []struct {
		name string
		a, b score.Projection
		want bool
	}{
		{name: "running out before", a: runsOut(2 * time.Hour), b: runsOut(time.Hour), want: true},
		{name: "running out after", a: runsOut(time.Hour), b: runsOut(2 * time.Hour)},
		{name: "running out together", a: runsOut(time.Hour), b: runsOut(time.Hour)},
		{name: "running out, against keeping pace", a: onPace(0.9), b: runsOut(time.Hour), want: true},
		{name: "keeping pace, against running out", a: runsOut(time.Hour), b: onPace(0.9)},
		{name: "keeping pace for more", a: onPace(0.6), b: onPace(0.9), want: true},
		{name: "keeping pace for less", a: onPace(0.9), b: onPace(0.6)},
		{name: "saying anything, against nothing", a: score.Projection{}, b: onPace(0.1), want: true},
		{name: "saying nothing, against anything", a: onPace(0.1), b: score.Projection{}},
		{name: "exhausted alike", a: score.Projection{Kind: score.Exhausted, At: now}, b: score.Projection{Kind: score.Exhausted, At: now}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.Sooner(tt.a, tt.b); got != tt.want {
				t.Errorf("Sooner(%+v, %+v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAWindowStartedAgainRunsFromThen(t *testing.T) {
	// The week began three days ago and resets in four, 4% used.
	startedAgain := func(ago time.Duration) quota.Window {
		w := week(0.04, 3*24*time.Hour)
		w.RestartedAt = now.Add(-ago)
		return w
	}
	tests := []struct {
		name        string
		window      quota.Window
		wantElapsed float64
		want        score.Projection
		wantPace    float64
	}{
		{
			name:        "started again six hours ago, of the 102 it runs until its reset",
			window:      startedAgain(6 * time.Hour),
			wantElapsed: 6.0 / 102,
			want:        score.Projection{Kind: score.OnPace, AtReset: 0.68},
			wantPace:    0.04 / 6,
		},
		{
			name:        "started again an hour ago, before 5% of it has passed",
			window:      startedAgain(time.Hour),
			wantElapsed: 1.0 / 97,
		},
		{
			name:        "never started again, from a week before its reset",
			window:      week(0.04, 3*24*time.Hour),
			wantElapsed: 3.0 / 7,
			want:        score.Projection{Kind: score.OnPace, AtReset: 0.04 * 7 / 3},
			wantPace:    0.04 / 72,
		},
		{
			name:        "started again before the week began, from a week before its reset",
			window:      startedAgain(8 * 24 * time.Hour),
			wantElapsed: 3.0 / 7,
			want:        score.Projection{Kind: score.OnPace, AtReset: 0.04 * 7 / 3},
			wantPace:    0.04 / 72,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := score.Elapsed(tt.window, now); math.Abs(got-tt.wantElapsed) > 1e-9 || !ok {
				t.Errorf("Elapsed() = %v, %v, want %v, true", got, ok, tt.wantElapsed)
			}
			if got := score.Project(tt.window, now); !sameProjection(got, tt.want) {
				t.Errorf("Project() = %+v, want %+v", got, tt.want)
			}
			if got, _ := score.PaceOf(tt.window, nil, now); math.Abs(got.Rate-tt.wantPace) > 1e-9 {
				t.Errorf("PaceOf() = %+v, want a rate of %v", got, tt.wantPace)
			}
		})
	}
}

// sameProjection reports whether two projections match, their utilizations
// to within rounding.
func sameProjection(a, b score.Projection) bool {
	return a.Kind == b.Kind && math.Abs(a.AtReset-b.AtReset) < 1e-9 && a.At.Equal(b.At)
}

func TestResetByHand(t *testing.T) {
	held := session(0.3, time.Hour)
	tests := []struct {
		name string
		kept quota.Window
		want bool
	}{
		{name: "ten points lower, its reset the same, allowing for rounding", kept: session(0.2, time.Hour), want: true},
		{name: "emptied, its reset the same", kept: session(0, time.Hour), want: true},
		{name: "nine points lower is noise", kept: session(0.21, time.Hour)},
		{name: "higher", kept: session(0.5, time.Hour)},
		{name: "emptied, with another reset, a new window", kept: session(0, -4*time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.ResetByHand(held, tt.kept); got != tt.want {
				t.Errorf("ResetByHand(%+v, %+v) = %v, want %v", held, tt.kept, got, tt.want)
			}
		})
	}
}
