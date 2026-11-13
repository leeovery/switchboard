package setup_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setuptoken"
)

// acmeMade is a new pool, Acme, fenced to /Code/acme, as the new pool card
// gives one.
var acmeMade = accounts.NewPool{Label: "Acme", Dirs: []string{"/Code/acme"}}

// An account added in a new pool is checked with it before any token's made,
// writing nothing; the pool's written with it, in the one edit that adds it,
// once its token's saved, the account its first, so marked its primary.
func TestSetupAddsAnAccountInTheNewPoolItMakes(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.writeConfig(t, doneInPools(defaultPool))
	w.api.organizations = map[string]string{madeToken: "org-personal"}
	r := w.tokenRig(t, &fakeMaker{})
	was := w.readConfig(t)

	checked, err := r.s.CheckAdding(setup.Adding{Label: "Acme 1", Plan: "ultra", NewPool: &acmeMade})
	if err != nil || checked.Account.Primary || checked.NewPool == nil || checked.Account.Pool != "" {
		t.Fatalf("CheckAdding = %+v, %v; want Acme 1 as given, the pool kept to make", checked, err)
	}
	if got := w.readConfig(t); got != was {
		t.Fatalf("checked, the config reads\n%s\nwant it as it was", got)
	}

	if _, err := r.s.MakeToken(t.Context(), setup.TokenFor{Account: checked.Account, NewPool: checked.NewPool, Add: true}, nil); err != nil {
		t.Fatalf("MakeToken() error = %v", err)
	}
	cfg, err := config.Load(w.config, nil)
	if err != nil {
		t.Fatalf("the config doesn't load: %v", err)
	}
	pool, ok := cfg.Pools.ByID(randomID(t))
	if !ok || pool.Label != "Acme" || !slices.Equal(pool.Dirs, acmeMade.Dirs) {
		t.Fatalf("the config's pools are %+v, want Acme, fenced to /Code/acme", cfg.Pools)
	}
	if added, _ := cfg.Accounts.ByID(derivedID("org-personal")); added.Pool != pool.ID || !added.Primary {
		t.Errorf("Acme 1 is %+v, want it in Acme, marked its primary", added)
	}
}

// A token not saved leaves no new pool: an empty fenced pool would stop every
// session started in its directories.
func TestATokenNotSavedLeavesNoNewPool(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.writeConfig(t, doneInPools(defaultPool))
	r := w.tokenRig(t, &fakeMaker{token: refusedToken})
	before := w.snapshot(t)

	got, err := r.s.MakeToken(t.Context(), setup.TokenFor{Account: config.NewAccount{Label: "Acme 1", Primary: true}, NewPool: &acmeMade, Add: true}, nil)
	if err != nil || got.Result != setup.TokenRefused {
		t.Fatalf("MakeToken() = %+v, %v; want the token refused", got, err)
	}
	w.checkUnchanged(t, before)
}

