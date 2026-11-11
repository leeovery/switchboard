package status_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
)

// configuredOf are the configured accounts with the ids given, in that
// order, each labelled by its id.
func configuredOf(ids ...string) config.Accounts {
	accounts := make(config.Accounts, len(ids))
	for i, id := range ids {
		accounts[i] = config.Account{ID: id, Label: id}
	}
	return accounts
}

// idsOf are the accounts' ids, in their order.
func idsOf(accounts []status.AccountNow) []string {
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	return ids
}

// The configured accounts keep the config's order, whatever the router's
// document holds and in what order, and with no document at all.
func TestTheConfiguredAccountsKeepTheConfigsOrder(t *testing.T) {
	trio := routerDocOf(readAt("work"), readAt("personal"), readAt("side"))
	tests := []struct {
		name string
		doc  status.Document
		ids  []string
	}{
		{name: "the config and the router agree", doc: trio, ids: []string{"work", "personal", "side"}},
		{name: "one removed, still the router's", doc: trio, ids: []string{"work", "side"}},
		{name: "two added among the rest", doc: trio, ids: []string{"spare", "work", "client", "side"}},
		{name: "the config reordered", doc: trio, ids: []string{"side", "work", "personal"}},
		{name: "no document", ids: []string{"work", "personal", "side"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.doc.At(built, policy)
			if got := idsOf(m.Configured(configuredOf(tt.ids...))); !slices.Equal(got, tt.ids) {
				t.Errorf("the moment lists %v as %v, want the config's order", tt.ids, got)
			}
		})
	}
}

// A configured account the moment holds is as the moment has it, its place
// the document's; one it doesn't hold yet is as the config names and marks
// it, nothing read of it and placed nowhere.
func TestAConfiguredAccountIsTheMomentsElseTheConfigs(t *testing.T) {
	m := routerDocOf(readAt("work"), readAt("side")).At(built, policy)
	configured := configuredOf("side", "client")
	configured[1].Label, configured[1].Primary = "a client", true
	got := m.Configured(configured)
	if side, _ := m.Account("side"); !got[0].ReadAt.Equal(side.ReadAt) || !got[0].Read || got[0].Place != 2 {
		t.Errorf("side is %+v, want the moment's, read at %v, placed 2", got[0], side.ReadAt)
	}
	client := got[1]
	if client.ID != "client" || client.Label != "a client" || !client.Primary || client.Read || client.TokenSet || client.Place != 0 {
		t.Errorf("client is %+v, want as the config names and marks it, nothing read, placed nowhere", client)
	}
}
