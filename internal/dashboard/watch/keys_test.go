package watch

import (
	"slices"
	"testing"

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
				h.model.picker.open || h.model.focus != "" {
				t.Errorf("%s acted: orders %q, read %d times, note %q, open %v, sections %v, by %v, showing %q, picker open %v, focus on %q; want nothing",
					key, h.source.orders, len(h.source.asked)-asked, h.model.note, h.model.open, h.model.sections.Names(), h.model.by, h.model.view, h.model.picker.open, h.model.focus)
			}
		})
	}
}

func TestTheKeyLineGroupsTheKeysOfEachTab(t *testing.T) {
	tests := []struct {
		view dashboard.View
		lead int
		want [][]string
		// wantPhone are the keys a phone's line keeps between tab views and
		// ? and q quit, and wantHelp ?'s word there.
		wantPhone []string
		wantHelp  string
	}{
		{view: dashboard.Overview, lead: 5, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}, wantHelp: "keys and toggles"},
		{view: dashboard.Accounts, lead: 3, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}, wantHelp: "keys"},
		{view: dashboard.History, lead: 3, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}, wantHelp: "keys"},
		{view: dashboard.Sessions, lead: 3, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}, wantHelp: "keys"},
		{view: dashboard.Runway, lead: 3, want: [][]string{{"tab"}, {"w"}, {"p"}, {"?", "q"}}, wantPhone: []string{"w"}, wantHelp: "keys"},
		{view: dashboard.Log, lead: 3, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}, wantHelp: "keys"},
	}
	for _, tt := range tests {
		t.Run(string(tt.view), func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.model.view = tt.view
			h.start()

			l := h.model.keyLine(h.clock.now)
			var got [][]string
			for _, group := range l.Groups {
				got = append(got, listedKeys(group))
			}
			if l.Lead != tt.lead || !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("the key line is %q, %d cells after tab views; want %q, %d after", got, l.Lead, tt.want, tt.lead)
			}
			if phone := listedKeys(l.Phone); !slices.Equal(phone, tt.wantPhone) || l.PhoneHelp != tt.wantHelp {
				t.Errorf("a phone's line keeps %q, ? saying %q; want %q, ? saying %q", phone, l.PhoneHelp, tt.wantPhone, tt.wantHelp)
			}
		})
	}
}

func TestTheEmptyTabsListPPinOnlyWhereThereIsAPinToSet(t *testing.T) {
	pinned := routerDocument(three()[0])
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	tests := []struct {
		name string
		doc  status.Document
		want [][]string
	}{
		{name: "three accounts", doc: routerDocument(three()...), want: [][]string{{"tab"}, {"p"}, {"?", "q"}}},
		{name: "one account", doc: routerDocument(three()[0]), want: [][]string{{"tab"}, {"?", "q"}}},
		{name: "one account, pinned from the command line", doc: pinned, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}},
	}
	for _, tt := range tests {
		for _, view := range []dashboard.View{dashboard.Accounts, dashboard.History, dashboard.Log} {
			t.Run(tt.name+" on "+string(view), func(t *testing.T) {
				h := routedHarness(t, tt.doc)
				h.model.view = view
				h.start()

				var got [][]string
				for _, group := range h.model.keyLine(h.clock.now).Groups {
					got = append(got, listedKeys(group))
				}
				if !slices.EqualFunc(got, tt.want, slices.Equal) {
					t.Errorf("the key line is %q, want %q", got, tt.want)
				}
			})
		}
	}
}
