package dashboard

import (
	"slices"
	"strings"
	"testing"
)

// pastedSecret is a token pasted into the paste card, a placeholder of 43
// glyphs, wider than its box.
const pastedSecret = "sk-ant-oat01-test-token-personal-0123456789"

// pasteScreen is tokenScreen's page, the paste card open as card is, in the
// token card's place.
func pasteScreen(card SetupPasteCard) Setup {
	s := tokenScreen(SetupTokenCard{})
	s.Accounts.Token, s.Accounts.Paste = nil, &card
	return s
}

// pastingCard is personal's paste card, pastedSecret typed in, its cursor
// after it.
func pastingCard() SetupPasteCard {
	return SetupPasteCard{ID: "personal", Token: Field{Value: pastedSecret, Cursor: 43}}
}

// The paste card, which no frame draws, is laid as frame 2's, in words of
// its own: what it asks for, bold, from where the token card's words start;
// what setup does with the token, in text.secondary; its field there too,
// typed in, the token drawn as dots, its start cut and marked; how a token
// copied across lines is taken, muted; and its keys, enter saving.
func TestThePasteCardIsLaidAsFrame2s(t *testing.T) {
	c := setupDrawn(pasteScreen(pastingCard()), 160, 37)
	want := []string{
		"╭─ PASTE A TOKEN · PERSONAL " + strings.Repeat("─", 51) + "╮",
		inside(""),
		inside("       Paste the token you made with claude setup-token"),
		inside("       Setup checks it with the API, then saves it, a year from today."),
		inside(""),
		inside("       Token     …" + strings.Repeat("•", 21) + "    shown as dots"),
		inside(""),
		inside("       A token copied across lines is joined up, the card's edges left out."),
		inside(""),
		inside(blanks(53) + "enter save   esc cancel  "),
		"╰" + strings.Repeat("─", 78) + "╯",
	}
	if got := tokenCardRows(c, len(want)); !slices.Equal(got, want) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	top, says := cardTop(len(want)), editCardContent+tokenSaysAt
	for _, tt := range []struct {
		name string
		x, y int
		want ink
	}{
		{name: "what it asks for", x: says, y: top + 2, want: titleInk},
		{name: "what setup does with it", x: says, y: top + 3, want: secondaryInk},
		{name: "its field's name", x: says, y: top + 5, want: dimInk},
		{name: "its dots, typed in", x: says + 11, y: top + 5, want: typingInk},
		{name: "its cursor, after them", x: says + 32, y: top + 5, want: caretInk},
		{name: "how a token copied across lines is taken", x: says, y: top + 7, want: mutedInk},
	} {
		if got := inkAt(c, tt.x, tt.y); got != tt.want {
			t.Errorf("%s, at %d,%d, is in %+v, want %+v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
	if got := rowOf(c, 36); got != "" {
		t.Errorf("the key line reads %q, want it empty: the card's keys are its own", got)
	}
}

// Whatever's pasted, the card draws a dot a glyph and never what's pasted,
// in colour or without, though the field it's given isn't marked hidden;
// while it's checked, and where setup refused it, too.
func TestThePasteCardNeverDrawsWhatsPasted(t *testing.T) {
	cards := map[string]SetupPasteCard{
		"typed in":  pastingCard(),
		"checking":  {ID: "personal", Token: Field{Value: pastedSecret}, Checking: true},
		"refused":   {ID: "personal", Token: Field{Value: pastedSecret, Cursor: 43}, Why: "the API refused the token (HTTP 401), so nothing is saved"},
		"wide":      {ID: "personal", Token: Field{Value: "漢字😀", Cursor: 3}},
		"its start": {ID: "personal", Token: Field{Value: pastedSecret, Cursor: 0}},
	}
	for name, card := range cards {
		t.Run(name, func(t *testing.T) {
			s := pasteScreen(card)
			for _, look := range []Look{NoColour(), Screen(builtin(t, "nord"))} {
				drawn := strings.Join(s.Draw(160, 37, look), "\n")
				for _, part := range []string{"sk-ant", "test-token", "0123456789", "漢", "😀"} {
					if strings.Contains(drawn, part) {
						t.Fatalf("the card is drawn with %q of what's pasted:\n%s", part, drawn)
					}
				}
				if !strings.Contains(drawn, "•••") {
					t.Errorf("the card is drawn without dots:\n%s", drawn)
				}
			}
		})
	}
}

// While the token's checked, the card spins, as the token card's checking
// does, says so in what it asks for's place, its field drawn plain, still
// dots, and has no keys, as nothing waits on one; where setup refused the
// token, why is said under its field, in state.destructive.
func TestThePasteCardChecksAndSaysWhyItsRefused(t *testing.T) {
	checking := SetupPasteCard{ID: "personal", Token: Field{Value: pastedSecret}, Checking: true}
	c := setupDrawn(pasteScreen(checking), 160, 37)
	want := []string{
		"╭─ PASTE A TOKEN · PERSONAL " + strings.Repeat("─", 51) + "╮",
		inside(""),
		inside("  ●∙∙  Checking the token and saving it"),
		inside("       Setup checks it with the API, then saves it, a year from today."),
		inside(""),
		inside("       Token     " + strings.Repeat("•", 22) + "…   shown as dots"),
		inside(""),
		inside("       A token copied across lines is joined up, the card's edges left out."),
		inside(""),
		"╰" + strings.Repeat("─", 78) + "╯",
	}
	if got := tokenCardRows(c, len(want)); !slices.Equal(got, want) {
		t.Errorf("checking, the card reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := inkAt(c, editCardContent+tokenSaysAt+11, cardTop(len(want))+5); got != secondaryInk {
		t.Errorf("checking, its dots are in %+v, want them plain, in text.secondary", got)
	}
	if _, spins := pasteScreen(checking).SpinDelay(); !spins {
		t.Error("checking, the card's spinner doesn't move")
	}
	if _, spins := pasteScreen(pastingCard()).SpinDelay(); spins {
		t.Error("typed in, the card's spinner moves, want none")
	}

	refused := pastingCard()
	refused.Why = "what's pasted holds more than the token: paste the token alone, as claude setup-token showed it"
	c = setupDrawn(pasteScreen(refused), 160, 37)
	rows := tokenCardRows(c, 13)
	for i, line := range []string{
		"                 what's pasted holds more than the token: paste the token",
		"                 alone, as claude setup-token showed it",
	} {
		if got := rows[6+i]; got != inside(line) {
			t.Errorf("refused, the card's row %d reads\n%q\nwant\n%q", 6+i, got, inside(line))
		}
	}
	if got := inkAt(c, editCardContent+tokenSaysAt+10, cardTop(13)+6); got != errorInk {
		t.Errorf("why it's refused is in %+v, want state.destructive", got)
	}
}
