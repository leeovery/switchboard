package watch

import (
	"maps"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// findGuardDays are the days the find's guarded world has run as it starts,
// in the evening: every one before today's head built, and its side
// standing.
const findGuardDays = 4

// The find's part: the Log's find opened in the evening, walked back through
// every past day the world ran, reads each day's side where it stands and
// no day's lines whose side does: here every past day's, so it reads none
// of their lines.
func TestTheFindReadsNoDayRawWhoseSideStands(t *testing.T) {
	start := soakAt(21, 0)
	g := &findGuard{sides: make(map[string]bool)}
	s := newSoak(t, soakSetup{start: start, days: findGuardDays, pace: guardPace, guard: g})
	s.show("log")
	s.typed("/")
	s.runUntil(start.Add(time.Minute))
	if found := s.h.model.log.page.Finding.Found; !found.Done() {
		t.Fatalf("the find still has %s to read a minute after it opened, want it done", found.Next)
	}
	read := slices.Sorted(maps.Keys(g.sides))
	for back := findGuardDays - 1; back > 0; back-- {
		if past := dayfile.DateOf(dayfile.DayStart(start, -back)); !g.sides[past] {
			t.Errorf("the find read the sides of %v, want every past day's among them, %s's", read, past)
		}
	}
	if len(g.raw) > 0 {
		t.Errorf("the find read the lines of %v, want none, every past day's side standing", g.raw)
	}
}

// findGuard holds the find to what it reads: the dates of the days whose
// sides stood as it read them, and of the days whose lines it read, in
// turn.
type findGuard struct {
	sides map[string]bool
	raw   []string
}

func (g *findGuard) reads(_ *soak, l views.Ledger) views.Ledger {
	return &countingLedger{Ledger: l, read: g.read}
}

// read notes a read of a day's side that stood, or of a day's lines.
func (g *findGuard) read(r ledgerRead) {
	switch r.method {
	case "DaySide":
		g.sides[r.asked[0]] = g.sides[r.asked[0]] || r.stood
	case "Day":
		g.raw = append(g.raw, r.asked[0])
	}
}

func (g *findGuard) took(tea.Msg, Model) {}
