package screen

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
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

// choosingUltra chooses Ultra on the add card open, Label chosen, as it
// opens: tab to Plan, then ← from none chosen to the last.
func choosingUltra(h *harness) {
	h.pressAndDeliver("tab")
	h.pressAndDeliver("left")
}

// firstRun is a harness of a first run, started.
func firstRun(t *testing.T) *harness {
	t.Helper()
	h := accountsHarness(t, noAccounts())
	h.start()
	return h
}

// The add card, as it opens: its fields empty, no plan chosen, as it's the
// user's to say, Label chosen, typed in; its plans a subscription's alone,
// with no not set, as every account setup adds is on one. It has no ID, as the token's
// organization decides the id, and no Pool in a config without pools.
var addCardOpened = []string{
	"",
	"  Each account is one Claude subscription: a Pro, Max or Ultra plan.",
	"",
	"  Label                               what every screen calls it",
	"",
	"  Plan      □ Pro    □ Max    □ Ultra",
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
	if got := h.model.add.chosen; got != setup.LabelField {
		t.Errorf("the card opens on field %d, want Label, typed in", got)
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
	if got := rows[firstRow]; !strings.HasPrefix(got, "▌ + add an account") {
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

// tab goes through Label and Plan, and round to Label; ← and → move Plan's
// choice no further than its first or last, from none chosen ← to the last
// and → to the first, and move a text field's cursor, never the plan.
func TestTheAddCardsTabAndArrows(t *testing.T) {
	h := firstRun(t)
	for _, want := range []setup.Field{setup.PlanField, setup.LabelField} {
		h.pressAndDeliver("tab")
		if got := h.model.add.chosen; got != want {
			t.Fatalf("tab chose field %d, want %d", got, want)
		}
	}
	h.typeIn("Wrk")
	h.pressAndDeliver("left")
	h.pressAndDeliver("left")
	h.typeIn("o")
	if got := h.model.add.label.value(); got != "Work" {
		t.Errorf("← in Label left it reading %q, want the cursor moved, so it reads Work", got)
	}
	plan := func() string { return h.model.add.planChosen() }
	if got := plan(); got != "" {
		t.Errorf("← in Label moved the plan to %q", got)
	}
	h.pressAndDeliver("tab")
	for _, tt := range []struct {
		key, want string
	}{{"left", "ultra"}, {"right", "ultra"}, {"left", "max"}, {"left", "pro"}, {"left", "pro"}, {"right", "max"}} {
		h.pressAndDeliver(tt.key)
		if got := plan(); got != tt.want {
			t.Fatalf("%s chose %q, want %q", tt.key, got, tt.want)
		}
	}
	if got := h.cardRow("Plan"); got != "  Plan      □ Pro    ■ Max    □ Ultra" {
		t.Errorf("Plan reads %q, want Max chosen", got)
	}
	h.model.add.plan = noPlan
	if h.pressAndDeliver("right"); plan() != "pro" {
		t.Errorf("→ from none chosen chose %q, want pro", plan())
	}
}

// enter with no plan chosen is refused, as setup refuses it, saying so under
// Plan, the card staying open and no token made; a plan chosen unsays it,
// and enter goes on.
func TestEnterWithNoPlanChosenIsRefusedUnderPlan(t *testing.T) {
	h := firstRun(t)
	h.typeIn("Work")
	h.pressAndDeliver("enter")
	_, rows := h.card()
	at := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "  Plan ") })
	if at < 0 || at+1 >= len(rows) || strings.TrimSpace(rows[at+1]) != setup.PlanUnchosen || h.model.token != nil {
		t.Fatalf("the card reads\n%s\nwant %q under Plan, no token card", strings.Join(rows, "\n"), setup.PlanUnchosen)
	}
	choosingUltra(h)
	if strings.Contains(strings.Join(cardRows(h), "\n"), setup.PlanUnchosen) {
		t.Errorf("a plan chosen, the card still says %q", setup.PlanUnchosen)
	}
	h.pressAndDeliver("enter")
	if got := h.steps.nextMake(t); got.Account.Plan != "ultra" {
		t.Errorf("MakeToken was asked %+v, want Work on Ultra", got)
	}
}

