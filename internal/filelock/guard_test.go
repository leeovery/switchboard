package filelock_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so no package but filelock can take
// a file lock of its own, rather than through filelock, its one owner,
// without failing here. A test may: it holds a lock as another process
// would, to see what the code under test does.

// lockOwner is the one package, by directory from the module's root, that
// takes a file lock.
const lockOwner = "internal/filelock"

// lockCalls are the calls that take a file lock, by their full names: flock
// and fcntl's, the standard library's and golang.org/x/sys/unix's.
var lockCalls = []string{
	"syscall.Flock",
	"syscall.FcntlFlock",
	"golang.org/x/sys/unix.Flock",
	"golang.org/x/sys/unix.FcntlFlock",
}

// lockFix is the fix a breach gives.
const lockFix = "take the lock through internal/filelock, adding what it lacks there"

func TestOnlyFilelockTakesAFileLock(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	others := slices.DeleteFunc(sourcecheck.Packages(t, root), func(dir string) bool { return dir == lockOwner })
	sourcecheck.ReportEach(t, locksTaken(t, root, sourcecheck.ModulePath(t, root), others))
}

func TestTheLockGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, lockFixture)
	got := locksTaken(t, root, "example.com/fixture", []string{"internal/store"})
	want := []string{
		"internal/store/store.go:10: " + lockSays("syscall.Flock"),
		"internal/store/store.go:11: " + lockSays("syscall.FcntlFlock"),
		"internal/store/store.go:12: " + lockSays("golang.org/x/sys/unix.Flock"),
		"internal/store/store.go:13: " + lockSays("golang.org/x/sys/unix.FcntlFlock"),
		"internal/store/store.go:16: " + lockSays("syscall.Flock"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// locksTaken are where the non-test files of the packages in dirs, of the
// module at root with the given path, take a file lock, or hand on what
// takes one as a value.
func locksTaken(t *testing.T, root, module string, dirs []string) []string {
	t.Helper()
	calls := make(map[string]sourcecheck.CallBan, len(lockCalls))
	for _, name := range lockCalls {
		calls[name] = sourcecheck.CallBan{Says: lockSays(name)}
	}
	return sourcecheck.Check(t, root, module, sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Calls: calls}, dirs, sourcecheck.Allowance{})
}

// lockSays says how calling what's named breaks the rule.
func lockSays(name string) string {
	return name + " takes a file lock outside " + lockOwner + "; " + lockFix
}

// lockFixture is a module whose filelock takes a lock, as it may, and a
// store that takes each kind itself, and hands one on as a value, beside a
// test of its own that takes one, as a test may.
var lockFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/filelock/filelock.go": `package filelock

import "syscall"

func Lock(fd int) error { return syscall.Flock(fd, syscall.LOCK_EX) }
`,
	"internal/store/store.go": `package store

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func locks(fd int) error {
	_ = syscall.Flock(fd, syscall.LOCK_EX)
	_ = syscall.FcntlFlock(uintptr(fd), syscall.F_SETLK, nil)
	_ = unix.Flock(fd, 2)
	return unix.FcntlFlock(uintptr(fd), 8, nil)
}

var handedOn = syscall.Flock
`,
	"internal/store/store_test.go": `package store

import (
	"syscall"
	"testing"
)

func TestHeld(t *testing.T) { _ = syscall.Flock(0, syscall.LOCK_EX) }
`,
}
