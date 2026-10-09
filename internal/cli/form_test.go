package cli_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// printForm is where a data verb prints, and the flags that choose its form.
type printForm struct {
	name     string
	terminal bool
	args     []string
}

// run runs switchboard with args, then f's flags, its output a terminal
// where f says.
func (f printForm) run(t *testing.T, deps cli.Deps, args ...string) result {
	t.Helper()
	f.on(&deps)
	return run(t, deps, slices.Concat(args, f.args)...)
}

// on has commands run with deps take their output for a terminal where f
// says, and for none where it doesn't: every test's faked terminal is set
// here.
func (f printForm) on(deps *cli.Deps) {
	deps.Terminal = func(io.Writer) bool { return f.terminal }
}

var (
	onATerminal  = printForm{name: "on a terminal", terminal: true}
	offATerminal = printForm{name: "off a terminal"}
	// withJSON and withPretty ask for each form wherever stdout is.
	withJSON = []printForm{
		{name: "with --json, on a terminal", terminal: true, args: []string{"--json"}},
		{name: "with --json, off a terminal", args: []string{"--json"}},
	}
	withPretty = []printForm{
		{name: "with --pretty, on a terminal", terminal: true, args: []string{"--pretty"}},
		{name: "with --pretty, off a terminal", args: []string{"--pretty"}},
	}
)

// prettyForms are those that print a data verb's form for a person, and
// jsonForms those that print its JSON.
func prettyForms() []printForm { return append([]printForm{onATerminal}, withPretty...) }
func jsonForms() []printForm   { return append([]printForm{offATerminal}, withJSON...) }

func TestTheFormChosenIsLoggedWithWhy(t *testing.T) {
	tests := []struct {
		form printForm
		want []string
	}{
		{form: onATerminal, want: []string{"json=false", `because="stdout is a terminal"`}},
		{form: offATerminal, want: []string{"json=true", `because="stdout isn't a terminal"`}},
		{form: withJSON[0], want: []string{"json=true", "because=--json"}},
		{form: withPretty[1], want: []string{"json=false", "because=--pretty"}},
	}
	for _, tt := range tests {
		t.Run(tt.form.name, func(t *testing.T) {
			deps := testDeps(map[string]string{"SWITCHBOARD_LOG_LEVEL": "debug"}, t.TempDir())
			deps.Now = func() time.Time { return ledgerNow }
			configure(t, deps)

			if got := tt.form.run(t, deps, "requests"); got.code != 0 {
				t.Fatalf("switchboard requests %s = %+v, want exit status 0", strings.Join(tt.form.args, " "), got)
			}
			if log := readLog(t, deps, "cli.log"); !hasLine(log, append([]string{"level=DEBUG", `msg="chose the form to print"`}, tt.want...)...) {
				t.Errorf("cli.log reads\n%s\nwant the form chosen, and why: %q", log, tt.want)
			}
		})
	}
}

func TestTheDataVerbsRefuseJSONWithPretty(t *testing.T) {
	commands := [][]string{
		{"status"},
		{"status", "--session", "0b5c"},
		{"usage"},
		{"usage", "--watch"},
		{"requests"},
		{"history"},
	}
	for _, command := range commands {
		for _, where := range []printForm{onATerminal, offATerminal} {
			args := slices.Concat(command, []string{"--json", "--pretty"})
			t.Run(strings.Join(args, " ")+", "+where.name, func(t *testing.T) {
				deps, _ := ledgerDeps(t)
				deps.Watch = func(context.Context, watch.Config, io.Writer, []string) error {
					t.Error("the dashboard started")
					return nil
				}

				got := where.run(t, deps, args...)
				const want = "Error: --json prints JSON, so it takes no --pretty\n"
				// The usage a mistyped command line is answered with goes where
				// the command prints, which a test captures: switchboard's own
				// goes to stderr.
				if got.code != 1 || !strings.HasPrefix(got.stdout, "Usage:") || !strings.HasPrefix(got.stderr, want) {
					t.Errorf("switchboard %s, %s, = %+v, want exit status 1, nothing printed but the usage, and an error starting %q",
						strings.Join(args, " "), where.name, got, want)
				}
			})
		}
	}
}
