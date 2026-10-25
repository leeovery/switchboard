package dashboard

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// What the preferences keep of Sessions reads back as it was kept: the
// sub-tab, by its name; never the row chosen, which is the run's alone.
func TestSessionsChoicesAreKeptByNameAndReadBack(t *testing.T) {
	tests := []struct {
		name  string
		shown SessionsShown
		kept  theme.SessionsPrefs
	}{
		{name: "as Sessions opens", kept: theme.SessionsPrefs{View: "switchboard"}},
		{name: "the List", shown: SessionsShown{Shown: SessionsList}, kept: theme.SessionsPrefs{View: "list"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chosen := tt.shown
			chosen.Chosen = Plug{Session: idD28C, Account: "work"}
			if got := chosen.Kept(); got != tt.kept {
				t.Errorf("Kept() = %+v, want %+v", got, tt.kept)
			}
			if got := SessionsKept(tt.kept); !reflect.DeepEqual(got, tt.shown) {
				t.Errorf("SessionsKept(%+v) = %+v, want %+v", tt.kept, got, tt.shown)
			}
		})
	}
}

// What the preferences kept that names nothing Sessions knows, as a later
// build's, shows as Sessions opens: the Switchboard.
func TestSessionsChoicesKeptThatNameNothingReadAsItOpens(t *testing.T) {
	for _, kept := range []theme.SessionsPrefs{{View: "lanes"}, {View: "List"}, {}} {
		if got := SessionsKept(kept); !reflect.DeepEqual(got, SessionsShown{}) {
			t.Errorf("SessionsKept(%+v) = %+v, want the Switchboard", kept, got)
		}
	}
}

// Sessions' heading row names it and its sub-tabs, the one shown picked out,
// and [ ] after them, as sessions-switchboard-160 draws it; on a phone, its
// sub-tabs set apart after its name, as sessions-phone-52 draws them.
func TestSessionsHeadingRow(t *testing.T) {
	tests := []struct {
		name          string
		width, row    int
		shown         SessionsView
		picked, other int
		want          string
	}{
		{name: "the Switchboard", width: 160, row: 4, picked: 12, other: 25, want: "SESSIONS    Switchboard  List  [ ]  " + rule(124)},
		{name: "the List", width: 160, row: 4, shown: SessionsList, picked: 25, other: 12, want: "SESSIONS    Switchboard  List  [ ]  " + rule(124)},
		{name: "on a phone", width: 52, row: 6, picked: 11, other: 24, want: "SESSIONS   Switchboard  List"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 37)
			f.View, f.SessionsTab = Sessions, SessionsShown{Shown: tt.shown}
			c := f.page(threeRouted(), now)
			if got := strings.TrimRight(c.rows(Look{})[tt.row], " "); got != tt.want {
				t.Errorf("row %d reads\n%q\nwant\n%q", tt.row+1, got, tt.want)
			}
			if got := c.at(tt.picked, tt.row).ink; got != shownSubTabInk {
				t.Errorf("the sub-tab shown, at column %d, is in %+v, want it picked out", tt.picked, got)
			}
			if got := c.at(tt.other, tt.row).ink; got != tabInk {
				t.Errorf("the other sub-tab, at column %d, is in %+v, want %+v", tt.other, got, tabInk)
			}
		})
	}
}

// sessionsOnTheBoard is Sessions with d28c on work and side, a row on each,
// and 7f3a and 41e0 on side, as the patch keeps them; the List listing 41e0,
// then d28c, then 9c2e, ended today.
func sessionsOnTheBoard(shown SessionsView) SessionsShown {
	calls := []views.Call{
		{Session: "41e0", Account: "side"},
		{Session: idD28C, Account: "side"},
		{Session: idD28C, Account: "work"},
		{Session: id7F3A, Account: "side"},
	}
	list := views.SessionList{Sessions: []views.ListedSession{{Session: "41e0"}, {Session: idD28C}, {Session: "9c2e"}}}
	return SessionsShown{Shown: shown, Patched: views.Patch(views.Patched{}, calls, nil), Calls: calls, List: list}
}

