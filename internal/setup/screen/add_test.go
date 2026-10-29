package screen

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// noAccounts are the steps of a first run: no router running, and no
// account.
func noAccounts() *fakeSteps {
	steps := newFakeSteps()
	steps.standing = setup.RouterStanding{State: setup.RouterDown}
	steps.review = setup.Review{At: start}
	return steps
}

// firstRun is a harness of a first run, started.
func firstRun(t *testing.T) *harness {
	t.Helper()
	h := accountsHarness(t, noAccounts())
	h.start()
	return h
}

// The add card, as it opens: its fields empty, ID chosen, typed in.
var addCardOpened = []string{
	"",
	"  Each account is one Claude subscription: a Pro or Max plan.",
	"",
	"  ID",
	"            letters, numbers, - and _ · can't be changed later",
	"",
	"  Label                               shown in the dashboard",
	"",
	"  Plan      □ Pro    □ Max 5x    □ Max 20x    ■ not set",
	"",
	"  Setup then creates a token with claude setup-token. You approve it in",
	"  the browser while signed in to this subscription. Tokens last a year.",
	"",
	"                                  tab next   enter create token   esc cancel",
}

// checkAddCardOpened checks the card open is the add card, as it opens, the
// key line empty.
func checkAddCardOpened(t *testing.T, h *harness) {
	t.Helper()
	title, rows := h.card()
	if title != "ADD AN ACCOUNT" {
		t.Fatalf("the card's title reads %q, want ADD AN ACCOUNT", title)
	}
	if !slices.Equal(rows, addCardOpened) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(addCardOpened, "\n"))
	}
	if got := h.model.add.chosen; got != setup.IDField {
		t.Errorf("the card opens on field %d, want ID, typed in", got)
	}
	if got := h.rows()[keyRow]; got != "" {
		t.Errorf("with the card open, the key line reads %q, want it empty: the card's keys are its own", got)
	}
}

// On a first run, the add card is open at once; esc leaves the page with no
// account, saying so, + add an account chosen; and enter there opens the
// card again, never moving on, as there's nothing to set up without an
// account.
func TestAFirstRunOpensTheAddCardAndWaitsForAnAccount(t *testing.T) {
	h := firstRun(t)
	checkAddCardOpened(t, h)

	h.pressAndDeliver("esc")
	if title, _ := h.card(); title != "" {
		t.Fatalf("esc left the card %q open", title)
	}
	rows := h.rows()
	if want := "  No accounts yet. Add each Claude subscription you want switchboard to route across."; rows[firstUnderHeading] != want {
		t.Errorf("the page's line reads %q, want %q", rows[firstUnderHeading], want)
	}
	if got := rows[firstRow]; !strings.HasPrefix(got, "▌ +  add an account") {
		t.Errorf("the first row reads %q, want + add an account, chosen", got)
	}
	if got := rows[keyRow]; !strings.Contains(got, "enter add an account") {
		t.Errorf("the key line reads %q, want enter adding an account", got)
	}

	h.pressAndDeliver("enter")
	checkAddCardOpened(t, h)
	if got := h.model.page().At; got != dashboard.SetupAccounts {
		t.Errorf("enter shows %s, want Accounts, waiting for an account", got.Title())
	}
	if len(h.steps.adds) != 0 {
		t.Errorf("CheckAdding was asked %+v, want nothing asked till the card's enter", h.steps.adds)
	}
}

// On a first run, once there's an account, the page's line says what to add,
// never that the tokens were checked, as a run again's does.
func TestAFirstRunsLineOnceThereIsAnAccount(t *testing.T) {
	h := firstRun(t)
	h.pressAndDeliver("esc")
	h.steps.mu.Lock()
	h.steps.review.Accounts = []setup.Reviewed{{ID: "work", Label: "Work", Primary: true, Token: setup.TokenStanding{State: setup.TokenWorking}}}
	h.steps.mu.Unlock()
	m, cmd := h.model.askAgain()
	h.model = m
	h.deliver(h.run(cmd)...)
	if got, want := h.rows()[firstUnderHeading], "  Add each Claude subscription you want switchboard to route across."; got != want {
		t.Errorf("the page's line reads %q, want %q", got, want)
	}
	if title, _ := h.card(); title != "" {
		t.Errorf("read again, the card %q opened, want it opened only as the accounts are first read", title)
	}
}

