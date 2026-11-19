package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// poolsDocument is what pool --json prints: every pool, in the config's
// order.
type poolsDocument struct {
	Pools []listedPool `json:"pools"`
}

// listedPool is a pool as pool lists it: its id and label, the directories
// it's fenced to, as the config writes them and as it resolves them,
// absolute, as the router fences sessions by them, none for the unfenced
// pool, and its accounts, in the config's order.
type listedPool struct {
	Pool      string          `json:"pool"`
	PoolLabel string          `json:"pool_label"`
	Dirs      []string        `json:"dirs"`
	Paths     []string        `json:"paths"`
	Accounts  []pooledAccount `json:"accounts"`
}

// pooledAccount is an account of a pool, as pool lists it: its id and
// label, and whether it's the pool's primary.
type pooledAccount struct {
	Account string `json:"account"`
	status.AccountName
	Primary bool `json:"primary,omitempty"`
}

func newPoolCommand(a *app) *cobra.Command {
	var form formFlags
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "List the pools, their directories, accounts and primaries",
		Long: `List the pools, in the config's order: each one's id and label, the
directories it's fenced to, or none for the pool that serves every session
started outside every fence, and its accounts, in the config's order, its
primary marked.

A session started inside a pool's directories, or in a git worktree of a
repository inside them, is routed across that pool's accounts and no others;
one started outside every fence, across the unfenced pool's, where there's
one. pool add, edit and remove make a pool, change its label or its
directories, and remove one that has no accounts. accounts add and edit put
an account in a pool with --pool.

On a terminal, pool prints its pools as text. Anywhere else, as in a pipe or
an agent's shell, it prints them as JSON, for an agent or a script to read.
--json prints the JSON, and --pretty the text, wherever stdout is.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			listed := listPools(a.standing(cmd.Context(), cfg))
			out := cmd.OutOrStdout()
			if a.printsJSON(form, out) {
				return writeJSON(out, poolsDocument{Pools: listed})
			}
			return writePools(out, listed, a.noPools())
		},
	}
	form.add(cmd)
	cmd.AddCommand(newPoolAddCommand(a), newPoolEditCommand(a), newPoolRemoveCommand(a))
	return cmd
}

// listPools are cfg's pools as pool lists them, cfg as its pools stand, as
// pooledConfig has them: none in a config from before pools, or where none
// can be told.
func listPools(cfg *config.Config) []listedPool {
	listed := []listedPool{}
	for _, p := range cfg.Pools {
		in := cfg.Accounts.InPool(p.ID)
		pool := listedPool{Pool: p.ID, PoolLabel: p.Label, Dirs: listedDirs(p.WrittenDirs), Paths: listedDirs(p.Dirs), Accounts: []pooledAccount{}}
		primary, _ := cfg.Accounts.PrimaryOf(p.ID)
		for _, acct := range in {
			pool.Accounts = append(pool.Accounts, pooledAccount{Account: acct.ID, AccountName: status.ConfiguredName(acct), Primary: acct.ID == primary.ID})
		}
		listed = append(listed, pool)
	}
	return listed
}

// listedDirs are dirs as a pool's JSON lists them: a copy, empty for none.
func listedDirs(dirs []string) []string {
	return append([]string{}, dirs...)
}

// noPools says why there are no pools to list: before the move to pools, the
// router makes the first as it moves the history; after it, none of the
// config, the router or the accounts record names any.
func (a *app) noPools() string {
	if a.poolsMoved() {
		return "no pools: " + launch.NoPoolsNamed
	}
	return "no pools: the router makes the first as it moves the history to pools"
}

// writePools writes the pools as pool's text: a row a pool, under its heads,
// its directories joined, a dash for none, and its accounts, each by its id,
// the primary marked; or, where there are none, none as it says.
func writePools(out io.Writer, pools []listedPool, none string) error {
	if len(pools) == 0 {
		_, err := fmt.Fprintln(out, none)
		return err
	}
	rows := [][]string{{"ID", "LABEL", "DIRS", "ACCOUNTS"}}
	for _, p := range pools {
		dirs := make([]string, len(p.Dirs))
		for i, d := range p.Dirs {
			dirs[i] = prose.Clean(d)
		}
		var held []string
		for _, acct := range p.Accounts {
			name := prose.Clean(acct.Account)
			if acct.Primary {
				name += " (primary)"
			}
			held = append(held, name)
		}
		rows = append(rows, []string{prose.Clean(p.Pool), prose.Clean(prose.LabelOr(p.PoolLabel, p.Pool)), orNone(strings.Join(dirs, ", ")), orNone(strings.Join(held, ", "))})
	}
	return writeTable(out, rows)
}

func newPoolAddCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "add <label> [<dir>...]",
		Short: "Make a pool, fenced to the directories given, or unfenced",
		Long: `Make a pool, shown as the label given, and fenced to the directories given,
each an absolute path or one starting ~/, covering everything beneath it, and
every git worktree of a repository in them, wherever it lives: a [[pool]]
table in the config, under an id switchboard makes. Without any, it's
the unfenced pool, which serves every session started outside every fence;
only one pool can be, so it's refused where one is already, saying which.
Every pool has a label, no two alike, ignoring case, and no two pools'
directories overlap.

A pool made has no accounts: a session started in its directories refuses to
launch until one is added to it, with accounts add --pool or accounts edit
--pool. A session already running in its directories keeps the pool it
started in, and that pool's accounts, till it's resumed.

` + editingTheConfig,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("give the pool's label, then the directories it's fenced to, if any")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := a.accountFiles()
			if err != nil {
				return err
			}
			added, err := registry.AddPool(args[0], args[1:])
			if err != nil {
				return err
			}
			if err := poolAddedSays(cmd.OutOrStdout(), added); err != nil {
				return err
			}
			warnUnmade(cmd.ErrOrStderr(), a.Root, added.Pool)
			return nil
		},
	}
}

// poolAddedSays writes to out that the pool added was, by its label and the
// id switchboard made for it, fenced or not as fenceSaid says; of one
// fenced, that the sessions running in its directories keep the pool they
// started in till they're resumed, as runningKeepTheirPool says; of one
// unfenced, that the sessions outside every fence go to it; and where the
// pool the verbs act on without --pool is another now, as verbsSaid says.
func poolAddedSays(out io.Writer, added accounts.AddedPool) error {
	var running string
	var outside []string
	if added.Pool.Fenced() {
		running = "; " + runningKeepTheirPool
	} else {
		outside = append(outside, outsideGoToIt)
	}
	if verbs := verbsSaid(added.Before(), added.Pools, added.Pool); verbs != "" {
		outside = append(outside, verbs)
	}
	if len(outside) > 0 {
		running += "; " + strings.Join(outside, ", and ")
	}
	p := added.Pool
	_, err := fmt.Fprintf(out, "added pool %s as %s, %s%s\n", prose.Quoted(p.Label), p.ID, fenceSaid(p), running)
	return err
}

// runningKeepTheirPool says that the sessions running in directories a fence
// takes in keep the pool they started in, as a session's pool is decided as
// it starts: their work goes on that pool's accounts till they're resumed.
const runningKeepTheirPool = "sessions running in its directories keep the pool they started in, and its accounts, till they're resumed"

// What pool add and edit say a pool's fence changed for the sessions started
// outside every fence, and for the verbs.
const (
	// outsideGoToIt says that the sessions started outside every fence go to
	// the pool, unfenced.
	outsideGoToIt = "sessions started outside every fence go to it now"
	// noneOutside says that no session starts outside every fence, every
	// pool fenced, but one started within a session, which takes its pool,
	// or in a worktree of a repository inside a fence, which is in its pool.
	noneOutside = "every pool is fenced now, so claude won't start in a folder outside every fence, " +
		"unless it's started within a session, or in a worktree of a repository inside a fence"
	// verbsReadIt says that the verbs read and act on the pool without
	// --pool, as config.Pools' Implicit gives it.
	verbsReadIt = "the verbs read and act on it without --pool"
	// verbsNeedPool says that the verbs read and act on no pool without
	// --pool, as config.Pools' Implicit gives none.
	verbsNeedPool = "the verbs, accounts add among them, need --pool to read or act on a pool"
)

// poolRemovedSays writes to out that the pool removed was, by its id and
// label, and that its history stays, read by its id.
func poolRemovedSays(out io.Writer, removed config.Pool) error {
	_, err := fmt.Fprintf(out, "removed pool %s (%s): its history stays, read with --pool %s by requests, history, sessions and events\n",
		removed.ID, prose.Clean(prose.LabelOr(removed.Label, removed.ID)), removed.ID)
	return err
}

// fenceSaid says where a pool is fenced: "fenced to ~/Code/acme", or
// "unfenced".
func fenceSaid(p config.Pool) string {
	if !p.Fenced() {
		return "unfenced"
	}
	return "fenced to " + dirsListed(p)
}

// dirsListed lists p's directories as the config writes them:
// "~/Code/acme and /Volumes/Acme".
func dirsListed(p config.Pool) string {
	dirs := make([]string, len(p.WrittenDirs))
	for i, d := range p.WrittenDirs {
		dirs[i] = prose.Clean(d)
	}
	return prose.List(dirs)
}

// poolEditFlags are pool edit's flags, of which it's given one or more.
var poolEditFlags = []string{"--label", "--dirs", "--unfence"}

func newPoolEditCommand(a *app) *cobra.Command {
	var (
		label   string
		dirs    []string
		unfence bool
	)
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Change a pool's label or its directories",
		Long: `Change a pool's label, or the directories it's fenced to, in its [[pool]]
table in the config. Give one or more of --label, --dirs and --unfence; what
isn't given is left as it is, and when every value given is what the pool has
already, nothing is written.

--dirs, given once a directory, replaces the pool's directories with those
given, each an absolute path or one starting ~/. --unfence takes them out,
leaving it the unfenced pool, which is refused where another is already,
saying which. Directories the config writes across lines can't be changed in
place: edit them by hand. A session running keeps the pool it started in,
and that pool's accounts, till it's resumed, even in directories the pool's
new fence takes in.

` + editingTheConfig,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("give one pool's id")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			change, err := poolChangeOf(cmd.Flags().Changed, label, dirs, unfence)
			if err != nil {
				return err
			}
			registry, err := a.accountFiles()
			if err != nil {
				return err
			}
			edited, err := registry.EditPool(args[0], change)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(cmd.OutOrStdout(), poolEditedSays(edited)+"\n"); err != nil {
				return err
			}
			if !slices.Equal(edited.Was.WrittenDirs, edited.Now.WrittenDirs) {
				warnUnmade(cmd.ErrOrStderr(), a.Root, edited.Now)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "show the pool as `LABEL`")
	cmd.Flags().StringArrayVar(&dirs, "dirs", nil, "fence the pool to `DIR` in place of its directories, given once a directory")
	cmd.Flags().BoolVar(&unfence, "unfence", false, "take the pool's directories out, leaving it the unfenced pool")
	return cmd
}

// poolChangeOf is the change pool edit's flags ask for, those given as given
// reports: a label, the directories in place of the pool's, or none, to
// unfence it. It fails for none of them, and for --dirs and --unfence
// together.
func poolChangeOf(given func(name string) bool, label string, dirs []string, unfence bool) (accounts.PoolChange, error) {
	var change accounts.PoolChange
	switch {
	case !slices.ContainsFunc(poolEditFlags, func(f string) bool { return given(strings.TrimPrefix(f, "--")) }):
		return change, errors.New("give one or more of " + prose.ListOr(poolEditFlags))
	case given("unfence") && !unfence:
		return change, errors.New("--unfence takes the pool's directories out, and can't be false: --dirs fences it")
	case unfence && given("dirs"):
		return change, errors.New("--unfence takes the pool's directories out, so it takes no --dirs")
	}
	if given("label") {
		change.Label = &label
	}
	switch {
	case unfence:
		change.Dirs = &[]string{}
	case given("dirs"):
		change.Dirs = &dirs
	}
	return change, nil
}

// poolEditedSays says what an edit changed of a pool, each value as it is
// now, then as it was, and where it fenced it anew, that the sessions
// running in its directories keep their pool, as runningKeepTheirPool
// says, and what its fence changed for the sessions outside every fence, as
// outsideSaid has it; where it wrote its directories in another form alone,
// as from ~/, that they're rewritten, as they hold the same; or that it
// changed nothing.
func poolEditedSays(e accounts.EditedPool) string {
	if !e.Changed() {
		return "pool " + e.Now.ID + " has every value given already, so nothing changed"
	}
	var changed []string
	if e.Was.Label != e.Now.Label {
		changed = append(changed, labelEdited(prose.Quoted(e.Now.Label), prose.Quoted(e.Was.Label)))
	}
	var running string
	switch {
	case slices.Equal(e.Was.WrittenDirs, e.Now.WrittenDirs):
	case slices.Equal(e.Was.Dirs, e.Now.Dirs):
		changed = append(changed, "directories rewritten as "+dirsListed(e.Now)+" (was "+dirsListed(e.Was)+"), the same directories")
	default:
		changed = append(changed, fenceSaid(e.Now)+" (was "+fenceSaid(e.Was)+")")
		if e.Now.Fenced() {
			running = "; " + runningKeepTheirPool
		}
		if outside := outsideSaid(e); outside != "" {
			running += "; " + outside
		}
	}
	return "edited pool " + e.Now.ID + ": " + strings.Join(changed, ", ") + running
}

// outsideSaid says what an edit of a pool's fence changed for the sessions
// started outside every fence, and for the verbs: unfencing it, that those
// sessions go to it now; fencing the one pool that was unfenced, that none
// starts outside every fence; and where the pool the verbs act on without
// --pool is another now, as verbsSaid says: "" where it changed nothing for
// them.
func outsideSaid(e accounts.EditedPool) string {
	var said []string
	switch _, unfenced := e.Pools.Unfenced(); {
	case e.Was.Fenced() && !e.Now.Fenced():
		said = append(said, outsideGoToIt)
	case !e.Was.Fenced() && !unfenced:
		said = append(said, noneOutside)
	}
	if verbs := verbsSaid(e.Before(), e.Pools, e.Now); verbs != "" {
		said = append(said, verbs)
	}
	return strings.Join(said, ", and ")
}

// verbsSaid says, where the pool the verbs act on without --pool, as
// config.Pools' Implicit gives it, is another among the pools after a change
// than before it, that it's p now, or that there's none, so they need
// --pool: "" where it's the one it was.
func verbsSaid(before, after config.Pools, p config.Pool) string {
	was, _ := before.Implicit()
	switch now, ok := after.Implicit(); {
	case config.SamePool(was.ID, now.ID):
		return ""
	case !ok:
		return verbsNeedPool
	case config.SamePool(now.ID, p.ID):
		return verbsReadIt
	}
	return ""
}

// warnUnmade warns w of each of p's directories that isn't a directory on
// this Mac, as root, the filesystem from its root, finds it, as
// config.Pool's Missing has them: the pool's made all the same, as its
// directory may be made later.
func warnUnmade(w io.Writer, root fs.FS, p config.Pool) {
	for _, dir := range p.Missing(root) {
		launch.Notice(w, prose.Clean(dir)+" isn't a directory, so no session starts in it yet: if it's mistyped, fence the pool anew with switchboard pool edit "+p.ID+" --dirs")
	}
}

func newPoolRemoveCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a pool that has no accounts",
		Long: `Remove a pool: its [[pool]] table from the config, with the comments directly
above it. A pool with accounts is refused, naming them: move each to another
pool with accounts edit --pool, or remove it, first, so no edit puts a pool's
accounts in another. The last pool can't be removed, as an account added
needs a pool to go into. Its directories are no pool's once it's gone, so a
session started in them goes to the unfenced pool, where there's one. Its
history stays, under its id: requests, history, sessions and events read it
with --pool and that id.

` + editingTheConfig,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("give one pool's id")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := a.accountFiles()
			if err != nil {
				return err
			}
			removed, err := registry.RemovePool(args[0])
			if err != nil {
				return err
			}
			return poolRemovedSays(cmd.OutOrStdout(), removed)
		},
	}
}
