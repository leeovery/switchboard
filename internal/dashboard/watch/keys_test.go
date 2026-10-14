package watch

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

func TestKeysThePageNoLongerTakesDoNothing(t *testing.T) {
	for _, key := range []string{"1", "3", "0", "a", "m", "Q", "W", "G", "T", "S", "J", "K", "A", "M", "U", "B"} {
		t.Run(key, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.start()
			asked := len(h.source.asked)

			msgs := h.press(key)
			h.deliver(msgs...)
			if quits(msgs) || len(h.source.orders) > 0 || len(h.source.asked) > asked || h.model.note != "" || h.model.open != nothingOpen ||
				h.model.sections != (dashboard.Sections{}) || h.model.by != dashboard.ByWindow || h.model.view != dashboard.Overview ||
				h.model.picker.open || h.model.focus != "work" {
				t.Errorf("%s acted: orders %q, read %d times, note %q, open %v, sections %v, by %v, showing %q, picker open %v, focus on %q; want nothing",
					key, h.source.orders, len(h.source.asked)-asked, h.model.note, h.model.open, h.model.sections.Names(), h.model.by, h.model.view, h.model.picker.open, h.model.focus)
			}
		})
	}
}

func TestTheKeyLineGroupsTheKeysOfEachTab(t *testing.T) {
	tests := []struct {
		view        dashboard.View
		lead, after int
		want        [][]string
		// wantPhone are the keys a phone's line keeps between tab views and
		// ? and q quit, and wantHelp ?'s word there.
		wantPhone []string
		wantHelp  string
	}{
		{view: dashboard.Overview, lead: 5, after: 5, want: [][]string{{"tab"}, {"u", "g", "r", "s", "c", "l", "b"}, {"↑↓", "p", "?", "q"}}, wantHelp: "keys and toggles"},
		{view: dashboard.Accounts, lead: 3, want: [][]string{{"tab"}, {"[ ]"}, {"< >"}, {"←→", "enter"}, {"p"}, {"?", "q"}}, wantPhone: []string{"[ ]"}, wantHelp: "keys"},
		{view: dashboard.History, lead: 3, want: [][]string{{"tab"}, {"[ ]"}, {"< >"}, {"v"}, {"p"}, {"?", "q"}}, wantPhone: []string{"[ ]"}, wantHelp: "keys"},
		{
			view: dashboard.Sessions, lead: 3, want: [][]string{{"tab"}, {"[ ]"}, {"↑↓", "enter"}, {"p"}, {"?", "q"}},
			wantPhone: []string{"↑↓", "enter", "[ ]"}, wantHelp: "keys",
		},
		{view: dashboard.Runway, lead: 3, want: [][]string{{"tab"}, {"w"}, {"p"}, {"?", "q"}}, wantPhone: []string{"w"}, wantHelp: "keys"},
		{
			view: dashboard.Log, lead: 3, want: [][]string{{"tab"}, {"[ ]"}, {"↑↓", "enter", "f", "/"}, {"p"}, {"?", "q"}},
			wantPhone: []string{"↑↓", "enter", "[ ]"}, wantHelp: "keys",
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.view), func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.model.view = tt.view
			h.start()

			l := h.model.live(h.clock.now).KeyLine
			var got [][]string
			for _, group := range l.Groups {
				got = append(got, listedKeys(group))
			}
			if l.Lead != tt.lead || l.After != tt.after || !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("the key line is %q, %d cells after tab views and %d after each group past it; want %q, %d and %d",
					got, l.Lead, l.After, tt.want, tt.lead, tt.after)
			}
			if phone := listedKeys(l.Phone); !slices.Equal(phone, tt.wantPhone) || l.PhoneHelp != tt.wantHelp {
				t.Errorf("a phone's line keeps %q, ? saying %q; want %q, ? saying %q", phone, l.PhoneHelp, tt.wantPhone, tt.wantHelp)
			}
		})
	}
}

func TestTheOverviewsKeyLineIsTheDesigns(t *testing.T) {
	pinned := routerDocument(three()[0])
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	toggles := "tab views     u usage  g graphs  r routing  s sessions  c coming up  l lately  b by: window     "
	phone := "tab views   ? keys and toggles   q quit"
	tests := []struct {
		name  string
		doc   status.Document
		width int
		want  string
	}{
		{name: "three accounts", doc: routerDocument(three()...), width: 160, want: toggles + "↑↓ focus  p pin  ? help  q quit"},
		{name: "one account, with none other to focus or pin to", doc: routerDocument(three()[0]), width: 160, want: toggles + "? help  q quit"},
		{name: "one account, pinned from the command line", doc: pinned, width: 160, want: toggles + "p pin  ? help  q quit"},
		{name: "too narrow for the wide line whole: a phone's", doc: routerDocument(three()...), width: 126, want: phone},
		{name: "a phone", doc: routerDocument(three()...), width: 52, want: phone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tt.doc)
			h.update(tea.WindowSizeMsg{Width: tt.width, Height: 40})
			h.start()

			rows := strings.Split(h.view(), "\n")
			if got := strings.TrimRight(rows[len(rows)-1], " "); got != tt.want {
				t.Errorf("at %d columns, the key line reads\n%q\nwant\n%q", tt.width, got, tt.want)
			}
		})
	}
}

func TestTheOverviewsKeyLineSaysWhichSectionsShowAndHowTheWindowsAreGrouped(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.typed("g", "u", "b")

	want := []dashboard.Key{
		{Key: "u", Does: "usage", Hidden: true}, {Key: "g", Does: "graphs"}, {Key: "r", Does: "routing"},
		{Key: "s", Does: "sessions", Hidden: true}, {Key: "c", Does: "coming up"}, {Key: "l", Does: "lately"},
		{Key: "b", Does: "by", State: "account"},
	}
	if got := h.model.live(h.clock.now).KeyLine.Groups[1]; !slices.Equal(got, want) {
		t.Errorf("with GRAPHS shown, USAGE hidden and the windows grouped by account, the toggles are\n%+v\nwant\n%+v", got, want)
	}
	if !slices.ContainsFunc(helpOver(h.model), func(k dashboard.Key) bool { return k == want[0] }) {
		t.Errorf("help lists %+v, want the toggles worded as the key line words them", helpOver(h.model))
	}
}