// A new pool config's rules refuse is a Refused of the new pool card's field
// it's of, why alone: its label, none or another pool's; its directories,
// the one refused, or the first, where it would be a second pool unfenced.
// Nothing is written.
func TestCheckPoolRefusesAsTheConfigsRulesDo(t *testing.T) {
	tests := []struct {
		name  string
		pool  accounts.NewPool
		field setup.Field
		dir   int
		why   string
	}{
		{name: "no label", pool: accounts.NewPool{Dirs: []string{"/Code/acme"}}, field: setup.PoolLabelField, why: "a pool needs a label"},
		{name: "another pool's label", pool: accounts.NewPool{Label: "acme", Dirs: []string{"/Code/client"}}, field: setup.PoolLabelField, why: `"acme" is Acme (q7mt)'s label already, ignoring case: give the pool its own`},
		{name: "a label config refuses", pool: accounts.NewPool{Label: "Client\nLtd", Dirs: []string{"/Code/client"}}, field: setup.PoolLabelField, why: "holds a control character, such as a newline or a tab, which can't show"},
		{
			name: "a second pool unfenced", pool: accounts.NewPool{Label: "Client"}, field: setup.DirsField,
			why: "Default (b2n8) is unfenced already, and only one pool can be, as a session started outside every fence would have two pools to be in: give this one its directories, or fence Default first with switchboard pool edit b2n8 --dirs",
		},
		{
			name: "a directory inside another pool's", pool: accounts.NewPool{Label: "Client", Dirs: []string{"/Code/client", "/Code/acme/client"}}, field: setup.DirsField, dir: 1,
			why: "is inside Acme's /Code/acme: no directory may be the same as, inside or around another pool's",
		},
		{
			name: "a directory neither absolute nor from ~", pool: accounts.NewPool{Label: "Client", Dirs: []string{"Code/client"}}, field: setup.DirsField,
			why: "must be an absolute path, or one starting ~/, the home directory's, such as ~/Code/acme",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.writeConfig(t, doneInPools(defaultPool+"\n"+acmePool))
			was := w.readConfig(t)

			err := w.setup(t).CheckPool(tt.pool)
			refused, ok := errors.AsType[*setup.Refused](err)
			if !ok || refused.Field != tt.field || refused.Dir != tt.dir || refused.Why != tt.why {
				t.Fatalf("CheckPool() error = %v (%+v), want a Refused of field %d, dir %d, saying %q", err, refused, tt.field, tt.dir, tt.why)
			}
			if got := w.readConfig(t); got != was {
				t.Errorf("the config reads\n%s\nwant it as it was", got)
			}
		})
	}
}

// acmePool is a second pool, Acme, fenced to /Code/acme, with no account.
const acmePool = "[[pool]]\nid    = \"q7mt\"\nlabel = \"Acme\"\ndirs  = [\"/Code/acme\"]\n"

// The pool chosen, or new, on the add card or the edit card, refused is a
// Refused of Pool: one config no longer has, or a new pool its rules refuse,
// its label among them, though config refuses it as it would an account's.
func TestThePoolChosenRefusedIsARefusedOfPool(t *testing.T) {
	tests := []struct {
		name    string
		pool    string
		newPool *accounts.NewPool
		why     string
	}{
		{name: "a pool not configured", pool: "z9z9", why: `pool "z9z9" is not configured`},
		{name: "a new pool's label another's", newPool: &accounts.NewPool{Label: "Default", Dirs: []string{"/Code/spare"}}, why: `"Default" is Default (b2n8)'s label already, ignoring case: give the pool its own`},
		{name: "a new pool's label config refuses", newPool: &accounts.NewPool{Label: "Acme\tLtd", Dirs: []string{"/Code/spare"}}, why: "holds a control character, such as a newline or a tab, which can't show"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.writeConfig(t, doneInPools(defaultPool))
			s := w.setup(t)
			var pool *string
			if tt.pool != "" {
				pool = &tt.pool
			}

			for name, err := range map[string]error{
				"CheckAdding": second(s.CheckAdding(setup.Adding{Label: "Spare", Plan: "pro", Pool: tt.pool, NewPool: tt.newPool})),
				"EditAccount": second(s.EditAccount("side", setup.Edit{Pool: pool, NewPool: tt.newPool})),
			} {
				if refused, ok := errors.AsType[*setup.Refused](err); !ok || refused.Field != setup.PoolField || refused.Why != tt.why {
					t.Errorf("%s() error = %v (%+v), want a Refused of Pool saying %q", name, err, refused, tt.why)
				}
			}
		})
	}
}

// An account edited that's no longer configured fails the edit, as no field
// of the card can put it right.
func TestEditAccountOfAnAccountNoLongerConfiguredIsNoRefusal(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, doneInPools(defaultPool))

	_, err := w.setup(t).EditAccount("gone", setup.Edit{Pool: new("b2n8")})
	if _, refused := errors.AsType[*setup.Refused](err); refused || !errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("EditAccount() error = %v, want it not configured, no Refused", err)
	}
}

// second is the error of a call that returns a value and an error.
func second[V any](_ V, err error) error {
	return err
}

