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

// LookedDay is a staged day as Look found the ledger before anything of its
// folder was taken in: Unseen, where its summary is staged, and the ledger
// held neither a summary of the day, readable or not, nor any line of it, as
// of a day it never saw, or one past keeping whose summary is its only
// record, so the summary staged is copied in.
type LookedDay struct {
	StagedDay
	Unseen bool
}

// ImportedDay is what taking a staged day in did: how its lines were taken
// in, as dayfile.Imported says, and whether its summary was copied in.
type ImportedDay struct {
	Lines   dayfile.Imported
	Summary bool
}

// Look looks at the ledger for each of the days staged, as LookedDay says,
// asked before anything of their folder is taken in: a round may summarise
// one of them before its turn comes, as from the readings history's days
// taken in, and that summary is the import's to replace. It fails where the
// ledger can't be looked at.
func (l *Ledger) Look(days []StagedDay) ([]LookedDay, error) {
	looked := make([]LookedDay, len(days))
	for i, day := range days {
		looked[i].StagedDay = day
		if day.Summary == "" {
			continue
		}
		unseen, err := l.days.unseen(day.Date)
		if err != nil {
			return nil, fmt.Errorf("import the request ledger's summary of %s: %w", day.Date, err)
		}
		looked[i].Unseen = unseen
	}
	return looked, nil
}

// Import takes the day looked at in, as one piece of work on Run's
// goroutine, between the writer's writes and its rounds: its lines, as the
// writer's ImportThen takes a day in, by the ledger's keep and clock, then
// its summary, as copySummary copies it, whatever the keep, as summaries are
// kept for good; so a caller whose context ends finds the day with both, or
// neither. A day not unseen keeps the ledger's own summary, which its round
// makes again where the day's lines changed. It fails as ImportThen does,
// and where the staged summary can't be read or the summary written.
func (l *Ledger) Import(ctx context.Context, day LookedDay) (ImportedDay, error) {
	var copied bool
	copySummary := func() (err error) {
		copied, err = l.copySummary(day)
		return err
	}
	var lines dayfile.Imported
	var err error
	if day.Lines == "" {
		if done := l.writer.Do(ctx, func() { err = copySummary() }); done != nil {
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

// copySummary copies the summary staged of the day looked at in, where it's
// unseen, as copyIn copies it, over any a round wrote since it was looked
// at, holding summarising, so no round writes one as it does, reporting
// whether it did.
func (l *Ledger) copySummary(day LookedDay) (bool, error) {
	if !day.Unseen {
		return false, nil
	}
	l.summarising.Lock()
	defer l.summarising.Unlock()
	copied, err := l.days.copyIn(day.StagedDay)
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
