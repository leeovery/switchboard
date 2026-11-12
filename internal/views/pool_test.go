package views_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// acmeRead reads Acme, q7mt, where work is now, Default, b2n8, the first
// pool, holding side.
var acmeRead = views.PoolRead{
	ID: "q7mt", First: "b2n8",
	In:     func(account string) bool { return account == "work" },
	Pooled: func(account string) bool { return account == "work" || account == "side" },
}

func TestAPoolReadShowsTheEventsOfThePool(t *testing.T) {
	tests := []struct {
		name  string
		event status.Event
		want  bool
	}{
		{name: "a start in the pool", event: status.Event{Kind: status.EventStarted, Account: "side", Pool: "q7mt"}, want: true},
		{name: "a start in another, on an account in the pool now", event: status.Event{Kind: status.EventStarted, Account: "work", Pool: "b2n8"}},
		{name: "a pin naming no pool, from before pins named theirs: the first pool's", event: status.Event{Kind: status.EventPin, Account: "work", Accounts: []string{"work"}}},
		{name: "a move naming no pool, from before pools: the first pool's", event: status.Event{Kind: status.EventMoved, From: "side", To: "work"}},
		{name: "a limit of an account in the pool now", event: status.Event{Kind: status.EventLimit, Account: "work"}, want: true},
		{name: "a limit of an account in another pool now", event: status.Event{Kind: status.EventLimit, Account: "side"}},
		{name: "a limit of an account in no pool, as one removed", event: status.Event{Kind: status.EventLimit, Account: "gone"}, want: true},
		{name: "the router's own", event: status.Event{Kind: status.EventRestart}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acmeRead.Shows(tt.event); got != tt.want {
				t.Errorf("Shows(%+v) = %v, want %v", tt.event, got, tt.want)
			}
			if got := (views.PoolRead{}).Shows(tt.event); !got {
				t.Errorf("a read of every pool's Shows(%+v) = false, want every event shown", tt.event)
			}
		})
	}
}
