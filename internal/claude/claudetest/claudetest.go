// Package claudetest makes stand-ins, for tests, of what switchboard finds
// claude among: Claude Code, and switchboard's own binary and claude link.
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
