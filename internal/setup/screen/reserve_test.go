package screen

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setuptoken"
	"github.com/leeovery/switchboard/internal/tokens"
)

// threeAdded are the accounts once a first run has added them: work, then
// personal and side, none marked the primary, as setup adds them, and none
// with a reserve, each token working.
func threeAdded() []setup.Reviewed {
	working := setup.TokenStanding{State: setup.TokenWorking}
	return []setup.Reviewed{
		{ID: "work", Label: "Work", Token: working},
		{ID: "personal", Label: "Personal", Token: working},
		{ID: "side", Label: "Side", Token: working},
	}
}

// workMarked are the accounts threeAdded has, work marked the primary, as p
// marks it.
func workMarked() []setup.Reviewed {
	added := threeAdded()
	added[0].Primary = true
	return added
}

// firstRunAdded is a first run, its accounts added as given, each marked the
// primary as given, read again, work's row chosen.
func firstRunAdded(t *testing.T, added []setup.Reviewed) *harness {
	t.Helper()
	h := firstRun(t)
	h.pressAndDeliver("esc")
	h.steps.mu.Lock()
	h.steps.review.Accounts = added
	h.steps.mu.Unlock()
	m, cmd := h.model.askAgain()
	h.model = m
	h.deliver(h.run(cmd)...)
	for range added {
		h.pressAndDeliver("up")
	}
	return h
}

// reserving is a first run, its three accounts added, none the primary, then
// p and enter pressed on work's row, as a user must, the primary's reserve
// card open; the edits EditAccount is given counted from there, p's mark
// put by.
func reserving(t *testing.T) *harness {
	t.Helper()
	h := firstRunAdded(t, threeAdded())
	h.pressAndDeliver("p")
	h.pressAndDeliver("enter")
	if title, _ := h.card(); title != "RESERVE · WORK" {
		t.Fatalf("p and enter on a first run opened the card %q, want RESERVE · WORK", title)
	}
	h.steps.mu.Lock()
	defer h.steps.mu.Unlock()
	if want := (fakeEdit{id: "work", edit: setup.Edit{Primary: true}}); len(h.steps.edits) != 1 || h.steps.edits[0].id != want.id || !sameEdit(h.steps.edits[0].edit, want.edit) {
		t.Fatalf("p gave EditAccount %+v, want work marked the primary", h.steps.edits)
	}
	h.steps.edits = nil
	return h
}

// The card as it opens: what the primary is, and its reserve; its field
// empty, typed in; what it takes; where other reserves are set; and its
// keys. The key line is empty, and the step still Accounts.
func TestOnAFirstRunEnterOpensThePrimarysReserveCard(t *testing.T) {
	h := reserving(t)
	_, rows := h.card()
	want := []string{
		"",
		"  work is the primary, the account the Claude apps are signed in to.",
		"  Its reserve is the share of each of its windows kept back for them.",
		"",
		"  Reserve                             usage kept back for the Claude apps",
		"            a percentage, or none",
		"",
		"  Other accounts' reserves are set with e, or switchboard accounts edit.",
		"",
		"                                                 enter save   esc no reserve",
	}
	if !slices.Equal(rows, want) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	if !h.model.page().Accounts.Reserve.Reserve.Typing {
		t.Error("the card's field is handed over not typed in, want it typed in")
	}
	if got := h.rows()[keyRow]; got != "" {
		t.Errorf("with the card open, the key line reads %q, want it empty", got)
	}
	if got := h.model.page().At; got != dashboard.SetupAccounts {
		t.Errorf("with the card open, the screen shows %s, want Accounts", got.Title())
	}
	if len(h.steps.edits) != 0 {
		t.Errorf("EditAccount was given %+v as the card opened, want nothing", h.steps.edits)
	}
}

