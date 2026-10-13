package prose

import (
	"fmt"
	"strconv"
	"time"
)

// Countdown says how long it is from now until t, as Duration says how long
// that is: "5d 12h" from a day away, "4h 57m" from an hour, else "7m"; "now"
// once t has come.
func Countdown(now, t time.Time) string {
	if !t.After(now) {
		return "now"
	}
	return Duration(t.Sub(now))
}

// Duration says how long d is, in whole units, as Countdown counts down a
// span that long: "5d 12h" from a day, "4h 57m" from an hour, else "7m", and
// "0m" for none. A second unit of zero is left off, as in "6d" and "4h".
func Duration(d time.Duration) string {
	return countdown(max(d, 0), "%dm")
}

// countdown says how long d, none or more, is as Duration does, the minutes
// after hours as the format given writes them.
func countdown(d time.Duration, minutesAfterHours string) string {
	hours, minutes := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case hours >= 24:
		return withRest(fmt.Sprintf("%dd", hours/24), hours%24, "%dh")
	case hours >= 1:
		return withRest(fmt.Sprintf("%dh", hours), minutes, minutesAfterHours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// Span says how long d is, as the dashboard writes a span, as how long a
// session has idled or run: in seconds under a minute, as "34s"; in minutes,
// as "9m"; in hours and minutes, the minutes two digits, as "1h 05m"; and in
// days and hours, as "1d 3h"; each unit whole, what's left over cut, and a
// second unit of zero left off, as in "2h".
func Span(d time.Duration) string {
	if d < time.Minute {
		return strconv.Itoa(int(max(d, 0)/time.Second)) + "s"
	}
	return countdown(d, "%02dm")
}

// SeenAgo says how long before now seen was, as a row of sessions running
// has room for: "now" within the half minute, else to the nearest minute, as
// "9m", and from the hour, to the nearest hour, as "2h".
func SeenAgo(now, seen time.Time) string {
	d := now.Sub(seen)
	switch minutes := int((d + time.Minute/2) / time.Minute); {
	case minutes < 1:
		return "now"
	case minutes < 60:
		return strconv.Itoa(minutes) + "m"
	default:
		return strconv.Itoa(int((d+time.Hour/2)/time.Hour)) + "h"
	}
}

// Seconds says how long a request took, or took to its first byte, as the
// Log writes it: under a minute in seconds to a tenth, as "0.8s" or "11.8s";
// from a minute in minutes and seconds, the seconds two digits, as "1m 05s".
// Each form is for what rounds to it, so 59.96 seconds is "1m 00s".
func Seconds(d time.Duration) string {
	if tenths := d.Round(100 * time.Millisecond); tenths < time.Minute {
		return strconv.FormatFloat(tenths.Seconds(), 'f', 1, 64) + "s"
	}
	seconds := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
}

// withRest follows a count with what's left over in the next unit down, as
// the format given writes it, unless nothing is: "5d 12h", but "6d".
func withRest(count string, rest int, format string) string {
	if rest == 0 {
		return count
	}
	return count + " " + fmt.Sprintf(format, rest)
}

// Until counts down from now to t, as "in 1h 12m", rounded up to the whole
// minute, so within a day it never says less time is left than is, and the
// minutes after hours two digits, as "in 4h 08m", so a column of them reads
// evenly; from a day out, in days and whole hours, as "in 1d 15h", the
// minutes left off, as the frames count them: "now" once t has come.
func Until(now, t time.Time) string {
	left := t.Sub(now)
	if left <= 0 {
		return "now"
	}
	if up := left.Truncate(time.Minute); up < left {
		left = up + time.Minute
	}
	return "in " + countdown(left, "%02dm")
}

// Resets counts down from now to a window's reset at t: "resets in 4h 57m",
// or "resets now" once t has come.
func Resets(now, t time.Time) string {
	if !t.After(now) {
		return "resets now"
	}
	return "resets in " + Countdown(now, t)
}

// Over says the span d a recent rate is measured over, back from now: "last
// 30 min", "last 18 min", or from an hour, as Countdown says it, "last 2h".
func Over(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("last %d min", int(d/time.Minute))
	}
	return "last " + countdown(d, "%dm")
}

// About says about how long d is, as Runway's key says what a column spans:
// to the five minutes, as "75 min", under two hours; else to the hour, as
// "6h".
func About(d time.Duration) string {
	if d < 2*time.Hour {
		return fmt.Sprintf("%d min", d.Round(5*time.Minute)/time.Minute)
	}
	return fmt.Sprintf("%dh", d.Round(time.Hour)/time.Hour)
}

// Minutes says how many minutes there are from the minute from falls in to
// the one to falls in, both counted, as a turn's length is said: "9m" from
// 14:29 to 14:37, and "1m" within one minute.
func Minutes(from, to time.Time) string {
	return strconv.Itoa(int(to.Truncate(time.Minute).Sub(from.Truncate(time.Minute))/time.Minute)+1) + "m"
}

// ClockOf shows a time of day, as the time since midnight, as HH:MM, such as
// "08:00".
func ClockOf(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d/time.Hour), int(d%time.Hour/time.Minute))
}
