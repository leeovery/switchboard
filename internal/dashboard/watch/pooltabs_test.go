package watch

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
)

// ofSession is l, a request of the session with the given id, routed in the
// pool with the given id, "" for none.
func ofSession(l ledger.Held, session, pool string) ledger.Held {
	l.Session, l.Pool = session, pool
	return l
}

// pooledToday are today's lines of three sessions: d28c in Acme's pool; 7f3a
// naming none, as before pools, so Default's; and 9e21 in Default's, then
// resumed in Acme's, as once its directory's fence changes.
var pooledToday = []ledger.Held{
	ofSession(arrived("0a000001", "k3f9", -50*time.Minute), idD28C, "q7mt"),
	ofSession(arrived("0a000002", "work", -40*time.Minute), id7F3A, ""),
	ofSession(arrived("0a000003", "work", -30*time.Minute), id9E21, "b2n8"),
	ofSession(arrived("0a000004", "k3f9", -20*time.Minute), id9E21, "q7mt"),
	ofSession(arrived("0a000005", "k3f9", -10*time.Minute), id9E21, "q7mt"),
}

// listedRequests are the requests of each session the List lists, by its
// id.
func listedRequests(h *harness) map[string]int {
	requests := make(map[string]int)
	for _, s := range h.model.sessionsTab.shown.List.Sessions {
		requests[s.Session] = s.Requests
	}
	return requests
}

// Sessions lists the sessions of the pool shown alone, each with its
// requests in the pool, as its first read holds them and as a look gains
// more, which the store takes in of the pool alone; and a session's page
// shows its work in the pool alone: Acme's d28c and 9e21's two last; and
// Default's 7f3a and 9e21's first; and every pool's, all.
func TestSessionsAndAPageHoldThePoolShownsWorkAlone(t *testing.T) {
	tests := []struct {
		name         string
		read         views.PoolRead
		listed, more map[string]int
		page         int
	}{
		{name: "Acme's", read: acmeRead, listed: map[string]int{idD28C: 1, id9E21: 2}, more: map[string]int{idD28C: 2, id9E21: 2}, page: 2},
		{name: "Default's", read: firstRead, listed: map[string]int{id7F3A: 1, id9E21: 1}, more: map[string]int{id7F3A: 2, id9E21: 1}, page: 1},
		{name: "every pool's", read: views.PoolRead{}, listed: map[string]int{idD28C: 1, id7F3A: 1, id9E21: 3}, more: map[string]int{idD28C: 2, id7F3A: 2, id9E21: 3}, page: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, pooledToday...)
			h := newHarness(t, probedWithoutTheRouter())
			h.model.cfg.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, views.DigestVersion, logs.For("watch"))
			h.model.cfg.PoolRead = tt.read
			h.start()

			h.keys(toSessions...)
			if got := listedRequests(h); !maps.Equal(got, tt.listed) {
				t.Errorf("Sessions lists %v, want %v, the pool's requests alone", got, tt.listed)
			}
			writeLines(t, state, ofSession(arrived("0a000006", "k3f9", -5*time.Second), idD28C, "q7mt"), ofSession(arrived("0a000007", "work", -4*time.Second), id7F3A, ""))
			h.read(probedWithoutTheRouter())
			if got := listedRequests(h); !maps.Equal(got, tt.more) {
				t.Errorf("a look on, Sessions lists %v, want %v, the pool's requests alone", got, tt.more)
			}
			h.openPage(id9E21)
			if got := h.shownPage().Page.Totals.Requests; got != tt.page {
				t.Errorf("9e21's page counts %d requests, want %d, its requests in the pool", got, tt.page)
			}
		})
	}
}

// requestsHeld counts the requests of the days History's reads hold, as
// their summaries count them.
func requestsHeld(h *harness) int {
	now := h.clock.Now()
	n := 0
	for _, s := range h.model.past.last.past.Summaries(dayfile.DayStart(now, -400), now) {
		for _, a := range s.Accounts {
			for _, m := range a.Models {
				n += m.Upstream + m.Unsent
			}
		}
	}
	return n
}

// History, and Graphs' WHEN YOU WORK, which share its reads, read the days
// of the pool shown's work alone: its requests of the day before and today,
// Acme's two, Default's three, one of them naming none, and every pool's
// five.
func TestTheDaysHistoryAndGraphsReadAreThePoolShowns(t *testing.T) {
	lines := []ledger.Held{
		pooledArrived("0b000001", "k3f9", "q7mt", -day), pooledArrived("0b000002", "work", "b2n8", -day+time.Minute),
		pooledArrived("0b000003", "work", "", -day+2*time.Minute),
		pooledArrived("0a000001", "k3f9", "q7mt", -3*time.Minute), pooledArrived("0a000002", "work", "b2n8", -2*time.Minute),
	}
	tests := []struct {
		name     string
		read     views.PoolRead
		requests int
	}{
		{name: "Acme's", read: acmeRead, requests: 2},
		{name: "Default's", read: firstRead, requests: 3},
		{name: "every pool's", read: views.PoolRead{}, requests: 5},
	}
	for _, view := range []dashboard.View{dashboard.History, dashboard.Graphs} {
		for _, tt := range tests {
			t.Run(string(view)+", "+tt.name, func(t *testing.T) {
				state := t.TempDir()
				writeLines(t, state, lines...)
				h := newHarness(t, probedWithoutTheRouter())
				h.model.cfg.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, views.DigestVersion, logs.For("watch"))
				h.model.cfg.PoolRead, h.model.view = tt.read, view
				h.model.graphs = dashboard.GraphsSectionsNamed([]string{"when"})
				h.start()

				if !h.model.past.last.held() {
					t.Fatalf("%s read no days", view)
				}
				if got := requestsHeld(h); got != tt.requests {
					t.Errorf("%s's days hold %d requests, want %d, the pool's", view, got, tt.requests)
				}
			})
		}
	}
}

