package watch

import (
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// projectsLedgerOf is h's ledger, as projectsHarness made it.
func projectsLedgerOf(h *harness) *projectsLedger {
	return h.model.cfg.Ledger.(*projectsLedger)
}

// freshPage is Projects' page of h as one read of its days afresh builds it
// at the clock's time: from the day its days are held from, today's lines
// read whole, the List as it stands.
func freshPage(h *harness) views.ProjectsPage {
	now, m := h.clock.now, h.model
	l := projectsLedgerOf(h).Ledger
	days := views.ReadSessionDays(l, m.projects.from, now).Tailed(l.Day(dayfile.DateOf(now)), true, nil, nil, now)
	return days.Projects(m.projectsInput(now, m.held.List(m.listLive(now, m.traffic.stream(now)))))
}

// sameAsFresh fails the test where Projects' page shown isn't the one its
// days read afresh build, as freshPage builds it, every project's page and
// sessions alike, as drawnOf has them, counting requests in all.
func sameAsFresh(t *testing.T, h *harness, requests int, after string) {
	t.Helper()
	page := h.model.projects.shown.Page
	if page == nil {
		t.Fatalf("after %s, Projects shows no page, want one built", after)
	}
	if got, want := drawnOf(*page), drawnOf(freshPage(h)); page.All.Requests != requests || !reflect.DeepEqual(got, want) {
		t.Errorf("after %s, the page counts %d requests, its projects\n%+v\nwant %d, the projects of the days read afresh\n%+v", after, page.All.Requests, got, requests, want)
	}
}

// readToday reports whether Projects' tests' ledger has had today's lines
// read whole, as Day reads a day.
func readToday(h *harness) bool {
	return slices.Contains(projectsLedgerOf(h).days, dayfile.DateOf(h.clock.now))
}

// The sessions' days Projects is built of take today's from every read of
// the ledger's tail, from the first, though Projects never shows; shown,
// Projects reads the days before today alone, never today's whole, and its
// page counts what the tail gave.
func TestTodayIsFedFromTheFirstReadOfTheTailThoughProjectsNeverShows(t *testing.T) {
	h := projectsHarness(t, dashboard.Sessions)
	h.start()
	now := h.clock.now
	rows := func(days views.SessionDays) []string {
		var paths []string
		for _, r := range days.Projects(h.model.projectsInput(now, views.SessionList{})).Rows {
			paths = append(paths, r.Path)
		}
		return paths
	}
	if got, want := rows(h.model.projects.tailed), []string{"/code/alpha", "/code/beta"}; h.model.projects.asked != 0 || !slices.Equal(got, want) {
		t.Fatalf("on Sessions, Projects read %d times, the tail's days hold %v; want no read, and today's sessions, %v", h.model.projects.asked, got, want)
	}
	projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", now.Add(time.Second), 1)...)
	h.tickUntilAsked()
	h.keys(shiftTabKey, shiftTabKey)
	if h.model.view != dashboard.Projects {
		t.Fatalf("shift+tab twice from Sessions shows %s, want Projects", h.model.view)
	}
	sameAsFresh(t, h, 7, "the tail gained beta's third, then Projects showed")
	if readToday(h) {
		t.Errorf("today's lines were read whole, the days %v; want them the tail's alone", projectsLedgerOf(h).days)
	}
}

// Till the tail is first read, Projects builds no page, its own read and
// the List's landed, and says it's loading, as today's sessions are the
// tail's to give; once it's read, the page is built of them.
func TestProjectsAwaitsTheTailsFirstRead(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	held := holdingBack[tailedMsg](h, h.init()...)
	if p := h.model.projects; len(held) != 1 || p.days == nil || !h.model.sessionsTab.listed || p.shown.Page != nil || !h.model.awaiting() {
		t.Fatalf("the tail's first read held back, %d held, Projects' days read %v, the List held %v, a page built %v, awaiting %v; want one held, its days and the List read, no page, awaiting",
			len(held), p.days != nil, h.model.sessionsTab.listed, p.shown.Page != nil, h.model.awaiting())
	}
	h.deliver(held...)
	sameAsFresh(t, h, 6, "the tail's first read landed")
}

// A read of the tail that gains a line builds Projects' page afresh while
// it shows, counting the line; one that gains none builds nothing.
func TestProjectsIsBuiltAfreshOnlyAsTheTailGainsALine(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.start()
	pages := h.model.projects.pages
	projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", h.clock.now.Add(time.Second), 1)...)
	h.tickUntilAsked()
	if got := h.model.projects.pages; got != pages+1 {
		t.Errorf("a read of the tail gaining a line built the page %d times, want once", got-pages)
	}
	sameAsFresh(t, h, 7, "a read of the tail gained beta's third")
	read := h.model.tail.at
	h.tickUntilAsked()
	if h.model.tail.at.Equal(read) {
		t.Fatal("the second look read no more of the tail, want a read of it landed")
	}
	if got := h.model.projects.pages; got != pages+1 {
		t.Errorf("a read of the tail gaining none built the page %d times more, want none", got-pages-1)
	}
}

// As the day turns, what the tail folded of the day gone is that day's,
// and the read of the tail after it reads it whole, and each day slept
// through, each once, though the store wants the day gone too: the page
// then counts every line once, those filed on the day gone after its last
// read among them, as the days read afresh give it, awake through the
// turn or asleep for days.
func TestProjectsLosesNoLineAndCountsNoneTwiceAsTheDayTurns(t *testing.T) {
	night := onTheDay(23, 59, 30)
	tests := []struct {
		name  string
		from  time.Time
		after []time.Time
		to    time.Time
	}{
		{name: "awake", from: night, after: []time.Time{night.Add(10 * time.Second), night.Add(35 * time.Second)}, to: night.Add(40 * time.Second)},
		{
			name: "asleep for days", from: onTheDay(23, 0, 0),
			after: []time.Time{onTheDay(23, 30, 0), onTheDay(24+10, 0, 0), onTheDay(48+10, 0, 0), onTheDay(72+9, 0, 0)}, to: onTheDay(72+9, 30, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := projectsHarness(t, dashboard.Projects)
			h.clock.now = tt.from
			h.start()
			sameAsFresh(t, h, 6, "it opened")
			l := projectsLedgerOf(h)
			for _, at := range tt.after {
				l.write(projectAsks(betaOnly, "/code/beta", at, 1)...)
			}
			days := dayfile.Span(dayfile.DayStart(tt.from, 0), dayfile.DayStart(tt.to, -1))
			if owed, unread := h.model.projectsFeed(tt.to).owed, h.model.held.Unread(days); !slices.Equal(owed, days) || !slices.Equal(unread, days) {
				t.Fatalf("before the turn's read, Projects wants %v read again and the store %v; want both to want %v", owed, unread, days)
			}
			read := len(l.days)
			h.clock.now = tt.to
			h.read(routerDocument(three()...))
			if got := l.days[read:]; !slices.Equal(got, days) {
				t.Errorf("the turn's read read the days %v whole, want each day before today since the last read once, %v", got, days)
			}
			sameAsFresh(t, h, 6+len(tt.after), "the turn's read")
			if owed := h.model.projects.tailed.Owed(tt.to); len(owed) > 0 {
				t.Errorf("after the turn's read, the tail's days owe %v, want none", owed)
			}
		})
	}
}

// Till Projects first asks for its own days, the tail's days keep today's
// alone: as the day turns on the Overview, while the stream awaits the line
// of a request that ended and no other reader wants the day gone, the read
// of the tail reads no day for them, and they owe none; Projects, first
// shown after, reads the day gone itself, counting what was filed on it
// after the tail's last read.
func TestTheTurnReadsNoDayForTheTailsDaysTillProjectsAsks(t *testing.T) {
	night := onTheDay(23, 59, 30)
	h := projectsHarness(t, dashboard.Overview)
	h.clock.now = night
	h.start()
	l := projectsLedgerOf(h)
	l.write(projectAsks(betaOnly, "/code/beta", night.Add(10*time.Second), 1)...)
	h.clock.now = night.Add(40 * time.Second)
	at := func(kind string, ago time.Duration) router.StreamEvent {
		return alter(told(kind, "0a000009", "work", 0), func(e *router.StreamEvent) { e.At, e.Status = h.clock.now.Add(-ago).UTC(), 200 })
	}
	h.hear(at(router.StreamSent, 5*time.Second), at(router.StreamDone, time.Second))
	if !h.model.traffic.awaitsLines() || h.model.heldFold(h.clock.now) != nil || h.model.log.opened {
		t.Fatal("the stream awaits no line, or the store or the Log folds the tail; want the tail followed for the stream alone")
	}
	read := len(l.days)
	h.read(routerDocument(three()...))
	if got := l.days[read:]; len(got) > 0 {
		t.Errorf("the turn's read read the days %v whole, want none, no reader wanting them", got)
	}
	if owed := h.model.projects.tailed.Owed(h.clock.now); h.model.tail.unread() || len(owed) > 0 {
		t.Errorf("after the turn's read, the tail read %v, its days owing %v; want it read, owing none", !h.model.tail.unread(), owed)
	}
	h.keys(tabKey, tabKey)
	sameAsFresh(t, h, 7, "Projects first showed after the turn")
}

// The days the sessions' days owe a read are read again whole by the read of
// the tail, though no reader that folds it wants them, and the days take
// them in whole: so they owe none after, and count the day gone's lines
// after their last read.
func TestTheDaysTheTailsDaysOweAreReadThoughNoReaderWantsThem(t *testing.T) {
	night, turned := onTheDay(23, 0, 0), onTheDay(24, 0, 10)
	gone := dayfile.DateOf(night)
	owed := []string{dayfile.DateOf(night.AddDate(0, 0, -1)), gone}
	if got := lateDays(owed, nil, nil, nil); !slices.Equal(got, owed) {
		t.Errorf("no reader folding the read, its late days are %v, want those owed, %v", got, owed)
	}
	if got := lateDays(owed, nil, nil, &heldFold{late: owed[1:]}); !slices.Equal(got, owed) {
		t.Errorf("the store wanting the second too, the late days are %v, want each once, %v", got, owed)
	}
	early := projectAsks(alphaFirst, "/code/alpha", night.Add(-time.Hour), 3)
	late := projectAsks(alphaFirst, "/code/alpha", night.Add(59*time.Minute), 1)
	days := views.ReadSessionDays(ledger.Empty{}, onTheDay(0, 0, 0), night).Tailed(early, true, nil, nil, night)
	feed := projectsFeed{days: days, keeps: true, owed: []string{gone}}
	fed := feed.fed(tailLines{gained: projectAsks(betaOnly, "/code/beta", turned, 1), afresh: true, days: map[string][]ledger.Held{gone: slices.Concat(early, late)}}, turned)
	if owed := fed.Owed(turned); len(owed) > 0 {
		t.Errorf("fed the day gone whole, the days owe %v, want none", owed)
	}
	in := views.ProjectsInput{Period: views.PeriodThisWeek, Names: status.Names{}, Prices: ledger.Pricing, WeekStarts: time.Monday, Family: claude.Provider{}.Family, Now: turned}
	if got := fed.Projects(in).All.Requests; got != 5 {
		t.Errorf("fed the day gone whole, the days count %d requests, want its 4 and today's 1", got)
	}
}

// After a night on another tab, Projects first shown builds no page till
// the tail is read on the new day, though its own read lands first, and
// asks that read at once, not at the next look: the tail's fold of the day
// gone never stands for that day, whose lines filed after the tail's last
// read the page then counts.
func TestProjectsAwaitsTheTailsFirstReadOfTheDay(t *testing.T) {
	h := projectsHarness(t, dashboard.Overview)
	h.clock.now = onTheDay(23, 0, 0)
	h.start()
	projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", onTheDay(23, 30, 0), 1)...)
	h.clock.now = onTheDay(24+9, 0, 0)
	h.keys(tabKey)
	held := holdingBack[tailedMsg](h, h.update(tabKey)...)
	if p := h.model.projects; h.model.view != dashboard.Projects || len(held) != 1 || p.days == nil || p.shown.Page != nil || !h.model.awaiting() {
		t.Fatalf("shown the morning after, on %s, %d reads of the tail held, its days read %v, a page built %v, awaiting %v; want Projects, the tail's read asked, its days read, no page, awaiting it",
			h.model.view, len(held), p.days != nil, p.shown.Page != nil, h.model.awaiting())
	}
	h.deliver(held...)
	sameAsFresh(t, h, 7, "the tail's first read of the day")
}

// Open across midnight, Projects keeps its page drawn as it was built, not
// loading, nor built again of the tail's fold of the day gone, and asks a
// read of the tail at once, not at the next look; once it lands, the page is
// today's, the day gone's and today's lines counted.
func TestProjectsKeepsItsPageAcrossMidnightTillTheTailsReadOfTheDay(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.clock.now = onTheDay(23, 59, 30)
	h.start()
	if h.model.tail.due || h.model.tail.busy() {
		t.Fatal("the read of the tail is due or under way before midnight, want neither, for the turn to ask it")
	}
	pages, page := h.model.projects.pages, h.model.projects.shown.Page
	projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", onTheDay(24, 0, 2), 1)...)
	h.clock.now = onTheDay(24, 0, 5)
	held := holdingBack[tailedMsg](h, h.update(tea.WindowSizeMsg{Width: 160, Height: 50})...)
	if p := h.model.projects; len(held) != 1 || p.shown.Page != page || p.pages != pages || h.model.awaiting() {
		t.Fatalf("past midnight, %d reads of the tail held, the page kept %v, built %d times more, awaiting %v; want the tail's read asked, the page kept unbuilt, not awaiting",
			len(held), p.shown.Page == page, p.pages-pages, h.model.awaiting())
	}
	h.deliver(held...)
	sameAsFresh(t, h, 7, "the tail's read of the day landed")
}

// A later read of a day gone stands for the tail's copy of it: the day read
// whole as it turned, a line filed on it after with no reader awaiting it,
// and a step further back hours later reading it with the line, the page
// counts the line; and the watch has let go of what Without lets go of,
// whose drops views' TestTheLaterReadOfADayStands holds.
func TestALaterReadOfADayGoneStandsForTheTailsCopy(t *testing.T) {
	night := onTheDay(23, 59, 30)
	h := projectsHarness(t, dashboard.Projects)
	h.clock.now = night
	h.start()
	h.clock.now = onTheDay(24, 0, 30)
	h.read(routerDocument(three()...))
	projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", night.Add(20*time.Second), 1)...)
	h.clock.now = onTheDay(24+10, 0, 0)
	h.deliver(h.press(">")...)
	sameAsFresh(t, h, 8, "a step to 30 days at ten read the day gone again")
	if p := h.model.projects; p.tailed.Told(p.tailed.Without(*p.days)) {
		t.Error("the tail's days hold what Without lets go of, want the watch to have let go of it")
	}
}

// A read of the tail under way as Projects first shows, asked before it
// asked for its days, so feeding them today's alone, still tells its page
// the line it gained once it lands, though Projects' read landed first and
// the page was built of the fold before it.
func TestATailReadUnderWayAsProjectsFirstShowsBuildsItsPageAgain(t *testing.T) {
	h := projectsHarness(t, dashboard.Sessions)
	h.start()
	projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", h.clock.now.Add(-time.Second), 1)...)
	read := h.readingTheTail()
	if h.model.projects.asked != 0 {
		t.Fatal("Projects asked for its days before the read of the tail was asked, want it not yet shown")
	}
	h.keys(shiftTabKey, shiftTabKey)
	if p := h.model.projects; h.model.view != dashboard.Projects || p.shown.Page == nil || p.shown.Page.All.Requests != 6 {
		t.Fatalf("shown, on %s, a page built %v; want Projects, its page built of the fold before the read under way, 6 requests", h.model.view, p.shown.Page != nil)
	}
	h.deliver(h.run(read)...)
	sameAsFresh(t, h, 7, "the read under way landed")
}

// A read of the tail counts as feeding Projects' days only where it changed
// them: a line of today, or a day they keep read whole again; never lines
// all of another day, nor a day read whole for another reader before the
// first Projects' read reads.
func TestProjectsCountsAsFedOnlyWhatChangedItsDays(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.start()
	h.deliver(h.press(">")...)
	p, now := h.model.projects, h.clock.now
	later := now.Add(time.Second)
	gamma := projectAsks(gammaOnly, "/code/gamma", now.AddDate(0, 0, -1), 1)
	tests := []struct {
		name   string
		gained []ledger.Held
		afresh bool
		days   map[string][]ledger.Held
		fed    bool
	}{
		{name: "nothing read"},
		{name: "a line of today", gained: projectAsks(betaOnly, "/code/beta", now, 1), fed: true},
		{name: "today's read afresh", afresh: true, fed: true},
		{name: "lines all of another day", gained: gamma},
		{name: "a day they keep read whole after Projects' read", days: map[string][]ledger.Held{dayfile.DateOf(gamma[0].At): gamma}, fed: true},
		{name: "a day before Projects' read's first read whole", days: map[string][]ledger.Held{dayfile.DateOf(now.AddDate(0, 0, -40)): nil}},
	}
	for _, tt := range tests {
		fed := p.tailFed(projectsFeed{days: p.tailed, keeps: true}.fed(tailLines{gained: tt.gained, afresh: tt.afresh, days: tt.days}, later))
		if got := fed.fed != p.fed; got != tt.fed {
			t.Errorf("%s, counted as fed %t, want %t", tt.name, got, tt.fed)
		}
	}
}

// A line filed on the day gone after the turn's read reaches Projects as
// another reader reads the day again for it: on Sessions, Projects' days
// read, the store, awaiting the line of a request across midnight the
// stream told of, reads the day gone again once a read of today's lines
// hasn't found it, and Projects' days take it in, though they owe it
// nothing, its page counting it as Projects shows again.
func TestALateLineOfTheDayGoneReachesProjectsThroughAnotherReadersRead(t *testing.T) {
	night := onTheDay(23, 59, 30)
	h := projectsHarness(t, dashboard.Projects)
	h.clock.now = night
	h.start()
	h.keys(tabKey, tabKey)
	if h.model.view != dashboard.Sessions {
		t.Fatalf("tab twice from Projects shows %s, want Sessions", h.model.view)
	}
	h.clock.now = onTheDay(24, 0, 10)
	h.read(routerDocument(three()...))
	arrival := onTheDay(23, 59, 50)
	line := took(arrivedAt("0a000009", arrival), 40*time.Second)
	at := func(kind string, when time.Time) router.StreamEvent {
		return alter(told(kind, "0a000009", "work", 0), func(e *router.StreamEvent) { e.At, e.Status = when.UTC(), 200 })
	}
	l := projectsLedgerOf(h)
	l.write(line)
	h.hear(at(router.StreamSent, arrival.Add(100*time.Millisecond)), at(router.StreamDone, line.At.Add(40*time.Second)))
	read := len(l.days)
	for _, second := range []int{35, 40, 45} {
		h.clock.now = onTheDay(24, 0, second)
		if owed := h.model.projectsFeed(h.clock.now).owed; len(owed) > 0 {
			t.Fatalf("at 00:00:%d Projects' days owe %v, want none, the day gone read whole as it turned", second, owed)
		}
		h.read(routerDocument(three()...))
	}
	if gone := dayfile.DateOf(night); !slices.Contains(l.days[read:], gone) {
		t.Fatalf("the looks after the request ended read the days %v whole, want the day gone among them, for the store", l.days[read:])
	}
	h.keys(shiftTabKey, shiftTabKey)
	sameAsFresh(t, h, 7, "the store read the day gone again for its line, then Projects showed")
}

// A read further back landing after a read of the tail keeps what the tail
// gained meanwhile: today is the tail's, never the read's.
func TestAReadFurtherBackKeepsTheLinesTheTailGaveMeanwhile(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.start()
	held := holdingBack[projectsReadMsg](h, h.press(">")...)
	if len(held) != 1 {
		t.Fatalf("a step to 30 days held %d reads, want one", len(held))
	}
	projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", h.clock.now.Add(time.Second), 1)...)
	h.tickUntilAsked()
	h.deliver(held...)
	sameAsFresh(t, h, 8, "the read over 30 days landed after the tail gained beta's third")
}

// While another tab shows, a read of the tail that gains a line leaves
// Projects' page stale, unbuilt; shown again, it's built once, of the line.
func TestProjectsHiddenIsBuiltOnShow(t *testing.T) {
	h := projectsHarness(t, dashboard.Projects)
	h.start()
	h.keys(tabKey)
	pages, fed := h.model.projects.pages, h.model.projects.fed
	projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", h.clock.now.Add(time.Second), 1)...)
	h.tickUntilAsked()
	if p := h.model.projects; p.pages != pages || p.fed == fed || p.pageOf.fed == p.fed {
		t.Fatalf("hidden, a read of the tail gaining a line built the page %d times, fed %d to %d, the page of %d; want none built, the page stale", p.pages-pages, fed, p.fed, p.pageOf.fed)
	}
	h.keys(shiftTabKey)
	if got := h.model.projects.pages; got != pages+1 {
		t.Errorf("shown again, the page was built %d times, want once", got-pages)
	}
	sameAsFresh(t, h, 7, "Projects showed again")
}
