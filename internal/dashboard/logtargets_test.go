package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// overMidnight is a page of Requests of a line the day before and two of
// today's, a day's rule between, at noon.
func overMidnight() LogPage {
	src := views.LogSources{From: theDayBefore, Now: logDayNoon}
	return requestsOf(src, []ledger.Line{asked("0", theDayBefore), asked("1", logDay), asked("2", logDay.Add(time.Minute))})
}

// logTargetsOf are the targets of the Log's page p, at 160×24, of the kind
// given.
func logTargetsOf(p LogPage, on Clicked) []Target {
	var targets []Target
	for _, t := range logFrameOf(p, 160, 24).Targets(routerDoc("side", 0), logDayNoon) {
		if t.On == on {
			targets = append(targets, t)
		}
	}
	return targets
}

// Each row of the table in view is a click's target across the width, on
// the row it's drawn on, by its place among the table's lines: a day's rule
// is none.
func TestEachRowOfTheLogsTableInViewIsATarget(t *testing.T) {
	p := overMidnight()
	f := logFrameOf(p, 160, 24)
	lines := p.lines()
	var rows []int
	for _, target := range logTargetsOf(p, OnRow) {
		rows = append(rows, target.Row)
		if !lines[target.Row].choosable() {
			t.Errorf("line %d is a target, but no row", target.Row)
		}
		if want := f.top() + 1 + target.Row; target.Y != want || target.X != 0 || target.Width != 160 {
			t.Errorf("line %d is a target along row %d from %d, %d wide; want row %d, from 0, across the 160", target.Row, target.Y, target.X, target.Width, want)
		}
	}
	if want := []int{0, 2, 3}; !slices.Equal(rows, want) {
		t.Errorf("the lines %v are targets, want %v, the rows but the day's rule", rows, want)
	}
}

// While the state line counts lines come since a pause, "▼ 1 new" is a
// target over the cells it's drawn on, as end, which follows again; not
// while the find's line takes the state line's place, nor following.
func TestTheLinesComeSinceAreATargetThatFollows(t *testing.T) {
	src := views.LogSources{From: logDay, Now: logDayNoon}
	lines := []ledger.Line{asked("1", logDay), asked("2", logDay.Add(time.Minute))}
	paused := requestsOf(src, append(lines, asked("3", logDay.Add(2*time.Minute)))).withStands(requestsOf(src, lines).Chose(-1, 10))
	finding := paused
	finding.Finding = LogFinding{Open: true}
	got := logTargetsOf(paused, OnKey)
	i := slices.IndexFunc(got, func(t Target) bool { return t.Y == 22 })
	if i < 0 {
		t.Fatalf("paused, a line come since, the state line's targets are %v, want ▼ 1 new", got)
	}
	if target := got[i]; target.Key != "end" || target.X != 23 || target.Width != 7 {
		t.Errorf("the state line's target is %+v, want end, from column 23, 7 cells wide", target)
	}
	text := textOf(logFrameOf(paused, 160, 24).Draw(routerDoc("side", 0), logDayNoon))
	if drawn := string([]rune(text[22])[23:30]); drawn != "▼ 1 new" {
		t.Errorf("the target's cells read %q, want ▼ 1 new", drawn)
	}
	for name, p := range map[string]LogPage{"finding": finding, "following": paused.Followed()} {
		if slices.ContainsFunc(logTargetsOf(p, OnKey), func(t Target) bool { return t.Y == 22 }) {
			t.Errorf("%s, the state line's row has a target", name)
		}
	}
}

// A click on a row chooses it, pausing the table where it follows; the row
// chosen is lit, a click on it opening its details. A click on a line that's
// no row chooses nothing.
func TestAClickChoosesARowOfTheLogsTable(t *testing.T) {
	p := overMidnight()
	chosen := p.ChoseLine(2, 10)
	if row, _, ok := chosen.Chosen(); !ok || row.Request != "1" || !chosen.Paused() {
		t.Errorf("a click on today's first row chose %q, paused %t; want it chosen, the table paused", row.Request, chosen.Paused())
	}
	if !chosen.Lit(2) || chosen.Lit(3) || p.Lit(2) {
		t.Error("Lit says otherwise than which line is chosen")
	}
	if again := chosen.ChoseLine(3, 10); again.Lit(2) || !again.Lit(3) {
		t.Error("paused, a click on another row doesn't choose it")
	}
	for _, at := range []int{1, -1, 4} {
		if got := p.ChoseLine(at, 10); got.Paused() {
			t.Errorf("a click on line %d, no row, paused the table", at)
		}
	}
}

// A click on a sub-tab shows it, each table standing as it stood.
func TestAClickOnTheLogsSubTabShowsIt(t *testing.T) {
	p := overMidnight().Chose(-1, 10)
	events := p.Showing(LogEvents)
	if events.Shown != LogEvents || events.Paused() {
		t.Errorf("shown Events, the page shows %d, paused %t; want Events, following as they stood", events.Shown, events.Paused())
	}
	if back := events.Showing(LogRequests); back.Shown != LogRequests || !back.Paused() {
		t.Error("shown Requests again, they don't stand paused as they stood")
	}
	if none := p.Showing(LogView(2)); none.Shown != LogRequests {
		t.Errorf("shown a sub-tab of none, the page shows %d", none.Shown)
	}
}
