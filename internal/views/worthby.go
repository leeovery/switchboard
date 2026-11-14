package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/score"
)

// BucketUnit is what a bucket of a chart over a period spans: an hour, two
// hours, a day, a week or a month.
type BucketUnit int

const (
	// ByHour is an hour of today, by the local clock.
	ByHour BucketUnit = iota
	// ByTwoHours is two hours of today, by the local clock, from midnight.
	ByTwoHours
	// ByDay is a local day.
	ByDay
	// ByWeek is a calendar week, from the day weeks start on.
	ByWeek
	// ByMonth is a calendar month.
	ByMonth
)

// Bucket is a span a chart's column is of: when it starts, as dayfile.Civil
// gives a day, or of an hour's, as hourBuckets has it; its name, as "14",
// "Wed", "17 Aug" or "Sep"; and what its axis names it, "" where nothing.
type Bucket struct {
	Start time.Time
	Label string
	Axis  string
}

// WorthBy is worth over a period in buckets, oldest first, as WORTH BY
// draws it: what each spans; each bucket; the index of the one worth most,
// the first of any that tie, -1 where none is worth anything; and how many
// of the last are still to come, as a week's days after today are.
type WorthBy struct {
	Unit    BucketUnit
	Buckets []WorthBucket
	Peak    int
	ToCome  int
}

// WorthBucket is a bucket of WORTH BY, and what the requests in it were
// worth.
type WorthBucket struct {
	Bucket
	Worth
}

// PeakNamed names WORTH BY's peak bucket, as the line under it says it: an
// hour by its start, "14:00"; two hours by their span, "14:00–16:00"; a day
// by its name, "Wed"; a week by its first day, "week of 17 Aug"; a month by
// its name, "Sep", or with its year, "Oct 2026", where another bucket has
// its name; "" where there's no peak.
func (by WorthBy) PeakNamed() string {
	if by.Peak < 0 {
		return ""
	}
	b := by.Buckets[by.Peak]
	switch by.Unit {
	case ByHour:
		return prose.TimeOfDay(b.Start, b.Start)
	case ByTwoHours:
		from := time.Duration(by.Peak*hoursSpanned) * time.Hour
		return prose.ClockSpan(from, from+hoursSpanned*time.Hour)
	case ByWeek:
		return "week of " + b.Label
	case ByMonth:
		if by.named(b.Label) > 1 {
			return prose.Month(b.Start, true)
		}
	}
	return b.Label
}

// named is how many of the buckets have the name given.
func (by WorthBy) named(label string) int {
	n := 0
	for _, b := range by.Buckets {
		if b.Label == label {
			n++
		}
	}
	return n
}

// Now is the index of the bucket now falls in, the last of those not still
// to come, as the axis picks its name out: -1 where all are to come.
func (by WorthBy) Now() int {
	return len(by.Buckets) - 1 - by.ToCome
}

// WorthCharts are worth over a period in each bucket WORTH BY may chart it
// in, as worthCharts has them, the finest first, for a drawing to take the
// finest that fits it.
type WorthCharts []WorthBy

// Fitting is the finest of the charts whose count of buckets fits reports
// fits, else the coarsest: the zero WorthBy, with no peak, where there are
// none.
func (c WorthCharts) Fitting(fits func(buckets int) bool) WorthBy {
	for _, by := range c {
		if fits(len(by.Buckets)) {
			return by
		}
	}
	return c.Coarsest()
}

// Coarsest is the coarsest of the charts: the zero WorthBy, with no peak,
// where there are none.
func (c WorthCharts) Coarsest() WorthBy {
	if len(c) == 0 {
		return WorthBy{Peak: -1}
	}
	return c[len(c)-1]
}

// dayWorths are what requests were worth each local day, by its date.
type dayWorths map[string]Worth

// add adds w to the day with the given date's worth.
func (d dayWorths) add(date string, w Worth) {
	day := d[date]
	day.add(w)
	d[date] = day
}

// BucketsMost are the most buckets the coarsest of any period's charts has,
// as worthCharts has them, so a drawing that fits so many fits every
// period's: the last 12 weeks' weeks, or the last 12 months' months, where
// either begins partway through its first.
const BucketsMost = 13

// hoursSpanned are the hours a bucket ByTwoHours spans.
const hoursSpanned = 2

// worthCharts are the worth of days, and of today's hours, where they're
// given, over the period at now, weeks starting on the day given, in the
// buckets WORTH BY may chart it in, the finest first, the coarsest of no
// more than BucketsMost: today's by hour and by two hours, or where the
// hours aren't given, by the day; this week's by day; the last 30 days'
// and the last 12 weeks' by day and by week; and the last 12 months' by
// week and by month.
func worthCharts(period Period, now time.Time, starts time.Weekday, days dayWorths, hours *[24]Worth) WorthCharts {
	today := dayfile.CivilOf(now)
	first, last := period.first(today, starts), period.last(today, starts)
	byDay := days.by(ByDay, dayBuckets(first, last), first, last, today)
	switch period {
	case PeriodToday:
		if hours != nil {
			hourly := byHour(*hours, now)
			return WorthCharts{hourly, byTwoHours(hourly)}
		}
	case PeriodThirtyDays, PeriodTwelveWeeks:
		return WorthCharts{byDay, days.by(ByWeek, weekBuckets(first, last, starts), first, last, today)}
	case PeriodTwelveMonths:
		return WorthCharts{days.by(ByWeek, weekBuckets(first, last, starts), first, last, today), days.by(ByMonth, monthBuckets(first, last), first, last, today)}
	}
	return WorthCharts{byDay}
}

