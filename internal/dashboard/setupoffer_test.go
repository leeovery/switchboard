package dashboard

import (
	"strings"
	"testing"
)

// The offer card at 160×37, over linkScreen: its box, centred across the
// screen and down it, by the dashboard Card's one rule, 128 cells wide and
// 12 rows tall, as the line wraps onto two rows; the columns what it says
// and the line run from, as frame 8 has them; and its rows, from its top.
const (
	offerX       = 16
	offerTop     = 12
	offerSaysX   = offerX + cardInset + offerSaysAt
	offerLineX   = offerX + cardInset + offerLineAt
	offerCanRow  = offerTop + 2
	offerLineRow = offerTop + 4
	offerHandRow = offerTop + 7
	offerKeysRow = offerTop + 10
)

// What the offer card says of ~/.zshrc, as frame 8 has it.
const (
	offerCanZshrc    = "Setup can add one line to the end of ~/.zshrc, so your shell finds switchboard's claude first:"
	offerHandsOffAll = "The line hands off to a file of switchboard's own, which puts its claude first and loads its completions."
	offerLeavesZshrc = "It does nothing once switchboard is gone, and nothing else in ~/.zshrc changes."
)

// offering is the claude link step's offer card open, as card is.
func offering(card SetupOfferCard) func(*Setup) {
	return func(s *Setup) { s.Linking.Offer = &card }
}

// offerRow is the card's row y at 160×37, from its line to its line, as
// text.
func offerRow(c *canvas, y int) string {
	return string([]rune(c.row(y, Look{}))[offerX : offerX+offerCardWidth])
}

// offerInside is a row of the offer card, 128 cells wide, text set in
// from its line by in cells, then blanks to its line.
func offerInside(in int, text string) string {
	return "│" + blanks(in) + text + blanks(126-in-len([]rune(text))) + "│"
}

func TestTheOfferCardIsFrame8s(t *testing.T) {
	c := setupDrawn(linkScreen(offering(SetupOfferCard{})), 160, 37)
	title := "─ ADD A LINE TO ~/.zshrc "
	keys := "enter add it   n I'll add it myself"
	want := map[int]string{
		offerTop:         "╭" + title + strings.Repeat("─", 126-len([]rune(title))) + "╮",
		offerTop + 1:     offerInside(0, ""),
		offerCanRow:      offerInside(3, offerCanZshrc),
		offerCanRow + 1:  offerInside(0, ""),
		offerLineRow + 2: offerInside(0, ""),
		offerHandRow:     offerInside(3, offerHandsOffAll),
		offerHandRow + 1: offerInside(3, offerLeavesZshrc),
		offerKeysRow - 1: offerInside(0, ""),
		offerKeysRow:     offerInside(126-2-len(keys), keys),
		offerKeysRow + 1: "╰" + strings.Repeat("─", 126) + "╯",
	}
	for y, w := range want {
		if got := offerRow(c, y); got != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, got, w)
		}
	}
	var line string
	for y := offerLineRow; y < offerLineRow+2; y++ {
		row := offerRow(c, y)
		if !strings.HasPrefix(row, "│"+blanks(5)) || strings.HasPrefix(row, "│"+blanks(6)) {
			t.Errorf("row %d reads %q, want the line from 5 cells in its line", y, row)
		}
		line += strings.TrimSpace(strings.Trim(row, "│")) + " "
	}
	if unspaced(line) != unspaced(shellLine) {
		t.Errorf("the line reads, joined,\n%q\nwant it whole,\n%q", line, shellLine)
	}
	rows := textOf(linkScreen(offering(SetupOfferCard{})).Draw(160, 37, Look{}))
	if rows[36] != "" {
		t.Errorf("the key line reads %q, want it empty: the card's keys are its own", rows[36])
	}
	for y := offerTop; y <= offerKeysRow+1; y++ {
		if outside := strings.TrimSpace(string([]rune(rows[y])[:offerX])); outside != "" {
			t.Errorf("row %d reads %q left of the card, want nothing to put right by hand while it offers the line", y, outside)
		}
	}
}

