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
// a column is focused; Detail's list's ↑↓ and enter, faint where no session
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
		{name: "Compare, a column focused", shown: dashboard.AccountsShown{Accounts: threeConfigured, Focused: true, Focus: "side"}, want: compare},
		{
			name: "Compare over the last 30 days", shown: dashboard.AccountsShown{Accounts: threeConfigured, Period: views.AccountsThirtyDays, Focused: true},
			want: "tab views   [ ] view    < > period: 30 days    ←→ account  enter its page" + ends,
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

// Help over Detail's list lists → into the chosen account's SESSIONS, which
// the key line leaves to help, while sessions run on it; not from SESSIONS,
// where ← is on the line.
func TestAccountsHelpListsTheWayIntoSessionsWhereTheKeyLineDoesnt(t *testing.T) {
	tests := []struct {
		name       string
		inSessions bool
		want       []string
	}{
		{name: "the list", want: []string{"[ ]", "< >", "↑↓", "→", "enter"}},
		{name: "SESSIONS", inSessions: true, want: []string{"[ ]", "< >", "↑↓", "←", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: "work", InSessions: tt.inSessions}
			h := accountsKeysHarness(t, routerDocument(three()...), true, shown, 0)
			if got := listedKeys(helpOver(h.model)); !containsInOrder(got, tt.want) || slices.Contains(h.listed(), "→") {
				t.Errorf("help lists %q and the key line %q; want %q among help's alone, → off the line", got, h.listed(), tt.want)
			}
		})
	}
}
