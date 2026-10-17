package watch

import (
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
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

// ↓ and ↑ on the Switchboard each scroll it to the call they choose, where
// it's out of view: through every call, down and back up, in a terminal
// short enough that its lines scroll, with personal's calls above its line
// and twenty on side, more than the view holds at once.
func TestChoosingACallScrollsTheSwitchboardToIt(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "personal", time.Second), sessionOn(id7F3A, opus, "personal", 2*time.Second)}
	for i := range 20 {
		h.source.sessions = append(h.source.sessions, sessionOn(fmt.Sprintf("%04x0000-0000-4000-8000-000000000000", i), opus, "side", time.Duration(i+3)*time.Second))
	}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 14})
	h.start()
	h.keys(toSessions...)
	shows := func() bool {
		chosen := h.model.sessionsTab.shown.Chosen
		return slices.ContainsFunc(h.targets(), func(t dashboard.Target) bool {
			return t.On == dashboard.OnSession && t.Session == chosen.Session && t.Account == chosen.Account
		})
	}
	furthest := 0
	for _, key := range append(slices.Repeat([]tea.KeyPressMsg{downKey}, 22), slices.Repeat([]tea.KeyPressMsg{upKey}, 21)...) {
		h.keys(key)
		if !shows() {
			t.Fatalf("%s chose %+v, the view scrolled %d, want it scrolled to show it", key, h.model.sessionsTab.shown.Chosen, h.model.scroll)
		}
		furthest = max(furthest, h.model.scroll)
	}
	if furthest == 0 {
		t.Error("the view never scrolled, want the calls past its foot to have scrolled it")
	}
}

// Where every account's lines fit the view, though not all at once, ↓ and ↑
// on the Switchboard scroll it an account at a time to the call they choose:
// ten accounts, two calls each, through every call, down and back up.
func TestChoosingACallScrollsTheSwitchboardAnAccountAtATimeToIt(t *testing.T) {
	var accounts []status.Account
	for i := range 10 {
		id := fmt.Sprintf("acct-%d", i+1)
		accounts = append(accounts, account(id, id, session(0.25, 3*time.Hour), week(0.5)))
	}
	h := routedHarness(t, routerDocument(accounts...))
	for i, a := range accounts {
		for j := range 2 {
			h.source.sessions = append(h.source.sessions, sessionOn(fmt.Sprintf("%02x%02x0000-0000-4000-8000-000000000000", i, j), opus, a.ID, time.Duration(2*i+j+1)*time.Second))
		}
	}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 20})
	h.start()
	h.keys(toSessions...)
	if f, doc, at := h.model.screen(h.clock.now); f.ScrollsRows(doc, at) {
		t.Fatal("the Switchboard scrolls a row at a time, want it an account at a time, every account's lines fitting the view")
	}
	shows := func() bool {
		chosen := h.model.sessionsTab.shown.Chosen
		return slices.ContainsFunc(h.targets(), func(t dashboard.Target) bool {
			return t.On == dashboard.OnSession && t.Session == chosen.Session && t.Account == chosen.Account
		})
	}
	furthest := 0
	for _, key := range append(slices.Repeat([]tea.KeyPressMsg{downKey}, 20), slices.Repeat([]tea.KeyPressMsg{upKey}, 19)...) {
		h.keys(key)
		if !shows() {
			t.Fatalf("%s chose %+v, the view scrolled %d, want it scrolled to show it", key, h.model.sessionsTab.shown.Chosen, h.model.scroll)
		}
		furthest = max(furthest, h.model.scroll)
	}
	if furthest == 0 {
		t.Error("the view never scrolled, want the accounts past its foot to have scrolled it")
	}
}