// On a run again, enter on + add an account opens the add card, as it opens.
func TestEnterOnAddAnAccountOpensTheAddCard(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	if h.model.add != nil {
		t.Fatal("on a run again, the add card opened at once")
	}
	for range 3 {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("enter")
	checkAddCardOpened(t, h)
}

// tab goes through ID, Label and Plan, and round to ID; ← and → move Plan's
// choice no further than its first or last, and move a text field's cursor,
// never the plan.
func TestTheAddCardsTabAndArrows(t *testing.T) {
	h := firstRun(t)
	for _, want := range []setup.Field{setup.LabelField, setup.PlanField, setup.IDField} {
		h.pressAndDeliver("tab")
		if got := h.model.add.chosen; got != want {
			t.Fatalf("tab chose field %d, want %d", got, want)
		}
	}
	h.typeIn("wrk")
	h.pressAndDeliver("left")
	h.pressAndDeliver("left")
	h.typeIn("o")
	if got := h.model.add.id.value(); got != "work" {
		t.Errorf("← in ID left it reading %q, want the cursor moved, so it reads work", got)
	}
	plan := func() string { return config.PlanChoices()[h.model.add.plan] }
	if got := plan(); got != config.PlanNone {
		t.Errorf("← in ID moved the plan to %q", got)
	}
	h.pressAndDeliver("tab")
	h.pressAndDeliver("tab")
	for _, tt := range []struct {
		key, want string
	}{{"right", config.PlanNone}, {"left", "max20x"}, {"left", "max5x"}, {"left", "pro"}, {"left", "pro"}, {"right", "max5x"}} {
		h.pressAndDeliver(tt.key)
		if got := plan(); got != tt.want {
			t.Fatalf("%s chose %q, want %q", tt.key, got, tt.want)
		}
	}
	if got := h.cardRow("Plan"); got != "  Plan      □ Pro    ■ Max 5x    □ Max 20x    □ not set" {
		t.Errorf("Plan reads %q, want Max 5x chosen", got)
	}
}

// enter with every field passing hands the account, as setup checked it, on
// to have its token made: till the token card takes it, the card holds it,
// waiting, and stays open, adding nothing; esc closes the card, the account
// dropped.
func TestEnterHandsTheAccountOnToHaveItsTokenMade(t *testing.T) {
	h := firstRun(t)
	h.typeIn("work")
	h.pressAndDeliver("tab")
	h.typeIn("Work")
	h.pressAndDeliver("tab")
	h.pressAndDeliver("left")
	h.pressAndDeliver("enter")
	if want := []setup.Adding{{ID: "work", Label: "Work", Plan: "max20x"}}; !slices.Equal(h.steps.adds, want) {
		t.Fatalf("CheckAdding was asked %+v, want %+v", h.steps.adds, want)
	}
	if card := h.model.add; card == nil || card.checking || card.waiting == nil || *card.waiting != (config.NewAccount{ID: "work", Label: "Work", Plan: "max20x"}) {
		t.Fatalf("the add card is %+v, want it open, holding work, waiting for its token", card)
	}
	if title, _ := h.card(); title != "ADD AN ACCOUNT" {
		t.Errorf("the card open reads %q, want the add card kept open", title)
	}
	if h.model.status.review == nil || len(h.model.status.review.Accounts) != 0 || len(h.steps.edits) != 0 {
		t.Errorf("an account was added: the review is %+v", h.model.status.review)
	}
	h.pressAndDeliver("esc")
	if h.model.add != nil {
		t.Error("esc left the add card open")
	}
}

// The card's keys alone act while it's open: q, j and k type into a text
// field; the page's keys and the wheel do nothing; ctrl+c leaves; and a
// paste goes into the field chosen.
func TestTheAddCardTakesEveryKey(t *testing.T) {
	h := firstRun(t)
	for _, k := range []string{"q", "j", "k"} {
		if quits(h.press(k)) {
			t.Fatalf("%s quit with the add card open", k)
		}
	}
	h.deliver(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	h.deliver(tea.PasteMsg{Content: "-spare"})
	if got := h.model.add.id.value(); got != "qjk-spare" {
		t.Errorf("ID reads %q, want q, j and k typed, then the paste", got)
	}
	if h.model.scroll != 0 || h.model.chosen != 0 {
		t.Errorf("with the card open, the page moved: row %d chosen, scrolled %d", h.model.chosen, h.model.scroll)
	}
	if !quits(h.press("ctrl+c")) {
		t.Error("ctrl+c didn't quit with the add card open")
	}
}

// addingTo is a harness of setup's own steps, of the config given, its add
// card open on + add an account, after its accounts.
func addingTo(t *testing.T, cfg string, accounts int) (*harness, *registered) {
	t.Helper()
	steps := newRegistered(t, cfg)
	h := registeredHarness(t, steps)
	for range accounts {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("enter")
	if h.model.add == nil {
		t.Fatal("enter on + add an account opened no add card")
	}
	return h, steps
}

// A value config refuses, as accounts add would, is said under its field,
// why alone, the card staying open and nothing written; and unsaid once the
// field's value changes.
func TestTheAddCardsRefusals(t *testing.T) {
	const two = "[[account]]\nid = \"work\"\nprimary = true\n\n[[account]]\nid = \"personal\"\n"
	tests := []struct {
		name      string
		id, label string
		field     setup.Field
		why       string
	}{
		{name: "an id that isn't a plain file name", id: "my side", field: setup.IDField, why: "must start with a letter or digit and contain only letters, digits, '-' and '_'"},
		{name: "an id an account has already", id: "personal", field: setup.IDField, why: "is already configured"},
		{name: "an id an account has in another case", id: "Personal", field: setup.IDField, why: `differs from "personal" only in case, so they'd share a token file, as macOS ignores case in file names`},
		{name: "no id", id: "", field: setup.IDField, why: "is required"},
		{name: "a label of blanks", id: "side", label: "   ", field: setup.LabelField, why: "is blank, showing nothing"},
		{name: "a label hiding the character after a sign", id: "side", label: "\u0600Side", field: setup.LabelField, why: "holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character"},
	}
	names := map[setup.Field]string{setup.IDField: "ID", setup.LabelField: "Label"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, steps := addingTo(t, two, 2)
			h.deliver(tea.PasteMsg{Content: tt.id})
			h.pressAndDeliver("tab")
			h.deliver(tea.PasteMsg{Content: tt.label})
			h.pressAndDeliver("enter")
			if steps.config(t) != two {
				t.Errorf("the config reads\n%s\nwant it as it was", steps.config(t))
			}
			_, rows := h.card()
			name := "  " + names[tt.field]
			at := slices.IndexFunc(rows, func(row string) bool { return row == name || strings.HasPrefix(row, name+" ") })
			if at < 0 || at+1 >= len(rows) || !strings.HasPrefix(rows[at+1], "            ") || !strings.HasPrefix(tt.why, strings.TrimSpace(rows[at+1])) {
				t.Fatalf("the card reads\n%s\nwant why under %s: %q", strings.Join(rows, "\n"), names[tt.field], tt.why)
			}
			if card := h.model.add; card.refused != tt.field || card.why != tt.why || card.waiting != nil {
				t.Errorf("the card says %q of field %d, holding %v, want it saying %q, holding nothing", card.why, card.refused, card.waiting, tt.why)
			}

			for h.model.add.chosen != tt.field {
				h.pressAndDeliver("tab")
			}
			h.typeIn("x")
			if why := h.model.add.why; why != "" {
				t.Errorf("the field changed, the card still says %q, want it unsaid", why)
			}
		})
	}
}

// A failure of the check that's no refusal of a value fails the step, not
// checked, as nothing was being saved, the card closed.
func TestTheAddCardsCheckFailingFailsTheStep(t *testing.T) {
	h := firstRun(t)
	h.steps.addErr = errors.New("read config: the config doesn't parse")
	h.typeIn("work")
	h.pressAndDeliver("enter")
	if h.model.add != nil {
		t.Error("the check failed, and the add card is still open")
	}
	if h.model.walk.failed == nil || h.model.accountsFailed != dashboard.AccountsNotChecked {
		t.Errorf("the walk is %+v, failed at %d; want the step failed, not checked", h.model.walk, h.model.accountsFailed)
	}
	if got := h.rows()[firstUnderHeading]; !strings.Contains(got, "Not checked") || !strings.Contains(got, "the config doesn't parse") {
		t.Errorf("the page reads %q, want the step's row not checked, and why", got)
	}
}

// work checked and waiting, a later check refused, or any field changed,
// drops it, so the account waiting is only ever the one the card shows.
func TestTheAccountWaitingIsTheOneTheCardShows(t *testing.T) {
	waitingWork := func(t *testing.T) *harness {
		t.Helper()
		h := firstRun(t)
		h.typeIn("work")
		h.pressAndDeliver("enter")
		if h.model.add.waiting == nil {
			t.Fatal("work checked, nothing waits")
		}
		return h
	}
	t.Run("a later check refused", func(t *testing.T) {
		h, steps := addingTo(t, "[[account]]\nid = \"work\"\n", 1)
		h.typeIn("side")
		h.pressAndDeliver("enter")
		if h.model.add.waiting == nil {
			t.Fatal("side checked, nothing waits")
		}
		if err := os.WriteFile(steps.setup.Accounts.ConfigPath, []byte("[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		h.pressAndDeliver("enter")
		if card := h.model.add; card.waiting != nil || card.why != "is already configured" {
			t.Errorf("refused, the card holds %v, saying %q; want nothing waiting, and why", card.waiting, card.why)
		}
	})
	for _, keys := range [][]string{{"x"}, {"backspace"}, {"tab", "x"}, {"tab", "tab", "left"}} {
		t.Run("changed by "+strings.Join(keys, " "), func(t *testing.T) {
			h := waitingWork(t)
			for _, k := range keys {
				h.pressAndDeliver(k)
			}
			if h.model.add.waiting != nil {
				t.Errorf("the card changed, and holds %v waiting", h.model.add.waiting)
			}
		})
	}
	t.Run("changed by a paste", func(t *testing.T) {
		h := waitingWork(t)
		h.deliver(tea.PasteMsg{Content: "-2"})
		if h.model.add.waiting != nil {
			t.Errorf("the card changed, and holds %v waiting", h.model.add.waiting)
		}
	})
	t.Run("not changed by tab, a key a field takes that changes nothing, or one no field takes", func(t *testing.T) {
		h := waitingWork(t)
		// left moves ID's cursor; right on Plan, at its last choice, stays.
		for _, k := range []string{"left", "tab", "tab", "right", "pgdown"} {
			h.pressAndDeliver(k)
		}
		if h.model.add.waiting == nil {
			t.Error("nothing changed, and work no longer waits")
		}
	})
}

// While the card's account is being checked, the card takes no key: keys,
// esc and a paste change nothing, the card open; once the check answers, it
// takes them again.
func TestTheAddCardTakesNoKeyWhileItsChecked(t *testing.T) {
	h := firstRun(t)
	h.typeIn("work")
	answered := h.press("enter")
	if !h.model.add.checking {
		t.Fatal("enter, and the card isn't being checked")
	}
	for _, k := range []string{"x", "tab", "backspace", "esc", "enter"} {
		h.deliver(h.press(k)...)
	}
	h.deliver(tea.PasteMsg{Content: "-2"})
	if card := h.model.add; card == nil || card.account().ID != "work" || card.chosen != setup.IDField {
		t.Fatalf("while it's checked, the card is %+v, want it open as it was", card)
	}
	if len(h.steps.adds) != 1 {
		t.Errorf("CheckAdding was asked %+v, want once", h.steps.adds)
	}
	h.deliver(answered...)
	h.pressAndDeliver("x")
	if card := h.model.add; card.checking || card.account().ID != "workx" {
		t.Errorf("checked, the card is %+v, want it taking keys again", card)
	}
}

// What CheckAdding answers once the card's gone is dropped: nothing opens,
// fails or waits.
func TestACheckAnsweredWithTheCardGoneIsDropped(t *testing.T) {
	h := firstRun(t)
	h.pressAndDeliver("esc")
	for _, msg := range []addCheckedMsg{{account: config.NewAccount{ID: "work"}}, {err: errors.New("read config: the config doesn't parse")}} {
		h.deliver(msg)
		if h.model.add != nil || h.model.walk.failed != nil {
			t.Errorf("answered %+v with the card gone, the card is %+v and the walk %+v; want it dropped", msg, h.model.add, h.model.walk)
		}
	}
}
