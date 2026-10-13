package cli

import (
	"fmt"
	"io"
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

// dayTable is what requests, history and events print of a day: rows,
// aligned under the day's heading, and lines of notes after them. Its day is
// named in the time zone it carries: the local day's, of the ledger's days,
// and now's, of those a verb puts its lines under.
type dayTable struct {
	day   time.Time
	rows  [][]string
	notes []string
}

// write writes the table under its day's heading, as heading says, and its
// rows and notes, as writeRows does.
func (d dayTable) write(out io.Writer, now time.Time, first bool) error {
	if err := d.writeHeading(out, now, first); err != nil {
		return err
	}
	return d.writeRows(out)
}

// writeHeading writes the table's day's heading, as prose.DayInYear names
// it, the day it holds saying it's so far at now, after a blank line unless
// it's the first.
func (d dayTable) writeHeading(out io.Writer, now time.Time, first bool) error {
	heading := prose.DayInYear(d.day)
	if prose.SameDay(d.day, d.day, now) {
		heading += ", so far"
	}
	if !first {
		heading = "\n" + heading
	}
	_, err := fmt.Fprintln(out, heading)
	return err
}

// dayOf is the time t, in now's time zone, as a table of the day it falls on
// there carries it.
func dayOf(now, t time.Time) time.Time {
	return t.In(now.Location())
}

// writeRows writes the table's rows, aligned, as indented has them, and its
// notes after them.
func (d dayTable) writeRows(out io.Writer) error {
	if err := writeTable(out, d.indented()); err != nil {
		return err
	}
	for _, note := range d.notes {
		if _, err := fmt.Fprintf(out, "  %s\n", note); err != nil {
			return err
		}
	}
	return nil
}

// indented are the table's rows as they're written, each indented under the
// day's heading.
func (d dayTable) indented() [][]string {
	rows := make([][]string, len(d.rows))
	for i, row := range d.rows {
		rows[i] = indented(row)
	}
	return rows
}

// indented is row as it's written under a day's heading.
func indented(row []string) []string {
	return append([]string{"  " + row[0]}, row[1:]...)
}

// writeDays writes items as a table a day, under each day's heading, oldest
// first, a row each, as row makes it, each on the day its time, as at gives
// it, falls on in now's time zone; or says there were none of what since
// from. It returns the last table written: none where it wrote none.
func writeDays[T any](out io.Writer, items iter.Seq[T], at func(T) time.Time, row func(T) []string, what string, from, now time.Time) (dayTable, error) {
	day, first := dayTable{}, true
	for item := range items {
		t := at(item)
		if len(day.rows) > 0 && !prose.SameDay(now, day.day, t) {
			if err := day.write(out, now, first); err != nil {
				return dayTable{}, err
			}
			day.rows, first = nil, false
		}
		if len(day.rows) == 0 {
			day.day = dayOf(now, t)
		}
		day.rows = append(day.rows, row(item))
	}
	if len(day.rows) == 0 {
		_, err := fmt.Fprintf(out, "no %s since %s %s\n", what, prose.DayInYear(dayOf(now, from)), prose.TimeOfDay(now, from))
		return dayTable{}, err
	}
	return day, day.write(out, now, first)
}
