package watch

import (
	"errors"
	"fmt"
	"iter"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// asksOfThree are the histories a full read asks of three accounts' windows:
// the session at five minutes, and the week at half an hour.
var asksOfThree = []string{"5h 5m0s", "7d 30m0s"}

func TestReadsTheHistoryWithEachFullReadNeverALook(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	if !slices.Equal(h.source.histories, asksOfThree) {
		t.Fatalf("the first read asked for the history of %q, want %q", h.source.histories, asksOfThree)
	}
	h.tickUntil(at(13, 13, 0))
	if got := len(h.source.histories); got != 2 {
		t.Errorf("a minute of looks asked for %d histories more, want none", got-2)
	}
	h.deliver(h.press("R")...)
	if got := len(h.source.histories); got != 4 {
		t.Errorf("r asked for %d histories, want both again", got-2)
	}
	h.tickUntil(at(13, 43, 10))
	if got := len(h.source.histories); got != 6 {
		t.Errorf("the next full read asked for %d histories, want both again", got-4)
	}
}

func TestReadsTheHistoryAsTheRouterAnswersAgain(t *testing.T) {
	tests := []struct {
		name string
		// then has the router go and come back, or another router answer.
		then func(h *harness)
	}{
		{name: "once it answers a look again", then: func(h *harness) {
			h.stopRouter()
			h.tickUntilAsked()
			h.startRouter(routerDocument(three()...))
			h.tickUntilAsked()
		}},
		{name: "once it answers after probing", then: func(h *harness) {
			h.stopRouter()
			h.deliver(h.press("R")...)
			h.startRouter(routerDocument(three()...))
			h.tickUntil(at(13, 13, 0).Add(tickSlack))
		}},
		{name: "once another router answers", then: func(h *harness) {
			h.source.health = router.Health{PID: 2}
			h.tickUntilAsked()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.source.health = router.Health{PID: 1}
			h.start()

			tt.then(h)
			if got := len(h.source.histories); got != 4 {
				t.Errorf("asked for %d histories since the first read, want both again", got-2)
			}
		})
	}
}

func TestARouterFromBeforeGETHistoryHasNone(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.source.historyErr = fmt.Errorf("%w: the router answered GET /history with 404 Not Found", router.ErrNoHistory)
	h.start()

	if want := []string{"5h 5m0s"}; !slices.Equal(h.source.histories, want) {
		t.Errorf("asked for the history of %q, want %q: nothing more once the router can't be asked", h.source.histories, want)
	}
	if !strings.Contains(h.view(), "restart the router for recent events") {
		t.Errorf("the screen is\n%s\nwant LATELY to ask for the router to be restarted", h.view())
	}
	h.deliver(h.press("R")...)
	if n := strings.Count(log.String(), "the router is from before GET /history"); n != 1 {
		t.Errorf("log reads\n%s\nwant the router from before GET /history noted once", log)
	}

	h.source.historyErr = nil
	h.source.health = router.Health{PID: 9}
	h.tickUntilAsked()
	if strings.Contains(h.view(), "restart the router") {
		t.Errorf("once a router with GET /history answers, the screen is\n%s\nwant RECENT to ask for nothing", h.view())
	}
}

func TestAHistoryThatCantBeReadKeepsTheLast(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.source.history = map[string]router.History{"5h": historyOf("work", start.Add(-time.Hour), 0.1, 0.2)}
	h.start()

	h.source.historyErr = fmt.Errorf("ask the router: %w", errTimedOut)
	h.deliver(h.press("R")...)
	if got := len(h.model.trails[dashboard.Ref{Account: "work", Window: "5h"}].Readings); got < 2 {
		t.Errorf("work's session has %d readings, want the router's last history of it kept", got)
	}
	if want := []string{"level=WARN", `msg="couldn't read the router's history"`}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
}

func TestAnAnswerALaterAskingOvertookIsDropped(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.source.history = map[string]router.History{"5h": historyOf("work", start.Add(-time.Hour), 0.1, 0.2)}
	var asking tea.Cmd
	h.model, asking = h.model.askHistory(h.model.doc.At(start, policy))
	stale := asking()

	h.deliver(h.press("R")...)
	h.deliver(stale)
	if got := h.model.history.asked; got != stale.(historyMsg).asked+1 {
		t.Fatalf("asked %d times, want the stale answer's asking overtaken", got)
	}
	if len(h.model.history.router["5h"].Accounts) == 0 {
		t.Error("the stale answer replaced the later one's history")
	}
}

func TestTheChartsDrawTheRoutersHistoryCarriedOnByWhatTheWatchSees(t *testing.T) {
	sessionStart := start.Add(-2 * time.Hour)
	work := account("work", "Work", session(0.25, 3*time.Hour), week(0.5))
	h := routedHarness(t, routerDocument(work))
	h.source.history = map[string]router.History{"5h": historyOf("work", sessionStart, 0.1, 0.2)}
	h.start()

	read := work
	read.Windows = []quota.Window{session(0.3, 3*time.Hour), week(0.5)}
	read.FetchedAt = start.Add(time.Minute).UTC()
	h.startRouter(routerDocument(read))
	h.tickUntil(at(13, 13, 10))

	want := []score.Reading{
		{At: sessionStart.UTC(), Utilization: 0.1},
		{At: sessionStart.Add(5 * time.Minute).UTC(), Utilization: 0.2},
		{At: start.UTC(), Utilization: 0.25},
		{At: start.Add(time.Minute).UTC(), Utilization: 0.3},
	}
	trail := h.model.trails[dashboard.Ref{Account: "work", Window: "5h"}]
	if !slices.EqualFunc(trail.Readings, want, sameReading) {
		t.Errorf("work's session's trail is %+v, want %+v", trail, want)
	}
	if week := h.model.trails[dashboard.Ref{Account: "work", Window: "7d"}]; len(week.Readings) != 1 {
		t.Errorf("work's week's trail is %+v, want the reading the watch saw, the router giving none", week)
	}
}

func TestTheChartsDrawNoReadingTheRoutersHistoryCoversAlready(t *testing.T) {
	sessionStart := start.Add(-time.Hour)
	ref := dashboard.Ref{Account: "work", Window: "5h"}
	h := history{
		router: map[string]router.History{"5h": historyOf("work", sessionStart, 0.1, 0.2, 0.2)},
		seen: map[dashboard.Ref][]score.Reading{ref: {
			{At: sessionStart.Add(-time.Hour), Utilization: 0.9},
			{At: sessionStart.Add(4 * time.Minute), Utilization: 0.2, Last: sessionStart.Add(30 * time.Minute)},
			{At: sessionStart.Add(40 * time.Minute), Utilization: 0.3},
		}},
	}
	doc := document(account("work", "Work", session(0.3, 4*time.Hour)))

	want := []score.Reading{
		{At: sessionStart.UTC(), Utilization: 0.1},
		{At: sessionStart.Add(5 * time.Minute).UTC(), Utilization: 0.2},
		{At: sessionStart.Add(10 * time.Minute).UTC(), Utilization: 0.2},
		{At: sessionStart.Add(40 * time.Minute), Utilization: 0.3},
	}
	if got := h.drawn(doc.At(start, policy))[ref].Readings; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("the trail's readings are %+v, want %+v: in order, none from before the session, nor the router's last", got, want)
	}
}

