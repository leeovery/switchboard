package dashboard

import (
	"iter"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// The day the Log's page tests are of, Thursday 1 October, from 09:00 and at
// noon, and the day before, near its end.
var (
	logDay       = time.Date(2026, 10, 1, 9, 0, 0, 0, now.Location())
	logDayNoon   = time.Date(2026, 10, 1, 12, 0, 0, 0, now.Location())
	theDayBefore = time.Date(2026, 9, 30, 23, 50, 0, 0, now.Location())
)

// threeAsked is a page of Requests of three lines of today's, from 09:00 a
// minute apart, at noon.
func threeAsked() LogPage {
	src := views.LogSources{From: logDay, Now: logDayNoon}
	return requestsOf(src, []ledger.Line{asked("1", logDay), asked("2", logDay.Add(time.Minute)), asked("3", logDay.Add(2*time.Minute))})
}

// The state line says how the table stands: following; paused, at the line
// chosen, with the lines come since; not live, to when the ledger last
// wrote, paused saying so first; and at its right today's lines, or
// events, the table shows.
func TestTheLogsStateLineSaysHowTheTableStands(t *testing.T) {
	notLive := threeAsked()
	notLive.Live = false
	paused := "‖ paused at 09:02:00   end to follow"
	tests := []struct {
		name        string
		page        LogPage
		left, right string
	}{
		{name: "following", page: threeAsked(), left: "● following", right: "3 lines today"},
		{name: "paused", page: threeAsked().Chose(-1, 10), left: paused, right: "3 lines today"},
		{
			name: "not live", page: notLive, right: "3 lines today",
			left: "○ not live   the ledger's lines to 09:02:01; those since come in as the router answers again",
		},
		{name: "paused while not live", page: notLive.Chose(-1, 10), left: paused, right: "3 lines today"},
		{name: "events", page: threeAsked().Turned(1), left: "● following", right: "0 events today"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.left + strings.Repeat(" ", 128-utf8.RuneCountInString(tt.left+tt.right)) + tt.right
			if got := tt.page.state(10).line(128, logDayNoon).plain(); got != want {
				t.Errorf("the state line reads\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// Paused, the state line counts the lines come since below the paused foot.
func TestThePausedStateLineCountsTheLinesComeSince(t *testing.T) {
	src := views.LogSources{From: logDay, Now: logDayNoon}
	lines := []ledger.Line{asked("1", logDay), asked("2", logDay.Add(time.Minute))}
	p := requestsOf(src, lines).Chose(-1, 10)
	p = requestsOf(src, append(lines, asked("3", logDay.Add(2*time.Minute)))).withStands(p)
	if got, want := p.state(10).line(128, logDayNoon).plain(), "‖ paused at 09:01:00   ▼ 1 new   end to follow"; !strings.HasPrefix(got, want) {
		t.Errorf("the state line reads %q, want it to start %q", got, want)
	}
}

// labelledWork is the router's document of work, labelled Work, and side,
// where new sessions go.
var labelledWork = routerDoc("side", 0, status.Account{ID: "work", Label: "Work", TokenSet: true}, status.Account{ID: "side", TokenSet: true})

// The heading row names the Log, its sub-tabs and what its filter keeps,
// and at its right, the table's count: of Requests, today's lines hidden,
// and by what where one thing alone hides them; of Events, today's; on a
// phone, the count alone, padded sub-tabs two cells after the name.
func TestTheLogsHeadingSaysWhatTheTableHides(t *testing.T) {
	src := views.LogSources{From: logDay, Now: logDayNoon, Filter: views.LogOpening()}
	checked := asked("2", logDay.Add(time.Minute))
	checked.Kind = ledger.KindCheck
	hidden := requestsOf(src, []ledger.Line{asked("1", logDay), checked})
	filtered := hidden
	filtered.Filter.Accounts = []string{"side"}
	onWork := hidden
	onWork.Filter.Accounts = []string{"work"}
	onD28C := hidden
	onD28C.Filter.Sessions = []string{"d28c5e17-0000-4000-8000-000000000001"}
	onOpus := hidden
	onOpus.Filter.Models = []string{"claude-opus-5-5"}
	eventsFiltered := hidden.Turned(1)
	eventsFiltered.Shows.Accounts = []string{"work"}
	sessionsEvents := eventsOfItsSession(views.LogFilter{Accounts: []string{"work"}})
	sessionsEvents.Shows.Sessions = []string{sessionsEvents.Events.Rows[0].Session}
	tests := []struct {
		name         string
		page         LogPage
		width        int
		starts, ends string
	}{
		{name: "before the first read", page: LogPage{}, width: 120, starts: "LOG    Requests  Events  [ ]  ─", ends: "──"},
		{name: "a check hidden", page: hidden, width: 120, starts: "LOG    Requests  Events  [ ]  ─", ends: "─  1 hidden: checks   f what it shows"},
		{name: "filtered", page: filtered, width: 120, starts: "LOG    Requests  Events  [ ]   only side ─", ends: "─  1 hidden: checks   f what it shows"},
		{name: "events", page: hidden.Turned(1), width: 120, starts: "LOG    Requests  Events  [ ]  ─", ends: "─  0 today   f what it shows"},
		{name: "filtered to an account, named by its label", page: onWork, width: 120, starts: "LOG    Requests  Events  [ ]   only Work ─", ends: "─  1 hidden: checks   f what it shows"},
		{name: "events filtered by their own panel", page: eventsFiltered, width: 120, starts: "LOG    Requests  Events  [ ]   only Work ─", ends: "─  0 today   f what it shows"},
		{name: "events, the Requests alone filtered", page: filtered.Turned(1), width: 120, starts: "LOG    Requests  Events  [ ]  ─", ends: "─  0 today   f what it shows"},
		{
			name: "events filtered to a session the Requests show none of, named by its directory as its event is", page: sessionsEvents, width: 120,
			starts: "LOG    Requests  Events  [ ]   only d28c switchboard ─", ends: "─  1 today   f what it shows",
		},
		{name: "on a phone", page: hidden, width: 52, starts: "LOG   Requests  Events" + strings.Repeat(" ", 19), ends: "   1 hidden  f"},
		{name: "on a phone, filtered to a session", page: onD28C, width: 52, starts: "LOG   Requests  Events    only d28c ", ends: "   1 hidden  f"},
		{name: "on a phone, filtered to an account", page: onWork, width: 52, starts: "LOG   Requests  Events    only Work ", ends: "   1 hidden  f"},
		{name: "on a phone, filtered to a model", page: onOpus, width: 52, starts: "LOG   Requests  Events    only opus-5-5 ", ends: "  1 hidden  f"},
		{name: "on a phone, Events filtered", page: eventsFiltered, width: 52, starts: "LOG   Requests  Events    only Work ", ends: "   0 today  f"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := logFrameOf(tt.page, tt.width, 24)
			got := textOf(f.Draw(labelledWork, logDayNoon))[f.headingY()]
			if !strings.HasPrefix(got, tt.starts) || !strings.HasSuffix(got, tt.ends) || utf8.RuneCountInString(got) != tt.width {
				t.Errorf("the heading row reads\n%q\nwant it to start %q and end %q, at the last column", got, tt.starts, tt.ends)
			}
		})
	}
}

// eventsOfItsSession is the Log's page showing Events, at noon, of d28c
// starting at 09:00 in switchboard, its one line on side, the Requests
// filtered as filter says.
func eventsOfItsSession(filter views.LogFilter) LogPage {
	first := asked("1", logDay)
	first.Dir = "/home/dev/switchboard"
	lines := []ledger.Line{first}
	src := views.LogSources{From: logDay, Now: logDayNoon, Filter: filter}
	p := requestsOf(src, lines)
	started := status.Event{ID: 1, At: logDay, Kind: status.EventStarted, Session: first.Session, Account: "side"}
	p.Events = views.LogEvents(views.LogEventSources{
		From: logDay, Events: views.ReadEvents(filedEvents{started}, logDay, logDayNoon), Now: logDayNoon,
		Held: viewstest.ReadLog(sessionsHeld(lines), heldOf(lines), ledger.Pricing, logDayNoon).Held,
	})
	return p.Turned(1)
}

// filedEvents are the router's files of events holding those given, of the
// router's run that started at noon the day before.
type filedEvents []status.Event

func (f filedEvents) Between(from, to time.Time) iter.Seq[events.Line] {
	return func(yield func(events.Line) bool) {
		for _, e := range f {
			if !e.At.Before(from) && e.At.Before(to) && !yield(events.Line{Event: e, Run: theDayBefore.Add(-12 * time.Hour)}) {
				return
			}
		}
	}
}

// FirstDay is the start of the day the router's run that filed them
// started on, the day before.
func (f filedEvents) FirstDay() (time.Time, error) {
	return views.Today(theDayBefore).Start, nil
}

// eventsShown is the Log's page showing Events of the events filed given, from
// the day before, at noon.
func eventsShown(filed ...status.Event) LogPage {
	src := views.LogSources{From: theDayBefore, Now: logDayNoon}
	p := requestsOf(src, nil)
	read := views.ReadEvents(filedEvents(filed), theDayBefore, logDayNoon)
	p.Events = views.LogEvents(views.LogEventSources{From: theDayBefore, Events: read, Now: logDayNoon})
	return p.Turned(1)
}

// work's cap, at 09:30, 95% of its 5-hour window, three sessions moved.
var workAtItsCap = status.Event{
	ID: 2, At: logDay.Add(30 * time.Minute), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05, Count: 3, To: "side",
}

// The Events draw their days as the Requests do, a day's rule between them,
// in the design's columns, or a phone's, the session leaving: a cap's use
// bold in its level's colour, and the sessions its moves counted a number
// that brightens as it rises.
func TestTheEventsDrawTheirDaysInTheirColumns(t *testing.T) {
	pinned := status.Event{ID: 1, At: theDayBefore, Kind: status.EventPin, Accounts: []string{"personal"}}
	p := eventsShown(pinned, workAtItsCap)
	wide := textOf(logFrameOf(p, 160, 24).Draw(logAccounts(), logDayNoon))
	want := []string{
		"  at        session            event     account          what happened",
		"● 23:50:00                     pin       personal         new sessions go there",
		"  Wed 30 Sep  " + rule(135) + "  Thu 1 Oct",
		"╎ 09:30:00                     cap       work             95% of its 5-hour window: its 3 sessions move to side as their next requests come",
	}
	if got := wide[6:10]; !slices.Equal(got, want) {
		t.Errorf("the Events read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	phone := logFrameOf(p, 52, 24)
	if got, want := textOf(phone.Draw(logAccounts(), logDayNoon))[phone.top()], "  at        event     account   what happened"; got != want {
		t.Errorf("on a phone, the heads read %q, want %q", got, want)
	}

	c := logFrameOf(p, 160, 24).page(logAccounts(), logDayNoon)
	if use := c.at(58, 9).ink; use.token != theme.VizRamp4 || !use.bold {
		t.Errorf("the cap's 95%% is inked %+v, want bold in the ramp's last stop", use)
	}
	numbers := logFrameOf(p, 160, 24).Numbers(routerDoc("side", 0), logDayNoon)
	if got, want := numbers[Number{Kind: NumberMoved, Of: eventKeyed(p.Events.Rows[1].Key)}], "3"; got != want {
		t.Errorf("the cap's moved sessions are numbered %q, want %q", got, want)
	}
}

// With none of today's, each table says so at its foot, under the last day
// read, with no rule above it.
func TestATableWithNothingTodaySaysSo(t *testing.T) {
	pinned := status.Event{ID: 1, At: theDayBefore, Kind: status.EventPin, Accounts: []string{"personal"}}
	tests := []struct {
		name string
		page LogPage
		doc  status.Document
		want []string
	}{
		{name: "requests", page: requestsOf(views.LogSources{From: theDayBefore, Now: logDayNoon}, []ledger.Line{asked("1", theDayBefore)}), doc: logAccounts(), want: []string{
			"▲ 23:50:00  d28c               opus-5-5   side      200",
			"  no requests today",
		}},
		{name: "events", page: eventsShown(pinned), doc: routerDoc("personal", 0, status.Account{ID: "personal", TokenSet: true}), want: []string{
			"● 23:50:00                     pin       personal  new sessions go there",
			"  no events today",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := logFrameOf(tt.page, 160, 24)
			got := textOf(f.Draw(tt.doc, logDayNoon))[f.top()+1 : f.top()+3]
			for i := range tt.want {
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Errorf("the table reads\n%s\nwant it to start\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
					break
				}
			}
		})
	}
}

// g lands on Requests, paused at its newest line, the first request at or
// after an event's moment chosen, in view.
func TestLandingChoosesTheFirstRequestAtOrAfterAMoment(t *testing.T) {
	p := threeAsked().Turned(1).Landed(logDay.Add(30*time.Second), 2)
	row, _, ok := p.Chosen()
	switch {
	case p.Shown != LogRequests || !p.Paused():
		t.Errorf("landed, the Log shows %v, paused %v; want Requests, paused", p.Shown, p.Paused())
	case !ok || !row.At.Equal(logDay.Add(time.Minute)):
		t.Errorf("landed at 09:00:30, the row chosen is %v, want 09:01:00's", row.At)
	}
	lines := p.lines()
	if first, last := p.window(lines, p.stand(), 2); first != 1 || last != 2 {
		t.Errorf("landed, lines %d to %d are in view, want 1 to 2, the chosen kept in", first, last)
	}
}

// A table whose filter changes what it shows stands as it first opened,
// unfilled, so, short of the view it filled, it reads back to fill it: as
// the find's enter, the panel's, and esc letting the filter go each change
// it, where the days' summaries bound it, as its page's Reach has it, the
// Events through a session so bound though an account is chosen too, and
// through kinds alone reading back as on opening. One they don't bound
// stays as it stood, reading back only as its top is reached: the Requests
// through a filter of the failed of no session, and the Events through one
// of an account alone.
func TestATableNewlyFilteredReadsBackToFillItsView(t *testing.T) {
	const session = "d28c5e17-0000-4000-8000-000000000001"
	src := views.LogSources{From: logDay, Now: logDayNoon}
	ofSession := []ledger.Line{asked("1", logDay), asked("2", logDay.Add(time.Minute)), asked("3", logDay.Add(2*time.Minute))}
	every := slices.Concat(ofSession, []ledger.Line{askedIn("4", "c4ec0000-0000-4000-8000-000000000009", logDay.Add(3*time.Minute))})
	pinned := status.Event{ID: 1, At: logDay, Kind: status.EventPin, Accounts: []string{"personal"}}
	// keeping is p, the table its filter's change refilters built afresh
	// of the lines or events it keeps, as the watch builds it.
	keeping := func(p LogPage, lines []ledger.Line, events ...status.Event) LogPage {
		if p.Shown == LogEvents {
			read := views.ReadEvents(filedEvents(events), logDay, logDayNoon)
			p.Events = views.LogEvents(views.LogEventSources{From: logDay, Events: read, Filed: views.OldestKept(theDayBefore, 0, logDayNoon), Now: logDayNoon})
			return p
		}
		p.Requests = requestsOf(src, lines).Requests
		return p
	}
	requests := requestsOf(src, every).Filling(4)
	events := keeping(threeAsked().Turned(1), nil, pinned, workAtItsCap).Filling(2)
	picked, pins, failed, failedOf, worked := requests.Panel(), events.Panel(), requests.Panel(), requests.Panel(), events.Panel()
	picked.Filter.Sessions, pins.Shows.Kinds, failed.Filter.Failed = []string{session}, []string{status.EventPin}, true
	failedOf.Filter.Sessions, failedOf.Filter.Failed, worked.Shows.Accounts = []string{session}, true, []string{"work"}
	workedOf := worked
	workedOf.Shows.Sessions = []string{session}
	tests := []struct {
		name  string
		page  LogPage
		rows  int
		wants bool
	}{
		{name: "the find's enter", page: keeping(requests.FoundOnly(session), ofSession), rows: 4, wants: true},
		{name: "the panel's enter", page: keeping(requests.Applied(picked), ofSession), rows: 4, wants: true},
		{name: "the Events' panel's enter", page: keeping(events.Applied(pins), nil, pinned), rows: 2, wants: true},
		{name: "esc letting the filter go", page: keeping(keeping(requests.FoundOnly(session), ofSession).Filling(3).Unfiltered(), every), rows: 5, wants: true},
		{name: "the panel's enter on the failed of a session", page: keeping(requests.Applied(failedOf), nil), rows: 4, wants: true},
		{name: "the find's enter, on the Events", page: keeping(events.FoundOnly(session), nil), rows: 2, wants: true},
		{name: "the Events' panel's enter on a session and an account", page: keeping(events.Applied(workedOf), nil), rows: 2, wants: true},
		{name: "the panel's enter on the failed", page: keeping(requests.Applied(failed), nil), rows: 4},
		{name: "the Events' panel's enter on an account", page: keeping(events.Applied(worked), nil, workAtItsCap), rows: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.page.WantsDayBefore(tt.rows); got != tt.wants {
				t.Errorf("refiltered, short of the view of %d rows it filled, the table wants the day before: %t, want %t", tt.rows, got, tt.wants)
			}
		})
	}
}

// The session of the line chosen, whose page s opens, is the one a request
// counts toward, as views count it: none for a quota check, sent under an
// id never used again; a session's event's; and none of an account's event,
// nor with nothing chosen.
func TestTheLineChosensSessionIsTheOneItCountsToward(t *testing.T) {
	const session = "d28c5e17-0000-4000-8000-000000000001"
	check := asked("2", logDay.Add(time.Minute))
	check.Kind, check.Session = ledger.KindCheck, "c4ec0000-0000-4000-8000-000000000009"
	requests := requestsOf(views.LogSources{From: logDay, Now: logDayNoon}, []ledger.Line{asked("1", logDay), check})
	started := status.Event{ID: 1, At: logDay, Kind: status.EventStarted, Account: "work", Session: session}
	tests := []struct {
		name string
		page LogPage
		want string
	}{
		{name: "nothing chosen", page: requests},
		{name: "a request", page: times(chose(-1, 4), 2)(requests), want: session},
		{name: "a quota check", page: requests.Chose(-1, 4)},
		{name: "a session's event", page: eventsShown(started, workAtItsCap).Chose(-1, 4).Chose(-1, 4), want: session},
		{name: "an account's event", page: eventsShown(started, workAtItsCap).Chose(-1, 4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.page.ChosenSession(); got != tt.want {
				t.Errorf("the line chosen is of the session %q, want %q", got, tt.want)
			}
		})
	}
}

// A table holding no row has nothing to pause at: ↑ and scrolling up leave
// it following. One holding rows, but none in view, chooses the newest above
// it, scrolled into view.
func TestATableHoldingNoRowDoesntPause(t *testing.T) {
	yesterdays := requestsOf(views.LogSources{From: theDayBefore, Now: logDayNoon}, []ledger.Line{asked("1", theDayBefore)})
	empty := requestsOf(views.LogSources{From: logDay, Now: logDayNoon}, nil)
	tests := []struct {
		name   string
		page   LogPage
		paused bool
	}{
		{name: "Requests holding none, ↑", page: empty.Chose(-1, 10)},
		{name: "Requests holding none, scrolled up", page: empty.Scrolled(-1, 10)},
		{name: "Events holding none, ↑", page: threeAsked().Turned(1).Chose(-1, 10)},
		{name: "Events holding none of the day before's either, scrolled up", page: yesterdays.Turned(1).Scrolled(-1, 10)},
		{name: "Requests of the day before's, no request today in view, ↑", page: yesterdays.Chose(-1, 1), paused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.page.state(1).line(128, logDayNoon).plain()
			if tt.page.Paused() != tt.paused || strings.Contains(state, "Jan") {
				t.Errorf("paused %v, the state line reading %q; want paused %v, never at the zero time", tt.page.Paused(), state, tt.paused)
			}
		})
	}
	if row, _, ok := yesterdays.Chose(-1, 1).Chosen(); !ok || !row.At.Equal(theDayBefore) {
		t.Errorf("no request today in view, ↑ chose %v, want the day before's 23:50:00", row.At)
	}
}

// On opening, a table reads back a day at a time till it fills its view,
// then only as its view's top, paused, reaches the first line held, or,
// following, as its view grows past the rows it filled, where it refills,
// as the failed alone don't; never past the oldest day the ledger keeps.
// At its reach, it fills its view no further back, and of the Requests
// reads back no further at all, while the Events, paused at their top, read
// back past it. A view of no rows fills nothing: the table reads back to
// fill it once the terminal gives it rows.
func TestATableWantsTheDayBeforeTillItFillsItsView(t *testing.T) {
	src := views.LogSources{From: logDay, Now: logDayNoon}
	oldest := src
	oldest.Oldest = views.OldestKept(logDay, 0, logDayNoon)
	failed := threeAsked()
	failed.Filter.Failed = true
	atReach := func(p LogPage) LogPage {
		p.AtReach = true
		return p
	}
	pinned := status.Event{ID: 1, At: logDay, Kind: status.EventPin, Accounts: []string{"personal"}}
	events := threeAsked().Turned(1)
	read := views.ReadEvents(filedEvents([]status.Event{pinned, workAtItsCap}), logDay, logDayNoon)
	events.Events = views.LogEvents(views.LogEventSources{From: logDay, Events: read, Filed: views.OldestKept(theDayBefore, 0, logDayNoon), Now: logDayNoon})
	tests := []struct {
		name string
		page LogPage
		rows int
		want bool
	}{
		{name: "short of its view", page: threeAsked(), rows: 4, want: true},
		{name: "its view filled", page: threeAsked().Filling(3), rows: 3},
		{name: "filled, short again as the view grows, following", page: threeAsked().Filling(3), rows: 10, want: true},
		{name: "filled again, as the view grew", page: threeAsked().Filling(1).Filling(3), rows: 3},
		{name: "filled, the view grown, paused short of its first line", page: threeAsked().Filling(1).Scrolled(-1, 1), rows: 2},
		{name: "filled, the view grown, paused at its first line", page: threeAsked().Filling(1).Scrolled(-1, 2), rows: 2, want: true},
		{name: "its failed alone, filled, short again as the view grows, following", page: failed.Filling(3), rows: 10},
		{name: "its failed alone, filled, the view grown, paused at its first line", page: failed.Filling(1).Scrolled(-1, 2), rows: 2, want: true},
		{name: "with no rows, nothing to read", page: threeAsked().Filling(0), rows: 0},
		{name: "opened with no rows, then given them", page: threeAsked().Filling(0), rows: 10, want: true},
		{name: "filled, paused at its first line", page: threeAsked().Filling(3).Scrolled(-1, 2), rows: 2, want: true},
		{name: "at the oldest day kept", page: requestsOf(oldest, nil), rows: 4},
		{name: "at its reach, short of its view", page: atReach(threeAsked()), rows: 4},
		{name: "at its reach, paused at its first line", page: atReach(threeAsked()).Filling(1).Scrolled(-1, 2), rows: 2},
		{name: "the Events short of their view", page: events, rows: 10, want: true},
		{name: "the Events at their reach, short of their view", page: atReach(events), rows: 10},
		{name: "the Events at their reach, paused at their first line", page: atReach(events).Filling(1).Scrolled(-1, 2), rows: 2, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.page.WantsDayBefore(tt.rows); got != tt.want {
				t.Errorf("WantsDayBefore(%d) = %v, want %v", tt.rows, got, tt.want)
			}
		})
	}
}

// A table that, as it will show, doesn't fill the view it filled from the
// day the Log lets go of the days before on is unfilled, so, following,
// short of its view without them, it reads back through its filter to fill
// it again: one that filled it from a day before; one that can't fill it
// from what it holds, as one reaching as far back as it reads marks it
// filled; and the Requests built through a filter not the page's, whatever
// they fill. One that filled it from that day or after stands as it stood,
// reading back nothing. Unfilled, the page draws as it did.
func TestATableThatFilledItsViewFromADayLetGoIsUnfilled(t *testing.T) {
	before := []ledger.Line{asked("0", theDayBefore)}
	today := []ledger.Line{asked("1", logDay), asked("2", logDay.Add(time.Minute)), asked("3", logDay.Add(2*time.Minute))}
	spanned := requestsOf(views.LogSources{From: theDayBefore, Now: logDayNoon}, slices.Concat(before, today))
	tests := []struct {
		name string
		rows int
		// atReach is whether the table reaches as far back as it reads, as
		// AtReach says, as it fills.
		atReach bool
		// from is in the day the Log lets go of the days before.
		from time.Time
		// failed is whether the page's filter keeps the failed alone, its
		// Requests built through every line.
		failed   bool
		unfilled bool
	}{
		{name: "filled from the day before, let go", rows: 4, from: logDay, unfilled: true},
		{name: "reaching as far back as it reads, short of its view", rows: 10, atReach: true, from: logDay, unfilled: true},
		{name: "the Requests built through a filter not the page's", rows: 3, from: logDay, failed: true, unfilled: true},
		{name: "filled from today", rows: 3, from: logDay},
		{name: "filled from the day before, kept", rows: 4, from: theDayBefore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reaching := spanned
			reaching.AtReach = tt.atReach
			filled := reaching.Filling(tt.rows)
			filled.Filter.Failed = tt.failed
			from := views.Today(tt.from)
			unfilled := filled.Unfilled(from)
			doc := routerDoc("side", 0)
			if drawn, was := logFrameOf(unfilled, 160, 24).Draw(doc, logDayNoon), logFrameOf(filled, 160, 24).Draw(doc, logDayNoon); !slices.Equal(drawn, was) {
				t.Errorf("unfilled, the page draws\n%s\nwant it drawn as before\n%s", strings.Join(drawn, "\n"), strings.Join(was, "\n"))
			}
			if !tt.unfilled {
				if !reflect.DeepEqual(unfilled.Stands, filled.Stands) {
					t.Errorf("the days before %s let go, the tables stand %+v, want them as they stood, %+v", from.Date, unfilled.Stands, filled.Stands)
				}
				return
			}
			letGo := unfilled
			letGo.From, letGo.AtReach = from, false
			letGo.Requests = requestsOf(views.LogSources{From: tt.from, Now: logDayNoon, Filter: letGo.Filter}, today).Requests
			if !letGo.WantsDayBefore(tt.rows) {
				t.Errorf("the days before %s let go, the table in a view of %d rows wants the day before: false, want true", from.Date, tt.rows)
			}
		})
	}
}
