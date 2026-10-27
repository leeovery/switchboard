package setuptoken

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/childenv"
)

// openFor bounds each program an Opener runs, which normally takes a moment.
const openFor = 10 * time.Second

// Opener opens a sign-in address in a browser on this machine, saying how.
type Opener interface {
	Open(ctx context.Context, address Address) (Opened, error)
}

// Runner runs the program named, as PATH finds it, with args, to its end, and
// returns what it printed.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Programs returns the Runner that runs programs for real, in no more of the
// environment getenv gives than they need, which can hold a token, as a
// Claude Code session's does.
func Programs(getenv func(string) string) Runner {
	env := childenv.Minimal(getenv)
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return runCommand(ctx, env, name, args...)
	}
}

// runCommand runs the program named with args and env, and returns what it
// printed.
func runCommand(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	return cmd.Output()
}

// Browser opens an address in the default browser as it is: signed in to
// whichever account it's signed in to.
type Browser struct {
	// Run runs open.
	Run Runner
}

// Open opens address in the default browser.
func (b Browser) Open(ctx context.Context, address Address) (Opened, error) {
	ctx, cancel := context.WithTimeout(ctx, openFor)
	defer cancel()
	if _, err := b.Run(ctx, "open", address.Reveal()); err != nil {
		return NotOpened, fmt.Errorf("open the sign-in address: %w", err)
	}
	return OpenedInBrowser, nil
}

// PrivateWindow opens an address in a private window of the default browser,
// which leaves the browser's own session alone, where its command line can be
// told to: the Chrome family's, Edge's and Firefox's. For any other, as
// Safari, it fails with a *NoPrivateWindowError.
type PrivateWindow struct {
	// Run runs osascript, which finds the default browser, and open.
	Run Runner
}

// NoPrivateWindowError is what PrivateWindow fails with for a default browser
// it can't open a private window of.
type NoPrivateWindowError struct {
	// Browser is the browser's name, as its application has it, such as
	// Safari.
	Browser string
}

func (e *NoPrivateWindowError) Error() string {
	return "switchboard can't open a private window of " + e.Browser
}

// privateWindows are the browsers whose private windows PrivateWindow opens,
// by how their application's name begins, as Google Chrome Canary's begins as
// Google Chrome's does, and the flag that opens one.
var privateWindows = []struct{ app, flag string }{
	{app: "Google Chrome", flag: "--incognito"},
	{app: "Chromium", flag: "--incognito"},
	{app: "Brave Browser", flag: "--incognito"},
	{app: "Vivaldi", flag: "--incognito"},
	{app: "Microsoft Edge", flag: "--inprivate"},
	{app: "Firefox", flag: "-private-window"},
}

// findDefaultBrowser is the JavaScript for Automation that prints the path of
// the application that opens https addresses, the default browser, or nothing
// where none does. It asks of an address of its own, so the sign-in address
// isn't on osascript's command line.
const findDefaultBrowser = `ObjC.import("AppKit");
const app = $.NSWorkspace.sharedWorkspace.URLForApplicationToOpenURL($.NSURL.URLWithString("https://claude.ai"));
app.isNil() ? "" : app.path.js;`

// errNoDefaultBrowser is what PrivateWindow fails with where no application
// opens https addresses.
var errNoDefaultBrowser = errors.New("there's no default browser")

// Open opens address in a private window of the default browser: a new one,
// as open -n starts it, given the flag that opens one, which a browser
// running already takes from it.
func (p PrivateWindow) Open(ctx context.Context, address Address) (Opened, error) {
	ctx, cancel := context.WithTimeout(ctx, openFor)
	defer cancel()
	app, err := p.defaultBrowser(ctx)
	if err != nil {
		return NotOpened, err
	}
	name := strings.TrimSuffix(filepath.Base(app), ".app")
	flag, ok := privateFlag(name)
	if !ok {
		return NotOpened, &NoPrivateWindowError{Browser: name}
	}
	if _, err := p.Run(ctx, "open", "-na", app, "--args", flag, address.Reveal()); err != nil {
		return NotOpened, fmt.Errorf("open the sign-in address in a private window of %s: %w", name, err)
	}
	return OpenedPrivately, nil
}

// defaultBrowser returns the path of the default browser's application.
func (p PrivateWindow) defaultBrowser(ctx context.Context) (string, error) {
	out, err := p.Run(ctx, "osascript", "-l", "JavaScript", "-e", findDefaultBrowser)
	if err != nil {
		return "", fmt.Errorf("find the default browser: %w", err)
	}
	app := strings.TrimSpace(string(out))
	if app == "" {
		return "", errNoDefaultBrowser
	}
	return app, nil
}

// privateFlag returns the flag that opens a private window of the browser
// named, false for one PrivateWindow can't open one of.
func privateFlag(name string) (string, bool) {
	for _, browser := range privateWindows {
		if strings.HasPrefix(name, browser.app) {
			return browser.flag, true
		}
	}
	return "", false
}
