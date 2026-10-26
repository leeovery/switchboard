package dashboard

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// numberedTab is a tab that numbers accounts, drawn of a numbering: its
// frame, and the ids of the accounts it names, each of which it must number.
type numberedTab struct {
	name  string
	frame Frame
	names []string
}

// numberedTabs are the tabs that number accounts, each drawn of n's
// document at printNow, wide and tall enough to show every account: the
// Overview, Runway's week, Sessions and Graphs' WEEK, 5-HOUR and GLANCE, of
// the router's accounts or the config's as each names them, and Accounts'
// Compare and Detail, of the config's.
func numberedTabs(n numbering) []numberedTab {
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
	graphs.Graphs.Sections = GraphsSectionsNamed([]string{"week", "5-hour", "glance"})
	graphs.Graphs.Accounts = n.config()
	compare.View, compare.Accounts = Accounts, AccountsShown{Shown: AccountsCompare, Accounts: n.config()}
	detail.View, detail.Accounts = Accounts, AccountsShown{Shown: AccountsDetail, Accounts: n.config()}
	return []numberedTab{
		{name: "the Overview", frame: overview, names: held},
		{name: "Sessions", frame: sessions, names: held},
		{name: "Runway", frame: runway, names: held},
		{name: "Graphs", frame: graphs, names: n.ids},
		{name: "Compare", frame: compare, names: n.ids},
		{name: "Detail", frame: detail, names: n.ids},
	}
}

// numbersOf are the numbers the rows give the account labelled as given,
// wherever a number heads its label: the number, a space, then the label,
// whole.
func numbersOf(rows []string, label string) []string {
	headed := regexp.MustCompile(`(?:^|[\s│▸])(\d+) ` + regexp.QuoteMeta(label) + `(?:$|[\s◆▲…])`)
	var numbers []string
	for _, row := range rows {
		for _, match := range headed.FindAllStringSubmatch(row, -1) {
			numbers = append(numbers, match[1])
		}
	}
	return numbers
}

// numberWanted is the number n gives the account with the given id, as
// Runway numbers it where the document holds it, else after the document's
// last.
func (n numbering) numberWanted(id string) string {
	label, _ := n.want(id)
	number, _, _ := strings.Cut(strings.TrimSpace(label), " ")
	return number
}

// While the config and the router disagree, as after an account is removed
// and another added while the router waits to restart, every tab that
// numbers accounts gives each the one number, the key that picks it on the
// routing card: Runway's, by the router's place, else after its last, in
// the config's order among those it lacks. So with personal removed from the
// config, side is still 3 on Accounts as on Runway, and client, added, is 4
// wherever it's named.
func TestEveryTabNumbersAnAccountAsTheRoutingCardKeysIt(t *testing.T) {
	for _, tt := range numberings() {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.doc.At(printNow, printPolicy)
			for _, row := range RoutingRows(m, nil, views.Stream{})[1:] {
				want := tt.numberWanted(row.Account)
				if n, _ := strconv.Atoi(want); n > numbered {
					want = ""
				}
				if row.Key != want {
					t.Errorf("the routing card keys %s %q, want %q", row.Account, row.Key, want)
				}
			}
			for _, tab := range numberedTabs(tt) {
				rows := textOf(tab.frame.Draw(tt.doc, printNow))
				for _, id := range tab.names {
					numbers, want := numbersOf(rows, id), tt.numberWanted(id)
					if len(numbers) == 0 {
						t.Errorf("%s doesn't number %s; want it numbered %s", tab.name, id, want)
					}
					for _, got := range numbers {
						if got != want {
							t.Errorf("%s numbers %s %s, want %s", tab.name, id, got, want)
						}
					}
				}
			}
		})
	}
}

// The layouts that fit names to a column lay them out numbered as they're
// drawn: so where a number reaches 10 while the config holds nine accounts,
// as the router holds ten and the first is removed from the config, GLANCE's
// name column and Detail's list make room for its two digits, a cell more
// than numbers from 1 to 9 take. Each label is long enough that the widest
// name sets Detail's list's column, not its least.
func TestLayoutsFitTheNumbersTheyDraw(t *testing.T) {
	doc, config := tenHeld(), configured(tenIDs[1:]...)
	for i := range config {
		config[i].Label += "-team"
	}
	f := Frame{Width: 300, Height: 90, Policy: printPolicy, Accounts: AccountsShown{Accounts: config}}
	held := f.At(doc, printNow)
	drawn := namesAt(held.Moment(doc, printNow).Configured(config), len(tenIDs))
	if got := configuredNames(held.numbering(), config); !sameNumbers(got, drawn) {
		t.Errorf("GLANCE lays its names out numbered %q, want %q, as drawn", numbersIn(got), numbersIn(drawn))
	}
	if laid, bare := layGlance(held.numbering(), config, 300), layGlance(status.Numbering{}, config, 300); laid.column != bare.column+1 {
		t.Errorf("GLANCE's name column is %d cells, want a cell more than the %d of numbers from 1 to 9", laid.column, bare.column)
	}
	if laid, bare := held.listNames(), f.listNames(); laid != bare+1 {
		t.Errorf("Detail's list takes %d cells, want a cell more than the %d of numbers from 1 to 9", laid, bare)
	}
}

// sameNumbers reports whether two runs of names are numbered alike.
func sameNumbers(a, b []accountName) bool {
	return strings.Join(numbersIn(a), ",") == strings.Join(numbersIn(b), ",")
}

// numbersIn are the names' numbers, in their order.
func numbersIn(names []accountName) []string {
	numbers := make([]string, len(names))
	for i, n := range names {
		numbers[i] = n.number
	}
	return numbers
}
