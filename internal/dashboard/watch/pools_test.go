package watch

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"weak"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The pools of these tests, as shown: Default, unfenced, with three
// accounts, shown first; and Acme, fenced, with one.
var (
	defaultShown = status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 2}
	acmeShown    = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Accounts: 1, Pools: 2}
)

// testPools are these tests' pools as the config has them, Default's and
// Acme's.
var testPools = config.Pools{
	{ID: "b2n8", Label: "Default"},
	{ID: "q7mt", Label: "Acme", Dirs: []string{"/home/me/Code/acme"}, WrittenDirs: []string{"~/Code/acme"}},
}

// poolTurns is a dashboard's pools as the config has them in these tests,
// read as Config's Turn reads them: the pools and every account, as the
// config file at path places them, each pool's source by its id, and the
// router's files of events, nil for none; the grounds of the request ledger,
// their first pool the one the accounts record notes, as ledgerGrounds
// takes them, and the accounts removed, as the config file has them, and
// the ledgers made on them, each of base, as many as made counts, none
// held; the turns P asked for, each by the pool turned from, and the times
// the pool shown was read again; and what a turn fails with, where it does.
type poolTurns struct {
	t        *testing.T
	pools    config.Pools
	accounts config.Accounts
	grounds  ledger.Grounds
	base     views.Ledger
	made     int
	removed  status.RemovedAccounts
	sources  map[string]Source
	filed    views.Events
	path     string
	changes  int
	from     []string
	again    int
	failure  error
}

// newPoolTurns is the pools given, placing the accounts given, each read by
// its source in sources, their config file written.
func newPoolTurns(t *testing.T, pools config.Pools, accounts config.Accounts, sources map[string]Source) *poolTurns {
	t.Helper()
	p := &poolTurns{t: t, pools: pools, accounts: accounts, sources: sources, path: filepath.Join(t.TempDir(), "config.toml")}
	p.written()
	return p
}

// turn gives what's shown of the pool asked for from from, as Config's Turn
// does: the pool after it, or it again, else, where it's gone, the first;
// of none shown, the one the verbs act on, as config's Pools Implicit has
// it, none where every pool is fenced or there are none.
func (p *poolTurns) turn(_ context.Context, from string, ask PoolAsk) (Pooled, error) {
	if ask == SamePool {
		p.again++
	} else {
		p.from = append(p.from, from)
	}
	if p.failure != nil {
		return Pooled{}, p.failure
	}
	if ask == SamePool && from == "" {
		implicit, _ := p.pools.Implicit()
		return p.pooled(implicit.ID), nil
	}
	pick := status.PoolAfter
	if ask == SamePool {
		pick = status.PoolAgain
	}
	next, _ := pick(p.pools, from)
	return p.pooled(next.ID), nil
}

// pooled is what's shown of the pool with the given id, as the config
// stands: its share of the router's events, as the verbs read them, by the
// one rule the dashboard reads them by; and how the Log reads its work of
// the request ledger, as poolRead has it, none's of none shown where there
// are pools.
func (p *poolTurns) pooled(id string) Pooled {
	var filed views.Events = events.Empty{}
	if p.filed != nil {
		filed = p.poolRead(id).Filed(p.filed)
	}
	read := p.poolRead(id)
	if id == "" && len(p.pools) > 0 {
		read = views.NoPool()
	}
	return Pooled{
		Pool:     status.PoolShown(p.pools, p.accounts, id),
		Accounts: status.PoolAccounts(p.pools, p.accounts, id),
		Source:   p.sources[id],
		Events:   filed,
		Read:     read,
		Stamp:    p.configStamp(),
		Placed:   status.Placed(p.pools, p.accounts),
		Grounds:  p.ledgerGrounds(),
		NewLedger: func() views.Ledger {
			p.made++
			return &configsLedger{Ledger: p.base}
		},
		Removed: p.removed,
	}
}

// poolRead is how the pool with the given id is read, as the config
// stands, as the verbs read a pool's, its first pool the ledger's grounds',
// as ledgerGrounds has them.
func (p *poolTurns) poolRead(id string) views.PoolRead {
	return views.PoolRead{ID: id, First: p.ledgerGrounds().First, PoolOf: status.AccountPools(p.accounts, func() identity.Record { return identity.Record{} })}
}

// ledgerGrounds are the grounds of the request ledger as the config stands,
// as the dashboard's Turn gives them: its first pool of the pools, as
// idmove.FirstOf has it of the one the grounds given note, as the accounts
// record notes it; so the config's first, where it notes none.
func (p *poolTurns) ledgerGrounds() ledger.Grounds {
	g := p.grounds
	g.First = idmove.FirstOf(p.pools, g.First)
	return g
}

// changeConfig has the config stand as change leaves it, its file written
// again.
func (p *poolTurns) changeConfig(change func(p *poolTurns)) {
	change(p)
	p.changes++
	p.written()
}

// written writes the config file, last modified as many minutes on as it has
// changed.
func (p *poolTurns) written() {
	p.t.Helper()
	if err := os.WriteFile(p.path, []byte("[[pool]]\n"), 0o600); err != nil {
		p.t.Fatal(err)
	}
	at := time.Date(2026, 11, 13, 9, p.changes, 0, 0, time.UTC)
	if err := os.Chtimes(p.path, at, at); err != nil {
		p.t.Fatal(err)
	}
}

