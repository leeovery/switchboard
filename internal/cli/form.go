package cli

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

// jsonUsage is --json's help for a verb whose JSON is what it prints off a
// terminal.
const jsonUsage = "print JSON, as off a terminal"

// formFlags are a data verb's --json and --pretty, which choose the form it
// prints in wherever stdout is: JSON, or the form for a person.
type formFlags struct {
	json, pretty bool
}

// add registers --json and --pretty on cmd, and has its argument check
// refuse the two together before it checks the rest.
func (f *formFlags) add(cmd *cobra.Command) {
	f.addWithJSON(cmd, jsonUsage)
}

// addWithJSON is add, with --json's help given, for a verb whose JSON isn't
// always what it prints off a terminal.
func (f *formFlags) addWithJSON(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolVar(&f.json, "json", false, usage)
	cmd.Flags().BoolVar(&f.pretty, "pretty", false, "print the form for a person, as on a terminal")
	next := cmd.Args
	if next == nil {
		// Cobra lets a command with no check, and no commands of its own,
		// take any arguments.
		next = cobra.ArbitraryArgs
	}
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if f.json && f.pretty {
			return errors.New("--json prints JSON, so it takes no --pretty")
		}
		return next(cmd, args)
	}
}

// printsJSON reports whether a data verb prints JSON to out, as f says: with
// --json, or off a terminal unless --pretty.
func (a *app) printsJSON(f formFlags, out io.Writer) bool {
	var asJSON bool
	var why string
	switch {
	case f.json:
		asJSON, why = true, "--json"
	case f.pretty:
		why = "--pretty"
	case a.Terminal(out):
		why = "stdout is a terminal"
	default:
		asJSON, why = true, "stdout isn't a terminal"
	}
	logger.Debug("chose the form to print", "json", asJSON, "because", why)
	return asJSON
}
