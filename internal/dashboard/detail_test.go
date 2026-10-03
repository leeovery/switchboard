package dashboard

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// now is a Monday, 13:12 an hour east of UTC.
var now = time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))

func TestDetailGivesWayAsWidthShrinks(t *testing.T) {
	runningOut := quota.Window{Key: "7d", Utilization: 0.28, ResetsAt: now.Add(6 * 24 * time.Hour)}
	backSoon := quota.Window{Key: "5h", Utilization: 1, ResetsAt: now.Add(80 * time.Minute), Status: quota.StatusRejected}
	backInSeconds := quota.Window{Key: "5h", Utilization: 1, ResetsAt: now.Add(7*time.Minute + 42*time.Second), Status: quota.StatusRejected}
	tests := []struct {
		name   string
		window quota.Window
		width  int
		want   string
	}{
		{name: "everything", window: runningOut, width: 46, want: "runs out ~Thu 02:54 · resets in 6d · Sun 13:12"},
		{name: "without the clock", window: runningOut, width: 45, want: "runs out ~Thu 02:54 · resets in 6d"},
		{name: "without the clock, exactly", window: runningOut, width: 34, want: "runs out ~Thu 02:54 · resets in 6d"},
		{name: "the projection alone", window: runningOut, width: 33, want: "runs out ~Thu 02:54"},
		{name: "the projection alone, exactly", window: runningOut, width: 19, want: "runs out ~Thu 02:54"},
		{name: "the projection cut short", window: runningOut, width: 18, want: "runs out ~Thu 02:…"},
		{name: "an ellipsis alone", window: runningOut, width: 1, want: "…"},
		{name: "nothing", window: runningOut, width: 0, want: ""},
		{name: "exhausted, and when it's back", window: backSoon, width: 26, want: "back in 1h 20m · Mon 14:32"},
		{name: "exhausted, without the clock", window: backSoon, width: 25, want: "back in 1h 20m"},
		{name: "exhausted, cut short", window: backSoon, width: 10, want: "back in 1…"},
		{name: "counting seconds, and when it's back", window: backInSeconds, width: 25, want: "back in 07:42 · Mon 13:19"},
		{name: "counting seconds, without the clock", window: backInSeconds, width: 24, want: "back in 07:42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detail(tt.window, status.Heading{Projection: score.Project(tt.window, now)}, now, tt.width)
			if got.plain() != tt.want {
				t.Errorf("detail() at width %d = %q, want %q", tt.width, got.plain(), tt.want)
			}
			if got.width() > tt.width {
				t.Errorf("detail() at width %d is %d cells wide", tt.width, got.width())
			}
		})
	}
}

func TestDetailSays(t *testing.T) {
	tests := []struct {
		name    string
		window  quota.Window
		want    string
		wantInk ink
	}{
		{
			name:    "keeping pace, quietly",
			window:  quota.Window{Key: "5h", Utilization: 0.37, ResetsAt: now.Add(5 * time.Minute)},
			want:    "on pace for 38% · resets in 5m · Mon 13:17",
			wantInk: dimInk,
		},
		{
			name:    "running out, as a warning",
			window:  quota.Window{Key: "7d", Utilization: 0.64, ResetsAt: now.Add(76 * time.Hour)},
			want:    "runs out ~Wed 16:57 · resets in 3d 4h · Thu 17:12",
			wantInk: warningInk,
		},
		{
			name:    "running out in the red",
			window:  quota.Window{Key: "7d", Utilization: 0.91, ResetsAt: now.Add(24 * time.Hour)},
			want:    "runs out ~Tue 03:26 · resets in 1d · Tue 13:12",
			wantInk: errorInk,
		},
		{
			name:    "exhausted, and when it's back",
			window:  quota.Window{Key: "5h", Utilization: 1, ResetsAt: now.Add(80 * time.Minute)},
			want:    "back in 1h 20m · Mon 14:32",
			wantInk: exhaustedInk,
		},
		{
			name:    "exhausted, back within ten minutes, to the second",
			window:  quota.Window{Key: "5h", Utilization: 1, ResetsAt: now.Add(7*time.Minute + 42*time.Second)},
			want:    "back in 07:42 · Mon 13:19",
			wantInk: exhaustedInk,
		},
		{
			name:    "exhausted, back at an unknown time",
			window:  quota.Window{Key: "7d_oi", Utilization: 1.04},
			want:    "exhausted",
			wantInk: exhaustedInk,
		},
		{
			name:    "too early to project",
			window:  quota.Window{Key: "5h", Utilization: 0.01, ResetsAt: now.Add(290 * time.Minute)},
			want:    "resets in 4h 50m · Mon 18:02",
			wantInk: dimInk,
		},
		{
			name:    "a length that can't be read",
			window:  quota.Window{Key: "burst", Utilization: 0.62, ResetsAt: now.Add(42 * time.Minute)},
			want:    "resets in 42m · Mon 13:54",
			wantInk: dimInk,
		},
		{
			name:    "reset since it was read",
			window:  quota.Window{Key: "5h", Utilization: 0.8, ResetsAt: now.Add(-12 * time.Minute)},
			want:    "resets now · Mon 13:00",
			wantInk: dimInk,
		},
		{
			name:    "an unknown reset",
			window:  quota.Window{Key: "5h", Utilization: 0.23},
			want:    "reset time unknown",
			wantInk: dimInk,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detail(tt.window, status.Heading{Projection: score.Project(tt.window, now)}, now, 80)
			if got.plain() != tt.want {
				t.Errorf("detail() = %q, want %q", got.plain(), tt.want)
			}
			if lead := got[0].ink; lead != tt.wantInk {
				t.Errorf("detail() leads in %+v, want %+v", lead, tt.wantInk)
			}
		})
	}
}

func TestBackIn(t *testing.T) {
	tests := []struct {
		name  string
		until time.Duration
		want  string
	}{
		{name: "hours away", until: 80 * time.Minute, want: "1h 20m"},
		{name: "ten minutes away", until: 10 * time.Minute, want: "10m"},
		{name: "just under ten minutes away", until: 10*time.Minute - time.Nanosecond, want: "09:59"},
		{name: "minutes and seconds", until: 7*time.Minute + 42*time.Second, want: "07:42"},
		{name: "seconds, rounded down", until: 7*time.Minute + 42*time.Second + 900*time.Millisecond, want: "07:42"},
		{name: "a second", until: time.Second, want: "00:01"},
		{name: "under a second", until: 999 * time.Millisecond, want: "00:00"},
		{name: "due", until: 0, want: "now"},
		{name: "past", until: -time.Minute, want: "now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backIn(now, now.Add(tt.until)); got != tt.want {
				t.Errorf("backIn(now, now+%v) = %q, want %q", tt.until, got, tt.want)
			}
		})
	}
}
