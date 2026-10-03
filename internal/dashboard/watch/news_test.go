package watch

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// withEvents is the router's document of three accounts, telling of events
// with the ids given, the newest first.
func withEvents(ids ...int) status.Document {
	doc := routerDocument(three()...)
	for _, id := range ids {
		doc.Events = append(doc.Events, status.Event{ID: id, At: start.Add(-time.Duration(10-id) * time.Minute).UTC(), Kind: status.EventRoom, Account: "work"})
	}
	return doc
}

// fresh are the ids of the events picked out at now.
func (h *harness) fresh() []int {
	return slices.Sorted(maps.Keys(h.model.news.faded(h.clock.now)))
}

func TestTheFirstLookPicksOutNoEvent(t *testing.T) {
	h := routedHarness(t, withEvents(2, 1))
	h.start()

	if got := h.fresh(); len(got) > 0 {
		t.Errorf("the first look picked out %v, want none: there's no look before it to be newer than", got)
	}
}

func TestAnEventNewerThanTheLastLookSawIsPickedOutFadingBack(t *testing.T) {
	h := routedHarness(t, withEvents(2, 1))
	h.start()
	h.startRouter(withEvents(3, 2, 1))
	seen := h.tickUntilAsked()

	tests := []struct {
		name  string
		after time.Duration
		want  map[int]float64
	}{
		{name: "as the look sees it", after: 0, want: map[int]float64{3: 0}},
		{name: "half its time on, faded an eighth", after: highlightFor / 2, want: map[int]float64{3: 0.125}},
		{name: "its time over", after: highlightFor, want: nil},
	}
	for _, tt := range tests {
		if got := h.model.news.faded(seen.Add(tt.after)); !maps.Equal(got, tt.want) {
			t.Errorf("%s: faded = %v, want %v", tt.name, got, tt.want)
		}
	}
	if _, ok := h.pendingFrame(); !ok {
		t.Fatal("no frames drawn as the highlight fades")
	}
	h.settle()
	if got := h.clock.now; got.Before(seen.Add(highlightFor)) {
		t.Errorf("frames stopped at %s, want them drawn until the highlight has faded, at %s", got.Format(time.StampMilli), seen.Add(highlightFor).Format(time.StampMilli))
	}
	h.startRouter(withEvents(3, 2, 1))
	h.tickUntilAsked()
	if got := h.fresh(); len(got) > 0 {
		t.Errorf("a look seeing nothing newer picked out %v, want none", got)
	}
}

func TestEveryEventOfAnotherRouterIsNew(t *testing.T) {
	started := start.Add(-time.Hour).UTC()
	tests := []struct {
		name string
		to   router.Health
		want []int
	}{
		{name: "the same router", to: router.Health{PID: 7, StartedAt: started}, want: nil},
		{name: "another process", to: router.Health{PID: 8, StartedAt: started}, want: []int{1, 2}},
		{name: "the same process, restarted in place", to: router.Health{PID: 7, StartedAt: started.Add(time.Hour)}, want: []int{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, withEvents(9, 8))
			h.source.health = router.Health{PID: 7, StartedAt: started}
			h.start()

			h.source.health = tt.to
			h.startRouter(withEvents(2, 1))
			h.tickUntilAsked()
			if got := h.fresh(); !slices.Equal(got, tt.want) {
				t.Errorf("picked out %v, want %v: a router's ids start again from 1", got, tt.want)
			}
		})
	}
}

func TestAProbeBetweenLooksAtOneRouterForgetsNothingSeen(t *testing.T) {
	h := routedHarness(t, withEvents(2, 1))
	h.start()
	h.stopRouter()
	h.deliver(h.press("r")...)

	h.startRouter(withEvents(3, 2, 1))
	h.tickUntilAsked()
	if got, want := h.fresh(), []int{3}; !slices.Equal(got, want) {
		t.Errorf("picked out %v, want %v: the probe told of none, and the router is the one seen before", got, want)
	}
}

func TestAFreshEventIsPickedOutOnScreen(t *testing.T) {
	h, _ := themedHarness(t, theme.Choice{})
	h.startRouter(withEvents(2, 1))
	h.start()
	h.answer(darkBackground)
	h.startRouter(withEvents(3, 2, 1))
	h.tickUntilAsked()

	const attention = "48;2;61;64;70" // nord's bg.attention
	if row := rowWith(h.model.View().Content, "13:05"); !strings.Contains(row, attention) {
		t.Errorf("the new event's row is %q, want it on bg.attention", row)
	}
	h.clock.now = h.clock.now.Add(highlightFor)
	if row := rowWith(h.model.View().Content, "13:05"); strings.Contains(row, attention) {
		t.Errorf("once its time is over, the new event's row is %q, want it back on the canvas", row)
	}
}

// rowWith is the screen's first row that, without its escapes, holds text.
func rowWith(screen, text string) string {
	for row := range strings.SplitSeq(screen, "\n") {
		if strings.Contains(ansi.Strip(row), text) {
			return row
		}
	}
	return ""
}
