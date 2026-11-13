package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// acmeRead reads Acme, q7mt, where work is now, Default, b2n8, the first
// pool, holding side; the accounts record keeps lapsed, removed from Acme.
var acmeRead = views.PoolRead{
	ID: "q7mt", First: "b2n8",
	PoolOf: status.AccountPools(config.Accounts{{ID: "work", Pool: "q7mt"}, {ID: "side", Pool: "b2n8"}}, func() identity.Record {
		return identity.Record{Accounts: []identity.Entry{{ID: "lapsed", Pool: "q7mt"}}}
	}),
}

// A pool read shows an event as status.Event's InPool has it, whose own
// tests hold the rule whole: these hold that the read asks it, of its pool,
// its first and its accounts' pools.
func TestAPoolReadShowsTheEventsOfThePoolAsTheEventsRuleHasIt(t *testing.T) {
	tests := []struct {
		name  string
		event status.Event
	}{
		{name: "a start in the pool", event: status.Event{Kind: status.EventStarted, Account: "side", Pool: "q7mt"}},
		{name: "a start in another, on an account in the pool now", event: status.Event{Kind: status.EventStarted, Account: "work", Pool: "b2n8"}},
		{name: "a start in a pool since removed", event: status.Event{Kind: status.EventStarted, Account: "work", Pool: "x9z9"}},
		{name: "a pin naming no pool, from before pins named theirs", event: status.Event{Kind: status.EventPin, Account: "work", Accounts: []string{"work"}}},
		{name: "a move naming no pool, from before pools", event: status.Event{Kind: status.EventMoved, From: "side", To: "work"}},
		{name: "a limit of an account in the pool now", event: status.Event{Kind: status.EventLimit, Account: "work"}},
		{name: "a limit of an account in another pool now", event: status.Event{Kind: status.EventLimit, Account: "side"}},
		{name: "a refusal of an account removed from the pool", event: status.Event{Kind: status.EventRefused, Account: "lapsed"}},
		{name: "a limit of an account none places", event: status.Event{Kind: status.EventLimit, Account: "gone"}},
		{name: "the router's own", event: status.Event{Kind: status.EventRestart}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.event.InPool(acmeRead.ID, acmeRead.First, acmeRead.PoolOf)
			if got := acmeRead.Shows(tt.event); got != want {
				t.Errorf("Shows(%+v) = %v, want %v, as InPool has it", tt.event, got, want)
			}
			if got := acmeRead.ShowsHeld(events.Held{Event: tt.event}); got != want {
				t.Errorf("ShowsHeld(%+v) = %v, want %v, as Shows has it", tt.event, got, want)
			}
			if got := (views.PoolRead{}).Shows(tt.event); !got {
				t.Errorf("a read of every pool's Shows(%+v) = false, want every event shown", tt.event)
			}
		})
	}
	// The rule's answers, as the read gives them, for its own pool.
	shown := map[string]bool{
		"a start in the pool": true, "a limit of an account in the pool now": true,
		"a refusal of an account removed from the pool": true, "a limit of an account none places": true, "the router's own": true,
	}
	for _, tt := range tests {
		if got := acmeRead.Shows(tt.event); got != shown[tt.name] {
			t.Errorf("%s: Acme's read shows it %v, want %v", tt.name, got, shown[tt.name])
		}
	}
}

// A pool's days hold the accounts that worked in it that day, each with its
// work there alone, and of those idle in it, the ones it holds alone, as
// PoolOf places them; the days the reader read stay as they were.
func TestAPoolsDaysHoldItsWorkAndItsOwnIdleAccounts(t *testing.T) {
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	whole := ledger.Summary{Day: "2026-11-03", Accounts: []ledger.AccountDay{
		{Account: "work"},
		{Account: "side", Sessions: 3, Pools: []ledger.PoolDay{
			{Pool: "b2n8", Sessions: 1}, {Pool: "q7mt", Sessions: 2},
		}},
		{Account: "lapsed"}, {Account: "gone"}, {Account: "spare", Pool: "b2n8", Sessions: 4}, {},
	}}
	read := givenDays{whole}
	days := acmeRead.Days(read).Days(day)
	if len(days) != 1 {
		t.Fatalf("Acme's days are %d, want 1", len(days))
	}
	sessions := map[string]int{}
	var kept []string
	for _, a := range days[0].Accounts {
		kept = append(kept, a.Account)
		sessions[a.Account] = a.Sessions
	}
	if want := []string{"work", "side", "lapsed"}; !slices.Equal(kept, want) {
		t.Errorf("Acme's day holds %q, want %q: side, which worked in it, and of those idle, the one it holds and the removed one the record keeps in it", kept, want)
	}
	if sessions["side"] != 2 {
		t.Errorf("side's day in Acme counts %d sessions, want its 2 there alone", sessions["side"])
	}
	if len(read[0].Accounts) != 6 || read[0].Accounts[1].Sessions != 3 {
		t.Errorf("the reader's day holds %d accounts, side's %d sessions, once Acme's was read; want its 6 as they were, side's 3",
			len(read[0].Accounts), read[0].Accounts[1].Sessions)
	}
	if every := (views.PoolRead{}).Days(read).Days(day); len(every[0].Accounts) != 6 {
		t.Errorf("every pool's day holds %d accounts, want all 6", len(every[0].Accounts))
	}
}

// givenDays is a reader of the days given, whatever is asked.
type givenDays []ledger.Summary

func (d givenDays) Days(time.Time) []ledger.Summary { return d }
