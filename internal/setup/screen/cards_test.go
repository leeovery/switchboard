package screen

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// card is the card open over the page, as it's drawn without colour: its
// title, from its top edge, and each row inside its line, from the cell
// after it, its blanks after trimmed; none where no card is open.
func (h *harness) card() (title string, rows []string) {
	h.t.Helper()
	at, x := -1, 0
	screen := h.rows()
	for y, row := range screen {
		if i := slices.Index([]rune(row), '╭'); i >= 0 {
			at, x = y, i
			break
		}
	}
	if at < 0 {
		return "", nil
	}
	top := []rune(screen[at])
	title = strings.Trim(string(top[x+1:]), "─╮ ")
	for _, row := range screen[at+1:] {
		r := []rune(row)
		if len(r) > x && r[x] == '╰' {
			break
		}
		inside := string(r[min(x+1, len(r)):min(x+79, len(r))])
		rows = append(rows, strings.TrimRight(inside, " "))
	}
	return title, rows
}

// cardRow is the card's row that starts with text, two cells in, as its
// content does: "" where none does.
func (h *harness) cardRow(text string) string {
	h.t.Helper()
	_, rows := h.card()
	for _, row := range rows {
		if strings.HasPrefix(row, "  "+text) {
			return row
		}
	}
	return ""
}

// editingPersonal is frame 3's accounts, personal chosen and its edit card
// open.
func editingPersonal(t *testing.T) *harness {
	t.Helper()
	h := accountsHarness(t, frame3Steps())
	h.start()
	h.pressAndDeliver("down")
	h.pressAndDeliver("e")
	return h
}

func TestEOpensTheChosenAccountsEditCard(t *testing.T) {
	h := editingPersonal(t)
	title, rows := h.card()
	if title != "EDIT PERSONAL" {
		t.Fatalf("the card's title reads %q, want EDIT PERSONAL", title)
	}
	want := []string{
		"",
		"  Label     Personal",
		"",
		"  Plan      □ Pro    □ Max 5x    □ Max 20x    ■ not set",
		"            used by History to value usage",
		"",
		"  Primary   ■ no    □ yes",
		"            the account the Claude apps are signed in to",
		"",
		"  Reserve   none                      usage kept back for the Claude apps",
		"",
		"                              tab next   ←→ select   enter save   esc cancel",
	}
	if !slices.Equal(rows, want) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	if got := h.rows()[keyRow]; got != "" {
		t.Errorf("with the card open, the key line reads %q, want it empty: the card's keys are its own", got)
	}
	if got := h.model.edit.chosen; got != setup.LabelField {
		t.Errorf("the card opens on field %d, want Label, typed in", got)
	}
}

// edited opens personal's edit card, presses keys, then enter, and returns
// the edits EditAccount was given.
func edited(t *testing.T, keys ...string) (*harness, []fakeEdit) {
	t.Helper()
	h := editingPersonal(t)
	for _, k := range keys {
		h.pressAndDeliver(k)
	}
	h.pressAndDeliver("enter")
	return h, h.steps.edits
}

func TestEachFieldOfTheEditCardIsSaved(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want setup.Edit
		row  string
	}{
		{name: "its label", keys: []string{"backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "H", "o", "m", "e"}, want: setup.Edit{Label: new("Home")}, row: "  2  personal        Home"},
		{name: "its plan", keys: []string{"tab", "left", "left"}, want: setup.Edit{Plan: new("max5x")}, row: "  2  personal        Personal      Max 5x"},
		{name: "its plan, no further than the first", keys: []string{"tab", "left", "left", "left", "left", "left"}, want: setup.Edit{Plan: new("pro")}, row: "  2  personal        Personal      Pro"},
		{name: "the primary", keys: []string{"tab", "tab", "right"}, want: setup.Edit{Primary: true}, row: "  2  personal ◆      Personal"},
		{name: "its reserve", keys: []string{"tab", "tab", "tab", "backspace", "backspace", "backspace", "backspace", "5", "%"}, want: setup.Edit{Reserve: new("5%")}, row: "  2  personal        Personal      not set    5%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, edits := edited(t, tt.keys...)
			if len(edits) != 1 || edits[0].id != "personal" || !sameEdit(edits[0].edit, tt.want) {
				t.Fatalf("EditAccount was given %+v, want personal's %+v alone", edits, tt.want)
			}
			if title, _ := h.card(); title != "" {
				t.Errorf("saved, the card %q is still open", title)
			}
			if got := h.rows()[firstRow+2]; !strings.HasPrefix(got, "▌"+tt.row[1:]) {
				t.Errorf("personal's row reads\n%q\nwant it read again, as saved, and still chosen\n%q", got, tt.row)
			}
			if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, 2 working"; got != want {
				t.Errorf("the status line reads %q, want %q: each token as it was checked, kept", got, want)
			}
			if got := len(h.steps.checks); got != 3 {
				t.Errorf("ReviewOf was asked %d times, want each account's token checked once, as none changed", got)
			}
		})
	}
}

