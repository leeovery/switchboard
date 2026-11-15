package identity_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/identity"
)

// recordOfTwo is an accounts record's file holding two accounts, as written.
const recordOfTwo = `{
  "version": 2,
  "accounts": [
    {
      "id": "b6s7",
      "organization": "test-org-work",
      "added": "2026-09-01T12:00:00Z"
    },
    {
      "id": "m2x7",
      "label": "side",
      "added": "2026-09-01T12:00:00Z",
      "removed": "2026-10-20T07:45:00Z",
      "was": "side"
    }
  ]
}
`

// The record is written whole, in its format: each account's id, its
// organization where known, its label once removed, when it was added and
// removed in UTC to the second, and its id before ids were made where it had
// one; and it reads back as it was written.
func TestTheRecordIsWrittenAndReadBack(t *testing.T) {
	cet := time.FixedZone("CET", 60*60)
	accounts := []identity.Entry{
		{ID: "b6s7", Organization: "test-org-work", Added: time.Date(2026, 11, 10, 10, 30, 0, 123456789, cet)},
		{
			ID: "qgr2", Organization: "test-org-personal", Label: "personal",
			Added: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), Removed: time.Date(2026, 11, 9, 18, 15, 30, 500000000, time.UTC), Was: "personal",
		},
		{ID: "m2x7", Label: "side", Added: added, Removed: removed, Was: "side"},
		{ID: "k3f9", Added: added},
	}
	f := identity.NewFile(t.TempDir())
	if err := f.Update(func(r *identity.Record) error { r.Accounts = accounts; return nil }); err != nil {
		t.Fatal(err)
	}
	const want = `{
  "version": 2,
  "accounts": [
    {
      "id": "b6s7",
      "organization": "test-org-work",
      "added": "2026-11-10T09:30:00Z"
    },
    {
      "id": "qgr2",
      "organization": "test-org-personal",
      "label": "personal",
      "added": "2026-10-01T08:00:00Z",
      "removed": "2026-11-09T18:15:30Z",
      "was": "personal"
    },
    {
      "id": "m2x7",
      "label": "side",
      "added": "2026-09-01T12:00:00Z",
      "removed": "2026-10-20T07:45:00Z",
      "was": "side"
    },
    {
      "id": "k3f9",
      "added": "2026-09-01T12:00:00Z"
    }
  ]
}
`
	if got := readFile(t, f); got != want {
		t.Errorf("the record's file holds\n%s\nwant\n%s", got, want)
	}
	got, err := f.Read()
	if err != nil {
		t.Fatal(err)
	}
	stamped := slices.Clone(accounts)
	stamped[0].Added = time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)
	stamped[1].Removed = time.Date(2026, 11, 9, 18, 15, 30, 0, time.UTC)
	if !slices.EqualFunc(got.Accounts, stamped, same) {
		t.Errorf("Read() = %+v, want %+v", got.Accounts, stamped)
	}
}

// A record without accounts is written as one holding none.
func TestAnEmptyRecordIsWrittenHoldingNone(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	if err := f.Update(func(*identity.Record) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, f), "{\n  \"version\": 2,\n  \"accounts\": []\n}\n"; got != want {
		t.Errorf("the record's file holds\n%s\nwant\n%s", got, want)
	}
}

// An account removed before it was added, by the times kept, as by a clock
// set ahead as it was added and put right before it was removed, is written
// and read back as any other.
func TestAnAccountRemovedBeforeItWasAddedIsKept(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	account := identity.Entry{ID: "m2x7", Label: "side", Added: removed, Removed: added}
	if err := f.Update(func(r *identity.Record) error { r.Accounts = []identity.Entry{account}; return nil }); err != nil {
		t.Fatal(err)
	}
	got, err := f.Read()
	if err != nil || !slices.EqualFunc(got.Accounts, []identity.Entry{account}, same) {
		t.Errorf("Read() = %+v, %v; want %+v", got.Accounts, err, account)
	}
}

// Without its file, the record is empty, and reading it makes none.
func TestARecordWithoutItsFileIsEmpty(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	got, err := f.Read()
	if err != nil || len(got.Accounts) != 0 {
		t.Fatalf("Read() = %+v, %v; want an empty record", got, err)
	}
	if _, err := os.Lstat(f.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("reading the record left something at %s (%v)", f.Path(), err)
	}
}

