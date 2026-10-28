package shell

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// The sentinels the shell's answers are printed between: startup files print
// banners and escape codes of their own first, which needn't end a line, and
// logout files and traps can print after.
const (
	startSentinel = "switchboard-asks-which-claude"
	endSentinel   = "switchboard-asked-which-claude"
)

// Kind is what the login shell's PATH finds for claude.
type Kind int

const (
	// Unasked is a login shell that couldn't be asked.
	Unasked Kind = iota
	// Link is a login shell whose PATH finds switchboard's claude link.
	Link
	// Other is a login shell whose PATH finds another claude.
	Other
	// None is a login shell whose PATH finds no claude.
	None
)

// Answer is which claude the login shell runs, and where it reads its
// startup file.
type Answer struct {
	// Kind is what the shell's PATH finds for claude.
	Kind Kind
	// Claude is the other claude the shell's PATH finds.
	Claude string
	// Front is an alias or function named claude that stands in front of
	// PATH's.
	Front Front
	// ConfigDir is where the shell reads its startup file from, as it says
	// itself: zsh's $ZDOTDIR, else the home, and fish's configuration
	// directory. It's empty for bash, which isn't asked, and where it isn't
	// absolute.
	ConfigDir string
	// Why is why the shell couldn't be asked.
	Why string
}

// Through reports whether claude, typed in a new terminal, goes through
// switchboard, as the answer says: where PATH finds the link and what's in
// front of it runs claude by name, or may; or where an alias in front runs
// the link by its path.
func (a Answer) Through() bool {
	switch a.Front.Runs {
	case TheLink:
		return true
	case APath, Else:
		return false
	}
	return a.Kind == Link
}

// Which asks the login shell which claude it runs, whether PATH's is link,
// switchboard's claude link, and where it reads its startup file. The
// shell's started as a new terminal's is, login and interactive, in the
// environment one starts in, so the PATH setup runs with can't answer for
// it: HOME, USER, LOGNAME, SHELL, TMPDIR, TERM, TMUX and the locale's
// variables from Environ, PATH as launchd sets it, and nothing else. Only
// zsh, bash and fish are asked, the shells switchboard knows the words of.
func (l Login) Which(ctx context.Context, link string) Answer {
	path := lookup(l.Environ, "SHELL")
	if path == "" {
		return Answer{Why: "$SHELL isn't set"}
	}
	sh, known := Of(path)
	if !known {
		return Answer{Why: "switchboard asks zsh, bash and fish alone"}
	}
	limit := cmp.Or(l.Limit, defaultLimit)
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := l.Run(ctx, path, []string{"-l", "-i", "-c", question(sh)}, loginEnv(l.Environ))
	if said, ok := between(out); ok {
		return read(said, lookup(l.Environ, "HOME"), link)
	}
	return Answer{Why: why(ctx, err, limit)}
}

// question is what sh is asked, between the sentinels, a line each: what
// claude is, which claude PATH finds, and where the shell reads its startup
// file.
func question(sh Shell) string {
	is, found, config := `"$(command -v claude)"`, `"$(type -P claude)"`, `""`
	switch sh {
	case Zsh:
		found, config = `"$(whence -p claude)"`, `"${ZDOTDIR:-$HOME}"`
	case Fish:
		is = "(functions -q claude; and echo function claude; or command --search claude)"
		found, config = "(command --search claude)", "$__fish_config_dir"
	}
	return strings.Join([]string{
		`printf '%s\n' ` + startSentinel,
		`printf 'is:%s\n' ` + is,
		`printf 'path:%s\n' ` + found,
		`printf 'config:%s\n' ` + config,
		`printf '%s\n' ` + endSentinel,
	}, "; ")
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
// own environment gives the login shell, with the locale's LC_ ones. TMUX is
// among them: a startup file that starts tmux where it's unset would start
// it in the shell's place, and the shells claude runs in inside tmux have it.
// ZDOTDIR isn't: an editor's terminal points it at a directory of its own,
// which zsh in a new terminal never reads.
var carried = []string{"HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "TERM", "TMUX", "LANG"}

// lookup returns the value of the variable key in environ.
func lookup(environ []string, key string) string {
	for _, kv := range environ {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			return v
		}
	}
	return ""
}

// answers are what the shell said between the sentinels, by name.
type answers map[string]string

// escapes are the terminal's escape sequences a startup file's hooks print,
// even inside the answers, as a prompt's marks before each command: OSC,
// ended by BEL or ST, CSI, and the rest, of one character.
var escapes = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)

// between returns the shell's answers, from out, its escape sequences
// passed over: what's between the last start sentinel, found anywhere, and
// the end sentinel after it; and whether both are there.
func between(out []byte) (answers, bool) {
	text := escapes.ReplaceAllString(strings.ReplaceAll(string(out), "\r", ""), "")
	at := strings.LastIndex(text, startSentinel+"\n")
	if at < 0 {
		return nil, false
	}
	text, _, ended := strings.Cut(text[at+len(startSentinel)+1:], endSentinel+"\n")
	if !ended {
		return nil, false
	}
	said := make(answers)
	for line := range strings.Lines(text) {
		name, value, ok := strings.Cut(strings.TrimSuffix(line, "\n"), ":")
		if _, given := said[name]; ok && !given {
			said[name] = strings.TrimSpace(value)
		}
	}
	return said, true
}

// read is the shell's answer, from what it said, home being the user's home
// and link switchboard's claude link. A path for PATH's claude that isn't
// one is no answer.
func read(said answers, home, link string) Answer {
	found := said["path"]
	if found != "" && !filepath.IsAbs(found) {
		return Answer{Why: fmt.Sprintf("it answered %q for the claude on its PATH, which isn't a path", redact.Text(found))}
	}
	a := Answer{Front: readFront(said["is"], home, link)}
	if dir := said["config"]; filepath.IsAbs(dir) {
		a.ConfigDir = dir
	}
	switch {
	case found == "":
		a.Kind = None
	case sameFile(found, link):
		a.Kind = Link
	default:
		a.Kind, a.Claude = Other, redact.Text(found)
	}
	return a
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

// mayNeedATerminal is why a shell may exit without answering, as one does
// whose startup file runs tmux in its place, which needs a terminal.
const mayNeedATerminal = ": a startup file may start something that needs a terminal, such as tmux"

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
		return fmt.Sprintf("it exited with status %d without answering%s", exit.ExitCode(), mayNeedATerminal)
	case err != nil:
		return redact.Text(err.Error())
	}
	return "it exited without answering" + mayNeedATerminal
}