// The charts draw the windows running at now: a session read running, on the
// tick after it resets, before it's read again, has lapsed, and draws no
// trail, while its week, still running, does.
func TestTheChartsDrawNoSessionThatHasLapsedSinceItWasRead(t *testing.T) {
	sessionStart := start.Add(-time.Hour)
	h := history{router: map[string]router.History{
		"5h": historyOf("work", sessionStart, 0.1, 0.2),
		"7d": historyOf("work", sessionStart, 0.4, 0.5),
	}}
	doc := document(account("work", "Work", session(0.2, 4*time.Hour), week(0.5)))

	drawn := h.drawn(doc.At(start.Add(4*time.Hour+time.Second), policy))
	if trail, ok := drawn[dashboard.Ref{Account: "work", Window: "5h"}]; ok {
		t.Errorf("work's session draws %+v, want no trail, as it has lapsed", trail)
	}
	if _, ok := drawn[dashboard.Ref{Account: "work", Window: "7d"}]; !ok {
		t.Error("work's week draws no trail, want the router's, as it runs")
	}
}

// heldSinceItReset is work, whose session, read at 0.97 a minute before it
// reset at reset, the router's limit holds an hour past it.
func heldSinceItReset(reset time.Time) status.Account {
	return status.Account{
		ID: "work", Label: "Work", TokenSet: true, FetchedAt: reset.Add(-time.Minute).UTC(),
		Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.97, ResetsAt: reset.UTC()}},
		Limit:   status.Limit{Windows: []string{"5h"}, Until: reset.Add(time.Hour).UTC()},
	}
}

