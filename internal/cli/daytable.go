package cli

import (
	"fmt"
	"io"
	"iter"
	"time"
)

// dayLayout lays out a day as a heading names it, as "Wed 7 Oct 2026".
const dayLayout = "Mon 2 Jan 2006"

// dayTable is what requests and history print of a day: rows, aligned under
// the day's heading, and lines of notes after them.
type dayTable struct {
	day   time.Time
	rows  [][]string
	notes []string
}

// of reports whether t falls on the table's day.
func (d dayTable) of(t time.Time) bool {
	return d.day.Format(time.DateOnly) == t.Format(time.DateOnly)
}

// write writes the table under its day's heading, today's saying it's so far
// at now, after a blank line unless it's the first.
func (d dayTable) write(out io.Writer, now time.Time, first bool) error {
	heading := d.day.Format(dayLayout)
	if d.of(now) {
		heading += ", so far"
	}
	if !first {
		heading = "\n" + heading
	}
	if _, err := fmt.Fprintln(out, heading); err != nil {
		return err
	}
	return d.writeRows(out)
}

// writeRows writes the table's rows, aligned, and its notes after them.
func (d dayTable) writeRows(out io.Writer) error {
	rows := make([][]string, len(d.rows))
	for i, row := range d.rows {
		rows[i] = append([]string{"  " + row[0]}, row[1:]...)
	}
	if err := writeTable(out, rows); err != nil {
		return err
	}
	for _, note := range d.notes {
		if _, err := fmt.Fprintf(out, "  %s\n", note); err != nil {
			return err
		}
	}
	return nil
}

// writeDays writes items as a table a day, under each day's heading, oldest
// first, a row each, as row makes it of an item at its time, as at gives it,
// in now's time zone; or says there were none of what since from. It returns
// the day of the last table written: zero where it wrote none.
func writeDays[T any](out io.Writer, items iter.Seq[T], at func(T) time.Time, row func(T, time.Time) []string, what string, from, now time.Time) (time.Time, error) {
	day, first := dayTable{}, true
	for item := range items {
		t := at(item).In(now.Location())
		if len(day.rows) > 0 && !day.of(t) {
			if err := day.write(out, now, first); err != nil {
				return time.Time{}, err
			}
			day.rows, first = nil, false
		}
		if len(day.rows) == 0 {
			day.day = t
		}
		day.rows = append(day.rows, row(item, t))
	}
	if len(day.rows) == 0 {
		_, err := fmt.Fprintf(out, "no %s since %s\n", what, from.In(now.Location()).Format(dayLayout+" 15:04"))
		return time.Time{}, err
	}
	return day.day, day.write(out, now, first)
}
