package setup_test

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/setup"
)

// Each value an edit card saves is written as accounts edit writes it, the
// config the same, byte for byte, as Registry.Edit leaves it given the
// change the command would read from its flags.
func TestEditAccountWritesAsAccountsEditDoes(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		edit   setup.Edit
		change accounts.Change
	}{
		{name: "a label", id: "personal", edit: setup.Edit{Label: new("Home")}, change: accounts.Change{Label: new("Home")}},
		{name: "a plan", id: "personal", edit: setup.Edit{Plan: new("max5x")}, change: accounts.Change{Plan: new("max5x")}},
		{name: "a plan set to none", id: "work", edit: setup.Edit{Plan: new("none")}, change: accounts.Change{Plan: new("")}},
		{name: "the primary", id: "side", edit: setup.Edit{Primary: true}, change: accounts.Change{Primary: true}},
		{name: "a reserve, as a percentage", id: "personal", edit: setup.Edit{Reserve: new("2.5%")}, change: accounts.Change{Reserve: new(0.025)}},
		{name: "a reserve of none", id: "work", edit: setup.Edit{Reserve: new("none")}, change: accounts.Change{Reserve: new(0.0)}},
		{name: "a reserve cleared, as none", id: "work", edit: setup.Edit{Reserve: new(" ")}, change: accounts.Change{Reserve: new(0.0)}},
		{name: "every value at once", id: "personal", edit: setup.Edit{Label: new("Home"), Plan: new("pro"), Primary: true, Reserve: new("10%")}, change: accounts.Change{Label: new("Home"), Plan: new("pro"), Primary: true, Reserve: new(0.1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byCommand := frame3(t)
			if _, err := byCommand.setup(t, &user{}).Accounts.Edit(tt.id, tt.change); err != nil {
				t.Fatalf("Registry.Edit error = %v", err)
			}
			w := frame3(t)
			edited, err := w.setup(t, &user{}).EditAccount(tt.id, tt.edit)
			if err != nil {
				t.Fatalf("EditAccount error = %v", err)
			}
			if got, want := w.readConfig(t), byCommand.readConfig(t); got != want {
				t.Errorf("the config reads\n%s\nwant it as accounts edit writes it\n%s", got, want)
			}
			if !edited.Changed() || edited.Now.ID != tt.id {
				t.Errorf("EditAccount says it edited %+v, want %s changed", edited.Now, tt.id)
			}
		})
	}
}

// A value config refuses is a Refused, of its field, saying why alone, as
// config words it, and nothing is written.
func TestEditAccountRefuses(t *testing.T) {
	tests := []struct {
		name  string
		edit  setup.Edit
		field setup.Field
		why   string
	}{
		{name: "a label of blanks", edit: setup.Edit{Label: new("  \u200b")}, field: setup.LabelField, why: "is blank, showing nothing"},
		{name: "a label hiding the character after a sign", edit: setup.Edit{Label: new("\u0600Home")}, field: setup.LabelField, why: "holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character"},
		{name: "a plan config doesn't know", edit: setup.Edit{Plan: new("max")}, field: setup.PlanField, why: "must be pro, max5x, max20x or none"},
		{name: "a reserve without its %", edit: setup.Edit{Reserve: new("5")}, field: setup.ReserveField, why: "must be written as a percentage, such as 5%, as a number alone could be a share or a percentage"},
		{name: "a reserve that isn't a percentage", edit: setup.Edit{Reserve: new("lots")}, field: setup.ReserveField, why: "must be a percentage, to two decimal places at most, such as 5% or 2.5%, or 0 for none"},
		{name: "a reserve of the whole window", edit: setup.Edit{Reserve: new("100%")}, field: setup.ReserveField, why: "must be at least 0% and less than 100%, the share of every window the router leaves unused"},
		{name: "a good label and a wrong reserve, the reserve said", edit: setup.Edit{Label: new("Home"), Reserve: new("5")}, field: setup.ReserveField, why: "must be written as a percentage, such as 5%, as a number alone could be a share or a percentage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := frame3(t)
			_, err := w.setup(t, &user{}).EditAccount("personal", tt.edit)
			refused, ok := errors.AsType[*setup.Refused](err)
			if !ok || refused.Field != tt.field || refused.Why != tt.why {
				t.Fatalf("EditAccount error = %v (%+v), want a Refused of field %d saying %q", err, refused, tt.field, tt.why)
			}
			if got := w.readConfig(t); got != frameConfig {
				t.Errorf("the config reads\n%s\nwant it as it was", got)
			}
		})
	}
}

// Made the primary where two accounts are configured and none is marked,
// the first, the primary only for want of a mark, is marked so, as p and the
// edit card's Primary make it, through the registry: so the review has it
// the primary, and no longer unmarked.
func TestEditAccountMarksTheFirstThePrimaryWhereNoneIsMarked(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit setup.Edit
	}{
		{name: "by p", edit: setup.Edit{Primary: true}},
		{name: "by the edit card, with a plan", edit: setup.Edit{Primary: true, Plan: new("pro")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			const unmarked = "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n"
			w.writeConfig(t, unmarked)
			s := w.setup(t, &user{})
			if review, err := s.Review(); err != nil || !review.Unmarked {
				t.Fatalf("the review before is %+v, %v; want it unmarked", review, err)
			}

			if _, err := s.EditAccount("work", tt.edit); err != nil {
				t.Fatalf("EditAccount error = %v", err)
			}
			review, err := s.Review()
			if err != nil {
				t.Fatal(err)
			}
			if review.Unmarked || !review.Accounts[0].Primary || review.Accounts[1].Primary {
				t.Errorf("the review after is %+v; want work the primary, marked", review)
			}
		})
	}
}

