package cli

import (
	"strings"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// keysColumns is the width the dashboard's help wraps its keys to.
const keysColumns = 78

// keysSaid says, of each page the dashboard's help names, by its name, what
// its keys do where their words don't say it.
var keysSaid = map[string]string{
	"every page": "t and p work where they apply: t not under NO_COLOR, and p neither with one account, " +
		"unless a pin set from the command line stands, nor with --probe. p opens the routing card, which picks " +
		"the accounts new sessions go to and changes nothing until enter. j, k, PgDn, PgUp and the wheel scroll " +
		"a view where it doesn't fit: the Overview's USAGE, Accounts' views, History's Year, Weeks' accounts, " +
		"Days' BY ACCOUNT and Tokens' periods, Sessions' List and lanes, Runway's lanes, the Log's table, and " +
		"a session's turns, or an open turn's requests. w, where it's listed, shows and hides each model's own " +
		"window; one near its limit shows either way.",
	"Overview": "u, g, r, s, c and l show and hide USAGE, GRAPHS, ROUTING, SESSIONS, COMING UP and LATELY; " +
		"b groups the windows by window or by account, as the config's [usage] by has them until b is pressed; " +
		"v, while GRAPHS shows, flips its charts between the burn-up and the burn-down; and ↑↓ moves the focus " +
		"between the accounts, which GRAPHS charts, but by account from 150 columns, where GRAPHS charts every " +
		"account beside its block.",
	"Accounts": "< and > step the period both views count over: today, this week, 30 days and 12 weeks. " +
		"←→ moves along Compare's accounts, and enter opens one in Detail; there ↑↓ chooses an account, " +
		"→ moves into its SESSIONS and ← back, and enter opens the session chosen.",
	"History": "< and > step the Year back through each calendar year the ledger holds and forward again, " +
		"and Tokens' periods between a day, a week and a month; v cycles what the Year shows, all use, a model " +
		"version or an account, Weeks' shape, and how Tokens splits each period; and ←→ steps the Year shown " +
		"for one account between the accounts.",
	"Runway": "< and > move between the week and the 5-hour views; ← steps back to today, then a day at a " +
		"time, or to this week, then a week at a time, and → on again to the live view; and a collapses the " +
		"lanes' labels to each account's number, marks and use, giving the lanes more room, and expands them " +
		"again.",
	"Log": "end, or esc, follows the newest line again while the table is paused; enter opens the chosen " +
		"line's details, and s its session's page; f sets the columns and which lines show; / finds a session " +
		"by its id's start or its directory's name; and esc, with a filter on, clears it.",
	"a session's page": "esc goes back where the page was opened from, or closes the open turn; enter " +
		"opens the chosen turn into its requests, or the chosen request's details.",
}

// dashboardKeys are the dashboard's keys, a page's to a paragraph, as the
// watch lists them for its help, so the two never drift: each page's name,
// then its keys, each by its key and what it does, kept whole on a line; and
// under them, what keysSaid says of the page.
func dashboardKeys() string {
	var pages []string
	for _, p := range watch.EveryPagesKeys() {
		told := make([]string, len(p.Keys))
		for i, k := range p.Keys {
			told[i] = k.Key + " " + k.Does
			if i < len(p.Keys)-1 {
				told[i] += ","
			}
		}
		lines := wrapped("  "+p.Page+":", told)
		if said, ok := keysSaid[p.Page]; ok {
			lines = append(lines, wrapped("   ", strings.Fields(said))...)
		}
		pages = append(pages, strings.Join(lines, "\n"))
	}
	return strings.Join(pages, "\n\n")
}

// wrapped are parts run together a space apart on lines keysColumns wide at
// most, the first after lead, the rest four cells in, each part whole on a
// line.
func wrapped(lead string, parts []string) []string {
	const indent = "   "
	var lines []string
	line := lead
	for _, p := range parts {
		if utf8.RuneCountInString(line+" "+p) > keysColumns && line != lead && line != indent {
			lines, line = append(lines, line), indent
		}
		line += " " + p
	}
	return append(lines, line)
}
