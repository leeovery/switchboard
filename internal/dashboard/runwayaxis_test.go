package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

func TestTheWeeksAxisTicksTheQuartersOfItsFirstDayToo(t *testing.T) {
	tl := views.LiveTimeline(views.RunwayWeek, now, 136)
	c := newCanvas(140, 2)
	weekdays(c, tl, 0, 0)
	evening := time.Date(tl.Start.Year(), tl.Start.Month(), tl.Start.Day(), 18, 0, 0, 0, tl.Start.Location())
	if got := c.at(tl.Column(evening), 1).glyph; got != "╵" {
		t.Errorf("the first day's 18:00, in column %d, is ticked %q, want ╵", tl.Column(evening), got)
	}
}

func TestTheWeeksAxisTicksEachDaysStartAndNoQuarterBeforeItWhereTheClocksGoForwardAtMidnight(t *testing.T) {
	santiago, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatalf("load America/Santiago: %v", err)
	}
	// Chile's clocks went from 00:00 to 01:00 on Sunday 6 September 2026: a
	// week of hours, from noon on the 4th.
	tl := views.LiveTimeline(views.RunwayWeek, time.Date(2026, 9, 5, 12, 0, 0, 0, santiago), 168)
	c := newCanvas(168, 2)
	weekdays(c, tl, 0, 0)
	at := func(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 0, 0, 0, santiago) }
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "the 6th's start, as the clocks went forward", at: at(6, 1), want: "┬"},
		{name: "the hour before it, the 5th's last", at: at(5, 23), want: " "},
		{name: "the 6th's first quarter", at: at(6, 6), want: "╵"},
		{name: "the 7th's start", at: at(7, 0), want: "┬"},
	}
	for _, tt := range tests {
		if got := c.at(tl.Column(tt.at), 1).glyph; got != tt.want {
			t.Errorf("%s, %v, in column %d, is ticked %q, want %q", tt.name, tt.at, tl.Column(tt.at), got, tt.want)
		}
	}
}

func TestTheDaysAxisNamesEachDayAtItsStartWhereTheClocksChangeOverMidnight(t *testing.T) {
	tests := []struct {
		name, zone string
		// now is the time by the clock, in the zone; and want, by the time
		// each falls in, the labels over the ticks, a tick where it's "".
		now  time.Time
		want map[time.Time]string
	}{
		{
			name: "the clocks went forward over it, as Chile's did on Sunday 6 September 2026",
			zone: "America/Santiago", now: time.Date(2026, 9, 6, 2, 30, 0, 0, time.UTC),
			// The day starts at 01:00, as the clocks went forward from 00:00,
			// and 02:00 is ticked but not labelled, an hour from its name.
			want: map[time.Time]string{time.Date(2026, 9, 6, 3, 0, 0, 0, time.UTC): "", time.Date(2026, 9, 6, 4, 0, 0, 0, time.UTC): "Sun",
				time.Date(2026, 9, 6, 5, 0, 0, 0, time.UTC): ""},
		},
		{
			name: "the clocks went back over it, as Gaza's did on Saturday 24 October 2020",
			zone: "Asia/Gaza", now: time.Date(2020, 10, 23, 20, 30, 0, 0, time.UTC),
			// The day starts at the first midnight, the clocks reading 00:00
			// again an hour on, ticked but not labelled, an hour from its name.
			want: map[time.Time]string{time.Date(2020, 10, 23, 20, 0, 0, 0, time.UTC): "", time.Date(2020, 10, 23, 21, 0, 0, 0, time.UTC): "Sat",
				time.Date(2020, 10, 23, 22, 0, 0, 0, time.UTC): ""},
		},
		{
			name: "the clocks went forward half an hour, as Lord Howe's did on Sunday 4 October 2026",
			zone: "Australia/Lord_Howe", now: time.Date(2026, 10, 3, 16, 0, 0, 0, time.UTC),
			// From 02:00 to 02:30: no tick for an hour the clocks never read.
			want: map[time.Time]string{time.Date(2026, 10, 3, 15, 30, 0, 0, time.UTC): "-", time.Date(2026, 10, 3, 16, 0, 0, 0, time.UTC): ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.zone)
			if err != nil {
				t.Fatalf("load %s: %v", tt.zone, err)
			}
			tl := views.LiveTimeline(views.RunwayFiveHour, tt.now.In(loc), 136)
			c := newCanvas(136, 2)

			hours(c, tl, 0, 0)
			for at, want := range tt.want {
				col := tl.Column(at)
				got, tick := labelAt(c, col), c.at(col, 1).glyph
				switch {
				case want == "-" && (tick != " " || got != ""):
					t.Errorf("at %v, in column %d, the axis reads %q over %q, want nothing", at.In(loc), col, got, tick)
				case want != "-" && (tick != "┬" || got != want):
					t.Errorf("at %v, in column %d, the axis reads %q over %q, want %q over a tick", at.In(loc), col, got, tick, want)
				}
			}
			if cols := labelColumns(c, 136); slices.ContainsFunc(cols[min(1, len(cols)):], func(col int) bool {
				i := slices.Index(cols, col)
				return col-cols[i-1] < labelCells
			}) {
				t.Errorf("the labels start in columns %v, want them %d apart at least", cols, labelCells)
			}
		})
	}
}

// labelAt is the label on the canvas's top row that starts in column col:
// none where that's blank.
func labelAt(c *canvas, col int) string {
	var label strings.Builder
	for ; col < c.width && c.at(col, 0).glyph != " "; col++ {
		label.WriteString(c.at(col, 0).glyph)
	}
	return label.String()
}

// labelColumns are the columns the labels on the canvas's top row start in,
// width cells wide.
func labelColumns(c *canvas, width int) []int {
	var cols []int
	for col := range width {
		if c.at(col, 0).glyph != " " && (col == 0 || c.at(col-1, 0).glyph == " ") {
			cols = append(cols, col)
		}
	}
	return cols
}
