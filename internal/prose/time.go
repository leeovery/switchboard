package prose

import "time"

// Clock shows t as its weekday and 24-hour time in now's time zone, such as
// "Mon 18:10".
func Clock(now, t time.Time) string {
	return t.In(now.Location()).Format("Mon 15:04")
}

// TimeOfDay shows t as its 24-hour time in now's time zone, such as "13:51".
func TimeOfDay(now, t time.Time) string {
	return t.In(now.Location()).Format("15:04")
}

// day is a day's length, as When has it.
const day = 24 * time.Hour

// When shows t in now's time zone: its time of day, such as "15:54", within
// a day of now, before or after, as what's coming up or happened lately is;
// else its weekday too, such as "Mon 21:00".
func When(now, t time.Time) string {
	if d := t.Sub(now); -day < d && d < day {
		return TimeOfDay(now, t)
	}
	return Clock(now, t)
}

// Dated shows t in now's time zone as a card does, and as every time of an
// event is said: its time of day, such as "16:05", on now's day; on any
// other, its weekday too, such as "Fri 04:06".
func Dated(now, t time.Time) string {
	if SameDay(now, t, now) {
		return TimeOfDay(now, t)
	}
	return Clock(now, t)
}

// SameDay reports whether a and b fall on one day in now's time zone.
func SameDay(now, a, b time.Time) bool {
	return a.In(now.Location()).Format(time.DateOnly) == b.In(now.Location()).Format(time.DateOnly)
}

// Stint says a stay, as a session's on an account, from from to to, or to now
// where to is zero, in now's time zone: "12:02 – 14:31", "14:31 – now"; or
// dated, as where the session spans more than one day, each end with its day,
// but an end on its start's, as "yesterday 16:40 – 18:05" or "today 09:12 –
// now".
func Stint(now, from, to time.Time, dated bool) string {
	start, end := TimeOfDay(now, from), "now"
	if !to.IsZero() {
		end = TimeOfDay(now, to)
	}
	if dated {
		start = withDay(now, from)
		if !to.IsZero() && !SameDay(now, from, to) {
			end = withDay(now, to)
		}
	}
	return start + " – " + end
}

// withDay says t with its day, in now's time zone: "today 09:12" on now's
// day, else as Past says it, as "yesterday 16:40".
func withDay(now, t time.Time) string {
	if SameDay(now, t, now) {
		return "today " + TimeOfDay(now, t)
	}
	return Past(now, t)
}

// Past shows t, before now, in now's time zone, by how far back its day is,
// as when a session started: its time of day, such as "12:47", on now's day;
// "yesterday 15:08" on the day before; its weekday too, such as "Tue 15:08",
// within the week; else its date, such as "28 Sep 15:08".
func Past(now, t time.Time) string {
	t = t.In(now.Location())
	switch days := daysBetween(t, now); {
	case days == 0:
		return TimeOfDay(now, t)
	case days == 1:
		return "yesterday " + TimeOfDay(now, t)
	case WithinWeek(now, t):
		return Clock(now, t)
	}
	return t.Format("2 Jan 15:04")
}

// WithinWeek reports whether t, before now, falls on now's day or one of the
// six before it, in now's time zone: the days Past names by their weekday.
func WithinWeek(now, t time.Time) bool {
	return daysBetween(t, now) < 7
}

// daysBetween counts the days from the one t falls on to the one now does,
// in now's time zone, however long the days between ran.
func daysBetween(t, now time.Time) int {
	noon := func(t time.Time) time.Time {
		y, m, d := t.In(now.Location()).Date()
		return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
	}
	return int(noon(now).Sub(noon(t)).Round(day) / day)
}
