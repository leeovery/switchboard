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
	"errors"
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

	// looked, where it's set, is called with each day's date once its files
	// of lines are looked at, before they're read, as dayLines says.
	looked func(date string)
}

// Result is what an export did: the Mac's folder it exported into; how many
// days, each a date of which the state directory holds a file of any kind,
// it wrote a file of, and how many it left as they were; and the days' files
// it couldn't read, and so passed over.
type Result struct {
	Folder              string
	Written, AsTheyWere int
	Unread              []DayError
}

// Days returns how many days the export exported.
func (r Result) Days() int {
	return r.Written + r.AsTheyWere
}

// DayError is a file of a day's the state directory holds that an export
// couldn't read, and passed over: What it was, as "the request ledger of
// 2026-10-30", the day's Date, and why.
type DayError struct {
	What, Date string
	Err        error
}

func (e DayError) Error() string {
	return "couldn't export " + e.What + ": " + e.Err.Error()
}

func (e DayError) Unwrap() error {
	return e.Err
}

// Export exports the history into the Mac's folder in dir, making each as
// it's needed, the user's alone: each day's file of lines, as dayLines.export
// says, and each day's summary, copied as copyFile says; then export.json,
// saying when. A day's file that can't be read is passed over, and named in
// the result. It fails where dir is a file, or anything in it can't be made
// or written, and where the state directory's files can't be listed.
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
	x := exporting{written: make(map[string]bool)}
	for _, lines := range e.lines(folder) {
		if err := x.lines(lines); err != nil {
			return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
		}
	}
	if err := x.summaries(ledger.Dir(e.StateDir), ledger.Dir(folder)); err != nil {
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
	}
	if err := writeManifest(folder, id, e.Now()); err != nil {
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
	}
	return x.result(folder), nil
}

// lines are the files of lines the state directory holds, each beside where
// the Mac's folder at folder holds them.
func (e Exporter) lines(folder string) []dayLines {
	return []dayLines{
		{src: ledger.Files(ledger.Dir(e.StateDir), e.Logger), dst: ledger.Files(ledger.Dir(folder), e.Logger), looked: e.looked},
		{src: events.Files(ledger.Dir(e.StateDir), e.Logger), dst: events.Files(ledger.Dir(folder), e.Logger), looked: e.looked},
		{src: readings.Files(readings.Dir(e.StateDir), e.Logger), dst: readings.Files(readings.Dir(folder), e.Logger), looked: e.looked},
	}
}

// exporting is what an export has done so far: whether it wrote a file of
// each day, by its date, and the days' files it couldn't read.
type exporting struct {
	written map[string]bool
	unread  []DayError
}

// lines exports each day lines' files in the state directory hold.
func (x *exporting) lines(lines dayLines) error {
	dates, err := lines.src.All()
	if err != nil {
		return fmt.Errorf("read the %s: %w", lines.src.Name, err)
	}
	for _, date := range dates {
		wrote, err := lines.export(date)
		if err := x.note("the "+lines.src.Name+" of "+date, date, wrote, err); err != nil {
			return err
		}
	}
	return nil
}

// summaries copies each day's summary the ledger in src holds to the ledger
// in dst, as copyFile copies it.
func (x *exporting) summaries(src, dst string) error {
	dates, err := ledger.SummaryDates(src)
	if err != nil {
		return fmt.Errorf("read the request ledger's summaries: %w", err)
	}
	for _, date := range dates {
		wrote, err := copyFile(ledger.SummaryFile(src, date), ledger.SummaryFile(dst, date))
		if err := x.note("the request ledger's summary of "+date, date, wrote, err); err != nil {
			return err
		}
	}
	return nil
}

// note notes how exporting what's given, of the day with the given date,
// went: whether it wrote it, or, where err is an unreadable, that it was
// passed over, and why. It returns any other error, saying what it was of.
func (x *exporting) note(what, date string, wrote bool, err error) error {
	if u, ok := errors.AsType[unreadable](err); ok {
		x.unread = append(x.unread, DayError{What: what, Date: date, Err: u.error})
		return nil
	}
	if err != nil {
		return fmt.Errorf("export %s: %w", what, err)
	}
	x.written[date] = x.written[date] || wrote
	return nil
}

// result is the result of the export into folder.
func (x *exporting) result(folder string) Result {
	r := Result{Folder: folder, Unread: x.unread}
	for _, wrote := range x.written {
		if wrote {
			r.Written++
		} else {
			r.AsTheyWere++
		}
	}
	return r
}
