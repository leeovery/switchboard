package dashboard

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// A session's page scrolls its turns a row at a time, in the rows its
// layout gives them, whatever tab it's open over; and what's under the
// pointer scrolls over those rows alone.
func TestASessionsPageScrollsItsTurnsInTheirRows(t *testing.T) {
	tests := []struct {
		name  string
		turns int
		want  Scrolling
	}{
		{name: "twenty turns in eleven rows", turns: 20, want: Scrolling{Most: 9, Page: 11}},
		{name: "as many as fit", turns: 11, want: Scrolling{Page: 11}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 37)
			f.Session = &SessionShown{Page: views.SessionPage{Accounts: []views.SessionAccount{{Account: "work"}}, Turns: make([]views.Turn, tt.turns)}}
			if got := f.Scrolling(status.Document{}, routedAt); got != tt.want || !f.ScrollsRows(status.Document{}, routedAt) {
				t.Errorf("its turns scroll %+v, rows at a time %t; want %+v, a row at a time", got, f.ScrollsRows(status.Document{}, routedAt), tt.want)
			}
			for y, want := range map[int]Under{23: UnderNothing, 24: UnderView, 34: UnderView, 35: UnderNothing} {
				if got := f.Under(status.Document{}, routedAt, 10, y); got != want {
					t.Errorf("row %d is under %v, want %v: the turns from row 24, eleven of them", y, got, want)
				}
			}
		})
	}
}

func TestRevealingScrollsNoFurtherThanItMustToShowTheAccount(t *testing.T) {
	ten := Scrolling{Most: 6, Page: 4}
	tests := []struct {
		name  string
		at, i int
		s     Scrolling
		want  int
	}{
		{name: "shown already, the first of the page: where it is", at: 2, i: 2, s: ten, want: 2},
		{name: "shown already, the last of the page: where it is", at: 2, i: 5, s: ten, want: 2},
		{name: "below the page: till it's the last shown", at: 0, i: 6, s: ten, want: 3},
		{name: "the last account: to the foot", at: 0, i: 9, s: ten, want: 6},
		{name: "above the page: till it's the first shown", at: 5, i: 1, s: ten, want: 1},
		{name: "none of them: where it is", at: 3, i: -1, s: ten, want: 3},
		{name: "scrolled past the foot, as the frame now has it: from the foot", at: 9, i: 7, s: ten, want: 6},
		{name: "nothing scrolls: the top", at: 2, i: 2, s: Scrolling{}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.revealing(tt.at, tt.i); got != tt.want {
				t.Errorf("%+v.revealing(%d, %d) = %d, want %d", tt.s, tt.at, tt.i, got, tt.want)
			}
		})
	}
}

// Each scroll line the design gives is a form of outOfView, said in the inks
// the design gives it, else in text.subtle, as a scroll line is (~991):
// USAGE's rows (~1564), the Switchboard's accounts, SESSIONS' sessions
// (~1685, ~1917), Days' accounts (~2162), Tokens' periods (~2191) and the
// routing card's rows (~3671), as they're drawn, SESSIONS' leading with + in
// place of its mark; and
// Compare's accounts on a phone (~1838), a session's turns (~3383), a turn's
// requests (~3403) and the Log's sessions to pick from (~3004), in forms of
// their own.
func TestEachScrollLineTheDesignGivesIsAFormOfOutOfView(t *testing.T) {
	key, words, apart, keys, subtle := ink{token: theme.AccentKey}, ink{token: theme.TextMuted}, ink{token: theme.TextFaint}, ink{token: theme.AccentKey, bold: true}, ink{token: theme.TextSubtle}
	page := outInks{mark: keyMarkInk, space: mutedInk, count: mutedInk, gap: mutedInk, keys: keyInk, does: mutedInk}
	tests := []struct {
		name         string
		above, below int
		forms        []outForm
		want         line
	}{
		{name: "USAGE's rows", above: 3, below: 12, forms: []outForm{usageRowsOut}, want: line{{"▲ 3 more above · ▼ 12 more below", subtle}}},
		{name: "the Switchboard's accounts, below", below: 4, forms: []outForm{boardOut}, want: line{{"▼ 4 more accounts", subtle}}},
		{name: "the Switchboard's accounts, above", above: 2, forms: []outForm{boardOut}, want: line{{"▲ 2 more accounts", subtle}}},
		{name: "SESSIONS' sessions", below: 2, forms: []outForm{runningOut}, want: line{{"+2 more", subtle}}},
		{
			name: "Days' accounts", below: 4, forms: historyAccountsOut,
			want: line{{"▼", key}, {" 4 more accounts below", words}, {"   ·   ", apart}, {"j/k", keys}, {" or the wheel to scroll them", words}},
		},
		{
			name: "Tokens' weeks", below: 3, forms: []outForm{periodsOut("week")},
			want: line{{"▼", key}, {" 3 more weeks below", words}, {"   ·   ", apart}, {"j/k", keys}, {" to scroll", words}},
		},
		{name: "the routing card's rows, below", below: 3, forms: []outForm{cardOut}, want: line{{"▼ 3 more", subtle}}},
		{name: "the routing card's rows, above", above: 2, forms: []outForm{cardOut}, want: line{{"▲ 2 more", subtle}}},
		{
			name: "Compare's accounts on a phone", below: 2, forms: []outForm{{below: " below", gap: " · ", keys: "j/k", inks: subtleOut}},
			want: line{{"▼ 2 more below · j/k", subtle}},
		},
		{
			name: "a session's turns", below: 3, forms: []outForm{turnsOut},
			want: line{{"▼", key}, {" 3 more turns   ·   ", words}, {"j/k", keys}, {" to scroll", words}},
		},
		{
			name: "a turn's requests", below: 32, forms: []outForm{requestsOut},
			want: line{{"▼", key}, {" 32 more of its requests   ·   ", words}, {"j/k", keys}, {" through them", words}},
		},
		{
			name: "a turn's requests, either way", above: 2, below: 1, forms: []outForm{requestsOut},
			want: line{{"▲", key}, {" 2 more of its requests   ·   ", words}, {"▼", key}, {" 1 more of its requests   ·   ", words}, {"j/k", keys}, {" through them", words}},
		},
		{
			name: "the Log's sessions to pick from", below: 8, forms: []outForm{{below: " today", gap: "   ", keys: "/", does: " finds one from another day", inks: page}},
			want: line{{"▼", key}, {" 8 more today   ", words}, {"/", keys}, {" finds one from another day", words}},
		},
	}
	for _, tt := range tests {
		if got := outOfView(tt.above, tt.below, 160, tt.forms...); !slices.Equal(got, tt.want) {
			t.Errorf("%s reads %q, %+v, want %q, %+v", tt.name, got.plain(), got, tt.want.plain(), tt.want)
		}
	}
}