// GRAPHS charts a window as it stands at now, so a session the router's limit
// holds, reset since it was read, has lapsed, as the router's next document
// has it: its chart is its name alone, with no axis and no history, however
// the router's history ran before.
func TestGRAPHSChartsAHeldSessionResetSinceItWasReadAsNotStarted(t *testing.T) {
	h := routedHarness(t, routerDocument(heldSinceItReset(at(13, 0, 0))))
	h.source.history = map[string]router.History{"5h": historyOf("work", at(8, 0, 0), 0.1, 0.2, 0.97)}
	h.start()
	h.deliver(h.press("g")...)

	screen := h.view()
	if !strings.Contains(screen, "5-HOUR ┄") || regexp.MustCompile(`(?m)^ {14}\d\d:\d\d +\d\d:\d\d$`).MatchString(screen) {
		t.Errorf("the screen is\n%s\nwant GRAPHS to chart work's session by its name alone, with no axis, as it has lapsed", screen)
	}
	if !strings.Contains(screen, "no history yet") {
		t.Errorf("the screen is\n%s\nwant work's session charted with no history yet", screen)
	}
}

// The readings history's files are read from the day the earliest span the
// charts draw started: a week reset since it was read is charted from its
// reset, so the files of the days before, when it started before it reset,
// aren't read.
func TestTheReadingsHistorysFilesAreReadFromTheDayAWeekResetSinceItWasReadStartedAgain(t *testing.T) {
	reset := time.Date(2026, 10, 14, 2, 0, 0, 0, time.Local)
	work := status.Account{
		ID: "work", Label: "Work", TokenSet: true, FetchedAt: reset.Add(-time.Minute).UTC(),
		Windows: []quota.Window{{Key: "7d", Label: "Week", Utilization: 0.6, ResetsAt: reset.UTC()}},
	}
	doc := status.Document{GeneratedAt: work.FetchedAt, Source: status.SourceRouter, Accounts: []status.Account{work}}
	files := &filedReadings{held: []readings.Reading{filedOn("work", "7d", reset.Add(-4*time.Hour), 0.5)}}

	got := filedOf(files, doc.At(reset.Add(time.Second), policy))
	if day := time.Date(2026, 10, 14, 0, 0, 0, 0, time.Local); !files.from.Equal(day) {
		t.Errorf("the files are read from %v, want from %v, the day the week started again as it reset, not the days before", files.from, day)
	}
	if len(got) != 0 {
		t.Errorf("the files gave %+v, want nothing of the week from before it reset", got)
	}
}

