package score_test

import (
	"math"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

const day = 24 * time.Hour

func TestAWindowAsItRunsNow(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		want   quota.Window
	}{
		{name: "as read, before its reset", window: week(0.5, 3*day), want: week(0.5, 3*day)},
		{name: "as read, reset by hand", window: restartedAt(week(0.5, 3*day), now.Add(-time.Hour)), want: restartedAt(week(0.5, 3*day), now.Add(-time.Hour))},
		{
			name:   "reset since it was read: from that reset, empty, its next a length on",
			window: refused(window("7d", 0.9, -day)),
			want:   quota.Window{Key: "7d", ResetsAt: now.Add(6 * day)},
		},
		{
			name:   "resetting now: from now, a length on",
			window: window("5h", 0.4, 0),
			want:   quota.Window{Key: "5h", ResetsAt: now.Add(5 * time.Hour)},
		},
		{
			name:   "more than a length since: as many lengths on as have passed",
			window: window("5h", 0.4, -11*time.Hour),
			want:   quota.Window{Key: "5h", ResetsAt: now.Add(4 * time.Hour)},
		},
		{
			name:   "reset by hand, then reset since: its restart left behind",
			window: restartedAt(window("7d", 0.3, -day), now.Add(-3*day)),
			want:   quota.Window{Key: "7d", ResetsAt: now.Add(6 * day)},
		},
		{name: "length unknown, its reset passed: as read", window: window("burst", 0.5, -time.Hour), want: window("burst", 0.5, -time.Hour)},
		{name: "reset unknown: as read", window: quota.Window{Key: "7d", Utilization: 0.5}, want: quota.Window{Key: "7d", Utilization: 0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.Current(tt.window, now); got != tt.want {
				t.Errorf("Current() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEvenPace(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		want   float64
		wantOK bool
	}{
		{name: "two hours into the session", window: session(0.4, 2*time.Hour), want: 0.4, wantOK: true},
		{name: "five days into the week", window: week(0.5, 5*day), want: 5.0 / 7, wantOK: true},
		{name: "reset by hand six hours ago, of the 102 it runs", window: restartedAt(week(0.04, 3*day), now.Add(-6*time.Hour)), want: 6.0 / 102, wantOK: true},
		{name: "reset since it was read: from that reset", window: window("7d", 0.9, -day), want: 1.0 / 7, wantOK: true},
		{name: "resetting now: from now", window: window("5h", 0.9, 0), want: 0, wantOK: true},
		{name: "reset unknown", window: quota.Window{Key: "5h", Utilization: 0.4}},
		{name: "length unknown", window: window("burst", 0.4, time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := score.EvenPace(tt.window, now)
			if math.Abs(got-tt.want) > 1e-12 || ok != tt.wantOK {
				t.Errorf("EvenPace() = %v, %v, want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestRoom(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		floor  float64
		want   float64
	}{
		{name: "to its limit", window: week(0.3, day), floor: 1, want: 0.7},
		{name: "to where its reserve starts", window: week(0.3, day), floor: 0.9, want: 0.6},
		{name: "past where its reserve starts: none, never less", window: week(0.95, day), floor: 0.9, want: 0},
		{name: "over its limit: none, never less", window: session(1.04, time.Hour), floor: 1, want: 0},
		{name: "refused short of its limit: none", window: refused(session(0.97, time.Hour)), floor: 1, want: 0},
		{name: "reset since it was read: all of it, from empty", window: refused(window("7d", 1, -day)), floor: 0.9, want: 0.9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := score.Room(tt.window, tt.floor, now); math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("Room() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllowanceOf(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		floor  float64
		want   quota.Allowance
		wantOK bool
	}{
		{name: "the session, its room by the hour", window: window("5h", 0.4, 3*time.Hour), floor: 1, want: quota.Allowance{Share: 0.2, Per: quota.PerHour}, wantOK: true},
		{name: "a window of a day, by the hour", window: window("1d", 0.4, 12*time.Hour), floor: 1, want: quota.Allowance{Share: 0.05, Per: quota.PerHour}, wantOK: true},
		{name: "the week, its room by the day", window: window("7d", 0.5, 2*day), floor: 1, want: quota.Allowance{Share: 0.25, Per: quota.PerDay}, wantOK: true},
		{name: "to where its reserve starts", window: window("7d", 0.5, 2*day), floor: 0.9, want: quota.Allowance{Share: 0.2, Per: quota.PerDay}, wantOK: true},
		{name: "the session, its reset within the hour: the room itself", window: window("5h", 0.66, 30*time.Minute), floor: 1, want: quota.Allowance{Share: 0.34}, wantOK: true},
		{name: "the week, its reset within the day: the room itself", window: window("7d", 0.66, 6*time.Hour), floor: 1, want: quota.Allowance{Share: 0.34}, wantOK: true},
		{name: "the week, its reset a day away: by the day", window: window("7d", 0.66, day), floor: 1, want: quota.Allowance{Share: 0.34, Per: quota.PerDay}, wantOK: true},
		{name: "reset since it was read: all its room, to its next reset a length on", window: window("7d", 0.9, -day), floor: 0.9, want: quota.Allowance{Share: 0.15, Per: quota.PerDay}, wantOK: true},
		{name: "reset by hand: to its reset", window: restartedAt(window("7d", 0.04, 4*day), now.Add(-6*time.Hour)), floor: 1, want: quota.Allowance{Share: 0.24, Per: quota.PerDay}, wantOK: true},
		{name: "at where its reserve starts: no room", window: window("7d", 0.9, 2*day), floor: 0.9},
		{name: "over its limit: no room", window: window("5h", 1.04, time.Hour), floor: 1},
		{name: "refused: no room", window: refused(window("5h", 0.5, time.Hour)), floor: 1},
		{name: "reset unknown", window: quota.Window{Key: "5h", Utilization: 0.4}, floor: 1},
		{name: "length unknown", window: window("burst", 0.4, time.Hour), floor: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := score.AllowanceOf(tt.window, tt.floor, now)
			if math.Abs(got.Share-tt.want.Share) > 1e-12 || got.Per != tt.want.Per || ok != tt.wantOK {
				t.Errorf("AllowanceOf() = %+v, %v, want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