// sameEdit reports whether two edits ask the same.
func sameEdit(a, b setup.Edit) bool {
	same := func(x, y *string) bool { return x == nil && y == nil || x != nil && y != nil && *x == *y }
	return same(a.Label, b.Label) && same(a.Plan, b.Plan) && same(a.Reserve, b.Reserve) && a.Primary == b.Primary
}

// A plan set to none is saved as config.PlanNone, one of the choices the
// card chooses between, and an account without a plan opens on it.
func TestAPlanSetToNoneIsSavedAsNone(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	h.pressAndDeliver("e")
	for _, k := range []string{"tab", "right", "enter"} {
		h.pressAndDeliver(k)
	}
	if edits := h.steps.edits; len(edits) != 1 || !sameEdit(edits[0].edit, setup.Edit{Plan: new("none")}) {
		t.Fatalf("EditAccount was given %+v, want work's plan none alone", edits)
	}
	if got := h.rows()[firstRow]; !strings.Contains(got, "not set") {
		t.Errorf("work's row reads %q, want its plan not set", got)
	}
}

func TestEnterOnAnEditCardChangingNothingSavesNothing(t *testing.T) {
	h, edits := edited(t, "tab", "left", "right")
	if len(edits) != 0 {
		t.Errorf("EditAccount was given %+v, want nothing: nothing changed", edits)
	}
	if title, _ := h.card(); title != "" {
		t.Errorf("the card %q is still open, want it closed", title)
	}
}

func TestEscClosesTheEditCardSavingNothing(t *testing.T) {
	h := editingPersonal(t)
	h.typeIn("Home")
	h.pressAndDeliver("tab")
	h.pressAndDeliver("left")
	reviews := h.steps.reviews
	h.pressAndDeliver("esc")
	if len(h.steps.edits) != 0 {
		t.Errorf("esc gave EditAccount %+v, want nothing saved", h.steps.edits)
	}
	if title, _ := h.card(); title != "" || h.model.edit != nil {
		t.Errorf("after esc, the card %q is open, want it closed", title)
	}
	if h.steps.reviews != reviews {
		t.Errorf("esc read the accounts again, want nothing asked")
	}
	if got := h.rows()[firstRow+2]; !strings.HasPrefix(got, "▌ 2  personal        Personal      not set") {
		t.Errorf("personal's row reads %q, want it as it was", got)
	}
}

func TestARefusalIsSaidUnderItsFieldTheCardKept(t *testing.T) {
	tests := []struct {
		name  string
		field setup.Field
		keys  []string
		under string
	}{
		{name: "the label's", field: setup.LabelField, keys: []string{"x"}, under: "  Label     Personalx"},
		{name: "the reserve's", field: setup.ReserveField, keys: []string{"tab", "tab", "tab", "5"}, under: "  Reserve   none5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := editingPersonal(t)
			const why = "must be written as a percentage, such as 5%"
			h.steps.editErr = &setup.Refused{Field: tt.field, Why: why}
			for _, k := range tt.keys {
				h.pressAndDeliver(k)
			}
			h.pressAndDeliver("enter")
			_, rows := h.card()
			i := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, tt.under) })
			if i < 0 || i+1 >= len(rows) || rows[i+1] != "            "+why {
				t.Fatalf("the card reads\n%s\nwant %q under %q, why alone", strings.Join(rows, "\n"), why, tt.under)
			}
			if h.model.edit == nil || h.model.walk.failed != nil {
				t.Fatal("the card closed, or the step failed, on a refusal: want the card kept")
			}
			h.pressAndDeliver("backspace")
			if _, rows := h.card(); slices.Contains(rows, "            "+why) {
				t.Errorf("once the field's changed, the card still says why it was refused")
			}
		})
	}
}

func TestAFailureSavingTheEditCardFailsTheStep(t *testing.T) {
	h := editingPersonal(t)
	h.steps.editErr = errNoConfig
	h.typeIn("x")
	h.pressAndDeliver("enter")
	if h.model.edit != nil {
		t.Error("the card's still open, want it closed: the step failed")
	}
	if got, want := h.rows()[firstUnderHeading], factRowOf("✗", setup.Said{Label: "Not saved", Detail: errNoConfig.Error()}); got != want {
		t.Errorf("the page's row reads\n%q\nwant\n%q", got, want)
	}
	if got := h.model.walk.left(); got != (Left{Ending: Failed, Err: errNoConfig}) {
		t.Errorf("setup leaves %+v, want it failed as saving did", got)
	}
}

