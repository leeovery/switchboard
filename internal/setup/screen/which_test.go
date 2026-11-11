package screen

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/setup"
)

// teamAsked is a token made to add team that setup asks about, its
// subscription's organization matching no account this Mac has had: it may
// be spare or old, each removed before switchboard made ids.
func teamAsked() *setup.Asked {
	return &setup.Asked{Question: accounts.Question{Named: true, Choices: []identity.Entry{
		{ID: "spare", Label: "Spare", Removed: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)},
		{ID: "old", Label: "Old", Removed: time.Date(2026, 3, 2, 23, 30, 0, 0, time.UTC)},
	}}}
}

// madeTeam is the token made to add team, a placeholder.
const madeTeam = "test-token-team"

// askingTeam is a harness of frame 3's accounts, team being added from + add an
// account, its token made and asked about as asked has it.
func askingTeam(t *testing.T, asked *setup.Asked) *harness {
	t.Helper()
	h := accountsHarness(t, frame3Steps())
	h.steps.making = fakeMaking{steps: []setup.Making{privately, approving, madeIt, checkIt}, ends: &setup.Tokened{Result: setup.TokenAsked, Asked: asked, Shown: []string{madeTeam}}}
	h.steps.answerFor = setup.TokenFor{Account: config.NewAccount{Label: "team"}, Add: true}
	h.start()
	for range 3 {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("enter")
	h.typeIn("team")
	h.pressAndDeliver("enter")
	h.hearAll(5)
	return h
}

// The card asking which account team's token is for, as it opens: why it
// asks, then a new account chosen, then each account it may be, by the label
// it last showed and the day it was last seen, in the clock's zone.
var whichCardOpened = []string{
	"",
	"  This subscription's organization matches no account this Mac has had.",
	"  Is it a new account, or one removed before switchboard made ids?",
	"",
	"  ■ new account",
	"  □ Spare          last seen 14 Sep 2026",
	"  □ Old            last seen 3 Mar 2026",
	"",
	"                                         ↑↓ select   enter save   esc cancel",
}

// A token made to add an account, whose organization doesn't settle which
// account it's for, closes the token card, the card asking which open in its
// place, nothing added.
func TestATokenAskedAboutOpensTheCardAskingWhich(t *testing.T) {
	h := askingTeam(t, teamAsked())
	if h.model.token != nil {
		t.Errorf("asked about, the token card is %+v, want it given way to the card asking which", h.model.token)
	}
	checkCard(t, h, "WHICH ACCOUNT · TEAM", whichCardOpened)
	if review := h.model.status.review; review == nil || len(review.Accounts) != 3 {
		t.Errorf("asked about, the review is %+v, want nothing added", review)
	}
}

// ↓ and j choose the answer below, ↑ and k the one above, no further than
// the last or the first, the mark moving with the choice.
func TestTheCardAskingWhichMovesItsChoice(t *testing.T) {
	h := askingTeam(t, teamAsked())
	for _, tt := range []struct {
		key  string
		want int
	}{{"down", 1}, {"j", 2}, {"down", 2}, {"up", 1}, {"k", 0}, {"k", 0}} {
		h.pressAndDeliver(tt.key)
		if got := h.model.which.chosen; got != tt.want {
			t.Fatalf("%s chose answer %d, want %d", tt.key, got, tt.want)
		}
	}
	h.pressAndDeliver("down")
	if got := h.cardRow("■"); !strings.HasPrefix(got, "  ■ Spare") {
		t.Errorf("the row marked reads %q, want Spare's", got)
	}
}

// enter saves the token as the answer chosen says, with Answer, the card
// taking no key till it's answered: saved, the card closes, the account
// added, its row chosen.
func TestEnterSavesTheTokenAsTheAnswerChosenSays(t *testing.T) {
	tests := []struct {
		name  string
		downs int
		want  accounts.Answer
	}{
		{name: "a new account", downs: 0, want: accounts.Answer{New: true}},
		{name: "one it may be", downs: 2, want: accounts.Answer{ID: "old"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asked := teamAsked()
			h := askingTeam(t, asked)
			for range tt.downs {
				h.pressAndDeliver("down")
			}
			answered := h.press("enter")
			if !h.model.which.saving {
				t.Fatal("enter, and the card isn't saving")
			}
			for _, k := range []string{"up", "down", "esc", "enter"} {
				h.deliver(h.press(k)...)
			}
			if card := h.model.which; card == nil || card.chosen != tt.downs {
				t.Fatalf("while it's saving, the card is %+v, want it open as it was", card)
			}
			h.deliver(answered...)
			if want := []fakeAnswer{{asked: asked, answer: tt.want}}; !slices.Equal(h.steps.answers, want) {
				t.Errorf("Answer was given %+v, want %+v", h.steps.answers, want)
			}
			if h.model.which != nil {
				t.Errorf("saved, the card is %+v, want it closed", h.model.which)
			}
			if review := h.model.status.review; review == nil || len(review.Accounts) != 4 || h.model.chosen != 3 {
				t.Errorf("saved, the review is %+v, row %d chosen; want team added, its row chosen", review, h.model.chosen)
			}
		})
	}
}

// An answer that saves nothing: asked again, as the accounts the token may
// be changed since, the card asks afresh; refused, the token card says why
// in its place; cancelled, the card closes. Nothing is added.
func TestAnAnswerThatSavesNothing(t *testing.T) {
	again := &setup.Asked{Question: accounts.Question{Named: true, Choices: []identity.Entry{{ID: "old", Label: "Old"}}}}
	tests := []struct {
		name  string
		ends  setup.Tokened
		card  string
		check func(t *testing.T, h *harness)
	}{
		{name: "asked again", ends: setup.Tokened{Result: setup.TokenAsked, Asked: again}, card: "WHICH ACCOUNT · TEAM", check: func(t *testing.T, h *harness) {
			if card := h.model.which; card.asked != again || card.chosen != 0 {
				t.Errorf("asked again, the card is %+v, want it asking afresh, a new account chosen", card)
			}
		}},
		{name: "configured already", ends: setup.Tokened{Result: setup.TokenSameAccount, SameAs: "Side"}, card: "NEW TOKEN · TEAM", check: func(t *testing.T, h *harness) {
			if h.model.token == nil || h.model.token.unsaved == nil {
				t.Errorf("refused, the token card is %+v, want it saying why", h.model.token)
			}
		}},
		{name: "cancelled", ends: setup.Tokened{Result: setup.TokenCancelled}, card: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := askingTeam(t, teamAsked())
			h.steps.answerEnds = &tt.ends
			h.pressAndDeliver("down")
			h.pressAndDeliver("enter")
			if title, _ := h.card(); title != tt.card {
				t.Errorf("answered, the card open is %q, want %q", title, tt.card)
			}
			if tt.check != nil {
				tt.check(t, h)
			}
			if review := h.model.status.review; review == nil || len(review.Accounts) != 3 {
				t.Errorf("answered, the review is %+v, want nothing added", review)
			}
		})
	}
}

