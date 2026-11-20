package views

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// What the capacity verdicts say, each once, as history's text says it:
// where no whole week can be judged yet; the labels of what the replays
// found, as the accounts stand, with one fewer, two fewer or one more; what
// the rest find without an account; and
// that the verdicts are an estimate, as many accounts' form words it, and as
// few accounts'.
const (
	NotEnoughWeeks           = "not enough weeks yet"
	ReplayAsNow              = "as now: "
	ReplayOneFewer           = "with one fewer: "
	ReplayTwoFewer           = "with two fewer: "
	ReplayOneMore            = "with one more: "
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
	return slices.Concat(ShortOn(n), status.Words{status.Plain(" of ")}, CountedWords(weeks, "week"))
}

// OutOf says n of a count of, both figures: "6 of 7".
func OutOf(n, of int) status.Words {
	return status.Words{status.Figure(prose.Grouped(n)), status.Plain(" of "), status.Figure(prose.Grouped(of))}
}

// LimitHit says how many of an account's weeks it hit its limit in: "limit
// hit 3 of 7 weeks", its counts figures.
func LimitHit(limits, weeks int) status.Words {
	return limitHit(limits, " of ", weeks)
}

// LimitHitOfTheLast says how many of the last weeks an account hit its limit
// in, as LimitHit does: "limit hit 3 of the last 7 weeks".
func LimitHitOfTheLast(limits, weeks int) status.Words {
	return limitHit(limits, " of the last ", weeks)
}

// limitHit says how many of the weeks judged an account hit its limit in,
// of as given between the two counts.
func limitHit(limits int, of string, weeks int) status.Words {
	return slices.Concat(status.Words{status.Plain("limit hit "), status.Figure(prose.Grouped(limits)), status.Plain(of)},
		CountedWords(weeks, "week"))
}

// WeeksShown says the weeks Detail's weeks section shows, n of them, as its
// rule heads them at now: "the last eight weeks", at now or where they end
// at this week, as ThisWeek has it, a past day of it among them; else by
// the last day of the last, as WholeTo has it, "the eight weeks to Sun 27
// Sep", as prose.DayAt names it at now.
func (p AccountsPage) WeeksShown(n int, now time.Time) string {
	if p.atNow() || p.ThisWeek {
		return "the last " + prose.Number(n) + " weeks"
	}
	return "the " + prose.Number(n) + " weeks to " + prose.DayAt(now, p.WholeTo)
}

// LimitHitOver says how many of the whole weeks given an account hit its
// limit in, limits of them, as Detail's facts on the page's weeks say it at
// now, fullest first: at now, as LimitHitOfTheLast and LimitHit say it,
// "limit hit 3 of the last 7 weeks", then "limit hit 3 of 7 weeks"; in a
// period gone, by the last day of the last whole week, as WholeTo has it and
// prose.DayAt names it at now, "limit hit 3 of the 7 weeks to Sun 27 Sep",
// then "limit hit 3 of 7 weeks to Sun 27 Sep".
func (p AccountsPage) LimitHitOver(limits, weeks int, now time.Time) (full, short status.Words) {
	if p.atNow() {
		return LimitHitOfTheLast(limits, weeks), LimitHit(limits, weeks)
	}
	to := status.Words{status.Plain(" to " + prose.DayAt(now, p.WholeTo))}
	return slices.Concat(limitHit(limits, " of the ", weeks), to), slices.Concat(LimitHit(limits, weeks), to)
}

// atNow is whether the page is of now, as WholeTo, zero only at now, has
// it.
func (p AccountsPage) atNow() bool {
	return p.WholeTo.IsZero()
}

// LimitHitOutOf says how many of an account's weeks it hit its limit in, as
// Compare's WEEKS says it: "limit hit 3 of 51", its counts figures.
func LimitHitOutOf(limits, weeks int) status.Words {
	return slices.Concat(status.Words{status.Plain("limit hit ")}, OutOf(limits, weeks))
}

// ShortOnOutOf says how many of the weeks judged were short, n of them, as
// Compare's WEEKS says it of every account: "short on 3 of 51", or "short
// on none of 51", its counts figures.
func ShortOnOutOf(n, weeks int) status.Words {
	return slices.Concat(ShortOn(n), status.Words{status.Plain(" of "), status.Figure(prose.Grouped(weeks))})
}

// Left says what an account's weeks left at a reset, on average, short, as
// Compare's WEEKS says it where LeftAtAReset doesn't fit: "21% left".
func Left(share float64) status.Words {
	return status.Words{status.Figure(prose.Percent(share)), status.Plain(" left")}
}

// LeftOnAverage says what an account's weeks left at a reset, on average, as
// LeftAtAReset says it: "9% left at a reset, on average".
func LeftOnAverage(share float64) status.Words {
	return append(LeftAtAReset(share), status.Plain(", on average"))
}

// LeftAtAReset says what an account's weeks left at a reset, as a whole
// percentage, a figure: "9% left at a reset".
func LeftAtAReset(share float64) status.Words {
	return status.Words{status.Figure(prose.Percent(share)), status.Plain(" left at a reset")}
}

// ToDrop says the account named is the one to drop: "side is the one to
// drop".
func ToDrop(name string) string {
	return name + " is the one to drop"
}

// DateNamed names the day with the given date, as YEAR names the busiest
// day: "Tue 22 Sep"; "" where the date isn't one.
func DateNamed(date string) string {
	day, ok := dayfile.Civil(date)
	if !ok {
		return ""
	}
	return prose.Day(day)
}

// DaysInARow says how many days in a row there were, as YEAR says the
// longest run: "58 days"; "" for none.
func DaysInARow(days int) string {
	if days == 0 {
		return ""
	}
	return prose.Counted(days, "day")
}
