package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

const (
	// fileName is the accounts record's name in the state directory.
	fileName = "accounts.json"
	// lockName is the name of the file in the state directory a change to the
	// accounts record holds its lock on.
	lockName = "accounts.lock"
	// version is the version of the accounts record's format.
	version = 1
)

// ErrDamaged is what reading an accounts record this build can't read fails
// with, wrapped.
var ErrDamaged = errors.New("the accounts record is damaged")

// errNoDirectory is what a File of no state directory fails with, so it never
// reads or writes a record of the working directory's.
var errNoDirectory = errors.New("the accounts record has no state directory")

// File is the accounts record's file in a state directory.
type File struct {
	dir string
}

// NewFile returns the accounts record's file in the state directory given.
func NewFile(stateDir string) File {
	return File{dir: stateDir}
}

// Path is where the accounts record is.
func (f File) Path() string {
	return filepath.Join(f.dir, fileName)
}

// Read returns the accounts record: empty where there's no file. It fails
// where the file can't be read, and, with an error matching ErrDamaged, where
// it's no accounts record this build can read: either is left as it is.
func (f File) Read() (Record, error) {
	if f.dir == "" {
		return Record{}, fmt.Errorf("read the accounts record: %w", errNoDirectory)
	}
	data, err := os.ReadFile(f.Path())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Record{}, nil
	case err != nil:
		return Record{}, fmt.Errorf("read the accounts record: %w", err)
	}
	r, err := decode(data)
	if err != nil {
		return Record{}, fmt.Errorf("%w: %s: %w", ErrDamaged, f.Path(), err)
	}
	return r, nil
}

// Update changes the accounts record as change says, and writes it whole, in
// place of the file, that only its owner can read or write, making the state
// directory where it isn't there. It's read afresh and written under the
// record's lock, so no other Update, in this process or another, writes back
// what it read before this one's change. Nothing is written where the file
// can't be read or is damaged, as Read says, or where change fails, its error
// returned as it is, or leaves a record Read would find damaged.
//
// Decide inside change, by the record it's given, so what's decided still
// holds when it's written. change must never call Update on the record
// itself: a second lock in one process waits on the first for good. Where the
// config's lock is held too, it's taken first.
func (f File) Update(change func(*Record) error) error {
	unlock, err := f.lock()
	if err != nil {
		return err
	}
	defer unlock()
	r, err := f.Read()
	if err != nil {
		return err
	}
	if err := change(&r); err != nil {
		return err
	}
	return f.write(r)
}

// Change changes the accounts record as Update does, for a change of other
// files that has to go with the record's, as the config's does. change makes
// its change to the record it's given, then calls commit, at the point its
// own steps say, to write it, where it changed; commit is called for it
// where change returns without, and once it's written, changes made after
// aren't. The lock's held throughout. Where change fails once commit has
// written the record, the record is put back as it was, under the same
// lock, its file taken away where there was none, any error of that joined
// to change's: should that fail too, the record is left ahead, which
// whatever changes it next puts right. Nothing's written where change fails
// before commit, nor where the record's unchanged.
func (f File) Change(change func(r *Record, commit func() error) error) error {
	unlock, err := f.lock()
	if err != nil {
		return err
	}
	defer unlock()
	_, statErr := os.Lstat(f.Path())
	r, err := f.Read()
	if err != nil {
		return err
	}
	was, existed := slices.Clone(r.Accounts), statErr == nil
	written := false
	commit := func() error {
		if written || slices.Equal(was, r.Accounts) {
			return nil
		}
		if err := f.write(r); err != nil {
			return err
		}
		written = true
		return nil
	}
	if err := change(&r, commit); err != nil {
		if written {
			err = errors.Join(err, f.restore(was, existed))
		}
		return err
	}
	return commit()
}

// restore puts the record back as it was, holding accounts, where its file
// existed, else takes away the file written in its place.
func (f File) restore(accounts []Entry, existed bool) error {
	var err error
	if existed {
		err = f.write(Record{Accounts: accounts})
	} else {
		err = os.Remove(f.Path())
	}
	if err != nil {
		return fmt.Errorf("put the accounts record back: %w", err)
	}
	return nil
}

// write writes the record whole, in place of the file, that only its owner
// can read or write, failing where it would leave a record Read would find
// damaged.
func (f File) write(r Record) error {
	data, err := encode(r)
	if err != nil {
		return fmt.Errorf("write the accounts record: %w", err)
	}
	if err := atomicfile.Write(f.Path(), data, 0o600); err != nil {
		return fmt.Errorf("write the accounts record: %w", err)
	}
	return nil
}

