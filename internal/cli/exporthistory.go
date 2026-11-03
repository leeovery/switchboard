package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
)

func newExportCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "export <dir>",
		Short: "Copy this Mac's history into a folder of its own in <dir>",
		Long: `Copy this Mac's history into a folder of its own in <dir>, mac-<id>,
named by this install's id, which the first export makes: the request
ledger's lines, the days' summaries, the readings history and the router's
events, each day's lines in one compressed file, and export.json, saying
whose folder it is and when it was exported. Kept in a folder Dropbox or
iCloud Drive syncs, the history outlasts the Mac.

A day exported before, and unchanged since, is left as it is, and nothing in
the folder is ever removed, a day past keeping included. Every file is
written beside where it goes and renamed into place, so a sync never carries
one half written. It reads the files where they lie, so it needs no router.

A day whose files can't be opened or read is passed over, and the rest
exported; then export names each it passed over, and why, and fails.`,
		Args: oneFolder("export into", "writes"),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := a.export(cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0])
			if errors.Is(err, errUnexported) {
				// export has said which days, and why.
				cmd.SilenceErrors = true
			}
			return err
		},
	}
}

// oneFolder takes one folder, to export into or import from as purpose says,
// refusing one whose path holds something shaped like a token, which
// switchboard never does as never says, nor repeats.
func oneFolder(purpose, never string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		switch {
		case len(args) != 1 || args[0] == "":
			return errors.New("give the folder to " + purpose)
		case redact.HoldsToken(args[0]):
			return errors.New("the folder given holds something shaped like a token, which switchboard never " + never + ": give another")
		}
		return nil
	}
}

// errUnexported is what export fails with where it passed over a day's file
// it couldn't read, once it has exported the rest and said which.
var errUnexported = errors.New("some days couldn't be exported")

// export exports the history in the state directory into the Mac's folder in
// dir, taken from the working directory where it's relative, and says what
// it did, then, on errOut, each day's file it couldn't read, failing with
// errUnexported where there's any.
func (a *app) export(out, errOut io.Writer, dir string) error {
	dir, err := a.absolute(dir)
	if err != nil {
		return err
	}
	state, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return err
	}
	r, err := export.Exporter{StateDir: state, Now: a.Now, Logger: logger}.Export(dir)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "exported %s to %s: %s written, %s as they were\n",
		prose.Counted(r.Days(), "day"), r.Folder, prose.Grouped(r.Written), prose.Grouped(r.AsTheyWere)); err != nil {
		return err
	}
	return sayUnexported(errOut, r.Unread)
}

// sayUnexported says on errOut, a line each, why each of unread, the days'
// files an export passed over, couldn't be exported, failing with
// errUnexported, and why each couldn't, where there's any.
func sayUnexported(errOut io.Writer, unread []export.DayError) error {
	if len(unread) == 0 {
		return nil
	}
	errs := make([]error, len(unread))
	for i, u := range unread {
		errs[i] = u
		if _, err := fmt.Fprintln(errOut, "switchboard: "+u.Error()); err != nil {
			return err
		}
	}
	return fmt.Errorf("%w: %w", errUnexported, errors.Join(errs...))
}

// absolute is the path given, taken from the working directory where it's
// relative.
func (a *app) absolute(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	wd, err := a.Getwd()
	if err != nil {
		return "", fmt.Errorf("find the working directory: %w", err)
	}
	return filepath.Join(wd, path), nil
}