// configStamp says how the config stands, as Config's ConfigStamp does.
func (p *poolTurns) configStamp() filestate.State {
	return filestate.Of(p.path)
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

// pooledHarness is a harness of the router's document of Default's three
// accounts, shown, beside Acme, with the accounts given, whose source reads
// the router's document of k3f9 alone, or of none where acme gives none, P
// turning between them.
func pooledHarness(t *testing.T, acme config.Accounts) (*harness, *fakeSource, *poolTurns) {
	t.Helper()
	return pooledHarnessFiled(t, acme, nil)
}

// pooledHarnessFiled is pooledHarness, the router's files of events those
// filed, nil for none.
func pooledHarnessFiled(t *testing.T, acme config.Accounts, filed views.Events) (*harness, *fakeSource, *poolTurns) {
	t.Helper()
	h := routedHarness(t, ofPool(routerDocument(three()...), "b2n8"))
	acmeDoc := ofPool(routerDocument(), "q7mt")
	if len(acme) > 0 {
		acmeDoc = ofPool(routerDocument(account("k3f9", "Acme 1", fableWeek(0.7))), "q7mt")
	}
	acmeSource := &fakeSource{doc: probedWithoutTheRouter(), router: &acmeDoc}
	pools := newPoolTurns(t, testPools, append(slices.Clone(defaultAccounts), acme...), map[string]Source{"b2n8": h.source, "q7mt": acmeSource})
	pools.filed = filed
	h.model = New(t.Context(), pools.opened(h.config(Size{Width: 160, Height: 40}), "b2n8"))
	return h, acmeSource, pools
}

// unpooledAccounts are Default's three accounts as a config without pools
// has them, placed in none.
var unpooledAccounts = config.Accounts{{ID: "work", Label: "Work"}, {ID: "personal", Label: "Personal"}, {ID: "side", Label: "Side"}}

// unpooledHarness is a harness of the router's document of the three
// accounts of a config without pools, showing no pool, its Turn reading the
// config as it stands.
func unpooledHarness(t *testing.T) (*harness, *poolTurns) {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	pools := newPoolTurns(t, nil, unpooledAccounts, map[string]Source{"": h.source})
	h.model = New(t.Context(), pools.opened(h.config(Size{Width: 160, Height: 40}), ""))
	return h, pools
}

// pooledOrNot are the dashboards these tests follow the config of: one of a
// config with pools, showing Default beside Acme, as pooledHarness has it;
// and one without them, showing none, as unpooledHarness has it; each with
// the id of the pool its router's documents are of.
var pooledOrNot = []struct {
	name    string
	harness func(t *testing.T) (*harness, *poolTurns)
	pool    string
}{
	{name: "with pools", harness: func(t *testing.T) (*harness, *poolTurns) {
		h, _, pools := pooledHarness(t, acmeAccounts)
		return h, pools
	}, pool: "b2n8"},
	{name: "without pools", harness: unpooledHarness},
}

// opened is cfg showing the pool with the given id, as the dashboard opens
// on it, its ledger one of cfg's, told apart by its address, on the grounds
// the config gives.
func (p *poolTurns) opened(cfg Config, id string) Config {
	p.base, p.removed = cfg.Ledger, cfg.Removed
	cfg.Ledger, cfg.LedgerGrounds = &configsLedger{Ledger: p.base}, p.ledgerGrounds()
	shown := p.pooled(id)
	cfg.Pool, cfg.Accounts, cfg.Source, cfg.Events, cfg.Turn = shown.Pool, shown.Accounts, shown.Source, shown.Events, p.turn
	cfg.ConfigStamp, cfg.Stamp, cfg.Placed, cfg.PoolRead = p.configStamp, shown.Stamp, shown.Placed, shown.Read
	return cfg
}

// titleRow is the screen's title row.
func (h *harness) titleRow() string {
	return strings.SplitN(h.view(), "\n", 2)[0]
}

// P turns to the next pool, its pools read afresh as Turn reads them: the
// dashboard reads its document, starting with a full read, as it opened,
// and names it in the title row; and P again turns back to the first.
func TestPTurnsToTheNextPoolReadingItAfresh(t *testing.T) {
	h, acme, pools := pooledHarness(t, acmeAccounts)
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

	h, _, _ := pooledHarness(t, acmeAccounts)
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
	h, acme, _ := pooledHarness(t, acmeAccounts)
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
	h, _, _ := pooledHarness(t, acmeAccounts)
	h.start()
	before := h.model.poolTurns
	stale := fetchedMsg{read: status.Read{Probe: true}, doc: routerDocument(three()...)}
	h.model.keeping.writing = 1

	tm := turns{Model: h.model}
	next, _ := tm.Update(ofTurn{turn: before, msg: turnedMsg{pooled: Pooled{Pool: acmeShown, Source: h.source, Events: events.Empty{}, Grounds: h.model.cfg.LedgerGrounds}}})
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
	h, _, _ := pooledHarness(t, acmeAccounts)
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
	deliver(ofTurn{turn: before, msg: turnedMsg{pooled: Pooled{Pool: acmeShown, Accounts: acmeAccounts, Source: h.source, Events: events.Empty{}, Grounds: h.model.cfg.LedgerGrounds}}})
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
	h, _, pools := pooledHarness(t, acmeAccounts)
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
	h, _, pools := pooledHarness(t, acmeAccounts)
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
	h, acme, _ := pooledHarness(t, acmeAccounts)
	pools := newPoolTurns(t, testPools[:1], defaultAccounts, map[string]Source{"b2n8": h.source, "q7mt": acme})
	h.model = New(t.Context(), pools.opened(h.config(Size{Width: 160, Height: 40}), "b2n8"))
	h.start()
	if got := h.keyLine(); strings.Contains(got, "P pool") {
		t.Errorf("with one pool, the key line reads %q, want no P pool", got)
	}
	turns := h.model.poolTurns
	h.deliver(h.press("P")...)
	if got, want := h.keyLine(), "there's one pool: add another with switchboard pool add"; got != want || h.model.poolTurns != turns || h.model.turning {
		t.Errorf("P with one pool still has the key line read %q, turned %v; want %q, nothing turned", got, h.model.poolTurns != turns, want)
	}

	pools.pools, pools.accounts = testPools, append(slices.Clone(defaultAccounts), acmeAccounts...)
	h.deliver(h.press("P")...)
	if got := h.model.cfg.Pool.ID; got != "q7mt" || !strings.HasSuffix(h.keyLine(), "P pool · ? help · q quit") {
		t.Errorf("P once Acme is added shows %q, its key line %q; want Acme, P pool listed", got, h.keyLine())
	}
}

// An empty pool given an account, the router routing it, lists it at the
// next read, as the config changed: its views drawn, Accounts, History and
// Graphs listing it, and p, with two, opening the routing card. One whose
// last account is taken out draws the empty page at the next.
func TestAnEmptyPoolGivenAnAccountListsItAtTheNextRead(t *testing.T) {
	h, acme, pools := pooledHarness(t, nil)
	h.start()
	h.deliver(h.press("P")...)
	if got, want := h.keyLine(), "tab views · P pool · ? help · q quit"; got != want {
		t.Fatalf("on empty Acme, the key line reads %q, want %q", got, want)
	}
	two := config.Accounts{acmeAccounts[0], {ID: "r5t2", Label: "Acme 2", Pool: "q7mt"}}
	pools.changeConfig(func(p *poolTurns) { p.accounts = append(slices.Clone(defaultAccounts), two...) })
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

	pools.changeConfig(func(p *poolTurns) { p.accounts = defaultAccounts })
	*acme.router = ofPool(routerDocument(), "q7mt")
	h.deliver(h.press("R")...)
	h.deliver(h.press("R")...)
	if view := h.view(); !strings.Contains(view, "Acme has no accounts, so") {
		t.Errorf("once Acme 1 and 2 are taken out and read, the screen is\n%s\nwant the empty page", view)
	}
}

// An account the config gives a pool that the router hasn't taken up has the
// page say so, and how to have it taken up; once the router routes it, the
// views are drawn.
func TestAnAccountTheRouterHasntTakenUpIsSaidToWaitOnIt(t *testing.T) {
	h, acme, pools := pooledHarness(t, nil)
	h.start()
	h.deliver(h.press("P")...)
	pools.changeConfig(func(p *poolTurns) { p.accounts = append(slices.Clone(defaultAccounts), acmeAccounts...) })
	acme.router.Restart = configChanged
	h.deliver(h.press("R")...)
	h.deliver(h.press("R")...)
	waitsOnTheRouter(t, h, "Acme", "● healthy  ·  restart due (config changed)  ·  no accounts routed yet")
	*acme.router = ofPool(routerDocument(account("k3f9", "Acme 1", fableWeek(0.7))), "q7mt")
	h.deliver(h.press("R")...)
	if view := h.view(); strings.Contains(view, "has no accounts") || !strings.Contains(view, "Acme 1") {
		t.Errorf("once the router routes Acme 1, the screen is\n%s\nwant Acme's views", view)
	}
}

// configChanged is the restart a router has due once its config changed.
var configChanged = status.Restart{Reason: status.RestartForConfig, InFlight: 1}

// waitsOnTheRouter fails the test unless h's screen says the pool with the
// label given waits on the router to take up its accounts, and how to have
// them taken up, its status line reading line in full.
func waitsOnTheRouter(t *testing.T, h *harness, label, line string) {
	t.Helper()
	view := h.view()
	for _, want := range []string{label + " has no accounts the router routes yet", "switchboard service restart"} {
		if !strings.Contains(view, want) {
			t.Errorf("with %s's accounts in the config, not yet routed, the screen is\n%s\nwant it to say %q", label, view, want)
		}
	}
	if got := statusLineOf(view); got != line {
		t.Errorf("with %s's accounts in the config, not yet routed, the status line reads %q, want %q", label, got, line)
	}
}

// statusLineOf is the status line of a pool with no accounts on the screen
// view, as it reads.
func statusLineOf(view string) string {
	for l := range strings.Lines(view) {
		if strings.Contains(l, "  ·  no accounts") {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

// refusingSource is a source whose router refuses every read while refusal
// is set, as it refuses one of a pool it doesn't know, giving its health as
// it answered.
type refusingSource struct {
	*fakeSource
	refusal *error
}

func (s refusingSource) Read(ctx context.Context, r status.Read) (status.Document, router.Health, error) {
	if *s.refusal != nil {
		s.asked = append(s.asked, r)
		return status.Document{}, s.health, *s.refusal
	}
	return s.fakeSource.Read(ctx, r)
}

// A pool the router doesn't know, as one added with an account since it
// started, which it refuses reads of, waits on the router as one it knows
// and routes none in does: the page says so, not that a read failed; once
// the router routes it, the views are drawn.
func TestAPoolTheRouterDoesntKnowWaitsOnIt(t *testing.T) {
	h, acme, pools := pooledHarness(t, acmeAccounts)
	refusal := error(router.NeedPool("q7mt", testPools[:1]))
	pools.sources["q7mt"] = refusingSource{fakeSource: acme, refusal: &refusal}
	acme.health = router.Health{OK: true, PID: 1, Restart: configChanged}
	h.start()
	h.deliver(h.press("P")...)
	refusedLine := "● healthy  ·  restart due (config changed)  ·  no accounts routed yet"
	waitsOnTheRouter(t, h, "Acme", refusedLine)
	h.deliver(h.press("R")...)
	if view := h.view(); h.model.failed != "" || strings.Contains(view, "there's no pool") {
		t.Errorf("with Acme unknown to the router, the screen is\n%s\nthe read failing %q; want no read failed", view, h.model.failed)
	}
	waitsOnTheRouter(t, h, "Acme", refusedLine)

	refusal = nil
	h.deliver(h.press("R")...)
	if view := h.view(); strings.Contains(view, "has no accounts") || !strings.Contains(view, "Acme 1") {
		t.Errorf("once the router routes Acme, the screen is\n%s\nwant Acme's views", view)
	}
}

// A refusal after the router was found gone is the router answering: it's
// lost no more, and the status line says how it stands as it answered.
func TestARefusalAfterTheRouterWasLostSaysHowItAnswered(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeAccounts)
	var refusal error
	h.model.cfg.Source = refusingSource{fakeSource: h.source, refusal: &refusal}
	h.start()
	h.stopRouter()
	h.tickUntilAsked()
	if h.model.lost.IsZero() {
		t.Fatal("a look that found the router gone left it not lost, want it lost")
	}
	refusal = router.NeedPool("b2n8", testPools[1:])
	h.source.health = router.Health{OK: true, PID: 1, Restart: configChanged}
	h.tickUntilAsked()
	if !h.model.lost.IsZero() {
		t.Errorf("the router refusing Default after it was lost leaves it lost since %v, want it lost no more", h.model.lost)
	}
	waitsOnTheRouter(t, h, "Default", "● healthy  ·  restart due (config changed)  ·  no accounts routed yet")
}

// A pool needed with no router's answer beside it, as a probe in place of
// the router finds of a pool the config no longer has, is a read that
// failed, not the router refusing: the pool shown isn't drawn as refused,
// nor the router as found again.
func TestAPoolNeededWithNoRoutersAnswerIsAReadThatFailed(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeAccounts)
	var refusal error
	h.model.cfg.Source = refusingSource{fakeSource: h.source, refusal: &refusal}
	h.start()
	h.stopRouter()
	h.tickUntilAsked()
	lost := h.model.lost
	if lost.IsZero() {
		t.Fatal("a look that found the router gone left it not lost, want it lost")
	}
	refusal = router.NeedPool("b2n8", testPools[1:])
	h.tickUntilAsked()
	if m := h.model; m.refusing != nil || m.failed == "" || !m.lost.Equal(lost) {
		t.Errorf("a pool needed with no router's answer drew the pool refused %v, the read failing %q, the router lost since %v; want no refusal, the read failed, lost since %v", m.refusing != nil, m.failed, m.lost, lost)
	}
}

// Another pool's refusal, as of a pool named otherwise, is a read that
// failed, not the pool shown waiting.
func TestAnotherPoolsRefusalIsAReadThatFailed(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeAccounts)
	refusal := error(router.NeedPool("x9z9", testPools))
	h.model.cfg.Source = refusingSource{fakeSource: h.source, refusal: &refusal}
	h.start()
	if h.model.pool(h.clock.now).Empty() || h.model.failed == "" {
		t.Errorf("refused a read of another pool, the pool shown is empty %v, the read failing %q; want it not empty, the read failed", h.model.pool(h.clock.now).Empty(), h.model.failed)
	}
}

// The other pool removed, the title row names none and P pool goes from the
// key line: at the next read, as the config changed, and at P, which finds
// the pool shown alone.
func TestTheOtherPoolRemovedUnnamesThePoolShown(t *testing.T) {
	removed := func(p *poolTurns) {
		p.pools, p.accounts = testPools[:1], defaultAccounts
	}
	for _, by := range []string{"R", "P"} {
		t.Run("at "+by, func(t *testing.T) {
			h, _, pools := pooledHarness(t, acmeAccounts)
			h.start()
			if by == "R" {
				pools.changeConfig(removed)
			} else {
				removed(pools)
			}
			h.deliver(h.press(by)...)
			if row := h.titleRow(); strings.Contains(row, "Default") || slices.Contains(h.listed(), "P") {
				t.Errorf("Acme removed, after %s the title row reads %q and the key line lists %q; want Default unnamed, no P", by, row, h.listed())
			}
		})
	}
}

// A pool relabelled is named afresh in the title row at the next read.
func TestAPoolRelabelledIsNamedAfreshAtTheNextRead(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeAccounts)
	h.start()
	pools.changeConfig(func(p *poolTurns) {
		p.pools = slices.Clone(p.pools)
		p.pools[0].Label = "Home"
	})
	h.deliver(h.press("R")...)
	if row := h.titleRow(); !strings.Contains(row, "SWITCHBOARD   Home ") {
		t.Errorf("Default relabelled Home, the title row reads %q, want Home named", row)
	}
}