// A damaged record is an error matching ErrDamaged, reading it and changing
// it, and is left as it was: never replaced, its change never made.
func TestADamagedRecordIsAnErrorAndLeftAsItWas(t *testing.T) {
	tests := []struct {
		name, data string
	}{
		{name: "cut short", data: recordOfTwo[:40]},
		{name: "not JSON", data: "accounts: b6s7\n"},
		{name: "empty", data: ""},
		{name: "of a later version", data: `{"version": 3, "accounts": []}`},
		{name: "of no version", data: `{"accounts": []}`},
		{name: "accounts not a list", data: `{"version": 1, "accounts": {"id": "b6s7"}}`},
		{name: "a time not RFC 3339", data: `{"version": 1, "accounts": [{"id": "b6s7", "added": "1 September 2026"}]}`},
		{name: "an account without an id", data: `{"version": 1, "accounts": [{"added": "2026-09-01T12:00:00Z"}]}`},
		{name: "an id that's a path", data: `{"version": 1, "accounts": [{"id": "../tokens", "added": "2026-09-01T12:00:00Z"}]}`},
		{name: "an id that starts with a digit", data: `{"version": 1, "accounts": [{"id": "4b6s", "added": "2026-09-01T12:00:00Z"}]}`},
		{name: "an id too short", data: `{"version": 1, "accounts": [{"id": "b6s", "added": "2026-09-01T12:00:00Z"}]}`},
		{name: "an id in capitals", data: `{"version": 1, "accounts": [{"id": "B6S7", "added": "2026-09-01T12:00:00Z"}]}`},
		{name: "an id that's the word pin takes", data: `{"version": 1, "accounts": [{"id": "auto", "added": "2026-09-01T12:00:00Z"}]}`},
		{name: "an account added at no time", data: `{"version": 1, "accounts": [{"id": "b6s7"}]}`},
		{
			name: "two accounts with one id",
			data: `{"version": 1, "accounts": [{"id": "b6s7", "added": "2026-09-01T12:00:00Z"}, {"id": "b6s7", "added": "2026-09-01T12:00:00Z"}]}`,
		},
		{
			name: "two accounts of one organization",
			data: `{"version": 1, "accounts": [{"id": "b6s7", "organization": "test-org-work", "added": "2026-09-01T12:00:00Z"},
				{"id": "qgr2", "organization": "test-org-work", "added": "2026-09-01T12:00:00Z"}]}`,
		},
		{
			name: "two accounts with one id before",
			data: `{"version": 1, "accounts": [{"id": "b6s7", "was": "work", "added": "2026-09-01T12:00:00Z"},
				{"id": "qgr2", "was": "work", "added": "2026-09-01T12:00:00Z"}]}`,
		},
		{
			name: "two accounts with ids before differing only in case",
			data: `{"version": 1, "accounts": [{"id": "b6s7", "was": "work", "added": "2026-09-01T12:00:00Z"},
				{"id": "qgr2", "was": "Work", "added": "2026-09-01T12:00:00Z"}]}`,
		},
		{
			name: "an account's id another had before",
			data: `{"version": 1, "accounts": [{"id": "b6s7", "added": "2026-09-01T12:00:00Z"},
				{"id": "qgr2", "was": "b6s7", "added": "2026-09-01T12:00:00Z"}]}`,
		},
		{
			name: "an account's id another had before, in another case",
			data: `{"version": 1, "accounts": [{"id": "qgr2", "was": "WORK", "added": "2026-09-01T12:00:00Z"},
				{"id": "work", "added": "2026-09-01T12:00:00Z"}]}`,
		},
		{
			name: "an account removed without a label",
			data: `{"version": 1, "accounts": [{"id": "m2x7", "added": "2026-09-01T12:00:00Z", "removed": "2026-10-20T07:45:00Z"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := identity.NewFile(t.TempDir())
			writeFile(t, f, tt.data, 0o600)
			if got, err := f.Read(); !errors.Is(err, identity.ErrDamaged) {
				t.Errorf("Read() = %+v, %v; want an error matching ErrDamaged", got, err)
			}
			checkUnchanged(t, f, tt.data)
		})
	}
}

// A record that can't be read is an error, reading it and changing it, but
// no error of a damaged one: it's left as it is.
func TestARecordThatCantBeReadIsAnError(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	if err := os.Mkdir(f.Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Read(); err == nil || errors.Is(err, identity.ErrDamaged) {
		t.Errorf("Read() of a directory = %v; want an error, not one of a damaged record", err)
	}
	called := false
	if err := f.Update(func(*identity.Record) error { called = true; return nil }); err == nil || called {
		t.Errorf("Update() of a directory = %v, the change made: %v; want an error, and no change", err, called)
	}
	if info, err := os.Lstat(f.Path()); err != nil || !info.IsDir() {
		t.Errorf("the directory at %s is gone (%v)", f.Path(), err)
	}
}

// The record is written that only its owner can read or write, in place of
// a file others could read.
func TestTheRecordIsWrittenPrivate(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	writeFile(t, f, recordOfTwo, 0o644)
	if err := f.Update(func(*identity.Record) error { return nil }); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(f.Path())
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the record's file has permissions %o, want 600", mode)
	}
}

// Changing the record makes the state directory where it isn't there.
func TestChangingTheRecordMakesTheStateDirectory(t *testing.T) {
	f := identity.NewFile(filepath.Join(t.TempDir(), "state", "switchboard"))
	if err := f.Update(func(r *identity.Record) error {
		r.Accounts = append(r.Accounts, identity.Entry{ID: "b6s7", Added: added})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := f.Read(); err != nil || !slices.Equal(ids(got.Accounts), []string{"b6s7"}) {
		t.Errorf("Read() = %+v, %v; want the account added", got, err)
	}
}

// A change that fails, or that would leave the record damaged, writes
// nothing, its error returned.
func TestAChangeThatFailsWritesNothing(t *testing.T) {
	failed := errors.New("the change failed")
	tests := []struct {
		name   string
		change func(*identity.Record) error
		want   error
	}{
		{
			name: "failing",
			change: func(r *identity.Record) error {
				r.Accounts = nil
				return failed
			},
			want: failed,
		},
		{
			name: "giving an account another's id",
			change: func(r *identity.Record) error {
				r.Accounts[1].ID = r.Accounts[0].ID
				return nil
			},
		},
		{
			name: "giving an account an id switchboard doesn't make",
			change: func(r *identity.Record) error {
				r.Accounts = append(r.Accounts, identity.Entry{ID: "work", Added: added}, identity.Entry{ID: "Personal", Added: added})
				return nil
			},
		},
		{
			name: "adding an account at no time",
			change: func(r *identity.Record) error {
				r.Accounts = append(r.Accounts, identity.Entry{ID: "qgr2"})
				return nil
			},
		},
		{
			name: "adding an account at a time the second it's kept to makes none",
			change: func(r *identity.Record) error {
				r.Accounts = append(r.Accounts, identity.Entry{ID: "qgr2", Added: time.Time{}.Add(time.Millisecond)})
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := identity.NewFile(t.TempDir())
			writeFile(t, f, recordOfTwo, 0o600)
			err := f.Update(tt.change)
			if err == nil || tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("Update() = %v, want an error, matching %v where it's the change's", err, tt.want)
			}
			if got := readFile(t, f); got != recordOfTwo {
				t.Errorf("the record's file holds\n%s\nwant it as it was", got)
			}
		})
	}
}

// Two changes at once both land: the second waits for the first to be
// written before it reads the record.
func TestChangesAtOnceBothLand(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	add := func(id string) func(*identity.Record) error {
		return func(r *identity.Record) error {
			r.Accounts = append(r.Accounts, identity.Entry{ID: id, Added: added})
			return nil
		}
	}
	firstIn, release, secondIn := make(chan struct{}), make(chan struct{}), make(chan struct{})
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() {
		firstDone <- f.Update(func(r *identity.Record) error {
			close(firstIn)
			<-release
			return add("b6s7")(r)
		})
	}()
	<-firstIn
	go func() {
		secondDone <- f.Update(func(r *identity.Record) error {
			close(secondIn)
			return add("qgr2")(r)
		})
	}()
	select {
	case <-secondIn:
		t.Error("the second change read the record while the first was making its change")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := errors.Join(<-firstDone, <-secondDone); err != nil {
		t.Fatal(err)
	}
	got, err := f.Read()
	if want := []string{"b6s7", "qgr2"}; err != nil || !slices.Equal(ids(got.Accounts), want) {
		t.Errorf("Read() = %q, %v; want both changes' accounts, %q", ids(got.Accounts), err, want)
	}
}

// checkUnchanged fails the test unless changing the record fails with an
// error matching ErrDamaged, its change never made, and its file still holds
// data.
func checkUnchanged(t *testing.T, f identity.File, data string) {
	t.Helper()
	called := false
	if err := f.Update(func(*identity.Record) error { called = true; return nil }); !errors.Is(err, identity.ErrDamaged) || called {
		t.Errorf("Update() = %v, the change made: %v; want an error matching ErrDamaged, and no change", err, called)
	}
	if got := readFile(t, f); got != data {
		t.Errorf("the record's file holds %q, want it as it was, %q", got, data)
	}
}

// same reports whether two accounts are the same, their times the same
// instants.
func same(a, b identity.Entry) bool {
	return a.ID == b.ID && a.Organization == b.Organization && a.Label == b.Label &&
		a.Added.Equal(b.Added) && a.Removed.Equal(b.Removed) && a.Was == b.Was
}

// writeFile writes data to the record's file, with the permissions perm.
func writeFile(t *testing.T, f identity.File, data string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(f.Path(), []byte(data), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f.Path(), perm); err != nil {
		t.Fatal(err)
	}
}

// readFile returns what the record's file holds.
func readFile(t *testing.T, f identity.File) string {
	t.Helper()
	data, err := os.ReadFile(f.Path())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
