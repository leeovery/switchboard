package dashboard

import "testing"

func TestRevealingScrollsNoFurtherThanItMustToShowTheAccount(t *testing.T) {
	ten := Scrolling{Most: 6, Page: 4}
	tests := []struct {
		name  string
		at, i int
		s     Scrolling
		want  int
	}{
		{name: "shown already, the first of the page: where it is", at: 2, i: 2, s: ten, want: 2},
		{name: "shown already, the last of the page: where it is", at: 2, i: 5, s: ten, want: 2},
		{name: "below the page: till it's the last shown", at: 0, i: 6, s: ten, want: 3},
		{name: "the last account: to the foot", at: 0, i: 9, s: ten, want: 6},
		{name: "above the page: till it's the first shown", at: 5, i: 1, s: ten, want: 1},
		{name: "none of them: where it is", at: 3, i: -1, s: ten, want: 3},
		{name: "scrolled past the foot, as the frame now has it: from the foot", at: 9, i: 7, s: ten, want: 6},
		{name: "nothing scrolls: the top", at: 2, i: 2, s: Scrolling{}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.revealing(tt.at, tt.i); got != tt.want {
				t.Errorf("%+v.revealing(%d, %d) = %d, want %d", tt.s, tt.at, tt.i, got, tt.want)
			}
		})
	}
}
