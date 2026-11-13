package watch

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// The pools of these tests: Default, unfenced, with three accounts, shown
// first; and Acme, fenced, with one, or none in emptyAcme.
var (
	defaultShown = status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 2}
	acmeShown    = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Accounts: 1, Pools: 2}
	emptyAcme    = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Pools: 2}
)

// poolTurns is a dashboard's pools as the config has them in these tests:
// each pool's source, by its id, in the config's order; the turns P asked
// for, each by the pool turned from, and the times the pool shown was read
// again; what a turn fails with, where it does; and how the config file
// stands, as Config's ConfigStamp says it.
type poolTurns struct {
	pools   map[string]Pooled
	order   []string
	from    []string
	again   int
	failure error
	stamp   string
}

// turn gives what's shown of the pool asked for from from, as Config's Turn
// does: the pool after it, or it again, else, where it's gone, the first.
func (p *poolTurns) turn(_ context.Context, from string, ask PoolAsk) (Pooled, error) {
	if ask == SamePool {
		p.again++
	} else {
		p.from = append(p.from, from)
	}
	if p.failure != nil {
		return Pooled{}, p.failure
	}
	i := slices.Index(p.order, from)
	if ask == SamePool && i >= 0 {
		return p.stamped(p.pools[from]), nil
	}
	return p.stamped(p.pools[p.order[(i+1)%len(p.order)]]), nil
}

// stamped is pooled as read of the config as it stands.
func (p *poolTurns) stamped(pooled Pooled) Pooled {
	pooled.Stamp = p.stamp
	return pooled
}

// configStamp says how the config stands, as Config's ConfigStamp does.
func (p *poolTurns) configStamp() string {
	return p.stamp
}

