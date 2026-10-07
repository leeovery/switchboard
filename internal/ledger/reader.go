package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"iter"
	"log/slog"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
)

// Reader reads the request ledger back where it lies, in the state directory,
// with no router: its lines, and its days' summaries, today's and those of
// the days not summarised since lines came to be filed under them summarised
// from their lines, with the readings history beside them, as they're read.
type Reader struct {
	days days
	now  func() time.Time
}

// NewReader returns a reader of the ledger in the state directory stateDir,
// by now's clock, what it can't read logged to logger.
func NewReader(stateDir string, now func() time.Time, logger *slog.Logger) *Reader {
	history := readings.Files(readings.Dir(stateDir), logger)
	return &Reader{
		days: days{
			files:   filesIn(Dir(stateDir), logger),
			history: func(from, to time.Time) iter.Seq[readings.Reading] { return readings.Between(history, from, to) },
			logger:  logger,
		},
		now: now,
	}
}

// Held is a line as the ledger holds it: what it reads as, and its JSON, as
// it was written, any field a later release added to it included.
type Held struct {
	Line
	JSON json.RawMessage
}

// Lines returns the lines of the requests that arrived from from on, until
// now, oldest first, by when each arrived, whichever day's file each is in,
// plain or compressed, as dayfile.Read reads them: a day's are written as
// their requests end, which a long one does after others that arrived later.
// A line that doesn't read as one, as one cut short, is passed over, and a
// damaged compressed file read up to the damage: how many lines couldn't be
// read is logged.
func (r *Reader) Lines(from time.Time) iter.Seq[Held] {
	return func(yield func(Held) bool) {
		now := r.now()
		unread := dayfile.Read(r.days.files, dayfile.Dates(from, now), heldIn, arrived, func(h Held) bool {
			switch {
			case h.At.After(now):
				return false
			case h.At.Before(from):
				return true
			}
			return yield(h)
		})
		if unread > 0 {
			r.days.logger.Warn("request ledger lines unread", "lines", unread)
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

// arrived is when the request h holds the line of arrived.
func arrived(h Held) time.Time {
	return h.At
}

// Days returns the summaries of the local days from from's to today's, oldest
// first, every one of them, so the last is today's: the summary the ledger
// holds of a day, as it's held, where it stands, as stands says; else the
// day summarised from its lines and the readings history, as they're read,
// as of today as far as it has gone, the history read once for them all. A
// day of no requests is a summary of no accounts.
func (r *Reader) Days(from time.Time) []Summary {
	dates := dayfile.Span(from, r.now())
	days := make([]Summary, len(dates))
	var unsummarised []int
	for i, date := range dates {
		if held, ok := r.standing(date); ok {
			days[i] = held
		} else {
			unsummarised = append(unsummarised, i)
		}
	}
	if len(unsummarised) == 0 {
		return days
	}
	_, until, _ := dayfile.Day(dates[unsummarised[len(unsummarised)-1]])
	history := readOnce(r.days.history, until)
	for _, i := range unsummarised {
		days[i] = r.summarised(dates[i], history)
	}
	return days
}

// standing returns the summary the ledger holds of the local day with the
// given date, reporting false where the day is to be summarised from its
// lines: the ledger holds no summary of it, or one that can't be read as the
// day's, which is warned of, or one that doesn't stand. One whose lines can't
// be counted to tell is taken as it's held, which is warned of too.
func (r *Reader) standing(date string) (Summary, bool) {
	held, err := r.days.held(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Summary{}, false
	case err != nil:
		r.days.logger.Warn("can't read the request ledger's summary of a day; summarising it from its lines", "day", date, "error", err)
		return Summary{}, false
	}
	stands, err := r.days.stands(held)
	if err != nil {
		r.days.logger.Warn("can't read the request ledger", "day", date, "error", err)
		return held, true
	}
	return held, stands
}

// summarised returns the summary of the local day with the given date from
// its lines and history: of those of its files that can be read, where one
// can't, which is warned of.
func (r *Reader) summarised(date string, history Readings) Summary {
	summary, err := r.days.summarise(date, history)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.days.logger.Warn("can't read the request ledger", "day", date, "error", err)
	}
	return summary
}
