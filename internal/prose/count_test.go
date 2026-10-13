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

func TestTokensAreCountedAsTheLogCountsThem(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0"}, {n: 845, want: "845"}, {n: 999, want: "999"},
		{n: 1000, want: "1.0k"}, {n: 5400, want: "5.4k"}, {n: 9949, want: "9.9k"},
		{n: 9950, want: "10k"}, {n: 34567, want: "35k"}, {n: 958000, want: "958k"}, {n: 999499, want: "999k"},
		{n: 999500, want: "1.0M"}, {n: 5700000, want: "5.7M"}, {n: 9949999, want: "9.9M"},
		{n: 9950000, want: "10.0M"}, {n: 16100000, want: "16.1M"}, {n: 62795575, want: "62.8M"}, {n: 1234567890, want: "1234.6M"},
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
	}
	for _, tt := range tests {
		if got := prose.SessionCount(tt.n); got != tt.want {
			t.Errorf("SessionCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