// An edit of an account no account has fails as Registry.Edit does, never
// as a refusal of a value.
func TestEditAccountOfAnAccountGone(t *testing.T) {
	_, err := frame3(t).setup(t, &user{}).EditAccount("gone", setup.Edit{Plan: new("pro")})
	if _, refused := errors.AsType[*setup.Refused](err); refused || !errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("EditAccount error = %v, want it matching ErrNotConfigured, not a Refused", err)
	}
}

// Removing an account is as accounts remove does: its table and its token
// file gone; the primary removed, none marked of the two left.
func TestRemoveAccount(t *testing.T) {
	for _, tt := range []struct {
		id string
		// primary is the primary once it's removed: "" for none marked.
		primary string
	}{{id: "side", primary: "work"}, {id: "work"}} {
		id := tt.id
		t.Run(id, func(t *testing.T) {
			byCommand := frame3(t)
			want, err := byCommand.setup(t, &user{}).Accounts.Remove(id)
			if err != nil {
				t.Fatalf("Registry.Remove error = %v", err)
			}
			w := frame3(t)
			s := w.setup(t, &user{})
			removed, err := s.RemoveAccount(id)
			if err != nil {
				t.Fatalf("RemoveAccount error = %v", err)
			}
			if removed != want {
				t.Errorf("RemoveAccount says %+v, want %+v", removed, want)
			}
			if got, want := w.readConfig(t), byCommand.readConfig(t); got != want {
				t.Errorf("the config reads\n%s\nwant it as accounts remove writes it\n%s", got, want)
			}
			w.checkNoToken(t, id)
			review, err := s.Review()
			if err != nil {
				t.Fatal(err)
			}
			var primary string
			for _, a := range review.Accounts {
				if a.ID == id {
					t.Errorf("the review still has %s", id)
				}
				if a.Primary {
					primary = a.ID
				}
			}
			if primary != tt.primary || review.Unmarked != (tt.primary == "") {
				t.Errorf("the review's primary is %q, unmarked %v, want %q", primary, review.Unmarked, tt.primary)
			}
		})
	}
}

// A removal that takes the account out of the config but can't delete its
// token file fails as Registry.Remove does, saying so, never as a refusal;
// the review read again is without it.
func TestRemoveAccountThatLeavesItsTokenFile(t *testing.T) {
	w := frame3(t)
	w.removeToken(t, "side")
	mkdir(t, filepath.Join(w.tokens().Path("side"), "kept"))
	s := w.setup(t, &user{})

	_, err := s.RemoveAccount("side")
	if _, refused := errors.AsType[*setup.Refused](err); refused {
		t.Fatalf("RemoveAccount error = %v, want it not a Refused", err)
	}
	if _, left := errors.AsType[*accounts.TokenFileLeft](err); !left {
		t.Fatalf("RemoveAccount error = %v, want an accounts.TokenFileLeft", err)
	}
	review, err := s.Review()
	if err != nil || len(review.Accounts) != 2 {
		t.Errorf("the review after is %+v, %v; want work and personal alone", review, err)
	}
}

// Removing the only account is a Refused of no field, saying why as config
// does, and removes nothing.
func TestRemoveAccountRefusesTheOnlyOne(t *testing.T) {
	w := newWorld(t)
	const only = "[[account]]\nid = \"work\"\n"
	w.writeConfig(t, only)
	w.writeToken(t, "work", workToken)
	_, err := w.setup(t, &user{}).RemoveAccount("work")
	refused, ok := errors.AsType[*setup.Refused](err)
	if !ok || refused.Field != setup.NoField || refused.Why != `account "work" is the only one, and a config needs one at least` {
		t.Fatalf("RemoveAccount error = %v, want a Refused of no field saying the config needs one", err)
	}
	if !errors.Is(err, config.ErrOnlyAccount) {
		t.Errorf("the error %v doesn't match ErrOnlyAccount", err)
	}
	if got := w.readConfig(t); got != only {
		t.Errorf("the config reads %q, want it as it was", got)
	}
	w.checkToken(t, "work", workToken)
}

// An account's reserve as its edit card types it: the percentage it was
// set, or none.
func TestReserveTyped(t *testing.T) {
	for _, tt := range []struct {
		reserve float64
		want    string
	}{{0, "none"}, {0.05, "5%"}, {0.025, "2.5%"}} {
		if got := (setup.Reviewed{Reserve: tt.reserve}).ReserveTyped(); got != tt.want {
			t.Errorf("ReserveTyped of %v = %q, want %q", tt.reserve, got, tt.want)
		}
	}
}

// An account's plan as its edit card chooses it: its own, or none, among
// config.PlanChoices.
func TestPlanChosen(t *testing.T) {
	for _, tt := range []struct{ plan, want string }{{"", "none"}, {"max5x", "max5x"}} {
		got := (setup.Reviewed{Plan: tt.plan}).PlanChosen()
		if got != tt.want || !slices.Contains(config.PlanChoices(), got) {
			t.Errorf("PlanChosen of %q = %q, want %q, one of config.PlanChoices", tt.plan, got, tt.want)
		}
	}
}
