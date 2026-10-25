package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// axisOf is the axis of a card of a window from start for length, at now,
// drawn width cells wide, and the canvas it's drawn on, its axis on row 1.
func axisOf(start time.Time, length time.Duration, now time.Time, width int) (string, *canvas) {
	cd := burnCard{window: chart{start: start, length: length, spanned: true, now: now}, ahead: steady(0, false)}
	c := cardIn(cd, Look{}, width, 1)
	return rowOf(c, 1), c
}

func TestAWeeksAxisTicksEachMidnightWithItsWeekdayGivingWayAsItNarrows(t *testing.T) {
	// From Monday's midnight for a week; now on Thursday.
	start := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		from  time.Duration
		width int
		want  string
	}{
		{name: "ten columns a day", width: 70, want: "╵Mon      ╵Tue      ╵Wed      ╵Thu      ╵Fri      ╵Sat      ╵Sun"},
		{name: "five: each whole", width: 35, want: "╵Mon ╵Tue ╵Wed ╵Thu ╵Fri ╵Sat ╵Sun"},
		{name: "under five: two letters", width: 28, want: "╵Mo ╵Tu ╵We ╵Th ╵Fr ╵Sa ╵Su"},
		{name: "three: a tick whose label meets the next left out", width: 21, want: "╵Mo   ╵We   ╵Fr   ╵Su"},
		{name: "just under five", width: 34, want: "╵Mo ╵Tu  ╵We  ╵Th  ╵Fr  ╵Sa  ╵Su"},
		// From 06:00, the next Monday's midnight is in column 33, its label
		// past the card's end.
		{name: "a label past the card's end left out", from: 6 * time.Hour, width: 35, want: "   ╵Tue ╵Wed ╵Thu ╵Fri ╵Sat ╵Sun"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := axisOf(start.Add(tt.from), 7*quota.Day, start.Add(3*quota.Day+12*time.Hour), tt.width); got != tt.want {
				t.Errorf("the axis is\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestAWeeksAxisPicksOutToday(t *testing.T) {
	start := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	_, c := axisOf(start, 7*quota.Day, start.Add(3*quota.Day+12*time.Hour), 35)
	for x, want := range map[int]ink{0: faintInk, 1: dimInk, 15: faintInk, 16: strongInk, 18: strongInk, 21: dimInk} {
		if got := c.at(x, 1).ink; got != want {
			t.Errorf("the axis's column %d is in %+v, want %+v: ╵ faint, each weekday subtle, today's bold text.secondary", x, got, want)
		}
	}
}

func TestAFiveHoursAxisTicksEachWholeHourGivingWayAsItNarrows(t *testing.T) {
	// From 11:00 for five hours.
	start := time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		width int
		want  string
	}{
		{name: "ten columns an hour", width: 50, want: "╵11:00    ╵12:00    ╵13:00    ╵14:00    ╵15:00"},
		{name: "seven: each whole", width: 35, want: "╵11:00 ╵12:00 ╵13:00 ╵14:00 ╵15:00"},
		{name: "under seven: the hour alone", width: 30, want: "╵11   ╵12   ╵13   ╵14   ╵15"},
		{name: "three: a tick whose label meets the next left out", width: 15, want: "╵11   ╵13   ╵15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, c := axisOf(start, 5*time.Hour, start.Add(90*time.Minute), tt.width)
			if got != tt.want {
				t.Errorf("the axis is\n%q\nwant\n%q", got, tt.want)
			}
			if k := c.at(1, 1).ink; k != dimInk {
				t.Errorf("an hour is in %+v, want text.subtle", k)
			}
		})
	}
}

func TestACardsAxisTicksInNowsTimeZoneAsTheClocksChange(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		start  time.Time
		length time.Duration
		width  int
		want   string
	}{
		{
			// A column an hour; Sunday 25 October has 25 of them, so Monday
			// starts a column later than a day of 24 would put it.
			name: "a week over the clocks going back", start: time.Date(2026, 10, 22, 0, 0, 0, 0, london), length: 7 * quota.Day, width: 168,
			want: "╵Thu" + strings.Repeat(" ", 20) + "╵Fri" + strings.Repeat(" ", 20) + "╵Sat" + strings.Repeat(" ", 20) +
				"╵Sun" + strings.Repeat(" ", 21) + "╵Mon" + strings.Repeat(" ", 20) + "╵Tue" + strings.Repeat(" ", 20) + "╵Wed",
		},
		{
			// Ten columns an hour from midnight: 01:00 is read twice, ticked
			// the first time, and 02:00 comes two hours after it.
			name: "a 5-hour over the clocks going back", start: time.Date(2026, 10, 25, 0, 0, 0, 0, london), length: 5 * time.Hour, width: 50,
			want: "╵00:00    ╵01:00              ╵02:00    ╵03:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := tt.start.Add(time.Hour)
			if got, _ := axisOf(tt.start.UTC(), tt.length, now, tt.width); got != tt.want {
				t.Errorf("the axis is\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
