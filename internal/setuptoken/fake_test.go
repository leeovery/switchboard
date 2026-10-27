//go:build darwin

package setuptoken_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/setuptoken"
)

// fakeSetupToken is claude, standing in for claude setup-token as Claude Code
// 2.1.311 draws it. It notes its process id, its arguments, its environment,
// its terminal, whether that's its controlling terminal, and its size, and
// the id of a process it starts and leaves running, deaf to the hang-up its
// exit sends, then draws "Opening
// browser to sign in…", runs $BROWSER with FAKE_ADDRESS, draws the manual
// address it would have the person paste, as a link, and waits for the test
// to approve the sign-in, as the browser's callback does, by writing
// FAKE_DIR/approved. Then it draws the token, FAKE_TOKEN, and exits
// FAKE_EXIT_AFTER seconds later, with FAKE_STATUS, 0 where it's unset. It
// redraws each frame over the last, as Ink does. FAKE_MODE has it do
// otherwise:
//
//	stall        nothing at all, waiting
//	on-hold      exit 1 at once, saying the account's on hold
//	refused      exit 1 once it has run $BROWSER, and FAKE_DIR/opened is
//	             written, saying the account's on hold
//	no-browser   no running of $BROWSER, waiting
//	oauth-error  an OAuth error where the token would be, waiting at it
//	broken       the token broken across two rows
//	slow         the token drawn in two pieces, FAKE_PAUSE seconds apart
//	in-place     the token's frame drawn over the last in place, as a
//	             renderer drawing what's changed does, cut short in the token,
//	             waiting
//	clears       the token's frame and what erases it drawn in one write
//	last         the token drawn last, nothing after it
const fakeSetupToken = `#!/bin/sh
d=$FAKE_DIR
note() { printf '%s\n' "$2" > "$d/$1.tmp" && /bin/mv "$d/$1.tmp" "$d/$1"; }
note args "$*"
env > "$d/env"
note tty "$(/usr/bin/tty)"
note controlling "$( (: < /dev/tty) 2>/dev/null && echo yes)"
note size "$(/bin/stty size)"
(trap '' HUP; exec /bin/sleep 60) &
note child "$!"
note pid "$$"

shown=0
frame() {
	[ "$shown" -gt 0 ] && printf '\033[%dA\033[J' "$shown"
	printf '%b' "$1"
	shown=$(( $(printf '%b' "$1" | /usr/bin/wc -l) ))
}
over() {
	printf '\033[%dA' "$shown"
	printf '%b' "$1" | while IFS= read -r row || [ -n "$row" ]; do printf '\033[2K%s\n' "$row"; done
}

[ "$FAKE_MODE" = stall ] && exec /bin/sleep 60
frame '\033[?25l \033[2mOpening browser to sign in…\033[22m\n'
if [ "$FAKE_MODE" = on-hold ]; then
	frame ' \033[31mThis account is on hold.\033[39m\n'
	exit 1
fi
[ "$FAKE_MODE" = no-browser ] && exec /bin/sleep 60
"$BROWSER" "$FAKE_ADDRESS"
if [ "$FAKE_MODE" = refused ]; then
	while [ ! -e "$d/opened" ]; do /bin/sleep 0.05; done
	frame ' \033[31mThis account is on hold.\033[39m\n'
	exit 1
fi
frame ' Opening browser to sign in…\n\n \033[2mBrowser didn'"'"'t open? Use the url below to sign in\033[22m\n\n \033]8;;'"$FAKE_MANUAL"'\a\033[2m'"$FAKE_MANUAL"'\033[22m\033]8;;\a\n\n Paste code here if prompted > \n'
while [ ! -e "$d/approved" ]; do /bin/sleep 0.05; done

if [ "$FAKE_MODE" = oauth-error ]; then
	frame ' \033[31mOAuth error: Request failed with status code 400\033[39m\n\n \033[35mPress \033[1mEnter\033[22m to retry.\033[39m\n'
	read -r _
	exec /bin/sleep 60
fi
made=' \033[32m✓ Long-lived authentication token created successfully!\033[39m\n\n Your OAuth token (valid for 1 year):\n\n'
after='\n\n \033[2mStore this token securely. You won'"'"'t be able to see it again.\033[22m\n\n \033[2mUse this token by setting: export CLAUDE_CODE_OAUTH_TOKEN=<token>\033[22m\n'
first=$(printf '%s' "$FAKE_TOKEN" | /usr/bin/cut -c1-40)
rest=$(printf '%s' "$FAKE_TOKEN" | /usr/bin/cut -c41-)
case $FAKE_MODE in
broken) frame "$made \\033[33m$first\n $rest\\033[39m$after" ;;
slow)
	printf '%b' "$made \\033[33m$first"
	/bin/sleep "$FAKE_PAUSE"
	printf '%b' "$rest\\033[39m$after"
	;;
in-place)
	over "$made \\033[33m$first"
	exec /bin/sleep 60
	;;
clears)
	drawn="$made \\033[33m$FAKE_TOKEN\\033[39m$after"
	{
		printf '\033[%dA\033[J' "$shown"
		printf '%b' "$drawn"
		printf '\033[%dA\033[J' $(( $(printf '%b' "$drawn" | /usr/bin/wc -l) ))
	} > "$d/frame"
	/bin/cat "$d/frame"
	;;
last) frame "$made \\033[33m$FAKE_TOKEN\\033[39m\n" ;;
*) frame "$made \\033[33m$FAKE_TOKEN\\033[39m$after" ;;
esac
/bin/sleep "${FAKE_EXIT_AFTER:-0.5}"
exit "${FAKE_STATUS:-0}"
`

