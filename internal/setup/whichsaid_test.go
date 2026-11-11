package setup

import (
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// The card asking which account a token is for says why it asks, as the
// API named the token's organization or didn't; then offers a new account
// first, then each account the token may be, by the label it last showed,
// else its id, seen last on the day it was removed in the clock's zone.
func TestAskingSaysWhyAndOffersEachAccountTheTokenMayBe(t *testing.T) {
	east := time.FixedZone("UTC+1", 60*60)
	now := time.Date(2026, 10, 27, 17, 42, 0, 0, east)
	choices := []identity.Entry{
		{ID: "spare", Label: "Spare", Removed: time.Date(2026, 9, 14, 23, 30, 0, 0, time.UTC)},
		{ID: "k3f9"},
	}
	offered := []WhichChoice{{Label: "new account"}, {Label: "Spare", Seen: "last seen 15 Sep 2026"}, {Label: "k3f9"}}
	tests := []struct {
		name  string
		named bool
		says  []string
	}{
		{name: "the organization named", named: true, says: []string{
			"This subscription's organization matches no account this Mac has had.",
			"Is it a new account, or one removed before switchboard made ids?",
		}},
		{name: "no organization named", says: []string{
			"The API didn't name this subscription's organization.",
			"Is it a new account, or one this Mac had?",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asked := &Asked{Question: accounts.Question{Named: tt.named, Choices: choices}}
			want := WhichAsk{Says: tt.says, Choices: offered}
			if got := asked.Asking(now); !reflect.DeepEqual(got, want) {
				t.Errorf("Asking() = %+v, want %+v", got, want)
			}
		})
	}
}

// Each choice answers as Asking offers it: the first a new account, then
// each account the token may be, by its id.
func TestAnsweredIsTheAnswerOfTheChoiceAtItsPlace(t *testing.T) {
	asked := &Asked{Question: accounts.Question{Named: true, Choices: []identity.Entry{{ID: "spare"}, {ID: "old"}}}}
	for choice, want := range []accounts.Answer{{New: true}, {ID: "spare"}, {ID: "old"}} {
		if got := asked.Answered(choice); got != want {
			t.Errorf("Answered(%d) = %+v, want %+v", choice, got, want)
		}
	}
}

// The token's cards name an account by its label, as the user knows it,
// where it has one, else by its id.
func TestTheTokensCardsNameTheAccount(t *testing.T) {
	account := config.NewAccount{ID: "work", Label: "Work"}
	if got := (TokenFor{Account: account, Add: true}).Name(); got != "Work" {
		t.Errorf("an add is named %q, want its label", got)
	}
	if got := (TokenFor{Account: account}).Name(); got != "Work" {
		t.Errorf("a replace is named %q, want its label", got)
	}
	if got := (TokenFor{Account: config.NewAccount{ID: "work"}}).Name(); got != "work" {
		t.Errorf("a replace without a label is named %q, want its id", got)
	}
}
