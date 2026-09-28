package status_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestText(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	personal := status.Account{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"}
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{
			name: "windows heading every way, and the account to use next",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceProbe,
				Best:        "work",
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(),
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC), Status: quota.StatusAllowed},
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
							{Key: "burst", Label: "burst", Utilization: 1},
						},
					},
					personal,
					{ID: "spare", Label: "Spare", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
				},
			},
			want: `work · Work
  Session     23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  Week        93%  resets in 5d 11h · Sun 02:10 · runs out ~Mon 16:54
  Fable offline: HTTP 529 · Overloaded

side · Side
  Session    100%  resets now · Mon 14:00
  Fable week 104%  resets in 7m · Mon 14:19 · exhausted
  30d          5%
  burst      100%  exhausted

personal · Personal
  token missing: set CLAUDE_TOKEN_PERSONAL

spare · Spare
  HTTP 401 · Invalid bearer token

best next: work · Work
`,
		},
		{
			name: "no account to use next",
			doc:  status.Document{GeneratedAt: now.UTC(), Source: status.SourceProbe, Accounts: []status.Account{personal}},
			want: `personal · Personal
  token missing: set CLAUDE_TOKEN_PERSONAL
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.doc.Text(now); got != tt.want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestAccountText(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name    string
		account status.Account
		want    string
	}{
		{
			name: "its windows, lined up by their own labels",
			account: status.Account{
				ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(),
				Windows: []quota.Window{
					{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC), Status: quota.StatusAllowed},
					{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC), Status: quota.StatusAllowedWarning},
				},
				Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
			},
			want: `work · Work
  Session  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  Week     93%  resets in 5d 11h · Sun 02:10 · runs out ~Mon 16:54
  Fable offline: HTTP 529 · Overloaded
`,
		},
		{
			name:    "why it couldn't be read",
			account: status.Account{ID: "spare", Label: "Spare", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
			want: `spare · Spare
  HTTP 401 · Invalid bearer token
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.Text(now); got != tt.want {
				t.Errorf("Text() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestProjection(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	tests := []struct {
		name       string
		projection score.Projection
		want       string
	}{
		{name: "on pace", projection: score.Projection{Kind: score.OnPace, AtReset: 0.92}, want: "on pace for 92%"},
		{name: "on pace, having used nothing", projection: score.Projection{Kind: score.OnPace}, want: "on pace for 0%"},
		{name: "runs out, in now's time zone", projection: score.Projection{Kind: score.RunsOut, At: time.Date(2026, 10, 2, 18, 40, 0, 0, time.UTC)}, want: "runs out ~Fri 19:40"},
		{name: "exhausted", projection: score.Projection{Kind: score.Exhausted, At: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)}, want: "exhausted"},
		{name: "exhausted, back at an unknown time", projection: score.Projection{Kind: score.Exhausted}, want: "exhausted"},
		{name: "nothing to say", projection: score.Projection{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Projection(now, tt.projection); got != tt.want {
				t.Errorf("Projection(%+v) = %q, want %q", tt.projection, got, tt.want)
			}
		})
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
		{name: "an hour", until: time.Hour, want: "1h"},
		{name: "an hour, and seconds too few for a minute", until: time.Hour + 59*time.Second, want: "1h"},
		{name: "just past an hour", until: time.Hour + time.Minute, want: "1h 1m"},
		{name: "whole hours", until: 4 * time.Hour, want: "4h"},
		{name: "hours and minutes", until: 4*time.Hour + 57*time.Minute + 30*time.Second, want: "4h 57m"},
		{name: "just under a day", until: 24*time.Hour - time.Second, want: "23h 59m"},
		{name: "a day", until: 24 * time.Hour, want: "1d"},
		{name: "a day, and minutes too few for an hour", until: 24*time.Hour + 59*time.Minute, want: "1d"},
		{name: "just past a day", until: 25 * time.Hour, want: "1d 1h"},
		{name: "whole days", until: 6 * 24 * time.Hour, want: "6d"},
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

func TestResets(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name  string
		until time.Duration
		want  string
	}{
		{name: "past", until: -time.Minute, want: "resets now"},
		{name: "due", until: 0, want: "resets now"},
		{name: "minutes", until: 7 * time.Minute, want: "resets in 7m"},
		{name: "days and hours", until: 5*24*time.Hour + 12*time.Hour, want: "resets in 5d 12h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Resets(now, now.Add(tt.until)); got != tt.want {
				t.Errorf("Resets(now, now+%v) = %q, want %q", tt.until, got, tt.want)
			}
		})
	}
}

func TestPercent(t *testing.T) {
	tests := []struct {
		utilization float64
		want        string
	}{
		{utilization: 0, want: "0%"},
		{utilization: 0.004, want: "0%"},
		{utilization: 0.23, want: "23%"},
		{utilization: 0.996, want: "100%"},
		{utilization: 1.04, want: "104%"},
	}
	for _, tt := range tests {
		if got := status.Percent(tt.utilization); got != tt.want {
			t.Errorf("Percent(%v) = %q, want %q", tt.utilization, got, tt.want)
		}
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
