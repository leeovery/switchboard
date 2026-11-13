package watch

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/status"
)

// The pools of these tests: Default, unfenced, with three accounts, shown
// first; and Acme, fenced, with one, or none in emptyAcme.
var (
	defaultShown = status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 2}
	acmeShown    = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Accounts: 1, Pools: 2}
	emptyAcme    = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Pools: 2}
)

// poolTurns is a dashboard's pools as P turns them in these tests: each
// pool's source, by its id, the turns asked for, each by the pool turned
// from, and what a turn fails with, where it does.
type poolTurns struct {
	pools   map[string]Pooled
	order   []string
	from    []string
	failure error
}

// turn gives what's shown of the pool after from, as Config's Turn does.
func (p *poolTurns) turn(_ context.Context, from string) (Pooled, error) {
	p.from = append(p.from, from)
	if p.failure != nil {
		return Pooled{}, p.failure
	}
	i := slices.Index(p.order, from)
	return p.pools[p.order[(i+1)%len(p.order)]], nil
}

// The accounts of these tests' pools, as the config has them: Default's
// three, and Acme's one.
var (
	defaultAccounts = config.Accounts{{ID: "work", Label: "Work", Pool: "b2n8"}, {ID: "personal", Label: "Personal", Pool: "b2n8"}, {ID: "side", Label: "Side", Pool: "b2n8"}}
	acmeAccounts    = config.Accounts{{ID: "k3f9", Label: "Acme 1", Pool: "q7mt"}}
)

// pooledHarness is a harness of the router's document of three accounts in
// Default, shown, beside Acme, whose source reads the router's document of
// k3f9 alone, or of none where acme has no accounts, P turning between them.
func pooledHarness(t *testing.T, acme status.ShownPool) (*harness, *fakeSource, *poolTurns) {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	acmeDoc, accounts := routerDocument(), config.Accounts(nil)
	if acme.Accounts > 0 {
		acmeDoc, accounts = routerDocument(account("k3f9", "Acme 1", fableWeek(0.7))), acmeAccounts
	}
	acmeSource := &fakeSource{doc: probedWithoutTheRouter(), router: &acmeDoc}
	pools := &poolTurns{order: []string{"b2n8", "q7mt"}, pools: map[string]Pooled{
		"b2n8": {Pool: defaultShown, Accounts: defaultAccounts, Source: h.source, Events: events.Empty{}},
		"q7mt": {Pool: acme, Accounts: accounts, Source: acmeSource, Events: events.Empty{}},
	}}
	cfg := h.config(Size{Width: 160, Height: 40})
	cfg.Pool, cfg.Accounts, cfg.Turn = defaultShown, defaultAccounts, pools.turn
	h.model = New(t.Context(), cfg)
	return h, acmeSource, pools
}

// titleRow is the screen's title row.
func (h *harness) titleRow() string {
	return strings.SplitN(h.view(), "\n", 2)[0]
}

