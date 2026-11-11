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

// Every token the screen makes is held till a save of its account's token is
// known done, whatever's made after: a second making, side's, that's
// cancelled or saved, leaves personal's token made but unsaved given back,
// and only it.
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
			h.steps.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &tt.ends}
			h.pressAndDeliver("down")
			h.pressAndDeliver("t")
			if got := h.steps.nextMake(t); got.Account.ID != "side" {
				t.Fatalf("MakeToken was asked %+v, want side's token", got)
			}
			h.hearAll(3)
			h.pressAndDeliver("esc")

			want := []setup.Tokened{madeNotSaved}
			if tt.ends.Result == setup.TokenUnread {
				want = append(want, tt.ends)
			}
			if unsaved, _ := h.model.tokenLeft(); !reflect.DeepEqual(unsaved, want) {
				t.Errorf("tokenLeft() = %+v, want %+v", unsaved, want)
			}
		})
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
