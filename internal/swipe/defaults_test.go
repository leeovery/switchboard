package swipe

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// standInTimeout bounds the wait for a stand-in to make a file, on however
// slow a machine.
const standInTimeout = time.Minute

func TestTheSettingsOutputIsntWaitedForPastWhatItLeavesRunning(t *testing.T) {
	dir := t.TempDir()
	// A defaults that answers, leaving running a program that holds its
	// output open for a minute, whose process id it notes, and that makes a
	// file as it exits.
	defaults, pidFile, exiting := filepath.Join(dir, "defaults"), filepath.Join(dir, "left.pid"), filepath.Join(dir, "exiting")
	script := "#!/bin/sh\n/bin/sleep 60 &\necho $! > '" + pidFile + "'\necho 0\n: > '" + exiting + "'\n"
	if err := os.WriteFile(defaults, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	type answer struct {
		out []byte
		err error
		at  time.Time
	}
	answered := make(chan answer, 1)
	go func() {
		out, err := readSetting(t.Context(), defaults)
		answered <- answer{out: out, err: err, at: time.Now()}
	}()

	exited := made(t, exiting)
	got := <-answered
	if got.err != nil || string(got.out) != "0\n" {
		t.Errorf("readSetting() = %q, %v; want the 0 it printed", got.out, got.err)
	}
	// Timed from the stand-in's exit, so however long it took to start, and
	// with room for a slow machine, far short of the minute it leaves its
	// output held open.
	if waited, within := got.at.Sub(exited), waitDelay+2*time.Second; waited >= within {
		t.Errorf("waited %v for the output once defaults exited, want less than %v", waited, within)
	}
}

// made returns when a file at path is first found, looking every few
// milliseconds for standInTimeout at most.
func made(t *testing.T, path string) time.Time {
	t.Helper()
	deadline := time.Now().Add(standInTimeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return time.Now()
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s after %v", path, standInTimeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