// Editing an account moves it into the pool chosen, or a new pool made for
// it in the same edit, through accounts.Registry.Edit; a new pool refused
// is a Refused of Pool, nothing written.
func TestEditAccountMovesItIntoAPool(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, doneInPools(defaultPool+"\n"+acmePool))
	s := w.setup(t)

	if _, err := s.EditAccount("side", setup.Edit{NewPool: &accounts.NewPool{Label: "Acme", Dirs: []string{"/Code/side"}}}); !isRefusedOf(err, setup.PoolField) {
		t.Fatalf("EditAccount() into a new pool labelled as Acme error = %v, want a Refused of Pool", err)
	}
	if _, err := s.EditAccount("side", setup.Edit{Pool: new("q7mt")}); err != nil {
		t.Fatalf("EditAccount() into Acme error = %v", err)
	}
	if _, err := s.EditAccount("work", setup.Edit{NewPool: &accounts.NewPool{Label: "Client", Dirs: []string{"/Code/client"}}}); err != nil {
		t.Fatalf("EditAccount() into a new pool error = %v", err)
	}
	cfg, err := config.Load(w.config, nil)
	if err != nil {
		t.Fatalf("the config doesn't load: %v", err)
	}
	client, ok := cfg.Pools.Labelled("", "Client")
	if !ok {
		t.Fatalf("the config's pools are %+v, want Client made", cfg.Pools)
	}
	for id, want := range map[string]string{"side": "q7mt", "work": client.ID} {
		if a, _ := cfg.Accounts.ByID(id); a.Pool != want || !a.Primary {
			t.Errorf("%s is %+v, want it in pool %s, its primary as its first", id, a, want)
		}
	}
}

// A pool's marked primary moved leaves its first account the pool's primary;
// an account moved, not made the primary, into a pool with one keeps that
// pool's primary as it is.
func TestEditAccountMovingAPoolsPrimaryLeavesThePoolItLeftOne(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, doneInPools(defaultPool+"\n"+acmePool)+"\n[[account]]\nid    = \"spare\"\nlabel = \"Spare\"\npool  = \"b2n8\"\n")
	s := w.setup(t)

	for _, id := range []string{"work", "side"} {
		if _, err := s.EditAccount(id, setup.Edit{Pool: new("q7mt")}); err != nil {
			t.Fatalf("EditAccount(%s) into Acme error = %v", id, err)
		}
	}
	cfg, err := config.Load(w.config, nil)
	if err != nil {
		t.Fatalf("the config doesn't load: %v", err)
	}
	for pool, want := range map[string]string{"b2n8": "spare", "q7mt": "work"} {
		if primary, _ := cfg.Accounts.PrimaryOf(pool); primary.ID != want {
			t.Errorf("pool %s's primary is %s, want %s", pool, primary.ID, want)
		}
	}
	if side, _ := cfg.Accounts.ByID("side"); side.Primary {
		t.Errorf("side is %+v, want it in Acme unmarked, work Acme's primary still", side)
	}
}

// isRefusedOf reports whether err is a Refused of field.
func isRefusedOf(err error, field setup.Field) bool {
	refused, ok := errors.AsType[*setup.Refused](err)
	return ok && refused.Field == field
}

