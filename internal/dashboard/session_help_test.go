package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

// sessionHelping is help open over a session's page, opened from the
// Overview, at width columns, tall enough to show it whole, listing keys.
func sessionHelping(width int, keys ...Key) Card {
	f := frameOf(width, 400)
	f.Session = &SessionShown{}
	f.Card = Help{Keys: keys}
	return f.Card.card(f)
}

func TestSessionHelpListsItsKeysThenEachPartInThePagesOrder(t *testing.T) {
	back := Key{Key: "esc", Does: "back"}
	card := sessionHelping(160, back, helpQuit[0], helpQuit[1])
	if card.title != "HELP · SESSION" {
		t.Errorf("help over a session's page opened from the Overview is titled %q, want HELP · SESSION", card.title)
	}
	if got := plainLines(card.body[:2]); !slices.Equal(got, []string{"esc back   ? help   q quit", ""}) {
		t.Errorf("help opens %q, want its keys, then a blank row", got)
	}
	var headings []string
	for i, row := range card.body[1:] {
		if len(card.body[i]) == 0 && len(row) == 1 && row[0].ink == dimInk {
			headings = append(headings, row[0].text)
		}
	}
	if want := []string{"STORY", "TURNS"}; !slices.Equal(headings, want) {
		t.Errorf("help explains %q, want %q, each headed in text.subtle after a blank row, the accounts between them as the page has them, unheaded", headings, want)
	}
}

func TestSessionHelpExplainsTheStorysScaleTurnsPointsWorthAndCachedAgain(t *testing.T) {
	said := strings.Join(plainLines(sessionHelping(160).body), " ")
	for _, want := range []string{
		"the busiest column fills all five rows, never less than " + strconv.Itoa(leastScale) + " requests a minute",
		"how many it cached again, and their worth at API prices",
		"A turn is a prompt and every request that serves it",
		"≈ points are an estimate of what the session took of a window",
		"Worth is what the same requests would cost through the API, at today's prices",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("help reads\n%s\nwant it to say %q", said, want)
		}
	}
}

func TestSessionHelpKeysTheStorysMarksAsTheDesignDrawsThem(t *testing.T) {
	var drawn []line
	for _, part := range sessionHelp() {
		for _, g := range part.glyphs {
			drawn = append(drawn, g.drawn)
		}
	}
	for _, want := range []line{
		{{"┊", warningInk}},
		{{"╱ ╱", faintInk}},
		{{"▁", faintInk}},
		{{"├─", faintInk}, {" side ", secondaryInk}, {"─┤", faintInk}},
		{{"↑ asking", keyInk}},
	} {
		if !slices.ContainsFunc(drawn, func(l line) bool { return slices.Equal(l, want) }) {
			t.Errorf("help keys %v, want %v among them", drawn, want)
		}
	}
}

func TestSessionHelpFitsTheCardAtEveryWidth(t *testing.T) {
	for _, width := range []int{216, 160, 100, 52} {
		card := sessionHelping(width, tabViews, helpQuit[0], helpQuit[1])
		inside := card.width - 2*cardInset
		for i, row := range card.body {
			if row.width() > inside || strings.Contains(row.plain(), prose.Ellipsis) {
				t.Errorf("at %d columns, help's row %d is %q, %d cells; want it whole within %d", width, i+1, row.plain(), row.width(), inside)
			}
		}
	}
}
