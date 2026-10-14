package watch

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// sessionsKeysHarness is a model of the router of doc, d28c and 7f3a running
// on work, showing Sessions' sub-tab given, in a terminal width cells wide
// and 37 tall. It has started.
func sessionsKeysHarness(t *testing.T, doc status.Document, shown dashboard.SessionsView, width int) *harness {
	t.Helper()
	h := routedHarness(t, doc)
	h.source.sessions = []status.Session{
		sessionOn(idD28C, opus, "work", 10*time.Second),
		sessionOn(id7F3A, "claude-haiku-4-5", "work", 9*time.Minute),
	}
	h.model.view, h.model.sessionsTab.shown.Shown = dashboard.Sessions, shown
	h.update(tea.WindowSizeMsg{Width: width, Height: 37})
	h.start()
	return h
}

// phoneOffKeys are the keys of the model's key line on a phone drawn faint,
// as they can't act now.
func (h *harness) phoneOffKeys() []string {
	var off []string
	for _, k := range h.model.live(h.clock.now).KeyLine.Phone {
		if k.Off {
			off = append(off, k.Key)
		}
	}
	return off
}

// Sessions' key lines are the design's, on both sub-tabs alike: enter faint
// till a session is chosen; with one account, no p pin, but where a pin set
// from the command line stands; p pin faint while the router doesn't
// answer; and a phone's line keeping ↑↓ session, enter open and [ ] view, in
// that order.
func TestSessionsKeyLinesAreTheDesigns(t *testing.T) {
	const (
		wide  = "tab views   [ ] view    ↑↓ session  enter its page    p pin    ? help  q quit"
		alone = "tab views   [ ] view    ↑↓ session  enter its page    ? help  q quit"
		phone = "↑↓ session   enter open   [ ] view   ? keys   q quit"
	)
	pinned := routerDocument(three()[0])
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	tests := []struct {
		name    string
		doc     status.Document
		width   int
		chosen  bool
		gone    bool
		want    string
		wantOff []string
	}{
		{name: "nothing chosen", width: 160, want: wide, wantOff: []string{"enter"}},
		{name: "a session chosen", width: 160, chosen: true, want: wide},
		{name: "the router not answering", width: 160, chosen: true, gone: true, want: wide, wantOff: []string{"p"}},
		{name: "one account", doc: routerDocument(three()[0]), width: 160, want: alone, wantOff: []string{"enter"}},
		{name: "one account, pinned from the command line", doc: pinned, width: 160, want: wide, wantOff: []string{"enter"}},
		{name: "a phone, nothing chosen", width: 52, want: phone, wantOff: []string{"enter"}},
		{name: "a phone, a session chosen", width: 52, chosen: true, want: phone},
	}
	for _, tt := range tests {
		for _, shown := range []dashboard.SessionsView{dashboard.SessionsSwitchboard, dashboard.SessionsList} {
			t.Run(tt.name+" on the "+[]string{"Switchboard", "List"}[shown], func(t *testing.T) {
				doc := tt.doc
				if doc.Accounts == nil {
					doc = routerDocument(three()...)
				}
				h := sessionsKeysHarness(t, doc, shown, tt.width)
				if tt.gone {
					h.stopRouter()
					h.tickUntilAsked()
				}
				if tt.chosen {
					h.model.sessionsTab.shown.Chosen = dashboard.Plug{Session: idD28C, Account: "work"}
				}
				rows := strings.Split(h.view(), "\n")
				if got := strings.TrimRight(rows[len(rows)-1], " "); got != tt.want {
					t.Errorf("the key line reads\n%q\nwant\n%q", got, tt.want)
				}
				off := h.offKeys()
				if tt.width < 100 {
					off = h.phoneOffKeys()
				}
				if !slices.Equal(off, tt.wantOff) {
					t.Errorf("the key line draws %q faint, want %q", off, tt.wantOff)
				}
			})
		}
	}
}

