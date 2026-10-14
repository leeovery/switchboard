package watch

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// fromOnSide is the account the Switchboard's call of d28c on side says it
// came from, failing the test where it draws no such call.
func fromOnSide(h *harness) string {
	h.t.Helper()
	for _, c := range h.model.sessionsTab.shown.Calls {
		if c.Session == idD28C && c.Account == "side" {
			return c.From
		}
	}
	h.t.Fatalf("the Switchboard's calls are %+v, want d28c's on side", h.model.sessionsTab.shown.Calls)
	return ""
}

// A call says the account the move that brought it there left, ▸ from work,
// while that move is its latest, from the moment it's told of: once a
// listing shows it done, its cord faded or held by the limit that moved it,
// as the ledger has no line of its yet, its request in flight, its page's
// story telling the move all the while; then as its line lands, which tells
// it the same, the stream's move let go seenFor on.
func TestACallSaysWhereItCameFromTillItsLineTakesOver(t *testing.T) {
	onSide := []status.Session{sessionOn(idD28C, opus, "side", 0)}
	pinnedAndSent := []router.StreamEvent{pinnedToSide(0), told(router.StreamSent, "r1", "side", 0)}
	tests := []struct {
		name  string
		moved []router.StreamEvent
		// shows is when the move shows, and listed whether a look lists
		// d28c on side then, looseFor later.
		shows  time.Duration
		listed bool
	}{
		{name: "the moved moment, by a pin", moved: pinnedAndSent},
		{name: "3 s after a pin, listed on side", moved: pinnedAndSent, listed: true},
		{name: "moved by work's limit, listed on side at once", moved: movedToSide(0), shows: pulseFor, listed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			h, _ := sessionsOnTheLedger(t, state, start)
			h.hear(tt.moved...)
			h.clock.now = past(tt.shows)
			if tt.listed {
				h.source.sessions = onSide
				h.clock.now = h.clock.now.Add(looseFor)
			}
			h.read(routerDocument(three()...))
			if got := fromOnSide(h); got != "work" {
				t.Errorf("d28c on side, no line of its yet, says it came from %q, want work", got)
			}
			h.openPage(idD28C)
			if moves := h.shownPage().Story.Moves; len(moves) != 1 || moves[0].From != "work" || moves[0].To != "side" {
				t.Errorf("its page's story tells the moves %+v, want its move from work to side", moves)
			}
		})
	}

	t.Run("its line landing", func(t *testing.T) {
		state := t.TempDir()
		h, _ := sessionsOnTheLedger(t, state, start)
		h.hear(movedToSide(0)...)
		h.source.sessions = onSide
		h.clock.now = past(pulseFor)
		h.read(routerDocument(three()...))

		line := took(arrived("r1", "side", 0), 2*time.Second)
		line.From, line.Reason = "work", "moved: work hit its limit"
		writeLines(t, state, line)
		h.hear(alter(told(router.StreamDone, "r1", "side", 2*time.Second), func(e *router.StreamEvent) { e.Status = 200 }))
		h.clock.now = past(3 * time.Second)
		h.read(routerDocument(three()...))
		if got := fromOnSide(h); got != "work" {
			t.Errorf("its line landed, d28c on side says it came from %q, want work", got)
		}
		h.clock.now = past(seenFor + time.Minute)
		h.source.sessions = []status.Session{sessionOn(idD28C, opus, "side", -seenFor)}
		h.read(routerDocument(three()...))
		if got := fromOnSide(h); got != "work" {
			t.Errorf("the stream's move let go, d28c on side says it came from %q, want work, as its line tells", got)
		}
	})
}
