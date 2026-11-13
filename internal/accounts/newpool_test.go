package accounts_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
)

// acmeMade is a new pool, Acme, fenced to /Code/acme, as setup's new pool
// card gives one.
var acmeMade = accounts.NewPool{Label: "Acme", Dirs: []string{"/Code/acme"}}

// An account added in a new pool makes the pool in the one edit that adds
// it, under an id switchboard draws, the account its first, so its primary,
// marked; CheckNew says so before the token's taken, writing nothing.
func TestAnAccountAddedInANewPoolMakesThePoolWithIt(t *testing.T) {
	was := pooled(defaultPool)
	w := newWorld(t, was)
	registry := w.unasked(t, &user{})
	adding := accounts.Adding{Account: config.NewAccount{Label: "Side"}, NewPool: &acmeMade}

	checked, err := registry.CheckNew(adding)
	if err != nil || !checked.Account.Primary || checked.Account.Pool != "" || checked.NewPool != &acmeMade {
		t.Fatalf("CheckNew() = %+v, %v; want it the new pool's primary, the pool kept to make", checked, err)
	}
	w.checkConfig(t, was)

	added, err := registry.AddChecked(t.Context(), adding, tokenOf(t, sideToken), accounts.Checked{Organization: organization}, nil, accounts.Answer{})
	if err != nil {
		t.Fatalf("AddChecked() error = %v", err)
	}
	cfg := w.load(t)
	pool, ok := cfg.Pools.ByID(randomID(t))
	if !ok || pool.Label != "Acme" || !slices.Equal(pool.Dirs, acmeMade.Dirs) {
		t.Fatalf("the config's pools are %+v, want Acme, fenced to /Code/acme, under the id drawn", cfg.Pools)
	}
	got, _ := cfg.Accounts.ByID(added.ID)
	if primary, _ := cfg.Accounts.PrimaryOf(pool.ID); got.Pool != pool.ID || primary.ID != added.ID || !got.Primary {
		t.Errorf("the account added is %+v, want it in the new pool, marked its primary", got)
	}
}

// The first account to go in a pool, in a config with pools, is marked its
// primary; one going in a pool with accounts isn't, nor one in a config
// without pools.
func TestTheFirstAccountInAPoolIsMarkedItsPrimary(t *testing.T) {
	empty := "[[pool]]\nid    = \"q7mt\"\nlabel = \"Acme\"\ndirs  = [\"/Code/acme\"]\n"
	tests := []struct {
		name, config, pool string
		want               bool
	}{
		{name: "a pool with no accounts", config: pooled(defaultPool, empty), pool: "q7mt", want: true},
		{name: "a pool with accounts", config: pooled(defaultPool, empty), pool: "b2n8"},
		{name: "a config without pools", config: twoAccounts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			checked, err := w.unasked(t, &user{}).CheckNew(accounts.Adding{Account: config.NewAccount{Label: "Side", Pool: tt.pool}})
			if err != nil || checked.Account.Primary != tt.want {
				t.Errorf("CheckNew() = %+v, %v; want primary %t", checked, err, tt.want)
			}
		})
	}
}

// A new pool the config's rules refuse, its label or its directories, is
// refused before the token's taken, by CheckNewPool, CheckNew and
// AddChecked alike, with a NewPoolError of AddPool's refusal, writing
// nothing.
func TestANewPoolTheRulesRefuseIsRefusedWritingNothing(t *testing.T) {
	tests := []struct {
		name string
		pool accounts.NewPool
		// is reports whether the refusal is the one wanted.
		is func(error) bool
	}{
		{name: "no label", pool: accounts.NewPool{Dirs: []string{"/Code/acme"}}, is: func(err error) bool { return errors.Is(err, accounts.ErrNoPoolLabel) }},
		{name: "another pool's label", pool: accounts.NewPool{Label: "default", Dirs: []string{"/Code/acme"}}, is: isA[*accounts.PoolLabelTakenError]},
		{name: "a second pool unfenced", pool: accounts.NewPool{Label: "Acme"}, is: isA[*accounts.UnfencedError]},
		{name: "a directory inside another pool's", pool: accounts.NewPool{Label: "Client", Dirs: []string{"/Code/client", "/Code/work/client"}}, is: func(err error) bool {
			refused, ok := errors.AsType[*config.DirError](err)
			return ok && refused.Dir == 1 && refused.Why == "is inside Default's /Code/work: no directory may be the same as, inside or around another pool's"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			was := pooled(fencedPool, defaultPoolUnfenced)
			w := newWorld(t, was)
			registry := w.unasked(t, &user{})
			adding := accounts.Adding{Account: config.NewAccount{Label: "Side"}, NewPool: &tt.pool}

			for name, err := range map[string]error{
				"CheckNewPool": registry.CheckNewPool(tt.pool),
				"CheckNew":     second(registry.CheckNew(adding)),
				"AddChecked":   second(registry.AddChecked(t.Context(), adding, tokenOf(t, sideToken), accounts.Checked{Organization: organization}, nil, accounts.Answer{})),
			} {
				if !isA[*accounts.NewPoolError](err) || !tt.is(err) {
					t.Errorf("%s() error = %v, want the new pool refused so", name, err)
				}
			}
			w.checkConfig(t, was)
			w.checkNoToken(t, sideID)
		})
	}
}

// defaultPoolUnfenced is a second pool, Rest, unfenced, beside fencedPool.
const defaultPoolUnfenced = "[[pool]]\nid    = \"r2d4\"\nlabel = \"Rest\"\n"

// Editing an account into a new pool makes the pool in the one edit that
// moves it, the pool it leaves told as Left tells it; a new pool refused
// moves nothing and writes nothing.
func TestEditMovesAnAccountIntoANewPool(t *testing.T) {
	was := pooled(defaultPool) + "\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\npool  = \"b2n8\"\n"
	w := newWorld(t, was)
	registry := w.unasked(t, &user{})

	if _, err := registry.Edit("work", accounts.Change{NewPool: &accounts.NewPool{Label: "default"}}); !isA[*accounts.NewPoolError](err) {
		t.Fatalf("Edit() into a new pool labelled as another error = %v, want a NewPoolError", err)
	}
	w.checkConfig(t, was)

	edited, err := registry.Edit("work", accounts.Change{NewPool: &acmeMade, Primary: true})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	cfg := w.load(t)
	pool, ok := cfg.Pools.ByID(randomID(t))
	if !ok || pool.Label != "Acme" {
		t.Fatalf("the config's pools are %+v, want Acme under the id drawn", cfg.Pools)
	}
	if work, _ := cfg.Accounts.ByID("work"); work.Pool != pool.ID || !work.Primary {
		t.Errorf("work is %+v, want it in Acme, marked its primary", work)
	}
	if edited.Left.Primary != "side" || edited.Now.Pool != pool.ID {
		t.Errorf("Edit() = left %+v, now %+v; want side Default's primary now, work in Acme", edited.Left, edited.Now)
	}
}

// isA reports whether err is, or wraps, an E.
func isA[E error](err error) bool {
	_, ok := errors.AsType[E](err)
	return ok
}

// second is the error of a call that returns a value and an error.
func second[V any](_ V, err error) error {
	return err
}