// Where nothing is out of view, outOfView says nothing, in any form.
func TestOutOfViewSaysNothingWhereNothingIsOutOfView(t *testing.T) {
	for _, forms := range [][]outForm{historyAccountsOut, {periodsOut("week")}, {usageRowsOut}, {boardOut}, {runningOut}, {cardOut}} {
		if got := outOfView(0, 0, 160, forms...); got != nil {
			t.Errorf("with nothing out of view, %+v says %q, want nothing", forms[0], got.plain())
		}
	}
}

// Every scroll line says what's out of view by one rule, at every width
// from 20 columns to 220: both counts wherever both fit, the keys only where
// they fit beside both, else one count, with its keys where they fit; cut
// only where no form fits whole; and never saying less as the width grows.
// Forms without a gap between their counts only ever count one way.
func TestEveryScrollLineCountsBothWaysWhereverBothFit(t *testing.T) {
	callers := []struct {
		name   string
		forms  []outForm
		oneWay bool
	}{
		{name: "a session's turns", forms: []outForm{turnsOut}},
		{name: "a turn's requests", forms: []outForm{requestsOut}},
		{name: "Compare's and Detail's panels", forms: []outForm{compareOut}},
		{name: "USAGE's rows", forms: []outForm{usageRowsOut}},
		{name: "History's rows", forms: historyRowsOut},
		{name: "Weeks' and Days' accounts", forms: historyAccountsOut},
		{name: "Detail's list", forms: listOut},
		{name: "Runway's lanes", forms: lanesOut},
		{name: "Tokens' periods", forms: []outForm{periodsOut("week")}},
		{name: "the Switchboard's accounts", forms: []outForm{boardOut}, oneWay: true},
		{name: "the routing card's rows", forms: []outForm{cardOut}, oneWay: true},
		{name: "a session's accounts", forms: []outForm{accountsOut}, oneWay: true},
		{name: "SESSIONS' sessions", forms: []outForm{runningOut}, oneWay: true},
		{name: "Runway's coming", forms: []outForm{comingOut}, oneWay: true},
	}
	var counts [][2]int
	for _, n := range [][2]int{{1, 3}, {12, 23}, {123, 456}} {
		counts = append(counts, n, [2]int{0, n[1]}, [2]int{n[0], 0})
	}
	for _, tt := range callers {
		for _, n := range counts {
			if tt.oneWay && n[0] > 0 && n[1] > 0 {
				continue
			}
			t.Run(fmt.Sprintf("%s, %d above and %d below", tt.name, n[0], n[1]), func(t *testing.T) {
				holdsOutOfViewsRule(t, tt.forms, n[0], n[1])
			})
		}
	}
}

