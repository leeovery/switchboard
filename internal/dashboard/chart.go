package dashboard

import (
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// levels fill a cell of a chart by eighths, from its floor up.
var levels = [...]string{"", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// floorLine is drawn along a chart's floor where its window has nothing
// left.
const floorLine = "▁"

// dayCells is how many cells a day's tick and name take on an axis, with the
// gap after them.
const dayCells = 5

// midnight is a midnight along an axis of days: when it is, the column it's
// in, whether its day is today, and whether its name shows.
type midnight struct {
	at           time.Time
	col          int
	today, named bool
}

// midnights are the midnights after start and before end, in now's time
// zone, each the start of its day, as dayStarts gives it, in the column
// column puts it in, of those before width, its name showing where it's
// dayCells clear of the last shown, and what shows it, cells wide from its
// column, fits within width.
func midnights(start, end, now time.Time, column func(time.Time) int, width, cells int) []midnight {
	loc := now.Location()
	today := now.In(loc).Format(time.DateOnly)
	var all []midnight
	last := -dayCells
	for at := range dayStarts(start.In(loc), 1) {
		col := column(at)
		switch {
		case !at.Before(end) || col >= width:
			return all
		case col < 0:
			continue
		}
		m := midnight{at: at, col: col, today: at.Format(time.DateOnly) == today, named: col-last >= dayCells && col+cells <= width}
		if m.named {
			last = col
		}
		all = append(all, m)
	}
	return all
}

// dayStarts are the starts of the days in t's time zone, as
// dayfile.DayStart gives them, from the one days after the day t falls on,
// one after another, for as long as they're asked for.
func dayStarts(t time.Time, days int) iter.Seq[time.Time] {
	return func(yield func(time.Time) bool) {
		for at := dayfile.DayStart(t, days); yield(at); at = dayfile.DayStart(at, 1) {
		}
	}
}
