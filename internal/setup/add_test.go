package setup_test

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/setup"
)

// An account to add that config takes is given as MakeToken adds it, its
// plan read as config.ParsePlan reads it, and nothing is written: to the
// config of a run again, or, on a first run, to none at all.
func TestCheckAddingWritesNothing(t *testing.T) {
	t.Run("a run again", func(t *testing.T) {
		w := frame3(t)
		got, err := w.setup(t, &user{}).CheckAdding(setup.Adding{ID: "spare", Label: "Spare", Plan: "max5x"})
		if err != nil {
			t.Fatalf("CheckAdding error = %v", err)
		}
		if want := (config.NewAccount{ID: "spare", Label: "Spare", Plan: "max5x"}); got != want {
			t.Errorf("CheckAdding gives %+v, want %+v", got, want)
		}
		if got := w.readConfig(t); got != frameConfig {
			t.Errorf("the config reads\n%s\nwant it as it was", got)
		}
	})
	t.Run("a first run", func(t *testing.T) {
		w := newWorld(t)
		got, err := w.setup(t, &user{}).CheckAdding(setup.Adding{ID: "work", Plan: config.PlanNone})
		if err != nil {
			t.Fatalf("CheckAdding error = %v", err)
		}
		if want := (config.NewAccount{ID: "work"}); got != want {
			t.Errorf("CheckAdding gives %+v, want %+v, its plan unknown", got, want)
		}
		if _, err := os.Stat(w.config); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the config is there (%v), want none written", err)
		}
	})
}

// A value config refuses is a Refused, of its field, saying why alone, as
// config words it, the id's and the label's held to accounts add's rules,
// and nothing is written.
func TestCheckAddingRefuses(t *testing.T) {
	tests := []struct {
		name   string
		adding setup.Adding
		field  setup.Field
		why    string
	}{
		{name: "no id", adding: setup.Adding{Plan: config.PlanNone}, field: setup.IDField, why: "is required"},
		{name: "an id that isn't a plain file name", adding: setup.Adding{ID: "my side", Plan: config.PlanNone}, field: setup.IDField, why: "must start with a letter or digit and contain only letters, digits, '-' and '_'"},
		{name: "an id an account has already", adding: setup.Adding{ID: "personal", Plan: config.PlanNone}, field: setup.IDField, why: "is already configured"},
		{name: "an id an account has in another case", adding: setup.Adding{ID: "Side", Plan: config.PlanNone}, field: setup.IDField, why: `differs from "side" only in case, so they'd share a token file, as macOS ignores case in file names`},
		{name: "a label of blanks", adding: setup.Adding{ID: "spare", Label: " \u200b", Plan: config.PlanNone}, field: setup.LabelField, why: "is blank, showing nothing"},
		{name: "a label hiding the character after a sign", adding: setup.Adding{ID: "spare", Label: "\u0600Spare", Plan: config.PlanNone}, field: setup.LabelField, why: "holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character"},
		{name: "a plan config doesn't know", adding: setup.Adding{ID: "spare", Plan: "max"}, field: setup.PlanField, why: "must be pro, max5x, max20x or none"},
		{name: "an id an account has already and a wrong label, the label said", adding: setup.Adding{ID: "personal", Label: "  ", Plan: config.PlanNone}, field: setup.LabelField, why: "is blank, showing nothing"},
		{name: "a wrong id and a wrong label, the id said", adding: setup.Adding{ID: "-spare", Label: "  ", Plan: config.PlanNone}, field: setup.IDField, why: "must start with a letter or digit and contain only letters, digits, '-' and '_'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := frame3(t)
			_, err := w.setup(t, &user{}).CheckAdding(tt.adding)
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
// as a refusal of a value, though what's wrong with it is an account's id.
func TestCheckAddingOfAConfigThatCantBeRead(t *testing.T) {
	w := newWorld(t)
	w.writeConfig(t, "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"Work\"\n")
	_, err := w.setup(t, &user{}).CheckAdding(setup.Adding{ID: "spare", Plan: config.PlanNone})
	if _, refused := errors.AsType[*setup.Refused](err); refused || err == nil {
		t.Errorf("CheckAdding error = %v, want the config's own, not a Refused", err)
	}
}
