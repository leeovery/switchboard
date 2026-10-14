package status_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// readEvery is how often the tests' read loop reads when nothing brings a
// read on.
const readEvery = 30 * time.Minute

// probedOf is the document probing built of the accounts given.
func probedOf(accounts ...status.Account) status.Document {
	return status.Document{GeneratedAt: built, Source: status.SourceProbe, Accounts: accounts}
}

func TestIncompleteAsRead(t *testing.T) {
	calm := readAt("work", windowIn("5h", 0.25, 3*time.Hour), windowIn("7d", 0.5, 3*day))
	tests := []struct {
		name string
		doc  status.Document
		want bool
	}{
		{name: "every account read", doc: probedOf(calm)},
		{name: "an account that couldn't be read", doc: probedOf(calm, status.Account{ID: "side", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"}), want: true},
		{
			name: "a window that couldn't be read",
			doc: probedOf(with(calm, func(a *status.Account) {
				a.Failures = []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}}
			})),
			want: true,
		},
		{name: "a missing token, which a retry can't mend", doc: probedOf(calm, status.Account{ID: "personal", Error: "token missing"})},
		{name: "no accounts", doc: probedOf()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.doc.IncompleteAsRead(); got != tt.want {
				t.Errorf("IncompleteAsRead() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNextFetchAsRead(t *testing.T) {
	week := windowIn("7d", 0.5, 3*day)
	sessionIn := func(resetsIn time.Duration) status.Account {
		return readAt("work", windowIn("5h", 0.4, resetsIn), week)
	}
	tests := []struct {
		name string
		doc  status.Document
		wait time.Duration
		want time.Duration
	}{
		{name: "the wait", doc: probedOf(sessionIn(3 * time.Hour)), wait: readEvery, want: readEvery},
		{name: "a minute after a reset within it", doc: probedOf(sessionIn(10 * time.Minute)), wait: readEvery, want: 11 * time.Minute},
		{
			name: "a minute after the soonest of several resets",
			doc:  probedOf(sessionIn(20*time.Minute), readAt("side", windowIn("5h", 0.7, 10*time.Minute), week)),
			wait: readEvery,
			want: 11 * time.Minute,
		},
		{name: "a reset after the wait", doc: probedOf(sessionIn(45 * time.Minute)), wait: readEvery, want: readEvery},
		{name: "a reset the grace would carry past the wait", doc: probedOf(sessionIn(readEvery - time.Second)), wait: readEvery, want: readEvery},
		{name: "a reset already past", doc: probedOf(readAt("work", spentIn("5h", 1, -5*time.Minute), week)), wait: readEvery, want: readEvery},
		{name: "a reset past by less than the grace", doc: probedOf(readAt("work", spentIn("5h", 1, -30*time.Second), week)), wait: readEvery, want: readEvery},
		{name: "a reset at the moment of the read", doc: probedOf(readAt("work", spentIn("5h", 1, 0), week)), wait: readEvery, want: readEvery},
		{name: "a reset that isn't known", doc: probedOf(readAt("work", quota.Window{Key: "5h", Label: "Session", Utilization: 0.4})), wait: readEvery, want: readEvery},
		{name: "nothing read", doc: probedOf(), wait: 2 * time.Minute, want: 2 * time.Minute},
		{name: "a retry sooner than a reset", doc: probedOf(sessionIn(10 * time.Minute)), wait: 2 * time.Minute, want: 2 * time.Minute},
		{name: "a reset sooner than a retry", doc: probedOf(sessionIn(30 * time.Second)), wait: 2 * time.Minute, want: 90 * time.Second},
		{name: "a reset sooner than a long retry", doc: probedOf(sessionIn(10 * time.Minute)), wait: 16 * time.Minute, want: 11 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := tt.doc.NextFetchAsRead(built, tt.wait), built.Add(tt.want); !got.Equal(want) {
				t.Errorf("NextFetchAsRead() = %v, want %v", got.Sub(built), tt.want)
			}
		})
	}
}

func TestRereadAfterResetAsRead(t *testing.T) {
	week := windowIn("7d", 0.5, 3*day)
	tests := []struct {
		name  string
		doc   status.Document
		since time.Time
		// want is how long after the document was built the reread is due:
		// none where it's zero.
		want time.Duration
	}{
		{name: "a grace after the soonest reset", doc: probedOf(readAt("work", windowIn("5h", 0.4, 20*time.Minute), week), readAt("side", windowIn("5h", 0.4, 10*time.Minute), week)), since: built, want: 11 * time.Minute},
		{name: "a reset as read, whose grace runs past since, though it has passed", doc: probedOf(readAt("work", windowIn("5h", 0.4, -30*time.Second), week)), since: built, want: 30 * time.Second},
		{name: "past one whose grace ran out by since, the next", doc: probedOf(readAt("work", windowIn("5h", 0.4, 10*time.Minute), week)), since: built.Add(11 * time.Minute), want: 3*day + time.Minute},
		{
			name:  "none of an account with a window lapsed",
			doc:   probedOf(with(readAt("work", quota.Window{Key: "5h", Label: "Session"}, week), func(a *status.Account) { a.Lapsed = []string{"5h"} })),
			since: built,
		},
		{name: "none where no reset is known", doc: probedOf(readAt("work", quota.Window{Key: "5h", Label: "Session", Utilization: 0.4})), since: built},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.doc.RereadAfterResetAsRead(tt.since)
			if want := built.Add(tt.want); (tt.want == 0) != got.IsZero() || (tt.want != 0 && !got.Equal(want)) {
				t.Errorf("RereadAfterResetAsRead() = %v, want %v after it was built", got, tt.want)
			}
		})
	}
}