func TestThePrimarysEditCardKeepsItThePrimary(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	h.pressAndDeliver("e")
	h.pressAndDeliver("tab")
	h.pressAndDeliver("tab")
	for _, k := range []string{"left", "right", "left"} {
		h.pressAndDeliver(k)
		if got := h.cardRow("Primary"); got != "  Primary   □ no    ■ yes" {
			t.Fatalf("after %s, Primary reads %q, want yes, unchanged", k, got)
		}
	}
	h.pressAndDeliver("enter")
	if len(h.steps.edits) != 0 {
		t.Errorf("EditAccount was given %+v, want nothing asked", h.steps.edits)
	}
}

func TestTheEditCardsChoicesAreBoldWhileChosen(t *testing.T) {
	h := editingPersonal(t)
	h.pressAndDeliver("tab")
	page := h.model.page()
	if got := page.Accounts.Edit.Chosen; got != setup.PlanField {
		t.Fatalf("tab chose field %d, want Plan", got)
	}
	if page.Accounts.Edit.Label.Typing {
		t.Error("with Plan chosen, the label is still typed in")
	}
	for range 3 {
		h.pressAndDeliver("tab")
	}
	if got := h.model.page().Accounts.Edit.Chosen; got != setup.LabelField {
		t.Errorf("tab past Reserve chose field %d, want Label again", got)
	}
}

