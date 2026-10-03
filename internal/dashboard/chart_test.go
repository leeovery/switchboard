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

// chartOf draws the burn-down of doc's account a's window with the given
// key, of the history given, width cells wide and rows tall, in attention's
// colour, as an account under pressure has it, and returns the canvas it's
// drawn on.
func chartOf(t *testing.T, doc status.Document, a status.Account, key string, history History, width, rows int) *canvas {
	t.Helper()
	return charting{doc: doc, account: a, key: key, history: history, condition: status.Pressed, width: width, rows: rows}.draw(t)
}

// charting is a chart drawn for a test: in its style, of doc's account's
// window with the given key, of the history given, its account in the
// condition given, and busy or not; at its moment, now where it's zero,
// width cells wide and rows tall, in nord, or in the look given, its axis
// under it.
type charting struct {
	style       Chart
	doc         status.Document
	account     status.Account
	key         string
	history     History
	condition   status.Condition
	busy        bool
	at          time.Time
	width, rows int
	look        *Look
}

// draw draws the chart, and returns the canvas it's drawn on.
func (ch charting) draw(t *testing.T) *canvas {
	t.Helper()
	at := ch.at
	if at.IsZero() {
		at = now
	}
	look := Screen(builtin(t, "nord"))
	if ch.look != nil {
		look = *ch.look
	}
	f := Frame{Look: look, Policy: claudeLike, History: ch.history, Chart: ch.style}
	w, ok := ch.account.Window(ch.key)
	if !ok {
		t.Fatalf("no window %s", ch.key)
	}
	c := newCanvas(ch.width, ch.rows+1)
	fc := face{account: ch.account, state: status.State{Condition: ch.condition}, featured: standingOf(ch.doc, ch.account, w, at, claudeLike), busy: []bool{ch.busy}}
	p := f.plotOf(ch.doc, fc, at)
	f.chart(c, p, 0, 0, ch.width, ch.rows)
	f.axis(c, p, 0, ch.rows, ch.width)
	return c
}

func TestGCyclesTheChartStyleRound(t *testing.T) {
	tests := []struct {
		from Chart
		name string
		next Chart
	}{
		{from: Burndown, name: "burn-down", next: BurnRate},
		{from: BurnRate, name: "burn rate", next: Hourglass},
		{from: Hourglass, name: "hourglass", next: Burndown},
		{from: "heartbeat", name: "burn-down", next: BurnRate},
	}
	for _, tt := range tests {
		if got := tt.from.Next(); got != tt.next {
			t.Errorf("g moves on from %q to %q, want %q", tt.from, got, tt.next)
		}
		if got := tt.from.Name(); got != tt.name {
			t.Errorf("%q is called %q, want %q", tt.from, got, tt.name)
		}
	}
}

func TestAChartStyleGDoesntReachIsDrawnAsBurnDown(t *testing.T) {
	a, trail := halfway()
	history := History{{Account: "work", Window: "5h"}: trail}
	drawn := func(style Chart) []string {
		return charting{style: style, doc: routerDoc("", 0, a), account: a, key: "5h", history: history, condition: status.Pressed, width: 30, rows: 3}.draw(t).rows(Look{})
	}

	if got, want := drawn("heartbeat"), drawn(Burndown); !slices.Equal(got, want) {
		t.Errorf("a style kept by a later switchboard drew\n%s\nwant burn-down's\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
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
	// Its session reads short of spent, so the limit alone holds it back.
	session := sessionOf(0.97, 150*time.Minute)
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

func TestAChartsLevelShowsALittleRoomLeftAsItsLowestEighth(t *testing.T) {
	session := sessionOf(0.98, 150*time.Minute)
	start := session.ResetsAt.Add(-5 * time.Hour)
	a := readAccount("work", session, weekOf(0.3, 4*day))
	trail := Trail{Start: start, Readings: []score.Reading{{At: start, Utilization: 0.5}, {At: start.Add(time.Hour), Utilization: 0.98}}}
	c := chartOf(t, routerDoc("", 0, a), a, "5h", History{{Account: "work", Window: "5h"}: trail}, 20, 2)

	if got := c.at(6, 1); got.glyph != "▁" || got.ink != (ink{token: theme.AccentAttention, fade: projectionFade}) {
		t.Errorf("a column with 2%% left draws %q in %+v, want ▁ in the level's ink: no limit holds it", got.glyph, got.ink)
	}
}

func TestAChartWithoutHistoryIsTheRoomNowFromItsStart(t *testing.T) {
	a, _ := halfway()
	tests := []struct {
		name  string
		width int
		want  []string
	}{
		{
			name: "marked across its level, clear of its marks", width: 40,
			want: []string{
				"                     │",
				"▂▂ no history yet ▂▂▂⡀⡀",
				"█████████████████████│ ⠁⠁⠁⠂⠂⠂⠄⠄⠄⡀✕",
				"10:42              now             15:42",
			},
		},
		{
			name: "its level too short for the words, marked across it all, over its marks", width: 14,
			want: []string{
				"        │",
				" no history…",
				"████████⠁⠂⠄✕",
				"10:42    15:42",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := chartOf(t, routerDoc("", 0, a), a, "5h", nil, tt.width, 3)
			if got := c.rows(Look{}); !slices.Equal(got, tt.want) {
				t.Errorf("drew\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			if got := c.at(0, 2).ink; got != faintLevelInk {
				t.Errorf("the level is in %+v, want dim", got)
			}
		})
	}
}

func TestALimitNamingNoWindowHoldsTheChartOnTheFloorFromWhenItWasReached(t *testing.T) {
	a, trail := halfway()
	reached := now.Add(-time.Hour)
	a.Limit = status.Limit{Until: now.Add(time.Hour)}
	doc := routerDoc("", 0, a)
	doc.Events = []status.Event{{ID: 1, At: reached.UTC(), Kind: status.EventLimit, Account: "work"}}
	c := chartOf(t, doc, a, "5h", History{{Account: "work", Window: "5h"}: trail}, 20, 2)

	if got, want := c.rows(Look{}), []string{"████▂▂     │", "██████▁▁▁▁▁▁▁▁", "10:42    now   15:42"}; !slices.Equal(got, want) {
		t.Errorf("drew\n%s\nwant\n%s\n: the level on the floor from the limit, and a line along it till the limit lifts", strings.Join(got, "\n"), strings.Join(want, "\n"))
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
		{name: "a week: its days from their midnights, each in the chart's column for its time", window: weekOf(0.3, 4*day+8*time.Hour+48*time.Minute), width: 44, want: "╵Sat  ╵Sun   ╵Mon  ╵Tue  ╵Wed  ╵Thu   ╵Fri"},
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
