package views

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// grower is a reader that grows a session's story as its lines come, a line
// at a time, at the day given: whether it prices its requests, and the story
// it tells once it has taken in each line, in turn.
type grower struct {
	name   string
	priced bool
	grow   func(lines []ledger.Line, prices ledger.Table, day LedgerDay) []*story
}

// growers are the readers that grow a story of their own as the ledger's
// tail is read: a session's page held. The store, which seals what it
// holds, is held to the same in its own tests, and the Log tells its rows
// of the store's stories.
var growers = []grower{
	{name: "a session's page held", priced: true, grow: func(lines []ledger.Line, prices ledger.Table, day LedgerDay) []*story {
		h := HoldPage("s", SessionsRead{}, prices, 0, day.Start)
		var told []*story
		for _, l := range lines {
			h = h.Take(day.Date, []ledger.Held{{Line: l}})
			told = append(told, h.story)
		}
		return told
	}},
}

// givenOrders are the orders a generated session's lines are given in, by
// name: as the ledger files them, as each ends, and in an order of the
// seed's own.
func givenOrders(seed uint64) map[string][]ledger.Line {
	filed := generatedSession(seed)
	shuffled := slices.Clone(filed)
	r := rand.New(rand.NewPCG(seed, 2))
	r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	return map[string][]ledger.Line{"as filed": filed, "shuffled": shuffled}
}

// growingSeeds is how many generated sessions each grower is given.
const growingSeeds = 60

// Each reader that grows a story as its lines come tells, after each line,
// the story of every line it was given, read at once, in the one order:
// given as the ledger files them, and in an order of their own, the
// requests of the generated sessions arriving to the millisecond together,
// ending apart and together, their ids apart.
func TestAStoryGrownLineByLineTellsWhatOneReadAtOnceTells(t *testing.T) {
	prices, day := changingPrices(), Today(foldDays[1])
	for seed := range uint64(growingSeeds) {
		for order, lines := range givenOrders(seed) {
			wants := map[bool][]*story{false: atOnce(lines, prices, day, false), true: atOnce(lines, prices, day, true)}
			for _, g := range growers {
				grown, want := g.grow(lines, prices, day), wants[g.priced]
				if i := firstOtherwise(grown, want); i >= 0 {
					q := queryOf(lines)
					t.Errorf("%s, session %d given %s: after its line %d the story tells\n%+v\nwant, as read at once\n%+v", g.name, seed, order, i,
						toldOfHeld(grown[i], q), toldOfHeld(want[i], q))
				}
			}
		}
	}
}

// atOnce are the stories of lines read at once after each line, in turn:
// the story of every line given till then, in the one order, each request
// priced where priced says, at the day's prices.
func atOnce(lines []ledger.Line, prices ledger.Table, day LedgerDay, priced bool) []*story {
	var given []heldRequest
	stories := make([]*story, len(lines))
	for i := range lines {
		if isRequest(&lines[i]) {
			given = append(given, heldRequestOf(&lines[i]))
		}
		requests := slices.SortedStableFunc(slices.Values(given), inOrder)
		if priced {
			requests = pricedAt(requests, prices, day.Date)
		}
		stories[i] = storyOf(requests, prices, day)
	}
	return stories
}

// firstOtherwise is the place of the first of the stories grown that holds
// otherwise than the one wanted in its place, as sameStory has it: -1 where
// none does.
func firstOtherwise(grown, want []*story) int {
	for i := range grown {
		if !sameStory(grown[i], want[i]) {
			return i
		}
	}
	return -1
}

// sameStory reports whether two stories hold the same, told by the same
// prices: so whatever is asked of one, the other tells alike.
func sameStory(a, b *story) bool {
	x, y := *a, *b
	x.prices, y.prices = ledger.Table{}, ledger.Table{}
	return reflect.DeepEqual(x, y)
}

// The generated sessions, given as the growers are given them, hold the
// moments the one order decides: a line given after one of a request that
// arrived with it and ended later, and after one that arrived and ended
// with it, of a later id. So a grower that takes a line in after those
// that arrived with it, or by when it arrived alone, fails the test above.
func TestTheGeneratedSessionsGiveTiesOutOfOrder(t *testing.T) {
	moments := []string{"a line after one that arrived with it and ended later", "a line after one that arrived and ended with it, of a later id"}
	held := map[string]bool{}
	for seed := range uint64(growingSeeds) {
		for _, lines := range givenOrders(seed) {
			for i, l := range lines {
				for _, o := range lines[:i] {
					switch {
					case !o.At.Equal(l.At):
					case ended(&o).After(ended(&l)):
						held[moments[0]] = true
					case ended(&o).Equal(ended(&l)) && o.Request > l.Request:
						held[moments[1]] = true
					}
				}
			}
		}
	}
	for _, moment := range moments {
		if !held[moment] {
			t.Errorf("no generated session, as given, holds %s", moment)
		}
	}
}