func TestTheOfferCardsInks(t *testing.T) {
	c := setupDrawn(linkScreen(offering(SetupOfferCard{})), 160, 37)
	for _, tt := range []struct {
		what    string
		x, y, n int
		ink     ink
	}{
		{what: "its title, as every card's", x: offerX + 3, y: offerTop, n: len("ADD A LINE TO ~/.zshrc"), ink: dimInk},
		{what: "what setup can do, text.secondary", x: offerSaysX, y: offerCanRow, n: len(offerCanZshrc), ink: secondaryInk},
		{what: "the line, accent.mode", x: offerLineX, y: offerLineRow, n: 10, ink: accentInk},
		{what: "what the line does, muted", x: offerSaysX, y: offerHandRow, n: len(offerHandsOffAll), ink: mutedInk},
		{what: "what it leaves, muted", x: offerSaysX, y: offerHandRow + 1, n: len(offerLeavesZshrc), ink: mutedInk},
	} {
		if says := inksFrom(c, tt.x, tt.y, tt.n, tt.ink); says != "" {
			t.Errorf("%s: %s", tt.what, says)
		}
	}
}

func TestTheOfferCardNamesTheFileSetupFound(t *testing.T) {
	const where = "~/.config/zsh/.zshrc"
	s := linkScreen(offering(SetupOfferCard{}), func(s *Setup) { s.Link.Shell.Where = where })
	text := strings.Join(textOf(s.Draw(160, 37, Look{})), "\n")
	for _, want := range []string{
		"╭─ ADD A LINE TO " + where + " ─",
		"Setup can add one line to the end of " + where + ", so your shell finds switchboard's claude first:",
		"It does nothing once switchboard is gone, and nothing else in " + where + " changes.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the card doesn't say %q:\n%s", want, text)
		}
	}
}

func TestWhileTheLineIsAddedTheOfferCardSaysSoAndOffersQAlone(t *testing.T) {
	s := linkScreen(offering(SetupOfferCard{Adding: true}))
	c := setupDrawn(s, 160, 37)
	for y, w := range map[int]string{
		offerCanRow:  offerInside(3, "●∙∙  Adding the line to the end of ~/.zshrc"),
		offerKeysRow: offerInside(126-2-len("q quit"), "q quit"),
	} {
		if got := offerRow(c, y); got != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, got, w)
		}
	}
	if _, spins := s.SpinDelay(); !spins {
		t.Error("the card's spinner doesn't move while the line's added")
	}
	if _, spins := linkScreen(offering(SetupOfferCard{})).SpinDelay(); spins {
		t.Error("the card's spinner moves while it only offers the line")
	}
}

func TestTheOfferCardCopiesNothing(t *testing.T) {
	s := linkScreen(offering(SetupOfferCard{}))
	for _, key := range []string{"c", "d"} {
		if got := s.Copies(key); got != "" {
			t.Errorf("Copies(%s) = %q while the card's open, want nothing", key, got)
		}
	}
}

func TestAtSetupsLeastTheOfferCardIsWholeItsLineWrappedNeverCut(t *testing.T) {
	c := setupDrawn(linkScreen(offering(SetupOfferCard{})), 95, 25)
	x := (95 - 93) / 2
	var top, keys, bottom int
	var line string
	for y := range 25 {
		row := []rune(c.row(y, Look{}))
		if len(row) > 95 {
			t.Errorf("row %d is %d cells wide, past 95", y, len(row))
		}
		text := string(row[min(x, len(row)):min(len(row), x+93)])
		switch {
		case strings.HasPrefix(text, "╭─ ADD A LINE TO ~/.zshrc"):
			top = y
		case strings.HasSuffix(text, "n I'll add it myself  │"):
			keys = y
		case strings.HasPrefix(text, "╰─"):
			bottom = y
		case strings.HasPrefix(text, "│"+blanks(5)) && !strings.HasPrefix(text, "│"+blanks(6)):
			line += strings.TrimSpace(strings.Trim(text, "│")) + " "
		}
	}
	if top == 0 || keys != bottom-1 {
		t.Errorf("at 95×25, the card's top edge is on row %d, its keys on %d, its bottom edge on %d; want it whole", top, keys, bottom)
	}
	if unspaced(line) != unspaced(shellLine) {
		t.Errorf("at 95×25, the line reads, joined,\n%q\nwant it whole,\n%q", line, shellLine)
	}
}

// unspaced is text without its spaces, as a line wrapped where it has them
// reads joined up again.
func unspaced(text string) string {
	return strings.ReplaceAll(text, " ", "")
}
