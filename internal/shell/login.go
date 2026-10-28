package shell

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
)

// Runner runs the program at path with args, in env, its input empty, until
// it exits, or ctx ends, when it's killed with whatever it started, as Run
// does; and returns what it printed on its standard output, which it returns
// as well when it fails. It fails, with an error that has an ExitCode
// method, as *exec.ExitError does, when the program exits with a status
// other than 0.
type Runner func(ctx context.Context, path string, args, env []string) ([]byte, error)

// exitError is a program's exit with a status other than 0.
type exitError interface {
	error
	ExitCode() int
}

// Login is the user's login shell, $SHELL, as a new terminal starts it.
type Login struct {
	// Environ is the environment setup runs in, which the login shell's is
	// made from.
	Environ []string
	// Run runs it, as Run does.
	Run Runner
	// Limit is how long it's given to answer. Zero means 15 seconds: its
	// startup files can take a while on a busy machine.
	Limit time.Duration
}

const defaultLimit = 15 * time.Second

// launchdPath is the PATH launchd starts the user's programs with, and so a
// new terminal's shell, before its startup files change it.
const launchdPath = "/usr/bin:/bin:/usr/sbin:/sbin"

// sentinel comes before the shell's answer, which is the line after it, as
// startup files print banners and escape codes of their own first.
const sentinel = "switchboard-asks-which-claude"

// Kind is what the login shell answered.
type Kind int

const (
	// Unasked is a login shell that couldn't be asked.
	Unasked Kind = iota
	// Link is a login shell that runs switchboard's claude link.
	Link
	// Other is a login shell that runs another claude.
	Other
	// None is a login shell that finds no claude.
	None
)

// Answer is which claude the login shell runs.
type Answer struct {
	Kind Kind
	// Claude is the other claude the shell runs, as it gives it: its path,
	// or an alias or function.
	Claude string
	// Why is why the shell couldn't be asked.
	Why string
}

// Which asks the login shell which claude it runs, and whether that's link,
// switchboard's claude link. The shell's started as a new terminal's is,
// login and interactive, in the environment one starts in, so the PATH
// setup runs with can't answer for it: HOME, USER, LOGNAME, SHELL, TMPDIR,
// TERM and the locale's variables from Environ, PATH as launchd sets it,
// and nothing else.
func (l Login) Which(ctx context.Context, link string) Answer {
	path := lookup(l.Environ, "SHELL")
	if path == "" {
		return Answer{Why: "$SHELL isn't set"}
	}
	limit := cmp.Or(l.Limit, defaultLimit)
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := l.Run(ctx, path, []string{"-l", "-i", "-c", question(path)}, loginEnv(l.Environ))
	if answer, ok := after(out); ok {
		return read(answer, link)
	}
	return Answer{Why: why(ctx, err, limit)}
}

// question is what the shell at path is asked: the sentinel, then which
// claude it runs.
func question(path string) string {
	which := "command -v claude"
	if sh, _ := Of(path); sh == Fish {
		which = "command --search claude"
	}
	return "printf '%s\\n' " + sentinel + "; " + which
}

// loginEnv is the environment a new terminal's shell starts in, taken from
// environ as Which says.
func loginEnv(environ []string) []string {
	env := []string{"PATH=" + launchdPath}
	for _, kv := range environ {
		if key, _, _ := strings.Cut(kv, "="); slices.Contains(carried, key) || strings.HasPrefix(key, "LC_") {
			env = append(env, kv)
		}
	}
	return env
}

// carried are the variables a new terminal's shell starts with that setup's
// own environment gives the login shell, with the locale's LC_ ones.
var carried = []string{"HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "TERM", "LANG"}

// lookup returns the value of the variable key in environ.
func lookup(environ []string, key string) string {
	for _, kv := range environ {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			return v
		}
	}
	return ""
}

// after returns the first line out holds after the sentinel's last line,
// trimmed, and whether the sentinel's there.
func after(out []byte) (string, bool) {
	text := strings.ReplaceAll(string(out), "\r", "")
	at := strings.LastIndex("\n"+text, "\n"+sentinel+"\n")
	if at < 0 {
		return "", false
	}
	rest := text[at+len(sentinel)+1:]
	for line := range strings.Lines(rest) {
		if line = strings.TrimSpace(line); line != "" {
			return line, true
		}
	}
	return "", true
}

// read is the shell's answer, answer, link being switchboard's claude link.
func read(answer, link string) Answer {
	switch {
	case answer == "":
		return Answer{Kind: None}
	case !filepath.IsAbs(answer):
		return Answer{Kind: Other, Claude: redact.Text(answer)}
	case sameFile(answer, link):
		return Answer{Kind: Link}
	}
	return Answer{Kind: Other, Claude: answer}
}

// sameFile reports whether a and b are the one file, neither's last link
// followed, or the same path.
func sameFile(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, err := os.Lstat(a)
	if err != nil {
		return false
	}
	bi, err := os.Lstat(b)
	return err == nil && os.SameFile(ai, bi)
}

// why is why the shell, run until ctx ended, given limit, and failing with
// err, gave no answer.
func why(ctx context.Context, err error, limit time.Duration) string {
	exit, exited := errors.AsType[exitError](err)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "it didn't answer within " + prose.Span(limit)
	case ctx.Err() != nil:
		return ctx.Err().Error()
	case exited:
		return fmt.Sprintf("it exited with status %d without answering", exit.ExitCode())
	case err != nil:
		return redact.Text(err.Error())
	}
	return "it exited without answering"
}