// While the router doesn't answer, the Switchboard's calls are its last
// listing's, what they had in flight as last known, never at work; and
// once it answers again, as it lists them. The frame every other view's
// sessions are listed of is unlisted by the one value the calls are.
func TestWhileTheRouterIsSilentACallsRequestInFlightIsAsLastKnown(t *testing.T) {
	h := streamingHarness(t)
	h.source.sessions[0].Assignments[0].InFlight = status.Answering
	h.keys(toSessions...)
	d28c := func() views.Activity {
		i := slices.IndexFunc(h.model.sessionsTab.shown.Calls, func(c views.Call) bool { return c.Session == idD28C })
		if i < 0 {
			t.Fatal("d28c has no call, want the one its last listing gives")
		}
		a := h.model.sessionsTab.shown.Calls[i].Activity
		if unlisted := h.model.frame(h.clock.now).Unlisted; unlisted != a.Stale {
			t.Errorf("the frame is unlisted %t, want %t, as d28c's call is as last known", unlisted, a.Stale)
		}
		return a
	}
	if a := d28c(); a.Doing != views.Answering || a.Stale || !a.Works() {
		t.Fatalf("the router answering, d28c is %+v, want it answering, at work", a)
	}
	h.stopRouter()
	h.tickUntilAsked()
	if a := d28c(); a.Doing != views.Answering || !a.Stale || a.Works() {
		t.Errorf("the router silent, d28c is %+v, want it answering as last known, not at work", a)
	}
	h.startRouter(routerDocument(three()...))
	h.source.sessions[0].Assignments[0].InFlight = ""
	h.tickUntilAsked()
	if a := d28c(); a.Doing != views.Idle || a.Stale {
		t.Errorf("the router answering again, d28c is %+v, want it idle as listed", a)
	}
}

// The choice follows its session from look to look: on the Switchboard, to
// its row on another account as the router lists it there, and ended once
// the router lists it no more, but kept while the router doesn't answer, as
// its last listing is drawn; on the List, kept on its row as its session
// ends, the row going among those ended today.
func TestSessionsChoiceFollowsItsSessionAtEachLook(t *testing.T) {
	tests := []struct {
		name string
		list bool
		then func(h *harness)
		want dashboard.Plug
	}{
		{
			name: "the router listing it on side", then: func(h *harness) { h.source.sessions[0] = sessionOn(idD28C, opus, "side", 0) },
			want: dashboard.Plug{Session: idD28C, Account: "side"},
		},
		{name: "the router listing it no more", then: func(h *harness) { h.source.sessions = h.source.sessions[1:] }},
		{name: "the router not answering", then: (*harness).stopRouter, want: dashboard.Plug{Session: idD28C, Account: "work"}},
		{
			name: "on the List, the router listing it no more", list: true, then: func(h *harness) { h.source.sessions = h.source.sessions[1:] },
			want: dashboard.Plug{Session: idD28C},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := streamingHarness(t)
			h.withLines(promptedLines(2, start.Add(-10*time.Minute)))
			h.keys(toSessions...)
			if tt.list {
				h.keyed("]")
			}
			h.keyed("down")
			if chosen := h.model.sessionsTab.shown.Chosen; chosen.Session != idD28C {
				t.Fatalf("↓ chose %+v, want d28c", chosen)
			}
			tt.then(h)
			h.tickUntilAsked()
			if got := h.model.sessionsTab.shown.Chosen; got != tt.want {
				t.Errorf("a look on, %+v is chosen, want %+v", got, tt.want)
			}
		})
	}
}

