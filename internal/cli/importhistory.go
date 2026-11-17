package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
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
over, and so is one whose export.json doesn't read, saying so, and one a
newer switchboard exported is refused.

A day this Mac doesn't hold is copied in, and to a day it holds, the lines it
doesn't hold are added, so an import repeated adds nothing. A day's summary
is copied in where the Mac holds no line of the day, and no summary of it,
or one that counts no line where the folder's counts some. A day's lines
past keeping, as [ledger] keep and [history] keep say, are left out, and its
summary taken in as that says.

A folder's accounts record joins this Mac's: an account this Mac knows,
matched by its id or its organization, is taken in under this Mac's id, and
one it doesn't know is added, named by its label. A folder exported before
switchboard made ids holds no record: this Mac's own is taken in under the
ids its accounts have now, and another Mac's is passed over, saying so, until
it's exported again from that Mac once it runs this version. A folder whose
record hasn't synced yet is passed over too, to import again once it has, and
one whose record doesn't read.

Each pool of a folder's Mac is taken in as the pool here holding the
accounts that Mac's config, copied into its folder, puts in it. One whose
accounts are in several pools here, or in none, is kept apart, saying so, and
read with --pool and its id.

It copies the folders into the state directory, then has the router take
them in, waiting however long that takes, so it needs the router running:
start it with switchboard service install (or switchboard serve). An import
run while another is running is refused.