// p marks another account the primary, and enter, pressed at once, waits
// for the mark: then the card opens on the new primary, and its reserve is
// saved to it, never to the one that was.
func TestTheReserveCardFollowsPToTheNewPrimary(t *testing.T) {
	h := firstRunAdded(t, workMarked())
	h.pressAndDeliver("down")
	marked := h.press("p")
	h.pressAndDeliver("enter")
	if title, _ := h.card(); title != "" {
		t.Fatalf("enter, while p's mark is written, opened the card %q, want it waiting", title)
	}
	if got := h.model.page().At; got != dashboard.SetupAccounts {
		t.Fatalf("enter, while p's mark is written, shows %s, want Accounts, waiting", got.Title())
	}
	h.deliver(marked...)
	h.pressAndDeliver("enter")
	if title, _ := h.card(); title != "RESERVE · PERSONAL" {
		t.Fatalf("enter, once p's mark is read, opened the card %q, want RESERVE · PERSONAL", title)
	}
	h.typeIn("5%")
	h.pressAndDeliver("enter")
	want := []fakeEdit{{id: "personal", edit: setup.Edit{Primary: true}}, {id: "personal", edit: setup.Edit{Reserve: new("5%")}}}
	if got := h.steps.edits; len(got) != len(want) || got[0].id != want[0].id || !sameEdit(got[0].edit, want[0].edit) || got[1].id != want[1].id || !sameEdit(got[1].edit, want[1].edit) {
		t.Fatalf("EditAccount was given %+v, want personal marked, then its reserve, 5%%", got)
	}
	if got := h.model.summary().Review.Accounts; got[0].Reserve != 0 || got[1].Reserve != 0.05 {
		t.Errorf("the reserves read work %v and personal %v, want personal's alone 5%%", got[0].Reserve, got[1].Reserve)
	}
}

// The card doesn't open on a run again, where the primary's reserve is
// set, or with one account, the primary, where its reserve is set: enter
// moves on. With two or more and none the primary, enter waits, opening
// nothing.
func TestThePrimarysReserveCardOpensOnlyWhereItsAsked(t *testing.T) {
	reserved := workMarked()
	reserved[0].Reserve = 0.05
	tests := []struct {
		name  string
		start func(t *testing.T) *harness
		moves bool
	}{
		{name: "a run again", start: func(t *testing.T) *harness {
			h := accountsHarness(t, newFakeSteps())
			h.start()
			return h
		}, moves: true},
		{name: "its reserve set", start: func(t *testing.T) *harness { return firstRunAdded(t, reserved) }, moves: true},
		{name: "one account, its reserve set", start: func(t *testing.T) *harness { return firstRunAdded(t, reserved[:1]) }, moves: true},
		{name: "none the primary", start: func(t *testing.T) *harness { return firstRunAdded(t, threeAdded()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.start(t)
			h.pressAndDeliver("enter")
			if title, _ := h.card(); title != "" {
				t.Errorf("enter opened the card %q, want none", title)
			}
			want := dashboard.SetupAccounts
			if tt.moves {
				want = dashboard.SetupPriming
			}
			if got := h.model.page().At; got != want {
				t.Errorf("enter shows %s, want %s", got.Title(), want.Title())
			}
		})
	}
}

// With one account, unmarked, it's the primary for want of a mark, as the
// config and the status read the only account, so its reserve card opens.
func TestWithOneAccountItsReserveIsAsked(t *testing.T) {
	h := firstRunAdded(t, threeAdded()[:1])
	h.pressAndDeliver("enter")
	if title, _ := h.card(); title != "RESERVE · WORK" {
		t.Errorf("enter opened the card %q, want RESERVE · WORK", title)
	}
}

// enter saves the reserve typed, as the edit card's Reserve saves it, and
// moves on to Priming, the accounts read again; none, in any case, or the
// field left empty, saves nothing, and moves on.
func TestTheReserveCardsEnter(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		want  []fakeEdit
	}{
		{name: "a percentage", typed: "5%", want: []fakeEdit{{id: "work", edit: setup.Edit{Reserve: new("5%")}}}},
		{name: "empty", typed: ""},
		{name: "none", typed: "none"},
		{name: "None", typed: "None"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := reserving(t)
			h.typeIn(tt.typed)
			h.pressAndDeliver("enter")
			if len(h.steps.edits) != len(tt.want) || len(tt.want) == 1 && (h.steps.edits[0].id != "work" || !sameEdit(h.steps.edits[0].edit, tt.want[0].edit)) {
				t.Fatalf("EditAccount was given %+v, want %+v", h.steps.edits, tt.want)
			}
			if title, _ := h.card(); title != "" {
				t.Errorf("enter left the card %q open", title)
			}
			if got := h.model.page().At; got != dashboard.SetupPriming {
				t.Errorf("enter shows %s, want Priming", got.Title())
			}
			reserve := 0.0
			if tt.want != nil {
				reserve = 0.05
			}
			if got := h.model.summary().Review.Accounts[0].Reserve; got != reserve {
				t.Errorf("Done reads work's reserve as %v, want %v, as saved", got, reserve)
			}
		})
	}
}

