package capture

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
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

// mixedLabels are the labels the mixed world gives the scene's accounts, by
// their ids: work's alone, of 31 cells, with no mark, beside personal's own,
// the primary's, and side's, ▲'s, so a move between work and side names a
// long name beside a short one.
var mixedLabels = map[string]string{
	"work": "work-account-of-thirty-one-cell",
}

// mostLabelled is the most cells an account's label is drawn in, its
// ellipsis among them, as the dashboard's column of names has it.
const mostLabelled = 24

// headed are the rows the guard passes over, as they say a name whole and
// are cut at the frame's edge, never as a label: COMING UP's and LATELY's
// lines, each led by its time to the minute, as "16:30" or "~Fri 12:20",
// where the Log's rows are led by theirs to the second; and a section's
// heading folded to what it holds, as "SESSIONS 6 running".
var headed = regexp.MustCompile(`^\s*(COMING UP|LATELY|SESSIONS|ROUTING|USAGE|~?(\pL{3} )?\d\d:\d\d )`)

// labelledWorld is a world of the scene's accounts, each labelled as labels
// has it, or its own where it has none, kept by the name given.
func (a accountWorlds) labelledWorld(name string, labels map[string]string) accountWorld {
	return a.kept(name, func() accountWorld {
		w := a.scene()
		w.accounts = append(w.accounts[:0:0], w.accounts...)
		for i, account := range w.accounts {
			if label, ok := labels[account.ID]; ok {
				w.accounts[i].Label = label
			}
		}
		return w
	})
}

// labelWorlds are the worlds the guard draws, each with the labels it gives
// past a label's most: every account's 30 cells, and work's alone 31.
var labelWorlds = []struct {
	name   string
	labels map[string]string
}{
	{name: "thirty", labels: thirtyCellLabels},
	{name: "mixed", labels: mixedLabels},
}

// labelledViews are the views that name an account, each named for what it
// shows: the Overview with every section, the routing card open, Compare,
// Detail of work and of all, History's four views, the Switchboard and the
// List, Runway, Graphs, the Log's requests and events, d28c's page, and its
// turn opened.
var labelledViews = []page{
	{name: "overview", keys: keysOf("s")},
	{name: "routing", keys: keysOf("p")},
	{name: "accounts-compare", keys: toAccounts()},
	{name: "accounts-detail", keys: toDetail(downKey)},
	{name: "accounts-detail-all", keys: toAccounts("]")},
	{name: "history-year", keys: toHistory()},
	{name: "history-weeks", keys: toHistory("]")},
	{name: "history-days", keys: toHistory("]", "]")},
	{name: "history-tokens", keys: toHistory("]", "]", "]")},
	{name: "sessions", keys: toSessions()},
	{name: "sessions-list", keys: toSessions("]")},
	{name: "runway", keys: toRunway()},
	{name: "graphs", keys: toGraphs()},
	{name: "log-requests", keys: toLog()},
	{name: "log-events", keys: toLog("]")},
	{name: "session", keys: toSession("d28c"), scale: storyScale},
	{name: "turn-opened", keys: toSession("d28c", downKey, enterKey), scale: storyScale},
}

// sayings are where a sentence or a heading names an account whole, each
// with %s where the label stands: the status line's best account; the
// legend's cap; Compare's verdict; Detail's picker and its head; and a line
// of LATELY's or a session's story beside the list, as "came from work: work
// came under pressure".
var sayings = []string{
	`new( sessions)? → ▲ %s`, `cap: %s at `, `time %s alone`, `^\s*%s alone`, `◂ \d+ %s ▸`, `│ +\d+ %s +●`,
	`%s reached its cap`, `%s came under`, `moved to %s`, `started on %s,`, `came from %s:`,
}

// unsaid is the row with each saying of the label given taken out.
func unsaid(row, label string) string {
	for _, s := range sayings {
		row = regexp.MustCompile(strings.ReplaceAll(s, "%s", regexp.QuoteMeta(label))).ReplaceAllString(row, "")
	}
	return row
}

// labelSizes are the sizes the guard draws each view at: the owner's, and a
// phone's narrowest, 52 columns.
var labelSizes = slices.Concat(ownersSizes, []watch.Size{{Width: 52, Height: 32}})

// No view draws an account's label past a label's most, 24 cells, its
// ellipsis among them, whole or cut, at any of labelSizes, wherever it sits
// beside a name whose marks are wider or narrower than its own, or beside a
// short name in a move; and the primary's cut label keeps its ◆ after it
// wherever its row draws one. The rows that say a name as a sentence or a
// folded heading, as headed has them, are passed over, and so are the
// sentences and headings sayings has naming a label whole.
func TestNoLabelIsDrawnPastALabelsMost(t *testing.T) {
	for _, w := range labelWorlds {
		for _, view := range labelledViews {
			p := view
			p.name, p.size = w.name+"-"+view.name, wide(37)
			p.world = func(a accountWorlds) accountWorld { return a.labelledWorld(w.name, w.labels) }
			t.Run(p.name, func(t *testing.T) {
				t.Parallel()
				f := p.fixture(worldsNow())
				m := withNothingOver(t, f.settle(f.Size), f.Size)
				for _, size := range labelSizes {
					for _, row := range fluidAt(m, size).rows {
						if headed.MatchString(row) {
							continue
						}
						for id, label := range w.labels {
							if drawn, ok := drawnPast(unsaid(row, label), label); ok {
								t.Errorf("at %s, %s's label is drawn in %d cells, past %d:\n%s", sizeName(size), id, drawn, mostLabelled, row)
							}
						}
						if primary, ok := w.labels["personal"]; ok {
							cut := prose.Cut(primary, mostLabelled)
							if strings.Contains(row, cut+" ") && strings.Contains(row, "◆") && !strings.Contains(row, cut+" ◆") {
								t.Errorf("at %s, personal's label is drawn without its ◆ after it:\n%s", sizeName(size), row)
							}
						}
					}
				}
			})
		}
	}
}

// drawnPast reports the cells the label is drawn in on the row, whole or
// cut, its ellipsis among them, where that's past a label's most.
func drawnPast(row, label string) (int, bool) {
	if strings.Contains(row, label) {
		return len(label), true
	}
	for k := len(label) - 1; k >= mostLabelled; k-- {
		if strings.Contains(row, label[:k]+prose.Ellipsis) {
			return k + 1, true
		}
	}
	return 0, false
}
