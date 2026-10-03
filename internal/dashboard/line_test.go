package dashboard

import (
	"reflect"
	"strings"
	"testing"
)

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

func TestFitWholeLeavesOffWholeSpans(t *testing.T) {
	says := line{{"runs out ~16:05", exhaustedInk}, {"  ·  back 17:10", mutedInk}, {", as it resets", mutedInk}}
	tests := []struct {
		name  string
		width int
		want  line
	}{
		{name: "fits", width: 44, want: says},
		{name: "its last left off", width: 43, want: says[:2]},
		{name: "all but its first left off", width: 20, want: says[:1]},
		{name: "its first cut short, where alone it doesn't fit", width: 9, want: line{{"runs out…", exhaustedInk}}},
		{name: "no room", width: 0, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := says.fitWhole(tt.width); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fitWhole(%d) = %q, want %q", tt.width, got.plain(), tt.want.plain())
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
