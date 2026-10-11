package dashboard

import (
	"strings"
	"testing"
)

func TestACardIsAsWideAsItWantsWithACellEitherSideAtLeast(t *testing.T) {
	tests := []struct {
		name          string
		width, wanted int
		want          int
	}{
		{name: "the routing card, with room", width: 160, wanted: routingWidth, want: 82},
		{name: "help, with room", width: 216, wanted: helpWidth, want: 128},
		{name: "help, short of room", width: 120, wanted: helpWidth, want: 118},
		{name: "on a phone, the phone's width less two", width: 52, wanted: routingWidth, want: 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameOf(tt.width, 40).cardWidth(tt.wanted); got != tt.want {
				t.Errorf("the card is %d cells wide, want %d", got, tt.want)
			}
		})
	}
}

// titled is a card of its title alone, as wide as help.
type titled string

func (k titled) card(f Frame) Card {
	return Card{title: string(k), width: f.cardWidth(helpWidth)}
}

func TestACardIsDrawnCentredOverThePage(t *testing.T) {
	f := frameOf(160, 40)
	page := f.Draw(threeRouted(), now)
	f.Card = titled("HELP · OVERVIEW")
	rows := f.Draw(threeRouted(), now)

	top, bottom := []rune(rows[19]), []rune(rows[20])
	if got, want := string(top[16:35]), "╭─ HELP · OVERVIEW "; got != want || top[143] != '╮' {
		t.Errorf("row 20 = %q, want the card's top edge from column 17, %q, to column 144", rows[19], want)
	}
	if bottom[16] != '╰' || bottom[143] != '╯' {
		t.Errorf("row 21 = %q, want the card's bottom edge from column 17 to column 144", rows[20])
	}
	for i := range rows {
		if i == 19 || i == 20 {
			continue
		}
		if rows[i] != page[i] {
			t.Errorf("row %d = %q, want the page's, %q, the card on rows 20 and 21 alone", i+1, rows[i], page[i])
		}
	}
	if got := string(top[:16]) + string(top[144:]); got != string([]rune(page[19])[:16])+string([]rune(page[19])[144:]) {
		t.Errorf("row 20 = %q, want the page's either side of the card", rows[19])
	}
}

func TestTheRoutingCardIsTitledAsWhatItRoutes(t *testing.T) {
	f := frameOf(160, 40)
	f.Card = RoutingCard{}
	rows := f.Draw(threeRouted(), now)
	if !strings.Contains(rows[19], "╭─ ROUTING · NEW SESSIONS ─") {
		t.Errorf("row 20 = %q, want the routing card's top edge, titled ROUTING · NEW SESSIONS", rows[19])
	}
}

func TestHelpIsTitledAsThePageIsNamed(t *testing.T) {
	for _, v := range Views() {
		if got, want := (Help{}).card(Frame{Width: 160, Height: 40, View: v}).title, "HELP · "+strings.ToUpper(v.Title()); got != want {
			t.Errorf("on %s, help is titled %q, want %q", v.Title(), got, want)
		}
	}
}

func TestNoCardScrollsWhileNoneIsOpen(t *testing.T) {
	if got := frameOf(160, 40).CardScrolling(); got != (Scrolling{}) {
		t.Errorf("with no card open, the card scrolls %+v, want nothing", got)
	}
}