// P turns to the next pool, its pools read afresh as Turn reads them: the
// dashboard reads its document, starting with a full read, as it opened,
// and names it in the title row; and P again turns back to the first.
func TestPTurnsToTheNextPoolReadingItAfresh(t *testing.T) {
	h, acme, pools := pooledHarness(t, acmeShown)
	h.start()
	if !strings.Contains(h.titleRow(), "SWITCHBOARD   Default ") {
		t.Fatalf("the title row reads %q, want Default named after the badge", h.titleRow())
	}

	h.deliver(h.press("P")...)
	if got := h.model.cfg.Pool; got.ID != "q7mt" || !slices.Equal(pools.from, []string{"b2n8"}) {
		t.Fatalf("after P, the pool shown is %q, turned from %q; want Acme, turned from Default", got.ID, pools.from)
	}
	if !slices.Equal(acme.asked, []status.Read{{Probe: true}}) {
		t.Errorf("Acme's source was asked for %+v, want a full read, as the dashboard opens", acme.asked)
	}
	if view := h.view(); !strings.Contains(h.titleRow(), "SWITCHBOARD   Acme ") || strings.Contains(view, "Personal") || strings.Contains(view, "Work") {
		t.Errorf("after P, the screen is\n%s\nwant Acme named, and its accounts alone", view)
	}
	m := h.model
	if got := []string{m.accounts.shown.Accounts.IDs()[0], m.past.page.Accounts.IDs()[0], m.plansAtMost[0].Account}; len(m.accounts.shown.Accounts) != 1 || len(m.past.page.Accounts) != 1 || len(m.plansAtMost) != 2 || !slices.Equal(got, []string{"k3f9", "k3f9", "k3f9"}) {
		t.Errorf("after P, Accounts lists %v, History %v, and the plans priced are %+v; want Acme's k3f9 alone", m.accounts.shown.Accounts.IDs(), m.past.page.Accounts.IDs(), m.plansAtMost)
	}
	h.deliver(h.press("tab")...)
	if view := h.view(); !strings.Contains(view, "Acme 1") || strings.Contains(view, "Personal") || strings.Contains(view, "Work") {
		t.Errorf("after P, Accounts is\n%s\nwant Acme 1 listed alone", view)
	}
	h.deliver(h.press("shift+tab")...)

	h.deliver(h.press("P")...)
	if got := h.model.cfg.Pool.ID; got != "b2n8" || !slices.Equal(pools.from, []string{"b2n8", "q7mt"}) {
		t.Errorf("after P again, the pool shown is %q, turned from %q; want Default again, turned from Acme", got, pools.from)
	}
}

// A turn starts the view afresh, as a router change does: no session's
// page, scroll, cursor, alert or held moment of the pool before carries
// into the next, and its request stream ends. What's the screen's and the
// user's stays: the tab shown, the Overview's sections and what each tab
// shows as the preferences keep it, the size and the preferences being
// kept.
func TestATurnStartsTheViewAfresh(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeShown)
	h.start()
	h.press("u")
	h.deliver(h.press("tab")...)
	h.model.scroll = 2
	h.model.accounts.shown.Focused, h.model.accounts.shown.Focus = true, "personal"
	h.model.session = &sessionShown{id: "d28c5e17"}
	h.model.keeping.writing = 1
	size := h.model.size

	h.deliver(h.press("P")...)
	m := h.model
	if m.view != dashboard.Accounts || m.sections.Has(dashboard.SectionUsage) || m.size != size || m.keeping.writing != 1 {
		t.Errorf("after P, the tab is %q, USAGE shown %v, the size %+v, a write of the preferences under way %d; want Accounts, USAGE hidden, %+v and 1, as they were",
			m.view, m.sections.Has(dashboard.SectionUsage), m.size, m.keeping.writing, size)
	}
	if m.session != nil || m.scroll != 0 || m.accounts.shown.Focused || m.accounts.shown.Focus != "" {
		t.Errorf("after P, a session's page open %v, scrolled %d, Compare focused %v on %q; want nothing of the pool before", m.session != nil, m.scroll, m.accounts.shown.Focused, m.accounts.shown.Focus)
	}
	if len(m.readings.looks) != 1 || m.readings.looks["k3f9"] == nil {
		t.Errorf("after P, the readings alerts compare with are of %d accounts, want Acme's k3f9 alone", len(m.readings.looks))
	}
}

// A turn ends the request stream of the pool before, and opens the next
// pool's from its own source.
func TestATurnEndsThePoolBeforesRequestStream(t *testing.T) {
	h, acme, _ := pooledHarness(t, acmeShown)
	h.start()
	for range 3 {
		h.deliver(h.press("tab")...)
	}
	opened := len(h.source.contexts)
	if h.model.view != dashboard.Sessions || opened == 0 || h.source.contexts[opened-1].Err() != nil {
		t.Fatalf("on %s, %d request streams opened, want Sessions, the last open", h.model.view, opened)
	}
	h.deliver(h.press("P")...)
	if h.source.contexts[opened-1].Err() == nil || len(h.source.contexts) != opened {
		t.Errorf("after P, Default's request stream is still open, or %d more opened, want it ended, and none opened", len(h.source.contexts)-opened)
	}
	if len(acme.contexts) != 1 || acme.contexts[0].Err() != nil {
		t.Errorf("after P, Acme's source opened %d request streams, want one, open", len(acme.contexts))
	}
}

