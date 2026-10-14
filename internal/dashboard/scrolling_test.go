package dashboard

import (
	"slices"
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
// USAGE's rows (~1450), SESSIONS' sessions (~1685, ~1917), Days' accounts
// (~2162), Tokens' periods (~2191) and the routing card's rows (~3671), as
// they're drawn, SESSIONS' leading with + in place of its mark; and
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
		{name: "USAGE's rows, below", below: 4, forms: []outForm{usageOut}, want: line{{"▼ 4 more accounts", subtle}}},
		{name: "USAGE's rows, above", above: 2, forms: []outForm{usageOut}, want: line{{"▲ 2 more accounts", subtle}}},
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
	for _, forms := range [][]outForm{historyAccountsOut, {periodsOut("week")}, {usageOut}, {runningOut}, {cardOut}} {
		if got := outOfView(0, 0, 160, forms...); got != nil {
			t.Errorf("with nothing out of view, %+v says %q, want nothing", forms[0], got.plain())
		}
	}
}
