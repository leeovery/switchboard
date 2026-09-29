package status_test

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestText(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	personal := status.Account{ID: "personal", Label: "Personal", Error: "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"}
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
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

spare · Spare
  HTTP 401 · Invalid bearer token

best next: work · Work
probed directly
`,
		},
		{
			name: "no account to use next",
			doc:  status.Document{GeneratedAt: now.UTC(), Source: status.SourceProbe, Accounts: []status.Account{personal}},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

probed directly
`,
		},
		{
			name: "the router's, pinned, with each account's sessions and a limit",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Best:        "work",
				Pin:         status.Pin{Account: "side", Since: now.UTC().Add(-time.Hour)},
				Router:      status.Health{Healthy: true, Requests: 12},
				Sessions:    3,
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(), Sessions: 2,
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)},
							{Key: "7d", Label: "Week", Utilization: 0.4, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC)},
						},
					},
					{
						ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC(), Sessions: 1,
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)},
						},
						Limit: status.Limit{Until: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)},
					},
					personal,
				},
			},
			want: `work · Work
  Session  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  Week     40%  resets in 5d 11h · Sun 02:10 · runs out ~Wed 20:15
  2 sessions

side · Side
  Session  60%  resets in 2h 58m · Mon 17:10 · runs out ~Mon 15:33
  limit until Mon 21:00
  1 session

personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

best next: work · Work
from the router: healthy  ·  3 sessions  ·  pinned to side · Side
`,
		},
		{
			name: "the router's, with refusals",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true, Requests: 12},
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(),
						Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)}},
						Refused: status.Refusal{Until: time.Date(2026, 9, 28, 13, 20, 0, 0, time.UTC), Status: 401},
					},
					{
						ID: "side", Label: "Side", TokenSet: true, FetchedAt: now.UTC(), Sessions: 1,
						Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 1, ResetsAt: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)}},
						Limit:   status.Limit{Windows: []string{"5h"}, Until: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)},
						Refused: status.Refusal{Until: time.Date(2026, 9, 28, 13, 21, 0, 0, time.UTC), Status: 403, Family: "opus"},
					},
					{
						ID: "spare", Label: "Spare", TokenSet: true, FetchedAt: now.UTC(),
						Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.5, ResetsAt: time.Date(2026, 9, 28, 16, 10, 0, 0, time.UTC)}},
						Refused: status.Refusal{Until: now.Add(-time.Minute), Status: 403, Family: "fable"},
					},
				},
			},
			want: `work · Work
  Session  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  refused (401) until 14:20

side · Side
  Session 100%  resets in 6h 48m · Mon 21:00 · exhausted
  limit until Mon 21:00
  refused (403, opus) until 14:21
  1 session

spare · Spare
  Session  50%  resets in 2h 58m · Mon 17:10 · runs out ~Mon 16:14

from the router: healthy  ·  no sessions  ·  routing automatically
`,
		},
		{
			name: "the router's, unhealthy, routing automatically, and a limit that has lifted",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceRouter,
				Router:      status.Health{Requests: 8, Failures: 6, Reason: "6 of the 8 requests in the last 5 minutes failed"},
				Accounts: []status.Account{
					{ID: "side", Label: "Side", TokenSet: true, Error: "HTTP 401 · Invalid bearer token", Limit: status.Limit{Until: now.Add(-time.Minute)}},
				},
			},
			want: `side · Side
  HTTP 401 · Invalid bearer token

from the router: unhealthy, 6 of the 8 requests in the last 5 minutes failed  ·  no sessions  ·  routing automatically
`,
		},
		{
			name: "probed, as the router isn't running",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterNotRunning},
				Accounts:    []status.Account{personal},
			},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

probed directly: the router isn't running
`,
		},
		{
			name: "probed, as the router didn't answer as it should",
			doc: status.Document{
				GeneratedAt: now.UTC(),
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"},
				Accounts:    []status.Account{personal},
			},
			want: `personal · Personal
  token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal

