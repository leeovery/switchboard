package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
)

func newImportCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "import <dir>",
		Short: "Bring in the history of each Mac's folder in <dir>, through the router",
		Long: `Bring in the history of each Mac's folder in <dir>, as export writes them,
or of the one folder <dir> is: the request ledger's lines, the days'
summaries, the readings history and the router's events. This Mac's own
folder is taken like any other, a folder without an export.json is passed
over, and one a newer switchboard exported is refused.

A day this Mac doesn't hold is copied in, and to a day it holds, the lines it
doesn't hold are added, so an import repeated adds nothing. A day's summary
is copied in where the Mac held nothing of the day. A day's lines past
keeping, as [ledger] keep and [history] keep say, are left out, and its
summary taken in.

It copies the folders into the state directory, then has the router take
them in, waiting however long that takes, so it needs the router running:
start it with switchboard service install (or switchboard serve). An import
run while another is running is refused.`,
		Args: oneFolder("import from", "reads"),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := a.importHistory(cmd.Context(), cmd.OutOrStdout(), args[0])
			if refused, ok := errors.AsType[importRefusal](err); ok {
				cmd.SilenceErrors = true
				if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "switchboard: "+refused.Error()); err != nil {
					return err
				}
			}
			return err
		},
	}
}

// importRefusal is an import refused, as while another runs, or by the
// router, saying why.
type importRefusal struct {
	error
}

func (r importRefusal) Unwrap() error {
	return r.error
}

// errImportUnanswered is what import fails with where the router closed
// without answering, as one stopping does, partway through or not.
var errImportUnanswered = errors.New("the router closed before it answered, so the import may have stopped partway: what it took in stays, and importing again takes in the rest")

// importHistory imports the Macs' folders in dir, taken from the working
// directory where it's relative, as importThrough does, holding the
// import's lock throughout, and says what it took in. It's refused at once
// where another holds the lock.
func (a *app) importHistory(ctx context.Context, out io.Writer, dir string) error {
	dir, err := a.absolute(dir)
	if err != nil {
		return err
	}
	state, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return err
	}
	unlock, err := export.LockImport(state)
	if errors.Is(err, export.ErrImportRunning) {
		return importRefusal{err}
	}
	if err != nil {
		return err
	}
	defer unlock()
	imported, err := importThrough(ctx, router.NewClient(router.SocketPath(state)), state, dir)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, importedSaid(imported))
	return err
}

// importThrough has the router whose state directory is state import the
// Macs' folders in dir, and returns what it took in: having asked it first
// whether it's running an import, refusing, touching nothing, where it is;
// then staging the folders, as export.Stage stages them; then asking it to
// take them in, and waiting for its answer.
func importThrough(ctx context.Context, client *router.Client, state, dir string) (router.Imported, error) {
	switch running, err := client.ImportRunning(ctx); {
	case err != nil:
		return router.Imported{}, fromRouter(err)
	case running:
		return router.Imported{}, importRefusal{export.ErrImportRunning}
	}
	folders, err := export.Folders(dir)
	if err != nil {
		return router.Imported{}, err
	}
	if err := export.Stage(router.ImportDir(state), folders); err != nil {
		return router.Imported{}, err
	}
	imported, err := client.Import(ctx)
	if err != nil {
		return router.Imported{}, fromImport(err)
	}
	return imported, nil
}

// fromImport is err, POST /import's, saying plainly where the router closed
// without answering or isn't running, and as an importRefusal otherwise, as
// where the router refused, saying why.
func fromImport(err error) error {
	switch {
	case errors.Is(err, router.ErrUnanswered):
		return errImportUnanswered
	case router.IsNotRunning(err):
		return errRouterDown
	}
	return importRefusal{err}
}

// importedSaid says what an import took in, as imported counts it: "imported
// 1 folder: 27 days added, 3 merged, 12 past keeping left out".
func importedSaid(imported router.Imported) string {
	return "imported " + prose.Counted(imported.Folders, "folder") + ": " + daysImported(imported)
}

// daysImported says how an import took in its days: the summaries copied in,
// as summariesImported says them, where it added and merged no day; else
// those added, merged and left out as past keeping, each where there's any;
// else nothing new.
func daysImported(imported router.Imported) string {
	days := imported.Days
	if days.Added == 0 && days.Merged == 0 && imported.Summaries > 0 {
		return summariesImported(imported)
	}
	var said []string
	for _, took := range []struct {
		days int
		how  string
	}{{days.Added, "added"}, {days.Merged, "merged"}, {days.PastKeep, "past keeping left out"}} {
		switch {
		case took.days == 0:
		case len(said) == 0:
			said = append(said, prose.Counted(took.days, "day")+" "+took.how)
		default:
			said = append(said, prose.Grouped(took.days)+" "+took.how)
		}
	}
	if len(said) == 0 {
		return "nothing new"
	}
	return strings.Join(said, ", ")
}

// summariesImported says the summaries an import copied in, then the days
// whose lines it left out as past keeping, where there's any: "12 days'
// summaries, 12 days' lines past keeping left out".
func summariesImported(imported router.Imported) string {
	said := daysOf(imported.Summaries, "summary", "summaries")
	if imported.Days.PastKeep > 0 {
		said += ", " + daysOf(imported.Days.PastKeep, "lines", "lines") + " past keeping left out"
	}
	return said
}

// daysOf counts n days' things, one or many, as one and many name them: "1
// day's summary", "12 days' summaries".
func daysOf(n int, one, many string) string {
	if n == 1 {
		return "1 day's " + one
	}
	return prose.Grouped(n) + " days' " + many
}
