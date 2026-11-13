package dashboard

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// namingTab is a tab that names accounts, drawn of a disagreement: its
// frame, and the ids of the accounts it names.
type namingTab struct {
	name  string
	frame Frame
	names []string
}

// namingTabs are the tabs that name accounts, each drawn of n's document at
// printNow, wide and tall enough to show every account: the Overview,
// Runway's week, Sessions and Graphs' WEEK and GLANCE, of the
// router's accounts or the config's as each names them, and Accounts'
// Compare and Detail, of the config's.
func namingTabs(n disagreement) []namingTab {
	const width, height = 300, 90
	m := n.doc.At(printNow, printPolicy)
	held := make([]string, len(m.Accounts))
	for i, a := range m.Accounts {
		held[i] = a.ID
	}
	base := Frame{Width: width, Height: height, Policy: printPolicy}
	overview, sessions, runway, compare, detail := base, base, base, base, base
	overview.View, sessions.View = Overview, Sessions
	tl := views.LiveTimeline(views.RunwayWeek, printNow, RunwayShown{}.Columns(width))
	lanes := views.Lanes{}.Look(views.LanesFrom{Moment: m, Timeline: tl, Window: views.RunwayWeek})
	runway.View, runway.Runway = Runway, RunwayShown{Window: views.RunwayWeek, Timeline: tl, Lanes: &lanes}.Labelled(m)
	graphs := cardsPage(n.doc, printNow, width, height, n.ids...)
	graphs.Graphs.Sections = GraphsSectionsNamed([]string{"week", "glance"})
	graphs.Graphs.Accounts = n.config()
	compare.View, compare.Accounts = Accounts, AccountsShown{Shown: AccountsCompare, Accounts: n.config()}
	detail.View, detail.Accounts = Accounts, AccountsShown{Shown: AccountsDetail, Accounts: n.config()}
	return []namingTab{
		{name: "the Overview", frame: overview, names: held},
		{name: "Sessions", frame: sessions, names: held},
		{name: "Runway", frame: runway, names: held},
		{name: "Graphs", frame: graphs, names: n.ids},
		{name: "Compare", frame: compare, names: n.ids},
		{name: "Detail", frame: detail, names: n.ids},
	}
}

// namedIn are the names the rows give the account labelled as given, each
// with what heads it on its row: a number and a space before the label, as
// "3 side", where a number heads it, else the label alone.
func namedIn(rows []string, label string) []string {
	named := regexp.MustCompile(`(?:^|[\s│▸])((?:\d+ )?` + regexp.QuoteMeta(label) + `)(?:$|[\s◆▲…])`)
	var names []string
	for _, row := range rows {
		for _, match := range named.FindAllStringSubmatch(row, -1) {
			names = append(names, match[1])
		}
	}
	return names
}

// Nothing numbers an account: every tab that names accounts names each by
// its label alone, whatever the config and the router hold, as after an
// account is removed and another added while the router waits to restart.
func TestEveryTabNamesAnAccountWithoutANumber(t *testing.T) {
	for _, tt := range disagreements() {
		t.Run(tt.name, func(t *testing.T) {
			for _, tab := range namingTabs(tt) {
				rows := textOf(tab.frame.Draw(tt.doc, printNow))
				for _, id := range tab.names {
					names := namedIn(rows, id)
					if len(names) == 0 {
						t.Errorf("%s doesn't name %s", tab.name, id)
					}
					if numbered := slices.DeleteFunc(names, func(n string) bool { return n == id }); len(numbered) > 0 {
						t.Errorf("%s names %s as %q, want its label alone", tab.name, id, strings.Join(numbered, ", "))
					}
				}
			}
		})
	}
}
