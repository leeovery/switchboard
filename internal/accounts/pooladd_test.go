package accounts_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
)

// pooled is a config of work in pool b2n8, Default, and of the pools given
// after it, each a [[pool]] table as written.
func pooled(pools ...string) string {
	return fmt.Sprintf("upstream = %q\n\n[[account]]\nid    = \"work\"\nlabel = \"Work\"\npool  = \"b2n8\"\n\n", upstream) + strings.Join(pools, "\n")
}

const (
	defaultPool = "[[pool]]\nid    = \"b2n8\"\nlabel = \"Default\"\n"
	fencedPool  = "[[pool]]\nid    = \"b2n8\"\nlabel = \"Default\"\ndirs  = [\"/Code/work\"]\n"
	acmePool    = "[[pool]]\nid    = \"q7mt\"\nlabel = \"Acme\"\ndirs  = [\"/Code/acme\"]\n"
)

// An account added naming no pool goes in the pool the verbs act on, as
// accounts add adds one without --pool: the only pool, fenced or not, else
// the unfenced one; one naming its pool goes in it. CheckNew says so before
// anything's taken, and Add adds it there.
func TestAnAccountNamingNoPoolGoesInThePoolTheVerbsActOn(t *testing.T) {
	tests := []struct {
		name   string
		config string
		named  string
		want   string
	}{
		{name: "the only pool", config: pooled(defaultPool), want: "b2n8"},
		{name: "the only pool, fenced", config: pooled(fencedPool), want: "b2n8"},
		{name: "the unfenced pool of two", config: pooled(acmePool, defaultPool), want: "b2n8"},
		{name: "the pool it names", config: pooled(defaultPool, acmePool), named: "q7mt", want: "q7mt"},
		{name: "none, in a config without pools", config: twoAccounts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			registry := w.registry(&user{gives: sideToken}, &fakeAPI{organization: organization})
			account := config.NewAccount{Label: "Side", Pool: tt.named}

			checked, err := registry.CheckNew(account)
			if err != nil || checked.Pool != tt.want {
				t.Fatalf("CheckNew() = %+v, %v, want it in pool %q", checked, err, tt.want)
			}
			w.checkConfig(t, tt.config)

			added, err := registry.Add(t.Context(), account, nil)
			if err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			got, _ := w.load(t).Accounts.ByID(added.ID)
			if got.Pool != tt.want {
				t.Errorf("the account was added in pool %q, want %q", got.Pool, tt.want)
			}
		})
	}
}

// Where every pool is fenced, an account naming no pool has none to go in:
// CheckNew refuses it, naming the pools, and Add refuses it before it asks
// for the token, saving nothing.
func TestAnAccountNamingNoPoolIsRefusedWhereEveryPoolIsFenced(t *testing.T) {
	fenced := pooled(fencedPool, acmePool)
	w := newWorld(t, fenced)
	user, api := &user{atTerminal: true, gives: sideToken}, &fakeAPI{organization: organization}
	registry := w.registry(user, api)
	account := config.NewAccount{Label: "Side"}

	_, err := registry.CheckNew(account)
	needed, ok := errors.AsType[*accounts.PoolNeededError](err)
	if want := "every pool is fenced, so which the account goes in is yours to say: add it with switchboard accounts add <label> --pool <id>, naming Default (b2n8) or Acme (q7mt)"; !ok || err.Error() != want {
		t.Fatalf("CheckNew() error = %v, want %q", err, want)
	}
	if len(needed.Pools) != 2 {
		t.Errorf("the error names pools %+v, want both", needed.Pools)
	}

	if _, err := registry.Add(t.Context(), account, nil); !isPoolNeeded(err) {
		t.Errorf("Add() error = %v, want a PoolNeededError", err)
	}
	if user.asked || len(api.asked) > 0 {
		t.Errorf("the user was asked for the token (%t), the API asked %q: want neither", user.asked, api.asked)
	}
	w.checkConfig(t, fenced)
	w.checkNoRecord(t)
}

// isPoolNeeded reports whether err is a PoolNeededError.
func isPoolNeeded(err error) bool {
	_, ok := errors.AsType[*accounts.PoolNeededError](err)
	return ok
}
