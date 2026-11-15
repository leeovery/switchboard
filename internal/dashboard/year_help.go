package dashboard

import (
	"github.com/leeovery/switchboard/internal/ledger"
)

// yearShadesSaid and yearSplitSaid say, as help says it, how the Year shades
// a day, and what v and ← and → have it show, on Trends' YEAR.
const (
	yearShadesSaid = "A day is shaded by how many requests it made, never by its worth, which shifts with the models used and " +
		"their prices. A day with nothing on it has a shade of its own, and the active days are split into quarters by how " +
		"much each did, on one scale for everything shown together, the key giving each quarter's bounds."
	yearSplitSaid = "`v` shows one model version alone, in four steps of its own colour, or one account, `←` and `→` " +
		"stepping between the accounts; the shades, their key and the numbers re-scale to what's shown."
)

// byMonthHelp is BY MONTH as help explains it, on Trends: each month's columns and its bar, the month
// so far and the one the ledger began in, and plans not known; then its
// bar's and its so far's marks.
func byMonthHelp() helpPart {
	return helpPart{
		heading: "BY MONTH",
		says: []line{
			said("Each month's requests, its worth at API prices, what the plans cost that month and its worth against it, " +
				"and the limits reached. Against is bold, in the theme's positive colour at 1× or more, the plans more than " +
				"paid for, and its attention colour under. A bar is a month's worth against the largest month's, split by " +
				"family, each in its colour, as the dots under the bars name them. The month still running says so far after " +
				"its bar, and the month the ledger began recording in the day it began, as from the 16th, its plans priced " +
				"from then. Where some accounts' plans aren't known, against is over those known, as the heading says, " +
				"with the worth of their accounts alone."),
		},
		glyphs: []glyph{
			{drawn: familyBar(), means: "a month's worth, by family"},
			{drawn: line{noFigure}, means: "against, for the month so far"},
		},
	}
}

// shadesHelp are the five shades what's shaded is drawn in, a day or an
// hour, as one and many name it, each as drawn draws one at its level, and
// what it means.
func shadesHelp(one, many string, drawn func(level int) line) []glyph {
	means := [shadeLevels]string{
		one + " with nothing on it",
		"the quietest quarter of the active " + many,
		"the second quarter",
		"the third quarter",
		"the busiest quarter",
	}
	glyphs := make([]glyph, 0, shadeLevels)
	for level, m := range means {
		glyphs = append(glyphs, glyph{drawn: drawn(level), means: m})
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
