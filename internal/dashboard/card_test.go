package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

func TestACardIsItsNaturalWidthOrTheScreensLessACellEitherSide(t *testing.T) {
	tests := []struct {
		name           string
		width, natural int
		want           int
		whole          bool
	}{
		{name: "the routing card, with room", width: 160, natural: routingWidth, want: 82, whole: true},
		{name: "the routing card, a cell clear either side", width: 84, natural: routingWidth, want: 82, whole: true},
		{name: "the routing card, a cell short", width: 83, natural: routingWidth, want: 81},
		{name: "the routing card on a phone, its width less two", width: 52, natural: routingWidth, want: 50},
		{name: "help, with room", width: 216, natural: helpWidth, want: 128, whole: true},
		{name: "help, short of room", width: 120, natural: helpWidth, want: 118},
		{name: "a screen of two cells, no card", width: 2, natural: routingWidth, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, whole := frameOf(tt.width, 40).cardOn(tt.natural); got != tt.want || whole != tt.whole {
				t.Errorf("the card is %d cells wide, whole %v; want %d, %v", got, whole, tt.want, tt.whole)
			}
			if got := frameOf(tt.width, 40).cardWidth(tt.natural); got != tt.want {
				t.Errorf("cardWidth gives %d cells, want %d, as cardOn has it", got, tt.want)
			}
		})
	}
}

// laidCard is a card of a body of the lines given, with keys or none, its
// cursor on the rows given, as wide as the routing card.
type laidCard struct {
	body []line
	keys []Key
	lit  bodyRows
}