// holdsOutOfViewsRule checks what outOfView says of above and below in the
// forms given, at every width from 20 to 220.
func holdsOutOfViewsRule(t *testing.T, forms []outForm, above, below int) {
	t.Helper()
	whole := wholeOutForms(forms, above, below)
	keys, ways := forms[0].keys, min(above, 1)+min(below, 1)
	was := outSays{}
	for width := 20; width <= 220; width++ {
		got := outOfView(above, below, width, forms...)
		says := saysOf(got.plain(), keys, whole)
		switch {
		case got.width() > width:
			t.Errorf("at %d, %q is wider than the width", width, got.plain())
		case says.counts < ways && narrowest(whole.both) <= width:
			t.Errorf("at %d, %q drops a count where both fit", width, got.plain())
		case keys != "" && says.keys != whole.keysFit(width):
			t.Errorf("at %d, %q gives the keys where they don't fit beside both, or leaves them out where they do", width, got.plain())
		case !says.whole && narrowest(slices.Concat(whole.both, whole.one)) <= width:
			t.Errorf("at %d, %q is cut where a form fits whole", width, got.plain())
		case says.less(was):
			t.Errorf("at %d, %q says less than %q a column narrower", width, got.plain(), was.said)
		}
		was = says
	}
}

// outWhole are the lines a scroll line's forms say whole: with both counts,
// those of them with keys among them; and with one count, below where any
// are, else above, those with keys among them. Where only one way has any
// out of view, both and one are alike.
type outWhole struct{ both, bothKeyed, one, oneKeyed []line }

// wholeOutForms are the lines the forms say whole, with their keys and
// without, of above and below.
func wholeOutForms(forms []outForm, above, below int) outWhole {
	alone := [2]int{0, below}
	if below == 0 {
		alone = [2]int{above, 0}
	}
	var w outWhole
	for _, form := range forms {
		bare := form.keyless()
		w.both = append(w.both, form.said(above, below), bare.said(above, below))
		w.one = append(w.one, form.said(alone[0], alone[1]), bare.said(alone[0], alone[1]))
		if form.keys != "" {
			w.bothKeyed = append(w.bothKeyed, form.said(above, below))
			w.oneKeyed = append(w.oneKeyed, form.said(alone[0], alone[1]))
		}
	}
	return w
}

// keysFit is whether the keys fit width cells beside both counts, or where
// both counts don't fit, beside one.
func (w outWhole) keysFit(width int) bool {
	if narrowest(w.both) <= width {
		return narrowest(w.bothKeyed) <= width
	}
	return narrowest(w.oneKeyed) <= width
}

// narrowest is how many cells the narrowest of the lines takes, more than
// any width where there are none.
func narrowest(lines []line) int {
	n := math.MaxInt
	for _, l := range lines {
		n = min(n, l.width())
	}
	return n
}

// outSays is what a scroll line says: how many counts, whether its keys, and
// whether it's whole, one of the lines its forms say, rather than cut.
type outSays struct {
	said        string
	counts      int
	keys, whole bool
}

// saysOf is what the scroll line said says, its forms' keys being keys.
func saysOf(said, keys string, whole outWhole) outSays {
	s := outSays{said: said, counts: strings.Count(said, "▲") + strings.Count(said, "▼") + strings.Count(said, "+")}
	s.keys = keys != "" && strings.Contains(said, keys)
	s.whole = slices.ContainsFunc(slices.Concat(whole.both, whole.one), func(l line) bool { return l.plain() == said })
	return s
}

// less is whether s says less than was: fewer counts, else without the keys
// was gave, else cut where was is whole.
func (s outSays) less(was outSays) bool {
	if was.said == "" || s.counts != was.counts {
		return s.counts < was.counts
	}
	if s.keys != was.keys {
		return was.keys
	}
	return was.whole && !s.whole
}

// A view paged a row at a time reaches every row it draws, a heading marked
// over nothing at its end included: such a heading has no row to be shown
// with, so the last page keeps it rather than ending over it, and over every
// heading whose first row it is.
func TestAHeadingOverNothingAtTheEndLeavesNoRowOutOfReach(t *testing.T) {
	tests := []struct {
		name  string
		paged rowsPaged
	}{
		{name: "a heading over nothing, under a heading it's the first row of", paged: rowsPaged{rows: 13, content: 24, heads: []headed{{at: 19, first: 22}, {at: 22, first: 24}}}},
		{name: "a heading over nothing, its first row past the end", paged: rowsPaged{rows: 6, content: 10, heads: []headed{{at: 2, first: 3}, {at: 8, first: 11}}}},
		{name: "with a foot and a line", paged: rowsPaged{rows: 9, foot: 2, content: 20, heads: []headed{{at: 15, first: 17}, {at: 17, first: 20}}, says: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.paged
			reached := make([]bool, p.content)
			for offset := range p.scrolling(0).Most + 1 {
				from, to := p.shown(offset)
				for y := from; y < to; y++ {
					reached[y] = true
				}
			}
			if y := slices.Index(reached, false); y >= 0 {
				t.Errorf("%+v leaves row %d out of reach at every offset", p, y)
			}
		})
	}
}
