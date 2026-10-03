package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// halfway is a session halfway through, 60% used, its readings rising from
// nothing at its start to 40% an hour in and 60% at two.
func halfway() (status.Account, Trail) {
	session := sessionOf(0.6, 150*time.Minute)
	start := session.ResetsAt.Add(-5 * time.Hour)
	trail := Trail{Start: start, Readings: []score.Reading{
		{At: start, Utilization: 0},
		{At: start.Add(time.Hour), Utilization: 0.4},
		{At: start.Add(2 * time.Hour), Utilization: 0.6},
	}}
	return readAccount("work", session, weekOf(0.3, 4*day)), trail
}

// chartOf draws the chart of doc's account a's window with the given key, of
// the history given, width cells wide and rows tall, and returns the canvas
// it's drawn on.
func chartOf(t *testing.T, doc status.Document, a status.Account, key string, history History, width, rows int) *canvas {
	t.Helper()
	f := Frame{Look: Screen(builtin(t, "nord")), Policy: claudeLike, History: history}
	w, ok := a.Window(key)
	if !ok {
		t.Fatalf("no window %s", key)
	}
	c := newCanvas(width, rows+1)
	b := f.burndownOf(doc, a, standingOf(doc, a, w, now, claudeLike), theme.AccentAttention, now)
	f.chart(c, b, 0, 0, width, rows)
	f.axis(c, b, 0, rows, width)
	return c
}

