//go:build unix

package shell_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/shell"
)

func TestRunGivesTheProgramNoInputAndItsOwnEnvironment(t *testing.T) {
	out, err := shell.Run(t.Context(), "/bin/sh", []string{"-c", `read line; printf '[%s] %s\n' "$line" "$GIVEN"; printf 'more\n'`}, []string{"GIVEN=given"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), "[] given\nmore\n"; got != want {
		t.Errorf("Run() printed %q, want %q", got, want)
	}
}

func TestRunReturnsWhatTheProgramPrintedWhenItFails(t *testing.T) {
	out, err := shell.Run(t.Context(), "/bin/sh", []string{"-c", "printf 'said\\n'; exit 3"}, nil)
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok || exit.ExitCode() != 3 || string(out) != "said\n" {
		t.Errorf("Run() = %q, %v; want %q, and the exit's status, 3", out, err, "said\n")
	}
}

func TestRunKillsEverythingTheProgramStartedOnceItsTimeIsUp(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()

	_, err := shell.Run(ctx, "/bin/sh", []string{"-c", `sleep 30 & echo $! >"$PID_FILE"; sleep 30`}, []string{"PID_FILE=" + pidFile})
	if err == nil {
		t.Fatal("Run() succeeded, want it to fail, killed")
	}
	if took := time.Since(started); took > 10*time.Second {
		t.Errorf("Run() took %s, want it to return once its time's up", took)
	}
	checkKilled(t, pidFile)
}

func TestRunKillsEverythingTheProgramStartedOnceItExits(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	started := time.Now()

	// The sleep holds the output open, as something a startup file starts
	// can.
	out, err := shell.Run(t.Context(), "/bin/sh", []string{"-c", `sleep 30 & echo $! >"$PID_FILE"; echo answered`}, []string{"PID_FILE=" + pidFile})
	if err != nil || string(out) != "answered\n" {
		t.Errorf("Run() = %q, %v; want %q", out, err, "answered\n")
	}
	if took := time.Since(started); took > 10*time.Second {
		t.Errorf("Run() took %s, want it to return once the program exits", took)
	}
	checkKilled(t, pidFile)
}

// checkKilled checks the program whose pid is in pidFile was killed.
func checkKilled(t *testing.T, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	// Killed, the program it started is reaped by launchd, as it's orphaned,
	// which takes a moment.
	for deadline := time.Now().Add(5 * time.Second); syscall.Kill(pid, 0) == nil; {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the sleep the program started, pid %d, is running still", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
