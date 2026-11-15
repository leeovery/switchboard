package watch

import (
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// Every document a read lands names the accounts removed, as the watch is
// configured with them, so the Log, Detail and every other look name one by
// the label it showed last.
func TestADocumentReadNamesTheAccountsRemoved(t *testing.T) {
	h := newHarness(t, routerDocument(three()...))
	h.model.cfg.Removed = status.RemovedAccounts{{ID: "h7c2", Label: "team", Removed: start}}
	h.deliver(h.update(fetchedMsg{doc: routerDocument(three()...)})...)

	if got := h.model.moment(h.clock.now).Named("h7c2"); got.Label != "team" || !got.Other {
		t.Errorf("the moment names h7c2 %+v, want team, not configured", got)
	}
}

// Every document a read lands names the accounts in another pool than the
// one shown, as the watch is configured with them, so the Log, the session's
// page and every other look name one moved out by its label, not of the
// pool, and where it is now.
func TestADocumentReadNamesTheAccountsMoved(t *testing.T) {
	h := newHarness(t, routerDocument(three()...))
	h.model.cfg.Moved = status.MovedAccounts{{ID: "h7c2", Label: "team", Pool: "q7mt", PoolLabel: "Acme"}}
	h.deliver(h.update(fetchedMsg{doc: routerDocument(three()...)})...)

	want := status.AccountName{Label: "team", Other: true, Moved: "now in Acme"}
	if got := h.model.moment(h.clock.now).Named("h7c2"); got != want {
		t.Errorf("the moment names h7c2 %+v, want %+v", got, want)
	}
}

// P turns to a pool with the accounts in another pool than it, as its turn
// gives them: Acme's moments name Default's accounts as now in Default, and
// Default's, turned back to, name Acme's as now in Acme.
func TestPTurnsToThePoolsAccountsMoved(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeAccounts)
	h.start()
	h.deliver(h.press("P")...)
	if got, want := h.model.moment(h.clock.now).Named("work"), (status.AccountName{Label: "Work", Other: true, Moved: "now in Default"}); got != want {
		t.Errorf("in Acme, the moment names work %+v, want %+v", got, want)
	}
	h.deliver(h.press("P")...)
	if got, want := h.model.moment(h.clock.now).Named("k3f9"), (status.AccountName{Label: "Acme 1", Other: true, Moved: "now in Acme"}); got != want {
		t.Errorf("back in Default, the moment names k3f9 %+v, want %+v", got, want)
	}
}
