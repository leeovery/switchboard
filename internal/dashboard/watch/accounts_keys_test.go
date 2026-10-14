package watch

import (
	"cmp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// accountsKeysHarness is a model of the router of the accounts given, d28c
// running on work where sessions run, showing Accounts as given, in a
// terminal width cells wide, or 160, and 37 tall. It has started.
func accountsKeysHarness(t *testing.T, doc status.Document, sessions bool, shown dashboard.AccountsShown, width int) *harness {
	t.Helper()
	h := routedHarness(t, doc)
	if sessions {
		h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 10*time.Second)}
	}
	h.start()
	h.model.view, h.model.accounts.shown = dashboard.Accounts, shown
	h.update(tea.WindowSizeMsg{Width: cmp.Or(width, 160), Height: 37})
	return h
}

// offKeys are the keys of the model's key line drawn faint, as they can't
// act now.
func (h *harness) offKeys() []string {
	var off []string
	for _, group := range h.model.live(h.clock.now).KeyLine.Groups {
		for _, k := range group {
			if k.Off {
				off = append(off, k.Key)
			}
		}
	}
	return off
}

// Accounts' key lines are the design's: Compare's ←→ and enter, faint till
// a column is focused, and esc while one is; Detail's list's ↑↓ and enter, faint where no session
// runs on the account chosen; its SESSIONS' ↑↓, ← and enter; with one
// account, no [ ] view, ↑↓ and enter joining while sessions run on it, and
// p pin where a pin set from the command line stands; and a phone's keeping
// [ ] view alone.
func TestAccountsKeyLinesAreTheDesigns(t *testing.T) {
	const (
		head    = "tab views   [ ] view    < > period: this week    "
		ends    = "    p pin    ? help  q quit"
		compare = head + "←→ account  enter its page" + ends
		list    = head + "↑↓ account  enter its page" + ends
	)
	one := config.Accounts{{ID: "work"}}
	pinned := routerDocument(three()[0])
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	tests := []struct {
		name     string
		doc      status.Document
		sessions bool
		shown    dashboard.AccountsShown
		width    int
		want     string
		wantOff  []string
	}{
		{name: "Compare, no column focused", shown: dashboard.AccountsShown{Accounts: threeConfigured}, want: compare, wantOff: []string{"enter"}},
		{
			name: "Compare, a column focused", shown: dashboard.AccountsShown{Accounts: threeConfigured, Focused: true, Focus: "side"},
			want: head + "←→ account  enter its page  esc unfocus" + ends,
		},
		{
			name: "Compare over the last 30 days", shown: dashboard.AccountsShown{Accounts: threeConfigured, Period: views.AccountsThirtyDays},
			want: "tab views   [ ] view    < > period: 30 days    ←→ account  enter its page" + ends, wantOff: []string{"enter"},
		},
		{
			name: "Compare, Detail's SESSIONS left as they were", sessions: true,
			shown: dashboard.AccountsShown{Accounts: threeConfigured, Chosen: "work", InSessions: true}, want: compare, wantOff: []string{"enter"},
		},
		{name: "Detail of all accounts", sessions: true, shown: dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail}, want: list, wantOff: []string{"enter"}},
		{name: "Detail of an account sessions run on", sessions: true, shown: dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: "work"}, want: list},
		{name: "Detail of an account none runs on", sessions: true, shown: dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: "side"}, want: list, wantOff: []string{"enter"}},
		{
			name: "Detail's SESSIONS", sessions: true, shown: dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: "work", InSessions: true},
			want: head + "↑↓ session  ← accounts  enter its page" + ends,
		},
		{
			name: "Detail's SESSIONS, its sessions ended", shown: dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: "work", InSessions: true},
			want: list, wantOff: []string{"enter"},
		},
		{
			name: "one account, sessions running on it", doc: routerDocument(three()[0]), sessions: true, shown: dashboard.AccountsShown{Accounts: one},
			want: "tab views   < > period: this week    ↑↓ session  enter its page    ? help  q quit",
		},
		{name: "one account, none running on it", doc: routerDocument(three()[0]), shown: dashboard.AccountsShown{Accounts: one}, want: "tab views   < > period: this week    ? help  q quit"},
		{name: "one account, pinned from the command line", doc: pinned, shown: dashboard.AccountsShown{Accounts: one}, want: "tab views   < > period: this week    p pin    ? help  q quit"},
		{name: "a phone on Compare, a column focused", shown: dashboard.AccountsShown{Accounts: threeConfigured, Focused: true}, width: 52, want: "tab views   [ ] view   ? keys   q quit"},
		{name: "a phone", sessions: true, shown: dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: "work"}, width: 52, want: "tab views   [ ] view   ? keys   q quit"},
		{name: "a phone of one account", doc: routerDocument(three()[0]), sessions: true, shown: dashboard.AccountsShown{Accounts: one}, width: 52, want: "tab views   ? keys   q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			if doc.Accounts == nil {
				doc = routerDocument(three()...)
			}
			h := accountsKeysHarness(t, doc, tt.sessions, tt.shown, tt.width)
			rows := strings.Split(h.view(), "\n")
			if got := strings.TrimRight(rows[len(rows)-1], " "); got != tt.want {
				t.Errorf("the key line reads\n%q\nwant\n%q", got, tt.want)
			}
			if got := h.offKeys(); tt.width == 0 && !slices.Equal(got, tt.wantOff) {
				t.Errorf("the key line draws %q faint, want %q", got, tt.wantOff)
			}
		})
	}
}

