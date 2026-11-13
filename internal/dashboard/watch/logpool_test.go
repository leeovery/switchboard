package watch

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The reads of the request ledger of these tests' pools: Acme's, and the
// first pool's, Default, where work naming no pool is.
var (
	acmeRead  = views.PoolRead{ID: "q7mt", First: "b2n8", PoolOf: status.AccountPools(slices.Concat(defaultAccounts, acmeAccounts), nil)}
	firstRead = views.PoolRead{ID: "b2n8", First: "b2n8", PoolOf: status.AccountPools(slices.Concat(defaultAccounts, acmeAccounts), nil)}
)

// pooledArrived is arrived, routed in the pool with the given id, "" for
// none.
func pooledArrived(request, account, pool string, d time.Duration) ledger.Held {
	l := arrived(request, account, d)
	l.Pool = pool
	return l
}

// pooledDays are the lines of today and the day before of Acme, of Default
// and naming no pool, as before pools, so Default's.
var pooledDays = []ledger.Held{
	pooledArrived("0b000001", "k3f9", "q7mt", -day), pooledArrived("0b000002", "work", "b2n8", -day+time.Minute),
	pooledArrived("0b000003", "work", "", -day+2*time.Minute),
	pooledArrived("0a000001", "k3f9", "q7mt", -3*time.Minute), pooledArrived("0a000002", "work", "b2n8", -2*time.Minute),
	pooledArrived("0a000003", "work", "", -time.Minute),
}

// The Log shows the requests of the pool shown alone, as its pool's read
// keeps them, today's as it opens and as a look gains them, and the day
// before's as it reads back, with the figures of the pool's work that day:
// Acme's its own, Default's those naming it and none, every pool's all, and
// none's none.
func TestTheLogShowsThePoolShownsRequestsAlone(t *testing.T) {
	tests := []struct {
		name     string
		read     views.PoolRead
		want     []string
		requests int
	}{
		{name: "Acme's", read: acmeRead, want: []string{"0b000001", "0a000001", "0a000004"}, requests: 1},
		{name: "Default's", read: firstRead, want: []string{"0b000002", "0b000003", "0a000002", "0a000003", "0a000005"}, requests: 2},
		{name: "every pool's", read: views.PoolRead{}, want: []string{
			"0b000001", "0b000002", "0b000003", "0a000001", "0a000002", "0a000003", "0a000004", "0a000005",
		}, requests: 3},
		{name: "no pool's", read: views.NoPool()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, pooledDays...)
			h := newHarness(t, probedWithoutTheRouter())
			onTheLog(h, state)
			h.model.cfg.LedgerKeep, h.model.cfg.PoolRead = 2*day, tt.read
			h.start()

			writeLines(t, state, pooledArrived("0a000004", "k3f9", "q7mt", -30*time.Second), pooledArrived("0a000005", "work", "", -20*time.Second))
			h.read(probedWithoutTheRouter())
			if got := shownRequests(h); !slices.Equal(got, tt.want) {
				t.Errorf("the Requests show %v, want %v", got, tt.want)
			}
			yesterday := dayfile.DateOf(past(-day))
			i := slices.IndexFunc(h.model.log.totals, func(d views.DayTotals) bool { return d.Date == yesterday })
			if requests := 0; i >= 0 {
				requests = h.model.log.totals[i].Requests
				if requests != tt.requests {
					t.Errorf("the figures of the day before are of %d requests, want %d, the pool's", requests, tt.requests)
				}
			} else if tt.requests > 0 {
				t.Errorf("the figures held are %+v, want the day before's", h.model.log.totals)
			}
		})
	}
}