// enter with every field passing hands the account, as setup checked it, on
// to have its token made: the add card gives way to the token card, which
// makes the token to add the account with, compared with the accounts read,
// adding nothing till it's saved.
func TestEnterHandsTheAccountOnToHaveItsTokenMade(t *testing.T) {
	h := firstRun(t)
	h.typeIn("Work")
	h.pressAndDeliver("tab")
	h.pressAndDeliver("left")
	h.pressAndDeliver("enter")
	if want := []setup.Adding{{Label: "Work", Plan: "ultra"}}; !reflect.DeepEqual(h.steps.adds, want) {
		t.Fatalf("CheckAdding was asked %+v, want %+v", h.steps.adds, want)
	}
	if h.model.add != nil {
		t.Errorf("the add card is %+v, want it given way to the token card", h.model.add)
	}
	if title, _ := h.card(); title != "NEW TOKEN · WORK" {
		t.Errorf("the card open reads %q, want work's token card", title)
	}
	want := setup.TokenFor{Account: config.NewAccount{Label: "Work", Plan: "ultra"}, Add: true, Organizations: setup.Organizations{}}
	if got := h.steps.nextMake(t); !reflect.DeepEqual(got, want) {
		t.Errorf("MakeToken was asked %+v, want %+v", got, want)
	}
	if h.model.status.review == nil || len(h.model.status.review.Accounts) != 0 {
		t.Errorf("an account was added before its token's saved: the review is %+v", h.model.status.review)
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
	if got := h.model.add.label.value(); got != "qjk-spare" {
		t.Errorf("Label reads %q, want q, j and k typed, then the paste", got)
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

// A label setup refuses, as accounts add would, is said under Label, why
// alone, the card staying open and nothing written; and unsaid once the
// label changes. Every account has a label, its own as it shows, ignoring
// case.
func TestTheAddCardsRefusals(t *testing.T) {
	const two = "[[account]]\nid = \"work\"\nlabel = \"Work\"\nprimary = true\n\n[[account]]\nid = \"personal\"\nlabel = \"Personal\"\n"
	tests := []struct {
		name, label string
		why         string
	}{
		{name: "no label", label: "", why: "a label is required: every account has one"},
		{name: "another account's label", label: "Personal", why: "another account has that label already, ignoring case: give each its own"},
		{name: "another account's label in another case", label: "personal", why: "another account has that label already, ignoring case: give each its own"},
		{name: "another account's label as it shows", label: "Personal\u200b", why: "another account has that label already, ignoring case: give each its own"},
		{name: "a label of blanks", label: "   ", why: "is blank, showing nothing"},
		{name: "a label hiding the character after a sign", label: "\u0600Side", why: "holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, steps := addingTo(t, two, 2)
			h.deliver(tea.PasteMsg{Content: tt.label})
			h.pressAndDeliver("enter")
			if steps.config(t) != two {
				t.Errorf("the config reads\n%s\nwant it as it was", steps.config(t))
			}
			_, rows := h.card()
			at := slices.IndexFunc(rows, func(row string) bool { return row == "  Label" || strings.HasPrefix(row, "  Label ") })
			if at < 0 || at+1 >= len(rows) || !strings.HasPrefix(rows[at+1], "            ") || !strings.HasPrefix(tt.why, strings.TrimSpace(rows[at+1])) {
				t.Fatalf("the card reads\n%s\nwant why under Label: %q", strings.Join(rows, "\n"), tt.why)
			}
			if card := h.model.add; card.refused != setup.LabelField || card.why != tt.why || h.model.token != nil {
				t.Errorf("the card says %q of field %d, the token card %+v; want it saying %q of Label, no token made", card.why, card.refused, h.model.token, tt.why)
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
	h.typeIn("Work")
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

// While the card's account is being checked, the card takes no key: keys,
// esc and a paste change nothing, the card open; once the check answers,
// refusing it, it takes them again.
func TestTheAddCardTakesNoKeyWhileItsChecked(t *testing.T) {
	h := firstRun(t)
	h.steps.addErr = &setup.Refused{Field: setup.LabelField, Why: "another account has that label already, ignoring case: give each its own"}
	h.typeIn("Work")
	answered := h.press("enter")
	if !h.model.add.checking {
		t.Fatal("enter, and the card isn't being checked")
	}
	for _, k := range []string{"x", "tab", "backspace", "esc", "enter"} {
		h.deliver(h.press(k)...)
	}
	h.deliver(tea.PasteMsg{Content: "-2"})
	if card := h.model.add; card == nil || card.account().Label != "Work" || card.chosen != setup.LabelField {
		t.Fatalf("while it's checked, the card is %+v, want it open as it was", card)
	}
	if len(h.steps.adds) != 1 {
		t.Errorf("CheckAdding was asked %+v, want once", h.steps.adds)
	}
	h.deliver(answered...)
	h.pressAndDeliver("x")
	if card := h.model.add; card.checking || card.account().Label != "Workx" {
		t.Errorf("checked, the card is %+v, want it taking keys again", card)
	}
}

// What CheckAdding answers once the card's gone is dropped: nothing opens,
// fails or waits.
func TestACheckAnsweredWithTheCardGoneIsDropped(t *testing.T) {
	h := firstRun(t)
	h.pressAndDeliver("esc")
	for _, msg := range []addCheckedMsg{{adding: accounts.Adding{Account: config.NewAccount{Label: "Work"}}}, {err: errors.New("read config: the config doesn't parse")}} {
		h.deliver(msg)
		if h.model.add != nil || h.model.walk.failed != nil {
			t.Errorf("answered %+v with the card gone, the card is %+v and the walk %+v; want it dropped", msg, h.model.add, h.model.walk)
		}
	}
}
