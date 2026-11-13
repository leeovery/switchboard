package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// The axis has now over now's column, from the cell before it, where the
// timeline has a now; under it, the days; and under them, a rule across the
// lanes, ticked.
func TestRunwaysAxisHasNowOverItsColumnAndARuleUnderItsLabels(t *testing.T) {
	tests := []struct {
		name  string
		shown RunwayShown
		now   int
	}{
		{name: "live", shown: runwayLive(), now: 19},
		{name: "this week, stepped back to", shown: runwayBack(civilDay(9, 28)), now: 70},
		{name: "a week gone", shown: runwayBack(civilDay(9, 21)), now: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(160, 12)
			runwayShowing(tt.shown).runwayAxis(c, runwayLaid{x: framesLanesFrom, fit: runwayFit{fits: true, axis: 6}})
			row := textAt(c, 0, 6, 160)
			if want := strings.Repeat(" ", framesLanesFrom+tt.now-1) + "now"; tt.now >= 0 && row != want || tt.now < 0 && row != "" {
				t.Errorf("the axis's first row reads %q", row)
			}
			if got := c.at(framesLanesFrom+tt.now, 6).ink; tt.now >= 0 && got != titleInk {
				t.Errorf("now is in %v, want %v", got, titleInk)
			}
			rule := strings.NewReplacer("┬", "─", "╵", "─").Replace(textAt(c, framesLanesFrom, 8, 136))
			if rule != strings.Repeat("─", 136) || c.at(framesLanesFrom-1, 8).glyph != " " {
				t.Errorf("the axis's rule reads %q, want 136 cells from the lanes' first column", rule)
			}
		})
	}
}

// Where now's column is the lanes' first and the lanes start at column 0, as
// collapsed on this week just after it starts, now reads whole from column
// 0, never cut at the frame's edge.
func TestNowReadsWholeWhereItsColumnIsTheFirstAtTheFramesEdge(t *testing.T) {
	shown := RunwayShown{
		Back: civilDay(9, 28), Timeline: views.Stepped(civilDay(9, 28), time.Date(2026, 9, 28, 0, 5, 0, 0, time.UTC), time.Monday, 160),
	}
	if col := shown.Timeline.NowColumn(); col != 0 {
		t.Fatalf("now's column is %d, want 0", col)
	}
	c := newCanvas(160, 12)
	runwayShowing(shown).runwayAxis(c, runwayLaid{x: 0, fit: runwayFit{fits: true, axis: 6}})
	if got := rowOf(c, 6); got != "now" {
		t.Errorf("the axis's first row reads %q, want now from column 0", got)
	}
}

// A week stepped back to, which has no now, has its days ticked by its own
// clock, as now's time zone has them.
func TestAWeekSteppedBackToIsToldByItsOwnClock(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load America/New_York: %v", err)
	}
	week := views.Stepped(civilDay(9, 21), routedAt.In(newYork), time.Monday, 136)
	c := newCanvas(136, 2)
	weekdays(c, week, 0, 0)
	for i := range 7 {
		midnight := time.Date(2026, 9, 21+i, 0, 0, 0, 0, newYork)
		got := c.at(week.Column(midnight), 0)
		if c.at(week.Column(midnight), 1).glyph != "┬" || labelAt(c, week.Column(midnight)) != midnight.Format("Mon") || got.ink != mutedInk {
			t.Errorf("%s's midnight in New York reads %q in %v over %q, want its name, muted, over a tick", midnight.Format("Mon 2"), labelAt(c, week.Column(midnight)), got.ink, c.at(week.Column(midnight), 1).glyph)
		}
	}
}

func TestTheWeeksAxisPicksOutTodayWhereTheTimelineHasANow(t *testing.T) {
	for _, tt := range []struct {
		name  string
		shown RunwayShown
		today bool
	}{
		{name: "live", shown: runwayLive(), today: true},
		{name: "this week, stepped back to", shown: runwayBack(civilDay(9, 28)), today: true},
		{name: "a week gone", shown: runwayBack(civilDay(9, 21))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tl := tt.shown.Timeline
			c := newCanvas(136, 2)
			weekdays(c, tl, 0, 0)
			var picked []string
			for _, at := range tl.Midnights() {
				if c.at(tl.Column(at), 0).ink == strongInk {
					picked = append(picked, labelAt(c, tl.Column(at)))
				}
			}
			if want := []string{"Thu"}; tt.today && !slices.Equal(picked, want) || !tt.today && picked != nil {
				t.Errorf("the days picked out are %v, want today's alone where there's a now", picked)
			}
		})
	}
}

// The week's axis ticks each day's quarters where a day spans 12 columns,
// and names each day where it clears the name before by five cells.
func TestTheWeeksAxisTicksQuartersAndNamesDaysAsTheWidthAllows(t *testing.T) {
	tests := []struct {
		name        string
		columns     int
		names, rule string
	}{
		{
			name: "a day 12 columns, its quarters ticked", columns: 84,
			names: "    Thu         Fri         Sat         Sun         Mon         Tue         Wed",
			rule:  " ╵  ┬  ╵  ╵  ╵  ┬  ╵  ╵  ╵  ┬  ╵  ╵  ╵  ┬  ╵  ╵  ╵  ┬  ╵  ╵  ╵  ┬  ╵  ╵  ╵  ┬  ╵  ╵",
		},
		{
			name: "a day 8 columns, its quarters not", columns: 56,
			names: "   Thu     Fri     Sat     Sun     Mon     Tue     Wed",
			rule:  "   ┬       ┬       ┬       ┬       ┬       ┬       ┬",
		},
		{
			name: "a day under 4 columns, a name where it clears the one before", columns: 26,
			names: " Thu    Sat     Mon    Wed",
			rule:  " ┬   ┬  ┬   ┬   ┬   ┬  ┬",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(tt.columns, 2)
			weekdays(c, views.LiveTimeline(routedAt, tt.columns), 0, 0)
			if names, rule := textAt(c, 0, 0, tt.columns), textAt(c, 0, 1, tt.columns); names != tt.names || rule != tt.rule {
				t.Errorf("the week's axis reads\n%q\n%q, want\n%q\n%q", names, rule, tt.names, tt.rule)
			}
		})
	}
}

func TestTheWeeksAxisTicksTheQuartersOfItsFirstDayToo(t *testing.T) {
	tl := views.LiveTimeline(now, 136)
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
	tl := views.LiveTimeline(time.Date(2026, 9, 5, 12, 0, 0, 0, santiago), 168)
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

// labelAt is the label on the canvas's top row that starts in column col:
// none where that's blank.
func labelAt(c *canvas, col int) string {
	var label strings.Builder
	for ; col < c.width && c.at(col, 0).glyph != " "; col++ {
		label.WriteString(c.at(col, 0).glyph)
	}
	return label.String()
}
