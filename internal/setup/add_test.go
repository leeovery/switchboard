package setup_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/setup"
)

// An account to add that config takes is given as MakeToken adds it, its
// plan read as config.ParsePlan reads it, without an id, which the token's
// organization decides, and nothing is written: to the config of a run
// again, or, on a first run, to none at all.
func TestCheckAddingWritesNothing(t *testing.T) {
	t.Run("a run again", func(t *testing.T) {
		w := frame3(t)
		got, err := w.setup(t).CheckAdding(setup.Adding{Label: "Spare", Plan: "max5x"})
		if err != nil {
			t.Fatalf("CheckAdding error = %v", err)
		}
		if want := (config.NewAccount{Label: "Spare", Plan: "max"}); got.Account != want || got.NewPool != nil {
			t.Errorf("CheckAdding gives %+v, want %+v", got, want)
		}
		if got := w.readConfig(t); got != frameConfig {
			t.Errorf("the config reads\n%s\nwant it as it was", got)
		}
	})
	t.Run("a first run", func(t *testing.T) {
		w := newWorld(t)
		got, err := w.setup(t).CheckAdding(setup.Adding{Label: "Work", Plan: "pro"})
		if err != nil {
			t.Fatalf("CheckAdding error = %v", err)
		}
		if want := (config.NewAccount{Label: "Work", Plan: "pro"}); got.Account != want || got.NewPool != nil {
			t.Errorf("CheckAdding gives %+v, want %+v", got, want)
		}
		if _, err := os.Stat(w.config); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the config is there (%v), want none written", err)
		}
	})
}

// A value refused is a Refused, of its field, saying why alone, the label
// held to accounts add's rules: required, as config takes it, and no other
// configured account's, ignoring case; the plan one, and chosen, its being
// none said once the rest passes; and nothing is written.
func TestCheckAddingRefuses(t *testing.T) {
	tests := []struct {
		name   string
		adding setup.Adding
		field  setup.Field
		why    string
	}{
		{name: "no label", adding: setup.Adding{Plan: config.PlanNone}, field: setup.LabelField, why: "a label is required: every account has one"},
		{name: "a label of blanks", adding: setup.Adding{Label: " \u200b", Plan: config.PlanNone}, field: setup.LabelField, why: "is blank, showing nothing"},
		{name: "a label hiding the character after a sign", adding: setup.Adding{Label: "\u0600Spare", Plan: config.PlanNone}, field: setup.LabelField, why: "holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character"},
		{name: "another account's label, in another case", adding: setup.Adding{Label: "side", Plan: config.PlanNone}, field: setup.LabelField, why: "another account has that label already, ignoring case: give each its own"},
		{name: "a plan config doesn't know", adding: setup.Adding{Label: "Spare", Plan: "max10x"}, field: setup.PlanField, why: "must be pro, max, ultra or none"},
		{name: "a wrong plan and a wrong label, the plan said", adding: setup.Adding{Label: "  ", Plan: "max10x"}, field: setup.PlanField, why: "must be pro, max, ultra or none"},
		{name: "no plan chosen", adding: setup.Adding{Label: "Spare"}, field: setup.PlanField, why: setup.PlanUnchosen},
		{name: "none chosen", adding: setup.Adding{Label: "Spare", Plan: config.PlanNone}, field: setup.PlanField, why: setup.PlanUnchosen},
		{name: "no plan chosen and a wrong label, the label said", adding: setup.Adding{Label: "side"}, field: setup.LabelField, why: "another account has that label already, ignoring case: give each its own"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := frame3(t)
			_, err := w.setup(t).CheckAdding(tt.adding)
			refused, ok := errors.AsType[*setup.Refused](err)
			if !ok || refused.Field != tt.field || refused.Why != tt.why {
				t.Fatalf("CheckAdding error = %v (%+v), want a Refused of field %d saying %q", err, refused, tt.field, tt.why)
			}
			if got := w.readConfig(t); got != frameConfig {
				t.Errorf("the config reads\n%s\nwant it as it was", got)
			}
		})
	}
}

