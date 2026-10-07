package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"iter"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
)

// Reader reads the request ledger back where it lies, in the state directory,
// with no router: its lines, and its days' summaries, today's and those of
// the days not yet summarised summarised from their lines, with the readings
// history beside them, as they're read.
type Reader struct {
	files   *dayfile.Files
	history Readings
	now     func() time.Time
	logger  *slog.Logger
}

// NewReader returns a reader of the ledger in the state directory stateDir,
// by now's clock, what it can't read logged to logger.
func NewReader(stateDir string, now func() time.Time, logger *slog.Logger) *Reader {
	history := readings.Files(readings.Dir(stateDir), logger)
	return &Reader{
		files:   filesIn(Dir(stateDir), logger),
		history: func(from, to time.Time) iter.Seq[readings.Reading] { return readings.Between(history, from, to) },
		now:     now,
		logger:  logger,
	}
}

// Held is a line as the ledger holds it: what it reads as, and its JSON, as
// it was written, any field a later release added to it included.
type Held struct {
	Line
	JSON json.RawMessage
}

// Lines returns the lines of the requests that arrived from from on, until
// now, oldest first, whichever day's file each is in, plain or compressed. A
// line that doesn't read as one, as one cut short, is passed over, and a
// damaged compressed file read up to the damage, as dayfile.Read says: how
// many lines couldn't be read is logged.
func (r *Reader) Lines(from time.Time) iter.Seq[Held] {
	return func(yield func(Held) bool) {
		now := r.now()
		unread := 0
		defer func() {
			if unread > 0 {
				r.logger.Warn("request ledger lines unread", "lines", unread)
			}
		}()
		for _, date := range dayfile.Dates(from, now) {
			var day []Held
			unread += dayfile.Read(r.files, []string{date}, heldIn, func(h Held) bool {
				if !h.At.Before(from) && !h.At.After(now) {
					day = append(day, h)
				}
				return true
			})
			// A day's lines are written as their requests end, which a long one
			// does after others that arrived later.
			slices.SortStableFunc(day, func(a, b Held) int { return a.At.Compare(b.At) })
			for _, h := range day {
				if !yield(h) {
					return
				}
			}
		}
	}
}

// heldIn returns the line a line of the ledger holds, as it's held, reporting
// false for one that doesn't read as a line.
func heldIn(data []byte) (Held, bool) {
	line, ok := lineIn(data)
	if !ok {
		return Held{}, false
	}
	return Held{Line: line, JSON: slices.Clone(data)}, true
}

// Days returns the summaries of the local days from from's to today's, oldest
// first, of each day the ledger holds: the summary it holds of the day, as
// it's held, never summarised again; else, as of today, as far as it has gone,
// and any day not yet summarised, the day summarised from its lines and the
// readings history, as they're read. A day of no summary none of whose lines
// read is left out.
func (r *Reader) Days(from time.Time) []Summary {
	var days []Summary
	for _, date := range dayfile.Span(from, r.now()) {
		if summary, ok := r.day(date); ok {
			days = append(days, summary)
		}
	}
	return days
}

// day returns the summary of the local day with the given date, as Days gives
// it, reporting false for a day it leaves out.
func (r *Reader) day(date string) (Summary, bool) {
	if summary, ok := r.held(date); ok {
		return summary, true
	}
	summary, unread, err := summariseDay(r.files, date, r.history)
	if unread > 0 {
		r.logger.Warn("request ledger lines unread", "day", date, "lines", unread)
	}
	return summary, err == nil && summary.requests() > 0
}

// held returns the summary the ledger holds of the local day with the given
// date, reporting false where it holds none, or one that can't be read, which
// is warned of: the day is summarised from its lines instead, as far as
// they're kept.
func (r *Reader) held(date string) (Summary, bool) {
	data, err := os.ReadFile(summaryFile(r.files.Dir, date))
	if errors.Is(err, fs.ErrNotExist) {
		return Summary{}, false
	}
	var summary Summary
	if err == nil {
		err = json.Unmarshal(data, &summary)
	}
	if err != nil {
		r.logger.Warn("can't read the request ledger's summary of a day; summarising it from its lines", "day", date, "error", err)
		return Summary{}, false
	}
	return summary, true
}
