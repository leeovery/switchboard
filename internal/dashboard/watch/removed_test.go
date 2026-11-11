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