// The sign-in addresses the fake hands $BROWSER and shows, and the token it
// makes: placeholders, shaped as setup-token's are.
var (
	fakeAddress = "https://claude.example/oauth/authorize?code=true&redirect_uri=http%3A%2F%2Flocalhost%3A54545%2Fcallback&code_challenge=test-challenge&state=test-state-work"
	fakeManual  = "https://claude.example/oauth/authorize?code=true&redirect_uri=https%3A%2F%2Fplatform.claude.com%2Foauth%2Fcode%2Fcallback&code_challenge=test-challenge&state=test-state-work"
	fakeToken   = "sk-ant-oat01-test-token-work-" + strings.Repeat("0123456789", 8)
)

// fake is a fake setup-token, and a Maker that finds it as the real claude.
type fake struct {
	// dir is where the fake notes what it's run with, and is approved.
	dir string
	// shims is the temporary directory, where Make makes its shim.
	shims string
	maker setuptoken.Maker
}

// newFake returns a fake setup-token doing as mode says, its environment the
// placeholders a user's might hold besides env, each "key=value".
func newFake(t *testing.T, mode string, env ...string) fake {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fakeSetupToken), 0o700); err != nil {
		t.Fatal(err)
	}
	f := fake{dir: filepath.Join(root, "fake"), shims: filepath.Join(root, "tmp")}
	for _, dir := range []string{f.dir, f.shims} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	switchboard := claudetest.Program(t, filepath.Join(root, "switchboard", "switchboard"))
	f.maker = setuptoken.Maker{
		Environ: with([]string{
			"PATH=" + bin + ":/usr/bin:/bin",
			"HOME=" + filepath.Join(root, "home"),
			"FAKE_DIR=" + f.dir,
			"FAKE_MODE=" + mode,
			"FAKE_ADDRESS=" + fakeAddress,
			"FAKE_MANUAL=" + fakeManual,
			"FAKE_TOKEN=" + fakeToken,
			"FAKE_EXIT_AFTER=0",
			"FAKE_PAUSE=0.3",
		}, env...),
		Executable: func() (string, error) { return switchboard, nil },
	}
	t.Setenv("TMPDIR", f.shims)
	return f
}