// An account of the pool shown relabelled, or put on another plan, is
// followed at the next read: the pool shown starts afresh, listing it as
// the config now has it, its plan priced.
func TestAnAccountOfThePoolShownChangedIsFollowedAtTheNextRead(t *testing.T) {
	tests := []struct {
		name   string
		change func(a *config.Account)
	}{
		{name: "relabelled", change: func(a *config.Account) { a.Label = "Personal 2" }},
		{name: "on another plan", change: func(a *config.Account) { a.Plan = config.PlanUltra }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, pools := pooledHarness(t, acmeAccounts)
			h.start()
			turns := h.model.poolTurns
			pools.changeConfig(func(p *poolTurns) {
				p.accounts = slices.Clone(p.accounts)
				tt.change(&p.accounts[1])
			})
			h.deliver(h.press("R")...)
			want := pools.accounts[:3]
			m := h.model
			if m.poolTurns != turns+1 || !slices.Equal(m.cfg.Accounts, want) || !slices.Equal(m.accounts.shown.Accounts, want) || !slices.Equal(m.past.page.Accounts, want) {
				t.Errorf("an account %s, the pool shown started afresh %v, listing %+v; want it started afresh, listing %+v", tt.name, m.poolTurns != turns, m.accounts.shown.Accounts, want)
			}
			plan := func(p views.PlanCost) string { return p.Account + " " + p.Plan }
			if got, want := mapped(m.plansAtMost, plan), mapped(views.PlansAtMost(want, m.cfg.Prices), plan); !slices.Equal(got, want) {
				t.Errorf("an account %s, the plans priced are %q, want %q", tt.name, got, want)
			}
		})
	}
}

