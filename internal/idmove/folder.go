package idmove

import (
	"bytes"
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
	// ErrConfigUnsynced is a folder whose export.json says it holds a copy
	// of its Mac's config, which it doesn't, as one a sync hasn't carried
	// whole yet: its pools can't be told without it.
	ErrConfigUnsynced = errors.New("its config hasn't synced yet: import it again once it has")
	// ErrStagedConfig is the folder's copy of its Mac's config not reading.
	ErrStagedConfig = errors.New("its config doesn't read")
)

// ErrNoFirstPool is what joining a folder exported before pools fails with
// where this Mac's accounts record notes no first pool, to give its lines.
var ErrNoFirstPool = errors.New("this Mac's accounts record notes no first pool to give the history of a folder exported before pools")

// Renames are how a folder staged to import has the accounts it names
// renamed onto the ids of this Mac's accounts record, as JoinFolder works
// them out, each a step after the one before; its Mac's pools renamed onto
// the pools here holding their accounts, as identity's Join takes them in;
// of a folder exported before pools, the pool its lines are given, this
// Mac's first; and its Mac's pools kept apart.
type Renames struct {
	steps    []renames
	pools    renames
	unpooled string
	apart    []identity.Apart
}

// Apart are the pools the folder's Mac configures that its join kept apart,
// as identity's Joined says.
func (r Renames) Apart() []identity.Apart {
	return r.apart
}

// Of renames id, as each step has it in turn.
func (r Renames) Of(id string) string {
	for _, step := range r.steps {
		id = step.of(id)
	}
	return id
}

// renaming is how the folder's history is renamed, as the ledger and
// rewritingLines rename it: its accounts renamed only where a step renames
// one, and its pools only where one's renamed.
func (r Renames) renaming() ledger.Renaming {
	rename := ledger.Renaming{Unpooled: r.unpooled}
	if !r.namesNone() {
		rename.Account = r.Of
	}
	if !r.pools.none() {
		rename.Pool = r.pools.of
	}
	return rename
}

// none reports whether nothing is renamed, nor any line given a pool.
func (r Renames) none() bool {
	return r.namesNone() && r.pools.none() && r.unpooled == ""
}

// namesNone reports whether no step renames an account.
func (r Renames) namesNone() bool {
	for _, step := range r.steps {
		if !step.none() {
			return false
		}
	}
	return true
}

// Here is the Mac a folder is imported into: Install gives its install's id,
// as export's InstallID does, asked only where a folder needs it; and
// Accounts and Pools are those it configures.
type Here struct {
	Install  func() (string, error)
	Accounts config.Accounts
	Pools    config.Pools

	// mapped are the pools the folders imported beside the one joined take
	// in as pools here, as JoinFolders finds them.
	mapped renames
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

// told is a Mac's folder as classify tells it: its kind, its export.json,
// and, where it carries one, its Mac's accounts record; and the accounts its
// Mac configures, each in its pool, as its copy of its Mac's config gives
// them, known where theirsKnown says it holds the copy.
type told struct {
	kind        folderKind
	manifest    export.Manifest
	record      identity.Record
	theirs      config.Accounts
	theirsKnown bool
}

// classify tells how the accounts of the Mac's folder at dir, as export
// writes one, are told, by its export.json and the accounts record it holds,
// if any, a link to none holding none, and the copy of its Mac's config it
// holds, as theirConfig reads it. It fails, with an error matching
// ErrStagedRecord, ErrUnsynced, ErrStagedConfig, ErrConfigUnsynced or
// ErrBeforeIDs, where they can't be, as those say; and where its export.json
// doesn't read, or this Mac's install id is needed and can't be had.
func classify(dir string, here Here) (told, error) {
	m, err := export.ReadManifest(dir)
	if err != nil {
		return told{}, fmt.Errorf("read the export.json of a folder to import: %w", err)
	}
	f := told{manifest: m}
	if f.theirs, f.theirsKnown, err = theirConfig(dir, m); err != nil {
		return f, err
	}
	data, err := os.ReadFile(identity.NewFile(dir).Path())
	switch {
	case err == nil:
		f.kind = carriesRecord
		if f.record, err = identity.Decode(data); err != nil {
			return f, fmt.Errorf("%w: %w", ErrStagedRecord, err)
		}
		return f, nil
	case !errors.Is(err, fs.ErrNotExist):
		return f, fmt.Errorf("%w: %w", ErrStagedRecord, err)
	case m.Accounts:
		return f, ErrUnsynced
	case m.IDs:
		f.kind = recordless
		return f, nil
	}
	install, err := here.Install()
	switch {
	case err != nil:
		return f, fmt.Errorf("read this Mac's install id: %w", err)
	case m.Install != install:
		return f, ErrBeforeIDs
	}
	f.kind = ownBeforeIDs
	return f, nil
}

// theirConfig returns the accounts the copy of its Mac's config in the Mac's
// folder at dir configures, each in its pool, as config.Members reads them,
// and reports whether the folder holds the copy, as configCopy reads it,
// failing as it does, and with ErrStagedConfig where the copy doesn't read.
func theirConfig(dir string, m export.Manifest) (config.Accounts, bool, error) {
	data, err := configCopy(dir, m)
	if err != nil || data == nil {
		return nil, false, err
	}
	theirs, err := config.Members(data)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrStagedConfig, err)
	}
	return theirs, true, nil
}

