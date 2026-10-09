package cli

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

// formFlags are a data verb's --json and --pretty, which choose the form it
// prints in wherever stdout is: JSON, or the form for a person.
type formFlags struct {
	json, pretty bool
}

// add registers --json and --pretty on cmd.
func (f *formFlags) add(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.json, "json", false, "print JSON, as off a terminal")
	cmd.Flags().BoolVar(&f.pretty, "pretty", false, "print the form for a person, as on a terminal")
}

// args refuses --json with --pretty, then checks the command line as next
// does.
func (f *formFlags) args(next cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if f.json && f.pretty {
			return errors.New("--json prints JSON, so it takes no --pretty")
		}
		return next(cmd, args)
	}
}

// printsJSON reports whether a data verb prints JSON to out, as f says: with
// --json, or off a terminal unless --pretty.
func (a *app) printsJSON(f formFlags, out io.Writer) bool {
	return f.json || !f.pretty && !a.Terminal(out)
}