// An account taken out of the config while it's open is named, from the
// next read, by the accounts removed as the config read has them; and a
// reserve changed has a ledger made on the new caps, the one before let go:
// nothing the dashboard holds still holds it.
func TestAnAccountTakenOutIsNamedAndTheLedgerMadeOnTheConfigReadsCaps(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeAccounts)
	h.start()
	before := weak.Make(h.model.cfg.Ledger.(*configsLedger))
	pools.changeConfig(func(p *poolTurns) {
		p.accounts = slices.Concat(defaultAccounts[:2], acmeAccounts)
		p.removed = status.RemovedAccounts{{ID: "side", Label: "Side"}}
		p.grounds = ledger.Grounds{Caps: ledger.Caps{Reserves: map[string]float64{"work": 0.05}}}
	})
	h.deliver(h.press("R")...)
	m := h.model
	if len(m.cfg.Removed) != 1 || m.cfg.Removed[0].Label != "Side" {
		t.Errorf("side taken out, the dashboard names the accounts removed %+v, want side, labelled Side", m.cfg.Removed)
	}
	if pools.made != 1 || !m.cfg.LedgerGrounds.Same(pools.ledgerGrounds()) || m.cfg.Ledger.(*configsLedger) == before.Value() {
		t.Errorf("a reserve changed, %d ledgers were made, the one in use on the new grounds %v; want one, made on them", pools.made, m.cfg.LedgerGrounds.Same(pools.ledgerGrounds()))
	}
	runtime.GC()
	if before.Value() != nil {
		t.Error("the ledger made on the grounds before is still held, want it let go")
	}
}

