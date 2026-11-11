package setup

import (
	"cmp"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/prose"
)

// WhichAsk is what the card asking which account a token is for says, where
// its organization doesn't settle it: why it asks, a line each; and its
// choices, a new account first, then each account the token may be.
type WhichAsk struct {
	Says    []string
	Choices []WhichChoice
}

// WhichChoice is an answer the card asking which account a token is for
// offers: a new account, or an account by the label it last showed, and
// when it was last seen, "" where it wasn't removed.
type WhichChoice struct {
	Label, Seen string
}

// newAccountChoice is the card's first choice, a new account.
const newAccountChoice = "new account"

// Asking is what the card asking which account the token asked is for says
// at now, each account it may be seen last on the day it was removed, in
// now's time zone.
func (a *Asked) Asking(now time.Time) WhichAsk {
	ask := WhichAsk{Says: []string{
		"This subscription's organization matches no account this Mac has had.",
		"Is it a new account, or one removed before switchboard made ids?",
	}}
	if !a.Question.Named {
		ask.Says = []string{
			"The API didn't name this subscription's organization.",
			"Is it a new account, or one this Mac had?",
		}
	}
	ask.Choices = []WhichChoice{{Label: newAccountChoice}}
	for _, e := range a.Question.Choices {
		choice := WhichChoice{Label: cmp.Or(e.Label, e.ID)}
		if !e.Removed.IsZero() {
			choice.Seen = "last seen " + prose.DayMonthYear(e.Removed.In(now.Location()))
		}
		ask.Choices = append(ask.Choices, choice)
	}
	return ask
}

// Answered is the answer of the choice at the place given, as Asking offers
// them: the first a new account, then each account the token may be.
func (a *Asked) Answered(choice int) accounts.Answer {
	if choice == 0 {
		return accounts.Answer{New: true}
	}
	return accounts.Answer{ID: a.Question.Choices[choice-1].ID}
}

// Name is how the token's cards name the account f is for: by its label,
// where it's to be added, as it has no id yet; else by its id.
func (f TokenFor) Name() string {
	if f.Add {
		return f.Account.Label
	}
	return f.Account.ID
}