// The review has the config's pools, in its order, and the accounts by pool,
// each pool's in the config's order, each pool's primary its own: the one
// marked, else one alone; a pool of two or more, none marked, has none, and
// a note says so, led by its label, the review waiting for one, the Claude
// apps signing in to the first pool's alone, the one the accounts record
// notes, else the config's first.
func TestTheReviewGroupsTheAccountsByPool(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, "[[account]]\nid = \"work\"\nlabel = \"Work\"\nprimary = true\npool = \"b2n8\"\n\n"+
		"[[account]]\nid = \"acme1\"\nlabel = \"Acme 1\"\npool = \"q7mt\"\n\n"+
		"[[account]]\nid = \"side\"\nlabel = \"Side\"\npool = \"b2n8\"\n\n"+
		"[[account]]\nid = \"acme2\"\nlabel = \"Acme 2\"\npool = \"q7mt\"\n\n"+
		"[[account]]\nid = \"solo\"\nlabel = \"Solo\"\npool = \"r2d4\"\n\n"+
		defaultPool+"\n"+acmePool+"\n[[pool]]\nid = \"r2d4\"\nlabel = \"Solo\"\ndirs = [\"/Code/solo\"]\n")

	review, err := w.setup(t).Review()
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	var got []string
	for _, a := range review.Accounts {
		if a.Primary {
			a.ID += " ◆"
		}
		got = append(got, a.Pool+" "+a.ID)
	}
	if want := []string{"b2n8 work ◆", "b2n8 side", "q7mt acme1", "q7mt acme2", "r2d4 solo ◆"}; !slices.Equal(got, want) {
		t.Errorf("the review's accounts are %q, want %q", got, want)
	}
	if !slices.Equal(review.Pools.IDs(), []string{"b2n8", "q7mt", "r2d4"}) || !review.Unmarked {
		t.Errorf("the review's pools are %q, unmarked %t; want the config's three, Acme unmarked", review.Pools.IDs(), review.Unmarked)
	}
	notes := review.Notes()
	if len(notes) == 0 || notes[0].Label != "Acme" || notes[0].Says != "has no primary marked, so its sessions start on its first account." || review.First != "b2n8" {
		t.Errorf("the first note is %+v, the first pool %q; want Acme's, that it has no primary, Default the first", notes[0], review.First)
	}

	w.noteFirst(t, "q7mt")
	if review, err = w.setup(t).Review(); err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	notes = review.Notes()
	if len(notes) == 0 || notes[0].Says != "has no primary marked, so its sessions start on its first account, and the Claude apps sign in to it." || review.First != "q7mt" {
		t.Errorf("Acme noted the first, the first note is %+v, the first pool %q; want Acme's, the Claude apps signing in to it", notes[0], review.First)
	}
}

// Which sessions use a pool, as the add card's hint says it: a fenced pool's
// started in its directories alone; the unfenced pool, every session where
// it's the only one, else those started everywhere else. Its heading over
// its accounts says its directories, or everywhere else.
func TestWhichSessionsUseAPool(t *testing.T) {
	tests := []struct {
		name         string
		pool         config.Pool
		pools        int
		used, places string
	}{
		{name: "fenced", pool: config.Pool{WrittenDirs: []string{"~/Code/acme"}}, pools: 2, used: "only sessions started in ~/Code/acme use it", places: "~/Code/acme"},
		{name: "fenced twice", pool: config.Pool{WrittenDirs: []string{"~/Code/acme", "/Volumes/Acme"}}, pools: 2, used: "only sessions started in ~/Code/acme or /Volumes/Acme use it", places: "~/Code/acme, /Volumes/Acme"},
		{name: "unfenced, the only pool", pools: 1, used: "every session uses it", places: "everywhere else"},
		{name: "unfenced, of two", pools: 2, used: "sessions started everywhere else use it", places: "everywhere else"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			choice := setup.PoolChoice{Dirs: tt.pool.WrittenDirs}
			if got := choice.UsedBy(tt.pools); got != tt.used {
				t.Errorf("UsedBy(%d) = %q, want %q", tt.pools, got, tt.used)
			}
			if got := setup.Places(tt.pool); got != tt.places {
				t.Errorf("Places() = %q, want %q", got, tt.places)
			}
		})
	}
}

// A Refused without config's error, as a fake's, says why alone.
func TestARefusedWithoutAnErrorSaysWhy(t *testing.T) {
	if got := (&setup.Refused{Field: setup.PlanField, Why: setup.PlanUnchosen}).Error(); got != setup.PlanUnchosen {
		t.Errorf("Error() = %q, want %q", got, setup.PlanUnchosen)
	}
}

