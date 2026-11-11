package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/theme"
)

// whichScreen is tokenScreen's page, the card asking which account a token
// is for open as card is, in the token card's place.
func whichScreen(card SetupWhichCard) Setup {
	s := tokenScreen(SetupTokenCard{})
	s.Accounts.Token, s.Accounts.Which = nil, &card
	return s
}

// teamWhich is the card asking which account team's token is for, Spare
// chosen: a new account, or Spare or Old, each removed.
func teamWhich() SetupWhichCard {
	return SetupWhichCard{Label: "Team", Chosen: 1, Ask: setup.WhichAsk{
		Says: []string{"Its organization matches no account this Mac has had.", "Is it a new account, or one removed?"},
		Choices: []setup.WhichChoice{
			{Label: "new account"},
			{Label: "Spare", Seen: "last seen 14 Sep 2026"},
			{Label: "Old", Seen: "last seen 3 Mar 2026"},
		},
	}}
}

// The card asking which account a token is for: titled by the account's
// label; why it asks, in text.secondary; then a choice list, ■ before the
// choice chosen, bold, □ before the rest, muted, each mark in accent.key,
// and when each account was last seen, faint, in a column past the widest
// label; and its keys, enter saving.
func TestTheCardAskingWhichIsAChoiceList(t *testing.T) {
	c := setupDrawn(whichScreen(teamWhich()), 160, 37)
	want := []string{
		"╭─ WHICH ACCOUNT · TEAM " + strings.Repeat("─", 55) + "╮",
		inside(""),
		inside("  Its organization matches no account this Mac has had."),
		inside("  Is it a new account, or one removed?"),
		inside(""),
		inside("  □ new account"),
		inside("  ■ Spare          last seen 14 Sep 2026"),
		inside("  □ Old            last seen 3 Mar 2026"),
		inside(""),
		inside(blanks(41) + "↑↓ select   enter save   esc cancel  "),
		"╰" + strings.Repeat("─", 78) + "╯",
	}
	if got := tokenCardRows(c, len(want)); !slices.Equal(got, want) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	top := cardTop(len(want))
	for _, tt := range []struct {
		name string
		x, y int
		want ink
	}{
		{name: "why it asks", x: editCardContent, y: top + 2, want: secondaryInk},
		{name: "the mark before a choice", x: editCardContent, y: top + 5, want: choiceMarkInk},
		{name: "the mark before the choice chosen", x: editCardContent, y: top + 6, want: choiceMarkInk},
		{name: "a choice not chosen", x: editCardContent + 2, y: top + 5, want: mutedInk},
		{name: "the choice chosen", x: editCardContent + 2, y: top + 6, want: ink{token: theme.TextPrimary, bold: true}},
		{name: "when it was last seen", x: editCardContent + 17, y: top + 6, want: faintInk},
	} {
		if got := inkAt(c, tt.x, tt.y); got != tt.want {
			t.Errorf("%s, at %d,%d, is in %+v, want %+v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
}

// A label too wide to leave when it was last seen room is cut short, never
// the day.
func TestALabelTooWideIsCutShortForWhenItWasLastSeen(t *testing.T) {
	card := teamWhich()
	card.Ask.Choices[1].Label = strings.Repeat("Spare ", 15)
	c := setupDrawn(whichScreen(card), 160, 37)
	row := setupCardRow(c, cardTop(11)+6)
	if !strings.Contains(row, "…") || !strings.HasSuffix(strings.TrimRight(row, " │"), "last seen 14 Sep 2026") {
		t.Errorf("the row reads %q, want the label cut short, then when it was last seen in full", row)
	}
}
