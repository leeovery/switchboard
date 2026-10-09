// Package events keeps the router's events across restarts: a line for each
// version of an event, as the status document gives it, with the run of the
// router that told it, in files a day beside the request ledger's, as
// internal/dayfile keeps them, events-<date>.jsonl, filed under the local day
// each line is written on, and kept as long as the ledger's lines. A Reader
// reads them back where they lie, with no router, each event as it last
// stands.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// queue is how many lines can wait to be written: past that, they're
	// dropped rather than hold the router up.
	queue = 1024
	// lineMax is the longest line read back: an event's is a few hundred
	// bytes, and a longer line, which no event makes, is skipped without being
	// held.
	lineMax = 1 << 16
)

// Line is a version of an event, as the status document gives it, with Run:
// when the router that told it started, as its GET /health gives started_at,
// so the ids of two runs are told apart. Fields may be added to it, never
// renamed.
type Line struct {
	status.Event
	Run time.Time `json:"run"`
	// JSON is the line as it was filed, any field a later release added to it
	// included, of a line read from the files: nil of one made otherwise.
	JSON json.RawMessage `json:"-"`
}

// In returns the line a line of the files holds, its JSON as filed among it,
// reporting false for one that doesn't read as an event: one that isn't
// JSON, or has no run, id, at or kind. Fields it doesn't know are passed
// over.
func In(data []byte) (Line, bool) {
	var line Line
	if json.Unmarshal(data, &line) != nil || !line.valid() {
		return Line{}, false
	}
	line.JSON = slices.Clone(data)
	return line, true
}

// Filed returns the line as the router files it: as it was filed, where it
// was read from the files, else made JSON as the router makes it.
func (l Line) Filed() ([]byte, error) {
	if l.JSON != nil {
		return l.JSON, nil
	}
	return json.Marshal(l)
}

// valid reports whether the line names an event: its run, its id, which
// rises from 1, when it happened and its kind.
func (l Line) valid() bool {
	return !l.Run.IsZero() && l.ID > 0 && !l.At.IsZero() && l.Kind != ""
}

// Files are the router's events' files in dir, the request ledger's
// directory, as dayfile keeps them, what can't be done with them logged to
// logger.
func Files(dir string, logger *slog.Logger) *dayfile.Files {
	return &dayfile.Files{Dir: dir, Prefix: "events", Name: "router's events", LineMax: lineMax, Logger: logger}
}

// Writer writes the lines noted to it to the files, on Run's goroutine, and
// keeps them as long as it was opened to: noting a line never waits, as a
// dayfile.Writer says. A line is written as JSON, filed under the local day
// it's written on, whatever its event's at, and never holds anything shaped
// like a token.
type Writer struct {
	writer *dayfile.Writer[Line]
	now    func() time.Time
	logger *slog.Logger
	// unwritable is set once a line couldn't be put as JSON, which is logged
	// once. Only Run's goroutine touches it.
	unwritable bool
}

// Open returns a writer of the events' files in dir, the request ledger's
// directory, which keeps a day's lines for keep once the day has ended, by
// now's clock, logging what can't be done with them to logger.
func Open(dir string, keep time.Duration, now func() time.Time, logger *slog.Logger) *Writer {
	w := &Writer{now: now, logger: logger}
	w.writer = dayfile.NewWriter(Files(dir, logger), w.lines, dayfile.WriterOptions{Queue: queue, Keep: keep, Now: now, Items: "events"})
	return w
}

// Note queues line for Run to write, which has it, its windows included, from
// then on. It never waits: with the queue full, the line is dropped. An event
// that changes after it's told is noted again, whole, as it then stands.
func (w *Writer) Note(line Line) {
	w.writer.Note(line)
}

// Run writes the lines noted, and keeps the files, as its writer's Run does,
// until ctx ends, when it writes what's still queued.
func (w *Writer) Run(ctx context.Context) {
	w.writer.Run(ctx)
}

// lines returns line as the files' lines, filed under the local day it's
// written on: none, should it not be put as JSON, which is logged once.
func (w *Writer) lines(line Line) dayfile.Lines {
	data, err := json.Marshal(line)
	if err != nil {
		if !w.unwritable {
			w.logger.Warn("router's events can't hold a line; it goes unwritten", "event", line.ID, "error", err)
		}
		w.unwritable = true
		return nil
	}
	lines := make(dayfile.Lines)
	// JSON escapes none of a token's characters, so a token anywhere in the
	// line shows whole, to be hidden, and what hides it needs no escaping.
	lines.Add(w.now(), []byte(redact.Text(string(data))))
	return lines
}