// A view keeps the row chosen in view, scrolling no further than it must:
// where it is, where the row shows there; else to the nearest offset that
// shows it, up or down; and where none does, as with nothing chosen, where it
// is, however far that is.
func TestTheViewScrollsNoFurtherThanShowsTheRowChosen(t *testing.T) {
	const page, most = 10, 20
	tests := []struct {
		name    string
		at, row int
		want    int
	}{
		{name: "in view", at: 5, row: 9, want: 5},
		{name: "in view, at the view's foot", at: 5, row: 14, want: 5},
		{name: "a row under the view", at: 5, row: 15, want: 6},
		{name: "rows under the view", at: 0, row: 27, want: 18},
		{name: "the last row, from the top", at: 0, row: 29, want: 20},
		{name: "rows over the view", at: 12, row: 3, want: 3},
		{name: "scrolled past where the view goes", at: 25, row: 29, want: 20},
		{name: "a row nowhere", at: 7, row: -1, want: 7},
		{name: "a row nowhere, past where the view goes", at: 25, row: -1, want: 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shows := func(offset int) bool { return tt.row >= offset && tt.row < offset+page }
			if got := revealed(tt.at, dashboard.Scrolling{Most: most, Page: page}, shows); got != tt.want {
				t.Errorf("revealed(%d) = %d, want %d", tt.at, got, tt.want)
			}
		})
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
// ids each read of today's sessions whole asked for, and the ids of those
// it read whole alone.
type sessionsCounted struct {
	followed
	asked [][]string
	alone []string
}

func (l *sessionsCounted) Sessions(ids []string, take func(string, ledger.Line)) map[string][]ledger.Line {
	l.asked = append(l.asked, slices.Clone(ids))
	return l.Ledger.Sessions(ids, take)
}

func (l *sessionsCounted) SessionsOf(ids []string, take func(string, ledger.Line)) map[string][]ledger.Line {
	l.alone = append(l.alone, ids...)
	return l.Ledger.SessionsOf(ids, take)
}

func (l *sessionsCounted) Session(id string) iter.Seq[ledger.Held] {
	l.alone = append(l.alone, id)
	return l.Ledger.Session(id)
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
// whole the once, alone, not every one of today's again.
func TestTheListIsReadOnceAsSessionsFirstShowsThenFedByTheTail(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -5*time.Minute), arrived("0a000002", "side", -4*time.Minute))
	l := &sessionsCounted{}
	l.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
	h.model.cfg.Ledger = l
	h.start()
	if len(l.asked) != 0 || h.model.sessionsTab.listed {
		t.Fatalf("on the Overview, the List was read %d times, held %t; want it unread", len(l.asked), h.model.sessionsTab.listed)
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
	if requests() != 3 || l.given[len(l.given)-1] != 2 || len(l.asked) != 1 || !slices.Equal(l.alone, []string{id9E21}) {
		t.Errorf("a look on, d28c is listed with %d requests, the tail's last read giving %v, the List read whole %d times, sessions read alone %q; want 3, the lines gained, once, 9e21 alone", requests(), l.given, len(l.asked), l.alone)
	}
	h.read(probedWithoutTheRouter())
	if len(l.asked) != 1 || len(l.alone) != 1 {
		t.Errorf("another look on, the List was read whole %d times, sessions alone %q; want once, 9e21 the once", len(l.asked), l.alone)
	}
	h.keys(tabKey, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if len(l.asked) != 1 || len(l.alone) != 1 || h.model.view != dashboard.Sessions {
		t.Errorf("Sessions shown again, on %s, the List was read whole %d times, sessions alone %q, want no more, held for the run", h.model.view, len(l.asked), l.alone)
	}
}

// Every view naming the sessions running names each as the List held does,
// by its main thread's lines first, so the List is read as the first of
// them shows: SESSIONS on the Overview, Accounts' Detail or Sessions. Till
// then, a session is named by the model the router saw it on last.
func TestAViewNamingTheSessionsRunningReadsTheListAndNamesThemByIt(t *testing.T) {
	tests := []struct {
		name string
		show func(*harness)
	}{
		{name: "SESSIONS on the Overview", show: func(h *harness) { h.keyed("s") }},
		{name: "Accounts' Detail", show: func(h *harness) {
			h.model.view, h.model.accounts.shown = dashboard.Accounts, dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Accounts: threeConfigured}
			h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 40})...)
		}},
		{name: "Sessions", show: func(h *harness) { h.keys(toSessions...) }},
	}
	seat := func(model string, seen time.Duration) status.Assignment {
		return status.Assignment{
			Model: model, Family: strings.Split(model, "-")[1], Account: "work", Reason: "new",
			AssignedAt: start.Add(-time.Hour).UTC(), LastSeen: start.Add(-seen).UTC(),
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, four())
			h.source.sessions = []status.Session{{ID: idD28C, Assignments: []status.Assignment{seat(opus, 2*time.Minute), seat(haiku, 10*time.Second)}}}
			l := h.withLines(promptedLines(1, start.Add(-30*time.Minute)))
			h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
			h.start()
			if r := d28cRunning(t, h, h.clock.now); len(l.asked) != 0 || r.Model != haiku {
				t.Fatalf("on the Overview, SESSIONS hidden, the List was read %d times, d28c named %s; want it unread, d28c named by the model the router saw it on last, %s", len(l.asked), r.Model, haiku)
			}
			tt.show(h)
			if r := d28cRunning(t, h, h.clock.now); len(l.asked) != 1 || r.Model != opus || r.Family != "opus" {
				t.Errorf("shown, the List was read %d times, d28c named %s, of %s; want it read once, d28c named by its main thread's lines, %s", len(l.asked), r.Model, r.Family, opus)
			}
		})
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