// The page shown lists Accounts' keys as it was drawn, at its moment, never
// the clock's as it's viewed: d28c, the one session on work, crossing the
// idle hour after the page was drawn, is still the one enter opens, as the
// page's SESSIONS still list it.
func TestTheKeyLineListsAccountsKeysAtThePagesMoment(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", views.IdleHour-time.Second)}
	h.start()
	h.model.view = dashboard.Accounts
	h.model.accounts.shown = dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: "work", InSessions: true}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	if running := h.model.running(h.clock.now); !slices.Equal(running, []string{idD28C}) {
		t.Fatalf("the page is drawn with %v running on work, want d28c", running)
	}

	h.clock.now = h.clock.now.Add(2 * time.Second)
	for _, group := range h.model.shownPage().KeyLine.Groups {
		for _, k := range group {
			if k.Key == "enter" && k.Off {
				t.Errorf("d28c crossing the idle hour since the page was drawn, its key line draws enter faint, want it as drawn")
			}
		}
	}
}

// Help over Detail's list lists → into the chosen account's SESSIONS, which
// the key line leaves to help, while a session is selected there; not where
// none runs, nor from SESSIONS, where ← is on the line.
func TestAccountsHelpListsTheWayIntoSessionsWhereTheKeyLineDoesnt(t *testing.T) {
	tests := []struct {
		name       string
		chosen     string
		inSessions bool
		want       []string
	}{
		{name: "the list", chosen: "work", want: []string{"[ ]", "< >", "↑↓", "→", "enter"}},
		{name: "the list, of an account none runs on", chosen: "side", want: []string{"[ ]", "< >", "↑↓", "enter"}},
		{name: "SESSIONS", chosen: "work", inSessions: true, want: []string{"[ ]", "< >", "↑↓", "←", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: tt.chosen, InSessions: tt.inSessions}
			h := accountsKeysHarness(t, routerDocument(three()...), true, shown, 0)
			if got := listedKeys(helpOver(h.model)); !containsInOrder(got, tt.want) || slices.Contains(h.listed(), "→") {
				t.Errorf("help lists %q and the key line %q; want %q among help's alone, → off the line", got, h.listed(), tt.want)
			}
		})
	}
}

// Compare's focus keys, ←→, enter and esc, are listed where its focus is
// drawn, wide; and on a phone, whose Compare draws none, or with no
// accounts, whose Compare draws no columns, neither in help nor on the line.
func TestAccountsListsCompareFocusKeysOnlyWhereTheFocusIsDrawn(t *testing.T) {
	focus := []string{"←→", "enter", "esc"}
	for _, tt := range []struct {
		name     string
		width    int
		accounts config.Accounts
		want     bool
	}{
		{name: "wide", width: 160, accounts: threeConfigured, want: true},
		{name: "at the narrowest wide", width: 100, accounts: threeConfigured, want: true},
		{name: "a phone", width: 99, accounts: threeConfigured},
		{name: "a phone's narrowest", width: 52, accounts: threeConfigured},
		{name: "no accounts", width: 160},
	} {
		t.Run(tt.name, func(t *testing.T) {
			shown := dashboard.AccountsShown{Accounts: tt.accounts, Focused: true, Focus: "work"}
			h := accountsKeysHarness(t, routerDocument(three()...), false, shown, tt.width)
			helped := listedKeys(helpOver(h.model))
			for _, k := range focus {
				if got := slices.Contains(helped, k); got != tt.want {
					t.Errorf("at %d columns, help lists %q, want %s listed %v", tt.width, helped, k, tt.want)
				}
			}
			if !tt.want && slices.ContainsFunc(h.listed(), func(k string) bool { return slices.Contains(focus, k) }) {
				t.Errorf("at %d columns, the key line lists %q, want none of %q", tt.width, h.listed(), focus)
			}
		})
	}
}

// enter is drawn faint on the key line exactly where pressing it opens
// nothing, both as Opens says: on Compare, with no column focused and with
// one; on Detail, of all accounts, of an account none runs on, and of one
// sessions run on, from its list and from its SESSIONS; and with one
// account, sessions running on it.
func TestEnterIsFaintExactlyWhereItOpensNothing(t *testing.T) {
	tests := []struct {
		name  string
		shown dashboard.AccountsShown
		alone bool
	}{
		{name: "Compare, no column focused"},
		{name: "Compare, a column focused", shown: dashboard.AccountsShown{Focused: true, Focus: "side"}},
		{name: "Detail of all accounts", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail}},
		{name: "Detail of an account none runs on", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "side"}},
		{name: "Detail's list, sessions running", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"}},
		{name: "Detail's SESSIONS", shown: dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work", InSessions: true}},
		{name: "one account, sessions running on it", alone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := accountsHarness(t, tt.shown)
			if tt.alone {
				h.model.accounts.shown.Accounts = config.Accounts{{ID: "work"}}
			}
			if !slices.Contains(h.listed(), "enter") {
				t.Fatalf("the key line lists %q, want enter", h.listed())
			}
			faint, was := slices.Contains(h.offKeys(), "enter"), h.model.accounts.shown.View()
			h.keys(enterKey)
			if opened := h.model.session != nil || h.model.accounts.shown.View() != was; opened == faint {
				t.Errorf("enter drawn faint %v, and pressed, opened %v; want it faint exactly where it opens nothing", faint, opened)
			}
		})
	}
}