func TestProbingTheChartsDrawTheReadingsTheWatchSees(t *testing.T) {
	read := func(used float64, at time.Time, resetsIn time.Duration) status.Document {
		a := account("work", "Work", session(used, resetsIn), week(0.5))
		a.FetchedAt = at.UTC()
		doc := probedWithoutTheRouter()
		doc.Accounts = []status.Account{a}
		return doc
	}
	h := newHarness(t, read(0.1, start, 3*time.Hour))
	h.start()

	h.clock.now = at(13, 20, 0)
	h.read(read(0.1, at(13, 20, 0), 3*time.Hour))
	h.clock.now = at(13, 30, 0)
	h.read(read(0.2, at(13, 30, 0), 3*time.Hour))
	ref := dashboard.Ref{Account: "work", Window: "5h"}
	want := []score.Reading{
		{At: start.UTC(), Utilization: 0.1, Last: at(13, 20, 0).UTC()},
		{At: at(13, 30, 0).UTC(), Utilization: 0.2},
	}
	if got := h.model.trails[ref].Readings; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("work's session's readings are %+v, want %+v: a reading of the same use carrying the last on", got, want)
	}
	if len(h.source.histories) > 0 {
		t.Errorf("asked for the history of %q, probing, want none", h.source.histories)
	}

	h.clock.now = at(16, 20, 0)
	h.read(read(0.05, at(16, 20, 0), 8*time.Hour))
	if got := h.model.trails[ref].Readings; len(got) != 1 || got[0].Utilization != 0.05 {
		t.Errorf("once the session started again, its readings are %+v, want those since alone", got)
	}
}

// filedReadings are a readings history's files as the watch reads them,
// holding the readings given; reads counts the times they're read between
// two times, and from is where the last such read started; starts are the
// starts of the tails made, in turn, and tails counts their reads.
type filedReadings struct {
	held   []readings.Reading
	reads  int
	from   time.Time
	starts []time.Time
	tails  int
}

// Tail gives a read of the readings held from start up to end, each window's
// last from from up to start carried in, as readings.Carried has them.
func (f *filedReadings) Tail(from, start, end time.Time) func() []readings.Reading {
	f.starts = append(f.starts, start)
	return func() []readings.Reading {
		f.tails++
		return readings.Carried(slices.Values(f.held), from, start, end)
	}
}

// Between gives the readings held of times from from up to to.
func (f *filedReadings) Between(from, to time.Time) iter.Seq[readings.Reading] {
	f.reads++
	f.from = from
	return func(yield func(readings.Reading) bool) {
		for _, r := range f.held {
			if !r.At.Before(from) && r.At.Before(to) && !yield(r) {
				return
			}
		}
	}
}

// filedOn is a line of the readings history of account id's window key,
// used as given at at.
func filedOn(id, key string, at time.Time, used float64) readings.Reading {
	return readings.Reading{At: at.UTC(), Account: id, Key: key, Utilization: used, Source: readings.FromProbe}
}

func TestProbingTheChartsDrawTheReadingsHistorysFilesReadAsProbingBegins(t *testing.T) {
	sessionStart := start.Add(-2 * time.Hour)
	work := account("work", "Work", session(0.3, 3*time.Hour), week(0.5))
	doc := probedWithoutTheRouter()
	doc.Accounts = []status.Account{work}
	h := newHarness(t, doc)
	files := &filedReadings{held: []readings.Reading{
		filedOn("work", "5h", sessionStart.Add(-time.Hour), 0.9),
		filedOn("work", "5h", sessionStart.Add(30*time.Minute), 0.1),
		filedOn("work", "5h", sessionStart.Add(90*time.Minute), 0.2),
	}}
	h.model.cfg.Readings = files
	h.start()
	h.deliver(h.press("R")...)

	want := []score.Reading{
		{At: sessionStart.Add(30 * time.Minute).UTC(), Utilization: 0.1},
		{At: sessionStart.Add(90 * time.Minute).UTC(), Utilization: 0.2},
		{At: start.UTC(), Utilization: 0.3},
	}
	if got := h.model.trails[dashboard.Ref{Account: "work", Window: "5h"}].Readings; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("work's session's readings are %+v, want %+v: the files' since the session started, then the watch's own", got, want)
	}
	if len(h.source.histories) > 0 {
		t.Errorf("asked the router for %q, probing, want nothing", h.source.histories)
	}
	h.tickUntil(at(13, 13, 0))
	h.deliver(h.press("R")...)
	h.deliver(h.press("R")...)
	if files.reads != 1 {
		t.Errorf("a minute of looks and several full reads, probing, read the files %d times, want once, as probing began", files.reads)
	}
}

