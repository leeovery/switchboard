// Package export copies the history in the state directory, what can't be
// built again, into a folder: the request ledger's lines, the days'
// summaries, the readings history and the router's events, into a folder of
// the Mac's own in it, mac-<install id>, laid out as the state directory lays
// them out, each day's lines in one compressed file, and an export.json
// saying whose folder it is. It writes a file only where it has changed
// since it was last exported, every one beside where it goes and renamed
// into place, and never removes one.
package export

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// Exporter exports the history in the state directory StateDir, by the clock
// Now, what can't be done with the files it reads logged to Logger.
type Exporter struct {
	StateDir string
	Now      func() time.Time
	Logger   *slog.Logger
}

// Result is what an export did: the Mac's folder it exported into, and how
// many days, each a date of which the state directory holds a file of any
// kind, it wrote a file of, and how many it left as they were.
type Result struct {
	Folder              string
	Written, AsTheyWere int
}

// Days returns how many days the export exported.
func (r Result) Days() int {
	return r.Written + r.AsTheyWere
}

// Export exports the history into the Mac's folder in dir, making each as
// it's needed, the user's alone: each day's file of lines, compressed as
// dayLines.export says, and each day's summary, copied as copyFile says; then
// export.json, saying when. It fails where dir is a file, or can't be made or
// written, and where a file of the history can't be read.
func (e Exporter) Export(dir string) (Result, error) {
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return Result{}, fmt.Errorf("can't export into %s: it's a file, not a folder", dir)
	}
	id, err := InstallID(e.StateDir)
	if err != nil {
		return Result{}, err
	}
	folder := Folder(dir, id)
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
	}
	written := make(map[string]bool)
	for _, lines := range e.lines(folder) {
		if err := exportLines(lines, written); err != nil {
			return Result{}, err
		}
	}
	if err := exportSummaries(ledger.Dir(e.StateDir), ledger.Dir(folder), written); err != nil {
		return Result{}, err
	}
	if err := writeManifest(folder, id, e.Now()); err != nil {
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
	}
	return resultOf(folder, written), nil
}

// lines are the files of lines the state directory holds, each beside where
// the Mac's folder at folder holds them.
func (e Exporter) lines(folder string) []dayLines {
	return []dayLines{
		{src: ledger.Files(ledger.Dir(e.StateDir), e.Logger), dst: ledger.Files(ledger.Dir(folder), e.Logger)},
		{src: events.Files(ledger.Dir(e.StateDir), e.Logger), dst: events.Files(ledger.Dir(folder), e.Logger)},
		{src: readings.Files(readings.Dir(e.StateDir), e.Logger), dst: readings.Files(readings.Dir(folder), e.Logger)},
	}
}

// exportLines exports each day lines' files in the state directory hold,
// noting in written, by its date, whether it wrote the day's file.
func exportLines(lines dayLines, written map[string]bool) error {
	dates, err := lines.src.All()
	if err != nil {
		return fmt.Errorf("read the %s: %w", lines.src.Name, err)
	}
	for _, date := range dates {
		wrote, err := lines.export(date)
		if err != nil {
			return fmt.Errorf("export the %s of %s: %w", lines.src.Name, date, err)
		}
		written[date] = written[date] || wrote
	}
	return nil
}

// exportSummaries copies each day's summary the ledger in src holds to the
// ledger in dst, as copyFile copies it, noting in written, by its date,
// whether it wrote it.
func exportSummaries(src, dst string, written map[string]bool) error {
	dates, err := ledger.SummaryDates(src)
	if err != nil {
		return fmt.Errorf("read the request ledger's summaries: %w", err)
	}
	for _, date := range dates {
		wrote, err := copyFile(ledger.SummaryFile(src, date), ledger.SummaryFile(dst, date))
		if err != nil {
			return fmt.Errorf("export the request ledger's summary of %s: %w", date, err)
		}
		written[date] = written[date] || wrote
	}
	return nil
}

// resultOf is the result of an export into folder that wrote a file of each
// day written reports true of, and none of those it reports false of.
func resultOf(folder string, written map[string]bool) Result {
	r := Result{Folder: folder}
	for _, wrote := range written {
		if wrote {
			r.Written++
		} else {
			r.AsTheyWere++
		}
	}
	return r
}