// byHour is today's worth by the local hour, now given, hours holding each
// hour's, as hourBuckets names them.
func byHour(hours [24]Worth, now time.Time) WorthBy {
	heads := hourBuckets(now)
	by := WorthBy{Unit: ByHour, Buckets: make([]WorthBucket, len(heads)), ToCome: len(heads) - 1 - now.Local().Hour()}
	for h, b := range heads {
		by.Buckets[h] = WorthBucket{Bucket: b, Worth: hours[h]}
	}
	return peaked(by)
}

// byTwoHours is today's worth by hour, as byHour has it, by two hours from
// midnight: each pair's worth together, starting as its first hour does
// and named as it is.
func byTwoHours(hourly WorthBy) WorthBy {
	by := WorthBy{Unit: ByTwoHours, Buckets: make([]WorthBucket, len(hourly.Buckets)/hoursSpanned), ToCome: hourly.ToCome / hoursSpanned}
	for h, b := range hourly.Buckets {
		span := &by.Buckets[h/hoursSpanned]
		if h%hoursSpanned == 0 {
			span.Bucket = b.Bucket
		}
		span.add(b.Worth)
	}
	return peaked(by)
}

// by is the worth of the days from first to last, each as dayfile.Civil
// gives a day, today given, in the buckets given, oldest first, each of the
// unit given: each bucket's the worth of its days from first, up to the next
// bucket's start, and to last; those starting after today still to come.
func (d dayWorths) by(unit BucketUnit, heads []Bucket, first, last, today time.Time) WorthBy {
	by := WorthBy{Unit: unit, Buckets: make([]WorthBucket, len(heads))}
	for i, b := range heads {
		end := last.AddDate(0, 0, 1)
		if i+1 < len(heads) {
			end = heads[i+1].Start
		}
		by.Buckets[i].Bucket = b
		for day := score.Later(b.Start, first); day.Before(end) && !day.After(last); day = day.AddDate(0, 0, 1) {
			by.Buckets[i].add(d[day.Format(time.DateOnly)])
		}
		if b.Start.After(today) {
			by.ToCome++
		}
	}
	return peaked(by)
}

// peaked is by with its peak found: the first of the buckets worth most,
// -1 where none is worth anything.
func peaked(by WorthBy) WorthBy {
	by.Peak = -1
	for i, b := range by.Buckets {
		if b.amount() > 0 && (by.Peak < 0 || b.amount() > by.Buckets[by.Peak].amount()) {
			by.Peak = i
		}
	}
	return by
}

// hourBuckets are today's hours, now given, each named, on the axis too,
// "00" to "23", each starting as today's clocks first read it, or went
// forward over it, as dayfile.TimeOfDay has it, so an hour a clock change
// repeats is one bucket, and one it skips starts with the next.
func hourBuckets(now time.Time) []Bucket {
	heads := make([]Bucket, 24)
	for h := range heads {
		named := prose.HourNamed(h)
		heads[h] = Bucket{Start: dayfile.TimeOfDay(now.Local(), h, 0), Label: named, Axis: named}
	}
	return heads
}

// dayBuckets are the days from first to last, each as dayfile.Civil gives a
// day: each named, on the axis too, by its weekday where they're a week or
// fewer, else by its date, as "21 Sep".
func dayBuckets(first, last time.Time) []Bucket {
	named := prose.DayMonth
	if last.Sub(first) < 7*24*time.Hour {
		named = func(d time.Time) string { return prose.Weekday(d.Weekday()) }
	}
	var heads []Bucket
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		heads = append(heads, Bucket{Start: d, Label: named(d), Axis: named(d)})
	}
	return heads
}

// weekBuckets are the calendar weeks from the one first falls in to the one
// last does, each as dayfile.Civil gives a day, weeks starting on the day
// given: each named by its first day's date, as "17 Aug", the axis naming
// the month of the first of its days from first, as "Aug", where it's
// another than the week before's.
func weekBuckets(first, last time.Time, starts time.Weekday) []Bucket {
	var heads []Bucket
	month := ""
	for week := weekOf(first, starts); !week.After(last); week = week.AddDate(0, 0, 7) {
		b := Bucket{Start: week, Label: prose.DayMonth(week)}
		if named := prose.Month(score.Later(week, first), false); named != month {
			b.Axis, month = named, named
		}
		heads = append(heads, b)
	}
	return heads
}

// monthBuckets are the calendar months from the one first falls in to the
// one last does, each as dayfile.Civil gives a day, each starting on its
// first day and named, on the axis too, as "Sep".
func monthBuckets(first, last time.Time) []Bucket {
	var heads []Bucket
	for month := first.AddDate(0, 0, 1-first.Day()); !month.After(last); month = month.AddDate(0, 1, 0) {
		named := prose.Month(month, false)
		heads = append(heads, Bucket{Start: month, Label: named, Axis: named})
	}
	return heads
}
