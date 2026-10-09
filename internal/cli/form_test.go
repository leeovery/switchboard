package cli_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

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
	deps.Terminal = func(io.Writer) bool { return f.terminal }
	return run(t, deps, slices.Concat(args, f.args)...)
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
		for _, terminal := range []bool{true, false} {
			args := slices.Concat(command, []string{"--json", "--pretty"})
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				deps, _ := ledgerDeps(t)
				deps.Watch = func(context.Context, watch.Config, io.Writer, []string) error {
					t.Error("the dashboard started")
					return nil
				}

				got := printForm{terminal: terminal}.run(t, deps, args...)
				const want = "Error: --json prints JSON, so it takes no --pretty\n"
				// The usage a mistyped command line is answered with goes where
				// the command prints, which a test captures: switchboard's own
				// goes to stderr.
				if got.code != 1 || !strings.HasPrefix(got.stdout, "Usage:") || !strings.HasPrefix(got.stderr, want) {
					t.Errorf("switchboard %s, its output a terminal: %v, = %+v, want exit status 1, nothing printed but the usage, and an error starting %q",
						strings.Join(args, " "), terminal, got, want)
				}
			})
		}
	}
}
