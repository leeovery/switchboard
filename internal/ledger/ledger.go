// Package ledger is the request ledger: a line for each request the router
// routes, written as the router finishes with it, which holds everything
// about the request but its content, in files a day, as internal/dayfile
// keeps them: requests-<date>.jsonl, compressed two days after its day ends,
// and removed once its day is past keeping; and a summary of each day once it
// has ended, day-<date>.json beside them, kept for good.
package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/redact"
)

const (
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

// Ledger writes the lines noted to it to its files, on Run's goroutine, and
// keeps them as long as it was opened to: noting a line never waits, as a
// dayfile.Writer says. A line is written as JSON, filed under the local day
// its request arrived on, and never holds anything shaped like a token. On
// its writer's round, it summarises each day that has ended.
type Ledger struct {
	writer   *dayfile.Writer[*Line]
	files    *dayfile.Files
	readings Readings
	logger   *slog.Logger
	// unwritable is set once a line couldn't be put as JSON, which is logged
	// once. Only Run's goroutine touches it.
	unwritable bool
}

// Open returns the ledger in dir, which keeps a day's lines for keep once the
// day has ended, by now's clock, and summarises its days with the readings
// readings gives, logging what can't be done with them to logger.
func Open(dir string, keep time.Duration, now func() time.Time, readings Readings, logger *slog.Logger) *Ledger {
	l := &Ledger{
		files:    &dayfile.Files{Dir: dir, Prefix: "requests", Name: "request ledger", LineMax: lineMax, Logger: logger},
		readings: readings,
		logger:   logger,
	}
	l.writer = dayfile.NewWriter(l.files, l.lines, dayfile.WriterOptions{Queue: queue, Keep: keep, Now: now, Items: "lines", Round: l.summarise})
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
			l.logger.Warn("request ledger can't hold a line; it goes unwritten", "request", written.Request, "error", err)
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

// summarise writes the summary of each day whose lines the ledger holds that
// ended summariseAfter or more before now, and has none yet: a summary is
// written once. One that can't be written is logged, and left to the next
// round.
func (l *Ledger) summarise(now time.Time) {
	for _, date := range l.files.Ended(now, summariseAfter) {
		path := filepath.Join(l.files.Dir, "day-"+date+".json")
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := l.writeSummary(date, path); err != nil {
			l.logger.Warn("can't summarise the request ledger", "day", date, "error", err)
		}
	}
}

// writeSummary writes the summary of the day with the given date to path,
// whole, the user's alone, once any of the day's lines read, logging how many
// couldn't be read, where any couldn't. A day none of whose lines read, as
// one whose file can't be read for now, is left for a round that finds them.
func (l *Ledger) writeSummary(date, path string) error {
	unread := 0
	lines := func(yield func(Line) bool) {
		unread = dayfile.Read(l.files, []string{date}, lineIn, yield)
	}
	summary, err := Summarise(date, lines, l.readings)
	if err != nil || summary.requests() == 0 {
		return err
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if unread > 0 {
		l.logger.Warn("request ledger lines unread", "day", date, "lines", unread)
	}
	l.logger.Info("summarised a day of the request ledger", "day", date, "requests", summary.requests())
	return nil
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
