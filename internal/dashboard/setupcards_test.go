package dashboard

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/theme"
)

// The edit card at 160×37: its box, centred across the screen and down it,
// by the dashboard Card's one rule, 80 cells wide and 14 rows tall; the
// column its content runs from; and its rows, from its top.
const (
	editCardX       = 40
	editCardTop     = 11
	editCardContent = editCardX + cardInset
	editLabelRow    = editCardTop + 2
	editPlanRow     = editCardTop + 4
	editPrimaryRow  = editCardTop + 7
	editReserveRow  = editCardTop + 10
	editKeysRow     = editCardTop + 12
	editBottomRow   = editCardTop + 13
)

// editingScreen is frame 3's Accounts page, personal chosen, its edit card
// open as card is.
func editingScreen(card SetupEditCard) Setup {
	s := accountsScreen(frame3Review(), 1)
	s.Accounts.Edit = &card
	return s
}

// frame6Card is personal's edit card as frame 6 has it: Plan chosen, and
// moved to Max 5x.
func frame6Card() SetupEditCard {
	return SetupEditCard{ID: "personal", Label: Field{Value: "Personal", Cursor: 8}, Reserve: Field{Value: "none", Cursor: 4}, Plan: "max5x", Chosen: setup.PlanField}
}

// setupCardRow is the card's row y, from its line to its line, as text.
func setupCardRow(c *canvas, y int) string {
	return string([]rune(rowOf(c, y))[editCardX : editCardX+setupCardWidth])
}

func TestTheEditCardIsFrame6s(t *testing.T) {
	c := setupDrawn(editingScreen(frame6Card()), 160, 37)
	want := map[int]string{
		editCardTop:        "╭─ EDIT PERSONAL " + strings.Repeat("─", 62) + "╮",
		editCardTop + 1:    "│" + blanks(78) + "│",
		editLabelRow:       "│  Label     Personal" + blanks(58) + "│",
		editPlanRow:        "│  Plan      □ Pro    ■ Max 5x    □ Max 20x    □ not set" + blanks(23) + "│",
		editPlanRow + 1:    "│            used by History to value usage" + blanks(36) + "│",
		editPrimaryRow:     "│  Primary   ■ no    □ yes" + blanks(53) + "│",
		editPrimaryRow + 1: "│            the account the Claude apps are signed in to" + blanks(22) + "│",
		editReserveRow:     "│  Reserve   none                      usage kept back for the Claude apps     │",
		editKeysRow:        "│" + blanks(30) + "tab next   ←→ select   enter save   esc cancel  │",
		editBottomRow:      "╰" + strings.Repeat("─", 78) + "╯",
	}
	for y, w := range want {
		if got := setupCardRow(c, y); got != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, got, w)
		}
	}
}