// with returns env with each of vars, "key=value", in place of any value its
// key had.
func with(env []string, vars ...string) []string {
	env = slices.Clone(env)
	for _, variable := range vars {
		key, _, _ := strings.Cut(variable, "=")
		env = append(slices.DeleteFunc(env, func(v string) bool { return strings.HasPrefix(v, key+"=") }), variable)
	}
	return env
}

// approve approves the sign-in, as the browser's callback does.
func (f fake) approve(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, "approved"), nil, 0o600); err != nil {
		t.Error(err)
	}
}

// noted returns what the fake noted as name, waiting for it to be noted: ""
// should it not be within a minute.
func (f fake) noted(t *testing.T, name string) string {
	t.Helper()
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if data, err := os.ReadFile(filepath.Join(f.dir, name)); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	t.Errorf("the fake setup-token never noted its %s", name)
	return ""
}

// ran reports whether the fake setup-token ran at all.
func (f fake) ran() bool {
	_, err := os.Stat(filepath.Join(f.dir, "pid"))
	return err == nil
}

// opener is an Opener that opens nothing, noting each address it's asked to
// open, and answering as it's set to.
type opener struct {
	opened setuptoken.Opened
	err    error
	asked  []string
}

func (o *opener) Open(_ context.Context, address setuptoken.Address) (setuptoken.Opened, error) {
	o.asked = append(o.asked, address.Reveal())
	return o.opened, o.err
}

// approving returns a Report that notes each event in events, and approves
// the sign-in once Make waits on it, as the person does in the browser.
func approving(t *testing.T, f fake, events *[]setuptoken.Event) func(setuptoken.Event) {
	return func(e setuptoken.Event) {
		*events = append(*events, e)
		if e.Step == setuptoken.StepWaiting {
			f.approve(t)
		}
	}
}

// steps returns the steps of events, in order.
func steps(events []setuptoken.Event) []setuptoken.Step {
	var steps []setuptoken.Step
	for _, e := range events {
		steps = append(steps, e.Step)
	}
	return steps
}

// leaks is what a test had open as Make began, to check nothing Make started
// outlives it.
type leaks struct {
	files, goroutines int
}

// watchLeaks notes what the test has open.
func watchLeaks(t *testing.T) leaks {
	t.Helper()
	// The runtime opens what it polls files with the first time it polls one,
	// and keeps it: a pipe has it opened before the count.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = r.Close(), w.Close()
	return leaks{files: openFiles(t), goroutines: runtime.NumGoroutine()}
}

// check fails the test where Make left anything running or open: the fake
// setup-token, the process it started, a file, such as its terminal, a
// goroutine, or its shim.
func (l leaks) check(t *testing.T, f fake) {
	t.Helper()
	if f.ran() {
		for _, name := range []string{"pid", "child"} {
			if pid, err := strconv.Atoi(f.noted(t, name)); err != nil || !gone(pid) {
				t.Errorf("the fake setup-token's %s, %d, outlived Make (%v)", name, pid, err)
			}
		}
	}
	if got := openFiles(t); got != l.files {
		t.Errorf("Make left %d files open, want none", got-l.files)
	}
	if !settles(func() bool { return runtime.NumGoroutine() <= l.goroutines }) {
		t.Errorf("Make left %d goroutines running, want none", runtime.NumGoroutine()-l.goroutines)
	}
	if left, err := os.ReadDir(f.shims); err != nil || len(left) > 0 {
		t.Errorf("Make left %d in the temporary directory (%v), want its shim removed", len(left), err)
	}
}

// openFiles counts the files this process has open.
func openFiles(t *testing.T) int {
	t.Helper()
	files, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

// gone reports whether the process pid is gone, waiting a while for it to go,
// and to be reaped, as one whose parent went before it is.
func gone(pid int) bool {
	return settles(func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) })
}

// settles reports whether done comes to hold within ten seconds.
func settles(done func() bool) bool {
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if done() {
			return true
		}
	}
	return done()
}