// ↑ and ↓ step through the rows top to bottom, no further than the first and
// the last: on the Switchboard, the calls, the lines in the accounts' order,
// each row of a session on two accounts a stop of its own; on the List, its
// sessions, ended or not. From none, ↓ chooses the first and ↑ the last.
func TestSessionsUpAndDownStepThroughTheRows(t *testing.T) {
	accounts := []string{"work", "personal", "side"}
	board, list := sessionsOnTheBoard(SessionsSwitchboard), sessionsOnTheBoard(SessionsList)
	tests := []struct {
		name    string
		shown   SessionsShown
		from    Plug
		by      int
		want    Plug
		changed bool
	}{
		{name: "from none, ↓: the first call", shown: board, by: 1, want: Plug{Session: idD28C, Account: "work"}, changed: true},
		{name: "from none, ↑: the last call", shown: board, by: -1, want: Plug{Session: id7F3A, Account: "side"}, changed: true},
		{name: "↓ from work's line to side's", shown: board, from: Plug{Session: idD28C, Account: "work"}, by: 1, want: Plug{Session: "41e0", Account: "side"}, changed: true},
		{name: "↓ onto d28c's other row", shown: board, from: Plug{Session: "41e0", Account: "side"}, by: 1, want: Plug{Session: idD28C, Account: "side"}, changed: true},
		{name: "↓ at the last: it stays", shown: board, from: Plug{Session: id7F3A, Account: "side"}, by: 1, want: Plug{Session: id7F3A, Account: "side"}},
		{name: "↑ at the first: it stays", shown: board, from: Plug{Session: idD28C, Account: "work"}, by: -1, want: Plug{Session: idD28C, Account: "work"}},
		{name: "the List, from none, ↓", shown: list, by: 1, want: Plug{Session: "41e0"}, changed: true},
		{name: "the List, ↓ past the ended rule", shown: list, from: Plug{Session: idD28C}, by: 1, want: Plug{Session: "9c2e"}, changed: true},
		{name: "the List, from none, ↑", shown: list, by: -1, want: Plug{Session: "9c2e"}, changed: true},
		{name: "nothing to choose", by: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.shown
			s.Chosen = tt.from
			got, changed := s.Choosing(tt.by, accounts)
			if got.Chosen != tt.want || changed != tt.changed {
				t.Errorf("Choosing(%d) chose %+v, changed %t; want %+v, %t", tt.by, got.Chosen, changed, tt.want, tt.changed)
			}
		})
	}
}

// [ and ] turn the sub-tab, round, the choice kept where its session shows on
// the other, on the Switchboard its first row, else ended; a click shows the
// sub-tab it's on, and no other.
func TestSessionsSubTabTurnsKeepingTheChoiceWhereItsSessionShows(t *testing.T) {
	accounts := []string{"work", "personal", "side"}
	tests := []struct {
		name    string
		shown   SessionsShown
		chosen  Plug
		turn    func(SessionsShown) (SessionsShown, bool)
		want    SessionsView
		kept    Plug
		changed bool
	}{
		{
			name: "] to the List, d28c listed", shown: sessionsOnTheBoard(SessionsSwitchboard), chosen: Plug{Session: idD28C, Account: "side"},
			turn: func(s SessionsShown) (SessionsShown, bool) { return s.Turned(1, accounts) }, want: SessionsList, kept: Plug{Session: idD28C}, changed: true,
		},
		{
			name: "] round to the Switchboard, d28c's first row", shown: sessionsOnTheBoard(SessionsList), chosen: Plug{Session: idD28C},
			turn: func(s SessionsShown) (SessionsShown, bool) { return s.Turned(1, accounts) }, want: SessionsSwitchboard, kept: Plug{Session: idD28C, Account: "work"}, changed: true,
		},
		{
			name: "[ to the Switchboard, 9c2e ended: the choice ends", shown: sessionsOnTheBoard(SessionsList), chosen: Plug{Session: "9c2e"},
			turn: func(s SessionsShown) (SessionsShown, bool) { return s.Turned(-1, accounts) }, want: SessionsSwitchboard, changed: true,
		},
		{
			name: "a click on the List", shown: sessionsOnTheBoard(SessionsSwitchboard), chosen: Plug{Session: "41e0", Account: "side"},
			turn: func(s SessionsShown) (SessionsShown, bool) { return s.Showing(1, accounts) }, want: SessionsList, kept: Plug{Session: "41e0"}, changed: true,
		},
		{
			name: "a click on the sub-tab shown", shown: sessionsOnTheBoard(SessionsSwitchboard), chosen: Plug{Session: "41e0", Account: "side"},
			turn: func(s SessionsShown) (SessionsShown, bool) { return s.Showing(0, accounts) }, want: SessionsSwitchboard, kept: Plug{Session: "41e0", Account: "side"},
		},
		{
			name: "a click on no sub-tab", shown: sessionsOnTheBoard(SessionsSwitchboard),
			turn: func(s SessionsShown) (SessionsShown, bool) { return s.Showing(2, accounts) }, want: SessionsSwitchboard,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.shown
			s.Chosen = tt.chosen
			got, changed := tt.turn(s)
			if got.Shown != tt.want || got.Chosen != tt.kept || changed != tt.changed {
				t.Errorf("it shows %d, %+v chosen, changed %t; want %d, %+v, %t", got.Shown, got.Chosen, changed, tt.want, tt.kept, tt.changed)
			}
		})
	}
}

