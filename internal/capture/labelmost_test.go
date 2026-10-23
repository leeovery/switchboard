package capture

import (
	"regexp"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

// thirtyCellLabels are the labels the world of labels past a label's most
// gives the scene's accounts, by their ids, 30 cells each: work's with no
// mark, personal's beside the primary's ◆, side's beside ▲.
var thirtyCellLabels = map[string]string{
	"work":     "work-account-labelled-thirty-c",
	"personal": "personal-labelled-thirty-cells",
	"side":     "side-account-labelled-thirty-c",
}

// labelMost is the most cells a label is drawn in, its ellipsis among them.
const labelMost = 24

// sentences are the rows of sentences that name accounts: COMING UP's and
// LATELY's headings, folded to their first line, and their lines, each led
// by its time to the minute, as "16:30" or "~Fri 12:20", where the Log's
// rows are led by theirs to the second.
var sentences = regexp.MustCompile(`^\s*(COMING UP|LATELY|~?(\pL{3} )?\d\d:\d\d )`)

// thirtyCellNames is the scene's world, each account labelled as
// thirtyCellLabels has it.
func (a accountWorlds) thirtyCellNames() accountWorld {
	return a.kept("thirtyCellNames", func() accountWorld {
		w := a.scene()
		w.accounts = append(w.accounts[:0:0], w.accounts...)
		for i, account := range w.accounts {
			w.accounts[i].Label = thirtyCellLabels[account.ID]
		}
		return w
	})
}

// thirtyCellViews are the world of 30-cell labels on every view that names
// an account.
var thirtyCellViews = []page{
	{name: "thirty-overview", world: accountWorlds.thirtyCellNames, keys: keysOf("g", "s")},
	{name: "thirty-accounts-compare", world: accountWorlds.thirtyCellNames, keys: toAccounts()},
	{name: "thirty-accounts-detail", world: accountWorlds.thirtyCellNames, keys: toDetail(downKey)},
	{name: "thirty-history-year", world: accountWorlds.thirtyCellNames, keys: toHistory()},
	{name: "thirty-history-weeks", world: accountWorlds.thirtyCellNames, keys: toHistory("]")},
	{name: "thirty-history-days", world: accountWorlds.thirtyCellNames, keys: toHistory("]", "]")},
	{name: "thirty-history-tokens", world: accountWorlds.thirtyCellNames, keys: toHistory("]", "]", "]")},
	{name: "thirty-sessions", world: accountWorlds.thirtyCellNames, keys: toSessions()},
	{name: "thirty-sessions-list", world: accountWorlds.thirtyCellNames, keys: toSessions("]")},
	{name: "thirty-runway", world: accountWorlds.thirtyCellNames, keys: toRunway()},
	{name: "thirty-log-requests", world: accountWorlds.thirtyCellNames, keys: toLog()},
	{name: "thirty-log-events", world: accountWorlds.thirtyCellNames, keys: toLog("]")},
	{name: "thirty-session", world: accountWorlds.thirtyCellNames, keys: toSession("d28c"), scale: storyScale},
}

// No view cuts an account's label past a label's most, 24 cells, its
// ellipsis among them, at any of the owner's sizes, wherever it sits beside
// a name whose marks are wider than its own; and the primary's cut label
// keeps its ◆ after it wherever its row draws one. A sentence says a label
// whole and is cut at its edge, so the rows of sentences, COMING UP's and
// LATELY's, each led by its time to the minute, are passed over.
func TestNoLabelIsDrawnPastALabelsMost(t *testing.T) {
	for _, p := range thirtyCellViews {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			p.size = wide(37)
			f := p.fixture(worldsNow())
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			for _, size := range ownersSizes {
				for _, row := range fluidAt(m, size).rows {
					if sentences.MatchString(row) {
						continue
					}
					for id, label := range thirtyCellLabels {
						if cut, ok := cutPast(row, label); ok {
							t.Errorf("at %s, %s's label is cut at %d cells, past %d:\n%s", sizeName(size), id, cut, labelMost, row)
						}
					}
					primary := prose.Cut(thirtyCellLabels["personal"], labelMost)
					if strings.Contains(row, primary+" ") && strings.Contains(row, "◆") && !strings.Contains(row, primary+" ◆") {
						t.Errorf("at %s, personal's label is drawn without its ◆ after it:\n%s", sizeName(size), row)
					}
				}
			}
		})
	}
}

// cutPast reports the cells the label is cut at on the row, its ellipsis
// among them, where that's past a label's most.
func cutPast(row, label string) (int, bool) {
	for k := len(label) - 1; k >= labelMost; k-- {
		if strings.Contains(row, label[:k]+prose.Ellipsis) {
			return k + 1, true
		}
	}
	return 0, false
}
