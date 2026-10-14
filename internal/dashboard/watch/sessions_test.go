package watch

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionsOnHarness is streamingHarness on Sessions, the preferences kept as
// prefs keeps them.
func sessionsOnHarness(t *testing.T) (*harness, *fakePrefs) {
	t.Helper()
	h := streamingHarness(t)
	h.keys(toSessions...)
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	return h, prefs
}

// callsOf are the sessions of the calls Sessions holds, as the last look
// found them, and those of the line of the account with the given id, with
// the jack each is plugged into.
func callsOf(h *harness, account string) ([]string, []views.Plugged) {
	var sessions []string
	for _, c := range h.model.sessionsTab.shown.Calls {
		sessions = append(sessions, c.Session)
	}
	return sessions, h.model.sessionsTab.shown.Patched.Line(account).Calls
}

// Each look builds Sessions afresh while it shows, its calls found and who
// sits where kept from the look before: a session gone from a line leaves
// the rest on their jacks. A frame takes nothing in, and builds nothing; nor
// does a look at another tab, or one under a session's page, Sessions
// standing as it was behind it.
func TestEachLookBuildsSessionsTheSwitchboardKeptFromTheLookBefore(t *testing.T) {
	h := streamingHarness(t)
	if calls, _ := callsOf(h, "work"); len(calls) > 0 {
		t.Errorf("on the Overview, Sessions' calls are %v, want none built", calls)
	}
	h.keys(toSessions...)
	calls, work := callsOf(h, "work")
	if want := []views.Plugged{{Session: idD28C, Jack: 0}, {Session: id7F3A, Jack: 1}}; !slices.Equal(calls, []string{idD28C, id7F3A}) || !slices.Equal(work, want) {
		t.Fatalf("Sessions shown, its calls are %v, work's line %+v; want d28c's and 7f3a's, work's line %+v", calls, work, want)
	}

	h.model.sessions = h.model.sessions[1:]
	h.update(frameMsg{gen: h.model.frames})
	if calls, _ := callsOf(h, "work"); len(calls) != 2 {
		t.Errorf("a frame on, d28c no longer listed, the calls are %v, want them as the last look found them", calls)
	}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	if calls, work := callsOf(h, "work"); !slices.Equal(calls, []string{id7F3A}) || !slices.Equal(work, []views.Plugged{{Session: id7F3A, Jack: 1}}) {
		t.Errorf("a look on, the calls are %v, work's line %+v; want 7f3a's alone, kept on jack 1", calls, work)
	}

	h.openPage(id7F3A)
	h.model.sessions = nil
	h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	if calls, _ := callsOf(h, "work"); !slices.Equal(calls, []string{id7F3A}) {
		t.Errorf("a look under 7f3a's page, the calls are %v, want them standing as they were", calls)
	}
}

// A frame builds Sessions afresh as a move the views read comes to show,
// once the refusal that moved it has bounced back along its cord, putting
// its session on its new account's line; and only then.
func TestAFrameBuildsSessionsAfreshAsAMoveComesToShow(t *testing.T) {
	h, _ := sessionsOnHarness(t)
	h.hear(movedToSide(0)...)

	for _, tt := range []struct {
		name string
		at   time.Duration
		on   string
	}{
		{name: "as its refusal bounces back", at: pulseFor - time.Millisecond, on: "work"},
		{name: "once it has", at: pulseFor, on: "side"},
		{name: "a frame on, the router listing no session since", at: pulseFor + time.Second, on: "side"},
	} {
		h.clock.now = past(tt.at)
		if tt.at > pulseFor {
			h.model.sessions = nil
		}
		h.update(frameMsg{gen: h.model.frames})
		calls, line := callsOf(h, tt.on)
		if !slices.ContainsFunc(line, func(p views.Plugged) bool { return p.Session == idD28C }) || !slices.Contains(calls, id7F3A) {
			t.Errorf("%s, the calls are %v, %s's line %+v; want 7f3a's among them, d28c on %s", tt.name, calls, tt.on, line, tt.on)
		}
	}
}

