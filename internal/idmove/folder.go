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

// Why a Mac's folder can't be taken in, its accounts not to be told, each
// said of the folder, as Refusal and JoinFolder give them, wrapped.
var (
	// ErrStagedRecord is the folder's accounts record not reading.
	ErrStagedRecord = errors.New("its accounts record doesn't read")
	// ErrBeforeIDs is a folder another Mac exported before switchboard made
	// account ids, whose ids are that Mac's own names for its accounts.
	ErrBeforeIDs = errors.New("another Mac exported it before switchboard made account ids, so its accounts can't be told: export it again from that Mac once it runs this version of switchboard, or leave it out")
	// ErrUnsynced is a folder whose export.json says it holds the accounts
	// record, which it doesn't, as one a sync hasn't carried whole yet.
	ErrUnsynced = errors.New("its accounts record hasn't synced yet: import it again once it has")
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

// Here is the Mac a folder is imported into: Install gives its install's id,
// as export's InstallID does, asked only where a folder needs it; and
// Accounts are those it configures.
type Here struct {
	Install  func() (string, error)
	Accounts config.Accounts
}

// folderKind is how a Mac's folder's accounts are told, as classify tells.
type folderKind int

const (
	// carriesRecord is a folder holding its Mac's accounts record.
	carriesRecord folderKind = iota
	// recordless is a folder a switchboard that makes account ids exported
	// from a Mac with no accounts record: no account to join, none renamed.
	recordless
	// ownBeforeIDs is this Mac's folder, exported before switchboard made
	// account ids, its ids those this Mac's record keeps as former ids.
	ownBeforeIDs
)

// classify tells how the accounts of the Mac's folder at dir, as export
// writes one, are told, by its export.json, which it returns, and whether it
// holds an accounts record. It fails, with an error matching ErrUnsynced or
// ErrBeforeIDs, where they can't be, as those say; and where its export.json
// doesn't read, or this Mac's install id is needed and can't be had.
func classify(dir string, here Here) (folderKind, export.Manifest, error) {
	m, err := export.ReadManifest(dir)
	if err != nil {
		return 0, m, fmt.Errorf("read the export.json of a folder to import: %w", err)
	}
	_, err = os.Lstat(identity.NewFile(dir).Path())
	switch {
	case err == nil:
		return carriesRecord, m, nil
	case !errors.Is(err, fs.ErrNotExist):
		return 0, m, fmt.Errorf("%w: %w", ErrStagedRecord, err)
	case m.Accounts:
		return 0, m, ErrUnsynced
	case m.IDs:
		return recordless, m, nil
	}
	install, err := here.Install()
	switch {
	case err != nil:
		return 0, m, fmt.Errorf("read this Mac's install id: %w", err)
	case m.Install != install:
		return 0, m, ErrBeforeIDs
	}
	return ownBeforeIDs, m, nil
}

// Refusal is why an import into the Mac here would refuse the Mac's folder
// at dir, as export writes one, as JoinFolder refuses it: an error matching
// ErrUnsynced or ErrBeforeIDs, or nil where it wouldn't. It fails, too, as
// classify does.
func Refusal(dir string, here Here) error {
	_, _, err := classify(dir, here)
	return err
}

// JoinFolder joins the accounts the folder at dir names into record, the
// folder laid out as a state directory lays out its history, as import
// stages an exported one, and returns how its history is renamed onto
// record's ids, as RenameFolder renames it, as classify tells the folder:
//   - one carrying its Mac's accounts record has each id it names renamed by
//     that record's former ids, the ids its own Mac made, then onto this
//     Mac's, as identity's Join joins the record into record, taking its
//     labels where it isn't this Mac's own;
//   - one exported with ids by a Mac with no record joins nothing and has
//     nothing renamed;
//   - this Mac's own, exported before switchboard made ids, is renamed by
//     the former ids record keeps.
//
// It fails, record as it was, as classify does, saying which folder, by its
// install, of a folder that's refused, and with ErrStagedRecord wrapped where
// its record can't be read.
func JoinFolder(dir string, record *identity.Record, here Here) (Renames, error) {
	kind, m, err := classify(dir, here)
	switch {
	case errors.Is(err, ErrStagedRecord), errors.Is(err, ErrUnsynced), errors.Is(err, ErrBeforeIDs):
		return Renames{}, fmt.Errorf("the folder %s can't be taken in: %w", export.FolderName(m.Install), err)
	case err != nil:
		return Renames{}, err
	case kind == recordless:
		return Renames{}, nil
	case kind == ownBeforeIDs:
		return Renames{steps: []renames{formerIDs(*record)}}, nil
	}
	other, err := identity.NewFile(dir).Read()
	if err != nil {
		return Renames{}, fmt.Errorf("the folder %s can't be taken in: %w: %w", export.FolderName(m.Install), ErrStagedRecord, err)
	}
	// Where this Mac's install can't be had, no label is taken, as from its
	// own folder, rather than the import refused.
	install, installErr := here.Install()
	mapped, err := record.Join(other, identity.JoinAs{Configured: here.Accounts, From: m.Install, Labels: installErr == nil && m.Install != install})
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
// ledger's, a day's lines with its summary, as renameStagedDay renames each.
// It stops, failing, once ctx ends.
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
			if err := renameStagedDay(ctx, stagedDay{files: kind.files, names: kind.names, date: date}, rename); err != nil {
				return err
			}
		}
	}
	dates, err := ledgerDates(h.Ledger)
	if err != nil {
		return err
	}
	for _, date := range dates {
		day := stagedDay{
			files: h.Ledger.Files(), names: lineNames, date: date,
			lines:   func(line func([]byte) []byte) error { return h.Ledger.RenameDay(date, line, rename.Of) },
			summary: func() error { return h.Ledger.RenameSummary(date, rename.Of) },
		}
		if err := renameStagedDay(ctx, day, rename); err != nil {
			return err
		}
	}
	return nil
}