// Done's Accounts line says the reserve saved.
func TestDoneSaysTheReserveSaved(t *testing.T) {
	h := reserving(t)
	h.typeIn("5%")
	h.pressAndDeliver("enter")
	lines := h.model.summary().Lines()
	if len(lines) == 0 || !strings.Contains(lines[0].Detail, "primary: work, with 5% reserved for the Claude apps") {
		t.Errorf("Done's Accounts line reads %+v, want the primary's reserve, 5%%", lines)
	}
}

// A reserve config refuses is said under the field, why alone, the card
// staying open; once the value changes, it's unsaid.
func TestTheReserveCardSaysARefusal(t *testing.T) {
	h := reserving(t)
	const why = "must be written as a percentage, such as 5%"
	h.steps.editErr = &setup.Refused{Field: setup.ReserveField, Why: why}
	h.typeIn("5")
	h.pressAndDeliver("enter")
	_, rows := h.card()
	i := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "  Reserve   5") })
	if i < 0 || i+1 >= len(rows) || rows[i+1] != "            "+why {
		t.Fatalf("the card reads\n%s\nwant %q under the field", strings.Join(rows, "\n"), why)
	}
	if h.model.reserve == nil || h.model.walk.failed != nil || h.model.page().At != dashboard.SetupAccounts {
		t.Fatal("on a refusal, the card closed, the step failed, or it moved on: want the card kept")
	}
	h.pressAndDeliver("%")
	if _, rows := h.card(); slices.Contains(rows, "            "+why) {
		t.Error("once the field's changed, the card still says why it was refused")
	}
}

// A save that fails otherwise fails the step, as the edit card's does: its
// row Not saved, the card closed.
func TestAFailureSavingTheReserveFailsTheStep(t *testing.T) {
	h := reserving(t)
	h.steps.editErr = errNoConfig
	h.typeIn("5%")
	h.pressAndDeliver("enter")
	if h.model.reserve != nil {
		t.Error("the card's still open, want it closed: the step failed")
	}
	if h.model.walk.failed == nil || h.model.accountsFailed != dashboard.AccountsNotSaved {
		t.Errorf("the step failed with %v at %v, want it failed, not saved", h.model.walk.failed, h.model.accountsFailed)
	}
	if got := h.model.page().At; got != dashboard.SetupAccounts {
		t.Errorf("the screen shows %s, want Accounts, where it failed", got.Title())
	}
}

// esc closes the card, writing nothing, and moves on; and once closed, the
// card never opens again.
func TestEscClosesTheReserveCardAndItNeverOpensAgain(t *testing.T) {
	h := reserving(t)
	h.typeIn("5%")
	h.pressAndDeliver("esc")
	if len(h.steps.edits) != 0 {
		t.Errorf("esc gave EditAccount %+v, want nothing written", h.steps.edits)
	}
	if title, _ := h.card(); title != "" {
		t.Errorf("esc left the card %q open", title)
	}
	if got := h.model.page().At; got != dashboard.SetupPriming {
		t.Errorf("esc shows %s, want Priming", got.Title())
	}
	if !h.model.reserveAsked || h.model.askingReserve() != nil {
		t.Error("once closed, the card would open again")
	}
}

