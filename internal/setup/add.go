package setup

import (
	"errors"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
)

// Adding is an account to add, as setup's add card gives it: its label as
// typed; its plan, one of AddPlans, "" where none's chosen; and the id of
// the pool it goes in, "" for the one accounts add adds it to without
// --pool, or NewPool, a pool made for it, in its place. Its id is the one
// the organization of the token made for it decides.
type Adding struct {
	Label   string
	Plan    string
	Pool    string
	NewPool *accounts.NewPool
}

// CheckAdding checks the account adding gives as accounts add checks one,
// against the config as it stands, writing nothing, and gives it as
// MakeToken adds it: its plan read as config.ParsePlan reads it, in its pool
// as accounts.Registry.CheckNew has it, a new pool checked as CheckNewPool
// checks one, and made with it. It fails with a Refused where the plan isn't
// one, the label isn't one the account can have, as CheckNew says, or the
// pool chosen is one config no longer has, or the new pool is refused, said
// of Pool, as refusedAccount has it; with CheckNew's
// accounts.PoolNeededError, before any token is made, where every pool is
// fenced and none is chosen; with a Refused of Plan, saying PlanUnchosen,
// where no plan's chosen, once the rest passes; and as config.Edit does
// otherwise.
func (s *Setup) CheckAdding(adding Adding) (accounts.Adding, error) {
	plan, err := planChosen(adding.Plan)
	if err != nil {
		return accounts.Adding{}, refusedValue(PlanField, err)
	}
	account := config.NewAccount{Label: adding.Label, Plan: plan, Pool: adding.Pool}
	checked, err := s.Accounts.CheckNew(accounts.Adding{Account: account, NewPool: adding.NewPool})
	if err != nil {
		return accounts.Adding{}, refusedAccount(err)
	}
	if adding.Plan == "" {
		return accounts.Adding{}, &Refused{Field: PlanField, Why: PlanUnchosen, err: errors.New(PlanUnchosen)}
	}
	return checked, nil
}

// planChosen is the plan chosen, as config.ParsePlan reads it: "" where
// none is, which CheckAdding refuses once the account's other values pass,
// so the card's refusals come in the order its fields do.
func planChosen(chosen string) (string, error) {
	if chosen == "" {
		return "", nil
	}
	return config.ParsePlan(chosen)
}

// CheckPool checks a pool to make with an account, as the new pool card gives
// it, against the config as it stands, writing nothing, as
// accounts.Registry.CheckNewPool checks it. It fails with a Refused of its
// label or its directories, as refusedPool has it, and as CheckNewPool does
// otherwise.
func (s *Setup) CheckPool(pool accounts.NewPool) error {
	return refusedPool(s.Accounts.CheckNewPool(pool))
}

// refusedPool is err, a pool to make refused, as an accounts.NewPoolError
// says, as a Refused of the value it's of: PoolLabelField, of a label that's
// none, another pool's, or one config refuses; DirsField, of a directory
// config refuses, the one at Dir, or, where it would be a second pool
// unfenced, of the first; else err as it is.
func refusedPool(err error) error {
	if _, ok := errors.AsType[*accounts.NewPoolError](err); !ok {
		return err
	}
	if wrong, ok := errors.AsType[*config.WrongLabel](err); ok {
		return &Refused{Field: PoolLabelField, Why: wrong.Why, err: err}
	}
	if dir, ok := errors.AsType[*config.DirError](err); ok {
		return &Refused{Field: DirsField, Dir: dir.Dir, Why: dir.Why, err: err}
	}
	if unfenced, ok := errors.AsType[*accounts.UnfencedError](err); ok {
		return &Refused{Field: DirsField, Why: unfenced.Error(), err: err}
	}
	if taken, ok := errors.AsType[*accounts.PoolLabelTakenError](err); ok {
		return &Refused{Field: PoolLabelField, Why: taken.Error(), err: err}
	}
	if errors.Is(err, accounts.ErrNoPoolLabel) {
		return &Refused{Field: PoolLabelField, Why: accounts.ErrNoPoolLabel.Error(), err: err}
	}
	return err
}

// refusedAccount is err, an account added or edited refused, as a Refused of
// the field it's of: Pool, where the pool chosen is one config no longer has,
// or the new pool's refused, as chosenPoolRefused has it; Label, as
// refusedLabel has it; else err as it is, as where the account edited is no
// longer configured.
func refusedAccount(err error) error {
	if errors.Is(err, config.ErrPoolNotConfigured) {
		return &Refused{Field: PoolField, Why: err.Error(), err: err}
	}
	return refusedLabel(chosenPoolRefused(err))
}

// chosenPoolRefused is err, of an account added to a pool, or moved into
// one, as a Refused of its Pool where it's a new pool's refusal, as
// refusedPool has it; else err as it is.
func chosenPoolRefused(err error) error {
	if refused, ok := errors.AsType[*Refused](refusedPool(err)); ok {
		return &Refused{Field: PoolField, Why: refused.Why, err: err}
	}
	return err
}

// refusedLabel is err, as an account's label is refused: by config, as one
// no account can have, or as none, or another configured account's, as
// accounts has them; a Refused of Label, why as config words it alone, or as
// accounts words it; err as it is where it's a Refused already, as a new
// pool's label refused is Pool's; else err as it is.
func refusedLabel(err error) error {
	if _, ok := errors.AsType[*Refused](err); ok {
		return err
	}
	if wrong, ok := errors.AsType[*config.WrongLabel](err); ok {
		return &Refused{Field: LabelField, Why: wrong.Why, err: err}
	}
	if errors.Is(err, accounts.ErrNoLabel) || errors.Is(err, accounts.ErrLabelTaken) {
		return &Refused{Field: LabelField, Why: err.Error(), err: err}
	}
	return err
}
