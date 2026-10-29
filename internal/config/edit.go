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
	"strings"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// newFilePerm is a new config file's permissions: it holds no secret.
const newFilePerm fs.FileMode = 0o644

var (
	// ErrConfigured is what adding an account fails with, wrapped, when an
	// account has its id already.
	ErrConfigured = errors.New("already configured")
	// ErrNotConfigured is what acting on an account fails with, wrapped, when
	// no account has its id.
	ErrNotConfigured = errors.New("not configured")
	// ErrOnlyAccount is what removing an account fails with, wrapped, when
	// it's the only one.
	ErrOnlyAccount = errors.New("a config needs one at least")
)

// NotConfigured is the error of acting on the account with the given id,
// which no account has: it matches ErrNotConfigured, and hides anything in
// the id that looks like a token, as a token pasted where an id goes would
// be.
func NotConfigured(id string) error {
	return fmt.Errorf("%s is %w", accountNamed(id), ErrNotConfigured)
}

// NewAccount is an account to add to the config file.
type NewAccount struct {
	ID string
	// Label is the name it's shown by: none shows it by its id.
	Label string
	// Plan is the plan its subscription is on, one of Plans: "" leaves it
	// unknown.
	Plan string
	// Primary makes it the primary, in place of any other.
	Primary bool
}

// taken returns the account with its label as labelOf takes it, failing,
// saying why, unless its id, label and plan are ones an account can have.
func (a NewAccount) taken() (NewAccount, error) {
	name := accountNamed(a.ID)
	label, labelErr := labelOf(a.ID, a.Label)
	if err := errors.Join(checkID(name, a.ID), labelErr, checkPlan(name, a.Plan)); err != nil {
		return NewAccount{}, err
	}
	a.Label = label
	return a, nil
}

