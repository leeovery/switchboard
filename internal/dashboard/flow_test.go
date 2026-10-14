package dashboard

import (
	"slices"
	"testing"
)

func TestFlowLaysPartsOnLines(t *testing.T) {
	text := func(s string) line { return line{{s, mutedInk}} }
	tests := []struct {
		name   string
		chunks []chunk
		widths []int
		want   []string
	}{
		{name: "following on a line", chunks: []chunk{fresh(text("5 sessions")), {text: text("auto")}}, widths: []int{24, 24}, want: []string{"5 sessions · auto"}},
		{name: "each on a line of its own", chunks: []chunk{fresh(text("● healthy")), fresh(text("5 sessions"))}, widths: []int{24, 24}, want: []string{"● healthy", "5 sessions"}},
		{name: "on the next line, where it doesn't fit", chunks: []chunk{fresh(text("5 sessions")), {text: text("pinned to work and side")}}, widths: []int{24, 24}, want: []string{"5 sessions", "pinned to work and side"}},
		{name: "broken between words onto the next lines", chunks: []chunk{fresh(text("router unhealthy: no answer within 500ms"))}, widths: []int{24, 24}, want: []string{"router unhealthy: no", "answer within 500ms"}},
		{name: "its last line cut short", chunks: []chunk{fresh(text("one two three four five six seven"))}, widths: []int{9, 9}, want: []string{"one two", "three fo…"}},
		{name: "after the last line, following on it, cut short", chunks: []chunk{fresh(text("● healthy")), fresh(text("restart due (config changed)"))}, widths: []int{20}, want: []string{"● healthy · restart…"}},
		{name: "nothing", chunks: nil, widths: []int{24}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, l := range flow(tt.chunks, tt.widths, " · ") {
				got = append(got, l.plain())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("flow() = %q, want %q", got, tt.want)
			}
		})
	}
}
