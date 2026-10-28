package prose_test

import (
	"fmt"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

// The ways a test replaces an unseen character MapUnseen hands it.
var (
	dropped = func(rune) string { return "" }
	marked  = func(r rune) string { return fmt.Sprintf("<%X>", r) }
	// keptMarks keeps a direction mark and drops anything else, as a
	// label's taken.
	keptMarks = func(r rune) string {
		if prose.DirectionMark(r) {
			return string(r)
		}
		return ""
	}
)

func TestMapUnseen(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		replace func(rune) string
		want    string
	}{
		{name: "a zero-width space dropped", text: "a\u200bb", replace: dropped, want: "ab"},
		{name: "seen characters as they are", text: "a \u0600 b", replace: marked, want: "a \u0600 b"},
		{name: "a dropped character passed over, the selector after it judged on what's before", text: "\u263a\u200b\ufe0f", replace: dropped, want: "\u263a\ufe0f"},
		{name: "a dropped character passed over, the joiner after it judged on what's before", text: "\u0915\u2063\u200d\u0937", replace: dropped, want: "\u0915\u200d\u0937"},
		{name: "a kept direction mark counted, so the selector after it is replaced", text: "\u263a\u200e\ufe0f", replace: keptMarks, want: "\u263a\u200e"},
		{name: "a replaced character owning nothing, so the selector after it is replaced", text: "\u263a\u200b\ufe0f", replace: marked, want: "\u263a<200B><FE0F>"},
		{name: "a replaced character owning nothing, so the joiner after it is replaced", text: "\u0915\u200b\u200d\u0937", replace: marked, want: "\u0915<200B><200D>\u0937"},
		{name: "an emoji's joiner owned", text: "\U0001f469\u200d\U0001f4bb", replace: marked, want: "\U0001f469\u200d\U0001f4bb"},
		{name: "a keycap's selector owned", text: "1\ufe0f\u20e3", replace: marked, want: "1\ufe0f\u20e3"},
		{name: "a digit's selector without a keycap replaced", text: "1\ufe0f", replace: marked, want: "1<FE0F>"},
		{name: "a flag's tags owned whole", text: england, replace: marked, want: england},
		{name: "an unfinished flag's tags replaced", text: "\U0001f3f4\U000e0067", replace: marked, want: "\U0001f3f4<E0067>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.MapUnseen(tt.text, tt.replace); got != tt.want {
				t.Errorf("MapUnseen(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
