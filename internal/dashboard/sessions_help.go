package dashboard

import (
	"strings"

	"github.com/leeovery/switchboard/internal/views"
)

// sessionsHelp are Sessions' parts as help explains them, after its keys, in
// the page's order, on both sub-tabs alike: the Switchboard's calls, with
// what a request is doing; its cords, with their marks; its lines; and the
// List.
func (f Frame) sessionsHelp() []helpPart {
	return []helpPart{
		{
			heading: "CALLS",
			says: []line{
				said("The Switchboard puts each session the router lists as running down the left, a call, grouped by the account " +
					"it's on, the accounts in the config's order; a session whose models are on two accounts has a call in each. " +
					"Calls keep their places from look to look: a new session, or one a move brings, joins the foot of its group, " +
					"which closes up over one that leaves."),
				said("A call names its session by its id and its directory's last part, the id bold while the session works: while " +
					"it asks, while it's answering, and while a refused request of its is retried. Then what it's doing, as the " +
					"marks below say; … 429 while a 429 is sent again on the same account; else idle, and for how long since its " +
					"last answer ended. Last, ▸ from the account it left, while the move that brought it is its latest."),
				said("`[` and `]` switch between the Switchboard and the List. `↑` and `↓` choose a session, a call here or a row " +
					"there, either of a session's two calls choosing it, and the choice follows it from look to look; `enter` opens " +
					"its page, and `esc` ends the choice."),
				said("While the router doesn't answer, the calls are the sessions it last listed, nothing travelling; with no " +
					"router, there are none."),
			},
			glyphs: []glyph{
				{drawn: line{rowMark(views.RequestRow{Mark: views.MarkAsking})}, means: "asking: its request sent, its answer not begun"},
				{drawn: line{rowMark(views.RequestRow{Mark: views.MarkAnswering})}, means: "answering"},
				{drawn: line{{refusedJack, exhaustedInk}}, means: "refused: a 429 at a limit, or a 401 or a 403, its request to go out again on another account"},
			},
		},
		{
			heading: "CORDS",
			says: []line{
				said("A cord runs from each call's plug to a jack on its account's line: along the call's row, down at its bend, " +
					"then along to the jack, the bends set so no two cords cross. Its colour says what's happening, never which " +
					"account: lit while its session works, dimmed at rest."),
				said("A request going out runs a pulse along it to the jack. An answer coming back shimmers along it, the jack lit, " +
					"and as it ends, a pulse runs home. A refusal marks the jack and runs a red pulse home; as it lands, the session " +
					"moves, joining the foot of its new account's group, its cord on that line's first free jack."),
				said("Where a limit forced the move, the old cord hangs from its jack while the limit holds, one for each session " +
					"the limit moved, the jack taken till then. A move by pressure, a pin or by hand leaves the old cord hanging a " +
					"moment, fading."),
				said("Where the terminal hasn't the width for the cords, as on a phone, the Switchboard draws lanes instead: each " +
					"account's line with its sessions under it, a session's words bold while it works."),
			},
			glyphs: []glyph{
				{drawn: line{{plugGlyph + strings.Repeat(heavyCord.along, 2) + pluggedJack, cordInk(true)}}, means: "a session's cord"},
				{drawn: line{{freeJack, faintInk}}, means: "a free jack"},
				{drawn: line{{hangingGlyph, errorInk}}, means: "a cord a limit let go"},
			},
		},
		{
			heading: "LINES",
			says: []line{
				said("Down the right, each account's line, as switchboard usage --by account prints it: its number, name and marks, " +
					"how it stands with the router in ROUTING's words, then a row a window, its bar, its percentage and its words. " +
					"Each of its rows has a jack; a line moves down to meet its calls, and grows a row for each session past its rows."),
				said("● pinned ends the name row of each account the global pin names, and ✕ out of the pool that of each it leaves " +
					"out, whose line is faded; an account closed by its cap or a limit never is, its words and colours saying why."),
				said("As rows run short, the lines compact as the Overview's sections fold: no blank rows within a line, then two " +
					"rows a line, then one, and past that they scroll, the calls with them."),
			},
		},
		{
			heading: strings.ToUpper(sessionsViews[SessionsList]),
			says: []line{
				said("Today's sessions, a row each, even one on two accounts: those running first, in the order they started, a new " +
					"one at their foot; then those ended today, the newest end first. A row moves only as its session ends, or as " +
					"it asks again, so rows never slide from under the choice. Its mark says it's asking or answering, or that it " +
					"moved to the account it's on."),
				said("On is the account its latest request went to, and model that request's model. Requests counts its requests " +
					"today that spend quota, and worth is theirs at API prices, at today's; routing says how it came to the account " +
					"it's on, and what writing its context again there cost. A session ends an hour after its last request, as the " +
					"router stops listing it, and runs again as it asks again."),
				said("The today line sums today's sessions, running and ended: their requests today, and their worth."),
			},
		},
	}
}