// linesOf are the lines given, each of the session with the given id.
func linesOf(id string, lines []ledger.Line) []ledger.Line {
	for i := range lines {
		lines[i].Session = id
	}
	return lines
}

// Probing, no router lists a session: the Switchboard has no calls, and the
// List comes from the ledger alone, a session idle within the router's hour
// since its last request, and one past it ended.
func TestProbingSessionsHasNoCallsAndTheListComesFromTheLedger(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.withLines(slices.Concat(promptedLines(2, start.Add(-30*time.Minute)), linesOf(id9E21, promptedLines(1, start.Add(-3*time.Hour)))))
	h.start()
	h.keys(toSessions...)

	if calls := h.model.sessionsTab.shown.Calls; len(calls) > 0 {
		t.Errorf("probing, the Switchboard's calls are %+v, want none", calls)
	}
	for _, tt := range []struct {
		id    string
		ended bool
	}{{id: idD28C}, {id: id9E21, ended: true}} {
		listed, ok := listedOf(h, tt.id)
		if !ok || listed.Running || (listed.Activity.Doing == views.Ended) != tt.ended {
			t.Errorf("the List has %s %+v (listed %t), want it listed, not running, ended %t", tt.id, listed, ok, tt.ended)
		}
	}
}

// A router restarted, or another, has the watch forget what the last one's
// stream told, but who sits where on the Switchboard is kept: the calls the
// new one lists keep their jacks, however it orders them.
func TestARouterRestartedKeepsWhoSitsWhere(t *testing.T) {
	h := streamingHarness(t)
	h.source.health = router.Health{OK: true, PID: 100, StartedAt: start.UTC()}
	h.tickUntilAsked()
	h.keys(toSessions...)
	want := []views.Plugged{{Session: idD28C, Jack: 0}, {Session: id7F3A, Jack: 1}}
	if _, work := callsOf(h, "work"); !slices.Equal(work, want) {
		t.Fatalf("work's line is %+v, want %+v", work, want)
	}

	h.source.health = router.Health{OK: true, PID: 200, StartedAt: start.Add(time.Minute).UTC()}
	h.source.sessions = []status.Session{sessionOn(id7F3A, haiku, "work", -time.Minute), sessionOn(idD28C, opus, "work", -10*time.Second)}
	h.tickUntilAsked()
	if _, work := callsOf(h, "work"); !slices.Equal(work, want) {
		t.Errorf("another router answering, 7f3a seen last, work's line is %+v, want it kept, %+v", work, want)
	}
}

// endedToday are n sessions that ended today, each a request two hours or
// more before the clock starts, a minute apart.
func endedToday(n int) []ledger.Line {
	var lines []ledger.Line
	for i := range n {
		lines = append(lines, linesOf(fmt.Sprintf("%08x-5e55-4000-8000-000000000000", i+1), promptedLines(1, start.Add(-2*time.Hour-time.Duration(i)*time.Minute)))...)
	}
	return lines
}

