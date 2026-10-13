package prose_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

func TestList(t *testing.T) {
	tests := []struct {
		items []string
		want  string
	}{
		{items: nil, want: ""},
		{items: []string{"work"}, want: "work"},
		{items: []string{"work", "side"}, want: "work and side"},
		{items: []string{"work", "personal", "side"}, want: "work, personal and side"},
	}
	for _, tt := range tests {
		if got := prose.List(tt.items); got != tt.want {
			t.Errorf("List(%q) = %q, want %q", tt.items, got, tt.want)
		}
	}
}

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

func TestTimesAreSaidOnceTwiceThenCounted(t *testing.T) {
	for n, want := range map[int]string{0: "0 times", 1: "once", 2: "twice", 3: "3 times", 12: "12 times"} {
		if got := prose.Times(n); got != want {
			t.Errorf("Times(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestCutEndsTextCutToItsCellsWithAnEllipsis(t *testing.T) {
	tests := []struct {
		text  string
		cells int
		want  string
	}{
		{text: "switchboard", cells: 12, want: "switchboard"},
		{text: "switchboard-dashboard", cells: 12, want: "switchboard…"},
		{text: "two words", cells: 5, want: "two…"},
		{text: "コードの部屋", cells: 7, want: "コード…"},
		{text: "anything", cells: 1, want: "…"},
		{text: "anything", cells: 0, want: ""},
		{text: "", cells: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.Cut(tt.text, tt.cells); got != tt.want {
			t.Errorf("Cut(%q, %d) = %q, want %q", tt.text, tt.cells, got, tt.want)
		}
	}
}

func TestCutFrontKeepsTheEndOfTextCutToItsCells(t *testing.T) {
	tests := []struct {
		text  string
		cells int
		want  string
	}{
		{text: "~/Code/switchboard", cells: 18, want: "~/Code/switchboard"},
		{text: "~/Code/clients/switchboard", cells: 13, want: "…/switchboard"},
		{text: "~/Code/clients/switchboard", cells: 16, want: "…nts/switchboard"},
		{text: "~/コード/部屋", cells: 6, want: "…/部屋"},
		{text: "anything", cells: 1, want: "…"},
		{text: "anything", cells: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.CutFront(tt.text, tt.cells); got != tt.want {
			t.Errorf("CutFront(%q, %d) = %q, want %q", tt.text, tt.cells, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{s: "0b5c6f2e-7d41-4a3b", n: 8, want: "0b5c6f2e"},
		{s: "0b5c6f2e", n: 8, want: "0b5c6f2e"},
		{s: "short", n: 8, want: "short"},
		{s: "", n: 8, want: ""},
		{s: "·······", n: 3, want: "···"},
		{s: "anything", n: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.Truncate(tt.s, tt.n); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestTruncateBytes(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{s: "0b5c6f2e-7d41-4a3b", n: 8, want: "0b5c6f2e"},
		{s: "0b5c6f2e", n: 8, want: "0b5c6f2e"},
		{s: "short", n: 8, want: "short"},
		{s: "", n: 8, want: ""},
		{s: "☃☃☃", n: 6, want: "☃☃"},
		{s: "☃☃☃", n: 5, want: "☃"},
		{s: "☃☃☃", n: 2, want: ""},
		{s: "anything", n: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.TruncateBytes(tt.s, tt.n); got != tt.want {
			t.Errorf("TruncateBytes(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestTruncateBytesFront(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{s: "~/Code/clients/project", n: 11, want: "…/project"},
		{s: "~/Code/clients/project", n: 12, want: "…s/project"},
		{s: "~/Code/project", n: 14, want: "~/Code/project"},
		{s: "short", n: 8, want: "short"},
		{s: "", n: 8, want: ""},
		{s: "☃☃☃", n: 6, want: "…☃"},
		{s: "☃☃☃", n: 8, want: "…☃"},
		{s: "☃☃☃", n: 5, want: "…"},
		{s: "☃☃☃", n: 3, want: "…"},
		{s: "☃☃☃", n: 2, want: ""},
		{s: "anything", n: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.TruncateBytesFront(tt.s, tt.n); got != tt.want {
			t.Errorf("TruncateBytesFront(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}
