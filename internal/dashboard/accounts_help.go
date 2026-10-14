package dashboard

import (
	"strconv"
	"strings"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// accountsHelp are Accounts' parts as help explains them, after its keys, in
// the design's order: Compare, its columns and how each row is counted; the
// value of the period's use, worth and against its price; what all accounts
// counts; and Detail. With one account, Compare and all accounts go, as they
// do from the page.
func (f Frame) accountsHelp() []helpPart {
	if f.Accounts.Alone() {
		return []helpPart{f.valueHelp(), f.detailHelp()}
	}
	return []helpPart{compareHelp(), f.valueHelp(), allAccountsHelp(), f.detailHelp()}
}

// compareHelp is Compare as help explains it: its columns and heads, each
// group and how each row is counted, and a key to the states its heads say.
func compareHelp() helpPart {
	return helpPart{
		heading: accountsHeading(AccountsCompare),
		says: []line{
			said("A column an account, numbered by its place in the config, the routing picker's key, and all accounts last; " +
				"under each name, how the account stands with the router, in a word. `←` and `→` move the focus along the " +
				"columns, scrolling them past as many as fit, all accounts staying put at the right; `enter` opens the " +
				"account focused in Detail, and `esc` ends the focus."),
			said("WEEK USED is each account's week window as switchboard usage draws it, its cap and even pace marked, and " +
				"always as it stands now, whatever the period. BY MODEL is each account's share of use over the period, a " +
				"row a version, its tokens weighed as worth weighs them, so cache reads, which run to millions and cost " +
				"little, count for little."),
			said("ACTIVITY counts the period's requests; the sessions served, each counted once, however many days and " +
				"accounts it ran on; and the sessions moved onto each account and off it. LIMITS counts the limits each hit, " +
				"by window; the time it spent at its cap or at a limit; and what its week had left as it last reset, 100% " +
				"less the peak of the week that ended last, naming the day it ran out where it did. VALUE gives each " +
				"account's plan and its price a month, and whether extra usage is on, Claude billing past the limit."),
		},
		glyphs: statesHelp(),
	}
}

// statesHelp are an account's states in a word, each as Compare's heads say
// it, in the order they're counted in, and what it means.
func statesHelp() []glyph {
	means := map[status.AccountState]string{
		status.StateOpen:     "new sessions can go to it, though a limit may hold back some models",
		status.StatePressure: "at its recent rate, it runs out before its window resets",
		status.StateAtCap:    "at its cap, where its reserve starts: the router passes it over, unless a pin spends it",
		status.StateLimit:    "a limit holds it back from every request",
		status.StateRefused:  "its token was refused",
		status.StateNotRead:  "its usage isn't read: it has no usable token, or its usage couldn't be read or hasn't been yet",
	}
	glyphs := make([]glyph, 0, len(means))
	for s := status.StateOpen; s <= status.StateNotRead; s++ {
		glyphs = append(glyphs, glyph{drawn: stateWord(s), means: means[s]})
	}
	return glyphs
}

// valueHelp is what the period's use was worth as help explains it: worth at
// API prices, as of the price table's date where the page is read; against
// its price, and how a period is priced; and a key to their marks.
func (f Frame) valueHelp() helpPart {
	asOf := ""
	if page := f.Accounts.Page; page != nil {
		asOf = " as of " + pricesDated(page.PricesAsOf)
	}
	more, less := 4.0, 0.6
	return helpPart{
		heading: "VALUE",
		says: []line{
			said("Worth at API prices is what the same tokens would have cost through the API, each version at its own " +
				"prices, at the price table's prices" + asOf + ", worked out as it's read. A + after it is part of it unpriced, " +
				"of a model the price table doesn't know or of requests that went upstream without their usage; unpriced, " +
				"all of it."),
			said("Against its price is that worth over what the plans cost over the same period, a plan's price a month " +
				"prorated: a day 12/365 of a month, a week 12/52, 30 days a month, and 12 weeks twelve weeks', a period still " +
				"running priced whole. An account without a plan has none, and all accounts sums the plans it knows, its " +
				"ratio counting only their accounts' worth. An account added part-way shows what was recorded of it, " +
				"priced against the whole period."),
			said("The periods are rolling, never billing months, as switchboard knows no renewal date: `<` and `>` step " +
				"through today, this week, the last 30 days and the last 12 weeks, Compare and Detail sharing the period."),
		},
		glyphs: []glyph{
			{drawn: line{{"$184", secondaryInk}, {views.PartUnpriced, faintInk}}, means: "worth, part of it unpriced"},
			{drawn: againstSaid(&more), means: "against its price at 1× or more: the plans more than paid for"},
			{drawn: againstSaid(&less), means: "under 1×: worth less than the plans cost"},
			{drawn: line{noFigure}, means: "nothing recorded, as on a fresh install, or not known"},
		},
	}
}

// allAccountsHelp is all accounts as help explains it: what it counts, and
// why it isn't the pool.
func allAccountsHelp() helpPart {
	return helpPart{
		heading: "ALL ACCOUNTS",
		says: []line{
			said("Every configured account, always, whatever is pinned, at its limit or refused: its totals count them all, " +
				"a session served by two of them counted once. A window's use is the mean of theirs and its pace the mean " +
				"of their paces, and its room, as 1.2 of 3 left, the sum of what each has left of it, to its limit. Its " +
				"accounts' states are counted, as 2 open · 1 at cap, or where that's too wide, how many are open, pressure " +
				"counting as open."),
			said("It isn't the pool, the accounts new sessions can go to now, each to its cap, which only the Overview " +
				"shows: an account's state on Accounts is its own."),
			said("In Detail, its days are every account's together, marked where any hit a 5-hour limit, and each week's " +
				"peak the mean of theirs. Its verdict is " + views.RoomToSpare + " where their weeks left " +
				strconv.Itoa(views.RoomLeft) + "% or more at a reset, on average, else " + views.LittleToSpare + ", naming " +
				"the accounts that end weeks with the most and the least left."),
		},
	}
}

// detailHelp is Detail as help explains it: the list, where there's more
// than one account; the panel, above the rule and below it; the verdict;
// SESSIONS; and a key to BY DAY's shades and marks and WEEKS'.
func (f Frame) detailHelp() helpPart {
	var says []line
	if !f.Accounts.Alone() {
		says = append(says, said("The list chooses the account the panel shows, or all accounts, with `↑` and `↓`."))
	}
	sessions := "With sessions running on the account, SESSIONS lists them: `→` takes the choice into them and `←` back, " +
		"`↑` and `↓` select one there, and `enter` opens its page, from the list too."
	if f.Accounts.Alone() {
		sessions = "With sessions running on it, SESSIONS lists them: `↑` and `↓` select one, and `enter` opens its page."
	}
	return helpPart{
		heading: accountsHeading(AccountsDetail),
		says: append(says,
			said("The head says how the account stands with the router, and where new sessions go. Above the rule, USAGE "+
				"is now, every window read as switchboard usage reads them; THIS WEEK, named for the period, and BY MODEL "+
				"are the period's. Below it, BY DAY and WEEKS always cover the last six weeks."),
			said("BY DAY shades each day by how much of its week the account used that day, in History's five shades, on "+
				"the account's own scale: each a quarter of its active days."),
			said("WEEKS gives each week's peak, the use read just before the reset that ended it, and this week's, its "+
				"highest so far. Under them, the verdict on the last 7 whole weeks, as History's Weeks gives it: "+
				accountVerdictsMean()),
			said(sessions),
		),
		glyphs: append(shadesHelp(f.byDaySwatch),
			glyph{drawn: f.swatch(dayInk(2), 2, true), means: daysLimitNote},
			glyph{drawn: line{{toCome, faintInk}}, means: "a day still to come"},
			glyph{drawn: line{{"100", exhaustedInk}}, means: "a week it hit its limit"},
		),
	}
}

// byDaySwatch is a day of Detail's BY DAY at a level, as Days draws a day.
func (f Frame) byDaySwatch(level int) line {
	return f.swatch(dayInk(level), level, false)
}

// accountsHeading is the heading help explains a sub-tab of Accounts under:
// its name, as the heading row names it, in capitals.
func accountsHeading(v AccountsView) string {
	return strings.ToUpper(accountsViews[v])
}
