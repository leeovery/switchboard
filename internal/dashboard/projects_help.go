package dashboard

import (
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// projectsHelp are Projects' parts as help explains them, after its keys, in
// the page's order: the list; the project chosen, its numbers and sections;
// and SESSIONS.
func projectsHelp() []helpPart {
	return []helpPart{
		{
			heading: "PROJECTS",
			says: []line{
				said("A project is the repository a session ran in: a session in a worktree or a subdirectory counts as its " +
					"repository's, and one started outside any repository as its own directory's. Each is named by its folder, " +
					"its parent too where two share a name; the sessions that didn't start through switchboard's claude, " +
					"which name no directory, share a row of their own."),
				said("The list ranks each project with a session in the period by its worth at API prices, the greatest " +
					"first, its bar its worth against the top project's, split by model family in the families' colours, as " +
					"History's BY MONTH splits a month's. Live counts its sessions running now, and last says when its last " +
					"request came, now while one of them is busy. All projects totals them, a session counted once."),
				said("`↑` and `↓` choose the project its page under the list shows; the list scrolls with the choice past " +
					"a third of the view's rows."),
				said(periodsRun + " They're shared with Accounts: `<` and `>` step through " + prose.List(views.PeriodNames()) + "."),
			},
		},
		{
			heading: "THE PROJECT",
			says: []line{
				said("Its head gives its path and its sessions running; under it, its numbers over the period: its " +
					"sessions, requests and worth, its share of every project's worth, and its tokens."),
				said("WORTH BY charts its worth in the finest bucket that fits: today by hour; this week and 30 days by " +
					"day; 12 weeks by day where a column a day fits, else by week; and 12 months by week where a column " +
					"a week fits, else by month, the peak under it. BY MODEL is each version's share of its worth, those " +
					"under 1% left out, and ON ACCOUNTS each account's, the greatest first."),
				said("Where rows run short, ON ACCOUNTS leaves first, then BY MODEL, then WORTH BY, so SESSIONS keeps " +
					"three rows at least."),
			},
		},
		{
			heading: "SESSIONS",
			says: []line{
				said("The project's sessions over the period, in the List's columns: those running first, then the rest " +
					"under a rule for the day they ended, newest first, each started at its first request, with its day " +
					"where it began on another."),
				said("`→` takes the choice into them and `←` back to the list; `↑` and `↓` select one, and `enter` opens " +
					"its page, as the List does. The chosen row is lit where `↑` and `↓` act; the other's stays bold, " +
					"marked ▸."),
			},
		},
	}
}
