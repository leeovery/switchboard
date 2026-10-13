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
		if got := prose.Clean(tt.text); got != tt.want {
			t.Errorf("Clean(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}
