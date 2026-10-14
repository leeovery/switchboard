package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// overviewKeys are the keys help lists on the Overview, as the key line
// words them, those it leaves out among them.
func overviewKeys() []Key {
	keys := []Key{tabViews}
	for _, s := range EverySection() {
		keys = append(keys, Key{Key: s.Key(), Does: s.Word()})
	}
	return append(keys,
		Key{Key: "b", Does: "by"}, Key{Key: "↑↓", Does: "focus"},
		Key{Key: "j k", Does: "scroll"}, Key{Key: "PgUp PgDn", Does: "page"}, Key{Key: "wheel", Does: "scroll"},
		pin, Key{Key: "t", Does: "themes"}, Key{Key: "R", Does: "refresh"}, helpQuit[0], helpQuit[1],
	)
}

// helping is a frame of the Overview width by height, as text alone, its
// help open, listing overviewKeys.
func helping(width, height int) Frame {
	f := frameOf(width, height)
	f.Card = Help{Keys: overviewKeys()}
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
	f := helping(160, 400)
	card := f.Card.card(f)
	keys := []string{
		"tab views   u usage   g graphs   r routing   s sessions   c coming up   l lately   b by   ↑↓ focus   j k scroll",
		"PgUp PgDn page   wheel scroll   p pin   t themes   R refresh   ? help   q quit",
	}
	if got := plainLines(card.body[:len(keys)]); !slices.Equal(got, keys) {
		t.Errorf("help opens\n%s\nwant its keys\n%s", strings.Join(got, "\n"), strings.Join(keys, "\n"))
	}
	if got, want := plainLines(card.body[len(keys):]), plainLines(helpRows(overviewHelp(), helpWidth-2*cardInset)); !slices.Equal(got, want) {
		t.Errorf("after its keys, help reads\n%s\nwant its parts\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if card.title != "HELP · OVERVIEW" || card.width != helpWidth {
		t.Errorf("help is titled %q, %d cells wide; want HELP · OVERVIEW, %d", card.title, card.width, helpWidth)
	}
	if got := keyRow(card.keys).plain(); got != "esc close" {
		t.Errorf("its keys row reads %q, want esc close", got)
	}
}

func TestHelpWordsItsKeysAsTheKeyLineDoesAndItsGlyphsAsTheViewDraws(t *testing.T) {
	f := helping(160, 400)
	f.Card = Help{Keys: []Key{tabViews, {Key: "p", Does: "pin", Off: true}}}
	body := f.Card.card(f).body
	if want := slices.Concat(tabViews.words(), line{spaces(keysApart)}, line{{"p pin", faintInk}}); !slices.Equal(body[0], want) {
		t.Errorf("its keys are drawn %v, want %v, as the key line draws them", body[0], want)
	}
	for _, part := range overviewHelp() {
		for _, g := range part.glyphs {
			drawn := slices.ContainsFunc(body, func(row line) bool {
				return len(row) > len(g.drawn) && slices.Equal(row[:len(g.drawn)], g.drawn) && strings.HasPrefix(g.means, row[len(row)-1].text) && row[len(row)-1].ink == secondaryInk
			})
			if !drawn {
				t.Errorf("%q isn't drawn %v then what it means in text.secondary", g.means, g.drawn)
			}
		}
	}
}

func TestHelpIsDrawnInTheCard(t *testing.T) {
	rows := helping(160, 40).Draw(threeRouted(), now)
	for _, want := range []string{"╭─ HELP · OVERVIEW ─", "│  tab views   u usage", "│  USAGE", "esc close"} {
		if !slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, want) }) {
			t.Errorf("rows are\n%s\nwant %q among them", strings.Join(rows, "\n"), want)
		}
	}
}