// ConfigCopy returns the copy of its Mac's config the Mac's folder at dir
// holds, as export writes one, as configCopy reads it, by the folder's
// export.json, failing where that doesn't read.
func ConfigCopy(dir string) ([]byte, error) {
	m, err := export.ReadManifest(dir)
	if err != nil {
		return nil, fmt.Errorf("read the export.json of a folder to import: %w", err)
	}
	return configCopy(dir, m)
}

// configCopy returns the copy of its Mac's config the Mac's folder at dir
// holds: nil where it holds none, as m, its export.json, says it doesn't. It
// fails with ErrConfigUnsynced where m says it holds one it doesn't, as a
// sync not done leaves it, and with ErrStagedConfig where it can't be read.
func configCopy(dir string, m export.Manifest) ([]byte, error) {
	data, err := os.ReadFile(export.ConfigFile(dir))
	switch {
	case errors.Is(err, fs.ErrNotExist) && m.Config:
		return nil, ErrConfigUnsynced
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrStagedConfig, err)
	}
	return data, nil
}

// Refusal is why an import into the Mac here would refuse the Mac's folder
// at dir, as export writes one, as JoinFolder refuses it: an error matching
// one of those Refused reports, or nil where it wouldn't. It fails, too, as
// classify does.
func Refusal(dir string, here Here) error {
	_, err := classify(dir, here)
	return err
}

// Refused reports whether err, as Refusal or JoinFolder gives it, is a
// folder refused, as ErrStagedRecord, ErrUnsynced, ErrStagedConfig,
// ErrConfigUnsynced and ErrBeforeIDs are, rather than one that couldn't be
// told.
func Refused(err error) bool {
	return slices.ContainsFunc([]error{ErrStagedRecord, ErrUnsynced, ErrStagedConfig, ErrConfigUnsynced, ErrBeforeIDs}, func(refused error) bool {
		return errors.Is(err, refused)
	})
}

