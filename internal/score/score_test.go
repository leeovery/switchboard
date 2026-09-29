package score_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// now is the time by the clock in every test: a Monday, 13:12 UTC.
var now = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

var policy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d"}

func TestPolicyIsShared(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{key: "5h", want: true},
		{key: "7d", want: true},
		{key: "7d_oi", want: false},
		{key: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := policy.IsShared(tt.key); got != tt.want {
				t.Errorf("IsShared(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestAvailable(t *testing.T) {
	tests := []struct {
		name    string
		windows []quota.Window
		// reserve is the share of each window left unused.
		reserve float64
		want    bool
	}{
		{
			name:    "every window with room",
			windows: []quota.Window{window("5h", 0.5, 3*time.Hour), window("7d", 0.99, 72*time.Hour)},
			want:    true,
		},
		{
			name:    "a window used up",
			windows: []quota.Window{window("5h", 0.5, 3*time.Hour), window("7d", 1, 72*time.Hour)},
			want:    false,
		},
		{
			name:    "a window over its limit",
			windows: []quota.Window{window("5h", 1.2, 3*time.Hour), window("7d", 0.5, 72*time.Hour)},
			want:    false,
		},
		{
			name:    "a window refused below its limit",
			windows: []quota.Window{refused(window("5h", 0.3, 3*time.Hour)), window("7d", 0.5, 72*time.Hour)},
			want:    false,
		},
		{
			name:    "a window warned but allowed",
			windows: []quota.Window{withStatus(window("5h", 0.95, 3*time.Hour), quota.StatusAllowedWarning), window("7d", 0.5, 72*time.Hour)},
			want:    true,
		},
		{
			name:    "a used-up window that doesn't apply",
			windows: []quota.Window{window("5h", 0.5, 3*time.Hour), window("7d", 0.5, 72*time.Hour), refused(window("7d_oi", 1, 72*time.Hour))},
			want:    true,
		},
		{
			name:    "a used-up window that has reset since it was read",
			windows: []quota.Window{refused(window("5h", 1, -time.Minute)), window("7d", 0.5, 72*time.Hour)},
			want:    true,
		},
		{
			name:    "a used-up window resetting now",
			windows: []quota.Window{refused(window("5h", 1, 0)), window("7d", 0.5, 72*time.Hour)},
			want:    true,
		},
		{
			name:    "a used-up window whose reset is unknown",
			windows: []quota.Window{{Key: "5h", Utilization: 1}, window("7d", 0.5, 72*time.Hour)},
			want:    false,
		},
		{
			name:    "no window that applies",
			windows: []quota.Window{window("7d_oi", 0.2, 72*time.Hour)},
			want:    true,
		},
		{
			name:    "no windows",
			windows: nil,
			want:    false,
		},
		{
			name:    "every window short of the reserve",
			windows: []quota.Window{window("5h", 0.89, 3*time.Hour), window("7d", 0.5, 72*time.Hour)},
			reserve: 0.1,
			want:    true,
		},
		{
			name:    "a window at the reserve",
			windows: []quota.Window{window("5h", 0.9, 3*time.Hour), window("7d", 0.5, 72*time.Hour)},
			reserve: 0.1,
			want:    false,
		},
		{
			name:    "a window past the reserve, short of its limit",
			windows: []quota.Window{window("5h", 0.5, 3*time.Hour), window("7d", 0.97, 72*time.Hour)},
			reserve: 0.1,
			want:    false,
		},
		{
			name:    "a window past a reserve of its own size",
			windows: []quota.Window{window("5h", 0.75, 3*time.Hour), window("7d", 0.5, 72*time.Hour)},
			reserve: 0.25,
			want:    false,
		},
		{
			name:    "a window used up, whatever the reserve",
			windows: []quota.Window{window("5h", 0.5, 3*time.Hour), window("7d", 1, 72*time.Hour)},
			reserve: 0.1,
			want:    false,
		},
		{
			name:    "a window at the reserve that doesn't apply",
			windows: []quota.Window{window("5h", 0.5, 3*time.Hour), window("7d", 0.5, 72*time.Hour), window("7d_oi", 0.95, 72*time.Hour)},
			reserve: 0.1,
			want:    true,
		},
		{
			name:    "a window at the reserve that has reset since it was read",
			windows: []quota.Window{window("5h", 0.95, -time.Minute), window("7d", 0.5, 72*time.Hour)},
			reserve: 0.1,
			want:    true,
		},
		{
			name:    "a window at 99% without a reserve",
			windows: []quota.Window{window("5h", 0.99, 3*time.Hour), window("7d", 0.5, 72*time.Hour)},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.Available(tt.windows, tt.reserve, policy.IsShared, now); got != tt.want {
				t.Errorf("Available(reserve %v) = %v, want %v", tt.reserve, got, tt.want)
			}
		})
	}
}

func TestAtReserve(t *testing.T) {
	tests := []struct {
		name    string
		windows []quota.Window
		reserve float64
		want    []string
	}{
		{
			name:    "the windows at or past the reserve, in order",
			windows: []quota.Window{window("5h", 0.9, 3*time.Hour), window("7d", 0.89, 72*time.Hour), window("7d_oi", 0.97, 72*time.Hour)},
			reserve: 0.1,
			want:    []string{"5h", "7d_oi"},
		},
		{
			name:    "but those spent, which their limits hold back",
			windows: []quota.Window{window("5h", 0.93, 3*time.Hour), window("7d", 1.02, 72*time.Hour), refused(window("7d_oi", 0.95, 72*time.Hour))},
			reserve: 0.1,
			want:    []string{"5h"},
		},
		{
			name:    "whether they apply to every model or not",
			windows: []quota.Window{window("5h", 0.2, 3*time.Hour), window("7d_oi", 0.95, 72*time.Hour)},
			reserve: 0.1,
			want:    []string{"7d_oi"},
		},
		{
			name:    "but one that has reset since it was read",
			windows: []quota.Window{window("5h", 0.95, 0), window("7d", 0.95, 72*time.Hour)},
			reserve: 0.1,
			want:    []string{"7d"},
		},
		{
			name:    "one whose reset isn't known",
			windows: []quota.Window{{Key: "5h", Utilization: 0.93}},
			reserve: 0.1,
			want:    []string{"5h"},
		},
		{
			name:    "none short of it",
			windows: []quota.Window{window("5h", 0.89, 3*time.Hour)},
			reserve: 0.1,
		},
		{
			name:    "none without a reserve, used up or not",
			windows: []quota.Window{window("5h", 1, 3*time.Hour), window("7d", 0.99, 72*time.Hour)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.AtReserve(tt.windows, tt.reserve, now); !slices.Equal(got, tt.want) {
				t.Errorf("AtReserve(reserve %v) = %q, want %q", tt.reserve, got, tt.want)
			}
		})
	}
}

func TestLetGo(t *testing.T) {
	tests := []struct {
		name    string
		windows []quota.Window
		want    time.Time
		wantOK  bool
	}{
		{
			name:    "at the reset of the one window at the reserve",
			windows: []quota.Window{window("5h", 0.92, 3*time.Hour), window("7d", 0.5, 72*time.Hour)},
			want:    now.Add(3 * time.Hour), wantOK: true,
		},
		{
			name:    "at the last reset of the windows at the reserve",
			windows: []quota.Window{window("5h", 0.92, 3*time.Hour), window("7d", 0.95, 72*time.Hour)},
			want:    now.Add(72 * time.Hour), wantOK: true,
		},
		{
			name:    "passing over a window at the reserve that doesn't apply",
			windows: []quota.Window{window("5h", 0.92, 3*time.Hour), window("7d", 0.5, 72*time.Hour), window("7d_oi", 0.95, 96*time.Hour)},
			want:    now.Add(3 * time.Hour), wantOK: true,
		},
		{
			name:    "passing over a window at the reserve that has reset since it was read",
			windows: []quota.Window{window("5h", 0.92, -time.Minute), window("7d", 0.95, 72*time.Hour)},
			want:    now.Add(72 * time.Hour), wantOK: true,
		},
		{
			name:    "unknown, when a window at the reserve doesn't say when it resets",
			windows: []quota.Window{window("5h", 0.92, 3*time.Hour), {Key: "7d", Utilization: 0.95}},
		},
		{
			name:    "none, with no window at the reserve",
			windows: []quota.Window{window("5h", 0.5, 3*time.Hour), window("7d", 0.5, 72*time.Hour)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := score.LetGo(tt.windows, 0.1, policy.IsShared, now)
			if !got.Equal(tt.want) || ok != tt.wantOK {
				t.Errorf("LetGo() = %v, %v, want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestPerishability(t *testing.T) {
	tests := []struct {
		name    string
		windows []quota.Window
		// reserve is the share of each window left unused.
		reserve float64
		// want is per hour.
		want   float64
		wantOK bool
	}{
		{name: "half left, resetting in five days: 10% a day", windows: []quota.Window{window("7d", 0.5, 5*24*time.Hour)}, want: 0.10 / 24, wantOK: true},
		{name: "half left, resetting tomorrow: 50% a day", windows: []quota.Window{window("7d", 0.5, 24*time.Hour)}, want: 0.50 / 24, wantOK: true},
		{name: "all left", windows: []quota.Window{window("7d", 0, 84*time.Hour)}, want: 1.0 / 84, wantOK: true},
		{name: "none left", windows: []quota.Window{window("7d", 1, 24*time.Hour)}, want: 0, wantOK: true},
		{name: "over the limit, none left", windows: []quota.Window{window("7d", 1.2, 24*time.Hour)}, want: 0, wantOK: true},
		{name: "a reading below zero, all left", windows: []quota.Window{window("7d", -0.2, 24*time.Hour)}, want: 1.0 / 24, wantOK: true},
		{name: "a reset minutes away counts as an hour away", windows: []quota.Window{window("7d", 0.5, 10*time.Minute)}, want: 0.5, wantOK: true},
		{name: "a reset an hour away", windows: []quota.Window{window("7d", 0.5, time.Hour)}, want: 0.5, wantOK: true},
		{name: "reset since it was read: all left, a week away", windows: []quota.Window{window("7d", 0.9, -time.Hour)}, want: 1.0 / 168, wantOK: true},
		{name: "resetting now: all left, a week away", windows: []quota.Window{window("7d", 0.9, 0)}, want: 1.0 / 168, wantOK: true},
		{
			name:    "measured on the perishable window alone",
			windows: []quota.Window{window("5h", 0.9, time.Hour), window("7d", 0.5, 24*time.Hour), window("7d_oi", 0.1, 10*time.Hour)},
			want:    0.50 / 24,
			wantOK:  true,
		},
		{name: "window missing", windows: []quota.Window{window("5h", 0.5, 3*time.Hour)}, wantOK: false},
		{name: "reset unknown", windows: []quota.Window{{Key: "7d", Utilization: 0.5}}, wantOK: false},
		{name: "no windows", windows: nil, wantOK: false},
		{name: "half used, with a tenth kept back, resetting tomorrow: 40% a day", windows: []quota.Window{window("7d", 0.5, 24*time.Hour)}, reserve: 0.1, want: 0.40 / 24, wantOK: true},
		{name: "at the reserve, none left", windows: []quota.Window{window("7d", 0.9, 24*time.Hour)}, reserve: 0.1, want: 0, wantOK: true},
		{name: "past the reserve, none left", windows: []quota.Window{window("7d", 0.95, 24*time.Hour)}, reserve: 0.1, want: 0, wantOK: true},
		{name: "reset since it was read: all but the reserve, a week away", windows: []quota.Window{window("7d", 0.95, -time.Hour)}, reserve: 0.1, want: 0.9 / 168, wantOK: true},
		{name: "a reading below zero, all but the reserve", windows: []quota.Window{window("7d", -0.2, 24*time.Hour)}, reserve: 0.1, want: 0.9 / 24, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := policy.Perishability(tt.windows, tt.reserve, now)
			if math.Abs(got-tt.want) > 1e-12 || ok != tt.wantOK {
				t.Errorf("Perishability(reserve %v) = %v, %v, want %v, %v", tt.reserve, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestPerishabilityOfAWindowWithoutALength(t *testing.T) {
	burst := score.Policy{Shared: []string{"burst"}, Perishable: "burst"}

	if got, ok := burst.Perishability([]quota.Window{window("burst", 0.5, time.Hour)}, 0, now); ok {
		t.Errorf("Perishability() = %v, true, want false: the window's length is unknown", got)
	}
}

func TestPick(t *testing.T) {
	allApply := func(string) bool { return true }
	tests := []struct {
		name       string
		candidates []score.Candidate
		// applies is policy.IsShared when nil.
		applies   func(string) bool
		preferred string
		want      string
		wantOK    bool
	}{
		{
			name:       "the account whose week resets soonest",
			candidates: []score.Candidate{account("a", 0.1, 0.5, 5*24*time.Hour), account("b", 0.1, 0.5, 24*time.Hour)},
			want:       "b", wantOK: true,
		},
		{
			name:       "a nearly used-up account scores low whatever its reset",
			candidates: []score.Candidate{account("a", 0.1, 0.95, 24*time.Hour), account("b", 0.1, 0.5, 5*24*time.Hour)},
			want:       "b", wantOK: true,
		},
		{
			name:       "passing over an account that can't take a request",
			candidates: []score.Candidate{account("a", 0.1, 0.5, 5*24*time.Hour), account("b", 1, 0.5, 24*time.Hour)},
			want:       "a", wantOK: true,
		},
		{
			name:       "passing over an account without a score",
			candidates: []score.Candidate{account("a", 0.1, 0.5, 5*24*time.Hour), {ID: "b", Windows: []quota.Window{window("5h", 0.1, 3*time.Hour)}}},
			want:       "a", wantOK: true,
		},
		{
			name: "a used-up window that doesn't apply",
			candidates: []score.Candidate{
				account("a", 0.1, 0.5, 5*24*time.Hour),
				account("b", 0.1, 0.5, 24*time.Hour, refused(window("7d_oi", 1, 48*time.Hour))),
			},
			want: "b", wantOK: true,
		},
		{
			name: "a used-up window that applies",
			candidates: []score.Candidate{
				account("a", 0.1, 0.5, 5*24*time.Hour),
				account("b", 0.1, 0.5, 24*time.Hour, refused(window("7d_oi", 1, 48*time.Hour))),
			},
			applies: allApply,
			want:    "a", wantOK: true,
		},
		{
			name:       "no window that applies, leaving the score alone to decide",
			candidates: []score.Candidate{account("a", 1, 0.5, 5*24*time.Hour), account("b", 0.1, 0.5, 24*time.Hour)},
			applies:    func(string) bool { return false },
			want:       "b", wantOK: true,
		},
		{
			name:       "a tie goes to the emptier session",
			candidates: []score.Candidate{account("a", 0.4, 0.5, 48*time.Hour), account("b", 0.2, 0.5, 48*time.Hour)},
			want:       "b", wantOK: true,
		},
		{
			name:       "a tie goes to the emptier session given first",
			candidates: []score.Candidate{account("b", 0.2, 0.5, 48*time.Hour), account("a", 0.4, 0.5, 48*time.Hour)},
			want:       "b", wantOK: true,
		},
		{
			name:       "scores equal but for rounding are a tie",
			candidates: []score.Candidate{account("a", 0.4, 0.7, 30*time.Hour), account("b", 0.2, 0.9, 10*time.Hour)},
			want:       "b", wantOK: true,
		},
		{
			name: "a session that has reset since it was read is empty in a tie",
			candidates: []score.Candidate{
				account("a", 0.2, 0.5, 48*time.Hour),
				{ID: "b", Windows: []quota.Window{refused(window("5h", 1, -time.Minute)), window("7d", 0.5, 48*time.Hour)}},
			},
			want: "b", wantOK: true,
		},
		{
			name: "a tie is broken on the shortest window that applies",
			candidates: []score.Candidate{
				account("a", 0.2, 0.5, 48*time.Hour, window("1h", 0.9, 30*time.Minute)),
				account("b", 0.4, 0.5, 48*time.Hour, window("1h", 0.1, 30*time.Minute)),
			},
			applies: allApply,
			want:    "b", wantOK: true,
		},
		{
			name: "a shorter window that doesn't apply leaves a tie alone",
			candidates: []score.Candidate{
				account("a", 0.2, 0.5, 48*time.Hour, window("1h", 0.9, 30*time.Minute)),
				account("b", 0.4, 0.5, 48*time.Hour, window("1h", 0.1, 30*time.Minute)),
			},
			want: "a", wantOK: true,
		},
		{
			name:       "a full tie goes to the first given",
			candidates: []score.Candidate{account("a", 0.2, 0.5, 48*time.Hour), account("b", 0.2, 0.5, 48*time.Hour)},
			want:       "a", wantOK: true,
		},
		{
			name:       "a full tie goes to the first given, whichever it is",
			candidates: []score.Candidate{account("b", 0.2, 0.5, 48*time.Hour), account("a", 0.2, 0.5, 48*time.Hour)},
			want:       "b", wantOK: true,
		},
		{
			name:       "the preferred account kept against one scoring 19% higher",
			candidates: []score.Candidate{account("a", 0.1, 0.5, 50*time.Hour), account("b", 0.1, 0.405, 50*time.Hour)},
			preferred:  "a",
			want:       "a", wantOK: true,
		},
		{
			name:       "the preferred account left for one scoring 20% higher",
			candidates: []score.Candidate{account("a", 0.1, 0.5, 50*time.Hour), account("b", 0.1, 0.4, 50*time.Hour)},
			preferred:  "a",
			want:       "b", wantOK: true,
		},
		{
			name:       "the preferred account left for one scoring 21% higher",
			candidates: []score.Candidate{account("a", 0.1, 0.5, 50*time.Hour), account("b", 0.1, 0.395, 50*time.Hour)},
			preferred:  "a",
			want:       "b", wantOK: true,
		},
		{
			name:       "the preferred account kept in a tie it would lose",
			candidates: []score.Candidate{account("a", 0.4, 0.5, 48*time.Hour), account("b", 0.2, 0.5, 48*time.Hour)},
			preferred:  "a",
			want:       "a", wantOK: true,
		},
		{
			name:       "the preferred account kept as the best",
			candidates: []score.Candidate{account("a", 0.1, 0.5, 24*time.Hour), account("b", 0.1, 0.5, 5*24*time.Hour)},
			preferred:  "a",
			want:       "a", wantOK: true,
		},
		{
			name:       "a preferred account that can't take a request",
			candidates: []score.Candidate{account("a", 1, 0.5, 24*time.Hour), account("b", 0.1, 0.5, 5*24*time.Hour)},
			preferred:  "a",
			want:       "b", wantOK: true,
		},
		{
			name:       "a preferred account without a score",
			candidates: []score.Candidate{{ID: "a", Windows: []quota.Window{window("5h", 0.1, 3*time.Hour)}}, account("b", 0.1, 0.5, 5*24*time.Hour)},
			preferred:  "a",
			want:       "b", wantOK: true,
		},
		{
			name:       "a preferred account that isn't a candidate",
			candidates: []score.Candidate{account("a", 0.1, 0.5, 5*24*time.Hour), account("b", 0.1, 0.5, 24*time.Hour)},
			preferred:  "gone",
			want:       "b", wantOK: true,
		},
		{
			name:       "no candidates",
			candidates: nil,
			want:       "", wantOK: false,
		},
		{
			name:       "no candidate that can take a request",
			candidates: []score.Candidate{account("a", 1, 0.5, 24*time.Hour), account("b", 0.1, 1, 24*time.Hour)},
			preferred:  "a",
			want:       "", wantOK: false,
		},
		{
			name: "no candidate with a score",
			candidates: []score.Candidate{
				{ID: "a", Windows: []quota.Window{window("5h", 0.1, 3*time.Hour)}},
				{ID: "b", Windows: []quota.Window{window("5h", 0.1, 3*time.Hour), {Key: "7d", Utilization: 0.5}}},
			},
			want: "", wantOK: false,
		},
		{
			name:       "passing over an account at its reserve",
			candidates: []score.Candidate{reserving(account("a", 0.91, 0.5, 24*time.Hour), 0.1), account("b", 0.1, 0.5, 5*24*time.Hour)},
			want:       "b", wantOK: true,
		},
		{
			name:       "an account short of its reserve",
			candidates: []score.Candidate{reserving(account("a", 0.89, 0.5, 24*time.Hour), 0.1), account("b", 0.1, 0.5, 5*24*time.Hour)},
			want:       "a", wantOK: true,
		},
		{
			name:       "an account's reserve left out of its score",
			candidates: []score.Candidate{reserving(account("a", 0.1, 0.5, 24*time.Hour), 0.2), account("b", 0.1, 0.5, 30*time.Hour)},
			want:       "b", wantOK: true,
		},
		{
			name:       "a preferred account at its reserve",
			candidates: []score.Candidate{reserving(account("a", 0.1, 0.93, 24*time.Hour), 0.1), account("b", 0.1, 0.5, 5*24*time.Hour)},
			preferred:  "a",
			want:       "b", wantOK: true,
		},
		{
			name:       "no candidate short of its reserve",
			candidates: []score.Candidate{reserving(account("a", 0.1, 0.95, 24*time.Hour), 0.1), account("b", 1, 0.5, 24*time.Hour)},
			want:       "", wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applies := tt.applies
			if applies == nil {
				applies = policy.IsShared
			}
			got, ok := policy.Pick(tt.candidates, applies, tt.preferred, now)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("Pick() = %q, %v, want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// account returns a candidate whose session window is at session, and whose
// week is at week and resets after weekLeft, followed by more windows.
func account(id string, session, week float64, weekLeft time.Duration, more ...quota.Window) score.Candidate {
	windows := []quota.Window{window("5h", session, 3*time.Hour), window("7d", week, weekLeft)}
	return score.Candidate{ID: id, Windows: append(windows, more...)}
}

// reserving is c, leaving reserve of every window unused.
func reserving(c score.Candidate, reserve float64) score.Candidate {
	c.Reserve = reserve
	return c
}

// window returns a window at utilization that resets after resetsIn.
func window(key string, utilization float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: key, Utilization: utilization, ResetsAt: now.Add(resetsIn)}
}

// session returns a five-hour window at utilization that began passed ago.
func session(utilization float64, passed time.Duration) quota.Window {
	return window("5h", utilization, 5*time.Hour-passed)
}

// week returns a seven-day window at utilization that began passed ago.
func week(utilization float64, passed time.Duration) quota.Window {
	return window("7d", utilization, 7*24*time.Hour-passed)
}

func refused(w quota.Window) quota.Window {
	return withStatus(w, quota.StatusRejected)
}

func withStatus(w quota.Window, status quota.Status) quota.Window {
	w.Status = status
	return w
}
