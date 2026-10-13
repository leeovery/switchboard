package views

import (
	"slices"
	"testing"
)

// A choice ticks those ticked, or every one where none is; toggling one
// flips its tick, and a choice that would tick every one, or none, is none,
// which ticks every one.
func TestTogglingAChoiceFlipsOneTickAndTicksEveryOneAsNone(t *testing.T) {
	every := []string{"work", "personal", "side"}
	tests := []struct {
		name         string
		choice       []string
		one          string
		want         []string
		wantTicksOne bool
	}{
		{name: "every one ticked, one unticked", one: "work", want: []string{"personal", "side"}, wantTicksOne: false},
		{name: "one ticked again: every one, as none", choice: []string{"personal", "side"}, one: "work", want: nil, wantTicksOne: true},
		{name: "one more ticked", choice: []string{"work"}, one: "side", want: []string{"work", "side"}, wantTicksOne: true},
		{name: "the last unticked: none, which is every one", choice: []string{"side"}, one: "side", want: nil, wantTicksOne: true},
		{name: "one ticked of none there is", choice: []string{"gone"}, one: "work", want: []string{"work"}, wantTicksOne: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Toggled(tt.choice, every, tt.one)
			if !slices.Equal(got, tt.want) || (got == nil) != (tt.want == nil) {
				t.Errorf("Toggled(%q, %q) = %q, want %q", tt.choice, tt.one, got, tt.want)
			}
			if ticks := Ticks(got, tt.one); ticks != tt.wantTicksOne {
				t.Errorf("Ticks(%q, %q) = %v, want %v", got, tt.one, ticks, tt.wantTicksOne)
			}
		})
	}
}
