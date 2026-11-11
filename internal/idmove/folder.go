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
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/linescan"
)

// lineEnding ends each line of a staged file written anew.
var lineEnding = []byte{'\n'}

// ErrStagedRecord is what joining a folder whose accounts record doesn't
// read fails with, wrapped.
var ErrStagedRecord = errors.New("the accounts record of a folder staged to import doesn't read")

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

// JoinFolder joins the accounts the folder at dir names into record, the
// folder laid out as a state directory lays out its history, as import
// stages an exported one, and returns how its history is renamed onto
// record's ids, as RenameFolder renames it:
//   - a folder holding an accounts record, as export writes one from this
//     build on, has it joined, as identity's Join joins one, each id the
//     folder names renamed by the record's own former ids, then onto
//     record's id for the account where Join maps it;
//   - one holding none was exported before switchboard made ids, so every id
//     it names is one an account had before: each record knows neither as
//     an id nor as a former id goes into it as removed, as the move puts an
//     account removed before ids were made, under a random id drawn from
//     random, crypto/rand's where it's nil, its old id its label.
//
// Then every id is renamed by record's former ids. It fails, record as it
// was, with ErrStagedRecord wrapped where the folder's record can't be read,
// identity.ErrConflict where it contradicts record, and where the folder's
// history can't be listed.
func JoinFolder(dir string, record *identity.Record, random io.Reader, now time.Time) (Renames, error) {
	staged := identity.NewFile(dir)
	if _, err := os.Lstat(staged.Path()); errors.Is(err, fs.ErrNotExist) {
		added := identity.Record{Accounts: slices.Clone(record.Accounts)}
		if err := addFormer(dir, &added, random, now); err != nil {
			return Renames{}, err
		}
		*record = added
		return Renames{steps: []renames{formerIDs(added)}}, nil
	}
	other, err := staged.Read()
	if err != nil {
		return Renames{}, fmt.Errorf("%w: %w", ErrStagedRecord, err)
	}
	mapped, err := record.Join(other)
	if err != nil {
		return Renames{}, err
	}
	return Renames{steps: []renames{formerIDs(other), renames(mapped), formerIDs(*record)}}, nil
}

// addFormer adds to record each id the history in dir names that it knows
// neither as an id nor as a former id, as an account removed before ids were
// made, as the move adds one, by the days the history tells of it on.
func addFormer(dir string, record *identity.Record, random io.Reader, now time.Time) error {
	m := &Mover{StateDir: dir, Random: random}
	seen, err := m.historyAccounts()
	if err != nil {
		return fmt.Errorf("read the accounts a folder staged to import names: %w", err)
	}
	d := &deciding{record: record, seen: seen, now: now, random: m.random(), decided: make(map[string]bool)}
	for _, was := range d.removedBefore() {
		if err := d.removed(was); err != nil {
			return err
		}
	}
	return nil
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
// nil, as files' Rewrite rewrites them. It stops, failing, once ctx ends.
func renameStagedDay(ctx context.Context, files *dayfile.Files, n names, date string, rename Renames, day func(line func([]byte) []byte) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := files.CompressedPath(date)
	renamed, short, err := scanStaged(path, files.LineMax, n, rename)
	if err != nil {
		return err
	}
	if day != nil && !renamed {
		renamed, err = summaryRenamed(files.Dir, date, rename)
		if err != nil {
			return err
		}
	}
	if !renamed {
		return nil
	}
	if short != nil {
		logger.Warn(files.Name+" staged to import read short; its lines up to the damage are taken in", "file", path, "error", short)
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

// scanStaged reads the staged compressed file at path, a line at a time, as
// import reads one, and reports whether a line names an account rename
// renames, as n says where, and why it read short, where it did. A file
// that isn't there names none; one that can't be opened fails.
func scanStaged(path string, most int, n names, rename Renames) (renamed bool, short, err error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = file.Close() }()
	short = eachStaged(file, most, func(line []byte) {
		n.visit(line, func(id string) { renamed = renamed || rename.Of(id) != id })
	})
	return renamed, short, nil
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
