package status_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
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

func TestCollectGivesNoDateItCantGoBy(t *testing.T) {
	tests := []struct {
		name  string
		taken func(id string, token tokens.Token) (time.Time, error)
	}{
		{name: "with no record", taken: tokenstest.Taken{}.Read},
		{name: "with a record it can't go by", taken: func(string, tokens.Token) (time.Time, error) {
			return time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC), errors.New("the record tokens/work.taken is of other tokens")
		}},
		{name: "knowing of no record, as a collector without Taken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := status.Collector{
				Prober: &fakeProber{},
				Policy: policy,
				Token:  tokenstest.Files{"work": "test-token-work"}.Read,
				Taken:  tt.taken,
				Now:    func() time.Time { return time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC) },
			}

			work := collector.Collect(t.Context(), []config.Account{{ID: "work"}}).Accounts[0]
			if !work.TokenSet || !work.TokenTaken.IsZero() || !work.TokenGoodUntil.IsZero() {
				t.Errorf("work reads %+v, want its token usable, with no date", work)
			}
		})
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

// A moment judges each account's token's year at its own now, never when
// its document was made or its account read, each well before it.
func TestAMomentJudgesEachTokensYearAtItsNow(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	generated := now.AddDate(0, 0, -40)
	tests := []struct {
		name string
		// left is how many days the token has left at now, to the instant;
		// unknown has its year's end not known.
		left               int
		unknown            bool
		warned, lastWarned bool
		days               int
		counted            bool
		gone               float64
	}{
		{name: "not known", unknown: true},
		{name: "31 days left", left: 31, days: 31, counted: true, gone: 334.0 / 365},
		{name: "30 days left", left: 30, warned: true, days: 30, counted: true, gone: 335.0 / 365},
		{name: "8 days left", left: 8, warned: true, days: 8, counted: true, gone: 357.0 / 365},
		{name: "7 days left", left: 7, warned: true, lastWarned: true, days: 7, counted: true, gone: 358.0 / 365},
		{name: "up at now", warned: true, lastWarned: true, gone: 1},
		{name: "past its year", left: -1, warned: true, lastWarned: true, gone: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := status.Account{ID: "work", TokenSet: true, FetchedAt: generated.Add(-time.Hour)}
			if !tt.unknown {
				a = a.WithTokenTaken(now.AddDate(0, 0, tt.left).Add(-claude.SetupTokenLifetime))
			}
			doc := status.Document{GeneratedAt: generated, Source: status.SourceProbe, Accounts: []status.Account{a}}
			work := doc.At(now, policy).Accounts[0]
			if got := work.TokenEndsWithin(status.TokenWarning); got != tt.warned {
				t.Errorf("TokenEndsWithin(TokenWarning) = %v, want %v", got, tt.warned)
			}
			if got := work.TokenEndsWithin(status.TokenLastWarning); got != tt.lastWarned {
				t.Errorf("TokenEndsWithin(TokenLastWarning) = %v, want %v", got, tt.lastWarned)
			}
			if days, counted := work.TokenDaysLeft(); days != tt.days || counted != tt.counted {
				t.Errorf("TokenDaysLeft() = %d, %v; want %d, %v", days, counted, tt.days, tt.counted)
			}
			if got := work.TokenGone(); math.Abs(got-tt.gone) > 1e-9 {
				t.Errorf("TokenGone() = %v, want %v", got, tt.gone)
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
	if work := m.Accounts[0].TokenYear; work.Taken != taken || work.GoodUntil != taken.Add(365*24*time.Hour) {
		t.Errorf("work's token's year is %+v; want from %v, to a year on", work, taken)
	}
	if side := m.Accounts[1].TokenYear; side != (status.TokenYear{}) {
		t.Errorf("side's token's year is %+v; want neither end known", side)
	}
}

func TestATokensYearIsJudgedAtNow(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("loading Europe/London: %v", err)
	}
	noon := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	frame := time.Date(2026, 10, 27, 17, 42, 0, 0, london)
	// Before the clocks go back on 25 Oct, a month ahead and a week.
	monthBefore, weekBefore := time.Date(2026, 10, 1, 0, 30, 0, 0, london), time.Date(2026, 10, 20, 0, 30, 0, 0, london)
	// endingAt is the year of a token whose year is up at until, as it's
	// read, in UTC.
	endingAt := func(until time.Time) status.TokenYear {
		return status.TokenYearFrom(until.Add(-claude.SetupTokenLifetime))
	}
	// daysOn is the time days after noon, to the second.
	daysOn := func(days int, seconds time.Duration) time.Time {
		return noon.AddDate(0, 0, days).Add(seconds * time.Second)
	}
	// goneWith is the share of a year gone with hours of it left.
	goneWith := func(hours float64) float64 { return 1 - hours/(365*24) }
	tests := []struct {
		name string
		year status.TokenYear
		now  time.Time
		// warned and lastWarned are whether it ends within TokenWarning and
		// TokenLastWarning, and days and counted what DaysLeft gives.
		known, warned, lastWarned, up bool
		days                          int
		counted                       bool
		gone                          float64
		// said is how prose.Expires words its end, told whether it's up as
		// Up judges it, where it's given.
		said string
	}{
		{name: "just taken", year: status.TokenYearFrom(frame), now: frame, known: true, days: 365, counted: true, said: "expires 27 Oct 2027"},
		{name: "a second before it's taken, a year of days left", year: status.TokenYearFrom(noon), now: noon.Add(-time.Second), known: true, days: 365, counted: true},
		{name: "a day before it's taken", year: status.TokenYearFrom(noon), now: noon.AddDate(0, 0, -1), known: true, days: 366, counted: true},
		{name: "31 days left", year: endingAt(daysOn(31, 0)), now: noon, known: true, days: 31, counted: true, gone: goneWith(31 * 24)},
		{name: "30 days left and a second", year: endingAt(daysOn(30, 1)), now: noon, known: true, warned: true, days: 30, counted: true, gone: goneWith(30 * 24)},
		{name: "30 days left less a second", year: endingAt(daysOn(30, -1)), now: noon, known: true, warned: true, days: 30, counted: true, gone: goneWith(30 * 24)},
		{name: "8 days left", year: endingAt(daysOn(8, 0)), now: noon, known: true, warned: true, days: 8, counted: true, gone: goneWith(8 * 24)},
		{name: "7 days left and a second", year: endingAt(daysOn(7, 1)), now: noon, known: true, warned: true, lastWarned: true, days: 7, counted: true, gone: goneWith(7 * 24)},
		{name: "7 days left less a second", year: endingAt(daysOn(7, -1)), now: noon, known: true, warned: true, lastWarned: true, days: 7, counted: true, gone: goneWith(7 * 24)},
		{name: "the year's last day", year: endingAt(noon.Add(time.Hour)), now: noon, known: true, warned: true, lastWarned: true, counted: true, gone: goneWith(1), said: "expires 28 Sep 2026"},
		{name: "up at now, to the instant", year: endingAt(noon), now: noon, known: true, warned: true, lastWarned: true, up: true, gone: 1, said: "expired 28 Sep 2026"},
		{name: "past its year", year: endingAt(noon.Add(-24 * time.Hour)), now: noon, known: true, warned: true, lastWarned: true, up: true, gone: 1, said: "expired 27 Sep 2026"},
		{name: "not known", year: status.TokenYear{}, now: noon},
		{name: "taken known, its end not", year: status.TokenYear{Taken: noon}, now: noon},
		{name: "its end known, when it was taken not", year: status.TokenYear{GoodUntil: daysOn(10, 0)}, now: noon, known: true, warned: true, days: 10, counted: true},
		{name: "of no length, before its end", year: status.TokenYear{Taken: noon, GoodUntil: noon}, now: noon.Add(-time.Hour), known: true, warned: true, lastWarned: true, counted: true},
		{name: "of no length, at its end", year: status.TokenYear{Taken: noon, GoodUntil: noon}, now: noon, known: true, warned: true, lastWarned: true, up: true, gone: 1},
		{name: "taken after its end, as of a record gone wrong", year: status.TokenYear{Taken: daysOn(400, 0), GoodUntil: daysOn(10, 0)}, now: noon, known: true, warned: true, days: 10, counted: true},
		{name: "27 Oct to 17 Nov, frame 3's", year: endingAt(time.Date(2026, 11, 17, 9, 0, 0, 0, london)), now: frame, known: true, warned: true, days: 21, counted: true, gone: goneWith(20*24 + 15.3), said: "expires 17 Nov 2026"},
		{
			// Counted in UTC, now is a day earlier, and in hours the span is
			// 3 days and a quarter, an hour longer for the clocks going back
			// on 25 Oct.
			name: "the clocks going back between", year: endingAt(time.Date(2026, 10, 26, 23, 45, 0, 0, london)), now: time.Date(2026, 10, 24, 0, 30, 0, 0, london),
			known: true, warned: true, lastWarned: true, days: 2, counted: true, gone: goneWith(3*24 + 0.25),
		},
		{
			// In hours the span is under a day, an hour shorter for the
			// clocks going forward on 29 Mar.
			name: "the clocks going forward between", year: endingAt(time.Date(2026, 3, 30, 0, 15, 0, 0, london)), now: time.Date(2026, 3, 28, 23, 30, 0, 0, london),
			known: true, warned: true, lastWarned: true, days: 2, counted: true, gone: goneWith(23.75),
		},
		// Each a day longer in hours than in dates, for the clocks going
		// back between: the warnings go by the dates.
		{name: "31 days left, the clocks going back", year: endingAt(time.Date(2026, 11, 1, 23, 30, 0, 0, london)), now: monthBefore, known: true, days: 31, counted: true, gone: goneWith(32 * 24)},
		{name: "30 days left, the clocks going back", year: endingAt(time.Date(2026, 10, 31, 23, 30, 0, 0, london)), now: monthBefore, known: true, warned: true, days: 30, counted: true, gone: goneWith(31 * 24)},
		{name: "8 days left, the clocks going back", year: endingAt(time.Date(2026, 10, 28, 23, 30, 0, 0, london)), now: weekBefore, known: true, warned: true, days: 8, counted: true, gone: goneWith(9 * 24)},
		{name: "7 days left, the clocks going back", year: endingAt(time.Date(2026, 10, 27, 23, 30, 0, 0, london)), now: weekBefore, known: true, warned: true, lastWarned: true, days: 7, counted: true, gone: goneWith(8 * 24)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y := tt.year
			if got := y.Known(); got != tt.known {
				t.Errorf("Known() = %v, want %v", got, tt.known)
			}
			if got := y.EndsWithin(tt.now, status.TokenWarning); got != tt.warned {
				t.Errorf("EndsWithin(TokenWarning) = %v, want %v", got, tt.warned)
			}
			if got := y.EndsWithin(tt.now, status.TokenLastWarning); got != tt.lastWarned {
				t.Errorf("EndsWithin(TokenLastWarning) = %v, want %v", got, tt.lastWarned)
			}
			if got := y.Up(tt.now); got != tt.up {
				t.Errorf("Up() = %v, want %v", got, tt.up)
			}
			if days, counted := y.DaysLeft(tt.now); days != tt.days || counted != tt.counted {
				t.Errorf("DaysLeft() = %d, %v; want %d, %v", days, counted, tt.days, tt.counted)
			}
			if got := y.Gone(tt.now); math.Abs(got-tt.gone) > 1e-6 {
				t.Errorf("Gone() = %v, want %v", got, tt.gone)
			}
			if got := prose.Expires(tt.now, y.GoodUntil, y.Up(tt.now)); tt.said != "" && got != tt.said {
				t.Errorf("Expires() = %q, want %q", got, tt.said)
			}
		})
	}
}

func TestAnAccountsTokenYearIsAsItsTokenWasTaken(t *testing.T) {
	taken := time.Date(2026, 3, 2, 9, 30, 0, 0, time.FixedZone("west", -5*60*60))
	if got, want := (status.Account{ID: "work"}).WithTokenTaken(taken).TokenYear(), status.TokenYearFrom(taken); got != want {
		t.Errorf("TokenYear() = %+v, want %+v", got, want)
	}
	if got := (status.Account{ID: "side"}).TokenYear(); got != (status.TokenYear{}) {
		t.Errorf("an account with no date has a token year of %+v, want none", got)
	}
}

func TestTheWarningsComeAMonthAndAWeekAhead(t *testing.T) {
	if status.TokenWarning != 30 || status.TokenLastWarning != 7 {
		t.Errorf("the warnings come %d and %d days ahead, want 30 and 7", status.TokenWarning, status.TokenLastWarning)
	}
}