// enter on the row chosen opens its session's page over Sessions, which
// stands as it was behind it, and esc comes back to it as it was left: its
// sub-tab, the row chosen, and how far it's scrolled to show it.
func TestEscFromASessionsPageComesBackToSessionsAsItWasLeft(t *testing.T) {
	h := streamingHarness(t)
	h.withLines(endedToday(30))
	h.update(tea.WindowSizeMsg{Width: 160, Height: 20})
	h.keys(toSessions...)
	h.keyed("] up")
	left, scroll := h.model.sessionsTab.shown, h.model.scroll
	if left.Shown != dashboard.SessionsList || left.Chosen == (dashboard.Plug{}) {
		t.Fatalf("] ↑ shows %d, %+v chosen; want the List, its last row chosen", left.Shown, left.Chosen)
	}

	h.keyed("enter")
	if h.model.session == nil || h.model.session.id != left.Chosen.Session {
		t.Fatalf("enter opened %+v, want the page of %s, the session chosen", h.model.session, left.Chosen.Session)
	}
	h.tickUntilAsked()
	h.keyed("esc")
	back := h.model.sessionsTab.shown
	if h.model.session != nil || h.model.view != dashboard.Sessions || back.Shown != left.Shown || back.Chosen != left.Chosen || h.model.scroll != scroll {
		t.Errorf("esc shows %s, page open %t, sub-tab %d, %+v chosen, scrolled %d; want Sessions as left: %d, %+v, %d",
			h.model.view, h.model.session != nil, back.Shown, back.Chosen, h.model.scroll, left.Shown, left.Chosen, scroll)
	}
}

// sessionsOnTheLedger is a model of the router of three, listing d28c on
// work, reading the request ledger in the state directory given, as
// daysCounted counts its reads, started at the time given on Sessions, the
// List read and held, the Log never shown.
func sessionsOnTheLedger(t *testing.T, state string, now time.Time) (*harness, *daysCounted) {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 0)}
	l := onTheLog(h, state)
	h.model.view = dashboard.Overview
	h.clock.now = now
	h.start()
	h.keys(toSessions...)
	if !h.model.sessionsTab.listed {
		t.Fatal("Sessions shown, the List isn't held")
	}
	return h, l
}

// A read of the ledger's tail landing on a store holding no session, as on
// a morning before any request, keeps it priced: a session it takes in
// after is worth what the List built afresh says.
func TestATailReadLandingOnAnEmptyStoreKeepsItPriced(t *testing.T) {
	state := t.TempDir()
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions, h.clock.now = nil, onTheDay(9, 0, 0)
	l := &daysCounted{}
	l.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Ledger, cfg.View = l, dashboard.Overview
	h.model = New(t.Context(), cfg)
	h.start()
	h.keys(toSessions...)
	for range 2 {
		h.clock.now = h.clock.now.Add(time.Second)
		h.read(routerDocument(three()...))
	}
	if n := h.model.held.Len(); n != 0 {
		t.Fatalf("the store holds %d sessions, want none, the ledger holding no line", n)
	}

	line := arrivedAt("0a000001", h.clock.now)
	line.Usage = json.RawMessage(hourUsage)
	writeLines(t, state, line)
	h.clock.now = h.clock.now.Add(time.Second)
	h.read(routerDocument(three()...))
	got, want := h.model.sessionsTab.shown.List, builtAfresh(h, l)
	if len(want.Sessions) == 0 || want.Sessions[0].Amount == nil {
		t.Fatalf("the List built afresh is %+v, want a session worth what it cost", want)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the List is\n%+v\nwant it as built afresh of every line, priced\n%+v", got, want)
	}
}

// builtAfresh is the List built afresh of every line the ledger l holds, at
// the clock's time, of what h's model reads beside them, as the List held
// must be.
func builtAfresh(h *harness, l views.Ledger) views.SessionList {
	now := h.clock.now
	return views.ListSessions(views.SessionSources{
		Running: h.model.sessions, Unlisted: !h.model.answering(), Stream: h.model.traffic.stream(now), Ledger: l, Prices: h.model.cfg.Prices, Now: now,
	})
}

