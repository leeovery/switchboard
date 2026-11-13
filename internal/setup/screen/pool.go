package screen

import (
	"slices"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// poolField is a card's Pool as the screen holds it: the pools configured,
// as the review gives them; the new pool the card holds, made with the
// account, where it holds one; and the option chosen, by its place among
// the options, as options has them.
type poolField struct {
	pools  []setup.PoolChoice
	made   *accounts.NewPool
	chosen int
}

// poolOf is the Pool of a card over review, the pool with the given id
// chosen, else the first: nil in a config without pools, where an account
// has none to choose.
func poolOf(review *setup.Review, id string) *poolField {
	if review == nil || len(review.Pools) == 0 {
		return nil
	}
	f := &poolField{pools: review.PoolChoices()}
	f.chosen = max(slices.IndexFunc(f.pools, func(p setup.PoolChoice) bool { return config.SamePool(p.ID, id) }), 0)
	return f
}

// choices are the pools the field offers: those configured, then the new
// pool it holds, where it holds one.
func (f poolField) choices() []setup.PoolChoice {
	if f.made == nil {
		return f.pools
	}
	return append(slices.Clone(f.pools), setup.PoolChoice{Label: f.made.Label, Dirs: f.made.Dirs, Held: true})
}

// options are the field's options, each pool by its label, as choices has
// them, then new pool.
func (f poolField) options() []string {
	var options []string
	for _, p := range f.choices() {
		options = append(options, p.Label)
	}
	return append(options, setup.NewPoolOption)
}

// choice is the pool chosen, reporting false where new pool is.
func (f poolField) choice() (setup.PoolChoice, bool) {
	choices := f.choices()
	if f.chosen >= len(choices) {
		return setup.PoolChoice{}, false
	}
	return choices[f.chosen], true
}

// moved is the field with the option by options on from the one chosen
// chosen, no further than the first or the last.
func (f poolField) moved(by int) poolField {
	f.chosen = min(max(f.chosen+by, 0), len(f.options())-1)
	return f
}

// holding is the field holding the new pool given, made with the account,
// in place of any it held, and chosen.
func (f poolField) holding(pool accounts.NewPool) poolField {
	f.made, f.chosen = &pool, len(f.pools)
	return f
}

// usedBy says which sessions use the pool chosen, as setup words it, of the
// pools the field offers: what new pool does, where it's chosen.
func (f poolField) usedBy() string {
	if p, ok := f.choice(); ok {
		return p.UsedBy(len(f.choices()))
	}
	return setup.NewPoolHint
}

// drawn is the field as dashboard.Setup draws it, hinted as given.
func (f poolField) drawn(hint string) *dashboard.SetupPool {
	_, chosen := f.choice()
	return &dashboard.SetupPool{Options: f.options(), Chosen: f.chosen, Hint: hint, New: !chosen}
}

// placed is the pool the field has an account go in: the id of the one
// configured chosen, or the new pool it holds, chosen; "" and nil where new
// pool is.
func (f poolField) placed() (string, *accounts.NewPool) {
	p, ok := f.choice()
	switch {
	case !ok:
		return "", nil
	case p.Held:
		return "", f.made
	}
	return p.ID, nil
}
