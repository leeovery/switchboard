package status

import "github.com/leeovery/switchboard/internal/quota"

// ExtraState is how an account stands for extra usage, which serves its
// requests past its limit and bills for them, as Accounts says it.
type ExtraState int

// The states, from not read, as nothing has given it.
const (
	// ExtraNotRead is extra usage nothing has given.
	ExtraNotRead ExtraState = iota
	// ExtraOff is extra usage given as off: its status rejected.
	ExtraOff
	// ExtraOn is extra usage given as on: its status anything but rejected.
	ExtraOn
)

// ExtraSaid is how the account stands for extra usage, as last given: on
// while its status isn't rejected, off where it is, and not read where
// nothing has given it.
func (a AccountNow) ExtraSaid() ExtraState {
	switch {
	case !a.Extra.Given():
		return ExtraNotRead
	case a.Extra.Status == quota.StatusRejected:
		return ExtraOff
	}
	return ExtraOn
}

// AllExtra is how every account stands for extra usage, taken together, as
// all accounts says it.
type AllExtra struct {
	// State is not read where none of them is, off where none read is on,
	// and on where any is.
	State ExtraState
	// On is how many of them are read on, of Of, every account: all of them,
	// as "on on all" says, or some, as "on for 1 of 3".
	On, Of int
}

// ExtraOfAll is how the accounts stand for extra usage, taken together, each
// as its ExtraSaid has it: those not read counted in Of alone.
func ExtraOfAll(accounts []AccountNow) AllExtra {
	all := AllExtra{Of: len(accounts)}
	off := 0
	for _, a := range accounts {
		switch a.ExtraSaid() {
		case ExtraOn:
			all.On++
		case ExtraOff:
			off++
		}
	}
	switch {
	case all.On > 0:
		all.State = ExtraOn
	case off > 0:
		all.State = ExtraOff
	}
	return all
}

// ExtraOf is how the accounts at the moment with the ids given stand for
// extra usage, taken together, as ExtraOfAll has it: those the config has,
// whatever the moment holds, one it lacks not read, as an account the config
// has just gained.
func (m Moment) ExtraOf(ids []string) AllExtra {
	accounts := make([]AccountNow, len(ids))
	for i, id := range ids {
		accounts[i], _ = m.Account(id)
	}
	return ExtraOfAll(accounts)
}
