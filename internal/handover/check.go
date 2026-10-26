package handover

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
)

// CheckFlag names the flag, --check, by which a process asks the program it's
// about to replace itself with whether that could start, as Ask asks. Run
// with it, its command line otherwise as it'll be run, a program that knows
// the flag checks what it would start from, and exits: 0 where it could
// start, and Refused, saying why on its standard output, where it couldn't.
// As with Variable, one version asks and the next answers, so its form
// changes only in ways the next can still read.
const CheckFlag = "check"

// Refused is the exit status of a program run with CheckFlag that couldn't
// start from what it would start from: EX_CONFIG, as sysexits.h names it. A
// program from before the check doesn't know the flag, and exits otherwise,
// with 1, as for any command line it can't read.
const Refused = 78

// runWaitDelay bounds the wait for a program's output once it has exited, or
// been killed, as Run runs it: one it leaves running can hold its output
// open.
const runWaitDelay = 500 * time.Millisecond

// Runner runs the program at path as argv with env until it exits, or ctx
// ends, as Run does, and returns what it printed on its standard output. It
// fails, with an error that has an ExitCode method, as *exec.ExitError does,
// when the program exits with a status other than 0.
type Runner func(ctx context.Context, path string, argv, env []string) ([]byte, error)

// Run runs the program at path as argv with env until it exits, or ctx ends,
// when it's killed, and returns what it printed on its standard output.
func Run(ctx context.Context, path string, argv, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path)
	cmd.Args, cmd.Env, cmd.WaitDelay = argv, env, runWaitDelay
	return cmd.Output()
}

// exitError is a program's exit with a status other than 0.
type exitError interface {
	error
	ExitCode() int
}

// Ask asks the program at path, which a process is about to replace itself
// with, run as argv with env, whether it could start: run by run as argv with
// CheckFlag added, in env less Variable, as it takes up no listener. It
// returns why the program refuses, as reason gives it, and fails when the
// program can't answer: it isn't there, it doesn't know the flag, as one from
// before the check doesn't, it fails otherwise, as by crashing, or ctx ends
// first.
func Ask(ctx context.Context, run Runner, path string, argv, env []string) (refusal string, err error) {
	out, err := run(ctx, path, append(slices.Clone(argv), "--"+CheckFlag), without(env))
	if exit, ok := errors.AsType[exitError](err); ok && exit.ExitCode() == Refused {
		return reason(out), nil
	}
	return "", err
}

// reason is why a program refuses, as it said on out, anything in it shaped
// like a token hidden: "it gave no reason" where nothing it said would read,
// once cleaned as prose.Clean cleans what a terminal shows, so a refusal
// always says something.
func reason(out []byte) string {
	why := redact.Text(strings.TrimSpace(string(out)))
	if prose.Clean(why) == "" {
		return "it gave no reason"
	}
	return why
}
