package status_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestText(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	doc := status.Document{
		GeneratedAt: now.UTC(),
		Source:      status.SourceProbe,
		Accounts: []status.Account{
			{
				ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(),
				Windows: []quota.Window{
					{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed},
					{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC), Status: quota.StatusAllowedWarning},
				},
				Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
			},
			{
				ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC(),
				Windows: []quota.Window{
					{Key: "5h", Label: "Session", Utilization: 1, ResetsAt: time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC), Status: quota.StatusRejected},
					{Key: "7d_oi", Label: "Fable week", Utilization: 1.04, ResetsAt: time.Date(2026, 9, 28, 13, 19, 0, 0, time.UTC)},
					{Key: "30d", Label: "30d", Utilization: 0.05},
				},
			},
			{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"},
			{ID: "spare", Label: "Spare", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
		},
	}
	want := `work · Work
  Session     23%  resets in 4h 58m · Mon 19:10
  Week        93%  resets in 5d 11h · Sun 02:10
  Fable offline: HTTP 529 · Overloaded

side · Side
  Session    100%  resets now · Mon 14:00
  Fable week 104%  resets in 7m · Mon 14:19
  30d          5%

personal · Personal
  token missing: set CLAUDE_TOKEN_PERSONAL

spare · Spare
  HTTP 401 · Invalid bearer token
`

	if got := doc.Text(now); got != want {
		t.Errorf("Text() =\n%s\nwant\n%s", got, want)
	}
}

func TestCountdown(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name  string
		until time.Duration
		want  string
	}{
		{name: "past", until: -time.Minute, want: "now"},
		{name: "due", until: 0, want: "now"},
		{name: "under a minute", until: 30 * time.Second, want: "0m"},
		{name: "minutes, rounded down", until: 7*time.Minute + 59*time.Second, want: "7m"},
		{name: "just under an hour", until: time.Hour - time.Second, want: "59m"},
		{name: "an hour", until: time.Hour, want: "1h 0m"},
		{name: "hours and minutes", until: 4*time.Hour + 57*time.Minute + 30*time.Second, want: "4h 57m"},
		{name: "just under a day", until: 24*time.Hour - time.Second, want: "23h 59m"},
		{name: "a day", until: 24 * time.Hour, want: "1d 0h"},
		{name: "days and hours, rounded down", until: 5*24*time.Hour + 12*time.Hour + 59*time.Minute, want: "5d 12h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Countdown(now, now.Add(tt.until)); got != tt.want {
				t.Errorf("Countdown(now, now+%v) = %q, want %q", tt.until, got, tt.want)
			}
		})
	}
}

func TestClock(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
		want string
	}{
		{name: "afternoon", time: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), want: "Mon 18:10"},
		{name: "just after midnight", time: time.Date(2026, 10, 4, 0, 5, 0, 0, time.UTC), want: "Sun 00:05"},
		{name: "in its own time zone", time: time.Date(2026, 9, 28, 23, 30, 0, 0, time.FixedZone("UTC-7", -7*60*60)), want: "Mon 23:30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Clock(tt.time); got != tt.want {
				t.Errorf("Clock(%v) = %q, want %q", tt.time, got, tt.want)
			}
		})
	}
}
