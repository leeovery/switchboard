package watch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestLATELYPicksOutAnEventNewToTheOverviewAlone(t *testing.T) {
	h, _ := themedHarness(t, theme.Choice{})
	h.startRouter(withEvents(2, 1))
	h.start()
	h.answer(darkBackground)

	const attention = "48;2;61;64;70" // nord's bg.attention
	if row := rowWith(h.model.View().Content, "13:04"); row == "" || strings.Contains(row, attention) {
		t.Errorf("on the first look, LATELY's newest row is %q, want it shown on the canvas: no look before it saw it new", row)
	}
	h.startRouter(withEvents(3, 2, 1))
	h.tickUntilAsked()
	if row := rowWith(h.model.View().Content, "13:05"); !strings.Contains(row, attention) {
		t.Errorf("the new event's row is %q, want it on bg.attention", row)
	}
	if row := rowWith(h.model.View().Content, "13:04"); strings.Contains(row, attention) {
		t.Errorf("the row seen before is %q, want it on the canvas", row)
	}
	h.clock.now = h.clock.now.Add(highlightFor)
	if row := rowWith(h.model.View().Content, "13:05"); strings.Contains(row, attention) {
		t.Errorf("once its time is over, the new event's row is %q, want it back on the canvas", row)
	}
}

func TestLATELYAsksForARouterFromBeforeEventsToBeRestarted(t *testing.T) {
	logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.source.historyErr = fmt.Errorf("%w: the router answered GET /history with 404 Not Found", router.ErrNoHistory)
	h.start()

	if !strings.Contains(h.view(), "restart the router for recent events") {
		t.Errorf("the screen is\n%s\nwant LATELY to ask for the router to be restarted", h.view())
	}
}
