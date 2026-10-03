package dashboard

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// burning is a session 45 minutes in, 10% used: 2% in its first 10 minutes,
// 6% in its next, none for the 20 after, and 2% in the 5 minutes since; and
// its readings, a chart's column a 10-minute stretch of it at 30 columns.
func burning() (status.Account, History) {
	session := sessionOf(0.1, 4*time.Hour+15*time.Minute)
	start := session.ResetsAt.Add(-5 * time.Hour)
	trail := Trail{Start: start, Readings: []score.Reading{
		{At: start, Utilization: 0},
		{At: start.Add(10 * time.Minute), Utilization: 0.02},
		{At: start.Add(20 * time.Minute), Utilization: 0.08},
		{At: start.Add(42 * time.Minute), Utilization: 0.1},
	}}
	return readAccount("work", session, weekOf(0.3, 4*day)), History{{Account: "work", Window: "5h"}: trail}
}

func TestABurnRatesBarsAreItsUsePer10MinutesAgainstTheFastestThatLasts(t *testing.T) {
	a, history := burning()
	c := charting{style: BurnRate, doc: routerDoc("", 0, a), account: a, key: "5h", history: history, condition: status.Open, width: 30, rows: 2}.draw(t)

	want := []string{
		"⡀█⡀⡀⡀│" + strings.Repeat("⡀", 24),
		"▅█  ▅│",
		"12:27                    17:27",
	}
	if got := c.rows(Look{}); !slices.Equal(got, want) {
		t.Errorf("drew\n%s\nwant\n%s\n: a bar a 10 minutes, the 6%% ten, and the 2%% five since now, on the scale of the tallest; the line for now; and a dotted line where it's clear of them, at the 3.5%% a 10 minutes that lasts to its reset", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, tt := range []struct {
		x, y int
		want ink
		why  string
	}{
		{x: 0, y: 1, want: ink{token: theme.StatePositive, fade: projectionFade}, why: "a bar slower than lasts, in the state's colour faded halfway"},
		{x: 1, y: 0, want: fastInk, why: "a bar faster than lasts, in attention"},
		{x: 4, y: 1, want: ink{token: theme.StatePositive, fade: projectionFade}, why: "the bar of the stretch under way, its use so far spread over it"},
		{x: 5, y: 0, want: borderInk, why: "the line for now"},
		{x: 2, y: 0, want: lastingInk, why: "the dotted line, the fastest that lasts"},
	} {
		if got := c.at(tt.x, tt.y).ink; got != tt.want {
			t.Errorf("column %d, row %d is in %+v, want %+v: %s", tt.x, tt.y, got, tt.want, tt.why)
		}
	}
}

func TestABurnRatesLineIsWhereItsAccountRunsOut(t *testing.T) {
	a, history := burning()
	a.Reserve = 0.5
	p := charting{doc: routerDoc("", 0, a), account: a, key: "5h"}.plot(t, history)

	got, ok := p.lasting()
	if want := 0.4 * float64(rateSpan) / float64(4*time.Hour+15*time.Minute); !ok || math.Abs(got-want) > score.Tolerance {
		t.Errorf("lasting() = %v, %v; want %v: what's left short of its reserve, as that holds it back, over the time to its reset", got, ok, want)
	}
}

func TestABurnRateWithoutHistorySpreadsItsUseSinceItStartedEvenly(t *testing.T) {
	a := readAccount("work", sessionOf(0.72, 2*time.Hour), weekOf(0.3, 4*day))
	c := charting{style: BurnRate, doc: routerDoc("", 0, a), account: a, key: "5h", condition: status.Open, width: 30, rows: 2}.draw(t)

	want := []string{
		strings.Repeat("█", 19) + "│" + strings.Repeat("⡀", 10),
		"█ no history yet ██│",
		"10:12            now     15:12",
	}
	if got := c.rows(Look{}); !slices.Equal(got, want) {
		t.Errorf("drew\n%s\nwant\n%s\n: its use since it started, 4%% a 10 minutes, spread evenly to now, marked, and the 2.3%% that lasts dotted beyond", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := c.at(0, 0).ink; got != faintLevelInk {
		t.Errorf("its bars are in %+v, want dim, even faster than lasts: spread evenly, they say nothing of when", got)
	}
}

func TestABurnRateHeldAtItsLimitRunsAlongTheFloorFromWhenItWasReached(t *testing.T) {
	session := sessionOf(1, time.Hour)
	session.Status = quota.StatusRejected
	a := readAccount("personal", session, weekOf(0.3, 4*day))
	a.Limit = status.Limit{Windows: []string{"5h"}, Until: now.Add(30 * time.Minute).UTC()}
	doc := routerDoc("", 0, a)
	doc.Events = []status.Event{{ID: 1, At: now.Add(-time.Hour).UTC(), Kind: status.EventLimit, Account: "personal"}}
	start := session.ResetsAt.Add(-5 * time.Hour)
	history := History{{Account: "personal", Window: "5h"}: {Start: start, Readings: []score.Reading{
		{At: start, Utilization: 0},
		{At: start.Add(time.Hour), Utilization: 0.5},
		{At: start.Add(3 * time.Hour), Utilization: 1},
	}}}
	c := charting{style: BurnRate, doc: doc, account: a, key: "5h", history: history, condition: status.Limited, width: 30, rows: 1}.draw(t)

	if got, want := c.rows(Look{})[0], "     █           █▁▁▁▁▁▁▁│▁"; got != want {
		t.Errorf("drew %q, want %q: its bars till it reached its limit, then a line along the floor till the limit lifts, and no dotted line, nothing lasting", got, want)
	}
	if got := c.at(20, 0).ink; got != errorInk {
		t.Errorf("the line along the floor is in %+v, want destructive", got)
	}
}

func TestALapsedWindowsBurnRateSaysWhenItStarts(t *testing.T) {
	a := readAccount("work", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.3, 4*day))
	a.Lapsed = []string{"5h"}
	doc := routerDoc("", 0, a)
	doc.Prime = status.Prime{Window: "5h", Slots: []status.Slot{{Account: "work", At: "14:20", Next: now.Add(68 * time.Minute).UTC()}}}
	c := charting{style: BurnRate, doc: doc, account: a, key: "5h", condition: status.Idle, width: 60, rows: 3}.draw(t)

	rows := c.rows(Look{})
	if got, want := strings.TrimSpace(rows[1]), "nothing used · window starts at its prime, 14:20"; got != want {
		t.Errorf("row 2 = %q, want %q", got, want)
	}
	if rows[0] != "" || rows[2] != "" || rows[3] != "" {
		t.Errorf("drew\n%s\nwant nothing else, not even its axis, the window not running", strings.Join(rows, "\n"))
	}
}

func TestAWeeksBurnRateMeasuresEachColumnsStretch(t *testing.T) {
	week := weekOf(0.35, 3*day+12*time.Hour)
	start := week.ResetsAt.Add(-7 * day)
	a := readAccount("work", sessionOf(0.1, time.Hour), week)
	history := History{{Account: "work", Window: "7d"}: {Start: start, Readings: []score.Reading{
		{At: start, Utilization: 0},
		{At: start.Add(day), Utilization: 0.14},
		{At: start.Add(2 * day), Utilization: 0.28},
		{At: start.Add(3 * day), Utilization: 0.35},
	}}}
	p := charting{doc: routerDoc("", 0, a), account: a, key: "7d"}.plot(t, history)

	perTen := float64(day / rateSpan)
	for col, want := range []float64{0.14 / perTen, 0.14 / perTen, 0.07 / perTen, 0} {
		if got, ok := p.rateAt(col, 7); !ok || math.Abs(got-want) > score.Tolerance {
			t.Errorf("day %d's bar is %v, %v; want %v, its use spread over the day, per 10 minutes", col+1, got, ok, want)
		}
	}
}

func TestTheBarOfTheColumnNowFallsInIsTheStretchUnderWay(t *testing.T) {
	// 49.5 minutes into a session drawn 44 columns wide, a column under 7
	// minutes, the column now falls in has its middle in the stretch from 50
	// minutes, still to come.
	session := sessionOf(0.08, 5*time.Hour-49*time.Minute-30*time.Second)
	start := session.ResetsAt.Add(-5 * time.Hour)
	a := readAccount("work", session, weekOf(0.3, 4*day))
	history := History{{Account: "work", Window: "5h"}: {Start: start, Readings: []score.Reading{
		{At: start, Utilization: 0},
		{At: start.Add(40 * time.Minute), Utilization: 0.05},
		{At: start.Add(45 * time.Minute), Utilization: 0.08},
	}}}
	p := charting{doc: routerDoc("", 0, a), account: a, key: "5h"}.plot(t, history)

	col := p.nowColumn(44) - 1
	if got, ok := p.rateAt(col, 44); !ok || math.Abs(got-0.03) > score.Tolerance {
		t.Errorf("the bar of column %d, which now falls in, is %v, %v; want 0.03, the use of the stretch from 40 minutes, under way", col, got, ok)
	}
}

// plot is the chart of the window, as plotOf has it at now, of the history
// given.
func (ch charting) plot(t *testing.T, history History) plot {
	t.Helper()
	w, ok := ch.account.Window(ch.key)
	if !ok {
		t.Fatalf("no window %s", ch.key)
	}
	f := Frame{Policy: claudeLike, History: history}
	fc := face{account: ch.account, state: status.State{Condition: status.Open}, featured: standingOf(ch.doc, ch.account, w, now, claudeLike), busy: []bool{ch.busy}}
	return f.plotOf(ch.doc, fc, now)
}
