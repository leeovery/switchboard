package setup

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
)

// What setup says of pools: the option that makes one, on the add card and
// the edit card; what that option does, under Pool; what moving an account
// does, under the edit card's Pool; and where the unfenced pool's sessions
// start, beside its label over its accounts.
const (
	NewPoolOption  = "new pool"
	NewPoolHint    = "a pool of its own: enter gives its label and directories"
	MovingHint     = "moving it takes its sessions off at once"
	EverywhereElse = "everywhere else"
)

// PoolChoice is a pool a card's Pool offers: one configured, or the new pool
// the card holds, by its label and the directories it's fenced to, as the
// config writes them, none of the unfenced pool's; whether it has accounts;
// and whether it's the first pool, whose primary the Claude apps sign in to.
type PoolChoice struct {
	ID, Label string
	Dirs      []string
	// Held is set on the new pool the card holds, made with the account.
	Held     bool
	Accounts bool
	First    bool
}

// PoolChoices are the review's pools, as a card's Pool offers them, in the
// config's order.
func (r Review) PoolChoices() []PoolChoice {
	choices := make([]PoolChoice, len(r.Pools))
	for i, p := range r.Pools {
		in := func(a Reviewed) bool { return config.SamePool(a.Pool, p.ID) }
		choices[i] = PoolChoice{ID: p.ID, Label: p.Label, Dirs: p.WrittenDirs, Accounts: slices.ContainsFunc(r.Accounts, in), First: config.SamePool(p.ID, r.First)}
	}
	return choices
}

// UsedBy says which sessions use the pool, as the add card's hint under Pool
// says it of the pool chosen, of pools in all: of a fenced pool, those
// started in its directories alone; of the unfenced, every session, where
// it's the only pool, else those started everywhere else.
func (c PoolChoice) UsedBy(pools int) string {
	switch {
	case len(c.Dirs) > 0:
		return "only sessions started in " + prose.ListOr(cleaned(c.Dirs)) + " use it"
	case pools == 1:
		return "every session uses it"
	}
	return "sessions started " + EverywhereElse + " use it"
}

// Places says where the sessions the pool serves start, beside its label
// over its accounts: its directories, as the config writes them, or, of the
// unfenced pool, everywhere else.
func Places(p config.Pool) string {
	if len(p.WrittenDirs) == 0 {
		return EverywhereElse
	}
	return strings.Join(cleaned(p.WrittenDirs), ", ")
}

// PrimaryHint is the edit card's hint under Primary, of the pool: its
// sessions start on its primary, which the Claude apps sign in to, where
// it's the first.
func (c PoolChoice) PrimaryHint() string {
	hint := prose.Clean(c.Label) + "'s sessions start on it"
	if c.First {
		hint += "; the Claude apps sign in to it"
	}
	return hint
}

// The edit card's hints under Reserve: of an account in the first pool,
// whose primary the Claude apps sign in to, AppsReserveHint; of one in
// another, PoolReserveHint, as the apps never use it.
const (
	AppsReserveHint = "usage kept back for the Claude apps"
	PoolReserveHint = "usage the router leaves unused"
)

// ReserveHint is the edit card's hint under Reserve, of an account in the
// pool, as AppsReserveHint and PoolReserveHint say.
func (c PoolChoice) ReserveHint() string {
	if c.First {
		return AppsReserveHint
	}
	return PoolReserveHint
}

// NewPoolChoice is the new pool a card's Pool offers before it's made, as
// Primary's and Reserve's hints name it.
var NewPoolChoice = PoolChoice{Label: "the new pool"}

// cleaned are texts made safe to show.
func cleaned(texts []string) []string {
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = prose.Clean(t)
	}
	return out
}
