package idmove

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/linescan"
)

// lineEnding ends each line of a staged file written anew.
var lineEnding = []byte{'\n'}

// What joining a folder staged to import fails with, wrapped, where it can't
// tell which of this Mac's accounts the folder's are.
var (
	// ErrStagedRecord is the folder's accounts record not reading.
	ErrStagedRecord = errors.New("the accounts record of a folder staged to import doesn't read")
	// ErrBeforeIDs is a folder another Mac exported before switchboard made
	// account ids, whose ids are that Mac's own names for its accounts.
	ErrBeforeIDs = errors.New("a folder was exported from another Mac before switchboard made account ids: export it again from that Mac once it runs this version of switchboard, then import that")
	// ErrUnsynced is a folder whose export.json says it holds the accounts
	// record, which it doesn't, as one a sync hasn't carried whole yet.
	ErrUnsynced = errors.New("a folder's accounts record hasn't synced yet: import it again once it has")
)

// Renames are how a folder staged to import has the accounts it names
// renamed onto the ids of this Mac's accounts record, as JoinFolder works
// them out: each a step after the one before.
type Renames struct {
	steps []renames
}

// Of renames id, as each step has it in turn.
func (r Renames) Of(id string) string {
	for _, step := range r.steps {
		id = step.of(id)
	}
	return id
}

// none reports whether nothing is renamed.
func (r Renames) none() bool {
	for _, step := range r.steps {
		if !step.none() {
			return false
		}
	}
	return true
}

// Here is the Mac a folder staged to import is joined to: its install, as
// export's InstallID names it, the accounts it configures, and where a random
// id is drawn from, crypto/rand's where it's nil.
type Here struct {
	Install  string
	Accounts config.Accounts
	Random   io.Reader
}

// JoinFolder joins the accounts the folder at dir names into record, the
// folder laid out as a state directory lays out its history, as import
// stages an exported one, and returns how its history is renamed onto
// record's ids, as RenameFolder renames it:
//   - a folder holding an accounts record, as export writes one from this
//     build on, has each id it names renamed by that record's former ids, the
//     ids its own Mac made, then onto this Mac's, as identity's Join joins the
//     record into record;
//   - one holding none, exported before switchboard made ids, is taken only
//     where it's this Mac's own, as its export.json says: its ids are then
//     those record knows as former ids, renamed by them.
//
// It fails, record as it was, with ErrStagedRecord wrapped where the folder's
// record can't be read; ErrUnsynced where its export.json says it holds one
// it doesn't; ErrBeforeIDs where it holds none and is another Mac's; and
// where its export.json can't be read, or a random id drawn.
func JoinFolder(dir string, record *identity.Record, here Here) (Renames, error) {
	staged := identity.NewFile(dir)
	_, err := os.Lstat(staged.Path())
	switch {
	case err == nil:
		return joinRecord(staged, record, here)
	case !errors.Is(err, fs.ErrNotExist):
		return Renames{}, fmt.Errorf("%w: %w", ErrStagedRecord, err)
	}
	m, err := export.ReadManifest(dir)
	switch {
	case err != nil:
		return Renames{}, fmt.Errorf("read the export.json of a folder staged to import: %w", err)
	case m.Accounts:
		return Renames{}, ErrUnsynced
	case m.Install != here.Install:
		return Renames{}, ErrBeforeIDs
	}
	return Renames{steps: []renames{formerIDs(*record)}}, nil
}

// joinRecord joins the accounts record staged into record, as identity's
// Join joins it, and returns how the folder it's in is renamed: by its
// former ids, then as Join maps its ids.
func joinRecord(staged identity.File, record *identity.Record, here Here) (Renames, error) {
	other, err := staged.Read()
	if err != nil {
		return Renames{}, fmt.Errorf("%w: %w", ErrStagedRecord, err)
	}
	mapped, err := record.Join(other, here.Accounts, (&Mover{Random: here.Random}).random())
	if err != nil {
		return Renames{}, err
	}
	return Renames{steps: []renames{formerIDs(other), renames(mapped)}}, nil
}

// formerIDs are the ids record's accounts had before switchboard made ids,
// each renamed to the id it made.
func formerIDs(record identity.Record) renames {
	r := make(renames)
	for _, e := range record.Accounts {
		if e.Was != "" {
			r[strings.ToLower(e.Was)] = e.ID
		}
	}
	return r
}