// While Sessions shows, with no session's page over it, a frame is drawn as
// a move yet to show shows; never on another tab, or under a page.
func TestAFrameIsDrawnAsAMoveYetToShowShows(t *testing.T) {
	h, _ := sessionsOnHarness(t)
	h.hear(movedToSide(0)...)
	m := h.model
	m.shown = shown{}
	if next, ok := m.nextFrame(start); !ok || next != pulseFor+stepSlack {
		t.Errorf("on Sessions, the next frame is %s on (%t), want %s, as the move shows", next, ok, pulseFor+stepSlack)
	}
	if _, ok := m.nextFrame(past(pulseFor)); ok {
		t.Error("the move shown, a frame is armed, want none")
	}
	m.view = dashboard.Overview
	if _, ok := m.nextFrame(start); ok {
		t.Error("on the Overview, a frame is armed for the move, want none")
	}
	m.view, m.session = dashboard.Sessions, &sessionShown{id: idD28C}
	if _, ok := m.nextFrame(start); ok {
		t.Error("under a session's page, a frame is armed for the move, want none")
	}
}

// sessionReadOf is the id of the session whose page the messages read, ""
// where none does.
func sessionReadOf(msgs []tea.Msg) string {
	for _, msg := range msgs {
		if read, ok := msg.(sessionReadMsg); ok {
			return read.id
		}
	}
	return ""
}

// ↑ and ↓ choose a call, enter opens its session's page, and esc ends the
// choice; enter with nothing chosen opens nothing; [ and ] turn the sub-tab,
// which the preferences keep, never the choice.
func TestSessionsKeysChooseOpenAndTurn(t *testing.T) {
	h, prefs := sessionsOnHarness(t)
	enter, opening := tea.KeyPressMsg{Code: tea.KeyEnter}, h.model.opening
	if read := sessionReadOf(h.update(enter)); read != "" || h.model.opening != opening {
		t.Errorf("enter with nothing chosen read %q's page, asked %d to open; want none", read, h.model.opening-opening)
	}
	h.keyed("down down down up")
	if got, want := h.model.sessionsTab.shown.Chosen, (dashboard.Plug{Session: idD28C, Account: "work"}); got != want {
		t.Fatalf("↓ ↓ ↓ ↑ chose %+v, want %+v", got, want)
	}
	h.keyed("down")
	chosen := dashboard.Plug{Session: id7F3A, Account: "work"}
	if got := h.model.sessionsTab.shown.Chosen; got != chosen || h.model.frame(h.clock.now).SessionsTab.Chosen != chosen {
		t.Fatalf("↓ then chose %+v, the frame %+v; want %+v", got, h.model.frame(h.clock.now).SessionsTab.Chosen, chosen)
	}
	if read := sessionReadOf(h.update(enter)); read != id7F3A {
		t.Errorf("enter read %q's page, want 7f3a's, the session chosen", read)
	}
	h.keyed("esc")
	if got := h.model.sessionsTab.shown.Chosen; got != (dashboard.Plug{}) || prefs.updates != 0 {
		t.Errorf("esc left %+v chosen, the preferences updated %d times; want none chosen, none kept", got, prefs.updates)
	}
	h.model.scroll = 3
	h.keyed("]")
	if got := h.model.sessionsTab.shown.Shown; got != dashboard.SessionsList || prefs.kept.Sessions != (theme.SessionsPrefs{View: "list"}) || prefs.updates != 1 || h.model.scroll != 0 {
		t.Errorf("] shows %d, scrolled %d, the preferences keep %+v, updated %d times; want the List from its top, kept once", got, h.model.scroll, prefs.kept.Sessions, prefs.updates)
	}
	h.keyed("[")
	if got := h.model.sessionsTab.shown.Shown; got != dashboard.SessionsSwitchboard || prefs.kept.Sessions != (theme.SessionsPrefs{View: "switchboard"}) || prefs.updates != 2 {
		t.Errorf("[ shows %d, the preferences keep %+v, updated %d times; want the Switchboard, kept again", got, prefs.kept.Sessions, prefs.updates)
	}
}

