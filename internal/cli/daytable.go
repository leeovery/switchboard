package cli

import (
	"fmt"
	"io"
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
