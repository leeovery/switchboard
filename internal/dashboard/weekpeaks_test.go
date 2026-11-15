package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// A peak is coloured by its level, as usage colours a share, but bold
// state.destructive where it hit its limit, at its cap, as views marks it,
// and never by its figure alone; this week's so far is in the ink given,
// and one not known is a faint dash.
func TestAPeakIsColouredByItsLevelBoldAtItsCap(t *testing.T) {
	peak := func(share float64, open bool) views.WeekPeak { return views.WeekPeak{Share: new(share), Open: open} }
	atCap := func(share float64) views.WeekPeak { return views.WeekPeak{Share: new(share), AtCap: true} }
	tests := []struct {
		name string
		peak views.WeekPeak
		want line
	}{
		{name: "under 70%", peak: peak(0.69, false), want: line{{"69%", ink{token: theme.VizRamp1}}}},
		{name: "from 70%", peak: peak(0.70, false), want: line{{"70%", ink{token: theme.VizRamp2}}}},
		{name: "from 90%, as shown", peak: peak(0.895, false), want: line{{"90%", ink{token: theme.VizRamp4}}}},
		{name: "at its limit", peak: atCap(1), want: line{{"100%", exhaustedInk}}},
		{name: "a reserved account's 96%, at its cap", peak: atCap(0.96), want: line{{"96%", exhaustedInk}}},
		{name: "a reserved account's 94%, short of its cap", peak: peak(0.94, false), want: line{{"94%", ink{token: theme.VizRamp4}}}},
		{name: "99.6%, said 100% but short of its limit", peak: peak(0.996, false), want: line{{"100%", ink{token: theme.VizRamp4}}}},
		{name: "this week's so far", peak: peak(0.34, true), want: line{{"34%…", secondaryInk}}},
		{name: "not known", peak: views.WeekPeak{Open: true}, want: line{{"—", faintInk}}},
	}
	for _, tt := range tests {
		if got := fitParts(peakCells, peakPart(tt.peak, secondaryInk, Number{})); !slices.Equal(got, tt.want) {
			t.Errorf("%s: the peak is %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// A week's column fills from the bottom to its peak, as a whole percent,
// its top cell in eighths: where the look blends, a whole cell a space on
// the ramp's colour and a part one its eighth block; where it doesn't, in
// blocks; a peak over its limit fills the column; a week of no use is ▁
// along the bottom; and one not known draws nothing.
func TestAWeeksColumnFillsFromTheBottomItsTopCellInEighths(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	const rows, cells = 3, 5
	tests := []struct {
		name string
		look Look
		peak views.WeekPeak
		// want are the column's rows, top first, as text, and inked its
		// rows inked, the rest left blank.
		want  [rows]string
		inked int
	}{
		{name: "72%: two rows whole and an eighth", look: nord, peak: views.WeekPeak{Share: new(0.72)}, want: [rows]string{"▁▁▁▁▁"}, inked: 3},
		{name: "a look that can't blend fills in blocks", peak: views.WeekPeak{Share: new(0.72)}, want: [rows]string{"▁▁▁▁▁", "█████", "█████"}, inked: 3},
		{name: "filled to the whole percent", look: nord, peak: views.WeekPeak{Share: new(0.7267)}, want: [rows]string{"▂▂▂▂▂"}, inked: 3},
		{name: "over its limit", look: nord, peak: views.WeekPeak{Share: new(1.3), AtCap: true}, inked: 3},
		{name: "no use", look: nord, peak: views.WeekPeak{Share: new(0.0)}, want: [rows]string{2: "▁▁▁▁▁"}, inked: 1},
		{name: "not known", look: nord},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(cells, rows)
			peakColumn(c, tt.look, 0, rows-1, rows, cells, tt.peak)
			drawn := textOf(c.rows(Look{}))
			for r, want := range tt.want {
				if got := strings.TrimRight(drawn[r], " "); got != want {
					t.Errorf("row %d of the column reads %q, want %q", r, got, want)
				}
			}
			for r := range rows {
				if inked := c.at(0, r).ink != (ink{}); inked != (r >= rows-tt.inked) {
					t.Errorf("row %d of the column is inked %t, want the bottom %d rows inked", r, inked, tt.inked)
				}
			}
		})
	}
}
