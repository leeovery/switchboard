package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// A session's auto is where the router would send its next request were its
// own pin cleared, as it lists the session: of the model naming it, never
// another model's; none where it lists the session not, or can't say.
func TestASessionsAutoIsTheRoutersOfTheModelNamingIt(t *testing.T) {
	seat := func(model, account, auto string, seen time.Duration) status.Assignment {
		return status.Assignment{Model: model, Account: account, Auto: auto, Reason: "sticky", LastSeen: now.Add(-seen)}
	}
	listed := func(assignments ...status.Assignment) *status.Session {
		return &status.Session{ID: paged, Pin: "personal", Assignments: assignments}
	}
	tests := []struct {
		name   string
		routed *status.Session
		lines  []ledger.Line
		model  string
		want   string
	}{
		{name: "its one model's", routed: listed(seat(opus, "personal", "side", 0)), model: opus, want: "side"},
		{name: "the model seen last naming it, not another's",
			routed: listed(seat(opus, "personal", "side", 0), seat(haiku, "personal", "work", time.Minute)), model: opus, want: "side"},
		{name: "the other model, seen last, naming it",
			routed: listed(seat(opus, "personal", "side", time.Minute), seat(haiku, "personal", "work", 0)), model: haiku, want: "work"},
		{name: "none where the router can't say, though another model's it can",
			routed: listed(seat(opus, "personal", "", 0), seat(haiku, "personal", "work", time.Minute)), model: opus},
		{name: "none where the router lists it not", lines: []ledger.Line{line(paged, opus, "personal", "new", local(13, 0, 0), hourUsage)}, model: opus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, ok := views.SessionPageOf(paged, views.PageSources{Routed: tt.routed, Read: readOf(summarisedLedger(tt.lines), tt.routed), Prices: ledger.Pricing, Now: now})
			if !ok {
				t.Fatal("SessionPageOf() found no page")
			}
			if page.Model != tt.model || page.Auto != tt.want {
				t.Errorf("the page is named by %s, its auto %q; want %s, %q", page.Model, page.Auto, tt.model, tt.want)
			}
		})
	}
}
