package setup

import (
	"errors"

	"github.com/leeovery/switchboard/internal/config"
)

// Adding is an account to add, as setup's add card gives it: its id and its
// label as typed, and its plan, one of config.PlanChoices.
type Adding struct {
	ID, Label string
	Plan      string
}

// CheckAdding checks the account adding gives as accounts add checks one,
// against the config as it stands, writing nothing, and gives it as
// MakeToken adds it: its plan read as config.ParsePlan reads it. It fails
// with a Refused where config refuses the id, as one no account can have or
// one an account has already, the label or the plan, and as config.Edit
// does otherwise.
func (s *Setup) CheckAdding(adding Adding) (config.NewAccount, error) {
	plan, err := config.ParsePlan(adding.Plan)
	if err != nil {
		return config.NewAccount{}, refusedValue(PlanField, err)
	}
	draft, err := config.Edit(s.Accounts.ConfigPath)
	if err != nil {
		return config.NewAccount{}, err
	}
	account := config.NewAccount{ID: adding.ID, Label: adding.Label, Plan: plan}
	if err := draft.AddAccount(account); err != nil {
		return config.NewAccount{}, refusedAccount(err)
	}
	return account, nil
}

// refusedAccount is err, as config refused an account's id or its label: a
// Refused of that field, why as config words it alone; else err as it is.
func refusedAccount(err error) error {
	if wrong, ok := errors.AsType[*config.WrongID](err); ok {
		return &Refused{Field: IDField, Why: wrong.Why, err: err}
	}
	return refusedLabel(err)
}
