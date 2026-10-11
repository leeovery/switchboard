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

// laidCard is a card of a body of the lines given, with keys or none, as
// wide as the routing card.
type laidCard struct {
	body []line
	keys []Key
}

func (k laidCard) card(f Frame) Card {
	return Card{title: "ROUTING · NEW SESSIONS", width: f.cardWidth(routingWidth), body: k.body, keys: k.keys}
}

func TestACardIsDrawnCentredOverThePage(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		card          laidCard
		// x and y are where its top left corner is, and rows how many it
		// takes.
		x, y, rows int
	}{
		{name: "with keys", width: 160, height: 37, card: laidCard{body: make([]line, 13), keys: []Key{{Key: "esc", Does: "cancel"}}}, x: 39, y: 9, rows: 18},
		{name: "without keys", width: 160, height: 37, card: laidCard{body: make([]line, 13)}, x: 39, y: 10, rows: 17},
		{name: "rounded down, at an odd size", width: 161, height: 40, card: laidCard{body: make([]line, 13), keys: []Key{{Key: "esc", Does: "cancel"}}}, x: 39, y: 11, rows: 18},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, tt.height)
			page := f.Draw(threeRouted(), now)
			f.Card = tt.card
			rows := f.Draw(threeRouted(), now)

			top, foot := []rune(rows[tt.y]), []rune(rows[tt.y+tt.rows-1])
			if got := string(top[tt.x : tt.x+27]); got != "╭─ ROUTING · NEW SESSIONS ─" || top[tt.x+81] != '╮' {
				t.Errorf("row %d = %q, want the card's top edge from column %d to %d", tt.y+1, rows[tt.y], tt.x+1, tt.x+82)
			}
			if foot[tt.x] != '╰' || foot[tt.x+81] != '╯' {
				t.Errorf("row %d = %q, want the card's foot from column %d to %d", tt.y+tt.rows, rows[tt.y+tt.rows-1], tt.x+1, tt.x+82)
			}
			for i := range rows {
				was, is := []rune(padded(page[i], tt.width)), []rune(padded(rows[i], tt.width))
				if i >= tt.y && i < tt.y+tt.rows {
					was, is = append(was[:tt.x:tt.x], was[tt.x+82:]...), append(is[:tt.x:tt.x], is[tt.x+82:]...)
				}
				if string(is) != string(was) {
					t.Errorf("row %d = %q, want the page's, %q, either side of the card", i+1, rows[i], page[i])
				}
			}
		})
	}
}

func TestACardsBodyRunsFromThreeCellsInAndItsKeysEndThere(t *testing.T) {
	f := frameOf(160, 37)
	f.Card = laidCard{body: []line{{{"Pick the accounts new sessions go to.", secondaryInk}}}, keys: []Key{{Key: "enter", Does: "apply"}, {Key: "esc", Does: "cancel"}}}
	rows := f.Draw(threeRouted(), now)

	y := (37 - 6) / 2
	want := map[int]string{
		y + 1: "│" + strings.Repeat(" ", 80) + "│",
		y + 2: "│  Pick the accounts new sessions go to." + strings.Repeat(" ", 41) + "│",
		y + 3: "│" + strings.Repeat(" ", 80) + "│",
		y + 4: "│" + strings.Repeat(" ", 54) + "enter apply   esc cancel  │",
	}
	for row, said := range want {
		if got := string([]rune(rows[row])[39:121]); got != said {
			t.Errorf("row %d reads %q, want %q: a blank row under its top edge and over its keys, its body from 3 cells in and its keys ending 3 cells in", row+1, got, said)
		}
	}
}

func TestACardWithAsMuchBodyAsItHasRoomForTakesAllButTheRowsSpare(t *testing.T) {
	f := frameOf(160, 37)
	for _, keys := range []bool{false, true} {
		k := laidCard{body: make([]line, f.bodyRoom(2, keys))}
		if keys {
			k.keys = []Key{{Key: "esc", Does: "cancel"}}
		}
		f.Card = k
		top, foot := -1, -1
		for i, row := range f.Draw(threeRouted(), now) {
			switch corners := []rune(padded(row, 160)); string(corners[39]) + string(corners[120]) {
			case "╭╮":
				top = i
			case "╰╯":
				foot = i
			}
		}
		if top != 1 || foot != 35 {
			t.Errorf("with keys %v, the card runs from row %d to %d, want 2 to 36: all but 2 of the frame's 37 rows", keys, top+1, foot+1)
		}
	}
}

func TestTheRoutingCardIsTitledAsWhatItRoutes(t *testing.T) {
	f := frameOf(160, 40)
	f.Card = RoutingCard{}
	for _, row := range f.Draw(threeRouted(), now) {
		if strings.Contains(row, "╭─ ROUTING · NEW SESSIONS ─") {
			return
		}
	}
	t.Error("no row draws the routing card's top edge, titled ROUTING · NEW SESSIONS")
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
