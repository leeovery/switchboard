package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

const (
	// newFilePerm is a new config file's permissions: it holds no secret.
	newFilePerm fs.FileMode = 0o644
	// maxLinks is how many links are followed to where the config file is, as
	// a loop of them leads nowhere.
	maxLinks = 40
)

var (
	// ErrConfigured is what adding an account fails with, wrapped, when an
	// account has its id already.
	ErrConfigured = errors.New("already configured")
	// ErrNotConfigured is what acting on an account fails with, wrapped, when
	// no account has its id.
	ErrNotConfigured = errors.New("not configured")
)

// NewAccount is an account to add to the config file.
type NewAccount struct {
	ID string
	// Label is the name it's shown by: none shows it by its id.
	Label string
	// Primary makes it the primary, in place of any other.
	Primary bool
}

// Draft is the config file's text, edited as text: its comments and layout
// stay as they are, but for the lines an edit changes. Each edit is checked,
// the text read back, to make exactly the change meant and leave the config
// valid, before it's taken. Save writes the draft.
type Draft struct {
	path string
	// read is what the file held when it was read: nil when it wasn't there.
	read []byte
	doc  document
	// file is what the draft's text holds, and cfg the config it makes: nil
	// while it holds no account.
	file file
	cfg  *Config
}

// Edit reads the config file at path to edit it: a valid config, or one
// without accounts, to add them to. A file that isn't there reads as empty,
// and Save makes it.
func Edit(path string) (*Draft, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read config: %w", err)
	}
	f, meta, err := decodeFile(path, data)
	if err != nil {
		return nil, err
	}
	d := &Draft{path: path, read: data, doc: newDocument(string(data)), file: f}
	if len(f.Accounts) > 0 {
		if d.cfg, err = f.check(path, meta); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// Config returns the config the draft holds, as edited: nil while it holds no
// account.
func (d *Draft) Config() *Config {
	return d.cfg
}

// AddAccount adds an [[account]] table for the account: after the last
// account's, else before the first table, else at the end. As the primary,
// it takes primary off every other account. It fails, matching ErrConfigured,
// when an account has its id already.
func (d *Draft) AddAccount(a NewAccount) error {
	if err := CheckID(a.ID); err != nil {
		return err
	}
	if d.index(a.ID) >= 0 {
		return fmt.Errorf("account %q is %w", a.ID, ErrConfigured)
	}
	change := fmt.Sprintf("add account %q", a.ID)
	tables, err := d.accountTables(change)
	if err != nil {
		return err
	}
	doc, want := d.doc, d.file
	want.Accounts = slices.Clone(d.file.Accounts)
	if a.Primary {
		var marks []int
		for i := range want.Accounts {
			if want.Accounts[i].Primary {
				marks = append(marks, doc.linesOf(primaryKey, tables[i])...)
				want.Accounts[i].Primary = false
			}
		}
		doc = doc.without(marks)
	}
	doc = doc.insert(doc.newAccountAt(), a.lines())
	want.Accounts = append(want.Accounts, fileAccount{ID: a.ID, Label: a.Label, Primary: a.Primary})
	return d.take(doc, want, change)
}

// RemoveAccount removes the account's [[account]] table, and the comments
// directly above it. It fails, matching ErrNotConfigured, when no account has
// the id, and for the only account, as a config needs one.
func (d *Draft) RemoveAccount(id string) error {
	i := d.index(id)
	switch {
	case i < 0:
		return fmt.Errorf("account %q is %w", id, ErrNotConfigured)
	case len(d.file.Accounts) == 1:
		return fmt.Errorf("account %q is the only one, and a config needs one at least", id)
	}
	change := fmt.Sprintf("remove account %q", id)
	tables, err := d.accountTables(change)
	if err != nil {
		return err
	}
	want := d.file
	want.Accounts = slices.Delete(slices.Clone(d.file.Accounts), i, i+1)
	return d.take(d.doc.cut(tables[i]), want, change)
}

// MarksPrimary reports whether the draft marks an account primary, rather
// than leaving the first to be the primary for want of one.
func (d *Draft) MarksPrimary() bool {
	return slices.ContainsFunc(d.file.Accounts, func(a fileAccount) bool { return a.Primary })
}

// SetPrimary makes the account with the given id the primary: it marks it so
// below the last key of its [[account]] table, in place of any primary it
// set, and takes primary off every other account. It fails, matching
// ErrNotConfigured, when no account has the id.
func (d *Draft) SetPrimary(id string) error {
	i := d.index(id)
	switch {
	case i < 0:
		return fmt.Errorf("account %q is %w", id, ErrNotConfigured)
	case d.file.Accounts[i].Primary:
		return nil
	}
	change := fmt.Sprintf("make account %q the primary", id)
	tables, err := d.accountTables(change)
	if err != nil {
		return err
	}
	want := d.file
	want.Accounts = slices.Clone(d.file.Accounts)
	var marks []int
	for j := range want.Accounts {
		if want.Accounts[j].Primary || j == i {
			marks = append(marks, d.doc.linesOf(primaryKey, tables[j])...)
		}
		want.Accounts[j].Primary = j == i
	}
	doc := d.doc.without(marks)
	doc = doc.put(doc.keysEnd(doc.accounts()[i]), "primary = true")
	return d.take(doc, want, change)
}

// SetPrimeDay sets the day [prime] gives, which turns priming on: in place of
// the day it gives, else as the first key of its table, else in a [prime]
// table made at the end. It fails, as ParseDay does, for a day that isn't one.
func (d *Draft) SetPrimeDay(day string) error {
	if _, err := ParseDay(day); err != nil {
		return err
	}
	want := d.file
	want.Prime.Day = day
	return d.take(d.doc.withPrimeDay(day), want, fmt.Sprintf("set prime.day to %q", day))
}

// index returns where the account with the given id is among the draft's
// accounts, or -1 when none has it.
func (d *Draft) index(id string) int {
	return slices.IndexFunc(d.file.Accounts, func(a fileAccount) bool { return a.ID == id })
}

// accountTables returns the span of each account's [[account]] table, in the
// accounts' order, failing, as change can't be made, when the text doesn't
// give each account a table of its own, as when it lists them inline.
func (d *Draft) accountTables(change string) ([]span, error) {
	tables := d.doc.accounts()
	if len(tables) != len(d.file.Accounts) {
		return nil, d.cantMake(change)
	}
	return tables, nil
}

// take makes doc, edited to make change, the draft's text once it reads back
// as want, and makes a valid config; otherwise the draft stays as it was.
func (d *Draft) take(doc document, want file, change string) error {
	got, meta, err := decode(doc.text())
	if err != nil || !reflect.DeepEqual(got, want) {
		return d.cantMake(change)
	}
	cfg, err := got.check(d.path, meta)
	if err != nil {
		return err
	}
	d.doc, d.file, d.cfg = doc, got, cfg
	return nil
}

// cantMake is the error of a change editing the text can't make, or can't
// make alone, which the user can make by hand.
func (d *Draft) cantMake(change string) error {
	return fmt.Errorf("editing %s as text wouldn't make exactly the change meant, so it's left as it was: %s by hand", d.path, change)
}

// Save writes the draft to the config file, whole or not at all: through a
// link, to where it leads, rather than in its place, and making a file, and
// its directory, that isn't there. The file keeps its permissions. It fails,
// leaving the file as it is, when the file has changed since Edit read it.
func (d *Draft) Save() error {
	target, err := linkTarget(d.path)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	perm, err := d.unchanged(target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := atomicfile.Write(target, []byte(d.doc.text()), perm); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// unchanged returns the permissions of the file at target, where the config
// file leads, once it's shown to hold what it did when Edit read it: those of
// a new file when there's none.
func (d *Draft) unchanged(target string) (fs.FileMode, error) {
	info, err := os.Stat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return newFilePerm, d.expect(nil)
	}
	if err != nil {
		return 0, fmt.Errorf("read config: %w", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return 0, fmt.Errorf("read config: %w", err)
	}
	return info.Mode().Perm(), d.expect(data)
}

// expect fails unless data is what the config file held when Edit read it.
func (d *Draft) expect(data []byte) error {
	if !bytes.Equal(data, d.read) {
		return fmt.Errorf("%s changed while it was being edited, so it's left as it is: try again", d.path)
	}
	return nil
}

// linkTarget returns where writing to path leads: path itself, or where the
// links it names lead, which needn't be there yet.
func linkTarget(path string) (string, error) {
	named := path
	for range maxLinks {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return path, nil
		case err != nil:
			return "", err
		case info.Mode()&fs.ModeSymlink == 0:
			return path, nil
		}
		dest, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(dest) {
			// A relative link leads on from the directory it's in, as the system
			// finds it, following any link to that directory, where joining the
			// two as text would take a ".." in the link back through it.
			dir, err := filepath.EvalSymlinks(filepath.Dir(path))
			if err != nil {
				return "", err
			}
			dest = filepath.Join(dir, dest)
		}
		path = dest
	}
	return "", fmt.Errorf("%s: too many links", named)
}
