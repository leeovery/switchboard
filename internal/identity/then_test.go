package identity_test

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
)

// What follows a change given to UpdateThen, as a change of the config's
// does, runs once the record's written, while the record's lock is still
// held: a second change waits for it.
func TestWhatFollowsAChangeRunsOnceItsWrittenUnderTheLock(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	writeFile(t, f, recordOfTwo, 0o600)
	following, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- f.UpdateThen(func(r *identity.Record) (func() error, error) {
			r.Accounts = append(r.Accounts, identity.Entry{ID: "qgr2", Added: added})
			return func() error {
				if got, err := f.Read(); err != nil || !slices.Contains(ids(got.Accounts), "qgr2") {
					t.Errorf("as what follows runs, Read() = %q, %v; want the change written", ids(got.Accounts), err)
				}
				close(following)
				<-release
				return nil
			}, nil
		})
	}()
	<-following
	second := make(chan error, 1)
	go func() { second <- f.Update(func(*identity.Record) error { return nil }) }()
	select {
	case err := <-second:
		t.Errorf("a second change ended, %v, while what follows the first ran, want it waiting on the lock", err)
	default:
	}
	close(release)
	if err := errors.Join(<-done, <-second); err != nil {
		t.Fatal(err)
	}
}

// A change that changes nothing writes nothing, not even a record of none,
// and what follows it runs all the same; what follows failing fails the
// change, the record left as it was written, ahead.
func TestUpdateThenWritesOnlyWhatChanged(t *testing.T) {
	failed := errors.New("the config's save failed")
	t.Run("unchanged", func(t *testing.T) {
		f := identity.NewFile(t.TempDir())
		ran := false
		if err := f.UpdateThen(func(*identity.Record) (func() error, error) {
			return func() error { ran = true; return nil }, nil
		}); err != nil || !ran {
			t.Fatalf("UpdateThen() = %v, what follows run: %v; want it run, no error", err, ran)
		}
		if _, err := os.Stat(f.Path()); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the record's file: %v, want none written", err)
		}
	})
	t.Run("what follows failing", func(t *testing.T) {
		f := identity.NewFile(t.TempDir())
		err := f.UpdateThen(func(r *identity.Record) (func() error, error) {
			r.Accounts = append(r.Accounts, identity.Entry{ID: "qgr2", Added: added})
			return func() error { return failed }, nil
		})
		if !errors.Is(err, failed) {
			t.Errorf("UpdateThen() = %v, want what follows' error", err)
		}
		if got, err := f.Read(); err != nil || !slices.Equal(ids(got.Accounts), []string{"qgr2"}) {
			t.Errorf("Read() = %q, %v; want the change written", ids(got.Accounts), err)
		}
	})
	t.Run("the record not written", func(t *testing.T) {
		f := identity.NewFile(t.TempDir())
		ran := false
		err := f.UpdateThen(func(r *identity.Record) (func() error, error) {
			r.Accounts = append(r.Accounts, identity.Entry{ID: "qgr2"})
			return func() error { ran = true; return nil }, nil
		})
		if err == nil || ran {
			t.Errorf("UpdateThen() = %v, what follows run: %v; want an error, and nothing following", err, ran)
		}
	})
}

// A File of no state directory reads and writes nothing, never a record of
// the working directory's.
func TestARecordOfNoStateDirectoryIsAnError(t *testing.T) {
	var f identity.File
	if _, err := f.Read(); err == nil {
		t.Error("Read() of no state directory succeeded, want an error")
	}
	if err := f.Update(func(*identity.Record) error { return nil }); err == nil {
		t.Error("Update() of no state directory succeeded, want an error")
	}
	if err := f.UpdateThen(func(*identity.Record) (func() error, error) { return nil, nil }); err == nil {
		t.Error("UpdateThen() of no state directory succeeded, want an error")
	}
	for _, name := range []string{"accounts.json", "accounts.lock"} {
		if _, err := os.Stat(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s in the working directory: %v, want none", name, err)
		}
	}
}
