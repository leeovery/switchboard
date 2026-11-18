//go:build unix

package filelock_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/filelock"
)

// The bounds on a lock's call: waited is how long one that should wait is
// watched before it's taken to be waiting, and returned how long one that
// should return is given to, however busy the machine.
const (
	waited   = 100 * time.Millisecond
	returned = 10 * time.Second
)

// lockPath is a path to hold a lock on, in a directory of the test's own.
func lockPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "test.lock")
}

// openLock opens the file at path, as another holder of its lock would.
func openLock(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// waiting reports whether the call done tells of is still waiting once
// waited has passed, its error, where it returned, a test error.
func waiting(t *testing.T, done <-chan error) bool {
	t.Helper()
	return !returnsWithin(t, done, waited)
}

// returns reports whether the call done tells of returns within returned,
// its error a test error.
func returns(t *testing.T, done <-chan error) bool {
	t.Helper()
	return returnsWithin(t, done, returned)
}

// returnsWithin reports whether the call done tells of returns within
// bound, its error a test error.
func returnsWithin(t *testing.T, done <-chan error, bound time.Duration) bool {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the lock waited for = %v", err)
		}
		return true
	case <-time.After(bound):
		return false
	}
}

func TestAnExclusiveLockWaitsWhileAnotherHoldsOne(t *testing.T) {
	path := lockPath(t)
	unlock, err := filelock.Take(path)
	if err != nil {
		t.Fatalf("Take() = %v", err)
	}
	done := make(chan error, 1)
	go func() {
		second, err := filelock.Take(path)
		if err == nil {
			second()
		}
		done <- err
	}()
	if !waiting(t, done) {
		t.Fatal("a second Take() returned while the first held the lock, want it waiting")
	}
	unlock()
	if !returns(t, done) {
		t.Error("a second Take() still waited once the first let the lock go")
	}
}

func TestTryingWhileAnotherHoldsTheLockReportsFalseAtOnce(t *testing.T) {
	path := lockPath(t)
	unlock, err := filelock.Take(path)
	if err != nil {
		t.Fatalf("Take() = %v", err)
	}
	defer unlock()
	second, ok, err := filelock.TryTake(path)
	if err != nil || ok || second != nil {
		t.Errorf("TryTake() while another held the lock = (unlock %t, %t, %v), want (no unlock, false, nil)", second != nil, ok, err)
	}
	if taken, err := filelock.TryLock(openLock(t, path)); err != nil || taken {
		t.Errorf("TryLock() while another held the lock = (%t, %v), want (false, nil)", taken, err)
	}
}

func TestSharedLocksAreHeldTogetherAndKeepAnExclusiveOneOut(t *testing.T) {
	path := lockPath(t)
	if err := filelock.LockShared(openLock(t, mustMake(t, path))); err != nil {
		t.Fatalf("LockShared() = %v", err)
	}
	second := openLock(t, path)
	done := make(chan error, 1)
	go func() { done <- filelock.LockShared(second) }()
	if !returns(t, done) {
		t.Error("a second LockShared() waited while the first held a shared lock, want both held at once")
	}
	if taken, err := filelock.TryLock(openLock(t, path)); err != nil || taken {
		t.Errorf("TryLock() while shared locks were held = (%t, %v), want (false, nil)", taken, err)
	}
}

func TestALockGoesAsItsFileClosesLeavingTheFileInPlace(t *testing.T) {
	path := lockPath(t)
	unlock, err := filelock.Take(path)
	if err != nil {
		t.Fatalf("Take() = %v", err)
	}
	unlock()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the lock's file once let go: %v, want it left in place", err)
	}
	again, ok, err := filelock.TryTake(path)
	if err != nil || !ok {
		t.Fatalf("TryTake() once the lock was let go = (%t, %v), want (true, nil)", ok, err)
	}
	defer again()
	unlock()
	if taken, err := filelock.TryLock(openLock(t, path)); err != nil || taken {
		t.Errorf("TryLock() once the first holder let go twice = (%t, %v), want (false, nil): its second unlock lets go no lock another holds", taken, err)
	}
}

func TestTakeMakesTheLocksFileTheUsersAlone(t *testing.T) {
	path := lockPath(t)
	unlock, err := filelock.Take(path)
	if err != nil {
		t.Fatalf("Take() = %v", err)
	}
	defer unlock()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("the lock's file is %v, want -rw-------", got)
	}
}

func TestTakeFailsWhereTheLocksFileCantBeOpened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "test.lock")
	if _, err := filelock.Take(path); err == nil {
		t.Error("Take() in a directory that isn't there = nil, want an error")
	}
	if _, _, err := filelock.TryTake(path); err == nil {
		t.Error("TryTake() in a directory that isn't there = nil, want an error")
	}
}

func TestTakeDirLocksTheDirectoryTillItsLetGo(t *testing.T) {
	dir := t.TempDir()
	unlock := filelock.TakeDir(dir)
	if taken, err := filelock.TryLock(openLock(t, dir)); err != nil || taken {
		t.Errorf("TryLock() on the directory TakeDir() held = (%t, %v), want (false, nil)", taken, err)
	}
	unlock()
	if taken, err := filelock.TryLock(openLock(t, dir)); err != nil || !taken {
		t.Errorf("TryLock() on the directory once let go = (%t, %v), want (true, nil)", taken, err)
	}
}

func TestTakeDirCarriesOnWithoutALockWhereItCantTakeOne(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	done := make(chan error, 1)
	go func() {
		unlock := filelock.TakeDir(dir)
		unlock()
		unlock()
		done <- nil
	}()
	if !returns(t, done) {
		t.Fatal("TakeDir() on a directory that isn't there waited, want it to carry on at once")
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if taken, err := filelock.TryLock(openLock(t, dir)); err != nil || !taken {
		t.Errorf("TryLock() on the directory made once TakeDir() carried on = (%t, %v), want (true, nil): it holds nothing", taken, err)
	}
}

// mustMake makes the file at path, empty, and returns path.
func mustMake(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
