package accounts

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// NewPool is a pool made with the account that's the first to go in it: shown
// as Label, fenced to Dirs, each an absolute path or one starting ~/, or
// unfenced without any. It's written in the one edit that adds the account
// or moves it, under an id switchboard makes as it's written, so an add
// cancelled, or a token that fails, leaves no pool: an empty fenced pool
// would stop every session started in its directories.
type NewPool struct {
	Label string
	Dirs  []string
}

// Adding is an account to add, and the pool made with it, where it goes in a
// new one: NewPool nil where it goes in one configured, Account's Pool, or
// without one, the pool accounts add adds it to without --pool.
type Adding struct {
	Account config.NewAccount
	NewPool *NewPool
}

// NewPoolError is the error of a pool to make with an account, which the
// config's rules refuse, as AddPool refuses one: Err says why.
type NewPoolError struct {
	Err error
}

func (e *NewPoolError) Error() string {
	return "the new pool: " + e.Err.Error()
}

// Unwrap is why the pool is refused.
func (e *NewPoolError) Unwrap() error {
	return e.Err
}

// CheckNewPool checks a pool to make with an account against the config file
// as it stands, as AddPool checks a pool it adds, writing nothing. It fails
// with a NewPoolError where the pool's refused, as AddPool says, and as
// config.Edit does otherwise.
func (r Registry) CheckNewPool(pool NewPool) error {
	draft, err := config.Edit(r.ConfigPath, r.HomeDir)
	if err != nil {
		return err
	}
	_, err = madeIn(draft, pool, checkedID)
	return err
}

// poolID gives the id of a pool being made, apart from the ids configured,
// the config's accounts' and pools'.
type poolID func(configured []string) (string, error)

// drawnFrom is the poolID that draws the id of a pool being made from the
// accounts record, as identity's NewPoolID draws one, under the record's
// lock, never one it holds.
func (r Registry) drawnFrom(record identity.Record) poolID {
	return func(configured []string) (string, error) {
		return record.NewPoolID(r.random(), configured)
	}
}

// apartFrom is the poolID that gives an id apart from the one given too, as
// an account's drawn before the pool it's added with: "" is apart from every
// id.
func (id poolID) apartFrom(other string) poolID {
	return func(configured []string) (string, error) {
		return id(append(configured, other))
	}
}

// checkedID is the poolID a pool is checked under, written nowhere: the first
// of z000 to z999, an id switchboard could make, that isn't configured, so
// a check draws nothing.
func checkedID(configured []string) (string, error) {
	for n := range 1000 {
		id := fmt.Sprintf("z%03d", n)
		if !slices.ContainsFunc(configured, func(c string) bool { return strings.EqualFold(c, id) }) {
			return id, nil
		}
	}
	return "", fmt.Errorf("check the new pool: every id it could be checked under is configured")
}

// madeIn adds pool to the draft, under the id given, as AddPool adds one,
// returning the id: it fails with ErrNoPools, unwrapped, in a config from
// before pools, and with a NewPoolError where the pool's refused, as
// poolMade has it.
func madeIn(draft *config.Draft, pool NewPool, id poolID) (string, error) {
	added, err := poolMade(draft, pool.Label, pool.Dirs, id)
	if err != nil && !errors.Is(err, ErrNoPools) {
		return "", &NewPoolError{Err: err}
	}
	return added, err
}

// poolMade adds a pool shown as label, fenced to dirs, to the draft, under
// the id given apart from those configured, returning it: it fails with
// ErrNoPools in a config from before pools; with ErrNoPoolLabel, or a
// PoolLabelTakenError, for a label that's none or another pool's; with an
// UnfencedError where it would be a second pool unfenced; and as the
// draft's AddPool does, as for directories that overlap another's.
func poolMade(draft *config.Draft, label string, dirs []string, id poolID) (string, error) {
	cfg := draft.Config()
	if cfg == nil || len(cfg.Pools) == 0 {
		return "", ErrNoPools
	}
	if err := poolTaken(cfg.Pools, "", label, len(dirs) == 0); err != nil {
		return "", err
	}
	made, err := id(slices.Concat(cfg.Accounts.IDs(), cfg.Pools.IDs()))
	if err != nil {
		return "", err
	}
	return made, draft.AddPool(config.NewPool{ID: made, Label: label, Dirs: dirs})
}
