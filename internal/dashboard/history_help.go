package dashboard

import (
	"fmt"
	"strings"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// historyHelp are History's parts as help explains them, after its keys, in
// the design's order: the Year and its five shades, BY MONTH, Weeks, Days
// and Tokens; each what it shows and how to read it, then a key to its marks,
// each drawn as the page draws it.
func (f Frame) historyHelp() []helpPart {
	return []helpPart{
		{
			heading: subTabHeading(HistoryYear),
			says: []line{
				said("A day a square, a week a column, over the last 12 months, or a calendar year, which `<` and `>` step through. " +
					"A day is shaded by how many requests it made, never by its worth, which shifts with the models used and their " +
					"prices. A day with nothing on it has a shade of its own, and the active days are split into quarters by how much " +
					"each did, on one scale for everything shown together, the key giving each quarter's bounds."),
				said("`v` shows one model version alone, in four steps of its own colour, or one account, `←` and `→` stepping " +
					"between the accounts; the shades, their key and the numbers re-scale to what's shown."),
			},
			glyphs: shadesHelp(f.yearDay),
		},
		{
			heading: "BY MONTH",
			says: []line{
				said("Each month's requests, its worth at API prices, what the plans cost that month and its worth against it, " +
					"and the limits reached. Against is bold, in the theme's positive colour at 1× or more, the plans more than " +
					"paid for, and its attention colour under. A bar is a month's worth against the largest month's, split by " +
					"family, each in its colour, as the dots under the bars name them. The month still running says so far after " +
					"its bar, and the month the ledger began recording in the day it began, as from the 16th, its plans priced " +
					"from then."),
			},
			glyphs: []glyph{
				{drawn: familyBar(), means: "a month's worth, by family"},
				{drawn: line{noFigure}, means: "against, for the month so far"},
			},
		},
		{
			heading: subTabHeading(HistoryWeeks),
			says: []line{
				said("A week is an account's own, from one reset of its week window to the next, under the calendar week it " +
					"began nearest. Its peak is the use read just before the reset that ended it, and this week's its highest " +
					"so far. A week is short where some account hit its limit, its peak at 100%."),
				said("The replay takes each of the last 7 whole weeks alone. Without an account, its peak, times its plan's " +
					"size, is shared among the rest by their sizes; with one more, of the plan most of the accounts have, every " +
					"peak falls in proportion, the same use over more room. A replayed week is short where any account would " +
					"reach 100%."),
				said("The verdict: " + views.OneMoreThanYouNeed + ", where one fewer would be short on no more weeks than now, " +
					"naming the account to drop; " + views.OneTooFew + ", where now is short on 4 or more of the 7 and one more " +
					"would halve that; " + views.TooFew + ", where one more wouldn't, the row under it saying how short one more " +
					"would still be; else " + views.AboutRight + "."),
				said("An account's: " + accountVerdictsMean()),
				said("`v` draws each week as a column filled to its peak, each row in the bar's colour at its height."),
			},
			glyphs: []glyph{
				{drawn: line{{"100%", exhaustedInk}}, means: "it hit its limit"},
				{drawn: line{{"34%…", strongInk}}, means: "this week, so far"},
				{drawn: line{{"▁▁▁▁▁", faintInk}}, means: "a week with no use"},
				{drawn: line{noFigure}, means: "a week whose peak isn't known"},
			},
		},
		{
			heading: subTabHeading(HistoryDays),
			says: []line{
				said("Eight weeks, the week so far and the seven before it, two cells a day, on two scales of their own. BY " +
					"MODEL is each family's worth a day at API prices, every account's, in four steps of the family's colour, " +
					"every family on one scale, and its share of the eight weeks' worth. BY ACCOUNT is how much of its week each " +
					"account used each day, in the five shades, every account on one scale, and the 5-hour limits it hit."),
			},
			glyphs: []glyph{
				{drawn: f.swatch(dayInk(2), 2, true), means: daysLimitNote},
				{drawn: line{{toCome, faintInk}}, means: "a day still to come"},
			},
		},
		{
			heading: subTabHeading(HistoryTokens),
			says: []line{
				said("Tokens by period, as ccusage reports them, split by what ccusage can't split by: the account. Its columns " +
					"are ccusage's: input; output; cache write, both lifetimes' writes together; cache read; and total, the " +
					"four kinds. Worth is what the same tokens would cost through the API, at the price table's prices as of " +
					"the date the footnote gives, and against its price, that worth over what the period's plans cost."),
				said("`v` splits each period by account, by model, each version at its own price, by both, or not at all, and " +
					"`<` and `>` make the periods days, weeks or months."),
			},
		},
	}
}

// accountVerdictsMean says what each of Weeks' verdicts on an account means,
// at the cuts views judges it by, as History's help and Accounts' say it.
func accountVerdictsMean() string {
	return fmt.Sprintf("%s, in %d or more of the %d; %s, %d%% or more left at a reset on average; else %s. %s, where the "+
		"rest would cover it with no more weeks short, as it hit no limit and used %d%% or less on average; %s, %d%% or "+
		"more left at a reset on average.",
		views.HitsItsLimit, views.LimitWeeks, views.VerdictWeeks, views.PlentyToSpare, views.PlentyLeft, views.EndsWithRoom,
		capitalised(views.CouldGo), views.CouldGoUsed, views.BarelyUsed, views.BarelyLeft)
}

// subTabHeading is the heading help explains a sub-tab of History under: its
// name, as the heading row names it, in capitals.
func subTabHeading(v HistoryView) string {
	return strings.ToUpper(historyViews[v])
}

// shadesHelp are the five shades a day is drawn in, each as day draws a day
// at its level, and what it means.
func shadesHelp(day func(level int) line) []glyph {
	means := [shadeLevels]string{
		"a day with nothing on it",
		"the quietest quarter of the active days",
		"the second quarter",
		"the third quarter",
		"the busiest quarter",
	}
	glyphs := make([]glyph, 0, shadeLevels)
	for level, m := range means {
		glyphs = append(glyphs, glyph{drawn: day(level), means: m})
	}
	return glyphs
}

// yearDay is a day of the Year at a level, as the Year draws it.
func (f Frame) yearDay(level int) line {
	return line{f.daySquare(dayInk(level), level)}
}

// familyBar is a BY MONTH bar as help draws it: a cell of each family, in its
// colour, in the version table's order.
func familyBar() line {
	var bar line
	for _, family := range ledger.Families() {
		bar = append(bar, span{"▅", familyHue(family).ink()})
	}
	return bar
}