// Past midnight, the List held takes in what the day before gained after the
// ledger's tail last read it, as a session's page does: the day read again
// whole at the first look past the turn, which rolls the List on to the new
// day, so no look after prices it afresh; then again as the stream tells a
// request across midnight ended, its line filed under the day it arrived on.
// The List is then the one built afresh of every line.
func TestTheListTakesInTheDayBeforeAsTheDayTurns(t *testing.T) {
	state := t.TempDir()
	night := onTheDay(23, 59, 30)
	writeLines(t, state, arrivedAt("0a000001", night.Add(-10*time.Minute)))
	h, l := sessionsOnTheLedger(t, state, night)

	writeLines(t, state, arrivedAt("0a000003", night.Add(5*time.Second)))
	arrival := night.Add(20 * time.Second)
	sent := func(kind string, when time.Time) router.StreamEvent {
		return alter(told(kind, "0a000002", "work", 0), func(e *router.StreamEvent) { e.At = when.UTC() })
	}
	h.hear(sent(router.StreamSent, arrival.Add(100*time.Millisecond)))
	h.clock.now = night.Add(35 * time.Second)
	h.read(routerDocument(three()...))
	if day := h.model.held.Holding().Day; day != views.Today(h.clock.now).Date {
		t.Errorf("the first look past midnight left the store held at %s, want it rolled on to the new day", day)
	}

	line := took(arrivedAt("0a000002", arrival), 22*time.Second)
	writeLines(t, state, line)
	h.hear(alter(sent(router.StreamDone, line.At.Add(22*time.Second)), func(e *router.StreamEvent) { e.Status = 200 }))
	h.clock.now = night.Add(45 * time.Second)
	h.read(routerDocument(three()...))

	yesterday := dayfile.DateOf(night)
	if want := []string{yesterday, yesterday}; !slices.Equal(l.days, want) {
		t.Errorf("the days %v were read whole, want %v: the day before at the turn, then as the stream told its request ended", l.days, want)
	}
	if got, want := h.model.sessionsTab.shown.List, builtAfresh(h, l); !reflect.DeepEqual(got, want) {
		t.Errorf("the List is\n%+v\nwant it as built afresh of every line\n%+v", got, want)
	}
}

// The List held reads a day it awaits a line on again whole only as its
// line could have come, as the Log and a session's page do: at the look
// after a request whose line is filed on it ended, then once a minute while
// none comes; never at every look.
func TestTheListReadsADayAwaitingALineOnlyAsItsLineCouldHaveCome(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, arrivedAt("0a000001", onTheDay(23, 50, 0)))
	h, l := sessionsOnTheLedger(t, state, onTheDay(24, 0, 30))

	at := func(kind, request string, when time.Time) router.StreamEvent {
		return alter(told(kind, request, "work", 0), func(e *router.StreamEvent) { e.At = when.UTC() })
	}
	sent := onTheDay(23, 59, 50)
	h.hear(at(router.StreamSent, "0a000003", sent), at(router.StreamSent, "0a000004", sent))
	opened, yesterday := len(l.days), dayfile.DateOf(sent)
	steps := []struct {
		name     string
		ends     string
		from, to time.Time
		readings int
	}{
		{name: "the looks after the first ended", ends: "0a000003", from: onTheDay(24, 0, 45), to: onTheDay(24, 1, 10), readings: 1},
		{name: "the look after the second ended", ends: "0a000004", from: onTheDay(24, 1, 15), to: onTheDay(24, 1, 15), readings: 2},
		{name: "the looks of the minute after", from: onTheDay(24, 1, 20), to: onTheDay(24, 2, 10), readings: 2},
		{name: "a minute on", from: onTheDay(24, 2, 15), to: onTheDay(24, 2, 15), readings: 3},
	}
	for _, s := range steps {
		if s.ends != "" {
			h.hear(alter(at(router.StreamDone, s.ends, s.from.Add(-3*time.Second)), func(e *router.StreamEvent) { e.Status = 200 }))
		}
		for h.clock.now = s.from; !h.clock.now.After(s.to); h.clock.now = h.clock.now.Add(lookEvery) {
			h.read(routerDocument(three()...))
		}
		if got := l.days[opened:]; !slices.Equal(got, slices.Repeat([]string{yesterday}, s.readings)) {
			t.Errorf("after %s, the days %v were read whole, want the day before %d times", s.name, got, s.readings)
		}
	}
}

