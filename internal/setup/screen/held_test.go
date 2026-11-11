package screen

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/setup"
)

// madePersonalUnsaved is a harness of frame 3's accounts, personal's token
// made but not saved, as madeNotSaved has it, its card closed with esc.
func madePersonalUnsaved(t *testing.T) *harness {
	t.Helper()
	h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}, ends: &madeNotSaved}, 1)
	h.steps.nextMake(t)
	h.hearAll(3)
	h.pressAndDeliver("esc")
	if title, _ := h.card(); title != "" {
		t.Fatalf("esc left the card %q open", title)
	}
	return h
}

// remake is h with t pressed on the row downs rows down, its making ending
// as ends has it, for account id.
func remake(t *testing.T, h *harness, downs int, id string, ends setup.Tokened) {
	t.Helper()
	h.steps.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &ends}
	for range downs {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("t")
	if got := h.steps.nextMake(t); got.Account.ID != id {
		t.Fatalf("MakeToken was asked %+v, want %s's token", got, id)
	}
	h.hearAll(3)
}

// personalLeft is personal's token made but unsaved, as setup gives it back.
var personalLeft = TokenLeft{For: "personal", Tokened: madeNotSaved}

// Every token the screen makes is held till a save of it is known done,
// whatever's made after: a second making, side's, that's cancelled or saved,
// leaves personal's token made but unsaved given back, and only it, each by
// the account it's for.
func TestEveryTokenMadeIsHeldTillItsSaved(t *testing.T) {
	tests := []struct {
		name string
		ends setup.Tokened
	}{
		{name: "the second cancelled", ends: setup.Tokened{Result: setup.TokenCancelled}},
		{name: "the second saved", ends: saved},
		{name: "the second not saved", ends: madeUnread},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := madePersonalUnsaved(t)
			remake(t, h, 1, "side", tt.ends)
			h.pressAndDeliver("esc")

			want := []TokenLeft{personalLeft}
			if tt.ends.Result == setup.TokenUnread {
				want = append(want, TokenLeft{For: "side", Tokened: tt.ends})
			}
			if unsaved, _ := h.model.tokenLeft(); !reflect.DeepEqual(unsaved, want) {
				t.Errorf("tokenLeft() = %+v, want %+v", unsaved, want)
			}
		})
	}
}

// A configured account's token saved is every token made for it saved: a
// second making for personal that saves leaves nothing of the first given
// back.
func TestATokenSavedForAnAccountIsEveryOneMadeForItSaved(t *testing.T) {
	h := madePersonalUnsaved(t)
	remake(t, h, 0, "personal", saved)
	if unsaved, _ := h.model.tokenLeft(); len(unsaved) > 0 {
		t.Errorf("tokenLeft() = %+v, want nothing given back", unsaved)
	}
}