// RenameFolder writes the history in dir, as JoinFolder takes the folder, anew,
// each account it names renamed as rename has it, as the move renames them:
// the readings history's days, then the router's events', then the request
// ledger's, a day's lines with its summary. A day none of whose files names
// an account renamed is left as it is; and one whose file reads short, as
// one cut short as it synced, is first written anew of its lines up to the
// damage, which is warned of, as import reads one. It stops, failing, once
// ctx ends.
func RenameFolder(ctx context.Context, dir string, rename Renames) error {
	if rename.none() {
		return nil
	}
	h := (&Mover{StateDir: dir}).history()
	for _, kind := range []struct {
		files *dayfile.Files
		names names
	}{{h.Readings, readingNames}, {h.Events, eventNames}} {
		dates, err := kind.files.All()
		if err != nil {
			return fmt.Errorf("rename the %s staged to import: %w", kind.files.Name, err)
		}
		for _, date := range dates {
			if err := renameStagedDay(ctx, kind.files, kind.names, date, rename, nil); err != nil {
				return err
			}
		}
	}
	dates, err := ledgerDates(h.Ledger)
	if err != nil {
		return err
	}
	for _, date := range dates {
		day := func(line func([]byte) []byte) error { return h.Ledger.RenameDay(date, line, rename.Of) }
		if err := renameStagedDay(ctx, h.Ledger.Files(), lineNames, date, rename, day); err != nil {
			return err
		}
	}
	return nil
}

// renameStagedDay renames the accounts of the staged day with the given date
// of files, whose lines n says where they name accounts, as rename has them:
// where its compressed file, or of the request ledger its summary, names one
// renamed, its file first written anew of what reads where it reads short,
// then rewritten as day rewrites it, its lines as a whole, or, where day is
// nil, as files' Rewrite rewrites them. A day whose file reads none of its
// lines, as one whose start is damaged, is left as it is, warned of, for the
// import to pass over as it passes over any it can't read. It stops,
// failing, once ctx ends.
func renameStagedDay(ctx context.Context, files *dayfile.Files, n names, date string, rename Renames, day func(line func([]byte) []byte) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := files.CompressedPath(date)
	scan, err := scanStaged(path, files.LineMax, n, rename)
	if err != nil {
		return err
	}
	if day != nil && !scan.renamed {
		scan.renamed, err = summaryRenamed(files.Dir, date, rename)
		if err != nil {
			return err
		}
	}
	switch {
	case !scan.renamed:
		return nil
	case scan.short != nil && !scan.read:
		logger.Warn(files.Name+" staged to import doesn't read, so it's left as it is", "file", path, "error", scan.short)
		return nil
	case scan.short != nil:
		logger.Warn(files.Name+" staged to import read short; its lines up to the damage are taken in", "file", path, "error", scan.short)
		if err := healStaged(path, files.LineMax); err != nil {
			return fmt.Errorf("rename the %s of %s staged to import: %w", files.Name, date, err)
		}
	}
	lines := renamingLines(n, rename.Of)
	if day != nil {
		return day(lines)
	}
	_, err = files.Rewrite(date, lines)
	return err
}

// stagedScan is what reading a staged compressed file found: whether a line
// names an account renamed, whether any line read, and why it read short,
// where it did.
type stagedScan struct {
	renamed, read bool
	short         error
}

// scanStaged reads the staged compressed file at path, a line at a time, as
// import reads one, finding what stagedScan says of it, n saying where a
// line names accounts. A file that isn't there names none; one that can't
// be opened fails.
func scanStaged(path string, most int, n names, rename Renames) (stagedScan, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return stagedScan{}, nil
	}
	if err != nil {
		return stagedScan{}, err
	}
	defer func() { _ = file.Close() }()
	var scan stagedScan
	scan.short = eachStaged(file, most, func(line []byte) {
		scan.read = true
		n.visit(line, func(id string) { scan.renamed = scan.renamed || rename.Of(id) != id })
	})
	return scan, nil
}

// summaryRenamed reports whether the request ledger's summary of the day
// with the given date in dir names an account rename renames: none where
// there's no summary.
func summaryRenamed(dir, date string, rename Renames) (bool, error) {
	data, err := os.ReadFile(ledger.SummaryFile(dir, date))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	renamed := false
	summaryNames.visit(data, func(id string) { renamed = renamed || rename.Of(id) != id })
	return renamed, nil
}

// healStaged writes the staged compressed file at path anew, as one gzip
// member, of the lines of it that read as JSON, up to where it reads short,
// with its modification time.
func healStaged(path string, most int) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	return atomicfile.WriteModified(path, 0o600, info.ModTime(), func(w io.Writer) error {
		member := gzip.NewWriter(w)
		var written error
		// The read ends short at the damage, as the caller knows: what reads
		// before it is the file written.
		_ = eachStaged(src, most, func(line []byte) {
			if written == nil {
				_, written = member.Write(line)
			}
			if written == nil {
				_, written = member.Write(lineEnding)
			}
		})
		return errors.Join(written, member.Close())
	})
}

// eachStaged hands take each line of what src gives through gzip that reads
// as JSON, its members as one stream, passing over those longer than most,
// and returns why the read ended short, where it did.
func eachStaged(src io.Reader, most int, take func(line []byte)) error {
	members, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	lines := linescan.New(members, most)
	for lines.Scan() {
		if line := lines.Bytes(); json.Valid(line) {
			take(line)
		}
	}
	return lines.Err()
}