// labelOf returns the label given the account with the given id as
// takeLabel takes it, or "" for one that's the id, which shows the same
// without one, so the file writes no label where the account's shown by its
// id.
func labelOf(id, given string) (string, error) {
	label, err := takeLabel(accountNamed(id), given)
	if err != nil || label == id {
		return "", err
	}
	return label, nil
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
// when an account has its id already, and fails too when one has it in
// another case, as the two would share a token file. Its label is taken as
// labelOf takes it.
func (d *Draft) AddAccount(given NewAccount) error {
	a, err := given.taken()
	if err != nil {
		return err
	}
	if d.index(a.ID) >= 0 {
		return fmt.Errorf("account %q is %w", a.ID, ErrConfigured)
	}
	if i := slices.IndexFunc(d.file.Accounts, func(b fileAccount) bool { return strings.EqualFold(b.ID, a.ID) }); i >= 0 {
		return caseClash(d.file.Accounts[i].ID, a.ID)
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
	want.Accounts = append(want.Accounts, fileAccount{ID: a.ID, Label: a.Label, Plan: a.Plan, Primary: a.Primary})
	return d.take(doc, want, change)
}

// RemoveAccount removes the account's [[account]] table, and the comments
// directly above it. It fails, matching ErrNotConfigured, when no account has
// the id, and matching ErrOnlyAccount for the only account, as a config needs
// one.
func (d *Draft) RemoveAccount(id string) error {
	i := d.index(id)
	switch {
	case i < 0:
		return NotConfigured(id)
	case len(d.file.Accounts) == 1:
		return fmt.Errorf("account %q is the only one, and %w", id, ErrOnlyAccount)
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

// Upstream returns the API's base URL the draft gives, its default where it
// gives none, failing, as a config would, for one a config can't have.
func (d *Draft) Upstream() (string, error) {
	if d.cfg != nil {
		return d.cfg.Upstream, nil
	}
	if err := checkUpstream(d.file.Upstream); err != nil {
		return "", invalid(d.path, err)
	}
	return d.file.Upstream, nil
}

// MarksPrimary reports whether the draft marks an account primary, rather
// than leaving the first to be the primary for want of one.
func (d *Draft) MarksPrimary() bool {
	return slices.ContainsFunc(d.file.Accounts, func(a fileAccount) bool { return a.Primary })
}

// MarksPrimaryOf reports whether the draft marks the account with the given
// id primary: never where it's the primary only as the first, none marked.
func (d *Draft) MarksPrimaryOf(id string) bool {
	i := d.index(id)
	return i >= 0 && d.file.Accounts[i].Primary
}

// Labelled reports whether the draft writes a label for the account with
// the given id, rather than leaving it to be shown by its id for want of one.
func (d *Draft) Labelled(id string) bool {
	i := d.index(id)
	return i >= 0 && d.file.Accounts[i].Label != ""
}

// SetPrimary makes the account with the given id the primary: it marks it so
// in place of the value any primary it sets gives, keeping a comment beside
// it, else below the last key of its [[account]] table, and takes primary off
// every other account. It fails, matching ErrNotConfigured, when no account
// has the id.
func (d *Draft) SetPrimary(id string) error {
	i := d.index(id)
	switch {
	case i < 0:
		return NotConfigured(id)
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
		if want.Accounts[j].Primary {
			marks = append(marks, d.doc.linesOf(primaryKey, tables[j])...)
		}
		want.Accounts[j].Primary = j == i
	}
	if own := d.doc.linesOf(primaryKey, tables[i]); len(own) > 0 {
		doc := d.doc.replaced(own[0], withValue(d.doc.lines[own[0]], "true"))
		return d.take(doc.without(marks), want, change)
	}
	doc := d.doc.without(marks)
	doc = doc.putKey(doc.accounts()[i], "primary = true")
	return d.take(doc, want, change)
}

// SetPrimeDay sets the day [prime] gives, which turns priming on: in place of
// the day it gives, else as the first key of its table, else in a [prime]
// table made at the end. "" takes the day out, turning priming off, and
// leaves the table. It fails, as ParseDay does, for a day that isn't one.
func (d *Draft) SetPrimeDay(day string) error {
	if _, err := ParseDay(day); err != nil {
		return err
	}
	want := d.file
	want.Prime.Day = day
	change := fmt.Sprintf("set prime.day to %q", day)
	if day == "" {
		change = "take prime.day out"
	}
	return d.take(d.doc.withPrimeDay(day), want, change)
}

// SetLabel sets the label of the account with the given id, as setAccountKey
// sets a key: "" takes it out, showing the account by its id. The label is
// taken as labelOf takes it, and an account shown by it already is left as
// it is, whatever its table writes. It fails, matching ErrNotConfigured, when
// no account has the id, and then as AddAccount does for a label no account
// can have.
func (d *Draft) SetLabel(id, given string) error {
	i := d.index(id)
	if i < 0 {
		return NotConfigured(id)
	}
	label, err := labelOf(id, given)
	if err != nil {
		return err
	}
	if d.file.Accounts[i].shownAs() == (fileAccount{ID: id, Label: label}).shownAs() {
		return nil
	}
	return d.setAccountKey(id, "label", written(label), func(a *fileAccount) { a.Label = label })
}

// SetPlan sets the plan of the account with the given id, one of Plans, as
// setAccountKey sets a key: "" takes it out, leaving the plan unknown. It
// fails, matching ErrNotConfigured, when no account has the id, and for a
// plan that isn't one of Plans.
func (d *Draft) SetPlan(id, plan string) error {
	if err := checkPlan(accountNamed(id), plan); err != nil {
		return err
	}
	return d.setAccountKey(id, "plan", written(plan), func(a *fileAccount) { a.Plan = plan })
}

// SetReserve sets the reserve of the account with the given id, a share of
// every window, as setAccountKey sets a key: 0 takes it out, leaving none. It
// fails, matching ErrNotConfigured, when no account has the id, and for a
// reserve below 0, or of the whole window or more.
func (d *Draft) SetReserve(id string, reserve float64) error {
	if err := checkReserve(accountNamed(id), reserve); err != nil {
		return err
	}
	return d.setAccountKey(id, "reserve", writtenFloat(reserve), func(a *fileAccount) { a.Reserve = reserve })
}

// setAccountKey sets key, in the [[account]] table of the account with the
// given id, to value, as TOML writes it, which set makes of the account: in
// place of the value the table gives it, keeping a comment beside it, else
// below the table's last key; or, for "", taking out the line that sets it.
// An account that has the value already is left as it is.
func (d *Draft) setAccountKey(id, key, value string, set func(*fileAccount)) error {
	i := d.index(id)
	if i < 0 {
		return NotConfigured(id)
	}
	want := d.file
	want.Accounts = slices.Clone(d.file.Accounts)
	set(&want.Accounts[i])
	if want.Accounts[i] == d.file.Accounts[i] {
		return nil
	}
	change := fmt.Sprintf("set %s to %s in account %q", key, value, id)
	if value == "" {
		change = fmt.Sprintf("take %s out of account %q", key, id)
	}
	tables, err := d.accountTables(change)
	if err != nil {
		return err
	}
	return d.take(d.doc.withKey(tables[i], key, value), want, change)
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
	target, err := atomicfile.Target(d.path)
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
