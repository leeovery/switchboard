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
one half written. It reads the files where they lie, so it needs no router.`,
		Args: oneFolder,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.export(cmd.OutOrStdout(), args[0])
		},
	}
}

// oneFolder takes one folder to export into, refusing one whose path holds
// something shaped like a token, which no command writes or repeats.
func oneFolder(_ *cobra.Command, args []string) error {
	switch {
	case len(args) != 1 || args[0] == "":
		return errors.New("give the folder to export into")
	case redact.HoldsToken(args[0]):
		return errors.New("the folder given holds something shaped like a token, which switchboard never writes: give another")
	}
	return nil
}

// export exports the history in the state directory into the Mac's folder in
// dir, taken from the working directory where it's relative, and says what
// it did.
func (a *app) export(out io.Writer, dir string) error {
	if !filepath.IsAbs(dir) {
		wd, err := a.Getwd()
		if err != nil {
			return fmt.Errorf("find the working directory: %w", err)
		}
		dir = filepath.Join(wd, dir)
	}
	state, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return err
	}
	r, err := export.Exporter{StateDir: state, Now: a.Now, Logger: logger}.Export(dir)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "exported %s to %s: %s written, %s as they were\n",
		prose.Counted(r.Days(), "day"), r.Folder, prose.Grouped(r.Written), prose.Grouped(r.AsTheyWere))
	return err
}
