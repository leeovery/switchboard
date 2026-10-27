package status_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

func TestCollectGivesWhenEachTokenWasTaken(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	taken := time.Date(2026, 3, 2, 9, 30, 0, 0, time.FixedZone("east", 3*60*60))
	collector := status.Collector{
		Prober: &fakeProber{},
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
		// personal has a time, but no token to have taken it of.
		Taken: tokenstest.Taken{"work": taken, "personal": taken}.Read,
		Now:   func() time.Time { return now },
	}
	accounts := []config.Account{{ID: "work"}, {ID: "personal"}, {ID: "side"}}

	doc := collector.Collect(t.Context(), accounts)
	want := map[string][2]time.Time{
		"work":     {time.Date(2026, 3, 2, 6, 30, 0, 0, time.UTC), time.Date(2027, 3, 2, 6, 30, 0, 0, time.UTC)},
		"personal": {},
		"side":     {},
	}
	for _, a := range doc.Accounts {
		got := [2]time.Time{a.TokenTaken, a.TokenGoodUntil}
		if got != want[a.ID] {
			t.Errorf("%s's token was taken at %v, good until %v; want %v", a.ID, got[0], got[1], want[a.ID])
		}
	}
}

func TestATokenIsGoodForAYearOfDaysFromWhenItWasTaken(t *testing.T) {
	tests := []struct {
		name        string
		taken, want time.Time
	}{
		{name: "in UTC", taken: time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC), want: time.Date(2027, 3, 2, 9, 30, 0, 0, time.UTC)},
		{
			name:  "in another zone, given in UTC",
			taken: time.Date(2026, 3, 2, 9, 30, 0, 0, time.FixedZone("west", -5*60*60)),
			want:  time.Date(2027, 3, 2, 14, 30, 0, 0, time.UTC),
		},
		{name: "365 days, a leap day among them", taken: time.Date(2027, 3, 2, 9, 30, 0, 0, time.UTC), want: time.Date(2028, 3, 1, 9, 30, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := status.Account{ID: "work"}.WithTokenTaken(tt.taken)
			if !a.TokenTaken.Equal(tt.taken) || a.TokenTaken.Location() != time.UTC {
				t.Errorf("TokenTaken = %v, want %v, in UTC", a.TokenTaken, tt.taken)
			}
			if a.TokenGoodUntil != tt.want {
				t.Errorf("TokenGoodUntil = %v, want %v", a.TokenGoodUntil, tt.want)
			}
		})
	}
}

func TestTokenEndsWithin(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name string
		// left is how long the token has left at now; unknown has its year's
		// end not known.
		left    time.Duration
		unknown bool
		span    time.Duration
		want    bool
	}{
		{name: "not known", unknown: true, span: status.TokenWarning},
		{name: "more than the span left", left: status.TokenWarning + time.Second, span: status.TokenWarning},
		{name: "the span left, to the instant", left: status.TokenWarning, span: status.TokenWarning},
		{name: "a moment less than the span left", left: status.TokenWarning - time.Nanosecond, span: status.TokenWarning, want: true},
		{name: "a week left, within the warning", left: 7 * 24 * time.Hour, span: status.TokenWarning, want: true},
		{name: "a week left, not within the last warning", left: 7 * 24 * time.Hour, span: status.TokenLastWarning},
		{name: "a day left, within the last warning", left: 24 * time.Hour, span: status.TokenLastWarning, want: true},
		{name: "up at now", span: status.TokenLastWarning, want: true},
		{name: "past its year", left: -24 * time.Hour, span: status.TokenLastWarning, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := status.Account{ID: "work", TokenSet: true}
			if !tt.unknown {
				a = a.WithTokenTaken(now.Add(tt.left).Add(-365 * 24 * time.Hour))
			}
			doc := status.Document{GeneratedAt: now, Source: status.SourceProbe, Accounts: []status.Account{a}}
			if got := doc.At(now, policy).Accounts[0].TokenEndsWithin(tt.span); got != tt.want {
				t.Errorf("TokenEndsWithin(%v) with %v left = %v, want %v", tt.span, tt.left, got, tt.want)
			}
		})
	}
}

func TestAMomentGivesWhenEachTokenWasTakenAsRead(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	taken := time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC)
	doc := status.Document{GeneratedAt: now, Source: status.SourceProbe, Accounts: []status.Account{
		status.Account{ID: "work", TokenSet: true}.WithTokenTaken(taken),
		{ID: "side", TokenSet: true},
	}}

	m := doc.At(now, policy)
	if work := m.Accounts[0]; work.TokenTaken != taken || work.TokenGoodUntil != taken.Add(365*24*time.Hour) {
		t.Errorf("work's token was taken at %v, good until %v; want %v, a year on", work.TokenTaken, work.TokenGoodUntil, taken)
	}
	if side := m.Accounts[1]; !side.TokenTaken.IsZero() || !side.TokenGoodUntil.IsZero() {
		t.Errorf("side's token was taken at %v, good until %v; want neither known", side.TokenTaken, side.TokenGoodUntil)
	}
}

func TestTheWarningsComeAMonthAndAWeekAhead(t *testing.T) {
	if status.TokenWarning != 30*24*time.Hour || status.TokenLastWarning != 7*24*time.Hour {
		t.Errorf("the warnings come %v and %v ahead, want 30 days and 7", status.TokenWarning, status.TokenLastWarning)
	}
}
