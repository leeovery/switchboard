// Package notify posts desktop notifications: the router's, and the
// dashboard's.
package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/childenv"
)

const (
	// title heads every notification.
	title = "Switchboard"
	// timeout bounds posting one, which normally takes a fraction of a second.
	timeout = 5 * time.Second
)

// Off posts nothing.
type Off struct{}

// Notify does nothing.
func (Off) Notify(string) error { return nil }

// Desktop posts to macOS's Notification Centre. It knows no other system's
// way, and elsewhere posts nothing.
type Desktop struct {
	goos string
	// env is the environment osascript runs in.
	env []string
	// run runs a command to its end. Tests replace it, so they never post.
	run func(ctx context.Context, env []string, name string, args ...string) error
}

// NewDesktop returns a Desktop for the system it runs on, whose osascript
// runs in no more of this process's environment than it needs: the router's
// holds every token.
func NewDesktop() Desktop {
	return Desktop{goos: runtime.GOOS, env: childenv.Minimal(os.Getenv), run: runCommand}
}

// Notify posts message under switchboard's name.
func (d Desktop) Notify(message string) error {
	if d.goos != "darwin" {
		return nil
	}
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