// A re-read finding the ledger's grounds changed alone, every account and
// pool placed as it was, as the first pool's moved in the accounts record,
// starts the pool shown afresh on a ledger made on the new grounds.
func TestTheLedgersGroundsChangedAloneStartThePoolShownAfresh(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeAccounts)
	h.start()
	turns := h.model.poolTurns
	pools.changeConfig(func(p *poolTurns) { p.grounds = ledger.Grounds{First: "q7mt"} })
	h.deliver(h.press("R")...)
	if m := h.model; m.poolTurns != turns+1 || pools.made != 1 || !m.cfg.LedgerGrounds.Same(pools.ledgerGrounds()) {
		t.Errorf("the first pool changed alone, the pool shown started afresh %d times, %d ledgers made, the one in use on the new grounds %v; want afresh once, one made on them",
			m.poolTurns-turns, pools.made, m.cfg.LedgerGrounds.Same(pools.ledgerGrounds()))
	}
}

// A turn on the grounds the ledger in use was made on keeps it, and what it
// holds of the days read: P to another pool, and a change to the config
// that leaves every cap and the first pool as they were, make none.
func TestATurnOnTheSameGroundsKeepsTheLedgerInUse(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeAccounts)
	h.start()
	inUse := h.model.cfg.Ledger
	h.deliver(h.press("P")...)
	pools.changeConfig(func(p *poolTurns) {
		p.pools = slices.Clone(p.pools)
		p.pools[0].Label = "Home"
	})
	h.deliver(h.press("R")...)
	if h.model.cfg.Pool.ID != "q7mt" || pools.made != 0 || h.model.cfg.Ledger != inUse {
		t.Errorf("P, then Default relabelled, showing %q, made %d ledgers, the one in use kept %v; want Acme, none made, the one in use kept", h.model.cfg.Pool.ID, pools.made, h.model.cfg.Ledger == inUse)
	}
}

