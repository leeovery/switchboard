package setuptoken

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// runner is a Runner that runs nothing, noting each run, a line each as
// "path args…", and answering each program as it's set to: what it printed,
// or that it failed.
type runner struct {
	printed map[program]string
	failed  map[program]error
	ran     []string
}

func (r *runner) run(_ context.Context, p program, args ...string) ([]byte, error) {
	r.ran = append(r.ran, strings.Join(append([]string{string(p)}, args...), " "))
	return []byte(r.printed[p]), r.failed[p]
}

// address is the sign-in address the openers are given.
func address(t *testing.T) Address {
	t.Helper()
	a, ok := newAddress(signIn)
	if !ok {
		t.Fatalf("newAddress(%q) refused it", signIn)
	}
	return a
}

func TestBrowserOpensTheAddressAsItIs(t *testing.T) {
	r := &runner{}

	opened, err := Browser{Run: r.run}.Open(t.Context(), address(t))

	if opened != OpenedInBrowser || err != nil {
		t.Errorf("Open() = %v, %v; want it opened in the browser", opened, err)
	}
	if want := []string{"/usr/bin/open " + signIn}; !slices.Equal(r.ran, want) {
		t.Errorf("Open() ran %q, want %q", r.ran, want)
	}
}

func TestBrowserSaysWhereOpenFails(t *testing.T) {
	failed := errors.New("exit status 1")
	r := &runner{failed: map[program]error{openProgram: failed}}

	opened, err := Browser{Run: r.run}.Open(t.Context(), address(t))

	if opened != NotOpened || !errors.Is(err, failed) {
		t.Errorf("Open() = %v, %v; want it not opened, with open's error", opened, err)
	}
}

func TestPrivateWindowOpensTheDefaultBrowsersPrivateWindow(t *testing.T) {
	tests := []struct {
		name string
		app  string
		flag string
	}{
		{name: "Google Chrome", app: "/Applications/Google Chrome.app", flag: "--incognito"},
		{name: "a channel of Google Chrome", app: "/Applications/Google Chrome Canary.app", flag: "--incognito"},
		{name: "Chromium", app: "/Applications/Chromium.app", flag: "--incognito"},
		{name: "Brave", app: "/Applications/Brave Browser.app", flag: "--incognito"},
		{name: "Vivaldi", app: "/Applications/Vivaldi.app", flag: "--incognito"},
		{name: "Microsoft Edge", app: "/Applications/Microsoft Edge.app", flag: "--inprivate"},
		{name: "Firefox", app: "/Applications/Firefox.app", flag: "-private-window"},
		{name: "a channel of Firefox", app: "/Applications/Firefox Developer Edition.app", flag: "-private-window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runner{printed: map[program]string{osascriptProgram: tt.app + "\n"}}

			opened, err := PrivateWindow{Run: r.run}.Open(t.Context(), address(t))

			if opened != OpenedPrivately || err != nil {
				t.Errorf("Open() = %v, %v; want it opened privately", opened, err)
			}
			want := []string{"/usr/bin/osascript -l JavaScript -e " + findDefaultBrowser, "/usr/bin/open -na " + tt.app + " --args " + tt.flag + " " + signIn}
			if !slices.Equal(r.ran, want) {
				t.Errorf("Open() ran %q, want %q", r.ran, want)
			}
		})
	}
}

func TestPrivateWindowOpensNothingOfABrowserItCantOpenOneOf(t *testing.T) {
	tests := []struct {
		name string
		app  string
		want string
	}{
		{name: "Safari", app: "/Applications/Safari.app", want: "Safari"},
		{name: "Safari, as macOS keeps it", app: "/System/Volumes/Preboot/Cryptexes/App/System/Applications/Safari.app", want: "Safari"},
		{name: "Arc", app: "/Applications/Arc.app", want: "Arc"},
		{name: "one unknown", app: "/Applications/Some Browser.app", want: "Some Browser"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runner{printed: map[program]string{osascriptProgram: tt.app + "\n"}}

			opened, err := PrivateWindow{Run: r.run}.Open(t.Context(), address(t))

			refused, ok := errors.AsType[*NoPrivateWindowError](err)
			if opened != NotOpened || !ok || refused.Browser != tt.want {
				t.Errorf("Open() = %v, %v; want it not opened, as %s has no private window it can open", opened, err, tt.want)
			}
			if len(r.ran) != 1 {
				t.Errorf("Open() ran %q, want osascript alone", r.ran)
			}
		})
	}
}

func TestPrivateWindowSaysWhereItCantOpenOne(t *testing.T) {
	failed := errors.New("exit status 1")
	tests := []struct {
		name   string
		runner *runner
		want   error
	}{
		{name: "osascript failing", runner: &runner{failed: map[program]error{osascriptProgram: failed}}, want: failed},
		{name: "no default browser", runner: &runner{printed: map[program]string{osascriptProgram: "\n"}}, want: errNoDefaultBrowser},
		{name: "open failing", runner: &runner{printed: map[program]string{osascriptProgram: "/Applications/Firefox.app"}, failed: map[program]error{openProgram: failed}}, want: failed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opened, err := PrivateWindow{Run: tt.runner.run}.Open(t.Context(), address(t))

			if opened != NotOpened || !errors.Is(err, tt.want) {
				t.Errorf("Open() = %v, %v; want it not opened, with %v", opened, err, tt.want)
			}
			if strings.Contains(err.Error(), "test-state-work") {
				t.Errorf("Open() = %v, want the address kept out of its error", err)
			}
		})
	}
}

func TestProgramsRunNothingButOpenAndOsascript(t *testing.T) {
	run := Programs(func(string) string { return "" })

	out, err := run(t.Context(), program("/bin/sh"), "-c", "echo ran")

	if err == nil || len(out) > 0 {
		t.Errorf("running /bin/sh = %q, %v; want it refused, as no Opener runs it", out, err)
	}
}

// open names the address it couldn't open on its standard error, and an
// *exec.ExitError keeps what it's given of that, for errors.As and %#v to find.
func TestOutputKeepsNothingOfWhatAProgramPrintsAsErrors(t *testing.T) {
	signIn := "https://claude.example/oauth/authorize?state=test-state-work"
	cmd := exec.Command("/bin/sh", "-c", `echo printed; echo "couldn't open $0" >&2; exit 3`, signIn)

	out, err := output(cmd)

	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok || exit.ExitCode() != 3 {
		t.Fatalf("output() = %v, want how the program exited, 3", err)
	}
	if string(out) != "printed\n" {
		t.Errorf("output() printed %q, want what the program printed", out)
	}
	if len(exit.Stderr) > 0 || strings.Contains(fmt.Sprintf("%#v %+v", exit, exit), "claude.example") {
		t.Errorf("output() = %#v, want nothing of what the program printed as errors", exit)
	}
}

func TestOpenersBoundWhatTheyRun(t *testing.T) {
	var deadlines []bool
	run := func(ctx context.Context, p program, _ ...string) ([]byte, error) {
		_, ok := ctx.Deadline()
		deadlines = append(deadlines, ok)
		if p == osascriptProgram {
			return []byte("/Applications/Firefox.app"), nil
		}
		return nil, nil
	}

	_, _ = Browser{Run: run}.Open(t.Context(), address(t))
	_, _ = PrivateWindow{Run: run}.Open(t.Context(), address(t))

	if want := []bool{true, true, true}; !slices.Equal(deadlines, want) {
		t.Errorf("the openers ran programs with deadlines %v, want each bounded", deadlines)
	}
}
