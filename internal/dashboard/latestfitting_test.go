package dashboard

import (
	"slices"
	"testing"
)

// The latest runs that fit lie in the room, the earliest giving way first:
// their spans, from 0, gap apart.
func TestLatestFittingKeepsTheLatestThatFit(t *testing.T) {
	tests := []struct {
		name                    string
		room, count, cells, gap int
		want                    []int
	}{
		{name: "all fit", room: 41, count: 6, cells: 5, gap: 1, want: []int{0, 6, 12, 18, 24, 30}},
		{name: "exactly", room: 35, count: 6, cells: 5, gap: 1, want: []int{0, 6, 12, 18, 24, 30}},
		{name: "a cell short, the earliest gone", room: 34, count: 6, cells: 5, gap: 1, want: []int{0, 6, 12, 18, 24}},
		{name: "four weeks at 73", room: 62, count: 6, cells: 14, gap: 1, want: []int{0, 15, 30, 45}},
		{name: "none", room: 4, count: 6, cells: 5, gap: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			for _, s := range latestFitting(tt.room, tt.count, tt.cells, tt.gap) {
				if s.Size != tt.cells {
					t.Errorf("a run is %d cells, want %d", s.Size, tt.cells)
				}
				got = append(got, s.At)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the runs lie at %v, want %v", got, tt.want)
			}
		})
	}
}
