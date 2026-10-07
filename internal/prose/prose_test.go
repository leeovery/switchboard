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

func TestACountIsAsBriefAsARowHasRoomFor(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0"}, {n: 999, want: "999"}, {n: 1000, want: "1.0k"}, {n: 1234, want: "1.2k"},
		{n: 9949, want: "9.9k"}, {n: 9950, want: "10k"}, {n: 34567, want: "35k"}, {n: 999499, want: "999k"},
		{n: 999500, want: "1.0M"}, {n: 1234567, want: "1.2M"}, {n: 9949999, want: "9.9M"}, {n: 9950000, want: "10M"},
		{n: 62795575, want: "63M"}, {n: 999499999, want: "999M"},
		{n: 999500000, want: "1.0B"}, {n: 1234567890, want: "1.2B"}, {n: 9949999999, want: "9.9B"}, {n: 9950000000, want: "10B"},
		{n: 999499999999, want: "999B"}, {n: 999500000000, want: "1000B"},
	}
	for _, tt := range tests {
		if got := prose.Count(tt.n); got != tt.want {
			t.Errorf("Count(%d) = %q, want %q", tt.n, got, tt.want)
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
