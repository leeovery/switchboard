package watch

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
)

func TestKeysThePageNoLongerTakesDoNothing(t *testing.T) {
	for _, key := range []string{"1", "3", "0", "a", "m", "r", "Q", "W", "G", "T", "S", "J", "K", "A", "M"} {
		t.Run(key, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.start()
			asked := len(h.source.asked)

			msgs := h.press(key)
			h.deliver(msgs...)
			if quits(msgs) || len(h.source.orders) > 0 || len(h.source.asked) > asked || h.model.note != "" || h.model.open != nothingOpen ||
				h.model.featured != dashboard.Auto || h.model.chart != dashboard.Burndown || h.model.view != dashboard.Overview ||
				h.model.picker.open || h.model.flipped != nil || h.model.focus != "" {
				t.Errorf("%s acted: orders %q, read %d times, note %q, open %v, featuring %q, drawing %q, showing %q, picker open %v, flipped %v, focus on %q; want nothing",
					key, h.source.orders, len(h.source.asked)-asked, h.model.note, h.model.open, h.model.featured, h.model.chart, h.model.view, h.model.picker.open, h.model.flipped, h.model.focus)
			}
		})
	}
}

func TestTheKeyLineGroupsTheKeysOfEachTab(t *testing.T) {
	tests := []struct {
		view dashboard.View
		lead int
		want [][]string
	}{
		{view: dashboard.Overview, lead: 5, want: [][]string{{"tab"}, {"w", "←→", "space", "g"}, {"p"}, {"?", "q"}}},
		{view: dashboard.Accounts, lead: 3, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}},
		{view: dashboard.History, lead: 3, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}},
		{view: dashboard.Sessions, lead: 3, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}},
		{view: dashboard.Runway, lead: 3, want: [][]string{{"tab"}, {"w"}, {"p"}, {"?", "q"}}},
		{view: dashboard.Log, lead: 3, want: [][]string{{"tab"}, {"p"}, {"?", "q"}}},
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
		})
	}
}