// configsLedger is a ledger as a config read has it, told apart by its
// address, telling where it begins as the ledger it holds does, so a turn
// built on it asks where the pool shown begins.
type configsLedger struct {
	views.Ledger
}

// FirstDay tells where the ledger held begins, as its LedgerStart does:
// failing as one holding no day does, where it can't tell.
func (l *configsLedger) FirstDay(now time.Time) (time.Time, error) {
	start, ok := l.Ledger.(views.LedgerStart)
	if !ok {
		return time.Time{}, fs.ErrNotExist
	}
	return start.FirstDay(now)
}

// A change to the config that places nothing otherwise, as a comment
// written, starts nothing, with pools or without: the pool shown, or none,
// is read again, as the config file changed, and kept as it is.
func TestAChangeThatPlacesNothingOtherwiseStartsNothing(t *testing.T) {
	for _, tt := range pooledOrNot {
		t.Run(tt.name, func(t *testing.T) {
			h, pools := tt.harness(t)
			h.start()
			h.deliver(h.press("tab")...)
			h.model.scroll = 2
			turns := h.model.poolTurns
			pools.changeConfig(func(*poolTurns) {})
			h.deliver(h.press("R")...)
			if pools.again != 1 || h.model.poolTurns != turns || h.model.scroll != 2 || !h.model.cfg.Stamp.Same(pools.configStamp()) {
				t.Errorf("a comment written, the pool shown was read again %d times, started afresh %v, scrolled %d, stamped as the config stands %v; want once, kept as it was, stamped afresh",
					pools.again, h.model.poolTurns != turns, h.model.scroll, h.model.cfg.Stamp.Same(pools.configStamp()))
			}
		})
	}
}

