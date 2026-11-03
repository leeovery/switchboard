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

// Import takes the day staged in, as one piece of work on Run's goroutine,
// between the writer's writes and its rounds: its lines, as the writer's
// ImportThen takes a day in, by the ledger's keep and clock, then its
// summary, as copySummary decides, whatever the keep, as summaries are kept
// for good. So whatever ran before, a round, a stop or an import cut short,
// the summary is decided on the files as its lines left them. Work taken up
// runs to its end, which leaves the day with both or neither, though a
// caller whose context ends first returns before it does, as Do says. It
// fails as ImportThen does, and where the ledger's files can't be looked at,
// the staged summary read, or the summary written.
func (l *Ledger) Import(ctx context.Context, day StagedDay) (ImportedDay, error) {
	var copied bool
	copySummary := func(took dayfile.Imported) (err error) {
		copied, err = l.copySummary(day, took)
		return err
	}
	var lines dayfile.Imported
	var err error
	if day.Lines == "" {
		if done := l.writer.Do(ctx, func() { err = copySummary(dayfile.Imported{}) }); done != nil {
			return ImportedDay{}, done
		}
	} else {
		lines, err = l.writer.ImportThen(ctx, day.Date, day.Lines, copySummary)
	}
	if err != nil {
		return ImportedDay{}, err
	}
	return ImportedDay{Lines: lines, Summary: copied}, nil
}

// RoundNow makes the ledger's round at once, on Run's goroutine, as it makes
// one every hour, summarising the days that have ended and kicking its
// digesting, as SummariseEnded does, and returns as dayfile.Writer's RoundNow
// does.
func (l *Ledger) RoundNow(ctx context.Context) error {
	return l.writer.RoundNow(ctx)
}

// copySummary copies the summary staged of the day in, its lines taken in as
// took says, as copyIn copies it, where the ledger holds no line of the day,
// as holdsNoLine says, and no summary of it that counts one, as countsNoLine
// says: what the ledger knows of the day's requests is then nothing, and the
// summary staged knows more. It holds summarising, so no round writes a
// summary between its look and its copy, and reports whether it copied one.
func (l *Ledger) copySummary(day StagedDay, took dayfile.Imported) (bool, error) {
	if day.Summary == "" {
		return false, nil
	}
	l.summarising.Lock()
	defer l.summarising.Unlock()
	copied, err := l.days.copyOver(day, took)
	if err != nil {
		return false, fmt.Errorf("import the request ledger's summary of %s: %w", day.Date, err)
	}
	return copied, nil
}

// copyOver copies the summary staged of the day in, as copyIn does, over
// none or one that knows nothing of the day's requests, where holdsNoLine and
// countsNoLine say the ledger knows nothing of them, its lines taken in as
// took says, reporting whether it did.
func (d days) copyOver(day StagedDay, took dayfile.Imported) (bool, error) {
	if none, err := d.holdsNoLine(day.Date, took); err != nil || !none {
		return false, err
	}
	if none, err := d.countsNoLine(day.Date); err != nil || !none {
		return false, err
	}
	return d.copyIn(day)
}

// holdsNoLine reports whether the ledger holds no line of the local day with
// the given date, its lines just taken in as took says: it held none before
// where they were added, and some where they were merged; else, as of a day
// past keeping or one staged of no line, it's as its files are now. It fails
// where it can't look.
func (d days) holdsNoLine(date string, took dayfile.Imported) (bool, error) {
	switch took.Taken {
	case dayfile.Added:
		return true, nil
	case dayfile.Merged:
		return false, nil
	}
	_, err := d.files.Stat(date)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	return false, err
}

// countsNoLine reports whether the ledger holds no summary of the local day
// with the given date that counts a line: none, or one of no line, as a round
// summarises a day from the readings alone. One that doesn't read as the
// day's may have counted some, which can't be had again, so it stands, as a
// round leaves it. It fails where the summary can't be read.
func (d days) countsNoLine(date string) (bool, error) {
	held, err := d.held(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case errors.Is(err, errNotTheDaysSummary):
		return false, nil
	case err != nil:
		return false, err
	}
	return held.Lines == 0, nil
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
