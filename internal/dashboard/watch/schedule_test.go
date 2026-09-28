package watch

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestNextFetch(t *testing.T) {
	tokenless := status.Account{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"}
	tests := []struct {
		name string
		doc  status.Document
		want time.Duration
	}{
		{name: "the interval", doc: calm(), want: interval},
		{name: "a minute after a reset within it", doc: document(account("work", "Work", session(0.4, 10*time.Minute), week(0.5))), want: 11 * time.Minute},
		{
			name: "a minute after the soonest of several resets",
			doc: document(
				account("work", "Work", session(0.4, 20*time.Minute), week(0.5)),
				account("side", "Side", session(0.7, 10*time.Minute), week(0.5)),
			),
			want: 11 * time.Minute,
		},
		{name: "a reset after the interval", doc: document(account("work", "Work", session(0.4, 45*time.Minute), week(0.5))), want: interval},
		{name: "a reset the grace would carry past the interval", doc: document(account("work", "Work", session(0.4, interval-time.Second), week(0.5))), want: interval},
		{name: "a reset already past", doc: document(account("work", "Work", refused(session(1, -5*time.Minute)), week(0.5))), want: interval},
		{name: "a reset past by less than the grace", doc: document(account("work", "Work", refused(session(1, -30*time.Second)), week(0.5))), want: interval},
		{name: "a reset at the moment of the read", doc: document(account("work", "Work", refused(session(1, 0)), week(0.5))), want: interval},
		{name: "a reset that isn't known", doc: document(account("work", "Work", quota.Window{Key: "5h", Label: "Session", Utilization: 0.4})), want: interval},
		{name: "an account that couldn't be read", doc: document(account("work", "Work", session(0.25, 3*time.Hour)), unreadable("side", "Side")), want: retryAfter},
		{name: "a window that couldn't be read", doc: document(partlyRead("work", "Work")), want: retryAfter},
		{name: "not for a missing token, which a retry can't mend", doc: document(account("work", "Work", session(0.25, 3*time.Hour)), tokenless), want: interval},
		{name: "a reset sooner than the retry", doc: document(account("work", "Work", session(0.4, 30*time.Second)), unreadable("side", "Side")), want: 90 * time.Second},
		{name: "the retry sooner than a reset", doc: document(account("work", "Work", session(0.4, 10*time.Minute)), unreadable("side", "Side")), want: retryAfter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := nextFetch(tt.doc, start, interval), start.Add(tt.want); !got.Equal(want) {
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