// mapped is each of xs as f has it.
func mapped[T any](xs []T, f func(T) string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

// eventsOf are the accounts the router's events through filed befell, as
// the dashboard reads them over the week to now.
func eventsOf(filed views.Events, now time.Time) []string {
	var befell []string
	for e := range filed.Between(now.Add(-7*day), now) {
		befell = append(befell, e.Befell())
	}
	return befell
}

// An account moved to the other pool while the pool shown is open is placed
// anew at the next read: its events go from the pool shown's, and from
// Runway's LATELY, and Accounts and History list it no more. In the other,
// they're shown.
func TestAnAccountMovedToTheOtherPoolIsPlacedAnewAtTheNextRead(t *testing.T) {
	files := filesOf(events.Line{Event: limitAt(1, past(-2*day), "side"), Run: past(-3 * day)}, events.Line{Event: limitAt(2, past(-day), "work"), Run: past(-3 * day)})
	h, _, pools := pooledHarnessFiled(t, acmeAccounts, files)
	files.clock = h.clock
	h.start()
	for h.model.view != dashboard.Runway {
		h.deliver(h.press("tab")...)
	}
	if got := latelySaid(h.model); !slices.Contains(got, "side reached its 5-hour limit") {
		t.Fatalf("before side is moved, LATELY tells %q, want side's limit among them", got)
	}

	pools.changeConfig(func(p *poolTurns) {
		p.accounts = slices.Clone(p.accounts)
		p.accounts[2].Pool = "q7mt"
	})
	h.deliver(h.press("R")...)
	m := h.model
	if got := eventsOf(m.cfg.Events, h.clock.now); !slices.Equal(got, []string{"work"}) {
		t.Errorf("side moved to Acme, Default's events befell %q, want work's alone", got)
	}
	if got := latelySaid(m); slices.Contains(got, "side reached its 5-hour limit") || !slices.Contains(got, "work reached its 5-hour limit") {
		t.Errorf("side moved to Acme, Default's LATELY tells %q, want work's limit, and side's no more", got)
	}
	if want := []string{"work", "personal"}; !slices.Equal(m.accounts.shown.Accounts.IDs(), want) || !slices.Equal(m.past.page.Accounts.IDs(), want) {
		t.Errorf("side moved to Acme, Accounts lists %v and History %v; want %v", m.accounts.shown.Accounts.IDs(), m.past.page.Accounts.IDs(), want)
	}

	h.deliver(h.press("P")...)
	if got := eventsOf(h.model.cfg.Events, h.clock.now); !slices.Equal(got, []string{"side"}) {
		t.Errorf("in Acme, side's pool now, the events befell %q, want side's alone", got)
	}
}

// An account added to the other pool, which the router's events name before
// it's configured, as every pool shows those of an account placed nowhere,
// never shows in the pool shown from the next read.
func TestAnAccountAddedToTheOtherPoolNeverShowsInThePoolShown(t *testing.T) {
	files := filesOf(events.Line{Event: limitAt(1, past(-day), "r5t2"), Run: past(-3 * day)})
	h, _, pools := pooledHarnessFiled(t, acmeAccounts, files)
	files.clock = h.clock
	h.start()
	if got := eventsOf(h.model.cfg.Events, h.clock.now); !slices.Equal(got, []string{"r5t2"}) {
		t.Fatalf("before r5t2 is configured, Default's events befell %q, want r5t2's, placed nowhere", got)
	}
	pools.changeConfig(func(p *poolTurns) {
		p.accounts = append(slices.Clone(p.accounts), config.Account{ID: "r5t2", Label: "Acme 2", Pool: "q7mt"})
	})
	h.deliver(h.press("R")...)
	if got := eventsOf(h.model.cfg.Events, h.clock.now); len(got) != 0 {
		t.Errorf("r5t2 added to Acme, Default's events befell %q, want none", got)
	}
}

// A re-read that fails is logged, the key line saying nothing of it, and
// asked again only once the config file changes again: not at a read that
// finds it as it was. So it is of a dashboard opened on no pool, as the
// pools couldn't be told, a config they still can't be told of failing to
// read it.
func TestAReadAgainThatFailsIsAskedAgainOnlyOnceTheConfigChangesAgain(t *testing.T) {
	tests := []struct {
		name    string
		harness func(t *testing.T) (*harness, *poolTurns)
	}{
		{name: "a pool shown", harness: pooledOrNot[0].harness},
		{name: "the pools not to be told", harness: func(t *testing.T) (*harness, *poolTurns) {
			h, pools := unpooledHarness(t)
			h.model.cfg.PoolRead = views.NoPool()
			return h, pools
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logged := logstest.Capture(t)
			h, pools := tt.harness(t)
			h.start()
			line := h.keyLine()
			pools.failure = errors.New("config file makes no valid config")
			pools.changeConfig(func(*poolTurns) {})
			h.deliver(h.press("R")...)
			if pools.again != 1 || h.keyLine() != line || !strings.Contains(logged.String(), "can't read the pool shown again as the config changed") {
				t.Errorf("a re-read that failed was asked %d times, the key line reading %q, the log\n%s\nwant once, the key line as it was, %q, the failure logged", pools.again, h.keyLine(), logged.String(), line)
			}
			h.deliver(h.press("R")...)
			if pools.again != 1 {
				t.Errorf("a read finding the config as it was asked again: %d re-reads in all, want one", pools.again)
			}
			pools.failure = nil
			pools.changeConfig(func(*poolTurns) {})
			h.deliver(h.press("R")...)
			if pools.again != 2 {
				t.Errorf("the config changed again, the pool shown was read again %d times in all, want twice", pools.again)
			}
		})
	}
}

// A change found while a turn is under way isn't dropped as the turn fails:
// the next read asks for it.
func TestAChangeFoundDuringATurnIsAskedForOnceItsDone(t *testing.T) {
	h, _, pools := pooledHarness(t, acmeAccounts)
	h.start()
	pools.failure = errors.New("no config file")
	turning := h.press("P")
	pools.changeConfig(func(*poolTurns) {})
	h.deliver(h.press("R")...)
	h.deliver(turning...)
	if pools.again != 0 {
		t.Fatalf("a change found during a turn was asked for %d times as it went on, want none", pools.again)
	}
	pools.failure = nil
	h.deliver(h.press("R")...)
	if pools.again != 1 {
		t.Errorf("once the turn failed, the next read asked for the change %d times, want once", pools.again)
	}
}

// A router that can't tell its pools answers with a document of no pool and
// no accounts, which leaves the pool shown as the config has it: no empty
// page.
func TestADocumentOfNoPoolDrawsNoEmptyPage(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeAccounts)
	h.start()
	*h.source.router = routerDocument()
	h.deliver(h.press("R")...)
	if view := h.view(); strings.Contains(view, "has no accounts") {
		t.Errorf("a document of no pool drew\n%s\nwant no empty page", view)
	}
}

// The pool shown, or none, is read again only where the config changed,
// with pools or without: never at a read that finds the config file as it
// was and the accounts as the last read found them. A read finding the
// router's accounts changed, the config as it was, as the router took one
// up as it restarted, reads it again and starts nothing.
func TestThePoolShownIsReadAgainOnlyWhereTheConfigChanged(t *testing.T) {
	for _, tt := range pooledOrNot {
		t.Run(tt.name, func(t *testing.T) {
			h, pools := tt.harness(t)
			h.start()
			for range 3 {
				h.deliver(h.press("R")...)
			}
			if pools.again != 0 {
				t.Errorf("reads that found nothing changed read the pool shown again %d times, want none", pools.again)
			}
			pools.changeConfig(func(*poolTurns) {})
			for range 3 {
				h.deliver(h.press("R")...)
			}
			if pools.again != 1 {
				t.Errorf("a config changed once had the pool shown read again %d times, want once", pools.again)
			}
			turns := h.model.poolTurns
			*h.source.router = ofPool(routerDocument(three()[:2]...), tt.pool)
			h.deliver(h.press("R")...)
			h.deliver(h.press("R")...)
			if pools.again != 2 || h.model.poolTurns != turns {
				t.Errorf("a read's accounts changed once had the pool shown read again %d times in all, started afresh %v; want twice, nothing started", pools.again, h.model.poolTurns != turns)
			}
		})
	}
}

