package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/views"
)

// sessionRouted is 9c2e, the session routing-session-160's card routes: in
// ~/Code/website, on personal, the router sending it to side were its own
// pin cleared.
var sessionRouted = RoutingSession{ID: "9c2e47b1-0d5a-4e83-b6f2-8a1c3e9d0f57", Dir: "~/Code/website", On: "personal", Auto: "side"}

// sessionCard is the session's card of the frames' world, 9c2e's, nothing
// picked, and changed as given.
func sessionCard(change func(*RoutingCard)) RoutingCard {
	return routedCard(func(r *RoutingCard) {
		r.Mode, r.Session = ThisSession, sessionRouted
		if change != nil {
			change(r)
		}
	})
}

func TestASessionsCardIs13RowsTallAndOneAnAccount(t *testing.T) {
	for _, accounts := range []int{1, 2, 3, 4} {
		doc := routerDoc("", 0, namedAccounts(accounts)...)
		r := RoutingCard{Mode: ThisSession, Session: sessionRouted, Rows: RoutingRows(doc.At(now, claudeLike), nil, views.Stream{})}
		if got, want := len(r.card(Frame{Width: 160, Height: 40}).body)+chrome(true), 13+accounts; got != want {
			t.Errorf("with %d accounts, the card is %d rows tall, want %d", accounts, got, want)
		}
	}
}

func TestASessionsCardIsTitledAndOpensForItsSession(t *testing.T) {
	title, body := cardText(sessionCard(nil), Frame{Width: 160, Height: 37})
	want := []string{
		"Pick the account this session goes to.",
		"",
		"9c2e  website  ~/Code/website                              runs on personal",
		"",
		"● 0 auto      the router chooses: side for now",
	}
	if title != "ROUTING · THIS SESSION" || !slices.Equal(body[:5], want) {
		t.Errorf("titled %q, opening\n%s\nwant ROUTING · THIS SESSION, opening\n%s", title, strings.Join(body[:5], "\n"), strings.Join(want, "\n"))
	}
}