// esc closes the card, nothing answered or added: a token made, which lasts
// a year on the account, is shown in its making's token card in its place,
// to copy, and given back should setup go; a token pasted, still the user's,
// closes with it.
func TestEscClosesTheCardAskingWhich(t *testing.T) {
	t.Run("made", func(t *testing.T) {
		h := askingTeam(t, teamAsked())
		h.pressAndDeliver("esc")
		title, rows := h.card()
		if title != "NEW TOKEN · TEAM" || !slices.ContainsFunc(rows, func(row string) bool { return strings.Contains(row, madeTeam) }) {
			t.Errorf("esc left the card %q reading\n%s\nwant team's token card, showing the token", title, strings.Join(rows, "\n"))
		}
		if unsaved, _ := h.model.tokenLeft(); unsaved == nil || !slices.Equal(unsaved.Shown, []string{madeTeam}) {
			t.Errorf("tokenLeft() = %+v, want the token given back", unsaved)
		}
		checkNothingAnswered(t, h)
	})
	t.Run("pasted", func(t *testing.T) {
		h := pastingTeamAsked(t)
		h.pressAndDeliver("esc")
		if title, _ := h.card(); title != "" {
			t.Errorf("esc left the card %q open", title)
		}
		checkNothingAnswered(t, h)
	})
}

