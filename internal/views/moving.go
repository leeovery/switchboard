package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// Moving is what a pin that moves the sessions running moves: how many
// sessions, and what rebuilding their caches costs, nil where no move's cost
// is known.
type Moving struct {
	Sessions int
	Cost     *ledger.Picodollars
}

// MovingTo is what a pin of the accounts with the given ids moves of the
// sessions running, as the router lists them, priced by listed, the List's
// sessions: every one with a model on an account the pin doesn't name, but
// one with a pin of its own, as the router moves each such model; and the
// cost of each whose conversation moves, its account not among those named,
// as its MoveCost prices it. With no account, auto, nothing moves.
func MovingTo(accounts []string, running []status.Session, listed []ListedSession) Moving {
	if len(accounts) == 0 {
		return Moving{}
	}
	var (
		m    Moving
		cost Worth
	)
	for _, s := range running {
		if !moves(s, accounts) {
			continue
		}
		m.Sessions++
		if c, ok := conversationMove(s.ID, accounts, listed); ok {
			cost.addAmount(c)
		}
	}
	m.Cost = cost.Amount
	return m
}

// moves reports whether a pin of the accounts given, moving the sessions
// running, moves the session: it has no pin of its own, and a model of its
// goes to an account the pin doesn't name.
func moves(s status.Session, accounts []string) bool {
	return s.Pin == "" && slices.ContainsFunc(s.Assignments, func(a status.Assignment) bool {
		return !slices.Contains(accounts, a.Account)
	})
}

// conversationMove is what moving the conversation of the session with the
// given id costs, as listed prices it, where a pin of the accounts given
// moves it. It reports false where it stays, or its cost isn't known.
func conversationMove(id string, accounts []string, listed []ListedSession) (ledger.Picodollars, bool) {
	i := slices.IndexFunc(listed, func(l ListedSession) bool { return l.Session == id })
	if i < 0 || listed[i].MoveCost == nil || slices.Contains(accounts, listed[i].Account) {
		return 0, false
	}
	return *listed[i].MoveCost, true
}