// The choice follows its session from look to look: kept on its row where it
// still shows, a row in two groups among them; else on its session's first
// row, as where a move took it to another account; else it ends, as once
// the router stops listing it, or the List lets it go.
func TestSessionsChoiceFollowsItsSessionFromLookToLook(t *testing.T) {
	accounts := []string{"work", "personal", "side"}
	tests := []struct {
		name   string
		shown  SessionsView
		chosen Plug
		want   Plug
	}{
		{name: "a call still there", chosen: Plug{Session: idD28C, Account: "side"}, want: Plug{Session: idD28C, Account: "side"}},
		{name: "a call moved to another account", chosen: Plug{Session: "41e0", Account: "work"}, want: Plug{Session: "41e0", Account: "side"}},
		{name: "a call the router lists no more", chosen: Plug{Session: "9e21", Account: "work"}},
		{name: "a row of the List, ended", shown: SessionsList, chosen: Plug{Session: "9c2e"}, want: Plug{Session: "9c2e"}},
		{name: "a row the List let go", shown: SessionsList, chosen: Plug{Session: "5e7d"}},
		{name: "none chosen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := sessionsOnTheBoard(tt.shown)
			s.Chosen = tt.chosen
			if got := s.Following(accounts).Chosen; got != tt.want {
				t.Errorf("Following() chose %+v, want %+v", got, tt.want)
			}
		})
	}
}

// esc deselects, reporting a change only where there was one; enter
// opens a page only while a row is chosen; a click chooses the row it's on.
func TestSessionsChoiceEndsOpensAndIsClicked(t *testing.T) {
	s := sessionsOnTheBoard(SessionsSwitchboard)
	if _, changed := s.Unchosen(); changed || s.Opens() {
		t.Errorf("nothing chosen, esc changed it: %t, and enter opens: %t; want neither", changed, s.Opens())
	}
	s = s.ChosenOn(Plug{Session: id7F3A, Account: "side"})
	if s.Chosen != (Plug{Session: id7F3A, Account: "side"}) || !s.Opens() {
		t.Errorf("clicked on 7f3a's call, %+v is chosen, enter opens: %t; want 7f3a on side, opening", s.Chosen, s.Opens())
	}
	if ended, changed := s.Unchosen(); ended.Chosen != (Plug{}) || !changed {
		t.Errorf("esc leaves %+v chosen, changed %t; want none, changed", ended.Chosen, changed)
	}
}

// The Switchboard's lines are the moment's accounts, in their order, each a
// row a window the live views list of it: each model's own, quiet, only
// while w shows them.
func TestTheSwitchboardsLinesAreTheMomentsAccounts(t *testing.T) {
	doc := routerDoc("side", 5, pressedAccount("work"), limitedAccount("personal"),
		readAccount("side", sessionOf(0.12, 4*time.Hour), weekOf(0.33, 5*quota.Day), windowOf("7d_oi", "Fable week", 0.1, 5*quota.Day)))
	m := doc.At(now, claudeLike)
	for _, models := range []bool{false, true} {
		got := Frame{Models: models}.boardLines(m)
		want, windows := []string{"work", "personal", "side"}, len(m.Listed(models))
		if !reflect.DeepEqual(got.accounts, want) || got.windows != windows {
			t.Errorf("with models %t, the lines are %+v, want %v, each of %d windows", models, got, want, windows)
		}
	}
	if hidden, shown := len(m.Listed(false)), len(m.Listed(true)); hidden != 2 || shown != 3 {
		t.Errorf("the moment lists %d windows, and %d with models; want 2 and 3", hidden, shown)
	}
}

// The List scrolls a row at a time; the Switchboard, where it scrolls, an
// account at a time.
func TestSessionsListScrollsRowsAndTheSwitchboardAccounts(t *testing.T) {
	f := frameOf(160, 37)
	f.View = Sessions
	if f.ScrollsRows(status.Document{}, now) {
		t.Errorf("the Switchboard scrolls a row at a time, want an account at a time")
	}
	f.SessionsTab.Shown = SessionsList
	if !f.ScrollsRows(status.Document{}, now) {
		t.Errorf("the List scrolls a thing at a time, want a row")
	}
}