// A look days after the ledger's tail was last read, as after the machine
// slept, has the List held read again whole each day it missed, from the
// day it's held at, each once, the oldest first, rolling it on to today.
func TestTheListReadsTheDaysTheTailMissedOnce(t *testing.T) {
	state := t.TempDir()
	night := onTheDay(22, 0, 0)
	writeLines(t, state, arrivedAt("0a000001", night.Add(-10*time.Minute)))
	h, l := sessionsOnTheLedger(t, state, night)

	h.clock.now = night.AddDate(0, 0, 3)
	h.read(routerDocument(three()...))
	h.read(routerDocument(three()...))
	want := []string{dayfile.DateOf(night), dayfile.DateOf(night.AddDate(0, 0, 1)), dayfile.DateOf(night.AddDate(0, 0, 2))}
	if !slices.Equal(l.days, want) {
		t.Errorf("the days %v were read whole, want %v, each missed once", l.days, want)
	}
	if day := h.model.held.Holding().Day; day != views.Today(h.clock.now).Date {
		t.Errorf("the store is held at %s, a day missed, want it rolled on to today", day)
	}
}

// A reader's read of its own taken into the store while a read of the
// ledger's tail is under way, as heldWith takes it, is held once the read
// lands; a session the read let go, held as it began, nobody wanting it, is
// not, though the reader's read held it too; and a session new to both, as
// the read met it, is as the read fed it.
func TestASessionTakenIntoTheStoreAsTheTailIsReadIsHeldOnceTheReadLands(t *testing.T) {
	const fresh = "4c6b2e90-1f3d-4a57-9b8e-d5a0c7e3f612"
	state := t.TempDir()
	now := onTheDay(12, 0, 0)
	of := func(session, request string, at time.Time) ledger.Held {
		l := arrivedAt(request, at)
		l.Session = session
		return l
	}
	yesterday := now.AddDate(0, 0, -1)
	writeLines(t, state, arrivedAt("0a000001", now.Add(-10*time.Minute)), of(id7F3A, "0a000002", yesterday), of(id9E21, "0a000003", yesterday))
	h, l := sessionsOnTheLedger(t, state, now)
	readOf := func(ids ...string) views.HeldSessions {
		held, _ := views.HoldSessions(h.model.cfg.Prices).Fed(l, views.Taken{}, views.SessionsWanted{Log: ids}, h.clock.now)
		return held
	}
	h.model = h.model.heldWith(readOf(id7F3A))

	read := h.readingTheTail()
	writeLines(t, state, of(fresh, "0a000004", now.Add(-2*time.Minute)))
	h.model = h.model.heldWith(readOf(id9E21, id7F3A, fresh))
	writeLines(t, state, of(fresh, "0a000005", now.Add(-time.Minute)))
	h.deliver(h.run(read)...)

	held := slices.Sorted(maps.Keys(h.model.held.Holding().Open))
	if !slices.Contains(held, id9E21) {
		t.Errorf("the store holds %v, want %s, taken in as the read was under way", held, id9E21)
	}
	if slices.Contains(held, id7F3A) {
		t.Errorf("the store holds %v, want %s let go, as the read let it go", held, id7F3A)
	}
	list := h.model.held.List(h.model.listLive(h.clock.now, h.model.traffic.stream(h.clock.now)))
	if !slices.ContainsFunc(list.Sessions, func(s views.ListedSession) bool { return s.Session == fresh && s.Requests == 2 }) {
		t.Errorf("the List holds %+v, want %s with both its requests, as the read fed it", list.Sessions, fresh)
	}
}