func TestHelpsKeysWrapToTheCard(t *testing.T) {
	f := helping(60, 40)
	card := f.Card.card(f)
	keys := plainLines(card.body[:slices.IndexFunc(card.body, func(l line) bool { return len(l) == 0 })])
	want := []string{
		"tab views   u usage   g graphs   r routing",
		"s sessions   c coming up   l lately   b by",
		"↑↓ focus   j k scroll   PgUp PgDn page",
		"wheel scroll   p pin   t themes   R refresh   ? help",
		"q quit",
	}
	if card.width != 58 || !slices.Equal(keys, want) {
		t.Errorf("at 60 columns, help is %d cells wide, its keys\n%s\nwant 58, its keys\n%s", card.width, strings.Join(keys, "\n"), strings.Join(want, "\n"))
	}
}

func TestHelpScrollsPastThePagesHeightLessTwoRows(t *testing.T) {
	full := helping(160, 400)
	all := plainLines(full.Card.card(full).body)
	n := len(all)
	tests := []struct {
		name          string
		scroll        int
		from          int
		above, below  int
		height, shown int
	}{
		{name: "from its top", scroll: 0, height: 14, shown: 7, from: 0, above: 0, below: n - 7},
		{name: "scrolled", scroll: 3, height: 14, shown: 7, from: 3, above: 3, below: n - 10},
		{name: "no further than its last row", scroll: n + 2, height: 14, shown: 7, from: n - 7, above: n - 7, below: 0},
		{name: "never above its top", scroll: -2, height: 14, shown: 7, from: 0, above: 0, below: n - 7},
		{name: "too short for any", scroll: 0, height: 5, shown: 0, from: 0, above: 0, below: n},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := helping(160, tt.height)
			f.Card = Help{Keys: overviewKeys(), Scroll: tt.scroll}
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
			if got := f.CardScrolling(); got != (Scrolling{Most: n - tt.shown, Page: tt.shown}) {
				t.Errorf("CardScrolling() = %+v, want it to scroll %d rows, %d at once", got, n-tt.shown, tt.shown)
			}
		})
	}
}

func TestHelpIsNoTallerThanThePageLessTwoRows(t *testing.T) {
	for _, height := range []int{14, 16, 18} {
		rows := helping(160, height).Draw(threeRouted(), now)
		top := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "╭─ HELP · OVERVIEW") })
		bottom := slices.IndexFunc(rows[max(top, 0):], func(r string) bool { return strings.Contains(r, "╰────") })
		if top < 0 || bottom < 0 || bottom+1 > height-2 {
			t.Errorf("at %d rows, help is %d rows tall from row %d, want %d at most", height, bottom+1, top+1, height-2)
		}
	}
}

// overviewHelping is the Overview's help, open at width columns, tall
// enough to show it whole, listing keys.
func overviewHelping(width int, keys ...Key) Card {
	f := frameOf(width, 400)
	f.Card = Help{Keys: keys}
	return f.Card.card(f)
}

// helpGlyphs are the glyphs the Overview's help explains under the section
// headed heading.
func helpGlyphs(heading string) []glyph {
	parts := overviewHelp()
	if i := slices.IndexFunc(parts, func(p helpPart) bool { return p.heading == heading }); i >= 0 {
		return parts[i].glyphs
	}
	return nil
}

func TestOverviewHelpListsItsKeysThenEachSectionInThePagesOrder(t *testing.T) {
	card := overviewHelping(160, tabViews, helpQuit[0], helpQuit[1])
	if card.title != "HELP · OVERVIEW" {
		t.Errorf("help is titled %q, want HELP · OVERVIEW", card.title)
	}
	if got := plainLines(card.body[:2]); !slices.Equal(got, []string{"tab views   ? help   q quit", ""}) {
		t.Errorf("help opens %q, want its keys, then a blank row", got)
	}
	var headings, want []string
	for i, row := range card.body[1:] {
		if len(card.body[i]) == 0 && len(row) == 1 && row[0].ink == dimInk {
			headings = append(headings, row[0].text)
		}
	}
	for _, s := range EverySection() {
		want = append(want, s.heading())
	}
	if !slices.Equal(headings, want) {
		t.Errorf("help explains %q, want each section in the page's order, %q, each headed in text.subtle after a blank row", headings, want)
	}
}

