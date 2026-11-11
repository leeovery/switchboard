package identity_test

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
)

// What a change given to Change does once it commits, as a change of the
// config's does, runs once the record's written, while the record's lock is
// still held: a second change waits for it.
func TestWhatFollowsACommitRunsOnceItsWrittenUnderTheLock(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	writeFile(t, f, recordOfTwo, 0o600)
	following, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- f.Change(func(r *identity.Record, commit func() error) error {
			r.Accounts = append(r.Accounts, identity.Entry{ID: "qgr2", Added: added})
			if err := commit(); err != nil {
				return err
			}
			if got, err := f.Read(); err != nil || !slices.Contains(ids(got.Accounts), "qgr2") {
				t.Errorf("once committed, Read() = %q, %v; want the change written", ids(got.Accounts), err)
			}
			close(following)
			<-release
			return nil
		})
	}()
	<-following
	second := make(chan error, 1)
	go func() { second <- f.Update(func(*identity.Record) error { return nil }) }()
	select {
	case err := <-second:
		t.Errorf("a second change ended, %v, while the first's ran on past its commit, want it waiting on the lock", err)
	default:
	}
	close(release)
	if err := errors.Join(<-done, <-second); err != nil {
		t.Fatal(err)
	}
}

// Change writes the record only where it changed, committed or not; a change
// that fails once it's committed puts the record back as it was, its error
// returned; one that fails before writes nothing.
func TestChangeWritesOnlyWhatHolds(t *testing.T) {
	failed := errors.New("the config's save failed")
	qgr2 := identity.Entry{ID: "qgr2", Added: added}
	tests := []struct {
		name    string
		change  func(r *identity.Record, commit func() error) error
		wantErr error
		// want is the record's file after: recordOfTwo where it's "".
		want []string
	}{
		{
			name:   "changed, committed by Change",
			change: func(r *identity.Record, _ func() error) error { r.Accounts = append(r.Accounts, qgr2); return nil },
			want:   []string{"b6s7", "m2x7", "qgr2"},
		},
		{
			name: "failing once committed, put back",
			change: func(r *identity.Record, commit func() error) error {
				r.Accounts = append(r.Accounts, qgr2)
				return errors.Join(commit(), failed)
			},
			wantErr: failed,
		},
		{
			name: "failing before it's committed",
			change: func(r *identity.Record, _ func() error) error {
				r.Accounts = append(r.Accounts, qgr2)
				return failed
			},
			wantErr: failed,
		},
		{
			name: "committing a record it would find damaged",
			change: func(r *identity.Record, commit func() error) error {
				r.Accounts = append(r.Accounts, identity.Entry{ID: "qgr2"})
				return commit()
			},
			wantErr: errors.New("any"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := identity.NewFile(t.TempDir())
			writeFile(t, f, recordOfTwo, 0o600)
			err := f.Change(tt.change)
			if (err == nil) != (tt.wantErr == nil) || tt.wantErr == failed && !errors.Is(err, failed) {
				t.Errorf("Change() = %v, want %v", err, tt.wantErr)
			}
			if tt.want == nil {
				if got := readFile(t, f); got != recordOfTwo {
					t.Errorf("the record's file holds\n%s\nwant it as it was", got)
				}
				return
			}
			if got, err := f.Read(); err != nil || !slices.Equal(ids(got.Accounts), tt.want) {
				t.Errorf("Read() = %q, %v; want %q", ids(got.Accounts), err, tt.want)
			}
		})
	}
}

// A change that changes nothing writes nothing, not even a record of none.
func TestAChangeOfNothingWritesNothing(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	if err := f.Change(func(_ *identity.Record, commit func() error) error { return commit() }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the record's file: %v, want none written", err)
	}
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
	if err := f.Change(func(*identity.Record, func() error) error { return nil }); err == nil {
		t.Error("Change() of no state directory succeeded, want an error")
	}
	for _, name := range []string{"accounts.json", "accounts.lock"} {
		if _, err := os.Stat(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s in the working directory: %v, want none", name, err)
		}
	}
}

// A change that fails once it's committed the first record there's been
// takes its file away again, leaving none, as there was.
func TestAChangeThatFailsLeavesNoRecordWhereThereWasNone(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	failed := errors.New("the config's save failed")
	err := f.Change(func(r *identity.Record, commit func() error) error {
		r.Accounts = append(r.Accounts, identity.Entry{ID: "qgr2", Added: added})
		return errors.Join(commit(), failed)
	})
	if !errors.Is(err, failed) {
		t.Errorf("Change() = %v, want the change's error", err)
	}
	if _, err := os.Stat(f.Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the record's file: %v, want none", err)
	}
}
