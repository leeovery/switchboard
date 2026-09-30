// Package claudetest makes stand-ins, for tests, of what switchboard finds
// claude among: Claude Code, switchboard's own binary and claude link, and
// other builds of it.
package claudetest

import (
	"os"
	"path/filepath"
	"testing"
)

// Program writes a program at path, making its directory, and returns path.
// It fails, were it ever run.
func Program(t testing.TB, path string) string {
	t.Helper()
	makeDir(t, path)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// Build copies the test's own program, which Go built, to path, making its
// directory, and returns path: another build of the program os.Executable
// gives, as the Go build info each holds says, just as another build of
// switchboard is of switchboard. It's never run.
func Build(t testing.TB, path string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	program, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	makeDir(t, path)
	if err := os.WriteFile(path, program, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// Link makes a symbolic link at path leading to target, making its
// directory, and returns path.
func Link(t testing.TB, target, path string) string {
	t.Helper()
	makeDir(t, path)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// makeDir makes the directory path is in.
func makeDir(t testing.TB, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
}
