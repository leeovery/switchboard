package setup

import (
	"errors"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
)

// Adding is an account to add, as setup's add card gives it: its label as
// typed, and its plan, one of config.PlanChoices. Its id is the one the
// organization of the token made for it decides.
type Adding struct {
	Label string
	Plan  string
}

// CheckAdding checks the account adding gives as accounts add checks one,
// against the config as it stands, writing nothing, and gives it as
// MakeToken adds it: its plan read as config.ParsePlan reads it. It fails
// with a Refused where the plan isn't one, or the label isn't one the
// account can have, as accounts.Registry.CheckNew says, and as config.Edit
// does otherwise.
func (s *Setup) CheckAdding(adding Adding) (config.NewAccount, error) {
	plan, err := config.ParsePlan(adding.Plan)
	if err != nil {
		return config.NewAccount{}, refusedValue(PlanField, err)
	}
	account := config.NewAccount{Label: adding.Label, Plan: plan}
	if err := s.Accounts.CheckNew(account); err != nil {
		return config.NewAccount{}, refusedLabel(err)
	}
	return account, nil
}

// refusedLabel is err, as an account's label is refused: by config, as one
// no account can have, or as none, or another configured account's, as
// accounts has them; a Refused of Label, why as config words it alone, or as
// accounts words it; else err as it is.
func refusedLabel(err error) error {
	if wrong, ok := errors.AsType[*config.WrongLabel](err); ok {
		return &Refused{Field: LabelField, Why: wrong.Why, err: err}
	}
	if errors.Is(err, accounts.ErrNoLabel) || errors.Is(err, accounts.ErrLabelTaken) {
		return &Refused{Field: LabelField, Why: err.Error(), err: err}
	}
	return err
}