// While the card is open, its keys alone act: q types, and ctrl+c leaves.
func TestWhileTheReserveCardIsOpenItsKeysAloneAct(t *testing.T) {
	h := reserving(t)
	if quits(h.press("q")) {
		t.Error("q left setup with the card open, want it typed")
	}
	if got := h.model.reserve.reserve.value(); got != "q" {
		t.Errorf("the field holds %q, want q typed", got)
	}
	if !quits(h.press("ctrl+c")) {
		t.Error("ctrl+c didn't leave with the card open")
	}
}

// A paste goes into the card's field.
func TestAPasteGoesIntoTheReserveCard(t *testing.T) {
	h := reserving(t)
	h.model = h.model.pasted("5%")
	if got := h.model.reserve.reserve.value(); got != "5%" {
		t.Errorf("the field holds %q, want 5%% pasted", got)
	}
}

// The edit card's Reserve shares the rule: cleared, on an account with
// none, it's none as it was, and saves nothing.
func TestTheEditCardsReserveClearedOfNoneSavesNothing(t *testing.T) {
	_, edits := edited(t, "tab", "tab", "tab", "backspace", "backspace", "backspace", "backspace")
	if len(edits) != 0 {
		t.Errorf("EditAccount was given %+v, want nothing: the reserve was none, and is", edits)
	}
}

// madeInTurn is setup-token, as setup's TokenMaker, making each of made in
// turn, as madeAs makes it.
type madeInTurn struct {
	mu   sync.Mutex
	made []madeAs
}

func (m *madeInTurn) Make(ctx context.Context, s setuptoken.Signing) (tokens.Token, error) {
	m.mu.Lock()
	next := m.made[0]
	m.made = m.made[1:]
	m.mu.Unlock()
	return next.Make(ctx, s)
}

// On a first run through setup's own steps, setup-token faked, work, personal
// and side added, each token made, none is marked the primary, as config
// adds them: enter waits, the note saying one must be. p on work marks it,
// and enter then opens its reserve card.
func TestAFirstRunsAccountsWaitForPThroughSetup(t *testing.T) {
	steps := newRegistered(t, "")
	s := steps.setup
	s.Accounts.Prober = func(string) accounts.Prober { return tokenAPI{} }
	s.TokenMaker = &madeInTurn{made: []madeAs{"test-token-work", "test-token-personal", "test-token-side"}}
	s.Browser, s.PrivateWindow = privateWindow{}, privateWindow{}
	s.Interrupt = func(ctx context.Context) (context.Context, context.CancelFunc) { return context.WithCancel(ctx) }
	h := registeredHarness(t, steps)
	for i, id := range []string{"work", "personal", "side"} {
		if i > 0 {
			h.pressAndDeliver("down")
			h.pressAndDeliver("enter")
		}
		h.typeIn(id)
		h.pressAndDeliver("enter")
		h.hearAll(5)
	}
	if review := h.model.status.review; review == nil || len(review.Accounts) != 3 || !review.Unmarked {
		t.Fatalf("the three added, the review is %+v, want them all, none the primary", review)
	}

	h.pressAndDeliver("enter")
	if title, _ := h.card(); title != "" || h.model.page().At != dashboard.SetupAccounts {
		t.Fatalf("with none the primary, enter opened the card %q, showing %s; want it waiting at Accounts", title, h.model.page().At.Title())
	}
	if rows := strings.Join(h.rows(), "\n"); !strings.Contains(rows, "No account is the primary") {
		t.Errorf("the page reads\n%s\nwant the note that none is the primary", rows)
	}
	h.pressAndDeliver("up")
	h.pressAndDeliver("up")
	h.pressAndDeliver("p")
	h.pressAndDeliver("enter")
	if title, _ := h.card(); title != "RESERVE · WORK" {
		t.Errorf("p and enter on work opened the card %q, want RESERVE · WORK", title)
	}
}
