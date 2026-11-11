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
		if want := (config.NewAccount{Label: "Spare", Plan: "max"}); got != want {
			t.Errorf("CheckAdding gives %+v, want %+v", got, want)
		}
		if got := w.readConfig(t); got != frameConfig {
			t.Errorf("the config reads\n%s\nwant it as it was", got)
		}
	})
	t.Run("a first run", func(t *testing.T) {
		w := newWorld(t)
		got, err := w.setup(t).CheckAdding(setup.Adding{Label: "Work", Plan: config.PlanNone})
		if err != nil {
			t.Fatalf("CheckAdding error = %v", err)
		}
		if want := (config.NewAccount{Label: "Work"}); got != want {
			t.Errorf("CheckAdding gives %+v, want %+v, its plan unknown", got, want)
		}
		if _, err := os.Stat(w.config); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the config is there (%v), want none written", err)
		}
	})
}

// A value refused is a Refused, of its field, saying why alone, the label
// held to accounts add's rules: required, as config takes it, and no other
// configured account's, ignoring case; and nothing is written.
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
