package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The pool's line says both windows' room as Runway's pool lane words it,
// the week's first, three cells apart, after Σ pool; pinned, · pinned after
// the first; and where it doesn't fit, whole parts go from the end, the
// 5-hour's first, then the pin, then the week's.
func TestGraphsPoolsLineSaysBothWindowsRoomDroppingWholePartsFromTheEnd(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  string
	}{
		{name: "whole", width: 160, want: "Σ pool   week 1.4 of 3 left   5-hour 1.3 of 3 left"},
		{name: "a cell short", width: 49, want: "Σ pool   week 1.4 of 3 left"},
		{name: "the week's a cell short", width: 26, want: "Σ pool"},
		{name: "pinned, whole", width: 160, want: "Σ pool   week 1.6 of 2 left · pinned   5-hour 0.5 of 2 left"},
		{name: "pinned, the 5-hour's gone", width: 58, want: "Σ pool   week 1.6 of 2 left · pinned"},
		{name: "pinned, the pin gone", width: 35, want: "Σ pool   week 1.6 of 2 left"},
	}
	f := graphsFrame(160, 37, GraphsSections{})
	for _, tt := range tests {
		doc := threeRouted()
		if strings.HasPrefix(tt.name, "pinned") {
			doc = changedRouted()
		}
		if got := f.poolLine(f.Moment(doc, now), tt.width).plain(); got != tt.want {
			t.Errorf("%s, %d cells: the pool's line reads %q, want %q", tt.name, tt.width, got, tt.want)
		}
	}
}

// The pool's line keeps its row, laid out by the config, whatever the
// document holds: blank where it has no pool, as with no account in it yet,
// or one, as usage's pool row says none then; and Σ pool alone where its
// accounts are none of them read yet, as Runway's pool lane has it.
func TestGraphsPoolsLineIsBlankWithoutAPoolAndBareOfUnreadAccounts(t *testing.T) {
	unread := func(id string) status.Account { return status.Account{ID: id, Label: id, TokenSet: true} }
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{name: "nothing read yet", doc: status.Document{}},
		{name: "one account", doc: routerDoc("work", 1, pressedAccount("work"))},
		{name: "three accounts, none read", doc: routerDoc("work", 0, unread("work"), unread("personal"), unread("side")), want: "Σ pool"},
	}
	for _, tt := range tests {
		f := graphsFrame(160, 37, GraphsSections{})
		text, row := textOf(f.Draw(tt.doc, now)), f.top()+f.graphsLaid().poolLine
		if row < f.top() || text[row] != tt.want || text[row+2] != "▆▆ room left   ⠂⠄⡀ where it's heading   ✕ runs out" {
			t.Errorf("%s: the pool's line's row reads %q, and the legend two under it %q; want the row kept, reading %q, over the legend", tt.name, text[row], text[row+2], tt.want)
		}
	}
}

// The pool's line is Σ in text.subtle and pool in text.secondary; each
// window's name in text.subtle, its room bold text.secondary, brightening as
// it changes, and " of 3 left" in text.muted.
func TestGraphsPoolsLineIsInkedAsTheDesignHasIt(t *testing.T) {
	f := graphsFrame(160, 37, GraphsSections{})
	l := f.poolLine(f.Moment(threeRouted(), now), 160)
	inked := func(text string) ink {
		i := slices.IndexFunc(l, func(s span) bool { return strings.TrimSpace(s.text) == text })
		if i < 0 {
			t.Fatalf("the pool's line %q has no %q", l.plain(), text)
		}
		return l[i].ink
	}
	for text, want := range map[string]ink{"Σ": dimInk, "pool": noteInk, "week": dimInk, "5-hour": dimInk, "of 3 left": mutedInk} {
		if got := inked(text); got != want {
			t.Errorf("the pool's line has %q in %+v, want %+v", text, got, want)
		}
	}
	if room := inked("1.4"); room != strongInk.numbered(Number{Kind: NumberRoom, Of: "7d"}) {
		t.Errorf("the week's room is in %+v, want bold text.secondary, numbered as the pool's week's room", room)
	}
}