// checkNothingAnswered checks Answer was given nothing, and nothing was
// added.
func checkNothingAnswered(t *testing.T, h *harness) {
	t.Helper()
	if len(h.steps.answers) != 0 {
		t.Errorf("Answer was given %+v, want nothing answered", h.steps.answers)
	}
	if review := h.model.status.review; review == nil || len(review.Accounts) != 3 {
		t.Errorf("the review is %+v, want nothing added", review)
	}
}

// What a made token's answer saved once the screen had gone is how setup's
// left: nothing to give back where it saved the token; the token, where it
// couldn't.
func TestAnAnswerSavedOnceTheScreenWentIsHowSetupsLeft(t *testing.T) {
	tests := []struct {
		name string
		ends setup.Tokened
		want []string
	}{
		{name: "saved", ends: saved},
		{name: "not saved", ends: setup.Tokened{Result: setup.TokenNotSaved, Why: "the config changed while it was being edited", Shown: []string{madeTeam}}, want: []string{madeTeam}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := askingTeam(t, teamAsked())
			h.steps.answerEnds = &tt.ends
			h.press("enter")
			unsaved, _ := h.model.tokenLeft()
			if tt.want == nil && unsaved != nil || tt.want != nil && (unsaved == nil || !slices.Equal(unsaved.Shown, tt.want)) {
				t.Errorf("tokenLeft() = %+v, want the token given back: %v", unsaved, tt.want != nil)
			}
		})
	}
}

// A token pasted to add an account is asked about as a made one is: the
// paste card closes, the card asking which open in its place.
func TestAPastedTokenAskedAboutOpensTheCardAskingWhich(t *testing.T) {
	h := pastingTeamAsked(t)
	if h.model.paste != nil {
		t.Errorf("asked about, the paste card is %+v, want it given way to the card asking which", h.model.paste)
	}
	checkCard(t, h, "WHICH ACCOUNT · TEAM", whichCardOpened)
}

// pastingTeamAsked is a harness of frame 3's accounts, team being added
// from + add an account, its token's making failing, then a token pasted in
// its place, asked about as teamAsked has it.
func pastingTeamAsked(t *testing.T) *harness {
	t.Helper()
	h := accountsHarness(t, frame3Steps())
	h.steps.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &notMade}
	h.steps.pasteEnds = &setup.Tokened{Result: setup.TokenAsked, Asked: teamAsked()}
	h.start()
	for range 3 {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("enter")
	h.typeIn("team")
	h.pressAndDeliver("enter")
	h.hearAll(3)
	h.pressAndDeliver("p")
	h.typeIn(madeTeam)
	h.pressAndDeliver("enter")
	return h
}

// A made token an answer didn't save stays held, however the cards come and
// go: setup left gives it back, as the answer ended where that shows it,
// with the save's failure's words, else as the making ended, which shows it;
// and so after p, then esc back to its card.
func TestAMadeTokenAnAnswerDidntSaveIsGivenBack(t *testing.T) {
	notSaved := setup.Tokened{Result: setup.TokenNotSaved, Why: "the config changed while it was being edited", Shown: []string{madeTeam}}
	tests := []struct {
		name string
		ends setup.Tokened
		// want is what setup left gives back: the making's ending, asked,
		// where it's zero.
		want setup.Tokened
	}{
		{name: "not saved", ends: notSaved, want: notSaved},
		{name: "configured already", ends: setup.Tokened{Result: setup.TokenSameAccount, SameAs: "Side"}},
	}
	for _, tt := range tests {
		for _, keys := range [][]string{nil, {"p", "esc"}} {
			t.Run(tt.name+" "+strings.Join(keys, " then "), func(t *testing.T) {
				h := askingTeam(t, teamAsked())
				h.steps.answerEnds = &tt.ends
				h.pressAndDeliver("enter")
				for _, key := range keys {
					h.pressAndDeliver(key)
				}
				if title, _ := h.card(); title != "NEW TOKEN · TEAM" {
					t.Errorf("the card open is %q, want team's token card", title)
				}
				want := tt.want
				if want.Result == 0 {
					want = setup.Tokened{Result: setup.TokenAsked}
				}
				if unsaved, _ := h.model.tokenLeft(); unsaved == nil || unsaved.Result != want.Result || unsaved.Why != want.Why || !slices.Equal(unsaved.Shown, []string{madeTeam}) {
					t.Errorf("tokenLeft() = %+v, want %+v, the token given back", unsaved, want)
				}
			})
		}
	}
}
