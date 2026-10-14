package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

// sessionsHelping is Sessions' help on the sub-tab given, open at width
// columns, tall enough to show it whole, listing keys.
func sessionsHelping(width int, shown SessionsView, keys ...Key) Card {
	f := frameOf(width, 400)
	f.View, f.SessionsTab = Sessions, SessionsShown{Shown: shown}
	f.Card = Help{Keys: keys}
	return f.Card.card(f)
}

func TestSessionsHelpListsItsKeysThenEachPartInTheDesignsOrder(t *testing.T) {
	for _, shown := range []SessionsView{SessionsSwitchboard, SessionsList} {
		t.Run(sessionsViews[shown], func(t *testing.T) {
			card := sessionsHelping(160, shown, tabViews, helpQuit[0], helpQuit[1])
			if card.title != "HELP · SESSIONS" {
				t.Errorf("help is titled %q, want HELP · SESSIONS", card.title)
			}
			if got := plainLines(card.body[:2]); !slices.Equal(got, []string{"tab views   ? help   q quit", ""}) {
				t.Errorf("help opens %q, want its keys, then a blank row", got)
			}
			if got, want := helpHeadings(card), []string{"CALLS", "CORDS", "LINES", "LIST"}; !slices.Equal(got, want) {
				t.Errorf("help explains %q, want %q, each headed in text.subtle after a blank row", got, want)
			}
		})
	}
}

// The marks help explains are the design's, each drawn as the Switchboard
// draws it: a session's cord, plug to jack, in accent.key; a free jack in
// text.faint; a cord a limit let go in state.destructive; and what a request
// is doing, ↑ and ↓ bold accent.key, ✕ bold state.destructive.
func TestSessionsHelpDrawsEachMarkAsTheSwitchboardDoes(t *testing.T) {
	type drawn struct {
		glyph string
		token theme.Token
		bold  bool
	}
	var got []drawn
	for _, part := range (Frame{View: Sessions}).sessionsHelp() {
		for _, g := range part.glyphs {
			if len(g.drawn) != 1 || g.means == "" {
				t.Errorf("%q is drawn %v, meaning %q; want it in one ink, with what it means", part.heading, g.drawn, g.means)
				continue
			}
			k := g.drawn[0].ink
			if k.fade != 0 {
				t.Errorf("%s is drawn faded %v, want its token's own colour", g.drawn[0].text, k.fade)
			}
			got = append(got, drawn{g.drawn[0].text, k.token, k.bold})
		}
	}
	want := []drawn{
		{"↑", theme.AccentKey, true},
		{"↓", theme.AccentKey, true},
		{"✕", theme.StateDestructive, true},
		{"●━━◉", theme.AccentKey, false},
		{"○", theme.TextFaint, false},
		{"╌", theme.StateDestructive, false},
	}
	if !slices.Equal(got, want) {
		t.Errorf("help explains the marks\n%+v\nwant\n%+v", got, want)
	}
}

func TestSessionsHelpSaysWhatEachPartShows(t *testing.T) {
	said := strings.Join(plainLines(sessionsHelping(160, SessionsSwitchboard).body), " ")
	for _, tc := range []struct{ what, want string }{
		{"a call a session per account", "a session whose models are on two accounts has a call in each"},
		{"rows kept look to look", "joins the foot of its group"},
		{"when a session works", "while a refused request of its is retried"},
		{"the throttle", "… 429"},
		{"where a call moved from", "▸ from the account it left"},
		{"the sub-tabs' keys", "[ and ] switch between the Switchboard and the List"},
		{"the choice", "either of a session's two calls choosing it"},
		{"enter and esc", "enter opens its page, and esc ends the choice"},
		{"the router not answering", "the calls are the sessions it last listed"},
		{"a cord's colour", "never which account"},
		{"a refusal's move", "as it lands, the session moves"},
		{"a limit's stubs", "one for each session the limit moved"},
		{"a move no limit forced", "leaves the old cord hanging a moment, fading"},
		{"lanes", "draws lanes instead"},
		{"the lines", "as switchboard usage --by account prints it"},
		{"the pin's marks", "● pinned"},
		{"out of the pool", "✕ out of the pool"},
		{"compaction", "the lines compact as the Overview's sections fold"},
		{"the List's order", "the newest end first"},
		{"the List's still order", "rows never slide from under the choice"},
		{"the List's worth", "at API prices, at today's"},
		{"a session's end", "an hour after its last request"},
		{"the today line", "The today line sums today's sessions, running and ended"},
	} {
		if !strings.Contains(said, tc.want) {
			t.Errorf("help reads\n%s\nwant it to explain %s, %q", said, tc.what, tc.want)
		}
	}
}

func TestSessionsHelpFitsTheCardAtEveryWidth(t *testing.T) {
	for _, width := range []int{216, 160, 100, 52} {
		card := sessionsHelping(width, SessionsSwitchboard, tabViews, helpQuit[0], helpQuit[1])
		inside := card.width - 2*cardInset
		for i, row := range card.body {
			if row.width() > inside || strings.Contains(strings.ReplaceAll(row.plain(), "… 429", ""), prose.Ellipsis) {
				t.Errorf("at %d columns, help's row %d is %q, %d cells; want it whole within %d", width, i+1, row.plain(), row.width(), inside)
			}
		}
	}
}