// The add card offers a subscription's plans alone, never none; the edit
// card offers none only to an account the config gives no plan.
func TestThePlansACardOffers(t *testing.T) {
	plans := []string{"pro", "max", "ultra"}
	if got := setup.AddPlans(); !slices.Equal(got, plans) {
		t.Errorf("AddPlans() = %q, want %q", got, plans)
	}
	if got := (setup.Reviewed{Plan: "max"}).PlanChoices(); !slices.Equal(got, plans) {
		t.Errorf("an account on Max is offered %q, want %q", got, plans)
	}
	if got, want := (setup.Reviewed{}).PlanChoices(), append(plans, config.PlanNone); !slices.Equal(got, want) {
		t.Errorf("an account with no plan is offered %q, want %q", got, want)
	}
}

// The browser is signed in to the first pool's primary, as the Claude apps
// are, the first pool the one the accounts record notes, else the config's
// first: another pool's marked primary is never taken for it, though it's
// the first account marked. A token for any other account, or for every one
// where the first pool has none, opens in a private window.
func TestTheOpenerRuleTakesTheFirstPoolsPrimaryForTheBrowsers(t *testing.T) {
	const (
		// acmeFirst has an unmarked Default account and Acme's marked one,
		// Acme the config's first pool.
		acmeFirst = "[[account]]\nid      = \"side\"\nlabel   = \"Side\"\nprimary = true\npool    = \"q7mt\"\n\n" +
			"[[account]]\nid    = \"work\"\nlabel = \"Work\"\npool  = \"b2n8\"\n\n" + acmePool + "\n" + defaultPool
		// noFirstAccount has every account in Acme, Default the first.
		noFirstAccount = "[[account]]\nid      = \"work\"\nlabel   = \"Work\"\nprimary = true\npool    = \"q7mt\"\n\n" +
			"[[account]]\nid    = \"side\"\nlabel = \"Side\"\npool  = \"q7mt\"\n\n" + defaultPool + "\n" + acmePool
	)
	tests := []struct {
		name   string
		config string
		// noted is the first pool the accounts record notes.
		noted  string
		f      setup.TokenFor
		asItIs bool
		want   setup.Making
	}{
		{
			name:   "the first pool's unmarked primary: in the browser as it is",
			config: acmeFirst, noted: "b2n8",
			f:      replacing("work"),
			asItIs: true,
			want:   setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedInBrowser, For: "Work"},
		},
		{
			name:   "another pool's marked primary: in a private window, the first pool's signed in",
			config: acmeFirst, noted: "b2n8",
			f:    replacing("side"),
			want: setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedPrivately, Browser: "Firefox", For: "Side", SignedIn: "Work", Private: true},
		},
		{
			name:   "none noted: the config's first pool's primary in the browser as it is",
			config: acmeFirst,
			f:      replacing("side"),
			asItIs: true,
			want:   setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedInBrowser, For: "Side"},
		},
		{
			name:   "a first pool with no account: in a private window, signed in to none",
			config: noFirstAccount, noted: "b2n8",
			f:    replacing("work"),
			want: setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedPrivately, Browser: "Firefox", For: "Work", Private: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.writeConfig(t, tt.config)
			if tt.noted != "" {
				w.noteFirst(t, tt.noted)
			}
			r := w.tokenRig(t, &fakeMaker{})
			var h heard

			if got, err := r.s.MakeToken(t.Context(), tt.f, h.report); err != nil || got.Result != setup.TokenSaved {
				t.Fatalf("MakeToken() = %+v, %v; want the token saved", got, err)
			}

			opened, unasked := r.private, r.browser
			if tt.asItIs {
				opened, unasked = r.browser, r.private
			}
			if len(opened.asked) != 1 || len(unasked.asked) > 0 {
				t.Errorf("the browser as it is opened %q, the private window %q; want the address opened as it is: %t", r.browser.asked, r.private.asked, tt.asItIs)
			}
			at := h.address(t)
			at.Address = setuptoken.Address{}
			if at != tt.want {
				t.Errorf("the address ready was told as %+v, want %+v", at, tt.want)
			}
		})
	}
}
