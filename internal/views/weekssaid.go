package views

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// What Weeks' verdicts say, each once, as History's Weeks and history's text
// both say it: where no whole week can be judged yet; the labels of what the
// replays found, as the accounts stand, with one fewer, two fewer or one
// more, and of one account, with a second; what the rest find without an
// account; and that the verdicts are an estimate, as many accounts' form
// words it, and as few accounts'.
const (
	NotEnoughWeeks           = "not enough weeks yet"
	ReplayAsNow              = "as now: "
	ReplayOneFewer           = "with one fewer: "
	ReplayTwoFewer           = "with two fewer: "
	ReplayOneMore            = "with one more: "
	ReplayASecond            = "with a second: "
	WithoutTheRest           = "without it, the rest: "
	EstimateOfReplays        = "an estimate, from replaying these weeks"
	EstimateOfOneFewerOrMore = EstimateOfReplays + " with one account fewer or more"
)

// CountedWords count n of the thing a noun names, as prose.Counted counts
// it, the count a figure: "1,240", then " requests".
func CountedWords(n int, noun string) status.Words {
	count := prose.Grouped(n)
	return status.Words{status.Figure(count), status.Plain(strings.TrimPrefix(prose.Counted(n, noun), count))}
}

// Headline is Weeks' verdict on the accounts, their number in words a
// figure: "three is about right".
func Headline(accounts int, verdict string) status.Words {
	return status.Words{status.Figure(prose.Number(accounts)), status.Plain(" is " + verdict)}
}

// ShortOn says how many weeks were short, n of them: "short on 6", its count
// a figure, or "short on none".
func ShortOn(n int) status.Words {
	return status.Words{status.Plain("short on "), shortCount(n)}
}

// shortCount counts weeks short, n of them: a figure, as "3", or "none",
// words.
func shortCount(n int) status.Part {
	if n == 0 {
		return status.Plain("none")
	}
	return status.Figure(prose.Grouped(n))
}

// ShortOfWeeks says how many of the weeks judged were short, n of them:
// "short on 3 of 7 weeks", or "short on none of 7 weeks", its counts figures.
func ShortOfWeeks(n, weeks int) status.Words {
	return shortOf(n, " of ", weeks)
}

// ShortOfTheLastWeeks says how many of the last weeks were short, n of them,
// as ShortOfWeeks does: "short on 3 of the last 7 weeks".
func ShortOfTheLastWeeks(n, weeks int) status.Words {
	return shortOf(n, " of the last ", weeks)
}

// shortOf says how many of the weeks judged were short, n of them, of as
// given between the two counts.
func shortOf(n int, of string, weeks int) status.Words {
	return slices.Concat(ShortOn(n), status.Words{status.Plain(of)}, CountedWords(weeks, "week"))
}

// ShortOutOf says how many of the weeks judged were short, n of them, as a
// replay finds them: "short on 6 of 7", its counts figures, or "short on
// none".
func ShortOutOf(n, weeks int) status.Words {
	if n == 0 {
		return ShortOn(n)
	}
	return slices.Concat(status.Words{status.Plain("short on ")}, OutOf(n, weeks))
}

// OutOf says n of a count of, both figures: "6 of 7".
func OutOf(n, of int) status.Words {
	return status.Words{status.Figure(prose.Grouped(n)), status.Plain(" of "), status.Figure(prose.Grouped(of))}
}

// LimitHit says how many of an account's weeks it hit its limit in: "limit
// hit 3 of 7 weeks", its counts figures.
func LimitHit(limits, weeks int) status.Words {
	return slices.Concat(status.Words{status.Plain("limit hit "), status.Figure(prose.Grouped(limits)), status.Plain(" of ")},
		CountedWords(weeks, "week"))
}

// LeftOnAverage says what an account's weeks left at a reset, on average, as
// a whole percentage, a figure: "9% left at a reset, on average".
func LeftOnAverage(share float64) status.Words {
	return status.Words{status.Figure(prose.Percent(share)), status.Plain(" left at a reset, on average")}
}

// ToDrop says the account named is the one to drop: "side is the one to
// drop".
func ToDrop(name string) string {
	return name + " is the one to drop"
}

// DateNamed names the day with the given date, as History names the busiest
// day: "Tue 22 Sep"; "" where the date isn't one.
func DateNamed(date string) string {
	day, ok := dayfile.Civil(date)
	if !ok {
		return ""
	}
	return prose.Day(day)
}

// DaysInARow says how many days in a row there were, as History says the
// longest run: "58 days"; "" for none.
func DaysInARow(days int) string {
	if days == 0 {
		return ""
	}
	return prose.Counted(days, "day")
}
