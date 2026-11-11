// Package export copies the history in the state directory, what can't be
// built again, into a folder: the request ledger's lines, the days'
// summaries, the readings history, the router's events and the accounts
// record, which names the accounts the rest name by their ids, into a folder of
// the Mac's own in it, mac-<install id>, laid out as the state directory lays
// them out, each day's lines in one compressed file, and an export.json
// saying whose folder it is. It writes a file only where it has changed
// since it was last exported, every one beside where it goes and renamed
// into place, and removes none but those writes cut short left. It stages the Macs' folders an import
// brings in, too, in the state directory, for the router to take in.
package export

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// Exporter exports the history in the state directory StateDir, by the clock
// Now, what can't be done with the files it reads logged to Logger, the
// accounts record's accounts labelled as Accounts, those configured, labels
// them.
type Exporter struct {
	StateDir string
	Now      func() time.Time
	Logger   *slog.Logger
	Accounts config.Accounts
	// NoParents, where it's set, makes the folder exported into only while
	// the folder it goes in is there, and only where it can be made in it, so
	// one on a disk that isn't mounted, as /Volumes/<disk>/…, fails with
	// ErrNoFolder rather than be made on the Mac's own disk; and makes no
	// folder in the Mac's folder once that's gone, as with its disk ejected
	// mid-export. Else each is made with every folder it goes in.
	NoParents bool

	// looked and wrap, where they're set, are as dayLines has them.
	looked func(date string)
	wrap   func(io.Writer) io.Writer
}

// Result is what an export did: the Mac's folder it exported into; how many
// days, each a date of which the state directory holds a file of any kind,
// it wrote a file of, and how many it left as they were; the days' files it
// couldn't read, and so passed over; and the files writes cut short left in
// the Mac's folder that it couldn't remove.
type Result struct {
	Folder              string
	Written, AsTheyWere int
	Unread              []DayError
	Unremoved           []Leftover
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

// Leftover is a file a write cut short left in the Mac's folder, as
// removeLeftovers finds one, that an export couldn't remove: its Path, and
// why.
type Leftover struct {
	Path string
	Err  error
}

// Export exports the history into the Mac's folder in dir, making each as
// it's needed, the user's alone, as NoParents says, and removing first what
// writes cut short left in it, as removeLeftovers says: the accounts record,
// as exporting.record writes it, first, so the days a sync carries are never
// ahead of the record that names their accounts; each day's file of lines,
// as dayLines.export says; each day's summary, copied as copyFile says; then
// export.json, saying when, and whether it wrote the record. A day's file that can't be read is
// passed over, and named in the result, as is a file writes cut short left
// that can't be removed. It fails where dir is a file, or
// anything in it can't be made or written, and where the state directory's
// files can't be listed; with NoParents, where dir can't be made, before it
// writes anything.
func (e Exporter) Export(dir string) (Result, error) {
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return Result{}, fmt.Errorf("can't export into %s: it's a file, not a folder", dir)
	}
	if e.NoParents {
		if err := makeDir(dir); err != nil {
			return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
		}
	}
	id, err := InstallID(e.StateDir)
	if err != nil {
		return Result{}, err
	}
	folder := Folder(dir, id)
	switch err := e.making(dir)(folder); {
	case errors.Is(err, ErrNoFolder):
		// The Mac's folder is a level in dir, so the folder not there is dir.
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, ErrNoFolder)
	case err != nil:
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
	}
	x := exporting{written: make(map[string]bool), mkdir: e.making(folder), unremoved: e.removeLeftovers(folder)}
	recorded, err := x.record(e.StateDir, folder, e.Accounts)
	if err != nil {
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
	}
	for _, lines := range e.lines(folder, x.mkdir) {
		if err := x.lines(lines); err != nil {
			return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
		}
	}
	if err := x.summaries(ledger.Dir(e.StateDir), ledger.Dir(folder)); err != nil {
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
	}
	if err := writeManifest(folder, id, e.Now(), recorded); err != nil {
		return Result{}, fmt.Errorf("can't export into %s: %w", dir, err)
	}
	return x.result(folder), nil
}

// lines are the files of lines the state directory holds, each beside where
// the Mac's folder at folder holds them, made by mkdir.
func (e Exporter) lines(folder string, mkdir func(dir string) error) []dayLines {
	src, dst := linesIn(e.StateDir, e.Logger), linesIn(folder, e.Logger)
	lines := make([]dayLines, len(src))
	for i := range src {
		lines[i] = dayLines{src: src[i], dst: dst[i], mkdir: mkdir, looked: e.looked, wrap: e.wrap}
	}
	return lines
}