func (k laidCard) card(f Frame) Card {
	return Card{title: "ROUTING · NEW SESSIONS", width: f.cardWidth(routingWidth), body: k.body, keys: k.keys, lit: k.lit}
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

// A card whose body is taller than the page has room for is as tall as the
// page: its title in its top edge on the first row, its keys and its foot on
// the last two, and its body cut above them, its rows from its top as many
// as fit, the cursor's light with them, never over its keys.
func TestACardTallerThanThePageKeepsItsKeysAndCutsItsBody(t *testing.T) {
	f := frameOf(160, 12)
	body := make([]line, 13)
	for i := range body {
		body[i] = line{{fmt.Sprintf("row %d", i+1), secondaryInk}}
	}
	f.Card = laidCard{body: body, keys: []Key{{Key: "esc", Does: "cancel"}}, lit: bodyRows{first: 6, n: 2}}
	c := newCanvas(f.Width, f.Height)
	f.Card.card(f).draw(c)
	rows := textOf(c.rows(Look{}))

	if got := string([]rune(rows[0])[39:66]); got != "╭─ ROUTING · NEW SESSIONS ─" {
		t.Errorf("row 1 reads %q, want the card's top edge, its title in it", rows[0])
	}
	if got := strings.TrimSpace(string([]rune(rows[2])[42:])); !strings.HasPrefix(got, "row 1") {
		t.Errorf("row 3 reads %q, want the body's first row", rows[2])
	}
	if got := strings.TrimSpace(string([]rune(rows[8])[42:])); !strings.HasPrefix(got, "row 7") {
		t.Errorf("row 9 reads %q, want the last of the body's rows that fit, row 7", rows[8])
	}
	if !strings.Contains(rows[10], "esc cancel") || !strings.HasPrefix(strings.TrimSpace(rows[11]), "╰") {
		t.Errorf("its last rows read %q and %q, want its keys and its foot", rows[10], rows[11])
	}
	for y := range f.Height {
		if lit := c.at(60, y).ink.on == (hue{token: theme.BgSelection}); lit != (y == 8) {
			t.Errorf("row %d is lit: %v, want the cursor's rows lit only where they show, row 9", y+1, lit)
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

// A card of a line placed by it sits where it's placed; one of no line, or
// of one it couldn't be placed by, is centred.
func TestACardPlacedByItsLineSitsThere(t *testing.T) {
	tests := []struct {
		name string
		of   *ofLine
		want int
	}{
		{name: "of no line", want: 9},
		{name: "placed by its line", of: &ofLine{row: 5, top: 7, placed: true}, want: 7},
		{name: "of a line it couldn't be placed by", of: &ofLine{row: 5}, want: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := Card{width: 20, body: make([]line, 3), of: tt.of}
			if _, y, _, _ := k.box(40, 25); y != tt.want {
				t.Errorf("its top edge is on row %d, want %d", y+1, tt.want+1)
			}
		})
	}
}

func TestNoCardScrollsWhileNoneIsOpen(t *testing.T) {
	if got := frameOf(160, 40).CardScrolling(); got != (Scrolling{}) {
		t.Errorf("with no card open, the card scrolls %+v, want nothing", got)
	}
}

// widerForms are each card's and panel's forms, and the printout's
// header's and the picker's, by the frame's width: whether it's in its
// wider form there, and the width that form starts at, where its parts fit.
var widerForms = []struct {
	name  string
	wider func(width int) bool
	from  int
}{
	{name: "the routing card's rows on one line", wider: func(width int) bool {
		return !slices.ContainsFunc(plains(routedCard(nil).card(frameOf(width, 40)).body), func(row string) bool {
			return strings.HasPrefix(strings.TrimSpace(row), "week ")
		})
	}, from: 81},
	{name: "the Log's card in its columns", wider: func(width int) bool {
		return logFrameOf(LogPage{}, width, 37).logCardForm().wide
	}, from: 130},
	{name: "the Log's panel in its two sides", wider: func(width int) bool {
		return strings.Contains(strings.Join(bodyText(framesPanel(LogRequests, 0), frameOf(width, 40)), "\n"), "ONLY")
	}, from: 126},
	{name: "usage's header on one line", wider: func(width int) bool {
		return strings.HasPrefix(strings.TrimSpace(printed(trio(), width, ByWindow)[2]), "WEEK")
	}, from: 89},
	{name: "the theme picker at its widest", wider: func(width int) bool {
		return PickerWidth(width) == pickerWide
	}, from: 2 * pickerWide},
}

func TestEachCardsFormSwitchesOnceAsTheWidthGrows(t *testing.T) {
	for _, tt := range widerForms {
		t.Run(tt.name, func(t *testing.T) {
			for width := 40; width <= 220; width++ {
				if got, want := tt.wider(width), width >= tt.from; got != want {
					t.Errorf("at %d columns, in its wider form: %v; want %v, from %d alone", width, got, want, tt.from)
				}
			}
		})
	}
}

func TestTheRoutingCardsRowsAreStackedAlikeWhateverTheySay(t *testing.T) {
	cards := map[string]RoutingCard{
		"the frames' world":          routedCard(nil),
		"every account at its limit": {Rows: RoutingRows(routerDoc("", 0, namedAccounts(3)...).At(now, claudeLike), nil, views.Stream{})},
		"a long reason": routedCard(func(r *RoutingCard) {
			r.Rows[1].Open, r.Rows[1].Reason = false, "token refused: "+strings.Repeat("why ", 20)
		}),
	}
	for width := 40; width <= 220; width++ {
		heights := map[string]int{}
		for name, r := range cards {
			k := r.card(frameOf(width, 60))
			heights[name] = len(r.choice(r.Rows[1], choicesStacked(k.width-2*cardInset), k.width-2*cardInset))
		}
		if len(slices.Compact(slices.Sorted(maps.Values(heights)))) != 1 {
			t.Errorf("at %d columns, an account's row takes %v lines; want the same whatever the rows say", width, heights)
		}
	}
}

func TestTheRoutingCardNeverCutsAWindowsUse(t *testing.T) {
	r := routedCard(nil)
	for width := 40; width <= 220; width++ {
		k := r.card(frameOf(width, 60))
		body := strings.Join(plains(k.body), "\n")
		for _, use := range []string{"week 34%   5-hour 95%", "week 66%   5-hour 39%", "week 78%   5-hour 13%"} {
			if !strings.Contains(body, use) {
				t.Errorf("at %d columns, the card reads\n%s\nwant %q whole", width, body, use)
			}
		}
		if !choicesStacked(k.width-2*cardInset) && !strings.Contains(body, "at its cap till 17:10") {
			t.Errorf("at %d columns, an account's row is one line, and reads\n%s\nwant its reason whole beside its windows", width, body)
		}
	}
}