// A pool added to a ledger weeks old begins where its work begins: Acme,
// whose first request was ten days ago, Default working every day of five
// weeks. WHEN YOU WORK counts Acme's ten days alone, two weeks, none of the
// days before it as days of none, and History begins on its first day;
// Default's and every pool's count the four weeks, beginning where the
// ledger began.
func TestAPoolsDaysBeginWhereItsWorkBegins(t *testing.T) {
	var lines []ledger.Held
	for back := 1; back <= 35; back++ {
		lines = append(lines, pooledArrived(fmt.Sprintf("0b%06d", back), "work", "b2n8", -time.Duration(back)*day))
		if back <= 10 {
			lines = append(lines, pooledArrived(fmt.Sprintf("0a%06d", back), "k3f9", "q7mt", -time.Duration(back)*day))
		}
	}
	tests := []struct {
		name         string
		read         views.PoolRead
		began        int
		known, weeks int
	}{
		{name: "Acme's", read: acmeRead, began: 10, known: 10, weeks: 2},
		{name: "Default's", read: firstRead, began: 35, known: 28, weeks: 4},
		{name: "every pool's", read: views.PoolRead{}, began: 35, known: 28, weeks: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, lines...)
			h := newHarness(t, probedWithoutTheRouter())
			h.model.cfg.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, views.DigestVersion, logs.For("watch"))
			h.model.cfg.PoolRead, h.model.view = tt.read, dashboard.Graphs
			h.model.graphs = dashboard.GraphsSectionsNamed([]string{"when"})
			h.start()

			if want := dayfile.DayStart(past(-time.Duration(tt.began)*day).Local(), 0); !h.model.past.began.Equal(want) {
				t.Errorf("History begins %v, want %v, where the pool's work begins", h.model.past.began, want)
			}
			hours := h.model.past.last.past.Hours()
			known := 0
			for _, d := range hours.Days {
				known += d.Known
			}
			if known != tt.known || hours.Weeks != tt.weeks {
				t.Errorf("WHEN YOU WORK counts %d days over %d weeks, want %d over %d", known, hours.Weeks, tt.known, tt.weeks)
			}
		})
	}
}

// followedOn has h's dashboard read the request ledger in state as the
// dashboard's Turn makes it, on the grounds of the turn, as freshIn asks for
// one: following the files there, knowing the grounds' first pool, as
// idmove.FirstPool has the dashboard's know the one the accounts record
// notes; the one it reads now made so on the grounds it has.
func followedOn(h *harness, state string) {
	on := func(g ledger.Grounds) func() views.Ledger {
		return func() views.Ledger {
			return ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, views.DigestVersion, logs.For("watch"), ledger.FirstPool(func() string { return g.First }))
		}
	}
	turn := h.model.cfg.Turn
	h.model.cfg.Ledger = on(h.model.cfg.LedgerGrounds)()
	h.model.cfg.Turn = func(ctx context.Context, from string, ask PoolAsk) (Pooled, error) {
		pooled, err := turn(ctx, from, ask)
		pooled.NewLedger = on(pooled.Grounds)
		return pooled, err
	}
}

// digestedAsTheRouter has the days before today of the request ledger in
// state summarised and digested as the router digests them, at h's now, its
// first pool the one with the id first, failing where the day before
// today's digest isn't written.
func digestedAsTheRouter(t *testing.T, h *harness, state, first string) {
	t.Helper()
	quiet := slog.New(slog.DiscardHandler)
	l := ledger.Open(ledger.Dir(state), 400*day, h.clock.Now, ledger.HistoryIn(readings.Files(readings.Dir(state), quiet)), ledger.Caps{Shared: policy.Shared}, quiet,
		ledger.Digesting(views.Digests{}), ledger.FirstPool(func() string { return first }))
	l.SummariseEnded(h.clock.Now())
	l.DigestEnded(t.Context(), h.clock.Now())
	yesterday := dayfile.DateOf(dayfile.DayStart(h.clock.Now(), -1))
	if found, _ := filepath.Glob(filepath.Join(ledger.Dir(state), "digest-"+yesterday+"*")); len(found) == 0 {
		t.Fatalf("no digest of %s was written", yesterday)
	}
}