// ofPool is doc, the router's document of the pool with the given id.
func ofPool(doc status.Document, id string) status.Document {
	doc.PoolID = id
	return doc
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
	acmeDoc, accounts := ofPool(routerDocument(), "q7mt"), config.Accounts(nil)
	if acme.Accounts > 0 {
		acmeDoc, accounts = ofPool(routerDocument(account("k3f9", "Acme 1", fableWeek(0.7))), "q7mt"), acmeAccounts
	}
	acmeSource := &fakeSource{doc: probedWithoutTheRouter(), router: &acmeDoc}
	pools := &poolTurns{order: []string{"b2n8", "q7mt"}, stamp: "1", pools: map[string]Pooled{
		"b2n8": {Pool: defaultShown, Accounts: defaultAccounts, Source: h.source, Events: events.Empty{}},
		"q7mt": {Pool: acme, Accounts: accounts, Source: acmeSource, Events: events.Empty{}},
	}}
	cfg := h.config(Size{Width: 160, Height: 40})
	cfg.Pool, cfg.Accounts, cfg.Turn = defaultShown, defaultAccounts, pools.turn
	cfg.ConfigStamp, cfg.Stamp = pools.configStamp, pools.stamp
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
// page, scroll, cursor, routing card, alert, held moment or held session
// of the pool before carries into the next, and its request stream ends.
// What's the screen's and the user's stays: the tab shown, the Overview's
// sections and what each tab shows as the preferences keep it, the size
// and the preferences being kept.
func TestATurnStartsTheViewAfresh(t *testing.T) {
	state := t.TempDir()
	held, _ := openedOn(t, state, dashboard.Graphs, arrivedAt("0a000001", onTheDay(23, 40, 0)))
	held.clock.now = onTheDay(23, 50, 0)
	held.openPage(idD28C)
	held.model.tail.due = true
	held.deliver(held.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
	if held.model.held.Len() == 0 {
		t.Fatal("the store to carry into the turn holds no session, want one")
	}

	h, _, _ := pooledHarness(t, acmeShown)
	h.start()
	h.press("u")
	h.deliver(h.press("tab")...)
	h.model.scroll = 2
	h.model.accounts.shown.Focused, h.model.accounts.shown.Focus = true, "personal"
	h.model.keeping.writing = 1
	h.model.held = held.model.held
	size := h.model.size

	turning := h.press("P")
	h.press("p")
	if h.model.open != routingOpen || h.model.page.doc.GeneratedAt.IsZero() {
		t.Fatalf("p while the turn is under way opened %v, holding the page of %v; want the routing card over the page held", h.model.open, h.model.page.doc.GeneratedAt)
	}
	h.model.session = &sessionShown{id: "d28c5e17"}
	h.deliver(turning...)
	m := h.model
	if m.open != nothingOpen || !m.page.doc.GeneratedAt.IsZero() || m.held.Len() != 0 {
		t.Errorf("after P, the card open is %v, the page held of %v, the store holding %d sessions; want no card, no page held, no session", m.open, m.page.doc.GeneratedAt, m.held.Len())
	}
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

// Nothing of the pool before lands after a turn even where its counters
// match the fresh model's, as they start where the pool before's started:
// a tick, the request stream opened, and what it told. Each lands from the
// pool shown now.
func TestNothingOfThePoolBeforeLandsWhereItsCountersMatchTheFreshModels(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeShown)
	h.start()
	before := h.model.poolTurns
	tm := turns{Model: h.model}
	deliver := func(msgs ...tea.Msg) {
		for ; len(msgs) > 0; msgs = msgs[1:] {
			next, cmd := tm.Update(msgs[0])
			tm = next.(turns)
			msgs = append(msgs, h.run(cmd)...)
		}
	}
	deliver(ofTurn{turn: before, msg: turnedMsg{pooled: Pooled{Pool: acmeShown, Accounts: acmeAccounts, Source: h.source, Events: events.Empty{}}}})
	stream := make(chan router.StreamEvent)
	sent := router.StreamEvent{At: h.clock.now, Kind: router.StreamSent, Request: "r1", Attempt: 1, Session: idD28C, Model: "claude-opus-5-5", Account: "k3f9"}
	tests := []struct {
		name   string
		msg    func(m Model) tea.Msg
		landed func(m Model, timers, waits int) bool
	}{
		{
			name:   "a tick of the fresh model's chain",
			msg:    func(m Model) tea.Msg { return tickMsg{chain: m.chain} },
			landed: func(_ Model, timers, _ int) bool { return len(h.timers) > timers },
		},
		{
			name:   "the request stream opened, of the fresh model's generation",
			msg:    func(m Model) tea.Msg { return streamOpenedMsg{gen: m.stream.gen, events: stream} },
			landed: func(m Model, _, _ int) bool { return m.stream.events == stream },
		},
		{
			name:   "what the request stream told, of the fresh model's generation",
			msg:    func(m Model) tea.Msg { return streamHeardMsg{gen: m.stream.gen, events: []router.StreamEvent{sent}} },
			landed: func(_ Model, _, waits int) bool { return len(h.waits) > waits },
		},
	}
	for _, tt := range tests {
		timers, waits := len(h.timers), len(h.waits)
		next, cmd := tm.Update(ofTurn{turn: before, msg: tt.msg(tm.Model)})
		tm = next.(turns)
		h.run(cmd)
		if tt.landed(tm.Model, timers, waits) {
			t.Errorf("%s, of the pool before, landed after the turn, want it dropped", tt.name)
		}
		next, cmd = tm.Update(ofTurn{turn: tm.poolTurns, msg: tt.msg(tm.Model)})
		tm = next.(turns)
		h.run(cmd)
		if !tt.landed(tm.Model, timers, waits) {
			t.Errorf("%s, of the pool shown, didn't land, want it to", tt.name)
		}
	}
}

// The dashboard runs its model as turns runs it, so what a command of the
// pool shown before sends back once P has turned never lands: a read of
// Default under way as P turns to Acme, landing after Acme's own, leaves
// Acme's document on screen.
func TestTheDashboardRunsItsModelStampedWithThePoolShown(t *testing.T) {
	t.Setenv("TEA_TRACE", "")
	t.Setenv("TEA_DEBUG", "")
	release, acmeRead, staleCame := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var acmeOnce, staleOnce sync.Once
	acmeDoc := routerDocument(account("k3f9", "Acme 1", fableWeek(0.7)))
	cfg := noColourConfig()
	cfg.Source = heldSource{fakeSource: &fakeSource{}, doc: routerDocument(three()...), release: release}
	cfg.Pool, cfg.Accounts, cfg.Events = defaultShown, defaultAccounts, events.Empty{}
	cfg.Turn = func(context.Context, string, PoolAsk) (Pooled, error) {
		return Pooled{Pool: acmeShown, Accounts: acmeAccounts, Source: &fakeSource{router: &acmeDoc}, Events: events.Empty{}}, nil
	}
	filter := func(_ tea.Model, msg tea.Msg) tea.Msg {
		inner := msg
		if s, ok := msg.(ofTurn); ok {
			inner = s.msg
		}
		if read, ok := inner.(fetchedMsg); ok && len(read.doc.Accounts) > 0 {
			if read.doc.Accounts[0].ID == "k3f9" {
				acmeOnce.Do(func() { close(acmeRead) })
			} else {
				staleOnce.Do(func() { close(staleCame) })
			}
		}
		return msg
	}
	in, typing := io.Pipe()
	t.Cleanup(func() { _ = typing.Close() })
	go func() {
		_, _ = io.WriteString(typing, "P")
		for _, came := range []chan struct{}{acmeRead, staleCame} {
			select {
			case <-came:
			case <-time.After(10 * time.Second):
			}
			if came == acmeRead {
				close(release)
			}
		}
		_, _ = io.WriteString(typing, "q")
	}()
	var out lockedBuffer
	if err := run(t.Context(), cfg, &out, tea.WithInput(in), tea.WithoutSignals(), tea.WithFilter(filter)); err != nil {
		t.Fatal(err)
	}
	drawn := out.String()
	if acme, personal := strings.Contains(drawn, "Acme 1"), strings.Contains(drawn, "Personal"); !acme || personal {
		t.Errorf("once Default's read landed after the turn to Acme, the dashboard drew Acme 1 %v, Default's Personal %v; want Acme 1 alone", acme, personal)
	}
}

// heldSource is a source whose reads find doc once release is closed, as a
// read under way as P turns.
type heldSource struct {
	*fakeSource
	doc     status.Document
	release chan struct{}
}

func (s heldSource) Read(ctx context.Context, _ status.Read) (status.Document, router.Health, error) {
	select {
	case <-s.release:
		return s.doc, router.Health{}, nil
	case <-ctx.Done():
		return status.Document{}, router.Health{}, ctx.Err()
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
// saying why in a line: the first of the reason's, as a config that can't
// be read gives several.
func TestATurnThatFailsKeepsThePoolShownSayingWhy(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeShown)
	pools.failure = errors.New("no config file at ~/.config/switchboard/config.toml: write one such as\n\n[[account]]\nid = \"work\"\n")
	h.start()
	h.deliver(h.press("P")...)
	if h.model.cfg.Pool.ID != "b2n8" || h.model.turning {
		t.Errorf("after a turn that failed, the pool shown is %q, turning %v; want Default still, no turn under way", h.model.cfg.Pool.ID, h.model.turning)
	}
	if got, want := h.keyLine(), "can't turn to the next pool: no config file at ~/.config/switchboard/config.toml: write one such as"; got != want {
		t.Errorf("the key line reads %q, want %q", got, want)
	}
}

// With one pool, P isn't listed, but asks for the next all the same, the
// pools read afresh: still one, the key line says so, and the pool shown
// stays; a second added since, P turns to it, and lists P pool from then.
func TestPTurnsToASecondPoolAddedToAConfigOfOne(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeShown)
	only := defaultShown
	only.Pools = 1
	cfg := h.config(Size{Width: 160, Height: 40})
	cfg.Pool, cfg.Accounts, cfg.Turn = only, defaultAccounts, pools.turn
	h.model = New(t.Context(), cfg)
	pools.order = []string{"b2n8"}
	pools.pools["b2n8"] = Pooled{Pool: only, Accounts: defaultAccounts, Source: h.source, Events: events.Empty{}}
	h.start()
	if got := h.keyLine(); strings.Contains(got, "P pool") {
		t.Errorf("with one pool, the key line reads %q, want no P pool", got)
	}
	turns := h.model.poolTurns
	h.deliver(h.press("P")...)
	if got, want := h.keyLine(), "there's one pool: add another with switchboard pool add"; got != want || h.model.poolTurns != turns || h.model.turning {
		t.Errorf("P with one pool still has the key line read %q, turned %v; want %q, nothing turned", got, h.model.poolTurns != turns, want)
	}

	pools.order = []string{"b2n8", "q7mt"}
	h.deliver(h.press("P")...)
	if got := h.model.cfg.Pool.ID; got != "q7mt" || !strings.HasSuffix(h.keyLine(), "P pool · ? help · q quit") {
		t.Errorf("P once Acme is added shows %q, its key line %q; want Acme, P pool listed", got, h.keyLine())
	}
}

// changeConfig has the config stand as change leaves the pools, its file
// written again.
func (p *poolTurns) changeConfig(change func(pools map[string]Pooled)) {
	change(p.pools)
	p.stamp += "+"
}

// An empty pool given an account, the router routing it, lists it at the
// next read, as the config changed: its views drawn, Accounts, History and
// Graphs listing it, and p, with two, opening the routing card. One whose
// last account is taken out draws the empty page at the next.
func TestAnEmptyPoolGivenAnAccountListsItAtTheNextRead(t *testing.T) {
	h, acme, pools := pooledHarness(t, emptyAcme)
	h.start()
	h.deliver(h.press("P")...)
	if got, want := h.keyLine(), "tab views · P pool · ? help · q quit"; got != want {
		t.Fatalf("on empty Acme, the key line reads %q, want %q", got, want)
	}
	two := config.Accounts{acmeAccounts[0], {ID: "r5t2", Label: "Acme 2", Pool: "q7mt"}}
	pools.changeConfig(func(p map[string]Pooled) {
		acmeOfTwo := p["q7mt"]
		acmeOfTwo.Pool.Accounts, acmeOfTwo.Accounts = 2, two
		p["q7mt"] = acmeOfTwo
	})
	*acme.router = ofPool(routerDocument(account("k3f9", "Acme 1", fableWeek(0.7)), account("r5t2", "Acme 2", fableWeek(0.2))), "q7mt")
	h.deliver(h.press("R")...)
	if view := h.view(); strings.Contains(view, "has no accounts") || !strings.Contains(view, "Acme 1") {
		t.Errorf("once Acme 1 and 2 are added and read, the screen is\n%s\nwant Acme's views, Acme 1 among them", view)
	}
	m := h.model
	for name, listed := range map[string]config.Accounts{"Accounts": m.accounts.shown.Accounts, "History": m.past.page.Accounts, "Graphs": m.graphsShown().Accounts} {
		if !slices.Equal(listed.IDs(), two.IDs()) {
			t.Errorf("once Acme 1 and 2 are added and read, %s lists %v, want %v", name, listed.IDs(), two.IDs())
		}
	}
	if h.press("p"); h.model.open != routingOpen {
		t.Errorf("once Acme 1 and 2 are added and read, p opened %v, want the routing card", h.model.open)
	}
	h.press("esc")

	pools.changeConfig(func(p map[string]Pooled) { p["q7mt"] = Pooled{Pool: emptyAcme, Source: acme, Events: events.Empty{}} })
	*acme.router = ofPool(routerDocument(), "q7mt")
	h.deliver(h.press("R")...)
	if view := h.view(); !strings.Contains(view, "Acme has no accounts, so") {
		t.Errorf("once Acme 1 and 2 are taken out and read, the screen is\n%s\nwant the empty page", view)
	}
}

// An account the config gives a pool that the router hasn't taken up has the
// page say so, and how to have it taken up; once the router routes it, the
// views are drawn.
func TestAnAccountTheRouterHasntTakenUpIsSaidToWaitOnIt(t *testing.T) {
	h, acme, pools := pooledHarness(t, emptyAcme)
	h.start()
	h.deliver(h.press("P")...)
	pools.changeConfig(func(p map[string]Pooled) {
		acmeOfOne := p["q7mt"]
		acmeOfOne.Pool.Accounts, acmeOfOne.Accounts = 1, acmeAccounts
		p["q7mt"] = acmeOfOne
	})
	h.deliver(h.press("R")...)
	view := h.view()
	for _, want := range []string{"no accounts routed yet", "Acme has no accounts the router routes yet", "switchboard service restart"} {
		if !strings.Contains(view, want) {
			t.Errorf("with Acme 1 in the config, not yet routed, the screen is\n%s\nwant it to say %q", view, want)
		}
	}
	*acme.router = ofPool(routerDocument(account("k3f9", "Acme 1", fableWeek(0.7))), "q7mt")
	h.deliver(h.press("R")...)
	if view := h.view(); strings.Contains(view, "has no accounts") || !strings.Contains(view, "Acme 1") {
		t.Errorf("once the router routes Acme 1, the screen is\n%s\nwant Acme's views", view)
	}
}

// The other pool removed, the title row names none and P pool goes from the
// key line: at the next read, as the config changed, and at P, which finds
// the pool shown alone.
func TestTheOtherPoolRemovedUnnamesThePoolShown(t *testing.T) {
	removed := func(p map[string]Pooled) {
		delete(p, "q7mt")
		only := p["b2n8"]
		only.Pool.Pools = 1
		p["b2n8"] = only
	}
	for _, by := range []string{"R", "P"} {
		t.Run("at "+by, func(t *testing.T) {
			h, _, pools := pooledHarness(t, acmeShown)
			h.start()
			pools.order = []string{"b2n8"}
			if by == "R" {
				pools.changeConfig(removed)
			} else {
				removed(pools.pools)
			}
			h.deliver(h.press(by)...)
			if row, line := h.titleRow(), h.keyLine(); strings.Contains(row, "Default") || strings.Contains(line, "P pool") {
				t.Errorf("Acme removed, after %s the title row reads %q and the key line %q; want Default unnamed, no P pool", by, row, line)
			}
		})
	}
}

// A pool relabelled is named afresh in the title row at the next read.
func TestAPoolRelabelledIsNamedAfreshAtTheNextRead(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeShown)
	h.start()
	pools.changeConfig(func(p map[string]Pooled) {
		home := p["b2n8"]
		home.Pool.Label = "Home"
		p["b2n8"] = home
	})
	h.deliver(h.press("R")...)
	if row := h.titleRow(); !strings.Contains(row, "SWITCHBOARD   Home ") {
		t.Errorf("Default relabelled Home, the title row reads %q, want Home named", row)
	}
}

// A router that can't tell its pools answers with a document of no pool and
// no accounts, which leaves the pool shown as the config has it: no empty
// page.
func TestADocumentOfNoPoolDrawsNoEmptyPage(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeShown)
	h.start()
	*h.source.router = routerDocument()
	h.deliver(h.press("R")...)
	if view := h.view(); strings.Contains(view, "has no accounts") {
		t.Errorf("a document of no pool drew\n%s\nwant no empty page", view)
	}
}

// The pool shown is read again only where the config changed: never at a
// read that finds the config file as it was and the accounts as the last
// read found them.
func TestThePoolShownIsReadAgainOnlyWhereTheConfigChanged(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeShown)
	h.start()
	for range 3 {
		h.deliver(h.press("R")...)
	}
	if pools.again != 0 {
		t.Errorf("reads that found nothing changed read the pool shown again %d times, want none", pools.again)
	}
	pools.changeConfig(func(map[string]Pooled) {})
	for range 3 {
		h.deliver(h.press("R")...)
	}
	if pools.again != 1 {
		t.Errorf("a config changed once had the pool shown read again %d times, want once", pools.again)
	}
	*h.source.router = routerDocument(three()[:2]...)
	h.deliver(h.press("R")...)
	h.deliver(h.press("R")...)
	if pools.again != 2 {
		t.Errorf("a read's accounts changed once had the pool shown read again %d times in all, want twice", pools.again)
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
