package watch

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestBackoff(t *testing.T) {
	tests := []struct {
		name         string
		failures     int
		failed       bool
		interval     time.Duration
		wantFailures int
		wantWait     time.Duration
	}{
		{name: "a read that didn't fail", failures: 0, failed: false, interval: 30 * time.Minute, wantFailures: 0, wantWait: 30 * time.Minute},
		{name: "a first failure", failures: 0, failed: true, interval: 30 * time.Minute, wantFailures: 1, wantWait: 2 * time.Minute},
		{name: "a second failure in a row", failures: 1, failed: true, interval: 30 * time.Minute, wantFailures: 2, wantWait: 4 * time.Minute},
		{name: "a third", failures: 2, failed: true, interval: 30 * time.Minute, wantFailures: 3, wantWait: 8 * time.Minute},
		{name: "a fourth", failures: 3, failed: true, interval: 30 * time.Minute, wantFailures: 4, wantWait: 16 * time.Minute},
		{name: "a fifth, capped at the interval", failures: 4, failed: true, interval: 30 * time.Minute, wantFailures: 5, wantWait: 30 * time.Minute},
		{name: "long past the cap", failures: 400, failed: true, interval: 30 * time.Minute, wantFailures: 401, wantWait: 30 * time.Minute},
		{name: "a fifth under a longer interval", failures: 4, failed: true, interval: time.Hour, wantFailures: 5, wantWait: 32 * time.Minute},
		{name: "a sixth, capped at the longer interval", failures: 5, failed: true, interval: time.Hour, wantFailures: 6, wantWait: time.Hour},
		{name: "a third, capped at the shortest interval", failures: 2, failed: true, interval: 5 * time.Minute, wantFailures: 3, wantWait: 5 * time.Minute},
		{name: "a read that didn't fail after failures starts again", failures: 7, failed: false, interval: 30 * time.Minute, wantFailures: 0, wantWait: 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures, wait := backoff(tt.failures, tt.failed, tt.interval)
			if failures != tt.wantFailures || wait != tt.wantWait {
				t.Errorf("backoff(%d, %v, %v) = %d, %v; want %d, %v", tt.failures, tt.failed, tt.interval, failures, wait, tt.wantFailures, tt.wantWait)
			}
		})
	}
}

func TestIncomplete(t *testing.T) {
	tokenless := status.Account{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"}
	tests := []struct {
		name string
		doc  status.Document
		want bool
	}{
		{name: "every account read", doc: calm(), want: false},
		{name: "an account that couldn't be read", doc: document(account("work", "Work", session(0.25, 3*time.Hour)), unreadable("side", "Side")), want: true},
		{name: "a window that couldn't be read", doc: document(partlyRead("work", "Work")), want: true},
		{name: "a missing token, which a retry can't mend", doc: document(account("work", "Work", session(0.25, 3*time.Hour)), tokenless), want: false},
		{name: "no accounts", doc: document(), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := incomplete(tt.doc); got != tt.want {
				t.Errorf("incomplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNextFetch(t *testing.T) {
	tests := []struct {
		name string
		doc  status.Document
		wait time.Duration
		want time.Duration
	}{
		{name: "the wait", doc: calm(), wait: interval, want: interval},
		{name: "a minute after a reset within it", doc: document(account("work", "Work", session(0.4, 10*time.Minute), week(0.5))), wait: interval, want: 11 * time.Minute},
		{
			name: "a minute after the soonest of several resets",
			doc: document(
				account("work", "Work", session(0.4, 20*time.Minute), week(0.5)),
				account("side", "Side", session(0.7, 10*time.Minute), week(0.5)),
			),
			wait: interval,
			want: 11 * time.Minute,
		},
		{name: "a reset after the wait", doc: document(account("work", "Work", session(0.4, 45*time.Minute), week(0.5))), wait: interval, want: interval},
		{name: "a reset the grace would carry past the wait", doc: document(account("work", "Work", session(0.4, interval-time.Second), week(0.5))), wait: interval, want: interval},
		{name: "a reset already past", doc: document(account("work", "Work", refused(session(1, -5*time.Minute)), week(0.5))), wait: interval, want: interval},
		{name: "a reset past by less than the grace", doc: document(account("work", "Work", refused(session(1, -30*time.Second)), week(0.5))), wait: interval, want: interval},
		{name: "a reset at the moment of the read", doc: document(account("work", "Work", refused(session(1, 0)), week(0.5))), wait: interval, want: interval},
		{name: "a reset that isn't known", doc: document(account("work", "Work", quota.Window{Key: "5h", Label: "Session", Utilization: 0.4})), wait: interval, want: interval},
		{name: "nothing read", doc: document(), wait: retryAfter, want: retryAfter},
		{name: "a retry sooner than a reset", doc: document(account("work", "Work", session(0.4, 10*time.Minute))), wait: retryAfter, want: retryAfter},
		{name: "a reset sooner than a retry", doc: document(account("work", "Work", session(0.4, 30*time.Second))), wait: retryAfter, want: 90 * time.Second},
		{name: "a reset sooner than a long retry", doc: document(account("work", "Work", session(0.4, 10*time.Minute))), wait: 16 * time.Minute, want: 11 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := nextFetch(tt.doc, start, tt.wait), start.Add(tt.want); !got.Equal(want) {
				t.Errorf("nextFetch() = %s, want %s", got.Format(time.Kitchen), want.Format(time.Kitchen))
			}
		})
	}
}

func TestTickDelay(t *testing.T) {
	backSoon := document(account("work", "Work", refused(session(1, 5*time.Minute)), week(0.5)))
	tests := []struct {
		name string
		doc  status.Document
		now  time.Time
		want time.Duration
	}{
		{name: "on the minute", doc: calm(), now: at(13, 12, 0), want: time.Minute + tickSlack},
		{name: "part way through a minute", doc: calm(), now: at(13, 12, 30).Add(500 * time.Millisecond), want: 29*time.Second + 500*time.Millisecond + tickSlack},
		{name: "just past the minute", doc: calm(), now: at(13, 12, 0).Add(tickSlack), want: time.Minute},
		{name: "counting seconds, on the second", doc: backSoon, now: at(13, 12, 0), want: time.Second + tickSlack},
		{name: "counting seconds, part way through one", doc: backSoon, now: at(13, 12, 0).Add(300 * time.Millisecond), want: 700*time.Millisecond + tickSlack},
		{name: "before anything is read", doc: status.Document{}, now: at(13, 12, 0), want: time.Minute + tickSlack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tickDelay(tt.doc, tt.now); got != tt.want {
				t.Errorf("tickDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}
