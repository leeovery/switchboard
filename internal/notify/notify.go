// Package notify posts desktop notifications: the router's, and the
// dashboard's.
package notify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/childenv"
	"github.com/leeovery/switchboard/internal/logs"
)

const (
	// title heads every notification.
	title = "Switchboard"
	// timeout bounds posting one, which normally takes a fraction of a second.
	timeout = 5 * time.Second
	// notAllowed is the app's exit status for a post macOS doesn't allow.
	notAllowed = 2
)

// ErrNotAllowed is what a post through switchboard's app fails with when
// macOS doesn't allow its notifications, as when they're turned off in
// System Settings → Notifications → Switchboard.
var ErrNotAllowed = errors.New("macOS won't let Switchboard post notifications")

var logger = logs.For("notify")

// Off posts nothing.
type Off struct{}

// Notify does nothing.
func (Off) Notify(string) error { return nil }

// Prepare does nothing.
func (Off) Prepare() {}

// Authorize does nothing.
func (Off) Authorize() {}

// Desktop posts to macOS's Notification Centre: through switchboard's own
// app where the build embeds one, and it could be written out, else through
// osascript. It knows no other system's way, and elsewhere posts nothing.
type Desktop struct {
	goos string
	// env is the environment the programs it runs run in.
	env []string
	// run runs a program to its end, and start starts one without waiting
	// for it. Tests replace them, so they never post.
	run   func(ctx context.Context, env []string, name string, args ...string) error
	start func(env []string, name string, args ...string) error
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
		goos:  runtime.GOOS,
		env:   childenv.Minimal(getenv),
		run:   runCommand,
		start: startCommand,
		app:   newApp(embedded, getenv, homeDir),
	}
}

// Notify posts message under switchboard's name, writing out the app first
// where it hasn't been yet: once a process, as ready says. Through the app,
// a post macOS doesn't allow fails with ErrNotAllowed.
func (d Desktop) Notify(message string) error {
	if d.goos != "darwin" {
		return nil
	}
	if exe, ok := d.ready(); ok {
		return d.post(exe, message)
	}
	return d.script(message)
}

// Prepare writes out the app, where one is embedded, as the first post would,
// so none waits on it: the router does as it starts.
func (d Desktop) Prepare() {
	if d.goos == "darwin" {
		d.ready()
	}
}

// Authorize writes out the app, where one is embedded, and has it ask macOS
// whether it may post, which asks the user the first time, without waiting
// for the answer.
func (d Desktop) Authorize() {
	if d.goos != "darwin" {
		return
	}
	exe, ok := d.ready()
	if !ok {
		return
	}
	if err := d.start(d.env, exe, "--authorize"); err != nil {
		logger.Warn("can't ask macOS whether Switchboard may post notifications", "error", err)
	}
}

// post posts message through the app's executable, exe.
func (d Desktop) post(exe, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := d.run(ctx, d.env, exe, "--post", message)
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) && exit.ExitCode() == notAllowed {
		err = ErrNotAllowed
	}
	if err != nil {
		return fmt.Errorf("post notification: %w", err)
	}
	return nil
}

// script posts message through osascript.
func (d Desktop) script(message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
	return cmd.Run()
}

// startCommand starts a program and lets it go: it runs on past this
// process, which never waits for it.
func startCommand(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