probed directly: the router is unhealthy, no answer within 500ms
`,
		},
		{
			name: "no accounts",
			doc:  status.Document{GeneratedAt: now.UTC(), Source: status.SourceProbe},
			want: "probed directly\n",
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
		{
			name: "without what the router notes of it, as a statusline shows it",
			account: status.Account{
				ID: "spare", Label: "Spare", TokenSet: true, Error: "HTTP 401 · Invalid bearer token", Sessions: 1,
				Limit: status.Limit{Until: now.Add(time.Hour)},
			},
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

func TestTextShowsWhatCameFromElsewhereCleaned(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.UTC)
	// clear clears the screen, as a terminal shown it takes it.
	const clear = "\x1b[2J"
	work := status.Account{
		ID: "work", Label: "Work" + clear, TokenSet: true,
		Windows:  []quota.Window{{Key: "5h", Label: "Session" + clear, Utilization: 0.23}},
		Failures: []quota.Failure{{Label: "Fable" + clear, Window: "7d_oi", Error: "HTTP 529 · " + clear + "Overloaded"}},
		Error:    "HTTP 401 · " + clear + "Invalid bearer token",
	}
	shown := map[string]string{
		"an account, as status --session shows it": work.Text(now),
		"the router's document": status.Document{
			Source:   status.SourceRouter,
			Best:     "work",
			Pin:      status.Pin{Account: "gone" + clear},
			Router:   status.Health{Reason: clear + "6 of the 8 requests in the last 5 minutes failed"},
			Accounts: []status.Account{work},
		}.Text(now),
		"a document probed, as something else answered for the router": status.Document{
			Source:   status.SourceProbe,
			Fallback: status.Fallback{Router: status.RouterUnhealthy, Reason: "answered " + clear},
			Accounts: []status.Account{work},
		}.Text(now),
	}
	for name, text := range shown {
		if strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) {
			t.Errorf("%s reads %q, want no control character but its line ends", name, text)
		}
	}
	want := "work · Work [2J\n" +
		"  Session [2J  23%\n" +
		"  Fable [2J offline: HTTP 529 · [2JOverloaded\n" +
		"  HTTP 401 · [2JInvalid bearer token\n"
	if got := work.Text(now); got != want {
		t.Errorf("Text() =\n%s\nwant\n%s", got, want)
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "Work", want: "Work"},
		{text: "  Work \t team\n", want: "Work team"},
		{text: "Work\x1b[31m team\a", want: "Work [31m team"},
		{text: "\x1b]0;a title\a", want: "]0;a title"},
	}
	for _, tt := range tests {
		if got := status.Clean(tt.text); got != tt.want {
			t.Errorf("Clean(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestSessionCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "no sessions"},
		{n: 1, want: "1 session"},
		{n: 2, want: "2 sessions"},
		{n: 12, want: "12 sessions"},
	}
	for _, tt := range tests {
		if got := status.SessionCount(tt.n); got != tt.want {
			t.Errorf("SessionCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestRouting(t *testing.T) {
	accounts := []status.Account{{ID: "work", Label: "Work"}, {ID: "side", Label: "Side"}}
	tests := []struct {
		name string
		pin  status.Pin
		want string
	}{
		{name: "unpinned", want: "routing automatically"},
		{name: "pinned", pin: status.Pin{Account: "side"}, want: "pinned to side · Side"},
		{name: "pinned, moving running sessions", pin: status.Pin{Account: "work", Move: true}, want: "pinned to work · Work"},
		{name: "pinned to an account the document lacks", pin: status.Pin{Account: "gone"}, want: "pinned to gone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{Source: status.SourceRouter, Pin: tt.pin, Accounts: accounts}
			if got := doc.Routing(); got != tt.want {
				t.Errorf("Routing() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRefusal(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	until := time.Date(2026, 9, 28, 13, 20, 0, 0, time.UTC)
	tests := []struct {
		name    string
		refusal status.Refusal
		want    string
	}{
		{name: "its token", refusal: status.Refusal{Until: until, Status: 401}, want: "refused (401) until 14:20"},
		{name: "a family's requests", refusal: status.Refusal{Until: until, Status: 403, Family: "opus"}, want: "refused (403, opus) until 14:20"},
		{name: "a family named from elsewhere, cleaned", refusal: status.Refusal{Until: until, Status: 403, Family: "claude-x\x1b[2J"}, want: "refused (403, claude-x [2J) until 14:20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.refusal.Text(now); got != tt.want {
				t.Errorf("Text() = %q, want %q, in now's time zone", got, tt.want)
			}
		})
	}
	refusal := status.Refusal{Until: until, Status: 401}
	for _, tt := range []struct {
		at   time.Time
		want bool
	}{
		{at: now, want: true},
		{at: until.Add(-time.Second), want: true},
		{at: until, want: false},
	} {
		if got := refusal.Holds(tt.at); got != tt.want {
			t.Errorf("Holds(%s) = %v, want %v", tt.at.Format(time.Kitchen), got, tt.want)
		}
	}
	if (status.Refusal{}).Holds(now) {
		t.Error("no refusal holds")
	}
}

func TestLimit(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	limit := status.Limit{Windows: []string{"5h"}, Until: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)}
	if got, want := limit.Text(now), "limit until Mon 21:00"; got != want {
		t.Errorf("Text() = %q, want %q, in now's time zone", got, want)
	}
	for _, tt := range []struct {
		at   time.Time
		want bool
	}{
		{at: now, want: true},
		{at: limit.Until.Add(-time.Second), want: true},
		{at: limit.Until, want: false},
		{at: limit.Until.Add(time.Hour), want: false},
	} {
		if got := limit.Holds(tt.at); got != tt.want {
			t.Errorf("Holds(%s) = %v, want %v", tt.at.Format(time.Kitchen), got, tt.want)
		}
	}
	if (status.Limit{}).Holds(now) {
		t.Error("no limit holds")
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
	utc := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name          string
		now, time     time.Time
		want, wantDay string
	}{
		{name: "afternoon", now: utc, time: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), want: "Mon 18:10", wantDay: "18:10"},
		{name: "just after midnight", now: utc, time: time.Date(2026, 10, 4, 0, 5, 0, 0, time.UTC), want: "Sun 00:05", wantDay: "00:05"},
		{
			name:    "in now's time zone",
			now:     utc.In(time.FixedZone("UTC-7", -7*60*60)),
			time:    time.Date(2026, 9, 29, 6, 30, 0, 0, time.UTC),
			want:    "Mon 23:30",
			wantDay: "23:30",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Clock(tt.now, tt.time); got != tt.want {
				t.Errorf("Clock(%v, %v) = %q, want %q", tt.now, tt.time, got, tt.want)
			}
			if got := status.TimeOfDay(tt.now, tt.time); got != tt.wantDay {
				t.Errorf("TimeOfDay(%v, %v) = %q, want %q", tt.now, tt.time, got, tt.wantDay)
			}
		})
	}
}
