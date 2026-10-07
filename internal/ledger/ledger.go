// Package ledger is the request ledger: a line for each request the router
// routes, written as the router finishes with it, which holds everything
// about the request but its content, in files a day, as internal/dayfile
// keeps them: requests-<date>.jsonl, compressed two days after its day ends,
// and removed once its day is past keeping; and a summary of each day once it
// has ended, day-<date>.json beside them, written again while lines come to be
// filed under the day, and kept for good. A Reader reads them back where they
// lie, with no router, and a Table prices what they hold.
package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/redact"
)

const (
	// dirName is the ledger's directory in the state directory.
	dirName = "ledger"
	// queue is how many lines can wait to be written: past that, they're
	// dropped rather than hold a request up. It's eight times the readings
	// history's, as a line dropped is a request never counted.
	queue = 8192
	// lineMax is the longest line read back: a request's is a kilobyte or
	// so, and the longest written, every text, list and count at its most and
	// each character escaped, runs to some 450 KiB.
	lineMax = 1 << 20
	// summariseAfter is how long after its day ends a day is summarised: a
	// request still in flight as the day ends has its line filed under the
	// day once it's done, which an hour leaves it time for.
	summariseAfter = time.Hour
)

// Dir is the ledger's directory in the state directory stateDir.
func Dir(stateDir string) string {
	return filepath.Join(stateDir, dirName)
}

// filesIn are the ledger's files of lines in dir, what can't be done with
// them logged to logger.
func filesIn(dir string, logger *slog.Logger) *dayfile.Files {
	return &dayfile.Files{Dir: dir, Prefix: "requests", Name: "request ledger", LineMax: lineMax, Logger: logger}
}

// summaryFile is where the ledger in dir keeps the summary of the local day
// with the given date.
func summaryFile(dir, date string) string {
	return filepath.Join(dir, "day-"+date+".json")
}

// Ledger writes the lines noted to it to its files, on Run's goroutine, and
// keeps them as long as it was opened to: noting a line never waits, as a
// dayfile.Writer says. A line is written as JSON, filed under the local day
// its request arrived on, and never holds anything shaped like a token. On
// its writer's round, it summarises the days that have ended, as
// SummariseEnded says.
type Ledger struct {
	writer *dayfile.Writer[*Line]
	days   days
	// summarising is held while the days are summarised, as they are from
	// the readings history's writer's goroutine as well as Run's.
	summarising sync.Mutex
	// unwritable is set once a line couldn't be put as JSON, which is logged
	// once. Only Run's goroutine touches it.
	unwritable bool
}

// Open returns the ledger in dir, which keeps a day's lines for keep once the
// day has ended, by now's clock, and summarises its days with the readings
// history gives, logging what can't be done with them to logger.
func Open(dir string, keep time.Duration, now func() time.Time, history Readings, logger *slog.Logger) *Ledger {
	l := &Ledger{days: days{files: filesIn(dir, logger), history: history, logger: logger}}
	l.writer = dayfile.NewWriter(l.days.files, l.lines, dayfile.WriterOptions{Queue: queue, Keep: keep, Now: now, Items: "lines", Round: l.SummariseEnded})
	return l
}

// Note queues line for Run to write, which has it from then on. It never
// waits: with the queue full, the line is dropped.
func (l *Ledger) Note(line *Line) {
	l.writer.Note(line)
}

// Run writes the lines noted, and keeps the ledger's files, as its writer's
// Run does, until ctx ends, when it writes what's still queued.
func (l *Ledger) Run(ctx context.Context) {
	l.writer.Run(ctx)
}

// lines returns line as the ledger's lines, filed under the local day its
// request arrived on: none, should it not be put as JSON, which is logged
// once.
func (l *Ledger) lines(line *Line) dayfile.Lines {
	written := line.written()
	data, err := json.Marshal(written)
	if err != nil {
		if !l.unwritable {
			l.days.logger.Warn("request ledger can't hold a line; it goes unwritten", "request", written.Request, "error", err)
		}
		l.unwritable = true
		return nil
	}
	lines := make(dayfile.Lines)
	// JSON escapes none of a token's characters, so a token anywhere in the
	// line shows whole, to be hidden, and what hides it needs no escaping.
	lines.Add(written.At, []byte(redact.Text(string(data))))
	return lines
}