// Nothing a command of the pool shown before sends back lands once P has
// turned to another, but what the turn keeps: a write of the preferences,
// and Bubble Tea's own messages, which pass unstamped.
func TestNothingOfThePoolBeforeLandsAfterATurn(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeShown)
	h.start()
	before := h.model.poolTurns
	stale := fetchedMsg{read: status.Read{Probe: true}, doc: routerDocument(three()...)}
	h.model.keeping.writing = 1

	tm := turns{Model: h.model}
	next, _ := tm.Update(ofTurn{turn: before, msg: turnedMsg{pooled: Pooled{Pool: acmeShown, Source: h.source, Events: events.Empty{}}}})
	tm = next.(turns)
	if tm.poolTurns == before {
		t.Fatal("the turn landed, but the pools shown weren't counted on")
	}
	doc := tm.doc
	next, _ = tm.Update(ofTurn{turn: before, msg: stale})
	tm = next.(turns)
	if len(tm.doc.Accounts) != len(doc.Accounts) {
		t.Errorf("a read of the pool before landed after the turn, its %d accounts on screen; want it dropped", len(tm.doc.Accounts))
	}
	next, _ = tm.Update(ofTurn{turn: before, msg: prefsKeptMsg{}})
	if tm = next.(turns); tm.keeping.writing != 0 {
		t.Error("a write of the preferences under way before the turn didn't land after it, want it to")
	}
	next, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if tm = next.(turns); tm.size != (Size{Width: 120, Height: 30}) {
		t.Errorf("a resize after the turn left the size %+v, want 120×30", tm.size)
	}
}

// What the watch's own commands send back is stamped with the pool shown,
// those of a batch each; Bubble Tea's own pass as they are, as it acts on
// them itself.
func TestTheWatchsOwnMessagesAreStampedWithThePoolShown(t *testing.T) {
	own := func() tea.Msg { return noteMsg{} }
	if got := stamped(3, own)(); got != (ofTurn{turn: 3, msg: noteMsg{}}) {
		t.Errorf("the watch's own message is sent back as %+v, want it stamped with turn 3", got)
	}
	if got := stamped(3, tea.Quit)(); got != (tea.QuitMsg{}) {
		t.Errorf("a quit is sent back as %+v, want it as it is, for Bubble Tea to quit", got)
	}
	batch, ok := stamped(3, tea.Batch(own, tea.Quit))().(tea.BatchMsg)
	if !ok || len(batch) != 2 || batch[0]() != (ofTurn{turn: 3, msg: noteMsg{}}) || batch[1]() != (tea.QuitMsg{}) {
		t.Error("a batch isn't sent back as a batch of its commands each stamped as it would be alone")
	}
	if stamped(3, nil) != nil {
		t.Error("no command stamped is a command, want none")
	}
}

// P while a turn is under way turns nowhere: a second turns from the pool
// the first turned to, once it lands.
func TestPTurnsOnceWhileATurnIsUnderWay(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeShown)
	h.start()
	turning := h.press("P")
	h.press("P")
	h.deliver(turning...)
	if !slices.Equal(pools.from, []string{"b2n8"}) || h.model.cfg.Pool.ID != "q7mt" {
		t.Errorf("P twice before the first landed turned from %q to %q, want one turn, from Default to Acme", pools.from, h.model.cfg.Pool.ID)
	}
}