// addTeam is h with team being added from + add an account, the row downs
// rows down, its making ending as ends has it.
func addTeam(h *harness, downs int, ends setup.Tokened) {
	h.steps.making = fakeMaking{steps: []setup.Making{privately, approving, madeIt, checkIt}, ends: &ends}
	for range downs {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("enter")
	h.typeIn("team")
	h.pressAndDeliver("enter")
	h.hearAll(5)
}

// Two accounts to add that share a label can be different subscriptions, so
// a save of one's token is no save of the other's: team's second making
// saved, its first, unsaved, is still given back.
func TestATokenSavedToAddAnAccountIsItsMakingsAlone(t *testing.T) {
	first := setup.Tokened{Result: setup.TokenNotSaved, Why: "write the token file: permission denied", Shown: []string{madeTeam}}
	h := accountsHarness(t, frame3Steps())
	h.start()
	addTeam(h, 3, first)
	h.pressAndDeliver("esc")
	addTeam(h, 0, saved)
	if title, _ := h.card(); title != "" {
		t.Fatalf("team's second token saved, the card %q is open", title)
	}
	want := []TokenLeft{{For: "team", Tokened: first}}
	if unsaved, _ := h.model.tokenLeft(); !reflect.DeepEqual(unsaved, want) {
		t.Errorf("tokenLeft() = %+v, want team's first token given back", unsaved)
	}
}

// A token made but unsaved that's then saved, by a paste in its place or by
// the user's answer to which account it's for, is given back no longer.
func TestATokenMadeThenSavedIsntGivenBack(t *testing.T) {
	t.Run("a paste", func(t *testing.T) {
		h := pastingFor(t, madeNotSaved)
		h.typeIn("test-token-typed")
		h.pressAndDeliver("enter")
		if unsaved, _ := h.model.tokenLeft(); len(unsaved) > 0 {
			t.Errorf("tokenLeft() = %+v, want nothing given back", unsaved)
		}
	})
	t.Run("an answer", func(t *testing.T) {
		h := askingTeam(t, teamAsked())
		h.pressAndDeliver("enter")
		if unsaved, _ := h.model.tokenLeft(); len(unsaved) > 0 {
			t.Errorf("tokenLeft() = %+v, want nothing given back", unsaved)
		}
	})
}

// A paste or an answer that ends once the screen has gone settles its own
// making's token alone: personal's, made before, unsaved, is still given
// back, beside the other's where its save didn't save it.
func TestWhatEndsOnceTheScreenWentLeavesTheOtherMakingsHeld(t *testing.T) {
	answerNotSaved := setup.Tokened{Result: setup.TokenNotSaved, Why: "the config changed while it was being edited", Shown: []string{madeTeam}}
	t.Run("an answer saved", func(t *testing.T) {
		h := madePersonalUnsaved(t)
		addTeam(h, 2, setup.Tokened{Result: setup.TokenAsked, Asked: teamAsked(), Shown: []string{madeTeam}})
		h.press("enter")
		if unsaved, _ := h.model.tokenLeft(); !reflect.DeepEqual(unsaved, []TokenLeft{personalLeft}) {
			t.Errorf("tokenLeft() = %+v, want personal's token alone given back", unsaved)
		}
	})
	t.Run("an answer not saved", func(t *testing.T) {
		h := madePersonalUnsaved(t)
		addTeam(h, 2, setup.Tokened{Result: setup.TokenAsked, Asked: teamAsked(), Shown: []string{madeTeam}})
		h.steps.answerEnds = &answerNotSaved
		h.press("enter")
		want := []TokenLeft{personalLeft, {For: "team", Tokened: answerNotSaved}}
		if unsaved, _ := h.model.tokenLeft(); !reflect.DeepEqual(unsaved, want) {
			t.Errorf("tokenLeft() = %+v, want %+v", unsaved, want)
		}
	})
	t.Run("a paste saved", func(t *testing.T) {
		h := madePersonalUnsaved(t)
		remake(t, h, 1, "side", madeUnread)
		h.pressAndDeliver("p")
		h.typeIn("test-token-typed")
		h.press("enter")
		if unsaved, _ := h.model.tokenLeft(); !reflect.DeepEqual(unsaved, []TokenLeft{personalLeft}) {
			t.Errorf("tokenLeft() = %+v, want personal's token alone given back", unsaved)
		}
	})
}

// A making that ended showing no token to copy is given back only where its
// card is open, so one retried isn't given back once for each try.
func TestAMakingRetriedIsGivenBackOnce(t *testing.T) {
	unread := setup.Tokened{Result: setup.TokenNotSaved, Why: "setup couldn't read the token"}
	tests := []struct {
		name string
		keys []string
		want []TokenLeft
	}{
		{name: "the last card open", want: []TokenLeft{{For: "personal", Tokened: unread}}},
		{name: "the last card closed", keys: []string{"esc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}, ends: &unread}, 1)
			h.steps.nextMake(t)
			h.hearAll(3)
			h.pressAndDeliver("esc")
			remake(t, h, 0, "personal", unread)
			for _, key := range tt.keys {
				h.pressAndDeliver(key)
			}
			if unsaved, _ := h.model.tokenLeft(); !reflect.DeepEqual(unsaved, tt.want) {
				t.Errorf("tokenLeft() = %+v, want %+v", unsaved, tt.want)
			}
		})
	}
}

// A making that fails with no token made, which the screen showed, is held
// no longer: setup left gives nothing back of it, nor logs its failure again.
func TestAMakingThatFailedIsntHeld(t *testing.T) {
	h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}, err: errors.New("save the token: no space left on device")}, 1)
	h.steps.nextMake(t)
	h.hearAll(3)
	log := logstest.Capture(t)
	if unsaved, _ := h.model.tokenLeft(); len(unsaved) > 0 || len(h.model.held) > 0 {
		t.Errorf("tokenLeft() = %+v, %d makings held; want nothing", unsaved, len(h.model.held))
	}
	if strings.Contains(log.String(), "once setup's screen had gone") {
		t.Errorf("the log reads\n%s\nwant the failure, shown already, not logged again", log)
	}
}
