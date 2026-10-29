package screen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/tokens"
)

// registered are steps whose accounts are setup's own, in a config file and
// a state directory of the test's: Review, ReviewOf, CheckAdding,
// EditAccount, RemoveAccount, MakeToken and Reopen answer as setup does,
// through the registry, and every other step as fakeSteps does. No account
// has a token, so none is checked, unless a test gives it one, and the
// registry what checks it.
type registered struct {
	*fakeSteps
	setup *setup.Setup
}

// newRegistered is registered steps of the config given.
func newRegistered(t *testing.T, config string) *registered {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := accounts.Registry{
		ConfigPath: path,
		Tokens:     tokens.NewStore(filepath.Join(dir, "state"), os.Getuid()),
		Now:        func() time.Time { return start },
	}
	// The process's own interrupts are left alone: no test sends one.
	hold := func(chan<- os.Signal) func() { return func() {} }
	return &registered{fakeSteps: newFakeSteps(), setup: &setup.Setup{Accounts: registry, Hold: hold}}
}

func (s *registered) Review() (setup.Review, error) {
	return s.setup.Review()
}

func (s *registered) CheckAdding(adding setup.Adding) (config.NewAccount, error) {
	return s.setup.CheckAdding(adding)
}

func (s *registered) EditAccount(id string, edit setup.Edit) (accounts.Edited, error) {
	return s.setup.EditAccount(id, edit)
}

func (s *registered) RemoveAccount(id string) (accounts.Removed, error) {
	return s.setup.RemoveAccount(id)
}

func (s *registered) ReviewOf(ctx context.Context, id string) (setup.Reviewed, error) {
	return s.setup.ReviewOf(ctx, id)
}

func (s *registered) MakeToken(ctx context.Context, f setup.TokenFor, report func(setup.Making)) (setup.Tokened, error) {
	return s.setup.MakeToken(ctx, f, report)
}

func (s *registered) Reopen(ctx context.Context, at setup.Making) error {
	return s.setup.Reopen(ctx, at)
}

// config is what the config file reads.
func (s *registered) config(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(s.setup.Accounts.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// registeredHarness is a harness opening on Accounts, its steps the
// registered steps given, started.
func registeredHarness(t *testing.T, steps *registered) *harness {
	t.Helper()
	h := newHarness(t, dashboard.SetupAccounts)
	h.steps = steps.fakeSteps
	cfg := h.config(dashboard.SetupAccounts)
	cfg.Steps = steps
	h.model = New(t.Context(), cfg)
	t.Cleanup(h.model.jobs.wait)
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	h.start()
	return h
}

// threeAccounts is a config of three accounts, work the primary, marked.
const threeAccounts = "[[account]]\nid = \"work\"\nprimary = true\n\n[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"side\"\n"

// The primary removed, two left with none marked, p on the account chosen,
// the first, marks it in the config, and enter moves on: through setup's
// own steps, as the first, the primary only for want of a mark, must be
// marked so.
func TestRemovingThePrimaryThenPOnTheFirstMovesOn(t *testing.T) {
	steps := newRegistered(t, threeAccounts)
	h := registeredHarness(t, steps)
	h.pressAndDeliver("r")
	h.pressAndDeliver("enter")
	if review := h.model.status.review; review == nil || !review.Unmarked || h.model.chosen != 0 {
		t.Fatalf("work removed, the review is %+v, row %d chosen; want two unmarked, personal chosen", review, h.model.chosen)
	}

	h.pressAndDeliver("p")
	if want := "[[account]]\nid = \"personal\"\nprimary = true\n\n[[account]]\nid = \"side\"\n"; steps.config(t) != want {
		t.Errorf("the config reads\n%s\nwant personal marked the primary\n%s", steps.config(t), want)
	}
	if review := h.model.status.review; review.Unmarked || !review.Accounts[0].Primary {
		t.Errorf("the review is %+v, want personal the primary", review)
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().At; got != dashboard.SetupPriming {
		t.Errorf("enter shows %s, want Priming", got.Title())
	}
}

// Once a change is made, the account keys and enter wait till the accounts
// are read again, so none acts on a row the change has taken out.
func TestTheAccountKeysWaitTillAChangeIsReadAgain(t *testing.T) {
	h := removing(t, frame3Steps(), 2)
	var readAgain []tea.Msg
	for _, msg := range h.press("enter") {
		readAgain = append(readAgain, h.update(msg)...)
	}
	if len(readAgain) == 0 {
		t.Fatal("the removal asked for nothing to read the accounts again")
	}
	for _, key := range []string{"e", "r", "p", "enter"} {
		h.pressAndDeliver(key)
	}
	if h.model.edit != nil || h.model.remove != nil || len(h.steps.edits) != 0 || len(h.steps.removes) != 1 {
		t.Errorf("before the accounts are read again, a key acted on side: edits %+v, removes %v, cards %v %v",
			h.steps.edits, h.steps.removes, h.model.edit, h.model.remove)
	}
	if got := h.model.page().At; got != dashboard.SetupAccounts {
		t.Errorf("enter, before the accounts are read again, shows %s, want Accounts", got.Title())
	}

	h.deliver(readAgain...)
	h.pressAndDeliver("e")
	if title, _ := h.card(); title != "EDIT PERSONAL" {
		t.Errorf("e, once the accounts are read again, opens %q, want personal's edit card", title)
	}
}

// A removal that takes the account out of the config but can't delete its
// token file fails the step, saying so, and the accounts are read again,
// without it.
func TestARemovalThatLeavesItsTokenFileSaysSo(t *testing.T) {
	steps := newRegistered(t, threeAccounts)
	file := steps.setup.Accounts.Tokens.Path("side")
	if err := os.MkdirAll(filepath.Join(file, "kept"), 0o700); err != nil {
		t.Fatal(err)
	}
	h := registeredHarness(t, steps)
	h.pressAndDeliver("down")
	h.pressAndDeliver("down")
	h.pressAndDeliver("r")
	h.pressAndDeliver("enter")

	left := h.model.walk.left()
	if _, ok := errors.AsType[*accounts.TokenFileLeft](left.Err); left.Ending != Failed || !ok {
		t.Fatalf("setup leaves %+v, want it failed as the removal did", left)
	}
	want := "side is taken out of the config, but its token file, " + file + ", isn't deleted: directory not empty"
	if got := h.model.summary().Failure; got != want {
		t.Errorf("the failure reads %q, want %q", got, want)
	}
	if got := h.rows()[firstUnderHeading]; !strings.HasPrefix(got, factRowOf("✗", setup.Said{Label: "Not deleted", Detail: "side is taken out"})) {
		t.Errorf("the page's row reads %q, want it saying the token file isn't deleted", got)
	}
	if review := h.model.status.review; review == nil || len(review.Accounts) != 2 {
		t.Errorf("the accounts read again are %+v, want work and personal alone", review)
	}
}