func TestASessionsRowNamesItAndWhereItRuns(t *testing.T) {
	tests := []struct {
		name    string
		session RoutingSession
		// width is the row's cells: 76, the card's content at its natural
		// width, where it's 0.
		width int
		// left is what the row says from its start, and right what it says
		// at its end.
		left, right string
	}{
		{name: "its id, its directory's name, the directory, and where it runs", session: sessionRouted,
			left: "9c2e  website  ~/Code/website", right: "runs on personal"},
		{name: "a long directory cut from its start, its name kept, two cells clear of where it runs",
			session: RoutingSession{ID: "9c2e", Dir: "~/Code/clients/an-agency/projects/the-website-rebuilt/website", On: "personal"},
			left:    "9c2e  website  …gency/projects/the-website-rebuilt/website", right: "  runs on personal"},
		{name: "a directory too long to say more than its name, left out",
			session: RoutingSession{ID: "9c2e", Dir: "~/" + strings.Repeat("d", 40) + "/" + strings.Repeat("n", 40), On: "personal"},
			left:    "9c2e  " + strings.Repeat("n", 40), right: "runs on personal"},
		{name: "a directory's name too long, cut", session: RoutingSession{ID: "9c2e", Dir: "~/" + strings.Repeat("n", 60), On: "personal"},
			left: "9c2e  " + strings.Repeat("n", 51) + "…", right: "  runs on personal"},
		{name: "a directory under the home, as ~", session: RoutingSession{ID: "9c2e", Dir: "/home/dev/Code/website", Home: "/home/dev", On: "personal"},
			left: "9c2e  website  ~/Code/website", right: "runs on personal"},
		{name: "no directory, its id alone", session: RoutingSession{ID: "9c2e47b1", On: "personal"}, left: "9c2e", right: "runs on personal"},
		{name: "where it runs not known", session: RoutingSession{ID: "9c2e", Dir: "~/Code/website"}, left: "9c2e  website  ~/Code/website"},
		{name: "an account named by its name", session: RoutingSession{ID: "9c2e", On: "side"}, left: "9c2e", right: "runs on side"},
		{name: "stacked, no directory, as a phone's page heads it", session: sessionRouted, width: 44, left: "9c2e  website", right: "runs on personal"},
		{name: "a card too narrow to say more than its name, no directory", session: sessionRouted, width: 40, left: "9c2e  website", right: "runs on personal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width := cmp.Or(tt.width, 76)
			want := tt.left
			if tt.right != "" {
				want += strings.Repeat(" ", width-ansi.StringWidth(tt.left)-ansi.StringWidth(tt.right)) + tt.right
			}
			r := sessionCard(func(r *RoutingCard) { r.Session = tt.session })
			if got := strings.TrimRight(r.sessionRow(width, !r.columns(width).stacked).plain(), " "); got != want {
				t.Errorf("the row reads\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestASessionsRowIsInkedAsTheDesignSays(t *testing.T) {
	inked := map[string]ink{}
	for _, s := range sessionCard(nil).sessionRow(76, true) {
		inked[strings.TrimSpace(s.text)] = s.ink
	}
	for text, want := range map[string]ink{"9c2e": titleInk, "website": mutedInk, "~/Code/website": secondaryInk, "runs on": mutedInk, "personal": titleInk} {
		if got := inked[text]; got != want {
			t.Errorf("%q is drawn in %+v, want %+v", text, got, want)
		}
	}
}

func TestAutosRowOnASessionsCardNamesWhereTheRouterWouldSendIt(t *testing.T) {
	tests := []struct {
		name  string
		auto  string
		room  int
		want  string
		named bool
	}{
		{name: "the router naming an account", auto: "side", room: 60, want: "the router chooses: side for now", named: true},
		{name: "the router naming none, as no account has room, or from an older router", room: 60, want: "the router chooses"},
		{name: "too narrow for for now, its account kept", auto: "side", room: 28, want: "the router chooses: side", named: true},
		{name: "too narrow even for that, cut", auto: "side", room: 20, want: "the router chooses:…", named: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := sessionCard(func(r *RoutingCard) { r.Session.Auto = tt.auto })
			said := r.autoChooses(tt.room)
			if got := said.plain(); got != tt.want {
				t.Errorf("auto's row says %q, want %q", got, tt.want)
			}
			if named := slices.Contains(said, span{"side", titleInk}); named != tt.named {
				t.Errorf("auto's row names side in bold text.primary: %v, want %v", named, tt.named)
			}
		})
	}
	if got := routedCard(func(r *RoutingCard) { r.Session.Auto = "side" }).autoChooses(60).plain(); got != "the router chooses" {
		t.Errorf("on new sessions' card, auto's row says %q, want the router chooses, naming no account", got)
	}
}

func TestASessionsCardPicksOneMarkedWithADot(t *testing.T) {
	r := sessionCard(func(r *RoutingCard) { r.Picked = []string{"side"} })
	var marks []span
	for _, row := range r.Rows {
		marks = append(marks, r.choice(row, r.columns(76), 76)[0][0])
	}
	// work, at its cap, can be picked, as a pin spends an account's cap.
	want := []span{{"○", dimInk}, {"○", dimInk}, {"○", dimInk}, {"●", keyInk}}
	if !slices.Equal(marks, want) {
		t.Errorf("the rows are marked %v, want %v", marks, want)
	}
	if got := sessionCard(nil).choice(r.Rows[0], r.columns(76), 76)[0][0]; got != (span{"●", keyInk}) {
		t.Errorf("auto's mark, nothing picked, is %+v, want it picked", got)
	}
}

func TestASessionsOutcomeSaysWhatApplyingChangesForIt(t *testing.T) {
	tests := []struct {
		name string
		card func(*RoutingCard)
		want []string
	}{
		{name: "auto", want: []string{"This session follows the router."}},
		{name: "where it runs", card: func(r *RoutingCard) { r.Picked = []string{"personal"} }, want: []string{"This session stays on personal."}},
		{name: "another account, and what moving there costs", card: func(r *RoutingCard) { r.Picked, r.Cost = []string{"side"}, dollars(156) }, want: []string{
			"This session moves to side on its next request.", "Rebuilding its cache costs ≈ $1.56.",
		}},
		{name: "another account, the cost not known", card: func(r *RoutingCard) { r.Picked = []string{"side"} }, want: []string{
			"This session moves to side on its next request.",
		}},
		{name: "another account, costing nothing, idle past its cache", card: func(r *RoutingCard) { r.Picked, r.Cost = []string{"work"}, dollars(0) }, want: []string{
			"This session moves to work on its next request.",
		}},
		{name: "where it runs, the cost of moving said not", card: func(r *RoutingCard) { r.Picked, r.Cost = []string{"personal"}, dollars(156) }, want: []string{
			"This session stays on personal.",
		}},
		{name: "an account picked that's left unsent, which follows the router", card: func(r *RoutingCard) { r.Picked, r.Unsent = []string{"side"}, []string{"side"} }, want: []string{
			"This session follows the router.",
		}},
		{name: "refused, the router's reason in its place", card: func(r *RoutingCard) { r.Picked, r.Refused = []string{"side"}, "no usable token for side" }, want: []string{
			"no usable token for side",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := sessionCard(tt.card)
			outcome := r.outcome(76)
			if len(outcome) != sessionOutcomeRows {
				t.Errorf("the outcome takes %d rows, want %d whatever it says", len(outcome), sessionOutcomeRows)
			}
			if got := plains(outcome); !slices.Equal(got, tt.want) {
				t.Errorf("the outcome reads %q, want %q", got, tt.want)
			}
		})
	}
}

func TestASessionsOutcomeNamesItsAccountInBoldAndItsCostInAttention(t *testing.T) {
	inked := map[string]ink{}
	for _, l := range sessionCard(func(r *RoutingCard) { r.Picked, r.Cost = []string{"side"}, dollars(156) }).outcome(76) {
		for _, s := range l {
			inked[s.text] = s.ink
		}
	}
	for text, want := range map[string]ink{"side": titleInk, "≈": alertInk, "$1.56": alertInk, "This": secondaryInk, "Rebuilding": secondaryInk} {
		if got := inked[text]; got != want {
			t.Errorf("%q is drawn in %+v, want %+v", text, got, want)
		}
	}
}

func TestASessionsKeysRowPicksAndHasNoM(t *testing.T) {
	tests := []struct {
		name     string
		accounts int
		width    int
		want     string
	}{
		{name: "three accounts", accounts: 3, width: 160, want: "↑↓ move   space 0-3 pick   enter apply   esc cancel"},
		{name: "past nine accounts", accounts: 12, width: 160, want: "↑↓ move   space 0-9 pick   enter apply   esc cancel"},
		{name: "a phone of 52 columns, ↑↓ first to go", accounts: 3, width: 52, want: "space 0-3 pick   enter apply   esc cancel"},
		{name: "a narrow phone, enter and esc kept", accounts: 3, width: 30, want: "enter apply   esc cancel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RoutingCard{Mode: ThisSession, Rows: make([]RoutingRow, tt.accounts+1)}
			if got := keyRow(r.card(Frame{Width: tt.width, Height: 40}).keys).plain(); got != tt.want {
				t.Errorf("the keys row reads %q, want %q", got, tt.want)
			}
		})
	}
}

func TestANarrowSessionsCardKeepsItsRowsToTheScreen(t *testing.T) {
	_, body := cardText(sessionCard(func(r *RoutingCard) { r.Picked = []string{"side"} }), Frame{Width: 52, Height: 40})
	want := []string{
		"Pick the account this session goes to.",
		"",
		"9c2e  website               runs on personal",
		"",
		"○ 0 auto     the router chooses: side",
		"○ 1 work     closed    at its cap till 17:10",
		"             week 34%   5-hour 95%",
		"○ 2 personal open                  2 running",
		"             week 66%   5-hour 39%",
		"● 3 side     open                  4 running",
		"             week 78%   5-hour 13%",
		"",
		"This session moves to side on its next",
		"request.",
	}
	if got := body[:len(want)]; !slices.Equal(got, want) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestNoPickOrRefusalMovesASessionsCard(t *testing.T) {
	refusal := strings.Repeat("the router refused the pin, as the account it names can take no request now; ", 8)
	for _, accounts := range []int{1, 2, 3, 4, 12} {
		doc := routerDoc("", 0, namedAccounts(accounts)...)
		rows := RoutingRows(doc.At(now, claudeLike), nil, views.Stream{})
		picks := [][]string{nil}
		for _, a := range doc.Accounts {
			picks = append(picks, []string{a.ID})
		}
		for _, width := range []int{52, 160} {
			t.Run(fmt.Sprintf("%d accounts at %d columns", accounts, width), func(t *testing.T) {
				f := Frame{Width: width, Height: 37}
				opened := RoutingCard{Mode: ThisSession, Session: sessionRouted, Rows: rows}
				at := cardAt(f, opened)
				content := f.cardWidth(opened.natural()) - 2*cardInset
				for _, picked := range picks {
					for _, said := range []RoutingCard{{}, {Cost: dollars(99_999_999)}, {Refused: refusal}} {
						r := opened
						r.Picked, r.Cost, r.Refused = picked, said.Cost, said.Refused
						if got := cardAt(f, r); got != at {
							t.Errorf("picking %q, costing %v, refused %v, the card sits at %+v, want it where it opened, %+v", picked, r.Cost != nil, r.Refused != "", got, at)
						}
						if lines, room := len(wrappedAt(r.sentences(), content)), r.outcomeRoom(content); r.Refused == "" && lines > room {
							t.Errorf("picking %q, the outcome takes %d rows, more than the %d kept for it", picked, lines, room)
						}
					}
				}
			})
		}
	}
}