// A config that can't be read fails CheckAdding as config.Edit does, never
// as a refusal of a value.
func TestCheckAddingOfAConfigThatCantBeRead(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"Work\"\n")
	_, err := w.setup(t).CheckAdding(setup.Adding{Label: "Spare", Plan: config.PlanNone})
	if _, refused := errors.AsType[*setup.Refused](err); refused || err == nil {
		t.Errorf("CheckAdding error = %v, want the config's own, not a Refused", err)
	}
}

// doneInPools is doneConfig's accounts once the move to pools, in Default,
// b2n8, and the pools given after them, as [[pool]] tables are written.
func doneInPools(pools string) string {
	return "[[account]]\nid      = \"work\"\nlabel   = \"Work\"\nprimary = true\npool    = \"b2n8\"\n\n" +
		"[[account]]\nid    = \"side\"\nlabel = \"Side\"\npool  = \"b2n8\"\n\n" + pools
}

const (
	defaultPool = "[[pool]]\nid    = \"b2n8\"\nlabel = \"Default\"\n"
	fencedPools = "[[pool]]\nid    = \"b2n8\"\nlabel = \"Default\"\ndirs  = [\"/Code/work\"]\n\n[[pool]]\nid    = \"q7mt\"\nlabel = \"Acme\"\ndirs  = [\"/Code/acme\"]\n"
)

// Setup adds an account to a config with pools in the pool accounts add adds
// one to without --pool, the one the verbs act on, as it asks for none:
// CheckAdding gives it in that pool, and MakeToken adds it there once its
// token's made.
func TestSetupAddsAnAccountInThePoolTheVerbsActOn(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.writeConfig(t, doneInPools(defaultPool))
	w.api.organizations = map[string]string{madeToken: "org-personal"}
	r := w.tokenRig(t, &fakeMaker{})

	account, err := r.s.CheckAdding(setup.Adding{Label: "Personal", Plan: "max"})
	if want := (config.NewAccount{Label: "Personal", Plan: "max", Pool: "b2n8"}); err != nil || account.Account != want {
		t.Fatalf("CheckAdding = %+v, %v, want %+v", account, err, want)
	}
	got, err := r.s.MakeToken(t.Context(), setup.TokenFor{Account: account.Account, Add: true}, nil)

	id := derivedID("org-personal")
	checkTokened(t, got, err, setup.Tokened{Result: setup.TokenSaved, Checked: accounts.Checked{Organization: "org-personal"}, ID: id})
	cfg, err := config.Load(w.config, nil)
	if err != nil {
		t.Fatalf("the config doesn't load: %v", err)
	}
	if added, _ := cfg.Accounts.ByID(id); added.Pool != "b2n8" {
		t.Errorf("Personal was added in pool %q, want b2n8, Default", added.Pool)
	}
}

// Where every pool is fenced, there's no pool the verbs act on for setup to
// add an account to, so it refuses before any token is made, which would
// last a year on the account, saying to add it with accounts add --pool:
// CheckAdding fails as the step does, not as a value refused, and MakeToken
// makes nothing.
func TestSetupRefusesToAddAnAccountWhereEveryPoolIsFenced(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.writeConfig(t, doneInPools(fencedPools))
	r := w.tokenRig(t, &fakeMaker{})
	before := w.snapshot(t)

	_, err := r.s.CheckAdding(setup.Adding{Label: "Personal", Plan: "max"})
	if _, refused := errors.AsType[*setup.Refused](err); refused || !strings.Contains(fmt.Sprint(err), "switchboard accounts add <label> --pool <id>") {
		t.Errorf("CheckAdding error = %v, want the step's, saying to add it with accounts add --pool", err)
	}
	if _, needed := errors.AsType[*accounts.PoolNeededError](err); !needed {
		t.Errorf("CheckAdding error = %v, want an accounts.PoolNeededError", err)
	}

	_, err = r.s.MakeToken(t.Context(), adding, nil)
	if _, needed := errors.AsType[*accounts.PoolNeededError](err); !needed || r.maker.asked > 0 {
		t.Errorf("MakeToken() error = %v, made %d; want an accounts.PoolNeededError, with nothing made", err, r.maker.asked)
	}
	w.checkUnchanged(t, before)
}
