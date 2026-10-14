package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// logHelpPart is the part of the Log's help headed heading: false where
// there's none.
func logHelpPart(heading string) (helpPart, bool) {
	parts := Frame{View: Log}.logHelp()
	if i := slices.IndexFunc(parts, func(p helpPart) bool { return p.heading == heading }); i >= 0 {
		return parts[i], true
	}
	return helpPart{}, false
}

func TestLogHelpListsItsKeysThenEachPartInTheDesignsOrder(t *testing.T) {
	f := frameOf(160, 400)
	f.View = Log
	card := Help{Keys: []Key{tabViews, helpQuit[0], helpQuit[1]}}.card(f)
	if card.title != "HELP · LOG" {
		t.Errorf("help is titled %q, want HELP · LOG", card.title)
	}
	if got := plainLines(card.body[:2]); !slices.Equal(got, []string{"tab views   ? help   q quit", ""}) {
		t.Errorf("help opens %q, want its keys, then a blank row", got)
	}
	var headings []string
	for i, row := range card.body[1:] {
		if len(card.body[i]) == 0 && len(row) == 1 && row[0].ink == dimInk {
			headings = append(headings, row[0].text)
		}
	}
	want := []string{"REQUESTS", "EVENTS", "COLUMNS", "FIND", "CHECKS AND COUNTS", "WORTH AT API PRICES", "NEVER KEPT"}
	if !slices.Equal(headings, want) {
		t.Errorf("help explains %q, want %q, each headed in text.subtle after a blank row", headings, want)
	}
}

// Every mark a request's row can lead with is explained once, drawn as the
// table draws it, in the order the table picks the first that applies; and
// the event kinds' marks as LATELY's help explains them.
func TestLogHelpExplainsEachMarkAsTheTableDrawsIt(t *testing.T) {
	requests, _ := logHelpPart("REQUESTS")
	var got []line
	for _, g := range requests.glyphs {
		got = append(got, g.drawn)
	}
	var want []line
	for m := views.MarkAsking; m <= views.MarkHidden; m++ {
		want = append(want, line{{m.String(), rowMarkInks[m]}})
	}
	if !slices.EqualFunc(got, want, func(a, b line) bool { return slices.Equal(a, b) }) {
		t.Errorf("Requests' help explains the marks %v, want %v", got, want)
	}
	events, _ := logHelpPart("EVENTS")
	if !slices.EqualFunc(events.glyphs, latelyGlyphs(), func(a, b glyph) bool { return slices.Equal(a.drawn, b.drawn) && a.means == b.means }) {
		t.Errorf("Events' help explains %v, want LATELY's marks, %v", events.glyphs, latelyGlyphs())
	}
}

// Every column is explained by its head, drawn in text.subtle as the heads
// are, with what it holds: those shown at first in the table's order, then
// the extras f shows.
func TestLogHelpExplainsEveryColumnByItsHead(t *testing.T) {
	columns, _ := logHelpPart("COLUMNS")
	var heads []string
	for _, g := range columns.glyphs {
		if len(g.drawn) != 1 || g.drawn[0].ink != dimInk || g.means == "" {
			t.Errorf("the column %v is explained as %q, want its head in text.subtle and what it holds", g.drawn, g.means)
		}
		heads = append(heads, g.drawn.plain())
	}
	want := []string{
		"at", "session", "model", "on", "status", "first", "took", "read", "written", "out", "stopped", "routing",
		"worth", "input", "attempts", "size", "messages", "tools", "thinking", "agent", "request id",
	}
	if !slices.Equal(heads, want) {
		t.Errorf("help explains the columns %q, want %q", heads, want)
	}
}

// Help says what the ledger never keeps, in the words a request's details
// say it.
func TestLogHelpSaysWhatTheLedgerNeverKeeps(t *testing.T) {
	never, _ := logHelpPart("NEVER KEPT")
	if said := plainLines(never.says); len(said) != 1 || !strings.Contains(said[0], ledgerNeverKeeps) {
		t.Errorf("help says %q, want what the ledger never keeps, %q", said, ledgerNeverKeeps)
	}
}