// lock takes the accounts record's lock, an exclusive lock on lockName in
// the state directory, making both where they aren't there, waiting while
// another holds it, and returns what releases it. The file is never removed:
// the lock goes with the process that held it, however that ends.
func (f File) lock() (unlock func(), err error) {
	if f.dir == "" {
		return nil, fmt.Errorf("take the accounts record's lock: %w", errNoDirectory)
	}
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return nil, fmt.Errorf("take the accounts record's lock: %w", err)
	}
	l, err := os.OpenFile(filepath.Join(f.dir, lockName), os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("take the accounts record's lock: %w", err)
	}
	if err := flock(l); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("take the accounts record's lock: %w", err)
	}
	return func() { _ = l.Close() }, nil
}

// recordFile is the accounts record as its file holds it.
type recordFile struct {
	Version  int     `json:"version"`
	Accounts []Entry `json:"accounts"`
}

// decode returns the accounts record data holds, failing, saying why, where
// it's none this build can read.
func decode(data []byte) (Record, error) {
	var file recordFile
	if err := json.Unmarshal(data, &file); err != nil {
		return Record{}, err
	}
	if file.Version != version {
		return Record{}, fmt.Errorf("version %d, where %d is known", file.Version, version)
	}
	r := Record{Accounts: file.Accounts}
	if err := r.check(); err != nil {
		return Record{}, err
	}
	return r, nil
}

// encode returns the accounts record as its file holds it, its times in UTC
// to the second, failing, saying why, where decode would.
func encode(r Record) ([]byte, error) {
	stamped := Record{Accounts: make([]Entry, len(r.Accounts))}
	for i, e := range r.Accounts {
		e.Added, e.Removed = stamp(e.Added), stamp(e.Removed)
		stamped.Accounts[i] = e
	}
	if err := stamped.check(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(recordFile{Version: version, Accounts: stamped.Accounts}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// stamp is t as the accounts record keeps it: in UTC, to the second.
func stamp(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
}

// check fails, saying why, where the record is none switchboard keeps: an
// account that isn't one, as Entry.check says; two with one id, organization
// or id they had before switchboard made ids; or one whose id another had
// before. Ids are compared ignoring case. Accounts are named by their places,
// as what's damaged could be anything.
func (r Record) check() error {
	for i, e := range r.Accounts {
		if err := e.check(); err != nil {
			return fmt.Errorf("account #%d %w", i+1, err)
		}
	}
	return errors.Join(
		r.unique("id", func(e Entry) string { return strings.ToLower(e.ID) }),
		r.unique("organization", func(e Entry) string { return e.Organization }),
		r.unique("former id", func(e Entry) string { return strings.ToLower(e.Was) }),
		r.noIDFormer(),
	)
}

// check fails, saying why, where the account is none switchboard keeps:
// without an id switchboard makes, or a time it was added, or removed without
// a label. One removed before it was added is kept, as by a clock set ahead
// as it was added and put right before it was removed.
func (e Entry) check() error {
	switch {
	case !wellFormed(e.ID):
		return errors.New("has no id switchboard makes")
	case e.Added.IsZero():
		return errors.New("has no time it was added")
	case !e.Removed.IsZero() && e.Label == "":
		return errors.New("was removed without a label")
	}
	return nil
}

// unique fails where two accounts have the same of what key gives, "" being
// none, naming them by their places.
func (r Record) unique(what string, key func(Entry) string) error {
	seen := make(map[string]int, len(r.Accounts))
	for i, e := range r.Accounts {
		k := key(e)
		if k == "" {
			continue
		}
		if first, ok := seen[k]; ok {
			return fmt.Errorf("accounts #%d and #%d have the same %s", first+1, i+1, what)
		}
		seen[k] = i
	}
	return nil
}

// noIDFormer fails where an account's id is one another had before
// switchboard made ids, in any case, naming them by their places.
func (r Record) noIDFormer() error {
	had := make(map[string]int, len(r.Accounts))
	for i, e := range r.Accounts {
		if e.Was != "" {
			had[strings.ToLower(e.Was)] = i
		}
	}
	for i, e := range r.Accounts {
		if j, ok := had[strings.ToLower(e.ID)]; ok && j != i {
			return fmt.Errorf("account #%d's id is the one account #%d had before switchboard made ids", i+1, j+1)
		}
	}
	return nil
}