// SummariseEnded summarises each day whose lines the ledger holds that ended
// summariseAfter or more before now, and has no summary that stands, as
// standing says, the readings history read once for them all: the summary is
// written whole, the user's alone, over any there. A day that can't be
// summarised, as one of its files can't be opened, which a summary of it in
// part would be taken for whole, is logged, and left to the next round. It's
// safe to call from any goroutine: the readings history's writer calls it
// before it prunes the readings the summaries need.
func (l *Ledger) SummariseEnded(now time.Time) {
	l.summarising.Lock()
	defer l.summarising.Unlock()
	var due []string
	for _, date := range l.days.files.Ended(now, summariseAfter) {
		if stands, err := l.days.standing(date); err != nil {
			l.days.logger.Warn("can't summarise the request ledger", "day", date, "error", err)
		} else if !stands {
			due = append(due, date)
		}
	}
	if len(due) == 0 {
		return
	}
	_, until, _ := dayfile.Day(due[len(due)-1])
	history := readOnce(l.days.history, until)
	for _, date := range due {
		if err := l.writeSummary(date, history); err != nil {
			l.days.logger.Warn("can't summarise the request ledger", "day", date, "error", err)
		}
	}
}

// writeSummary writes the summary of the local day with the given date, from
// its lines and history, whole, the user's alone, over any there. It fails
// where one of the day's files can't be opened, writing none, and writes none
// of a day whose files are gone, as when they were pruned once listed.
func (l *Ledger) writeSummary(date string, history Readings) error {
	summary, err := l.days.summarise(date, history)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(summaryFile(l.days.files.Dir, date), append(data, '\n'), 0o600); err != nil {
		return err
	}
	l.days.logger.Info("summarised a day of the request ledger", "day", date, "requests", summary.requests())
	return nil
}

// days are the ledger's days where they lie, in its directory: their lines,
// and their summaries beside them, summarised with the readings history
// gives, what can't be done with them logged.
type days struct {
	files   *dayfile.Files
	history Readings
	logger  *slog.Logger
}

// errNotTheDaysSummary is the error of a summary held of a day that doesn't
// read as the day's.
var errNotTheDaysSummary = errors.New("not the day's summary")

// held returns the summary the ledger holds of the local day with the given
// date, failing with fs.ErrNotExist where it holds none, and with
// errNotTheDaysSummary where what it holds doesn't read as the day's, as one
// whose day is another's.
func (d days) held(date string) (Summary, error) {
	data, err := os.ReadFile(summaryFile(d.files.Dir, date))
	if err != nil {
		return Summary{}, err
	}
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return Summary{}, fmt.Errorf("%w: %w", errNotTheDaysSummary, err)
	}
	if summary.Day != date {
		return Summary{}, fmt.Errorf("%w: it's of %q", errNotTheDaysSummary, summary.Day)
	}
	return summary, nil
}

// standing reports whether the summary the ledger holds of the local day
// with the given date stands, as stands says: false where it holds none, or
// one that doesn't read as the day's, which is warned of, so the day is
// summarised from its lines. It fails where the summary can't be read, or
// the day's lines counted, to tell.
func (d days) standing(date string) (bool, error) {
	held, err := d.held(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case errors.Is(err, errNotTheDaysSummary):
		d.logger.Warn("can't read the request ledger's summary of a day; summarising it from its lines", "day", date, "error", err)
		return false, nil
	case err != nil:
		return false, err
	}
	return d.stands(held)
}

// stands reports whether held, a summary the ledger holds, stands: its day's
// lines are pruned, so it stands for good, or the day's files hold no more
// than it was made from. A line can come to be filed under a day after it's
// summarised, as one of a request in flight past the hour the day is given,
// or one filed after a change of time zone, or a clock set ahead and set
// right again; but none is taken away while they're kept, so fewer means some
// were lost since, as to a damaged file, or pruned before others came to be
// filed under the day, as after a clock set back, and the summary made from
// more stands. It fails where the lines can't be counted, as when one of the
// day's files can't be opened.
func (d days) stands(held Summary) (bool, error) {
	lines, err := d.files.Count(held.Day)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, err
	}
	return lines <= held.Lines, nil
}

// summarise returns the summary of the local day with the given date from its
// lines and history, warning of how many of the lines couldn't be read as
// one, where any couldn't. Where one of the day's files can't be opened, it
// fails, giving the summary of those that could be all the same; and with
// fs.ErrNotExist where the day has none, giving a summary of none.
func (d days) summarise(date string, history Readings) (Summary, error) {
	var unread int
	var read error
	summary, err := Summarise(date, func(yield func(Line) bool) {
		_, unread, read = dayfile.ReadDay(d.files, date, lineIn, yield)
	}, history)
	if err != nil {
		return Summary{}, err
	}
	summary.Lines += unread
	if unread > 0 {
		d.logger.Warn("request ledger lines unread", "day", date, "lines", unread)
	}
	return summary, read
}

// lineIn returns the line a line of the ledger holds, reporting false for one
// that doesn't read as a line, as one cut short.
func lineIn(data []byte) (Line, bool) {
	var line Line
	if json.Unmarshal(data, &line) != nil {
		return Line{}, false
	}
	return line, true
}