func TestAChartsLevelIsTheRoomItsReadingsLeftAndWhereItsHeading(t *testing.T) {
	a, trail := halfway()
	c := chartOf(t, routerDoc("", 0, a), a, "5h", History{{Account: "work", Window: "5h"}: trail}, 10, 2)

	want := []string{"██▂▂  │", "████▆▆⠂⠄✕"}
	if got := c.rows(Look{})[:2]; !slices.Equal(got, want) {
		t.Errorf("drew\n%s\nwant\n%s\n: the room each column's last reading left, now's line, and a dotted line to ✕ where it runs out", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, tt := range []struct {
		x, y int
		want ink
	}{
		{x: 0, y: 0, want: ink{token: theme.AccentAttention, fade: projectionFade}},
		{x: 6, y: 0, want: borderInk},
		{x: 6, y: 1, want: ink{token: theme.AccentAttention}},
		{x: 8, y: 1, want: outInk},
	} {
		if got := c.at(tt.x, tt.y).ink; got != tt.want {
			t.Errorf("column %d, row %d is in %+v, want %+v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestAChartHeadingForItsResetEndsAtTheRoomItLeaves(t *testing.T) {
	a := readAccount("work", sessionOf(0.2, 150*time.Minute), weekOf(0.3, 4*day))
	c := chartOf(t, routerDoc("", 0, a), a, "5h", nil, 10, 2)

	rows := c.rows(Look{})
	if got, want := rows[0], "▅▅▅▅▅▅⠄⠄⠄⡀"; got != want || strings.Contains(strings.Join(rows, ""), "✕") {
		t.Errorf("drew\n%s\nwant row 1 %q: the level, then dots down to the room left at its reset, 60%%, without ✕", strings.Join(rows, "\n"), want)
	}
}

func TestAChartsFloorIsItsReserveWhereThatHoldsItBack(t *testing.T) {
	a, trail := halfway()
	a.Reserve = 0.1
	c := chartOf(t, routerDoc("", 0, a), a, "5h", History{{Account: "work", Window: "5h"}: trail}, 10, 2)

	if got, want := c.rows(Look{})[1], "████▆▆⠂✕⠠"; got != want {
		t.Errorf("row 2 = %q, want %q: ✕ where it reaches its reserve, sooner, on its floor, and the floor dotted on", got, want)
	}
	if got, want := c.at(8, 1).ink, (ink{token: theme.VizReserve, fade: reserveFade}); got != want {
		t.Errorf("the reserve's floor is in %+v, want %+v", got, want)
	}
}

func TestAChartAtItsLimitRunsAlongTheFloorTillTheLimitLifts(t *testing.T) {
	session := sessionOf(1, 150*time.Minute)
	session.Status = quota.StatusRejected
	a := readAccount("work", session, weekOf(0.3, 4*day))
	a.Limit = status.Limit{Windows: []string{"5h"}, Until: session.ResetsAt.Add(-time.Hour)}
	start := session.ResetsAt.Add(-5 * time.Hour)
	trail := Trail{Start: start, Readings: []score.Reading{{At: start, Utilization: 0.5}, {At: start.Add(time.Hour), Utilization: 1}}}
	c := chartOf(t, routerDoc("", 0, a), a, "5h", History{{Account: "work", Window: "5h"}: trail}, 20, 2)

	if got, want := c.rows(Look{}), []string{"           │", "████▁▁▁▁▁▁▁▁▁▁▁▁", "10:42    now   15:42"}; !slices.Equal(got, want) {
		t.Errorf("drew\n%s\nwant\n%s\n: the level reaching the floor, and a line along it till the limit lifts", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := c.at(5, 1).ink; got != errorInk {
		t.Errorf("the line along the floor is in %+v, want destructive", got)
	}
}

func TestAChartWithoutHistoryIsTheRoomNowFromItsStart(t *testing.T) {
	a, _ := halfway()
	c := chartOf(t, routerDoc("", 0, a), a, "5h", nil, 30, 3)

	rows := c.rows(Look{})
	if got, want := rows[1], "▂▂▂▂▂▂▂ no history yet"; got != want || !strings.HasPrefix(rows[2], strings.Repeat("█", 16)+"│") {
		t.Errorf("drew\n%s\nwant row 2 %q: the room now, flat from its start, marked", strings.Join(rows, "\n"), want)
	}
	if got := c.at(0, 2).ink; got != faintLevelInk {
		t.Errorf("the level is in %+v, want dim", got)
	}
}

func TestALapsedWindowIsAFullLevelSayingWhenItStarts(t *testing.T) {
	a := readAccount("work", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.3, 4*day))
	a.Lapsed = []string{"5h"}
	doc := routerDoc("", 0, a)
	doc.Prime = status.Prime{Window: "5h", Slots: []status.Slot{{Account: "work", At: "14:20", Next: now.Add(68 * time.Minute).UTC()}}}
	c := chartOf(t, doc, a, "5h", nil, 44, 4)

	rows := c.rows(Look{})
	if want := strings.Repeat("█", 44); rows[0] != want || rows[3] != want {
		t.Errorf("drew\n%s\nwant a full level", strings.Join(rows, "\n"))
	}
	if got, want := rows[2], "█ full · window starts at its prime, 14:20 █"; got != want {
		t.Errorf("row 3 = %q, want %q", got, want)
	}
	if rows[4] != "" {
		t.Errorf("its axis reads %q, want none, the window not running", rows[4])
	}
}

func TestAChartsAxis(t *testing.T) {
	tests := []struct {
		name   string
		window quota.Window
		width  int
		want   string
	}{
		{name: "a session: its start, now and its reset", window: sessionOf(0.4, 150*time.Minute), width: 44, want: "10:42                now               15:42"},
		{name: "a session: now, where it would crowd its start, left off", window: sessionOf(0.1, 290*time.Minute), width: 44, want: "13:02                                  18:02"},
		{name: "a week: its days from their midnights", window: weekOf(0.3, 4*day+8*time.Hour+48*time.Minute), width: 44, want: " ╵Sat  ╵Sun  ╵Mon  ╵Tue  ╵Wed  ╵Thu  ╵Fri"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := readAccount("work", tt.window)
			c := chartOf(t, routerDoc("", 0, a), a, tt.window.Key, nil, tt.width, 1)
			if got := c.rows(Look{})[1]; got != tt.want {
				t.Errorf("the axis reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
	week := weekOf(0.3, 4*day+8*time.Hour+48*time.Minute)
	a := readAccount("work", week)
	c := chartOf(t, routerDoc("", 0, a), a, "7d", nil, 44, 1)
	today := strings.Index(c.rows(Look{})[1], "Mon")
	if got := c.at(len([]rune(c.rows(Look{})[1][:today])), 1).ink; got != strongInk {
		t.Errorf("today's name is in %+v, want it picked out", got)
	}
}
