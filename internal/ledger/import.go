package ledger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// StagedDay is a day of the ledger's staged to import, by its date: where the
// file of its lines is staged, as export writes a day's, and where its
// summary is, each "" where none is.
type StagedDay struct {
	Date           string
	Lines, Summary string
}

// ImportedDay is what taking a staged day in did: how its lines were taken
// in, as dayfile.Imported says, and whether its summary was copied in.
type ImportedDay struct {
	Lines   dayfile.Imported
	Summary bool
}

// Import takes the staged day in: its lines as the ledger's writer's Import
// takes a day in, on Run's goroutine, by the ledger's keep and clock; then
// its summary, as copySummary copies it, where the ledger held neither a
// summary of the day nor any line of it before its lines were taken in, as
// of a day it never saw, or one past keeping whose summary is its only
// record, whatever the keep, as summaries are kept for good. Else the
// ledger's own stands, and its round summarises the day again where its
// lines changed. It fails as the writer's Import does, and where the
// ledger's files can't be looked at, the staged summary read or the summary
// written, keeping what it took in before.
func (l *Ledger) Import(ctx context.Context, day StagedDay) (ImportedDay, error) {
	unseen, err := l.unseen(day)
	if err != nil {
		return ImportedDay{}, err
	}
	var imported ImportedDay
	if day.Lines != "" {
		if imported.Lines, err = l.writer.Import(ctx, day.Date, day.Lines); err != nil {
			return ImportedDay{}, err
		}
	}
	if unseen {
		if imported.Summary, err = l.copySummary(ctx, day); err != nil {
			return ImportedDay{}, err
		}
	}
	return imported, nil
}

// RoundNow makes the ledger's round at once, on Run's goroutine, as it makes
// one every hour, summarising the days that have ended and kicking its
// digesting, as SummariseEnded does, and returns as dayfile.Writer's RoundNow
// does.
func (l *Ledger) RoundNow(ctx context.Context) error {
	return l.writer.RoundNow(ctx)
}

// unseen reports whether a summary of the staged day is staged, and the
// ledger holds neither a summary of the day, readable or not, nor any line
// of it, as days.unseen says.
func (l *Ledger) unseen(day StagedDay) (bool, error) {
	if day.Summary == "" {
		return false, nil
	}
	unseen, err := l.days.unseen(day.Date)
	if err != nil {
		return false, fmt.Errorf("import the request ledger's summary of %s: %w", day.Date, err)
	}
	return unseen, nil
}

// copySummary copies the summary staged of the day in, as copyIn copies it,
// on Run's goroutine, holding summarising, so no round writes a summary as
// it does, reporting whether it did; or, where it didn't wait for that, why,
// as dayfile.Writer's Do says.
func (l *Ledger) copySummary(ctx context.Context, day StagedDay) (bool, error) {
	var copied bool
	var err error
	if done := l.writer.Do(ctx, func() {
		l.summarising.Lock()
		defer l.summarising.Unlock()
		copied, err = l.days.copyIn(day)
	}); done != nil {
		return false, done
	}
	if err != nil {
		return false, fmt.Errorf("import the request ledger's summary of %s: %w", day.Date, err)
	}
	return copied, nil
}

// unseen reports whether the ledger holds neither a summary of the local day
// with the given date, readable or not, nor any line of it. It fails where
// it can't look.
func (d days) unseen(date string) (bool, error) {
	if _, err := os.Stat(SummaryFile(d.files.Dir, date)); !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	_, err := d.files.Stat(date)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	return false, err
}

// copyIn copies the summary staged of the day in, as the ledger's summary of
// its day, with its stamp, its modification time, beside where it goes and
// renamed into place, reporting whether it did. It copies none that doesn't
// read as the day's, as one cut short as it synced, which is warned of: in
// place, it would stand in the way of the whole one a later import brings.
func (d days) copyIn(day StagedDay) (bool, error) {
	data, stamp, err := readStamped(day.Summary)
	if err != nil {
		return false, err
	}
	if _, err := summaryIn(data, day.Date); err != nil {
		d.logger.Warn("can't read the request ledger's summary of a day staged to import; it's left out", "day", day.Date, "file", day.Summary, "error", err)
		return false, nil
	}
	if err := os.MkdirAll(d.files.Dir, 0o700); err != nil {
		return false, err
	}
	err = atomicfile.WriteModified(SummaryFile(d.files.Dir, day.Date), 0o600, stamp, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
	return err == nil, err
}
