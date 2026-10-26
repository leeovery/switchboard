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

// placesOf are the accounts' places, in their order.
func placesOf(accounts []status.AccountNow) []int {
	places := make([]int, len(accounts))
	for i, a := range accounts {
		places[i] = a.Place
	}
	return places
}

// An account is numbered by its place in the router's document wherever the
// document holds it, whatever its place in the config; one the document
// doesn't hold yet comes after its last, in the config's order among those
// it lacks; and with no document, the config's order numbers them all.
func TestAnAccountIsNumberedByTheRoutersPlaceElseAfterItsLast(t *testing.T) {
	trio := routerDocOf(readAt("work"), readAt("personal"), readAt("side"))
	tests := []struct {
		name string
		doc  status.Document
		ids  []string
		want []int
	}{
		{name: "the config and the router agree", doc: trio, ids: []string{"work", "personal", "side"}, want: []int{1, 2, 3}},
		{name: "one removed, still the router's", doc: trio, ids: []string{"work", "side"}, want: []int{1, 3}},
		{name: "one added, not yet the router's", doc: trio, ids: []string{"work", "personal", "side", "client"}, want: []int{1, 2, 3, 4}},
		{name: "one removed and one added", doc: trio, ids: []string{"work", "side", "client"}, want: []int{1, 3, 4}},
		{name: "two added among the rest", doc: trio, ids: []string{"spare", "work", "client", "side"}, want: []int{4, 1, 5, 3}},
		{name: "the config reordered", doc: trio, ids: []string{"side", "work", "personal"}, want: []int{3, 1, 2}},
		{name: "no document", ids: []string{"work", "personal", "side"}, want: []int{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.doc.At(built, policy)
			configured := configuredOf(tt.ids...)
			if got := placesOf(m.Configured(configured)); !slices.Equal(got, tt.want) {
				t.Errorf("the moment numbers %v as %v, want %v", tt.ids, got, tt.want)
			}
			if got := placesOf(m.Held().Numbering().Configured(configured)); !slices.Equal(got, tt.want) {
				t.Errorf("the moment, held, numbers %v as %v, want %v", tt.ids, got, tt.want)
			}
			if got := placesOf(tt.doc.HoldAt(built, policy).Numbering().Configured(configured)); !slices.Equal(got, tt.want) {
				t.Errorf("the document, held at a moment, numbers %v as %v, want %v", tt.ids, got, tt.want)
			}
		})
	}
}

// The moment's own accounts are numbered by the one rule: each one's Place
// is the number its configured self takes.
func TestTheMomentsAccountsAreNumberedAsTheirConfiguredSelves(t *testing.T) {
	m := routerDocOf(readAt("work"), readAt("personal"), readAt("side")).At(built, policy)
	configured := m.Configured(configuredOf("side", "client", "work", "personal"))
	for _, a := range m.Accounts {
		i := slices.IndexFunc(configured, func(c status.AccountNow) bool { return c.ID == a.ID })
		if a.Place != configured[i].Place {
			t.Errorf("%s is the moment's %d, but configured, %d", a.ID, a.Place, configured[i].Place)
		}
	}
}

// A configured account the moment holds is as the moment has it; one it
// doesn't hold yet is as the config names and marks it, nothing read of it.
func TestAConfiguredAccountIsTheMomentsElseTheConfigs(t *testing.T) {
	m := routerDocOf(readAt("work"), readAt("side")).At(built, policy)
	configured := configuredOf("work", "client")
	configured[1].Label, configured[1].Primary = "a client", true
	got := m.Configured(configured)
	if work, _ := m.Account("work"); !got[0].ReadAt.Equal(work.ReadAt) || !got[0].Read || got[0].Place != 1 {
		t.Errorf("work is %+v, want the moment's, read at %v, numbered 1", got[0], work.ReadAt)
	}
	client := got[1]
	if client.ID != "client" || client.Label != "a client" || !client.Primary || client.Read || client.TokenSet || client.Place != 3 {
		t.Errorf("client is %+v, want as the config names and marks it, nothing read, numbered 3", client)
	}
}

// A frame holding no moment numbers the configured accounts in the
// config's order, as no document does.
func TestNoMomentHeldNumbersTheConfigsOrder(t *testing.T) {
	var held *status.HeldMoment
	got := placesOf(held.Numbering().Configured(configuredOf("side", "work")))
	if want := []int{1, 2}; !slices.Equal(got, want) {
		t.Errorf("no moment held numbers side and work as %v, want %v", got, want)
	}
	if held.Numbering().Held() != 0 {
		t.Errorf("no moment held holds %d accounts, want 0", held.Numbering().Held())
	}
}