// JoinFolder joins the accounts the folder at dir names into record, the
// folder laid out as a state directory lays out its history, as import
// stages an exported one, and returns how its history is renamed onto
// record's ids, as RenameFolder renames it, as classify tells the folder:
//   - one carrying its Mac's accounts record has each id it names renamed by
//     that record's former ids, the ids its own Mac made, then onto this
//     Mac's, as identity's Join joins the record into record, as this Mac's
//     own where it is, taking its labels where it's another Mac's;
//   - one exported with ids by a Mac with no record joins nothing and has
//     nothing renamed;
//   - this Mac's own, exported before switchboard made ids, is renamed by
//     the former ids record keeps.
//
// Its record's pools join record as identity's Join joins them, by the copy
// of its Mac's config it holds, where it holds one: each of its Mac's pools
// renamed onto the pool here holding its accounts, where Join finds one, the
// rest kept apart, as Renames' Apart says. Its lines, the events of the
// kinds that name their pool, and its summaries, where they name no pool,
// are given its Mac's first pool, as they're read there: of one exported
// before pools, as export.Manifest's BeforePools tells, this Mac's, as
// FirstOf has it of here's pools and the one record notes, as no other was
// noted then; of one exported since, the one its record notes, as it's
// taken in, where it notes one.
//
// It fails, record as it was, as classify does, saying which folder, by its
// install, of a folder that's refused; and with ErrNoFirstPool, of a folder
// exported before pools, where record notes no first pool.
func JoinFolder(dir string, record *identity.Record, here Here) (Renames, error) {
	f, err := classify(dir, here)
	switch {
	case Refused(err):
		return Renames{}, fmt.Errorf("the folder %s can't be taken in: %w", export.FolderName(f.manifest.Install), err)
	case err != nil:
		return Renames{}, err
	}
	var unpooled string
	if f.manifest.BeforePools() {
		first, ok := record.FirstPool()
		if !ok {
			return Renames{}, ErrNoFirstPool
		}
		unpooled = FirstOf(here.Pools, first.ID)
	}
	rename, err := joined(f, record, here)
	if err != nil {
		return Renames{}, err
	}
	if first, ok := f.record.FirstPool(); ok && unpooled == "" {
		unpooled = rename.pools.of(first.ID)
	}
	rename.unpooled = unpooled
	return rename, nil
}

// JoinFolders joins the accounts each of the folders at dirs names into
// record, in their order, as JoinFolder joins one, and returns how each is
// renamed, in the same order. A pool's id being random, it's one pool on
// every Mac: so a pool one folder's join takes in as a pool here is taken in
// as that one by each other folder's whose own Mac can't place it, as it
// doesn't configure it, as of history relayed through a Mac that imported
// it, or whose folder holds no copy of its config, unless two folders take
// it in as different pools here. It fails, record as it was, as JoinFolder
// does.
func JoinFolders(dirs []string, record *identity.Record, here Here) ([]Renames, error) {
	// A first round of joins, into a copy, finds what each folder takes its
	// pools in as: Join never changes the record it's given but to replace
	// it whole, so a copy of the value shares nothing it writes.
	scratch := *record
	mapped, clashed := make(renames), make(map[string]bool)
	for _, dir := range dirs {
		rename, err := JoinFolder(dir, &scratch, here)
		if err != nil {
			return nil, err
		}
		for id, to := range rename.pools {
			if was, ok := mapped[id]; ok && !config.SamePool(was, to) {
				clashed[id] = true
			}
			mapped[id] = to
		}
	}
	for id := range clashed {
		delete(mapped, id)
	}
	here.mapped = mapped
	joined := *record
	all := make([]Renames, len(dirs))
	for i, dir := range dirs {
		rename, err := JoinFolder(dir, &joined, here)
		if err != nil {
			return nil, err
		}
		all[i] = rename
	}
	*record = joined
	return all, nil
}