// A turn that can't tell the pools keeps the pool shown, the key line
// saying why.
func TestATurnThatFailsKeepsThePoolShownSayingWhy(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeShown)
	pools.failure = errors.New("no config file at ~/.config/switchboard/config.toml")
	h.start()
	h.deliver(h.press("P")...)
	if h.model.cfg.Pool.ID != "b2n8" || h.model.turning {
		t.Errorf("after a turn that failed, the pool shown is %q, turning %v; want Default still, no turn under way", h.model.cfg.Pool.ID, h.model.turning)
	}
	if got, want := h.keyLine(), "can't turn to the next pool: no config file at ~/.config/switchboard/config.toml"; got != want {
		t.Errorf("the key line reads %q, want %q", got, want)
	}
}

// The key line and help list P pool once there's a second pool, after p
// pin, a phone's line keeping it; never with one pool.
func TestPPoolIsListedOnceThereIsASecondPool(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeShown)
	h.start()
	if got, want := h.keyLine(), toggles+"p pin · P pool · ? help · q quit"; got != want {
		t.Errorf("the Overview's key line reads %q, want %q", got, want)
	}
	h.deliver(h.press("tab")...)
	if got := h.keyLine(); !strings.HasSuffix(got, "p pin · P pool · ? help · q quit") {
		t.Errorf("Accounts' key line reads %q, want p pin and P pool before ? help", got)
	}
	h.press("?")
	if !slices.ContainsFunc(h.model.helpKeys(), func(k dashboard.Key) bool { return k == poolKey }) {
		t.Errorf("help lists %+v, want P pool among them", h.model.helpKeys())
	}
	h.press("?")
	h.update(tea.WindowSizeMsg{Width: 73, Height: 61})
	h.deliver(h.press("shift+tab")...)
	if got, want := h.keyLine(), "tab views · P pool · ? keys and toggles · q quit"; got != want {
		t.Errorf("a phone's Overview key line reads %q, want %q", got, want)
	}

	one := routedHarness(t, routerDocument(three()...))
	one.start()
	if got := one.keyLine(); got != threeKeys {
		t.Errorf("with one pool, the key line reads %q, want %q, no P pool", got, threeKeys)
	}
	if one.press("P"); one.model.turning {
		t.Error("with one pool, P turned, want it to do nothing")
	}
}

// A pool with no accounts lists its own keys alone, on every tab: tab
// views, P pool, ? help and q quit. p opens no routing card, nor does a
// tab's key act; tab still moves between the tabs.
func TestAPoolWithNoAccountsListsItsKeysAlone(t *testing.T) {
	h, _, _ := pooledHarness(t, emptyAcme)
	h.start()
	h.deliver(h.press("P")...)
	for range dashboard.Views() {
		if got, want := h.keyLine(), "tab views · P pool · ? help · q quit"; got != want {
			t.Errorf("on %s, the key line reads %q, want %q", h.model.view, got, want)
		}
		h.press("p")
		h.press("u")
		if h.model.open != nothingOpen || !h.model.sections.Has(dashboard.SectionUsage) {
			t.Errorf("on %s, p opened %v and u hid USAGE %v, want neither to act", h.model.view, h.model.open, !h.model.sections.Has(dashboard.SectionUsage))
		}
		was := h.model.view
		h.deliver(h.press("tab")...)
		if h.model.view == was {
			t.Errorf("on %s, tab showed it still, want the next tab", was)
		}
	}
}

// The dashboard's own notifications name the pool shown once there's a
// second, its label leading the account's title, in the idiom of their
// words with one pool.
func TestTheDashboardsNotificationsNameThePoolOnceThereIsASecond(t *testing.T) {
	h := newHarness(t, sessionAndWeek(0.2, 0.85))
	cfg := h.config(Size{Width: 150, Height: 50})
	cfg.Pool = acmeShown
	h.model = New(t.Context(), cfg)
	h.start()
	h.clock.now = h.clock.now.Add(5 * time.Minute)
	h.read(sessionAndWeek(0.2, 0.91))
	if want := []string{"Acme · work · Work: Week at 91%"}; !slices.Equal(h.notifier.posted, want) {
		t.Errorf("posted %q, want %q", h.notifier.posted, want)
	}
}
