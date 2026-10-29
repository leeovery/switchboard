package setup

import (
	"cmp"
	"errors"
	"strings"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
)

// Field is a value of an account its add card or its edit card gives, as a
// refusal names it.
type Field int

const (
	// NoField is a refusal of no value alone, as of an account's removal.
	NoField Field = iota
	IDField
	LabelField
	PlanField
	PrimaryField
	ReserveField
)

// Edit is a change to an account, as setup's edit card gives it: each value
// as typed or chosen, nil leaving it as it is; and Primary, set to make it
// the primary, in place of the one that is.
type Edit struct {
	Label *string
	// Plan is one of config.PlanChoices, as PlanChosen writes it.
	Plan    *string
	Primary bool
	// Reserve is as ReserveTyped writes it: a percentage, as accounts edit
	// takes one, or none.
	Reserve *string
}

// reserveNone is the reserve as an edit types none.
const reserveNone = "none"

// ReserveTyped is the account's reserve as its edit types it: the percentage
// it was set, or none.
func (r Reviewed) ReserveTyped() string {
	if r.Reserve == 0 {
		return reserveNone
	}
	return prose.PercentSet(r.Reserve)
}

// PlanChosen is the account's plan as its edit chooses it, one of
// config.PlanChoices: its own, or config.PlanNone where it has none.
func (r Reviewed) PlanChosen() string {
	return cmp.Or(r.Plan, config.PlanNone)
}

// Refused is the error of an account to add, its edit, or its removal, that
// config refuses: of the value of Field, where it's a value's, and why, as config
// words it, for a form showing the value to say alone. It matches what
// config refused it with.
type Refused struct {
	Field Field
	Why   string
	err   error
}

// Error is config's error, as it refused.
func (r *Refused) Error() string {
	return r.err.Error()
}

// Unwrap is config's error, as it refused.
func (r *Refused) Unwrap() error {
	return r.err
}

// EditAccount changes the account with the given id as edit says, through
// accounts.Registry.Edit, as accounts edit does, and says what it changed:
// its plan read as config.ParsePlan reads it, and its reserve as
// config.ParseReserve does, none, or left empty, being none. It fails with a
// Refused where config refuses the label, the plan or the reserve, and as
// Registry.Edit does otherwise.
func (s *Setup) EditAccount(id string, edit Edit) (accounts.Edited, error) {
	change := accounts.Change{Label: edit.Label, Primary: edit.Primary}
	if edit.Plan != nil {
		plan, err := config.ParsePlan(*edit.Plan)
		if err != nil {
			return accounts.Edited{}, refusedValue(PlanField, err)
		}
		change.Plan = &plan
	}
	if edit.Reserve != nil {
		reserve, err := reserveOf(*edit.Reserve)
		if err != nil {
			return accounts.Edited{}, refusedValue(ReserveField, err)
		}
		change.Reserve = &reserve
	}
	edited, err := s.Accounts.Edit(id, change)
	if err != nil {
		return accounts.Edited{}, refusedLabel(err)
	}
	return edited, nil
}

// refusedLabel is err, as config refused an account's label: a Refused of
// Label, why as config words it alone; else err as it is.
func refusedLabel(err error) error {
	if wrong, ok := errors.AsType[*config.WrongLabel](err); ok {
		return &Refused{Field: LabelField, Why: wrong.Why, err: err}
	}
	return err
}

// refusedValue is the Refused of the value of field that config refused with
// err: why, as a config.WrongValue words it alone.
func refusedValue(field Field, err error) *Refused {
	why := err.Error()
	if wrong, ok := errors.AsType[*config.WrongValue](err); ok {
		why = wrong.Why
	}
	return &Refused{Field: field, Why: why, err: err}
}

// reserveOf is the reserve typed, as config.ParseReserve reads it: none, or
// nothing, being 0.
func reserveOf(typed string) (float64, error) {
	if typed = strings.TrimSpace(typed); typed == "" || strings.EqualFold(typed, reserveNone) {
		return 0, nil
	}
	return config.ParseReserve(typed)
}

// RemoveAccount removes the account with the given id, through
// accounts.Registry.Remove, as accounts remove does, and says what it
// removed. It fails with a Refused where it's the only account, which a
// config can't be without, and as Registry.Remove does otherwise.
func (s *Setup) RemoveAccount(id string) (accounts.Removed, error) {
	removed, err := s.Accounts.Remove(id)
	if errors.Is(err, config.ErrOnlyAccount) {
		return accounts.Removed{}, &Refused{Why: err.Error(), err: err}
	}
	return removed, err
}
