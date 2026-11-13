package watch

import (
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
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