// linesIn are the files of each kind of line in root, a state directory or a
// Mac's folder, which lay them out alike, what can't be done with them
// logged to logger.
func linesIn(root string, logger *slog.Logger) []*dayfile.Files {
	return []*dayfile.Files{
		ledger.Files(ledger.Dir(root), logger),
		events.Files(ledger.Dir(root), logger),
		readings.Files(readings.Dir(root), logger),
	}
}

// exporting is what an export has done so far: whether it wrote a file of
// each day, by its date, the days' files it couldn't read, and the files
// writes cut short left that it couldn't remove; and mkdir, what makes the
// folders in the Mac's folder it writes into.
type exporting struct {
	written   map[string]bool
	unread    []DayError
	unremoved []Leftover
	mkdir     func(dir string) error
}

// lines exports each day lines' files in the state directory hold.
func (x *exporting) lines(lines dayLines) error {
	dates, err := lines.src.All()
	if err != nil {
		return fmt.Errorf("read the %s: %w", lines.src.Name, err)
	}
	for _, date := range dates {
		did, err := lines.export(date)
		if err := x.note("the "+lines.src.Name+" of "+date, date, did, err); err != nil {
			return err
		}
	}
	return nil
}

// summaries copies each day's summary the ledger in src holds to the ledger
// in dst, as copyFile copies it, making the folder as x.mkdir does.
func (x *exporting) summaries(src, dst string) error {
	dates, err := ledger.SummaryDates(src)
	if err != nil {
		return fmt.Errorf("read the request ledger's summaries: %w", err)
	}
	for _, date := range dates {
		did, err := copyFile(ledger.SummaryFile(src, date), ledger.SummaryFile(dst, date), x.mkdir)
		if err := x.note("the request ledger's summary of "+date, date, did, err); err != nil {
			return err
		}
	}
	return nil
}

// record writes the accounts record in the state directory at state into the
// Mac's folder at folder, each account configured labelled as configured
// says, so a Mac that doesn't configure it names it all the same, making the
// folder as x.mkdir does: where it differs from the one there, and with the
// state record's modification time. Where the state directory has none, it
// removes the folder's, so the folder, its export.json marked as exported
// with ids made, reads as one whose Mac has no account to join. It reports
// whether the folder holds the record, counts for no day, and fails where
// the record can't be read, as an import of the folder needs it to take in
// the accounts its days name.
func (x *exporting) record(state, folder string, configured config.Accounts) (bool, error) {
	src, dst := identity.NewFile(state), identity.NewFile(folder).Path()
	info, err := os.Stat(src.Path())
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, fmt.Errorf("remove the accounts record exported before: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("export the accounts record: %w", err)
	}
	record, err := src.Read()
	if err != nil {
		return false, fmt.Errorf("export the accounts record: %w", err)
	}
	data, err := labelled(record, configured).Encode()
	if err != nil {
		return false, fmt.Errorf("export the accounts record: %w", err)
	}
	if was, err := os.ReadFile(dst); err == nil && bytes.Equal(was, data) {
		return true, nil
	}
	err = writeFile(dst, info.ModTime(), x.mkdir, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("export the accounts record: %w", err)
	}
	return true, nil
}

// labelled is record with each account configured marked so, as Entry's
// Configured says, and labelled as configured labels it, where it does.
func labelled(record identity.Record, configured config.Accounts) identity.Record {
	out := identity.Record{Accounts: slices.Clone(record.Accounts)}
	for i, e := range out.Accounts {
		at := slices.IndexFunc(configured, func(a config.Account) bool { return strings.EqualFold(a.ID, e.ID) })
		if at < 0 {
			continue
		}
		out.Accounts[i].Configured = true
		if configured[at].Label != "" {
			out.Accounts[i].Label = configured[at].Label
		}
	}
	return out
}

// note notes how exporting what's given, of the day with the given date,
// went: what it came to, as did says, a file gone counting its day for
// nothing; or, where err is an unreadable, that it was passed over, and why.
// It returns any other error, saying what it was of.
func (x *exporting) note(what, date string, did outcome, err error) error {
	if u, ok := errors.AsType[unreadable](err); ok {
		x.unread = append(x.unread, DayError{What: what, Date: date, Err: u.error})
		return nil
	}
	if err != nil {
		return fmt.Errorf("export %s: %w", what, err)
	}
	if did != gone {
		x.written[date] = x.written[date] || did == wrote
	}
	return nil
}

// result is the result of the export into folder.
func (x *exporting) result(folder string) Result {
	r := Result{Folder: folder, Unread: x.unread, Unremoved: x.unremoved}
	for _, wroteAny := range x.written {
		if wroteAny {
			r.Written++
		} else {
			r.AsTheyWere++
		}
	}
	return r
}
