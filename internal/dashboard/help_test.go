package dashboard

import (
	"slices"
	"strings"
	"testing"
)

// sessionsKeys are the keys help lists on Sessions, as the key line words
// them, those it leaves out among them.
var sessionsKeys = []Key{
	tabViews, subTabs, {Key: "↑↓", Does: "session"}, {Key: "enter", Does: "its page"},
	{Key: "j k", Does: "scroll"}, {Key: "PgUp PgDn", Does: "page"}, {Key: "wheel", Does: "scroll"},
	pin, {Key: "t", Does: "themes"}, {Key: "R", Does: "refresh"}, helpQuit[0], helpQuit[1],
}

// helping is a frame of Sessions width by height, as text alone, its help
// open, listing sessionsKeys.
func helping(width, height int) Frame {
	f := switchboard(width, height, flippingSessions(), Traffic{})
	f.Card = Help{Keys: sessionsKeys}
	return f
}

// plainLines are the lines as text alone.
func plainLines(lines []line) []string {
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = l.plain()
	}
	return texts
}

func TestHelpListsThePagesKeysThenItsParts(t *testing.T) {
	f := helping(160, 40)
	card := f.Card.card(f)
	want := []string{
		"tab views   [ ] view   ↑↓ session   enter its page   j k scroll   PgUp PgDn page   wheel scroll   p pin   t themes",
		"R refresh   ? help   q quit",
		"",
		"●━━◉   a session's cord to its account",
		"○      a free jack",
		"╌╌○    a cord a move let go",
		"",
		"↑      asking",
		"↓      streaming back, ~ its tokens so far",
		"✕      a limit or a refusal",
		"…      throttled",
		"↪      moved here",
	}
	if got := plainLines(card.body); !slices.Equal(got, want) {
		t.Errorf("help reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if card.title != "HELP · SESSIONS" || card.width != helpWidth {
		t.Errorf("help is titled %q, %d cells wide; want HELP · SESSIONS, %d", card.title, card.width, helpWidth)
	}
	if got := keyRow(card.keys).plain(); got != "esc close" {
		t.Errorf("its keys row reads %q, want esc close", got)
	}
}

func TestHelpWordsItsKeysAsTheKeyLineDoesAndItsGlyphsAsTheViewDraws(t *testing.T) {
	f := helping(160, 40)
	f.Card = Help{Keys: []Key{tabViews, {Key: "p", Does: "pin", Off: true}}}
	body := f.Card.card(f).body
	if want := slices.Concat(tabViews.words(), line{spaces(keysApart)}, line{{"p pin", faintInk}}); !slices.Equal(body[0], want) {
		t.Errorf("its keys are drawn %v, want %v, as the key line draws them", body[0], want)
	}
	at := 1
	for _, group := range switchboardGlyphs() {
		at++
		for _, g := range group {
			if row := body[at]; !slices.Equal(row[:len(g.drawn)], g.drawn) || row[len(row)-1] != (span{g.means, secondaryInk}) {
				t.Errorf("%q is drawn %v, want %v then what it means in text.secondary", g.means, row, g.drawn)
			}
			at++
		}
	}
}

func TestHelpIsDrawnInTheCard(t *testing.T) {
	rows := helping(160, 40).Draw(threeRouted(), now)
	for _, want := range []string{"╭─ HELP · SESSIONS ─", "│  ●━━◉   a session's cord to its account", "│  ↓      streaming back, ~ its tokens so far", "esc close"} {
		if !slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, want) }) {
			t.Errorf("rows are\n%s\nwant %q among them", strings.Join(rows, "\n"), want)
		}
	}
}

func TestHelpOnATabNotBuiltListsItsKeysAlone(t *testing.T) {
	f := frameOf(160, 40)
	f.View, f.Card = Accounts, Help{Keys: []Key{tabViews, pin, helpQuit[0], helpQuit[1]}}
	if got, want := plainLines(f.Card.card(f).body), []string{"tab views   p pin   ? help   q quit"}; !slices.Equal(got, want) {
		t.Errorf("help on Accounts reads %q, want %q", got, want)
	}
}

func TestHelpsKeysWrapToTheCard(t *testing.T) {
	f := helping(60, 40)
	card := f.Card.card(f)
	keys := plainLines(card.body[:slices.IndexFunc(card.body, func(l line) bool { return len(l) == 0 })])
	want := []string{
		"tab views   [ ] view   ↑↓ session   enter its page",
		"j k scroll   PgUp PgDn page   wheel scroll   p pin",
		"t themes   R refresh   ? help   q quit",
	}
	if card.width != 58 || !slices.Equal(keys, want) {
		t.Errorf("at 60 columns, help is %d cells wide, its keys\n%s\nwant 58, its keys\n%s", card.width, strings.Join(keys, "\n"), strings.Join(want, "\n"))
	}
}

func TestHelpScrollsPastThePagesHeightLessTwoRows(t *testing.T) {
	full := helping(160, 40)
	all := plainLines(full.Card.card(full).body)
	tests := []struct {
		name          string
		scroll        int
		from          int
		above, below  int
		height, shown int
	}{
		{name: "from its top", scroll: 0, height: 14, shown: 8, from: 0, above: 0, below: 4},
		{name: "scrolled", scroll: 3, height: 14, shown: 8, from: 3, above: 3, below: 1},
		{name: "no further than its last row", scroll: 9, height: 14, shown: 8, from: 4, above: 4, below: 0},
		{name: "never above its top", scroll: -2, height: 14, shown: 8, from: 0, above: 0, below: 4},
		{name: "too short for any", scroll: 0, height: 5, shown: 0, from: 0, above: 0, below: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := helping(160, tt.height)
			f.Card = Help{Keys: sessionsKeys, Scroll: tt.scroll}
			card := f.Card.card(f)
			if got := plainLines(card.body); !slices.Equal(got, all[tt.from:tt.from+tt.shown]) {
				t.Errorf("help shows\n%s\nwant its rows from %d\n%s", strings.Join(got, "\n"), tt.from, strings.Join(all[tt.from:tt.from+tt.shown], "\n"))
			}
			if card.scroll != (cardScroll{above: tt.above, below: tt.below}) {
				t.Errorf("help is scrolled %+v, want %d above and %d below", card.scroll, tt.above, tt.below)
			}
			if got := keyRow(card.keys).plain(); got != "j k scroll   esc close" {
				t.Errorf("its keys row reads %q, want j k scroll   esc close while it scrolls", got)
			}
			if got := f.CardScrolling(); got != (Scrolling{Most: 12 - tt.shown, Page: tt.shown}) {
				t.Errorf("CardScrolling() = %+v, want it to scroll %d rows, %d at once", got, 12-tt.shown, tt.shown)
			}
		})
	}
}

func TestHelpIsNoTallerThanThePageLessTwoRows(t *testing.T) {
	for _, height := range []int{14, 16, 18} {
		rows := helping(160, height).Draw(threeRouted(), now)
		top := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "╭─ HELP · SESSIONS") })
		bottom := slices.IndexFunc(rows[max(top, 0):], func(r string) bool { return strings.Contains(r, "╰────") })
		if top < 0 || bottom < 0 || bottom+1 > height-2 {
			t.Errorf("at %d rows, help is %d rows tall from row %d, want %d at most", height, bottom+1, top+1, height-2)
		}
	}
}