func TestOverviewHelpKeysEachMarkAsThePageDrawsIt(t *testing.T) {
	explains := func(t *testing.T, heading string, drawn line) {
		t.Helper()
		if !slices.ContainsFunc(helpGlyphs(heading), func(g glyph) bool { return slices.Equal(g.drawn, drawn) }) {
			t.Errorf("%s's help explains %v, want %v among them, as the page draws it", heading, helpGlyphs(heading), drawn)
		}
	}
	t.Run("USAGE's marks, as its key line names them", func(t *testing.T) {
		explains(t, "USAGE", line{{paceMark, paceInk}})
		explains(t, "USAGE", line{{capMark, reserveInk}})
		for _, mark := range keyMarks([]accountName{{primary: true}, {best: true}}, false) {
			explains(t, "USAGE", mark[:1])
		}
	})
	t.Run("GRAPHS' lines, as its charts draw them", func(t *testing.T) {
		for _, drawn := range []line{{{paceDot, dimInk}}, {{capLine, reserveInk}}, {{limitLine, faintInk}}, {{nowLine, faintInk}}} {
			explains(t, "GRAPHS", drawn)
		}
	})
	t.Run("every state's glyph, in its colour, and the pin's", func(t *testing.T) {
		for kind := status.RoutingTokenless; kind <= status.RoutingOpen; kind++ {
			explains(t, "ROUTING", line{{kind.Glyph(), standingWords(status.Routing{Kind: kind, Says: "says"}, false, 80)[0].ink}})
		}
		explains(t, "ROUTING", line{pinnedSpan})
	})
	t.Run("what a session is doing", func(t *testing.T) {
		explains(t, "SESSIONS", line{runningDoing(views.Activity{Doing: views.Asking})})
		explains(t, "SESSIONS", line{runningDoing(views.Activity{Doing: views.Answering, Chars: 4800})})
	})
	t.Run("every event's mark", func(t *testing.T) {
		for _, k := range eventKinds {
			t.Run(k.kind, func(t *testing.T) { explains(t, "LATELY", line{k.mark}) })
		}
	})
}

func TestOverviewHelpGivesTheTracesScale(t *testing.T) {
	said := strings.Join(plainLines(overviewHelping(160).body), " ")
	if want := "never less than " + strconv.Itoa(leastScale) + " requests a minute"; !strings.Contains(said, want) {
		t.Errorf("help reads\n%s\nwant it to say a trace's height is %q", said, want)
	}
}

func TestOverviewHelpFitsTheCardAtEveryWidth(t *testing.T) {
	for _, width := range []int{216, 160, 100, 52} {
		card := overviewHelping(width, tabViews, helpQuit[0], helpQuit[1])
		inside := card.width - 2*cardInset
		for i, row := range card.body {
			if row.width() > inside || strings.Contains(row.plain(), prose.Ellipsis) {
				t.Errorf("at %d columns, help's row %d is %q, %d cells; want it whole within %d", width, i+1, row.plain(), row.width(), inside)
			}
		}
	}
}

func TestHelpWrapsWhatItSaysToTheCardAndAGlyphsMeaningUnderItself(t *testing.T) {
	parts := []helpPart{{
		heading: "PART",
		says:    []line{said("words that wrap at twenty cells, `k` bold")},
		glyphs: []glyph{
			{drawn: line{{"●━◉", positiveInk}}, means: "a meaning that wraps"},
			{drawn: line{{"○", dimInk}}, means: "fits"},
		},
	}}
	want := []string{
		"",
		"PART",
		"words that wrap at",
		"twenty cells, k bold",
		"●━◉   a meaning that",
		"      wraps",
		"○     fits",
	}
	rows := helpRows(parts, 20)
	if got := plainLines(rows); !slices.Equal(got, want) {
		t.Fatalf("help's rows read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if k := rows[3][slices.IndexFunc(rows[3], func(s span) bool { return s.text == "k" })]; k.ink != keyInk {
		t.Errorf("the key in what it says is drawn %v, want bold accent.key", k.ink)
	}
	if got := rows[6]; !slices.Equal(got, line{{"○", dimInk}, spaces(5), {"fits", secondaryInk}}) {
		t.Errorf("a meaning that fits is drawn %v, want its glyph, then the meaning in text.secondary, in the glyphs' column", got)
	}
}