func TestTheEditCardsInks(t *testing.T) {
	c := setupDrawn(editingScreen(frame6Card()), 160, 37)
	value := editCardContent + 10
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{name: "its title, as every card's", x: editCardX + 3, y: editCardTop, want: dimInk},
		{name: "a field's name", x: editCardContent, y: editPlanRow, want: dimInk},
		{name: "the label not typed in", x: value, y: editLabelRow, want: secondaryInk},
		{name: "the mark before a choice's option", x: value, y: editPlanRow, want: choiceMarkInk},
		{name: "an option not chosen", x: value + 2, y: editPlanRow, want: mutedInk},
		{name: "the option chosen, bold while its field's chosen", x: value + 11, y: editPlanRow, want: ink{token: theme.TextPrimary, bold: true}},
		{name: "the option chosen of a field not chosen", x: value + 2, y: editPrimaryRow, want: ink{token: theme.TextPrimary}},
		{name: "a choice's hint", x: value, y: editPlanRow + 1, want: faintInk},
		{name: "a field's hint", x: editCardContent + 36, y: editReserveRow, want: faintInk},
	}
	for _, tt := range tests {
		if got := inkAt(c, tt.x, tt.y); got != tt.want {
			t.Errorf("%s, at %d,%d, is in %+v, want %+v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
}

func TestTheEditCardsTextFieldChosenIsTypedIn(t *testing.T) {
	card := frame6Card()
	card.Chosen = setup.ReserveField
	c := setupDrawn(editingScreen(card), 160, 37)
	if got := inkAt(c, editCardContent+10, editReserveRow); got != typingInk {
		t.Errorf("Reserve's value is in %+v, want it typed in", got)
	}
	if got := inkAt(c, editCardContent+14, editReserveRow); got != caretInk {
		t.Errorf("the cell after it is in %+v, want the cursor", got)
	}
	if got := inkAt(c, editCardContent+10, editLabelRow); got != secondaryInk {
		t.Errorf("Label's value is in %+v, want it not typed in", got)
	}
	if got := inkAt(c, editCardContent+21, editPlanRow); got != (ink{token: theme.TextPrimary}) {
		t.Errorf("Plan's chosen option is in %+v, want it not bold, its field not chosen", got)
	}
}

func TestARefusalIsSaidUnderItsField(t *testing.T) {
	const why = "must be written as a percentage, such as 5%"
	card := frame6Card()
	card.Refused, card.Why = setup.ReserveField, why
	c := setupDrawn(editingScreen(card), 160, 37)
	top := (37 - 15) / 2
	if got, want := setupCardRow(c, top+11), "│            "+why+blanks(78-12-len(why))+"│"; got != want {
		t.Errorf("the row under Reserve reads\n%q\nwant\n%q", got, want)
	}
	if got := inkAt(c, editCardContent+10, top+11); got != errorInk {
		t.Errorf("why is in %+v, want state.destructive", got)
	}
	if got := setupCardRow(c, top+14); got != "╰"+strings.Repeat("─", 78)+"╯" {
		t.Errorf("the card's last row reads %q, want its bottom edge, the card grown by the row", got)
	}
}

func TestACardOpenLeavesThePageUnfadedAndTheKeyLineEmpty(t *testing.T) {
	open := setupDrawn(editingScreen(frame6Card()), 160, 37)
	shut := setupDrawn(accountsScreen(frame3Review(), 1), 160, 37)
	if got := rowOf(open, 36); got != "" {
		t.Errorf("the key line reads %q, want it empty: the card's keys are its own", got)
	}
	for _, y := range []int{4, accountsHeadsRow, accountsFirstRow} {
		for x := range 160 {
			if inkAt(open, x, y) != inkAt(shut, x, y) {
				t.Fatalf("the page's row %d is drawn otherwise at %d with the card open: want it as it is, unfaded", y, x)
			}
		}
	}
}

func TestTheRemoveCardNamesTheAccount(t *testing.T) {
	s := accountsScreen(frame3Review(), 2)
	s.Accounts.Remove = &SetupRemoveCard{ID: "side"}
	c := setupDrawn(s, 160, 37)
	top := (37 - 8) / 2
	want := map[int]string{
		top:     "╭─ REMOVE SIDE " + strings.Repeat("─", 64) + "╮",
		top + 2: "│  side is taken out of the config, and its token file deleted, so no" + blanks(10) + "│",
		top + 3: "│  requests are routed to it." + blanks(50) + "│",
		top + 4: "│  Its Claude subscription is left as it is." + blanks(35) + "│",
		top + 6: "│" + blanks(50) + "enter remove   esc keep it  │",
		top + 7: "╰" + strings.Repeat("─", 78) + "╯",
	}
	for y, w := range want {
		if got := setupCardRow(c, y); got != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, got, w)
		}
	}
	if got := inkAt(c, editCardContent, top+2); got != titleInk {
		t.Errorf("the account's id is in %+v, want it bold text.primary", got)
	}
}

func TestTheRemoveCardSaysWhySetupRefused(t *testing.T) {
	const why = `account "work" is the only one, and a config needs one at least`
	s := accountsScreen(frame3Review(), 0)
	s.Accounts.Remove = &SetupRemoveCard{ID: "work", Refused: why}
	c := setupDrawn(s, 160, 37)
	top := (37 - 10) / 2
	if got := setupCardRow(c, top+6); got != "│  "+why+blanks(76-len(why))+"│" {
		t.Errorf("the card's row reads %q, want why setup refused", got)
	}
	if got := inkAt(c, editCardContent, top+6); got != errorInk {
		t.Errorf("why is in %+v, want state.destructive", got)
	}
}

// The failed fact is labelled as the page says the step failed, the
// accounts read or not.
func TestAFailedChangeToTheAccountsSaysWhatFailed(t *testing.T) {
	for _, tt := range []struct {
		failed AccountsFailed
		label  string
	}{
		{failed: AccountsNotSaved, label: "Not saved"},
		{failed: AccountsNotDeleted, label: "Not deleted"},
		{failed: AccountsNotRead, label: "Not read"},
	} {
		t.Run(tt.label, func(t *testing.T) {
			s := accountsScreen(frame3Review(), 0)
			s.Failure, s.Accounts.Failed = "it failed, as setup says", tt.failed
			c := setupDrawn(s, 160, 37)
			if got, want := rowOf(c, 6), textsAt(160, map[int]string{2: "✗", 5: tt.label, 26: s.Failure}); got != want {
				t.Errorf("row 6 reads\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestACardIsWholeAtSetupsLeast(t *testing.T) {
	c := setupDrawn(editingScreen(frame6Card()), 95, 25)
	top := (25 - 14) / 2
	x := (95 - setupCardWidth) / 2
	for _, tt := range []struct {
		y    int
		want string
	}{{top, "╭─ EDIT PERSONAL"}, {top + 12, "esc cancel  │"}, {top + 13, "╰──"}} {
		if row := string([]rune(rowOf(c, tt.y))[x:]); !strings.Contains(row, tt.want) {
			t.Errorf("at 95×25, row %d reads %q, want %q in it", tt.y, row, tt.want)
		}
	}
}
