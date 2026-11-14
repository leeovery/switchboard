package watch

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/router"
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
// days read afresh build, as freshPage builds it, counting requests in all.
func sameAsFresh(t *testing.T, h *harness, requests int, after string) {
	t.Helper()
	page, fresh := h.model.projects.shown.Page, freshPage(h)
	if page == nil {
		t.Fatalf("after %s, Projects shows no page, want one built", after)
	}
	if page.All.Requests != requests || !reflect.DeepEqual(page.Rows, fresh.Rows) {
		t.Errorf("after %s, the page counts %d requests in rows\n%+v\nwant %d, the rows of the days read afresh\n%+v", after, page.All.Requests, page.Rows, requests, fresh.Rows)
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
	h.tickUntilAsked()
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
// the tail, though no reader that folds it wants them, so the days read
// always hold their lines.
func TestTheDaysTheTailsDaysOweAreReadThoughNoReaderWantsThem(t *testing.T) {
	owed := []string{"2026-09-27", "2026-09-28"}
	if got := lateDays(owed, nil, nil, nil); !slices.Equal(got, owed) {
		t.Errorf("no reader folding the read, its late days are %v, want those owed, %v", got, owed)
	}
	if got := lateDays(owed, nil, nil, &heldFold{late: owed[1:]}); !slices.Equal(got, owed) {
		t.Errorf("the store wanting the second too, the late days are %v, want each once, %v", got, owed)
	}
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