// A click on a call chooses it, and on the one chosen, opens its session's
// page; a click on a sub-tab shows it.
func TestSessionsClicksChooseThenOpen(t *testing.T) {
	h, _ := sessionsOnHarness(t)
	on := dashboard.Target{On: dashboard.OnSession, Session: idD28C, Account: "work"}
	next, _ := h.model.clickedOn(on)
	h.model = next.(Model)
	if got := h.model.sessionsTab.shown.Chosen; got != (dashboard.Plug{Session: idD28C, Account: "work"}) {
		t.Fatalf("a click on d28c's call chose %+v, want it", got)
	}
	_, cmd := h.model.clickedOn(on)
	if read := sessionReadOf(h.run(cmd)); read != idD28C {
		t.Errorf("a click on the call chosen read %q's page, want d28c's", read)
	}
	next, _ = h.model.clickedOn(dashboard.Target{On: dashboard.OnSubTab, SubTab: 1})
	if got := next.(Model).sessionsTab.shown.Shown; got != dashboard.SessionsList {
		t.Errorf("a click on List shows %d, want the List", got)
	}
}

// A watch opens Sessions as the preferences kept it.
func TestSessionsOpensAsKept(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.SessionsShown = dashboard.SessionsKept(theme.SessionsPrefs{View: "list"})
	if got := New(t.Context(), cfg).sessionsTab.shown.Shown; got != dashboard.SessionsList {
		t.Errorf("Sessions opens on %d, want the List, as kept", got)
	}
}

// sessionsCounted is a request ledger followed as it grows, which notes the
// ids each read of its sessions whole asked for.
type sessionsCounted struct {
	followed
	asked [][]string
}

func (l *sessionsCounted) Sessions(ids []string, take func(string, ledger.Line)) map[string][]ledger.Line {
	l.asked = append(l.asked, slices.Clone(ids))
	return l.Ledger.Sessions(ids, take)
}

// The List's first read waits for a read of the ledger's tail under way, and
// holds the tail's reads off till it lands; it's read the once, however many
// looks come while it's under way; and once it's held, the tail is read one
// read at a time.
func TestTheListIsReadOneReadOfTheLedgerAtATime(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute))
	l := &sessionsCounted{}
	l.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
	h.model.cfg.Ledger = l
	h.start()
	looked := func() []tea.Msg {
		h.model.tail.due = true
		return h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	}
	tailed := func(msgs []tea.Msg) bool {
		return slices.ContainsFunc(msgs, func(msg tea.Msg) bool { _, ok := msg.(tailedMsg); return ok })
	}

	h.model.tail.reading = true
	h.keys(toSessions...)
	if len(l.asked) != 0 {
		t.Fatalf("Sessions shown, a read of the tail under way, the List was read %d times, want none till it lands", len(l.asked))
	}
	h.model.tail.reading = false
	listed := looked()
	if len(l.asked) != 1 {
		t.Fatalf("the tail's read landed, a look on, the List was read %d times, want once", len(l.asked))
	}
	if again := looked(); len(l.asked) != 1 || tailed(again) {
		t.Errorf("a look on, the List's read under way, the List was read %d times, the tail read: %t; want neither again", len(l.asked), tailed(again))
	}
	h.openPage(idD28C)
	if len(l.given) > 0 {
		t.Errorf("a session's page opened, the List's read under way, the tail was read giving %v, want it held off till the List's lands", l.given)
	}
	h.deliver(listed...)
	h.model.tail.reading = true
	if again := looked(); tailed(again) {
		t.Error("the List held, a look read the tail beside a read of it under way, want one at a time")
	}
}

