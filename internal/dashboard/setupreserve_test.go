package dashboard

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
)

// The reserve card at 160×37: 12 rows tall, centred down the screen by the
// dashboard Card's one rule, and across it, 80 cells wide, as the edit
// card is.
const (
	reserveCardTop   = (37 - 12) / 2
	reserveFieldRow  = reserveCardTop + 5
	reserveTakesRow  = reserveCardTop + 6
	reserveOthersRow = reserveCardTop + 8
)

// reservingScreen is the Accounts page of a first run, work, the primary,
// its reserve unset, its reserve card open as card is.
func reservingScreen(card SetupReserveCard) Setup {
	review := frame3Review()
	review.Accounts[0].Reserve = 0
	s := accountsScreen(review, 0)
	s.Accounts.FirstRun, s.Accounts.Reserve = true, &card
	return s
}

// frame13Card is work's reserve card as frame 13 has it: 5% typed in.
func frame13Card() SetupReserveCard {
	return SetupReserveCard{ID: "work", Reserve: Field{Value: "5%", Cursor: 2, Typing: true}}
}

func TestTheReserveCardIsFrame13s(t *testing.T) {
	c := setupDrawn(reservingScreen(frame13Card()), 160, 37)
	want := []string{
		"╭─ RESERVE · WORK " + strings.Repeat("─", 61) + "╮",
		"│" + blanks(78) + "│",
		"│  work is the primary, the account the Claude apps are signed in to.          │",
		"│  Its reserve is the share of each of its windows kept back for them.         │",
		"│" + blanks(78) + "│",
		"│  Reserve   5%                        usage kept back for the Claude apps     │",
		"│            a percentage, or none                                             │",
		"│" + blanks(78) + "│",
		"│  Other accounts' reserves are set with e, or switchboard accounts edit.      │",
		"│" + blanks(78) + "│",
		"│                                                 enter save   esc no reserve  │",
		"╰" + strings.Repeat("─", 78) + "╯",
	}
	for i, w := range want {
		if got := setupCardRow(c, reserveCardTop+i); got != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", reserveCardTop+i, got, w)
		}
	}
	if got := rowOf(c, 36); got != "" {
		t.Errorf("the key line reads %q, want it empty: the card's keys are its own", got)
	}
}

func TestTheReserveCardsInks(t *testing.T) {
	c := setupDrawn(reservingScreen(frame13Card()), 160, 37)
	value := editCardContent + 10
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{name: "its title, as every card's", x: editCardX + 3, y: reserveCardTop, want: dimInk},
		{name: "what the primary is", x: editCardContent, y: reserveCardTop + 2, want: secondaryInk},
		{name: "what its reserve is", x: editCardContent, y: reserveCardTop + 3, want: secondaryInk},
		{name: "its field's name", x: editCardContent, y: reserveFieldRow, want: dimInk},
		{name: "its value, typed in", x: value, y: reserveFieldRow, want: typingInk},
		{name: "the cursor after it", x: value + 2, y: reserveFieldRow, want: caretInk},
		{name: "its hint, as the edit card's", x: editCardContent + 36, y: reserveFieldRow, want: faintInk},
		{name: "what it takes", x: value, y: reserveTakesRow, want: faintInk},
		{name: "where other reserves are set", x: editCardContent, y: reserveOthersRow, want: mutedInk},
	}
	for _, tt := range tests {
		if got := inkAt(c, tt.x, tt.y); got != tt.want {
			t.Errorf("%s, at %d,%d, is in %+v, want %+v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
}

// The card draws its field as it's given: empty and typed in, its cursor in
// its first cell; not typed in, none.
func TestTheReserveCardOpensEmptyTypedIn(t *testing.T) {
	for _, typing := range []bool{true, false} {
		c := setupDrawn(reservingScreen(SetupReserveCard{ID: "work", Reserve: Field{Typing: typing}}), 160, 37)
		if got := inkAt(c, editCardContent+10, reserveFieldRow) == caretInk; got != typing {
			t.Errorf("given typed in: %t, its field's first cell is the cursor: %t", typing, got)
		}
	}
}

// A reserve refused is said under the field, why alone, over what it takes,
// the card grown by the row.
func TestTheReserveCardSaysWhyItsReserveWasRefused(t *testing.T) {
	const why = "must be written as a percentage, such as 5%"
	card := frame13Card()
	card.Why = why
	c := setupDrawn(reservingScreen(card), 160, 37)
	top := (37 - 13) / 2
	if got, want := setupCardRow(c, top+6), "│            "+why+blanks(78-12-len(why))+"│"; got != want {
		t.Errorf("the row under Reserve reads\n%q\nwant\n%q", got, want)
	}
	if got := inkAt(c, editCardContent+10, top+6); got != errorInk {
		t.Errorf("why is in %+v, want state.destructive", got)
	}
	if got := setupCardRow(c, top+7); !strings.HasPrefix(got, "│            a percentage, or none") {
		t.Errorf("the row under why reads %q, want what the field takes", got)
	}
	if got := setupCardRow(c, top+12); got != "╰"+strings.Repeat("─", 78)+"╯" {
		t.Errorf("the card's last row reads %q, want its bottom edge", got)
	}
}

// At setup's least, 95×25, the card is whole; on 80 and 73 columns, the
// phone's, setup says only to make the terminal larger, as every page.
func TestTheReserveCardAtEachWidth(t *testing.T) {
	c := setupDrawn(reservingScreen(frame13Card()), 95, 25)
	top, x := (25-12)/2, (95-setupCardWidth)/2
	for _, tt := range []struct {
		y    int
		want string
	}{
		{top, "╭─ RESERVE · WORK"},
		{top + 2, "│  work is the primary, the account the Claude apps are signed in to."},
		{top + 8, "│  Other accounts' reserves are set with e, or switchboard accounts edit."},
		{top + 10, "enter save   esc no reserve  │"},
		{top + 11, "╰──"},
	} {
		if row := string([]rune(rowOf(c, tt.y))[x:]); !strings.Contains(row, tt.want) {
			t.Errorf("at 95×25, row %d reads %q, want %q in it", tt.y, row, tt.want)
		}
	}
	for _, width := range []int{80, 73} {
		c := setupDrawn(reservingScreen(frame13Card()), width, 37)
		if got := rowOf(c, 0); got != tooSmall {
			t.Errorf("at %d columns, the first row reads %q, want %q", width, got, tooSmall)
		}
	}
}

// The card's words are setup's, its lines each wrapped at the card's width.
func TestTheReserveCardSaysSetupsWords(t *testing.T) {
	asked := setup.AskingReserve("personal")
	c := setupDrawn(reservingScreen(SetupReserveCard{ID: "personal"}), 160, 37)
	if got := setupCardRow(c, reserveCardTop+2); !strings.HasPrefix(got, "│  "+asked.Says[0]) {
		t.Errorf("the card's first line reads %q, want %q", got, asked.Says[0])
	}
}
