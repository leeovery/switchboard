package dashboard

import (
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
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
	return []helpPart{f.compareHelp(), f.valueHelp(), allAccountsHelp(), f.detailHelp()}
}

// compareHelp is Compare as help explains it: its columns and heads, and
// their focus, but where Compare is turned, as CompareWide has it, holding
// none; its charts; each group and how each row is counted; and a key to the
// states its heads say.
func (f Frame) compareHelp() helpPart {
	focus := " `←` and `→` move the focus along the columns, scrolling them past as many as fit, all accounts staying " +
		"put at the right; `enter` opens the account focused in Detail, and `esc` ends the focus."
	charts := " WEEKS is how full each of its weeks got, at least the last eight, or every week the period begins where " +
		"there are more, a month its fullest week at 12 months; a dotted line marks the cap of an account that keeps " +
		"a reserve, and the axis marks this week, so far. Under them, how many of those weeks it hit its limit in, " +
		"and what they had left at a reset, on average; all accounts says how many weeks some account was short. " +
		"WORTH is what its use was worth at API prices over the period, in the finest buckets that fit, its peak " +
		"under it: the accounts share one scale, so their columns compare, and all accounts has its own."
	if !f.CompareWide() {
		focus = ""
	}
	return helpPart{
		heading: accountsHeading(AccountsCompare),
		says: []line{
			said("A column an account, in the config's order, and all accounts last; " +
				"under each name, how the account stands with the router, in a word." + focus),
			said("WEEK USED is each account's week window as switchboard usage draws it, its cap and even pace marked, and " +
				"always as it stands now, whatever the period. BY MODEL is each account's share of use over the period, a " +
				"row a version, its tokens weighed as worth weighs them, so cache reads, which run to millions and cost " +
				"little, count for little." + charts),
			said("ACTIVITY counts the period's requests; the sessions served, each counted once, however many days and " +
				"accounts it ran on; and the sessions moved onto each account and off it. LIMITS counts the limits each hit, " +
				"by window; the time it spent at its cap or at a limit; and what its week had left as it last reset, 100% " +
				"less the peak of the week that ended last, naming the day it ran out where it did. VALUE gives each " +
				"account's plan and its price a month, and its extra usage."),
		},
		glyphs: statesHelp(),
	}
}

// statesHelp are an account's states in a word, each as Compare's heads say
// it, in the order they're counted in, and what it means, as status says it.
func statesHelp() []glyph {
	var glyphs []glyph
	for s := status.StateOpen; s <= status.StateNotRead; s++ {
		glyphs = append(glyphs, glyph{drawn: stateWord(s), means: s.Means()})
	}
	return glyphs
}

// periodsRun is how the periods run, as Accounts' and Projects' helps say it.
const periodsRun = "The periods are never billing months, as switchboard knows no renewal date: today and this week run " +
	"from their starts, 30 days and 12 months back from today, and 12 weeks over this week and the 11 before."

// valueHelp is what the period's use was worth as help explains it: worth at
// API prices, as of the price table's date where the page is read; against
// its price, how a period is priced, and how all accounts sums the plans,
// where there's more than one account; extra usage; and a key to their
// marks.
func (f Frame) valueHelp() helpPart {
	asOf, plans, shared := "", " An account without a plan has none.", "."
	if page := f.Accounts.Page; page != nil {
		asOf = " as of " + pricesDated(page.PricesAsOf)
	}
	if !f.Accounts.Alone() {
		plans = " An account without a plan has none, and all accounts sums the plans it knows, its ratio counting only " +
			"their accounts' worth and saying how many plans it knows where it doesn't know them all, as the worth " +
			"beside it is every account's."
		shared = ", Compare and Detail sharing the period."
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
				"prorated: a day 12/365 of a month, a week 12/52, 30 days a month, 12 weeks twelve weeks' and 12 months twelve " +
				"months', a period still " +
				"running priced whole, and one the ledger began part-way through priced from its first day." + plans +
				" An account added part-way shows what was recorded of it, priced against the whole period."),
			said("Extra usage is on where Claude bills past the plan's limit, and off where the limit holds."),
			said(periodsRun + " `<` and `>` step through " + prose.List(views.PeriodNames()) + shared),
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
			said("In Detail, it has no BY DAY, and WEEKS gives a band of each account's week peaks, all accounts' first, " +
				"each week's peak the mean of theirs, the cap's level marked where an account keeps a reserve; at each " +
				"band's right, how many of the weeks shown it hit its limit in, or of all accounts how many some account " +
				"was short, and what they left at a reset, on average. Where the rows don't hold the bands, as on a " +
				"phone, a table gives the same, a row an account."),
		},
	}
}

// detailHelp is Detail as help explains it: the list, where there's more
// than one account; the panel, above the rule and below it; WORTH and LATELY;
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
				"are the period's, THIS WEEK counting its requests and the limits it hit, by window, beside its plan, its "+
				"worth against its price, and whether its extra usage is on. Below the rule, BY DAY and WEEKS cover the "+
				"weeks it names, as many as fit across the panel, the period's own at least where they fit, and never "+
				"fewer than six; at 12 months the newest, saying how many more there are."),
			said("BY DAY shades each day by how much of its week the account used that day, in History's five shades, on "+
				"the account's own scale: each a quarter of its active days."),
			said("WEEKS gives each week's peak, the use read just before the reset that ended it, and this week's, its "+
				"highest so far. Under them, how many of the whole weeks shown it hit its limit in, and what they left at "+
				"a reset, on average."),
			said("WORTH is what its use was worth at API prices over the period, across the panel, in the finest buckets "+
				"that fit, its peak under it. LATELY tells every event the router has for the account, newest first, "+
				"down to the key line, and the rest scrolls."),
			said(sessions),
		),
		glyphs: append(shadesHelp("a day", "days", f.byDaySwatch),
			glyph{drawn: f.daySwatch(dayInk(2), 2, true, f.detailDays()), means: daysLimitNote},
			glyph{drawn: dayToCome(f.detailDays()), means: "a day still to come"},
			glyph{drawn: line{{atItsCap, exhaustedInk}}, means: "a week it hit its limit"},
		),
	}
}

// byDaySwatch is a day of Detail's BY DAY at a level, as Days draws a day,
// as wide as Detail draws its days, as detailDays has them.
func (f Frame) byDaySwatch(level int) line {
	return f.daySwatch(dayInk(level), level, false, f.detailDays())
}

// accountsHeading is the heading help explains a sub-tab of Accounts under:
// its name, as the heading row names it, in capitals.
func accountsHeading(v AccountsView) string {
	return strings.ToUpper(accountsViews[v])
}