func TestProbingWithNoReadingsHistoryReadsNoFiles(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.model.cfg.Readings = nil
	msgs := h.init()
	for len(msgs) > 0 {
		if _, filed := msgs[0].(filedMsg); filed {
			t.Errorf("with no readings history, the watch read its files, want nothing asked")
		}
		msgs = append(msgs[1:], h.update(msgs[0])...)
	}
	if h.source.reads != 1 || len(h.model.doc.Accounts) != 3 {
		t.Errorf("probed %d times, the screen showing %d accounts; want the first read, of three", h.source.reads, len(h.model.doc.Accounts))
	}
}

func TestTheChartsAskTheRouterAndReadTheFilesOnceProbingBeginsAgain(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	files := &filedReadings{held: []readings.Reading{filedOn("work", "5h", start.Add(-time.Hour), 0.9)}}
	h.model.cfg.Readings = files
	h.start()
	h.deliver(h.press("R")...)
	if want := slices.Concat(asksOfThree, asksOfThree); !slices.Equal(h.source.histories, want) {
		t.Errorf("asked for the history of %q, want %q: GET /history with each full read", h.source.histories, want)
	}
	if files.reads > 0 {
		t.Errorf("read the readings history's files %d times, want none while the router answers", files.reads)
	}

	h.stopRouter()
	h.deliver(h.press("R")...)
	h.deliver(h.press("R")...)
	if files.reads != 1 {
		t.Errorf("probing once the router went, read the files %d times, want once", files.reads)
	}
	h.startRouter(routerDocument(three()...))
	h.deliver(h.press("R")...)
	h.stopRouter()
	h.deliver(h.press("R")...)
	if files.reads != 2 {
		t.Errorf("probing again once the router came back and went, read the files %d times, want once more, 2", files.reads)
	}
}

func TestStepFor(t *testing.T) {
	tests := []struct {
		key    string
		want   time.Duration
		wantOK bool
	}{
		{key: "5h", want: 5 * time.Minute, wantOK: true},
		{key: "7d", want: 30 * time.Minute, wantOK: true},
		{key: "7d_oi", want: 30 * time.Minute, wantOK: true},
		{key: "30d", want: 44 * time.Minute, wantOK: true},
		{key: "overage", wantOK: false},
	}
	for _, tt := range tests {
		got, ok := stepFor(tt.key)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("stepFor(%q) = %v, %v, want %v, %v", tt.key, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestRecordKeepsTheNewestReadings(t *testing.T) {
	var readings []score.Reading
	for i := range mostSeen + 5 {
		readings = record(readings, score.Reading{At: start.Add(time.Duration(i) * time.Minute), Utilization: float64(i)})
	}
	if len(readings) != mostSeen || readings[0].Utilization != 5 {
		t.Errorf("kept %d readings from %v, want the newest %d", len(readings), readings[0].Utilization, mostSeen)
	}
	if again := record(readings, score.Reading{At: start, Utilization: 99}); len(again) != len(readings) || again[len(again)-1].Utilization == 99 {
		t.Error("record() took a reading no newer than the newest")
	}
}

// historyOf is GET /history's answer of an account's window, started at
// start, a point each five minutes of the uses given.
func historyOf(id string, start time.Time, uses ...float64) router.History {
	a := router.AccountHistory{ID: id, Start: start.UTC()}
	for i, u := range uses {
		a.Points = append(a.Points, router.HistoryPoint{At: start.Add(time.Duration(i) * 5 * time.Minute).UTC(), Utilization: u})
	}
	return router.History{Accounts: []router.AccountHistory{a}}
}

// sameReading reports whether a and b are the same reading.
func sameReading(a, b score.Reading) bool {
	return a.At.Equal(b.At) && a.Utilization == b.Utilization && a.Last.Equal(b.Last)
}

// errTimedOut is a call to the router that took too long.
var errTimedOut = errors.New("context deadline exceeded")