// A dashboard showing no pool, as its pools couldn't be told, reads the pool
// shown again at a read its source fails with ErrPoolsTold, as at a change
// to the config, once a read: never at a read failing otherwise, as one
// saying the pools can't be told.
func TestAReadTellingThePoolsReadsThePoolShownAgain(t *testing.T) {
	h, pools := unpooledHarness(t)
	h.source.router = nil
	h.source.err = errors.New("can't tell which pool to read: neither the config, the router nor the accounts record names any pools")
	h.start()
	h.deliver(h.press("R")...)
	if pools.again != 0 {
		t.Errorf("a read failing as the pools can't be told read the pool shown again %d times, want none", pools.again)
	}
	h.source.err = errors.Join(ErrPoolsTold, errors.New("the router routes for them"))
	h.deliver(h.press("R")...)
	if pools.again != 1 {
		t.Errorf("a read telling the pools read the pool shown again %d times, want once", pools.again)
	}
}

// Where the pool shown is read again in vain as the pools are told, as when
// the config can't be read, the reads telling them ask for it no more, till
// something it rests on changes: the router going away and telling them
// again as it comes back, or the config changing. Once the turn can be made,
// the router telling them again shows the pool.
func TestPoolsToldInVainAreAskedForAgainOnlyOnceTheyMayBeToldAfresh(t *testing.T) {
	h, pools := unpooledHarness(t)
	h.source.router = nil
	untold := errors.New("can't tell which pool to read: neither the config, the router nor the accounts record names any pools")
	told := errors.Join(ErrPoolsTold, errors.New("the router routes for them"))
	pools.failure = errors.Join(ErrConfigUnread, errors.New("config file makes no valid config"))
	h.source.err = told
	h.start()
	steps := []struct {
		name  string
		err   error
		turn  func()
		again int
	}{
		{name: "the pools told, the turn failing", err: told, again: 1},
		{name: "the pools told again, nothing changed", err: told, again: 1},
		{name: "and again", err: told, again: 1},
		{name: "the router gone", err: untold, again: 1},
		{name: "the router back, telling them afresh", err: told, again: 2},
		{name: "the config changed", err: told, turn: func() { pools.changeConfig(func(*poolTurns) {}) }, again: 3},
		{name: "the pools told again, nothing changed since", err: told, again: 3},
		{name: "the router gone, the config readable", err: untold, turn: func() { pools.failure = nil }, again: 3},
		{name: "the router back, the turn made", err: told, again: 4},
	}
	for _, step := range steps {
		if step.turn != nil {
			step.turn()
		}
		h.source.err = step.err
		h.deliver(h.press("R")...)
		if pools.again != step.again {
			t.Fatalf("%s: the pool shown was read again %d times in all, want %d", step.name, pools.again, step.again)
		}
	}
	if h.model.toldInVain {
		t.Error("the pool shown once read again, the pools told are still taken as told in vain")
	}
}

// Where the pool shown is read again in vain as the pools are told, for a
// reason that may pass, as the router's health not answering the turn in
// time, the next read telling them asks for it again.
func TestPoolsToldInVainForAPassingReasonAreAskedForAgainAtTheNextRead(t *testing.T) {
	h, pools := unpooledHarness(t)
	h.source.router = nil
	pools.failure = errors.New("can't tell which pool to read: neither the config, the router nor the accounts record names any pools")
	h.source.err = errors.Join(ErrPoolsTold, errors.New("the router routes for them"))
	h.start()
	h.deliver(h.press("R")...)
	if pools.again != 1 {
		t.Fatalf("the pools told, the pool shown was read again %d times, want once", pools.again)
	}
	pools.failure = nil
	h.deliver(h.press("R")...)
	if pools.again != 2 {
		t.Fatalf("the pools told again after a turn that failed for a reason that may pass, the pool shown was read again %d times in all, want twice", pools.again)
	}
}

// P with no pools, though the watch has a Turn to ask, asks for no pool and
// starts nothing: there's none to turn to.
func TestPWithoutPoolsTurnsToNone(t *testing.T) {
	h, pools := unpooledHarness(t)
	h.start()
	turns := h.model.poolTurns
	h.deliver(h.press("P")...)
	if len(pools.from) != 0 || pools.again != 0 || h.model.poolTurns != turns || h.model.turning {
		t.Errorf("P without pools asked for the pools from %q, %d again, started afresh %v, turning %v; want nothing asked, nothing started",
			pools.from, pools.again, h.model.poolTurns != turns, h.model.turning)
	}
}

// The key line and help list P pool once there's a second pool, after p
// pin, a phone's line keeping it; never with one pool.
func TestPPoolIsListedOnceThereIsASecondPool(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeAccounts)
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
	h, _, _ := pooledHarness(t, nil)
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