func TestQDoesNothingWhileACardIsOpenAndCtrlCLeaves(t *testing.T) {
	h := editingPersonal(t)
	h.pressAndDeliver("tab")
	if quits(h.press("q")) {
		t.Error("q quit with the card open, want it to do nothing: the key line offers it nowhere")
	}
	if h.model.edit == nil {
		t.Error("q closed the card")
	}
	if !quits(h.press("ctrl+c")) {
		t.Error("ctrl+c didn't quit with the card open")
	}
	h.pressAndDeliver("esc")
	h.pressAndDeliver("r")
	if quits(h.press("q")) || h.model.remove == nil {
		t.Error("q quit, or closed the remove card")
	}
	for _, k := range []string{"up", "j", "pgdown", "p", "e"} {
		h.pressAndDeliver(k)
	}
	h.deliver(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if h.model.chosen != 1 || h.model.scroll != 0 || h.model.edit != nil || len(h.steps.edits) != 0 {
		t.Errorf("with the card open, the page's keys acted: row %d chosen, scrolled %d, edits %+v", h.model.chosen, h.model.scroll, h.steps.edits)
	}
}

func TestTheLabelsViewHoldsStillWhileItsCursorMovesWithinIt(t *testing.T) {
	h := editingPersonal(t)
	for range len("Personal") {
		h.pressAndDeliver("backspace")
	}
	h.typeIn("abcdefghijklmnopqrstuvwxyz0123")
	end := h.cardRow("Label")
	if want := "  Label     …jklmnopqrstuvwxyz0123"; end != want {
		t.Fatalf("typed to its end, the label reads %q, want %q: its start cut, marked", end, want)
	}
	for range 21 {
		h.pressAndDeliver("left")
		if got := h.cardRow("Label"); got != end {
			t.Fatalf("the cursor moved within the view, and it reads %q, want it held still, %q", got, end)
		}
	}
	h.pressAndDeliver("left")
	if got, want := h.cardRow("Label"), "  Label     …ijklmnopqrstuvwxyz0123"; got != want {
		t.Errorf("the cursor moved past the view's first, and it reads %q, want %q: a place back", got, want)
	}
	h.pressAndDeliver("home")
	if got, want := h.cardRow("Label"), "  Label     abcdefghijklmnopqrstuv…"; got != want {
		t.Errorf("at the start, it reads %q, want %q", got, want)
	}
}

func TestPMakesTheChosenAccountThePrimary(t *testing.T) {
	steps := frame3Steps()
	steps.review.Unmarked, steps.review.Accounts[0].Primary = true, false
	h := accountsHarness(t, steps)
	h.start()
	h.pressAndDeliver("enter")
	if got := h.model.page().At; got != dashboard.SetupAccounts {
		t.Fatalf("with none the primary, enter moved on to %s", got.Title())
	}
	h.pressAndDeliver("down")
	h.pressAndDeliver("p")
	if len(steps.edits) != 1 || steps.edits[0].id != "personal" || !sameEdit(steps.edits[0].edit, setup.Edit{Primary: true}) {
		t.Fatalf("p gave EditAccount %+v, want personal made the primary", steps.edits)
	}
	if got := h.rows()[firstRow+2]; !strings.HasPrefix(got, "▌ 2  personal ◆") {
		t.Errorf("personal's row reads %q, want it marked the primary, still chosen", got)
	}
	if rows := strings.Join(h.rows(), "\n"); strings.Contains(rows, "No account is the primary") {
		t.Errorf("the page still says none is the primary:\n%s", rows)
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().At; got != dashboard.SetupPriming {
		t.Errorf("once there's a primary, enter shows %s, want Priming", got.Title())
	}
}

func TestEnterWaitsWhilePMakesThePrimary(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	h.pressAndDeliver("down")
	answered := h.press("p")
	h.pressAndDeliver("enter")
	if got := h.model.page().At; got != dashboard.SetupAccounts {
		t.Fatalf("enter, while p's change is written, shows %s, want Accounts, waiting", got.Title())
	}
	h.deliver(answered...)
	h.pressAndDeliver("enter")
	if got := h.model.page().At; got != dashboard.SetupPriming {
		t.Errorf("enter, once p's change is written, shows %s, want Priming", got.Title())
	}
}

func TestPMovesThePrimarysMark(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	h.start()
	h.pressAndDeliver("down")
	h.pressAndDeliver("down")
	h.pressAndDeliver("p")
	if got := h.rows()[firstRow]; strings.Contains(got, "◆") {
		t.Errorf("work's row reads %q, want its mark taken off", got)
	}
	if got := h.rows()[firstRow+4]; !strings.HasPrefix(got, "▌ 3  side ◆") {
		t.Errorf("side's row reads %q, want it marked the primary", got)
	}
	h.pressAndDeliver("p")
	if len(h.steps.edits) != 1 {
		t.Errorf("p on the primary gave EditAccount %+v, want nothing more asked", h.steps.edits)
	}
}

// removing is frame 3's accounts, the account down rows from the first
// chosen, and its remove card open.
func removing(t *testing.T, steps *fakeSteps, down int) *harness {
	t.Helper()
	h := accountsHarness(t, steps)
	h.start()
	for range down {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("r")
	return h
}

func TestRAsksBeforeRemovingTheAccount(t *testing.T) {
	h := removing(t, frame3Steps(), 2)
	title, rows := h.card()
	want := []string{
		"",
		"  side is taken out of the config, and its token file deleted, so no",
		"  requests are routed to it.",
		"  Its Claude subscription is left as it is.",
		"",
		strings.Repeat(" ", 50) + "enter remove   esc keep it",
	}
	if title != "REMOVE SIDE" || !slices.Equal(rows, want) {
		t.Errorf("the card is %q, reading\n%s\nwant REMOVE SIDE, reading\n%s", title, strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	if got := h.rows()[keyRow]; got != "" {
		t.Errorf("the key line reads %q, want it empty", got)
	}
	if len(h.steps.removes) != 0 {
		t.Errorf("r removed %v before asking", h.steps.removes)
	}
	h.pressAndDeliver("enter")
	if !slices.Equal(h.steps.removes, []string{"side"}) {
		t.Fatalf("enter removed %v, want side", h.steps.removes)
	}
	if title, _ := h.card(); title != "" {
		t.Errorf("removed, the card %q is still open", title)
	}
	if got := h.rows()[firstRow+4]; !strings.HasPrefix(got, "  +  add an account") {
		t.Errorf("the row after personal's reads %q, want + add an account, side gone", got)
	}
	if got := h.rows()[firstRow+2]; !strings.HasPrefix(got, "▌ 2  personal") {
		t.Errorf("personal's row reads %q, want it chosen, the last account left, never + add an account", got)
	}
}

func TestEscKeepsTheAccount(t *testing.T) {
	h := removing(t, frame3Steps(), 1)
	h.pressAndDeliver("esc")
	if len(h.steps.removes) != 0 || h.model.remove != nil {
		t.Errorf("esc removed %v, or left the card open", h.steps.removes)
	}
	if got := h.rows()[firstRow+2]; !strings.HasPrefix(got, "▌ 2  personal") {
		t.Errorf("personal's row reads %q, want it kept, chosen", got)
	}
}

func TestRemovingThePrimaryLeavesNoneMarked(t *testing.T) {
	h := removing(t, frame3Steps(), 0)
	h.pressAndDeliver("enter")
	if got := h.rows()[firstRow]; !strings.HasPrefix(got, "▌ 1  personal ") || strings.Contains(got, "◆") {
		t.Errorf("the first row reads %q, want personal, chosen, as the one after work, unmarked", got)
	}
	if rows := strings.Join(h.rows(), "\n"); !strings.Contains(rows, "No account is the primary") {
		t.Errorf("the page doesn't say none is the primary:\n%s", rows)
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().At; got != dashboard.SetupAccounts {
		t.Errorf("enter moved on to %s, want it waiting for a primary", got.Title())
	}
}

func TestRemovingTheLastAccountIsRefusedSayingWhy(t *testing.T) {
	steps := newFakeSteps()
	steps.review.Accounts = steps.review.Accounts[:1]
	const why = `account "work" is the only one, and a config needs one at least`
	steps.removeErr = &setup.Refused{Why: why}
	h := removing(t, steps, 0)
	h.pressAndDeliver("enter")
	_, rows := h.card()
	if !slices.Contains(rows, "  "+why) {
		t.Errorf("the card reads\n%s\nwant it saying why", strings.Join(rows, "\n"))
	}
	if h.model.remove == nil || h.model.walk.failed != nil {
		t.Error("the card closed, or the step failed: want the card kept, saying why")
	}
	h.pressAndDeliver("esc")
	if got := h.rows()[firstRow]; !strings.HasPrefix(got, "▌ 1  work") {
		t.Errorf("the first row reads %q, want work kept", got)
	}
}

// The chosen row follows the account it was on once the accounts are read
// again: an account removed above it moves it, scrolled to it; a change that
// leaves it where it is keeps a page scrolled past it, as to the notes,
// where it is.
func TestTheChosenRowFollowsItsAccount(t *testing.T) {
	t.Run("an account removed above it", func(t *testing.T) {
		h := accountsHarness(t, frame3Steps())
		h.start()
		h.pressAndDeliver("down")
		h.pressAndDeliver("down")
		h.model.remove = &removeCard{id: "work"}
		h.pressAndDeliver("enter")
		if got := h.rows()[firstRow+2]; !strings.HasPrefix(got, "▌ 2  side") {
			t.Errorf("the second row reads %q, want side, still chosen", got)
		}
	})
	t.Run("the bottom account removed", func(t *testing.T) {
		h := removing(t, frame3Steps(), 2)
		h.pressAndDeliver("enter")
		if h.model.chosen != 1 || h.model.adding() {
			t.Errorf("row %d is chosen, want personal's, 1, the new last, never + add an account", h.model.chosen)
		}
	})
	t.Run("a change leaving it where it is, the page scrolled to the notes", func(t *testing.T) {
		h := accountsHarness(t, frame3Steps())
		h.start()
		h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
		h.pressAndDeliver("down")
		h.pressAndDeliver("pgdown")
		scrolled := h.model.scroll
		if scrolled == 0 {
			t.Fatal("pgdown didn't scroll the page")
		}
		h.pressAndDeliver("p")
		if got := h.model.scroll; got != scrolled {
			t.Errorf("p scrolled the page to %d, want it kept at %d: the chosen account didn't move", got, scrolled)
		}
		if got := h.model.chosen; got != 1 {
			t.Errorf("row %d is chosen, want personal's, 1", got)
		}
	})
	t.Run("the chosen account moved, the page scrolled to it", func(t *testing.T) {
		h := accountsHarness(t, frame3Steps())
		h.start()
		h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
		h.pressAndDeliver("down")
		h.pressAndDeliver("down")
		h.pressAndDeliver("pgdown")
		h.pressAndDeliver("pgdown")
		h.model.remove = &removeCard{id: "work"}
		h.pressAndDeliver("enter")
		if got := h.model.chosen; got != 1 {
			t.Fatalf("row %d is chosen, want side's, 1", got)
		}
		if got := chosenRowOf(h.rows()); !strings.HasPrefix(got, "▌ 2  side") {
			t.Errorf("the chosen row shown reads %q, want side's, scrolled into view", got)
		}
	})
	t.Run("an edit leaving its account where it is", func(t *testing.T) {
		h, _ := edited(t, "tab", "left")
		if got := h.model.chosen; got != 1 {
			t.Errorf("row %d is chosen, want personal's, 1", got)
		}
	})
}

// chosenRowOf is the row of rows ▌ leads: "" where none does.
func chosenRowOf(rows []string) string {
	for _, row := range rows {
		if strings.HasPrefix(row, "▌") {
			return row
		}
	}
	return ""
}