// Help lists esc, ending the choice, while a session is chosen, on both
// sub-tabs, as the key line leaves it to help; with none chosen, or once esc
// ends the choice, it lists none.
func TestSessionsHelpListsEscWhileASessionIsChosen(t *testing.T) {
	for _, shown := range []dashboard.SessionsView{dashboard.SessionsSwitchboard, dashboard.SessionsList} {
		h := sessionsKeysHarness(t, routerDocument(three()...), shown, 160)
		for _, step := range []struct {
			name string
			do   func()
			want bool
		}{
			{name: "nothing chosen", do: func() {}},
			{name: "a session chosen", do: func() { h.model.sessionsTab.shown.Chosen = dashboard.Plug{Session: idD28C, Account: "work"} }, want: true},
			{name: "the choice ended", do: func() { h.keys(escKey) }},
		} {
			step.do()
			got := slices.ContainsFunc(helpOver(h.model), func(k dashboard.Key) bool { return k == dashboard.Key{Key: "esc", Does: "end the choice"} })
			if got != step.want {
				t.Errorf("on the %s, %s, help lists esc end the choice: %t, want %t", []string{"Switchboard", "List"}[shown], step.name, got, step.want)
			}
		}
	}
}

// enter is drawn faint exactly where pressing it opens nothing, as Opens
// says, on both sub-tabs: with nothing chosen, and once esc ends the choice,
// where pressed, it says why in the key line's place; with a session chosen,
// it opens its page.
func TestSessionsEnterIsFaintExactlyWhereItOpensNothing(t *testing.T) {
	for _, shown := range []dashboard.SessionsView{dashboard.SessionsSwitchboard, dashboard.SessionsList} {
		t.Run([]string{"Switchboard", "List"}[shown], func(t *testing.T) {
			h := sessionsKeysHarness(t, routerDocument(three()...), shown, 160)
			for _, tt := range []struct {
				name          string
				chosen, faint bool
				keys          []tea.KeyPressMsg
			}{
				{name: "nothing chosen", faint: true},
				{name: "a session chosen", chosen: true},
				{name: "the choice ended", chosen: true, faint: true, keys: []tea.KeyPressMsg{escKey}},
			} {
				h.model.sessionsTab.shown.Chosen = dashboard.Plug{}
				if tt.chosen {
					h.model.sessionsTab.shown.Chosen = dashboard.Plug{Session: idD28C, Account: "work"}
				}
				h.keys(tt.keys...)
				m := h.model
				faint := slices.Contains(h.offKeys(), "enter")
				h.keys(enterKey)
				opened := h.model.session != nil || h.model.opening > m.opening
				if faint != tt.faint || opened == faint {
					t.Errorf("%s, enter drawn faint %v, and pressed, opened %v; want it faint %v, exactly where it opens nothing", tt.name, faint, opened, tt.faint)
				}
				if said := h.model.noted(h.clock.now); faint && said != noSessionChosen {
					t.Errorf("%s, enter pressed says %q in the key line's place, want %q", tt.name, said, noSessionChosen)
				}
				h.model = m
			}
		})
	}
}

// Help on Sessions lists its keys as the key line words them, on both
// sub-tabs, enter among them while it's faint, with t, R and the rest.
func TestSessionsHelpListsItsKeysAsTheKeyLineWordsThem(t *testing.T) {
	for _, shown := range []dashboard.SessionsView{dashboard.SessionsSwitchboard, dashboard.SessionsList} {
		t.Run([]string{"Switchboard", "List"}[shown], func(t *testing.T) {
			h := sessionsKeysHarness(t, routerDocument(three()...), shown, 160)
			want := []dashboard.Key{
				{Key: "tab", Does: "views"}, {Key: "[ ]", Does: "view"}, {Key: "↑↓", Does: "session"},
				{Key: "enter", Does: "its page", Off: true}, {Key: "p", Does: "pin"}, {Key: "R", Does: "refresh"}, helpKey, dashboard.Quit,
			}
			if got := helpOver(h.model); !slices.Equal(got, want) {
				t.Errorf("help lists\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}
