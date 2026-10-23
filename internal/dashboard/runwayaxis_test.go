package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// The axis has now over now's column, from the cell before it, where the
// timeline has a now; under it, the hours or the days; and under them, a
// rule across the lanes, ticked.
func TestRunwaysAxisHasNowOverItsColumnAndARuleUnderItsLabels(t *testing.T) {
	tests := []struct {
		name  string
		shown RunwayShown
		now   int
	}{
		{name: "live", shown: runwayLive(views.RunwayFiveHour), now: 6},
		{name: "the week, live", shown: runwayLive(views.RunwayWeek), now: 19},
		{name: "today, stepped back to", shown: runwayBack(views.RunwayFiveHour, civilDay(10, 1)), now: 83},
		{name: "a day gone", shown: runwayBack(views.RunwayFiveHour, civilDay(9, 30)), now: -1},
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

// The 5-hour view's hours are ticked and labelled as far apart as its
// columns' length asks, at each width: ticks three cells apart at the
// least, labels ten.
func TestTheDaysHoursAreTickedAndLabelledAsFarApartAsTheWidthAsks(t *testing.T) {
	tests := []struct {
		width, ticks, labels int
	}{
		{width: 200, ticks: 1, labels: 2},
		{width: 160, ticks: 1, labels: 2},
		{width: 122, ticks: 1, labels: 3},
		{width: 100, ticks: 1, labels: 3},
		{width: 80, ticks: 2, labels: 6},
		{width: 52, ticks: 2, labels: 6},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			tl := views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayShown{}.Columns(tt.width))
			c := newCanvas(tl.Columns, 2)
			hours(c, tl, 0, 0)
			midnight := tl.Column(tl.Midnights()[0])
			for _, at := range tl.Hours(1) {
				col := tl.Column(at)
				ticked, label := c.at(col, 1).glyph == "┬", labelFrom(c, col)
				clear := max(col-midnight, midnight-col) >= labelCells && col+5 <= tl.Columns
				switch {
				case ticked != (at.Hour()%tt.ticks == 0):
					t.Errorf("%s, in column %d, ticked %t, want ticks every %d hours", at.Format("15:04"), col, ticked, tt.ticks)
				case clear && (label != "") != (at.Hour()%tt.labels == 0):
					t.Errorf("%s, in column %d, labelled %q, want labels every %d hours", at.Format("15:04"), col, label, tt.labels)
				case label != "" && label != at.Format("15:04"):
					t.Errorf("%s is labelled %q", at.Format("15:04"), label)
				}
			}
		})
	}
}

// A day or week stepped back to, which has no now, has its hours labelled
// and its days ticked by its own clock, as now's time zone has them.
func TestADayOrWeekSteppedBackToIsToldByItsOwnClock(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load America/New_York: %v", err)
	}
	now := routedAt.In(newYork)
	day := views.Stepped(views.RunwayFiveHour, civilDay(9, 30), now, time.Monday, 136)
	c := newCanvas(136, 2)
	hours(c, day, 0, 0)
	if two := time.Date(2026, 9, 30, 2, 0, 0, 0, newYork); labelAt(c, day.Column(two)) != "02:00" {
		t.Errorf("Wednesday's 02:00 in New York is labelled %q, want 02:00", labelAt(c, day.Column(two)))
	}

	week := views.Stepped(views.RunwayWeek, civilDay(9, 21), now, time.Monday, 136)
	c = newCanvas(136, 2)
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
		{name: "live", shown: runwayLive(views.RunwayWeek), today: true},
		{name: "this week, stepped back to", shown: runwayBack(views.RunwayWeek, civilDay(9, 28)), today: true},
		{name: "a week gone", shown: runwayBack(views.RunwayWeek, civilDay(9, 21))},
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
			weekdays(c, views.LiveTimeline(views.RunwayWeek, routedAt, tt.columns), 0, 0)
			if names, rule := textAt(c, 0, 0, tt.columns), textAt(c, 0, 1, tt.columns); names != tt.names || rule != tt.rule {
				t.Errorf("the week's axis reads\n%q\n%q, want\n%q\n%q", names, rule, tt.names, tt.rule)
			}
		})
	}
}

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
			// The day starts at the first midnight. The clocks read 00:00
			// again an hour on, unticked, as an hour is ticked where the
			// clocks first read it; then 01:00, ticked.
			want: map[time.Time]string{time.Date(2020, 10, 23, 20, 0, 0, 0, time.UTC): "", time.Date(2020, 10, 23, 21, 0, 0, 0, time.UTC): "Sat",
				time.Date(2020, 10, 23, 22, 0, 0, 0, time.UTC): "-", time.Date(2020, 10, 23, 23, 0, 0, 0, time.UTC): ""},
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

// labelFrom is the label on the canvas's top row that starts in column col:
// none where that's blank, or in a label that starts before it.
func labelFrom(c *canvas, col int) string {
	if col > 0 && c.at(col-1, 0).glyph != " " {
		return ""
	}
	return labelAt(c, col)
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
