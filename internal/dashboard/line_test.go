package dashboard

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestWrap(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		maxLines int
		want     []string
	}{
		{name: "on one line", text: "HTTP 529 · Overloaded", width: 30, maxLines: 2, want: []string{"HTTP 529 · Overloaded"}},
		{name: "between words", text: "token missing: write it to tokens/personal", width: 22, maxLines: 3, want: []string{"token missing: write", "it to tokens/personal"}},
		{name: "a word too long for a line", text: "abcdefghijklmnopqrstuvwxyz", width: 10, maxLines: 3, want: []string{"abcdefghij", "klmnopqrst", "uvwxyz"}},
		{name: "after a broken word", text: "abcdefghijkl mn", width: 10, maxLines: 3, want: []string{"abcdefghij", "kl mn"}},
		{name: "wide characters", text: "東京都 大阪府", width: 5, maxLines: 4, want: []string{"東京", "都", "大阪", "府"}},
		{name: "cut short on its last line", text: "one two three four five six", width: 9, maxLines: 2, want: []string{"one two", "three fo…"}},
		{name: "a broken word cut short", text: "abcdefghijklmnopqrstuvwxyz", width: 10, maxLines: 2, want: []string{"abcdefghij", "klmnopqrs…"}},
		{name: "nothing", text: "", width: 10, maxLines: 2, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wrap(tt.text, tt.width, tt.maxLines); !slices.Equal(got, tt.want) {
				t.Errorf("wrap(%q, %d, %d) = %q, want %q", tt.text, tt.width, tt.maxLines, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{name: "fits", text: "Work", width: 4, want: "Work"},
		{name: "cut", text: "Personal", width: 5, want: "Pers…"},
		{name: "cut after a space", text: "Work team", width: 6, want: "Work…"},
		{name: "wide characters", text: "東京都", width: 4, want: "東…"},
		{name: "room for the ellipsis alone", text: "Work", width: 1, want: "…"},
		{name: "no room", text: "Work", width: 0, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.text, tt.width); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

func TestFit(t *testing.T) {
	name := line{{"Switch", nameInk}, {"board", dimInk}}
	tests := []struct {
		name  string
		line  line
		width int
		want  line
	}{
		{name: "fits", line: name, width: 11, want: name},
		{name: "cut inside a span, keeping its ink", line: name, width: 8, want: line{{"Switch", nameInk}, {"b…", dimInk}}},
		{name: "cut at the end of a span", line: name, width: 6, want: line{{"Switc…", nameInk}}},
		{name: "no room", line: name, width: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.line.fit(tt.width); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fit(%d) = %q, want %q", tt.width, got.plain(), tt.want.plain())
			}
		})
	}
}

func TestSpread(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		want        string
	}{
		{name: "apart", left: "Session", right: "37%", want: "Session  37%"},
		{name: "left cut to keep a space", left: "Fable week", right: "104%", want: "Fable…  104%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spread(line{{tt.left, textInk}}, line{{tt.right, textInk}}, 12)
			if got.plain() != tt.want {
				t.Errorf("spread(%q, %q, 12) = %q, want %q", tt.left, tt.right, got.plain(), tt.want)
			}
		})
	}
}

// plain is the line's text without its inks.
func (l line) plain() string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.text)
	}
	return b.String()
}