On a Mac with no config and no router, as a new one taking over from another
or restoring a backup, it lays down the config a folder holds instead,
recording its accounts, and says what to run next: setup, to give each
account its token, then import again. It lays one down only where nothing
is at the config's path, this Mac has no accounts record of its own, and no
folder in <dir> is passed over.`,
		Args: oneFolder("import from", "reads"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.importHistory(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0])
		},
	}
}

var (
	// errImportUnanswered is what import fails with where the router closed
	// without answering, as one stopping does, partway through or not.
	errImportUnanswered = errors.New("the router closed before it answered, so the import may have stopped partway: what it took in stays, and importing again takes in the rest")
	// errRouterBeforeImport is what import fails with where the router
	// running is from before import, as one whose restart into an upgrade is
	// still due.
	errRouterBeforeImport = errors.New("the router running is older than this switchboard: restart it with switchboard service restart (or, run by hand, stop it and run switchboard serve again), then import again")
)

// importHistory imports the Macs' folders in dir, taken from the working
// directory where it's relative, as importThrough does, holding the
// import's lock throughout, and says what it took in, then each pool it kept
// apart, as apartSaid says. It's refused at once, with
// export.ErrImportRunning, where another holds the lock. Where no router
// runs on a Mac with no config, it lays down the config of the Mac's folder
// in dir instead, as restore does.
func (a *app) importHistory(ctx context.Context, out, errOut io.Writer, dir string) error {
	dir, err := a.absolute(dir)
	if err != nil {
		return err
	}
	state, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return err
	}
	if err := historyMoved(state); err != nil {
		return err
	}
	unlock, err := export.LockImport(state)
	if err != nil {
		return err
	}
	defer unlock()
	imported, err := importThrough(ctx, router.NewClient(router.SocketPath(state)), errOut, state, dir)
	if errors.Is(err, errNoRouterToAsk) {
		return a.restore(out, errOut, state, dir)
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, importedSaid(imported)); err != nil {
		return err
	}
	for _, p := range imported.Apart {
		if _, err := fmt.Fprintln(errOut, "switchboard: "+apartSaid(p)); err != nil {
			return err
		}
	}
	return nil
}

// errNoRouterToAsk is what importThrough fails with where no router answers
// on its control socket, as none runs, before it stages anything.
var errNoRouterToAsk = fmt.Errorf("%w", errRouterDown)

// importThrough has the router whose state directory is state import the
// Macs' folders in dir, and returns what it took in: having asked it first
// whether it's running an import, refusing, touching nothing, where it is,
// and failing with errNoRouterToAsk where none runs; then staging the
// folders foldersIn finds, as export.Stage stages them; then asking it to
// take them in, and waiting for its answer.
func importThrough(ctx context.Context, client *router.Client, errOut io.Writer, state, dir string) (router.Imported, error) {
	switch running, err := client.ImportRunning(ctx); {
	case errors.Is(err, router.ErrNotRunning):
		return router.Imported{}, errNoRouterToAsk
	case err != nil:
		return router.Imported{}, fromImport(err)
	case running:
		return router.Imported{}, export.ErrImportRunning
	}
	folders, err := foldersIn(errOut, state, dir)
	if err != nil {
		return router.Imported{}, err
	}
	if err := export.Stage(router.ImportDir(state), folders); err != nil {
		return router.Imported{}, err
	}
	imported, err := client.Import(ctx)
	if errors.Is(err, router.ErrUnanswered) {
		return router.Imported{}, errImportUnanswered
	}
	if err != nil {
		return router.Imported{}, fromImport(err)
	}
	return imported, nil
}

// fromImport is err, a router client's from asking of an import, as
// fromRouter says, but errRouterBeforeImport where the router doesn't know
// import.
func fromImport(err error) error {
	if errors.Is(err, router.ErrNoImport) {
		return errRouterBeforeImport
	}
	return fromRouter(err)
}

// foldersIn returns the Macs' folders to import from dir into the Mac whose
// state directory is state, as foldersFound finds them, saying on errOut
// which it passed over, as sayPassed says them, the import carrying on
// without them. It fails as foldersFound does, and where it passed over
// every folder there was.
func foldersIn(errOut io.Writer, state, dir string) ([]string, error) {
	folders, passed, err := foldersFound(state, dir)
	if sayErr := sayPassed(errOut, passed); sayErr != nil {
		return nil, sayErr
	}
	switch {
	case err != nil:
		return nil, err
	case len(folders) == 0:
		return nil, fmt.Errorf("there's nothing to import in %s: each Mac's folder in it was passed over", dir)
	}
	return folders, nil
}

// foldersFound returns the Macs' folders in dir to import into the Mac whose
// state directory is state, as export.Folders finds them, and those it
// passes over, each as "<folder>: <why>": those export.Folders passes over,
// and those the router would refuse, as idmove's Refusal says. It fails
// where dir can't be read, as export.Folders does, and where a folder can't
// be told, as Refusal does, giving those it passed over all the same.
func foldersFound(state, dir string) (folders, passed []string, err error) {
	found, err := export.Folders(dir, func(folder string, why error) { passed = append(passed, folder+": "+why.Error()) })
	here := idmove.Here{Install: sync.OnceValues(func() (string, error) { return export.InstallID(state) })}
	for _, folder := range found {
		switch refused := idmove.Refusal(folder, here); {
		case idmove.Refused(refused):
			passed = append(passed, folder+": "+refused.Error())
		case refused != nil:
			err = cmp.Or(err, refused)
		default:
			folders = append(folders, folder)
		}
	}
	return folders, passed, err
}

// sayPassed says on errOut, a line each, the folders an import passed over,
// as foldersFound gives them.
func sayPassed(errOut io.Writer, passed []string) error {
	for _, said := range passed {
		if _, err := fmt.Fprintln(errOut, "switchboard: passed over "+said); err != nil {
			return err
		}
	}
	return nil
}

// apartSaid says a pool whose history an import keeps apart, why, as
// identity.ApartSplit and its kin say, and how to read it: `kept the pool
// "Acme" (q7mt) of mac-0123456789abcdef apart, as its accounts are in more
// than one pool here: read it with --pool q7mt`; or, of one whose days an
// import took in before alone, as identity.ApartBefore says, that those are.
func apartSaid(p router.ApartPool) string {
	pool := "the pool " + prose.Quoted(p.Label) + " (" + p.Pool + ") of " + export.FolderName(p.Install)
	if p.Why == identity.ApartBefore {
		return pool + " goes to the pool here holding its accounts now, but the days an import took in before stay under " + p.Pool + ": read them with --pool " + p.Pool
	}
	because := map[string]string{
		identity.ApartSplit:       ", as its accounts are in more than one pool here",
		identity.ApartNoneHere:    ", as none of its accounts is configured here",
		identity.ApartNoConfig:    ", as its folder holds no copy of that Mac's config to match its accounts by",
		identity.ApartEmpty:       ", as that Mac's config puts no account in it",
		identity.ApartNoPoolsHere: ", as this Mac's config has no pools",
	}[p.Why]
	return "kept " + pool + " apart" + because + ": read it with --pool " + p.Pool
}

// importedSaid says what an import took in, as imported counts it: "imported
// 1 folder: 27 days added, 3 merged, 12 past keeping left out, 27 days'
// summaries".
func importedSaid(imported router.Imported) string {
	return "imported " + prose.Counted(imported.Folders, "folder") + ": " + daysImported(imported)
}

// daysImported says how an import took in its days: the summaries copied in,
// as summariesImported says them, where it added and merged no day; else
// those added, merged and left out as past keeping, each where there's any,
// then every summary copied in, whichever days they're of, where any was;
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
	if imported.Summaries > 0 {
		said = append(said, daysOf(imported.Summaries, "summary", "summaries"))
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