// joined joins the accounts and pools the folder f names into record, as
// JoinFolder says, by its kind, and returns how its accounts are renamed.
func joined(f told, record *identity.Record, here Here) (Renames, error) {
	switch f.kind {
	case recordless:
		return Renames{}, nil
	case ownBeforeIDs:
		return Renames{steps: []renames{formerIDs(*record)}}, nil
	}
	// Where this Mac's install can't be had, the folder is joined as another
	// Mac's, but no label is taken, as from its own, rather than the import
	// refused.
	install, installErr := here.Install()
	own := installErr == nil && f.manifest.Install == install
	as := identity.JoinAs{
		Configured: here.Accounts, Pools: here.Pools, From: f.manifest.Install, Own: own, Labels: installErr == nil && !own,
		Theirs: f.theirs, TheirsKnown: f.theirsKnown, Mapped: here.mapped,
	}
	j, err := record.Join(f.record, as)
	if err != nil {
		return Renames{}, err
	}
	return Renames{steps: []renames{formerIDs(f.record), renames(j.Accounts)}, pools: renames(j.Pools), apart: j.Apart}, nil
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
// as rename has it, as the moves rewrite it: each account it names renamed,
// and, of a folder exported before pools, each line given this Mac's first
// pool: the readings history's days, then the router's events', then the
// request ledger's, a day's lines with its summary, as renameStagedDay
// renames each. It stops, failing, once ctx ends.
func RenameFolder(ctx context.Context, dir string, rename Renames) error {
	if rename.none() {
		return nil
	}
	h, renaming := historyIn(dir, time.Now), rename.renaming()
	for _, kind := range []struct {
		files *dayfile.Files
		names names
	}{{h.Readings, readingNames}, {h.Events, eventNames}} {
		if !kind.names.rewrittenBy(renaming) {
			continue
		}
		dates, err := kind.files.All()
		if err != nil {
			return fmt.Errorf("rename the %s staged to import: %w", kind.files.Name, err)
		}
		for _, date := range dates {
			if err := renameStagedDay(ctx, stagedDay{files: kind.files, names: kind.names, date: date}, renaming); err != nil {
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
			lines:   func(line func([]byte) []byte) error { return h.Ledger.RenameDay(date, line, renaming) },
			summary: func() error { return h.Ledger.RenameSummary(date, renaming) },
		}
		if err := renameStagedDay(ctx, day, renaming); err != nil {
			return err
		}
	}
	return nil
}

// stagedDay is a day staged to import: the files it's of, whose lines names
// says where they name accounts and give their pool, and its date; and, of
// the request ledger's, what renames its lines with its summary, and its
// summary alone.
type stagedDay struct {
	files   *dayfile.Files
	names   names
	date    string
	lines   func(line func([]byte) []byte) error
	summary func() error
}

// renameStagedDay renames the staged day as rename has it, where a line of
// its compressed file, or of the request ledger its summary, is changed by
// it: its file first written anew of what reads where it reads short, as one
// cut short as it synced, which is warned of; then rewritten, its lines with
// its summary as the day's lines does, else as files' Rewrite rewrites them.
// A day whose file reads none of its lines, as one whose start is damaged,
// is left as it is, warned of, for the import to pass over as any it can't
// read, but for the request ledger's summary, renamed alone. It stops,
// failing, once ctx ends.
func renameStagedDay(ctx context.Context, day stagedDay, rename ledger.Renaming) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	files := day.files
	path := files.CompressedPath(day.date)
	lines := rewritingLines(day.names, rename)
	scan, err := scanStaged(path, files.LineMax, lines)
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
	if day.lines != nil {
		return day.lines(lines)
	}
	_, err = files.Rewrite(day.date, lines)
	return err
}

// stagedScan is what reading a staged compressed file found: whether a line
// is renamed, whether any line read, and why it read short, where it did.
type stagedScan struct {
	renamed, read bool
	short         error
}

// scanStaged reads the staged compressed file at path, a line at a time, as
// import reads one, finding what stagedScan says of it, a line renamed where
// line changes it. A file that isn't there names none; one that can't be
// opened fails.
func scanStaged(path string, most int, line func([]byte) []byte) (stagedScan, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return stagedScan{}, nil
	}
	if err != nil {
		return stagedScan{}, err
	}
	defer func() { _ = file.Close() }()
	var scan stagedScan
	scan.short = eachStaged(file, most, func(read []byte) {
		scan.read = true
		scan.renamed = scan.renamed || !bytes.Equal(line(read), read)
	})
	return scan, nil
}

// summaryRenamed reports whether the request ledger's summary of the day
// with the given date in dir is renamed as rename has it: it names an
// account renamed, or a pool, or it has work in no pool to give one, as any
// summary may, so is renamed whenever rename gives one; none where there's
// no summary.
func summaryRenamed(dir, date string, rename ledger.Renaming) (bool, error) {
	data, err := os.ReadFile(ledger.SummaryFile(dir, date))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	renamed := rename.Unpooled != ""
	for _, named := range []struct {
		names  names
		rename func(id string) string
	}{{summaryNames, rename.Account}, {names{values: summaryNames.pools}, rename.Pool}} {
		if named.rename != nil {
			named.names.visit(data, func(id string, _ bool) { renamed = renamed || named.rename(id) != id })
		}
	}
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
