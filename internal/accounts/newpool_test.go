package accounts_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// writeConfig writes the world's config file anew, holding config.
func (w *world) writeConfig(t *testing.T, config string) {
	t.Helper()
	if err := os.WriteFile(w.config, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}

// An accounts record that can't be read has the config's first pool taken
// for the first, its primary the one signed in, warned of once, however
// often it's read.
func TestARecordThatCantBeReadIsWarnedOfOnce(t *testing.T) {
	w := newWorld(t, pooled(defaultPool, acmePool))
	if err := os.MkdirAll(w.state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity.NewFile(w.state).Path(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := w.unasked(t, &user{})
	cfg := w.load(t)
	log := logstest.Capture(t)

	for range 3 {
		if first := registry.FirstPool(cfg); first != "b2n8" {
			t.Errorf("FirstPool() = %q, want the config's first, b2n8", first)
		}
		if signedIn, ok := registry.SignedIn(cfg); !ok || signedIn.ID != "work" {
			t.Errorf("SignedIn() = %+v, %t; want work, Default's primary", signedIn, ok)
		}
	}
	if got := strings.Count(log.String(), "can't read the accounts record"); got != 1 {
		t.Errorf("the log reads\n%s\nwant the record warned of once, not %d times", log, got)
	}
}

// A pool made with an account, added or moved into it, is logged as AddPool
// logs one.
func TestAPoolMadeWithAnAccountIsLogged(t *testing.T) {
	tests := []struct {
		name string
		make func(*testing.T, accounts.Registry) error
	}{
		{name: "added", make: func(t *testing.T, r accounts.Registry) error {
			return second(r.AddChecked(t.Context(), accounts.Adding{Account: config.NewAccount{Label: "Side"}, NewPool: &acmeMade},
				tokenOf(t, sideToken), accounts.Checked{Organization: organization}, nil, accounts.Answer{}))
		}},
		{name: "moved", make: func(t *testing.T, r accounts.Registry) error {
			return second(r.Edit("work", accounts.Change{NewPool: &acmeMade}))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, pooled(defaultPool))
			log := logstest.Capture(t)
			if err := tt.make(t, w.unasked(t, &user{})); err != nil {
				t.Fatal(err)
			}
			if want := []string{"level=INFO", `msg="added a pool"`, "pool=" + randomID(t), "fenced=true"}; !log.Has(want...) {
				t.Errorf("the log reads\n%s\nwant a line with %q", log, want)
			}
		})
	}
}

// acmeMade is a new pool, Acme, fenced to /Code/acme, as setup's new pool
// card gives one.
var acmeMade = accounts.NewPool{Label: "Acme", Dirs: []string{"/Code/acme"}}

// An account added in a new pool makes the pool in the one edit that adds
// it, under an id switchboard draws, the account its first, so its primary,
// marked as it's added; CheckNew checks it before the token's taken, writing
// nothing, its Primary as given.
func TestAnAccountAddedInANewPoolMakesThePoolWithIt(t *testing.T) {
	was := pooled(defaultPool)
	w := newWorld(t, was)
	registry := w.unasked(t, &user{})
	adding := accounts.Adding{Account: config.NewAccount{Label: "Side"}, NewPool: &acmeMade}

	checked, err := registry.CheckNew(adding)
	if err != nil || checked.Account.Primary || checked.Account.Pool != "" || checked.NewPool != &acmeMade {
		t.Fatalf("CheckNew() = %+v, %v; want it as given, the pool kept to make", checked, err)
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
// primary as it's added, as the config reads then, whatever it read as the
// account was checked; one going in a pool with accounts isn't, unless the
// caller marks it, nor one in a config without pools.
func TestTheFirstAccountInAPoolIsMarkedItsPrimary(t *testing.T) {
	empty := "[[pool]]\nid    = \"q7mt\"\nlabel = \"Acme\"\ndirs  = [\"/Code/acme\"]\n"
	filled := "[[account]]\nid    = \"extra\"\nlabel = \"Extra\"\npool  = \"q7mt\"\n\n"
	tests := []struct {
		name, config, pool string
		// added is the config as the account's added, where it's changed
		// since the account was checked.
		added string
		// own is the caller marking it the primary.
		own  bool
		want bool
	}{
		{name: "a pool with no accounts", config: pooled(defaultPool, empty), pool: "q7mt", want: true},
		{name: "a pool with accounts", config: pooled(defaultPool, empty), pool: "b2n8"},
		{name: "a pool with accounts, the caller marking it", config: pooled(defaultPool, empty), pool: "b2n8", own: true, want: true},
		{name: "a pool emptied since the check", config: pooled(filled+defaultPool, empty), added: pooled(defaultPool, empty), pool: "q7mt", want: true},
		{name: "a config without pools", config: twoAccounts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			registry := w.unasked(t, &user{})
			adding := accounts.Adding{Account: config.NewAccount{Label: "Side", Pool: tt.pool, Primary: tt.own}}
			checked, err := registry.CheckNew(adding)
			if err != nil || checked.Account.Primary != tt.own {
				t.Fatalf("CheckNew() = %+v, %v; want primary %t, as given", checked, err, tt.own)
			}
			if tt.added != "" {
				w.writeConfig(t, tt.added)
			}

			added, err := registry.AddChecked(t.Context(), checked, tokenOf(t, sideToken), accounts.Checked{Organization: organization}, nil, accounts.Answer{})
			if err != nil {
				t.Fatalf("AddChecked() error = %v", err)
			}
			draft, err := config.Edit(w.config, nil)
			if err != nil {
				t.Fatal(err)
			}
			if marked := draft.MarksPrimaryOf(added.ID); marked != tt.want || added.Primary != tt.want {
				t.Errorf("the config marks the account added primary %t, added as %+v; want it marked %t", marked, added, tt.want)
			}
		})
	}
}

// Whether an account added is the first in its pool is worked out under the
// record's lock, as the config reads then, never as AddChecked first read
// it: a pool given an account after that read, before the lock's taken,
// keeps its primary.
func TestTheFirstInAPoolIsWorkedOutUnderTheRecordsLock(t *testing.T) {
	acme := "[[pool]]\nid    = \"q7mt\"\nlabel = \"Acme\"\ndirs  = [\"~/Code/acme\"]\n"
	filled := "[[account]]\nid    = \"extra\"\nlabel = \"Extra\"\npool  = \"q7mt\"\n\n"
	w := newWorld(t, pooled(defaultPool, acme))
	registry := w.unasked(t, &user{})
	// The home directory's asked for each time the config's read, as Acme's
	// directory is in it: the first read is AddChecked's own, before the
	// lock.
	read := make(chan struct{}, 8)
	registry.HomeDir = func() (string, error) {
		select {
		case read <- struct{}{}:
		default:
		}
		return "/Users/test", nil
	}
	token := tokenOf(t, sideToken)
	type outcome struct {
		added accounts.Added
		err   error
	}
	done := make(chan outcome, 1)

	err := identity.NewFile(w.state).Change(func(*identity.Record, func() error) error {
		go func() {
			added, err := registry.AddChecked(t.Context(), accounts.Adding{Account: config.NewAccount{Label: "Side", Pool: "q7mt"}},
				token, accounts.Checked{Organization: organization}, nil, accounts.Answer{})
			done <- outcome{added, err}
		}()
		<-read
		w.writeConfig(t, pooled(filled+defaultPool, acme))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.err != nil {
		t.Fatalf("AddChecked() error = %v", got.err)
	}
	cfg, err := config.Load(w.config, registry.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	if side, _ := cfg.Accounts.ByID(got.added.ID); side.Primary || got.added.Primary {
		t.Errorf("the account added is %+v, added as %+v; want it unmarked, Extra Acme's primary", side, got.added)
	}
}

// An account added with a new pool has its id first, drawn where its
// organization isn't known, then the pool's, apart from it, though the draw
// would give the pool the same.
func TestANewPoolsIDIsDrawnAfterItsAccountsApartFromIt(t *testing.T) {
	w := newWorld(t, pooled(defaultPool))
	registry := w.unasked(t, &user{})
	first := make([]byte, 32)
	if _, err := io.ReadFull(drawn(), first); err != nil {
		t.Fatal(err)
	}
	rest := func() io.Reader { return strings.NewReader(strings.Repeat("pools", 100)) }
	registry.Random = io.MultiReader(bytes.NewReader(first), bytes.NewReader(first), rest())
	account := randomID(t)
	pool, err := identity.Record{}.Random(rest(), []string{account})
	if err != nil {
		t.Fatal(err)
	}

	added, err := registry.AddChecked(t.Context(), accounts.Adding{Account: config.NewAccount{Label: "Side"}, NewPool: &acmeMade},
		tokenOf(t, sideToken), accounts.Checked{}, nil, accounts.Answer{})
	if err != nil {
		t.Fatalf("AddChecked() error = %v", err)
	}
	got, _ := w.load(t).Accounts.ByID(added.ID)
	if added.ID != account || got.Pool != pool {
		t.Errorf("added %s in pool %s, want %s in pool %s", added.ID, got.Pool, account, pool)
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

// An account moved into a pool with no accounts, configured or made with
// it, is marked its primary as the first in it, though the change doesn't
// ask; one moved into a pool with accounts isn't, that pool's primary
// marked where the account would take its place.
func TestAnAccountMovedIntoAPoolIsMarkedItsPrimaryWhereItsTheFirst(t *testing.T) {
	was := pooled(defaultPool, acmePool) + "\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\npool  = \"b2n8\"\n"
	acme := "q7mt"
	tests := []struct {
		name   string
		change accounts.Change
		// then moves side after work, where it's set.
		then       bool
		wantMarked bool
	}{
		{name: "into a pool configured", change: accounts.Change{Pool: &acme}, wantMarked: true},
		{name: "into a new pool", change: accounts.Change{NewPool: &accounts.NewPool{Label: "Client", Dirs: []string{"/Code/client"}}}, wantMarked: true},
		{name: "into a pool with an account", change: accounts.Change{Pool: &acme}, then: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, was)
			registry := w.unasked(t, &user{})
			id := "work"
			if tt.then {
				if _, err := registry.Edit("work", tt.change); err != nil {
					t.Fatal(err)
				}
				id = "side"
			}

			edited, err := registry.Edit(id, tt.change)
			if err != nil {
				t.Fatalf("Edit() error = %v", err)
			}
			got, _ := w.load(t).Accounts.ByID(id)
			if got.Primary != tt.wantMarked || edited.Marked != tt.wantMarked || edited.KeptPrimary != "" {
				t.Errorf("%s is %+v, edited %+v; want it marked its pool's primary: %t, and no other marked", id, got, edited, tt.wantMarked)
			}
		})
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
