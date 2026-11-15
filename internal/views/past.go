package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// Past is the ledger's days, priced once as they're given, which Accounts'
// and Trends' pages are built of.
type Past struct {
	in   HistoryInput
	days []Day
}

// NewPast is the Past of the days given, the summaries the ledger holds of a
// year and today, oldest first, read at now, priced at today's prices. The
// input gives the time now, the prices, the accounts, their weeks and the
// families; From, the day the days were asked for from, for Tokens to price a
// period they begin part-way through by its days alone; and Began, where the
// ledger began, never taken from the days, as the first held needn't be its
// first: its Ledger goes unread, as the days are given.
func NewPast(in HistoryInput, days []ledger.Summary) Past {
	in.read = in.Now
	today := dayfile.DateOf(in.Now)
	v := versions{table: in.Prices, family: in.Family}
	return Past{in: in, days: pricedDays(days, in, today, v)}
}

// Since is the Past at now, the days given, the summaries the ledger holds of
// the days from the first of them to today, oldest first, read at now, in
// place of those it held from that day on. Only they are priced, at now's
// day's prices, as they're given: the days held before them stand as they
// were priced, so a look that reads today alone prices today alone. Given
// none, the days held stand as read when they were, the Past at now alone.
func (p Past) Since(days []ledger.Summary, now time.Time) Past {
	p.in.Now = now
	if len(days) == 0 {
		return p
	}
	p.in.read = now
	today := dayfile.DateOf(now)
	before, _ := splitAt(p.days, days[0].Day)
	p.days = slices.Concat(before, pricedDays(days, p.in, today, p.versions()))
	return p
}

// Begun is the Past, the ledger having begun at began, the start of its
// first day, as LedgerStart tells it, once that's learned.
func (p Past) Begun(began time.Time) Past {
	p.in.Began = began
	return p
}

// Year is the Year's page, as asked for.
func (p Past) Year(ask YearAsk) YearPage {
	return p.year(ask)
}

// Versions are the versions used in the last 12 months, as the Year's page of
// them lists them for v to cycle through, without building it.
func (p Past) Versions() []string {
	span := yearSpanOf(0, dayfile.CivilOf(p.in.Now), p.in.WeekStarts)
	return between(p.days, span.first, span.last).versions(p.versions())
}

// versions are the version table and the families, as the Past's days are
// named by them.
func (p Past) versions() versions {
	return versions{table: p.in.Prices, family: p.in.Family}
}

// counted is the first day the Past counts of the days from first, as
// dayfile.Civil gives a day, as begunFrom has it, the ledger having begun
// where the input tells it.
func (p Past) counted(first time.Time) time.Time {
	return begunFrom(first, p.in.Began)
}

// begunFrom is the first day counted of the days from first, as
// dayfile.Civil gives a day, as begunOn has it of their dates.
func begunFrom(first, began time.Time) time.Time {
	if began.IsZero() {
		return first
	}
	// Both dates begunOn compares are written from times, so the later
	// parses.
	day, _ := dayfile.Civil(begunOn(first.Format(time.DateOnly), began))
	return day
}

// begunOn is the date of the first day counted of the days from the one
// with the date first, the work they're of having begun at began: began's
// date, as beganOn has it, where that's later, so nothing counts or charts
// a day before it, as a pool's days hold none of its work before; first
// where began is zero, as where it isn't known. It's the one rule of where
// counting begins, every page's and history's.
func begunOn(first string, began time.Time) string {
	return max(first, beganOn(began))
}

// beganOn is the date of the day work began, at began: "" where began is
// zero, as where it isn't known.
func beganOn(began time.Time) string {
	if began.IsZero() {
		return ""
	}
	return dayfile.DateOf(began)
}

// Summaries are the summaries of the days the Past holds from first to last,
// each as dayfile.Civil gives a day, oldest first, as the ledger gave them:
// what Runway's lanes read of the days a timeline stepped back to shows, and
// of the week before them.
func (p Past) Summaries(first, last time.Time) []ledger.Summary {
	var held []ledger.Summary
	for _, d := range between(p.days, first.Format(time.DateOnly), last.Format(time.DateOnly)) {
		held = append(held, d.Summary)
	}
	return held
}

// Days is Days' page: the eight weeks to this one, a day at a time.
func (p Past) Days() DaysPage {
	return daysPageOf(p.in, p.days)
}