// A turn, P or a fresh start as the config changes, has every tab read the
// pool turned to, as the turn reads it, its first pool among it, through the
// ledger in use after it: History's days and where they begin, which Runway,
// Accounts and Graphs share, Sessions, and a session's page, whose reads the
// routing card prices its moves by, never those of the pool or the first
// pool before. The lines naming none, 7f3a's five days ago and today, and
// 9e21's beside Default's yesterday, are the first pool's: Default's, till
// Acme is put first, the accounts record noting no first pool, and Acme's
// then. The router summarised and digested the days before today with
// Default first. So 9e21's entry of yesterday holds one state of both its
// lines: a read with Acme first, holding no state of its alone, reads its
// lines whole, its page in Default its Default request of yesterday and of
// today alone. And the summaries of those days hold their work in no pool
// as Default's, as a summary keeps the first pool it was made with (see
// Pools' What pools can't hold): History's days before today are Default's
// still, today's as the ledger the turn made, on Acme first, summarises
// them.
func TestATurnReadsEveryTabOfThePoolTurnedTo(t *testing.T) {
	everyAccount := slices.Concat(defaultAccounts, acmeAccounts)
	unpooled := config.Accounts{{ID: "work", Label: "Work"}, {ID: "personal", Label: "Personal"}, {ID: "side", Label: "Side"}, {ID: "k3f9", Label: "Acme 1"}}
	pressed := func(key string) func(*harness, *poolTurns) {
		return func(h *harness, _ *poolTurns) { h.deliver(h.press(key)...) }
	}
	changed := func(change func(p *poolTurns)) func(*harness, *poolTurns) {
		return func(h *harness, p *poolTurns) {
			p.changeConfig(change)
			h.deliver(h.press("R")...)
		}
	}
	pooled := changed(func(p *poolTurns) { p.pools, p.accounts = testPools, everyAccount })
	tests := []struct {
		name     string
		pools    config.Pools
		accounts config.Accounts
		opened   string
		noPool   bool
		turn     func(h *harness, p *poolTurns)
		pool     string
		listed   map[string]int
		page     int
		requests int
		began    time.Duration
	}{
		{
			name: "P, Default to Acme", pools: testPools, accounts: everyAccount, opened: "b2n8", turn: pressed("P"),
			pool: "q7mt", listed: map[string]int{idD28C: 1, id9E21: 2}, page: 2, requests: 3,
		},
		{
			name: "Acme put first, the record noting no first pool", pools: testPools, accounts: everyAccount, opened: "b2n8",
			turn: changed(func(p *poolTurns) { p.pools = config.Pools{testPools[1], testPools[0]} }),
			pool: "b2n8", listed: map[string]int{id9E21: 1}, page: 2, requests: 4, began: 5 * day,
		},
		{
			name: "the config naming pools, opened without", accounts: unpooled, turn: pooled,
			pool: "b2n8", listed: map[string]int{id7F3A: 1, id9E21: 1}, page: 3, requests: 5, began: 5 * day,
		},
		{
			name: "the pools told, opened where they couldn't be", accounts: unpooled, noPool: true, turn: pooled,
			pool: "b2n8", listed: map[string]int{id7F3A: 1, id9E21: 1}, page: 3, requests: 5, began: 5 * day,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			before := []ledger.Held{
				ofSession(arrived("0c000001", "work", -5*day), id7F3A, ""),
				ofSession(arrived("0b000001", "work", -day), id9E21, "b2n8"), ofSession(arrived("0b000002", "work", -day+time.Minute), id9E21, ""),
			}
			writeLines(t, state, slices.Concat(before, pooledToday)...)
			h := newHarness(t, probedWithoutTheRouter())
			digestedAsTheRouter(t, h, state, "b2n8")
			pools := newPoolTurns(t, tt.pools, tt.accounts, map[string]Source{"": h.source, "b2n8": h.source, "q7mt": h.source})
			h.model.cfg = pools.opened(h.model.cfg, tt.opened)
			followedOn(h, state)
			if tt.noPool {
				h.model.cfg.PoolRead = views.NoPool()
			}
			h.model.view = dashboard.History
			h.start()

			turns := h.model.poolTurns
			tt.turn(h, pools)
			if h.model.poolTurns != turns+1 || h.model.cfg.Pool.ID != tt.pool {
				t.Fatalf("the turn started the view afresh %d times, on %q; want once, on %q", h.model.poolTurns-turns, h.model.cfg.Pool.ID, tt.pool)
			}
			if got := requestsHeld(h); got != tt.requests {
				t.Errorf("History's days hold %d requests, want %d, the pool's", got, tt.requests)
			}
			if want := dayfile.DayStart(past(-tt.began).Local(), 0); !h.model.past.began.Equal(want) {
				t.Errorf("History begins %v, want %v, where the pool's work begins", h.model.past.began, want)
			}
			h.keys(tabKey)
			if got := listedRequests(h); !maps.Equal(got, tt.listed) {
				t.Errorf("Sessions lists %v, want %v, the pool's requests alone", got, tt.listed)
			}
			h.openPage(id9E21)
			if got := h.shownPage().Page.Totals.Requests; got != tt.page {
				t.Errorf("9e21's page counts %d requests, want %d, its requests in the pool", got, tt.page)
			}
		})
	}
}
