package prose_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

func TestANumberIsAWordToTenThenFigures(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "none"}, {n: 1, want: "one"}, {n: 3, want: "three"}, {n: 10, want: "ten"}, {n: 11, want: "11"}, {n: 120, want: "120"},
	}
	for _, tt := range tests {
		if got := prose.Number(tt.n); got != tt.want {
			t.Errorf("Number(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestACountedNounIsOneOrMany(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0 weeks"}, {n: 1, want: "1 week"}, {n: 2, want: "2 weeks"}, {n: 11, want: "11 weeks"}, {n: 1240, want: "1,240 weeks"},
	}
	for _, tt := range tests {
		if got := prose.Counted(tt.n, "week"); got != tt.want {
			t.Errorf("Counted(%d, week) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestANumberGroupedSetsItsThousandsApart(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0"}, {n: 999, want: "999"}, {n: 1000, want: "1,000"}, {n: 1240, want: "1,240"},
		{n: 1234567, want: "1,234,567"}, {n: -1240, want: "-1,240"},
	}
	for _, tt := range tests {
		if got := prose.Grouped(tt.n); got != tt.want {
			t.Errorf("Grouped(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

// No count of tokens is said wider than TokensWidest, at each form's widest
// or past it.
func TestTokensWidestIsAsWideAsTokensEverSays(t *testing.T) {
	widest := len(prose.TokensWidest())
	if prose.TokensWidest() != "999.9M" {
		t.Errorf("TokensWidest = %q, want %q", prose.TokensWidest(), "999.9M")
	}
	for _, n := range []int{999, 9_949, 999_499, 999_949_999, 9_949_999_999, 999_499_999_999} {
		if got := prose.Tokens(n); len(got) > widest {
			t.Errorf("Tokens(%d) = %q, wider than TokensWidest's %d", n, got, widest)
		}
	}
}

func TestTokensAreCountedAsTheLogCountsThem(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0"}, {n: 845, want: "845"}, {n: 999, want: "999"},
		{n: 1000, want: "1.0k"}, {n: 5400, want: "5.4k"}, {n: 9949, want: "9.9k"},
		{n: 9950, want: "10k"}, {n: 34567, want: "35k"}, {n: 958000, want: "958k"}, {n: 999499, want: "999k"},
		{n: 999500, want: "1.0M"}, {n: 5700000, want: "5.7M"}, {n: 9949999, want: "9.9M"},
		{n: 9950000, want: "10.0M"}, {n: 16100000, want: "16.1M"}, {n: 62795575, want: "62.8M"}, {n: 999_499_999, want: "999.5M"},
		{n: 999_500_000, want: "999.5M"}, {n: 999_949_999, want: "999.9M"},
		{n: 999_950_000, want: "1.0B"}, {n: 1_234_567_890, want: "1.2B"}, {n: 1_500_000_000, want: "1.5B"}, {n: 9_949_999_999, want: "9.9B"},
		{n: 9_950_000_000, want: "10B"}, {n: 123_456_789_012, want: "123B"}, {n: 1_234_567_890_123, want: "1235B"},
	}
	for _, tt := range tests {
		if got := prose.Tokens(tt.n); got != tt.want {
			t.Errorf("Tokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestCharactersAreCountedInThousandsToATenthAtEverySize(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0.0k"}, {n: 49, want: "0.0k"}, {n: 50, want: "0.1k"}, {n: 800, want: "0.8k"}, {n: 999, want: "1.0k"},
		{n: 5100, want: "5.1k"}, {n: 9949, want: "9.9k"}, {n: 9950, want: "10.0k"}, {n: 14_200, want: "14.2k"},
		{n: 958_250, want: "958.3k"}, {n: 999_949, want: "999.9k"}, {n: 999_950, want: "1000.0k"}, {n: 1_234_567, want: "1234.6k"},
	}
	for _, tt := range tests {
		if got := prose.Characters(tt.n); got != tt.want {
			t.Errorf("Characters(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestTimesAreSaidOnceTwiceThenCounted(t *testing.T) {
	for n, want := range map[int]string{0: "0 times", 1: "once", 2: "twice", 3: "3 times", 12: "12 times"} {
		if got := prose.Times(n); got != want {
			t.Errorf("Times(%d) = %q, want %q", n, got, want)
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
		{n: 1240, want: "1,240 sessions"},
	}
	for _, tt := range tests {
		if got := prose.SessionCount(tt.n); got != tt.want {
			t.Errorf("SessionCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestACountedNounOrNoneSaysNoneInItsOwnWords(t *testing.T) {
	tests := []struct {
		n          int
		noun, zero string
		want       string
	}{
		{n: 0, noun: "move", zero: "none", want: "none"},
		{n: 0, noun: "session", zero: "no sessions", want: "no sessions"},
		{n: 1, noun: "move", zero: "none", want: "1 move"},
		{n: 4, noun: "move", zero: "none", want: "4 moves"},
		{n: 1240, noun: "request", zero: "none", want: "1,240 requests"},
	}
	for _, tt := range tests {
		if got := prose.CountedOr(tt.n, tt.noun, tt.zero); got != tt.want {
			t.Errorf("CountedOr(%d, %q, %q) = %q, want %q", tt.n, tt.noun, tt.zero, got, tt.want)
		}
	}
}

func TestMillionsAreToAPlaceThenWholeThenBillionsToTwo(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0.0M"},
		{n: 12, want: "0.0M"},
		{n: 800_000, want: "0.8M"},
		{n: 4_249_999, want: "4.2M"},
		{n: 75_400_000, want: "75.4M"},
		{n: 99_949_999, want: "99.9M"},
		{n: 99_950_000, want: "100M"},
		{n: 615_000_000, want: "615M"},
		{n: 999_499_999, want: "999M"},
		{n: 999_500_000, want: "1.00B"},
		{n: 1_056_800_000, want: "1.06B"},
	}
	for _, tt := range tests {
		if got := prose.Millions(tt.n); got != tt.want {
			t.Errorf("Millions(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestThousandsAreAsTheyAreUnderAThousandThenAsManyPlacesAsTheyTake(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0"}, {n: 450, want: "450"}, {n: 999, want: "999"}, {n: 1000, want: "1k"}, {n: 1250, want: "1.25k"},
		{n: 12000, want: "12k"}, {n: 12500, want: "12.5k"},
	}
	for _, tt := range tests {
		if got := prose.Thousands(tt.n); got != tt.want {
			t.Errorf("Thousands(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