// v0.1's legend: each mark in its ink, a lasting window's, and what it means
// in text.muted, three cells apart; the reserve's only where an account has
// one; flowed onto as many rows as it takes, each item whole, as at 52
// columns.
func TestGraphsLegendIsV01sFlowedWholeTheReserveOnlyWhereThereIsOne(t *testing.T) {
	room := line{{"▆▆", ink{token: theme.VizRamp1, fade: roomFade}}, {" room left", mutedInk}}
	heading := line{{"⠂⠄⡀", ink{token: theme.VizRamp1}}, {" where it's heading", mutedInk}}
	out := line{{"✕", outInk}, {" runs out", mutedInk}}
	reserve := line{{"⠠ ⠠", capDotInk}, {" reserve", mutedInk}}
	apart := spaces(3)
	tests := []struct {
		name     string
		width    int
		reserved bool
		want     []line
	}{
		{name: "a reserve, wide", width: 160, reserved: true, want: []line{joined(apart, room, heading, out, reserve)}},
		{name: "no reserve, wide", width: 160, want: []line{joined(apart, room, heading, out)}},
		{name: "a reserve, at 52", width: 52, reserved: true, want: []line{joined(apart, room, heading, out), reserve}},
		{name: "no reserve, at 52", width: 52, want: []line{joined(apart, room, heading, out)}},
		{name: "a reserve, at 30", width: 30, reserved: true, want: []line{room, heading, joined(apart, out, reserve)}},
	}
	for _, tt := range tests {
		if got := burnLegend(tt.width, tt.reserved); !slices.EqualFunc(got, tt.want, slices.Equal) {
			t.Errorf("%s: the legend is %q, want %q", tt.name, plains(got), plains(tt.want))
		}
	}
}

// The legend is the view's last rows, over the blank row before the key
// line, while a burn-down section shows; the rest of the rows over it blank.
func TestGraphsLegendIsTheViewsLastRowsWhileABurnDownShows(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		shown  GraphsSections
		legend []string
	}{
		{name: "at first, wide", width: 160, legend: []string{"▆▆ room left   ⠂⠄⡀ where it's heading   ✕ runs out   ⠠ ⠠ reserve"}},
		{name: "at first, at 52", width: 52, legend: []string{"▆▆ room left   ⠂⠄⡀ where it's heading   ✕ runs out", "⠠ ⠠ reserve"}},
		{name: "5-HOUR alone", width: 160, shown: GraphsSectionsNamed([]string{"5-hour"}), legend: []string{"▆▆ room left   ⠂⠄⡀ where it's heading   ✕ runs out   ⠠ ⠠ reserve"}},
		{name: "no burn-down", width: 160, shown: GraphsSectionsNamed([]string{"pool", "glance"})},
	}
	for _, tt := range tests {
		f := graphsFrame(tt.width, 80, tt.shown)
		f.Graphs.Accounts[0].Reserve = 0.05
		text := textOf(f.Draw(threeRouted(), now))
		foot := f.Height - underView
		if got := text[foot-len(tt.legend) : foot]; !slices.Equal(got, tt.legend) || text[foot] != "" {
			t.Errorf("%s: the view's last rows read %q and the state row %q, want the legend %q over a blank row", tt.name, got, text[foot], tt.legend)
		}
		if drawn, want := strings.Contains(strings.Join(text, "\n"), "room left"), len(tt.legend) > 0; drawn != want {
			t.Errorf("%s: the legend is drawn %t, want %t", tt.name, drawn, want)
		}
		spare := text[f.top()+f.graphsLaid().rows : foot-len(tt.legend)]
		if len(spare) == 0 || slices.ContainsFunc(spare, func(row string) bool { return row != "" }) {
			t.Errorf("%s: the rows between the sections and the legend read %q, want them blank", tt.name, spare)
		}
	}
}