// The find finds the sessions of the pool shown alone, walking back only
// the days whose summaries in the pool name one: api, in Acme three days
// back, and api-docs, in Default the day before, "api" finding each in its
// own pool alone.
func TestTheFindFindsThePoolShownsSessionsAlone(t *testing.T) {
	inPool := func(l ledger.Held, pool string) ledger.Held {
		l.Pool = pool
		return l
	}
	tests := []struct {
		name   string
		read   views.PoolRead
		walked []string
		found  []string
	}{
		{name: "Acme's", read: acmeRead, walked: []string{dayfile.DateOf(past(-3 * day))}, found: []string{apiSession + " ~/Code/api"}},
		{name: "Default's", read: firstRead, walked: []string{dayfile.DateOf(past(-day))}, found: []string{docsSession + " ~/Code/api-docs"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, inPool(ranIn("0a000001", apiSession, "~/Code/api", -3*day), "q7mt"), inPool(ranIn("0a000002", docsSession, "~/Code/api-docs", -day), ""))
			h := newHarness(t, probedWithoutTheRouter())
			l := onTheLog(h, state)
			h.model.cfg.PoolRead = tt.read
			h.start()
			opened := len(l.days)

			h.typed("/", "a", "p", "i")
			if walked := l.days[opened:]; !slices.Equal(walked, tt.walked) {
				t.Errorf("the find read the days %v, want %v, the pool's alone", walked, tt.walked)
			}
			if got := foundIDs(h); !slices.Equal(got, tt.found) {
				t.Errorf("api finds %q, want %q", got, tt.found)
			}
		})
	}
}

// The Log reads back no further than the days' summaries in the pool shown
// say its lines reach, though the ledger keeps a week: Acme's to the day
// before, its first, though Default's go back three days, and Default's to
// its own first.
func TestTheLogReadsBackNoFurtherThanThePoolShownsLinesReach(t *testing.T) {
	tests := []struct {
		name string
		read views.PoolRead
		from time.Duration
	}{
		{name: "Acme's", read: acmeRead, from: -day},
		{name: "Default's", read: firstRead, from: -3 * day},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, pooledArrived("0c000001", "work", "", -3*day), pooledArrived("0b000001", "k3f9", "q7mt", -day),
				pooledArrived("0a000001", "k3f9", "q7mt", -time.Minute))
			h := newHarness(t, probedWithoutTheRouter())
			onTheLog(h, state)
			h.model.cfg.LedgerKeep, h.model.cfg.PoolRead = 7*day, tt.read
			h.start()
			if from, want := h.model.log.from.Date, dayfile.DateOf(past(tt.from)); from != want {
				t.Errorf("the Log spans from %s, want %s, the first day of the pool's lines", from, want)
			}
		})
	}
}

// After P, the Log reads the next pool's requests afresh, as the view
// starts afresh: none of the pool before carried into it.
func TestPTurnsTheLogToTheNextPoolsRequests(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, pooledDays...)
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	pools := &poolTurns{order: []string{"q7mt", "b2n8"}, pools: map[string]Pooled{
		"q7mt": {Pool: acmeShown, Accounts: acmeAccounts, Source: h.source, Events: events.Empty{}, Read: acmeRead},
		"b2n8": {Pool: defaultShown, Accounts: defaultAccounts, Source: h.source, Events: events.Empty{}, Read: firstRead},
	}}
	h.model.cfg.Pool, h.model.cfg.Accounts, h.model.cfg.Turn, h.model.cfg.PoolRead = acmeShown, acmeAccounts, pools.turn, acmeRead
	h.model.cfg.LedgerKeep = 2 * day
	h.start()
	if got, want := shownRequests(h), []string{"0b000001", "0a000001"}; !slices.Equal(got, want) {
		t.Fatalf("in Acme, the Requests show %v, want Acme's %v", got, want)
	}

	h.deliver(h.press("P")...)
	if got, want := shownRequests(h), []string{"0b000002", "0b000003", "0a000002", "0a000003"}; h.model.cfg.Pool.ID != "b2n8" || !slices.Equal(got, want) {
		t.Errorf("after P, in %q, the Requests show %v, want Default's %v", h.model.cfg.Pool.ID, got, want)
	}
}