// The List is read whole once, as Sessions first shows, and held for the
// run, each look's read of the ledger's tail feeding it what today's lines
// gained: never read whole again, but a session it first meets there, read
// whole the once.
func TestTheListIsReadOnceAsSessionsFirstShowsThenFedByTheTail(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	l := &sessionsCounted{}
	l.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
	h.model.cfg.Ledger = l
	h.start()
	if len(l.asked) != 0 || h.model.sessionsTab.held != nil {
		t.Fatalf("on the Overview, the List was read %d times, held %t; want it unread", len(l.asked), h.model.sessionsTab.held != nil)
	}
	h.keys(toSessions...)
	requests := func() int {
		list := h.model.sessionsTab.shown.List.Sessions
		if i := slices.IndexFunc(list, func(s views.ListedSession) bool { return s.Session == idD28C }); i >= 0 {
			return list[i].Requests
		}
		return 0
	}
	if len(l.asked) != 1 || requests() != 2 {
		t.Fatalf("Sessions shown, the List was read whole %d times, d28c listed with %d requests; want once, 2", len(l.asked), requests())
	}

	met := arrived("0a000004", "side", -2*time.Minute)
	met.Session = id9E21
	writeLines(t, state, arrived("0a000003", "work", -3*time.Minute), met)
	h.read(probedWithoutTheRouter())
	if requests() != 3 || l.given[len(l.given)-1] != 2 || !slices.Equal(l.asked[len(l.asked)-1], []string{id9E21}) {
		t.Errorf("a look on, d28c is listed with %d requests, the tail's last read giving %v, the List's reads whole asking for %q; want 3, the lines gained, 9e21 read whole", requests(), l.given, l.asked)
	}
	h.read(probedWithoutTheRouter())
	if len(l.asked) != 2 {
		t.Errorf("another look on, the List was read whole %d times, want twice: 9e21 read the once", len(l.asked))
	}
	h.keys(tabKey, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if len(l.asked) != 2 || h.model.view != dashboard.Sessions {
		t.Errorf("Sessions shown again, on %s, the List was read whole %d times, want no more, held for the run", h.model.view, len(l.asked))
	}
}

// listedOf is the session with the given id as the List Sessions holds lists
// it, reporting false where it doesn't.
func listedOf(h *harness, id string) (views.ListedSession, bool) {
	list := h.model.sessionsTab.shown.List.Sessions
	if i := slices.IndexFunc(list, func(s views.ListedSession) bool { return s.Session == id }); i >= 0 {
		return list[i], true
	}
	return views.ListedSession{}, false
}

// While the router doesn't answer, the Switchboard, the List and a session's
// page alike draw the sessions its last listing ran, idle counting on, the
// owner's call of 14 October 2026: ten minutes on, d28c, whose last line
// came two hours before, has its call on work, runs on the List, and runs on
// its page as the List has it.
func TestTheRouterSilentForTenMinutesSessionsAndAPageDrawItsLastListing(t *testing.T) {
	h := streamingHarness(t)
	h.withLines(promptedLines(2, start.Add(-2*time.Hour)))
	h.keys(toSessions...)
	h.stopRouter()
	h.tickUntil(start.Add(10 * time.Minute))
	if h.model.answering() || h.model.lostShown().IsZero() {
		t.Fatal("ten minutes on, the router is taken to answer, want it lost")
	}

	if !slices.ContainsFunc(h.model.sessionsTab.shown.Calls, func(c views.Call) bool { return c.Session == idD28C && c.Account == "work" }) {
		t.Errorf("the Switchboard's calls are %+v, want d28c's on work, as the router last listed it", h.model.sessionsTab.shown.Calls)
	}
	listed, ok := listedOf(h, idD28C)
	if !ok || !listed.Running {
		t.Fatalf("the List has d28c %+v (listed %t), want it running", listed, ok)
	}
	h.openPage(idD28C)
	if page := h.shownPage().Page; !page.Running || !page.Runs() || page.Activity != listed.Activity {
		t.Errorf("its page runs %t (%t by its activity), doing %+v; want it running, as the List has it, %+v", page.Running, page.Runs(), page.Activity, listed.Activity)
	}
}