// stagedDay is a day staged to import: the files it's of, whose lines names
// says where they name accounts, and its date; and, of the request ledger's,
// what renames its lines with its summary, and its summary alone.
type stagedDay struct {
	files   *dayfile.Files
	names   names
	date    string
	lines   func(line func([]byte) []byte) error
	summary func() error
}

// renameStagedDay renames the accounts of the staged day as rename has them,
// where its compressed file, or of the request ledger its summary, names one
// renamed: its file first written anew of what reads where it reads short,
// as one cut short as it synced, which is warned of; then rewritten, its
// lines with its summary as the day's lines does, else as files' Rewrite
// rewrites them. A day whose file reads none of its lines, as one whose
// start is damaged, is left as it is, warned of, for the import to pass
// over as any it can't read, but for the request ledger's summary, renamed
// alone. It stops, failing, once ctx ends.
func renameStagedDay(ctx context.Context, day stagedDay, rename Renames) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	files := day.files
	path := files.CompressedPath(day.date)
	scan, err := scanStaged(path, files.LineMax, day.names, rename)
	if err != nil {
		return err
	}
	if day.summary != nil && !scan.renamed {
		scan.renamed, err = summaryRenamed(files.Dir, day.date, rename)
		if err != nil {
			return err
		}
	}
	switch {
	case !scan.renamed:
		return nil
	case scan.short != nil && !scan.read:
		logger.Warn(files.Name+" staged to import doesn't read, so it's left as it is", "file", path, "error", scan.short)
		if day.summary != nil {
			return day.summary()
		}
		return nil
	case scan.short != nil:
		logger.Warn(files.Name+" staged to import read short; its lines up to the damage are taken in", "file", path, "error", scan.short)
		if err := healStaged(path, files.LineMax); err != nil {
			return fmt.Errorf("rename the %s of %s staged to import: %w", files.Name, day.date, err)
		}
	}
	lines := renamingLines(day.names, rename.Of)
	if day.lines != nil {
		return day.lines(lines)
	}
	_, err = files.Rewrite(day.date, lines)
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
