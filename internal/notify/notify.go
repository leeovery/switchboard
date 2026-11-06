// Package notify posts desktop notifications: the router's, and the
// dashboard's.
package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/childenv"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/redact"
)

const (
	// title heads every notification.
	title = "Switchboard"
	// timeout bounds posting one, which normally takes a fraction of a second.
	timeout = 5 * time.Second
	// notAllowed is the app's exit status for a post macOS doesn't allow.
	notAllowed = 2
	// reasonSize is how much of the end of what a program writes to its
	// errors is kept, for its last line to say why it failed.
	reasonSize = 512
	// waitDelay is how long a program, once killed or exited, gets for its
	// errors to be read to their end: something it started can hold them
	// open past every bound.
	waitDelay = time.Second
)

// ErrNotAllowed is what a post through switchboard's app fails with when
// macOS doesn't allow its notifications, as when they're turned off in
// System Settings → Notifications → Switchboard.
var ErrNotAllowed = errors.New("macOS won't let Switchboard post notifications")

var logger = logs.For("notify")

// Off posts nothing.
type Off struct{}

// Notify does nothing.
func (Off) Notify(context.Context, string) error { return nil }

// Prepare does nothing.
func (Off) Prepare(context.Context) {}

// Uninstall does nothing.
func (Off) Uninstall(context.Context) error { return nil }

// Desktop posts to macOS's Notification Centre: through switchboard's own
// app where the build embeds one, and it could be written out, else through
// osascript. It knows no other system's way, and elsewhere posts nothing.
type Desktop struct {
	goos string
	// env is the environment the programs it runs run in.
	env []string
	// run runs a program to its end. Tests replace it, so they never post.
	run func(ctx context.Context, env []string, name string, args ...string) error
	// appPath returns where the app is written out.
	appPath func() (string, error)
	// lockWait is how long it waits for another switchboard writing the app
	// out, holding the lock beside it.
	lockWait time.Duration
	// app is the app embedded, nil where there's none.
	app *app
}

// NewDesktop returns a Desktop for the system it runs on, whose programs run
// in no more of the environment getenv reads than they need, which can hold
// a token, as a Claude Code session's does, and which writes the app embedded
// where that environment keeps switchboard's data, as config.AppPath finds
// it.
func NewDesktop(getenv func(string) string, homeDir func() (string, error)) Desktop {
	return Desktop{
		goos:     runtime.GOOS,
		env:      childenv.Minimal(getenv),
		run:      runCommand,
		appPath:  func() (string, error) { return config.AppPath(getenv, homeDir) },
		lockWait: lockWait,
		app:      newApp(embedded),
	}
}

// Notify posts message under switchboard's name, readying the app first
// where it hasn't been yet: once a process, as ready says. Where ctx ends
// before the app is readied, it posts nothing, returning ctx's error, but
// once the app's readied, a post takes its own time, whatever ctx says, so
// the router's last notifications, posted as its context ends, go out.
// Through the app, a post macOS doesn't allow fails with ErrNotAllowed.
func (d Desktop) Notify(ctx context.Context, message string) error {
	if d.goos != "darwin" {
		return nil
	}
	exe, err := d.ready(ctx)
	switch {
	case err != nil:
		return fmt.Errorf("post notification: stopped waiting for its app to be readied: %w", err)
	case exe != "":
		return d.post(ctx, exe, message)
	}
	return d.script(ctx, message)
}

// Prepare readies the app, where one is embedded, as the first post would,
// so the question whether it may post comes as the router starts, until ctx
// ends.
func (d Desktop) Prepare(ctx context.Context) {
	if d.goos == "darwin" {
		_, _ = d.ready(ctx)
	}
}

// post posts message through the app's executable, exe.
func (d Desktop) post(ctx context.Context, exe, message string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	err := d.run(ctx, d.env, exe, "--post", message)
	if notAllowedBy(err) {
		err = ErrNotAllowed
	}
	if err != nil {
		return fmt.Errorf("post notification: %w", err)
	}
	return nil
}

// notAllowedBy reports whether err is the app exiting as macOS doesn't allow
// its notifications.
func notAllowedBy(err error) bool {
	var exit interface{ ExitCode() int }
	return errors.As(err, &exit) && exit.ExitCode() == notAllowed
}

// script posts message through osascript.
func (d Desktop) script(ctx context.Context, message string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	if err := d.run(ctx, d.env, "osascript", "-e", displayNotification(message)); err != nil {
		return fmt.Errorf("post notification: %w", err)
	}
	return nil
}

// displayNotification is the AppleScript that shows message under the title.
func displayNotification(message string) string {
	return "display notification " + quote(message) + " with title " + quote(title)
}

// quote makes s an AppleScript string literal: in double quotes, with its
// backslashes and double quotes escaped.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func runCommand(ctx context.Context, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.WaitDelay = waitDelay
	var stderr tail
	cmd.Stderr = &stderr
	return withReason(cmd.Run(), stderr.b)
}

// withReason is err, a program failing, saying why as the last line it wrote
// to its errors does, hiding anything token-shaped: err itself where it
// wrote none.
func withReason(err error, stderr []byte) error {
	if err == nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return fmt.Errorf("%w: %s", err, redact.Text(last))
	}
	return err
}

// tail keeps the last reasonSize bytes written to it, less what's left of a
// character cut in two at their start.
type tail struct {
	b []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if extra := len(t.b) - reasonSize; extra > 0 {
		t.b = bytes.Clone(fromRuneStart(t.b[extra:]))
	}
	return len(p), nil
}

// fromRuneStart is b from the first of its bytes that starts a character,
// past the rest of one cut in two at its start: b itself where none of its
// first few does, as what's not UTF-8 isn't cut short.
func fromRuneStart(b []byte) []byte {
	for i := range min(len(b), utf8.UTFMax) {
		if utf8.RuneStart(b[i]) {
			return b[i:]
		}
	}
	return b
}
