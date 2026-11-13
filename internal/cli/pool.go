package cli

import (
	"errors"
	"fmt"
	"io"
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
// it's fenced to, as the config writes them, none for the unfenced pool, and
// its accounts, in the config's order.
type listedPool struct {
	Pool      string          `json:"pool"`
	PoolLabel string          `json:"pool_label"`
	Dirs      []string        `json:"dirs"`
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

A session started inside a pool's directories is routed across that pool's
accounts and no others; one started outside every fence, across the unfenced
pool's, where there's one. pool add, edit and remove make a pool, change its
label or its directories, and remove one that has no accounts. accounts add
and edit put an account in a pool with --pool.

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
		pool := listedPool{Pool: p.ID, PoolLabel: p.Label, Dirs: slices.Clone(p.WrittenDirs), Accounts: []pooledAccount{}}
		if pool.Dirs == nil {
			pool.Dirs = []string{}
		}
		primary, _ := cfg.Accounts.PrimaryOf(p.ID)
		for _, acct := range in {
			pool.Accounts = append(pool.Accounts, pooledAccount{Account: acct.ID, AccountName: status.ConfiguredName(acct), Primary: acct.ID == primary.ID})
		}
		listed = append(listed, pool)
	}
	return listed
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
		rows = append(rows, []string{prose.Clean(p.Pool), prose.Clean(p.PoolLabel), orNone(strings.Join(dirs, ", ")), orNone(strings.Join(held, ", "))})
	}
	return writeTable(out, rows)
}

func newPoolAddCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "add <label> [<dir>...]",
		Short: "Make a pool, fenced to the directories given, or unfenced",
		Long: `Make a pool, shown as the label given, and fenced to the directories given,
each an absolute path or one starting ~/, covering everything beneath it: a
[[pool]] table in the config, under an id switchboard makes. Without any, it's
the unfenced pool, which serves every session started outside every fence;
only one pool can be, so it's refused where one is already, saying which.
Every pool has a label, no two alike, ignoring case, and no two pools'
directories overlap.

A pool made has no accounts: a session started in its directories refuses to
launch until one is added to it, with accounts add --pool or accounts edit
--pool.

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
			return poolAddedSays(cmd.OutOrStdout(), added)
		},
	}
}

// fenceSaid says where a pool is fenced: "fenced to ~/Code/acme", or
// "unfenced".
// poolAddedSays writes to out that the pool added was, by its label and the
// id switchboard made for it, fenced or not as fenceSaid says.
func poolAddedSays(out io.Writer, added config.Pool) error {
	_, err := fmt.Fprintf(out, "added pool %s as %s, %s\n", prose.Quoted(prose.Clean(added.Label)), added.ID, fenceSaid(added))
	return err
}

// poolRemovedSays writes to out that the pool removed was, by its id and
// label, and that its history stays, read by its id.
func poolRemovedSays(out io.Writer, removed config.Pool) error {
	_, err := fmt.Fprintf(out, "removed pool %s (%s): its history stays, read with --pool %s by requests, history, sessions and events\n",
		removed.ID, prose.Clean(removed.Label), removed.ID)
	return err
}

func fenceSaid(p config.Pool) string {
	if !p.Fenced() {
		return "unfenced"
	}
	dirs := make([]string, len(p.WrittenDirs))
	for i, d := range p.WrittenDirs {
		dirs[i] = prose.Clean(d)
	}
	return "fenced to " + prose.List(dirs)
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
place: edit them by hand. A session running keeps the pool it started in
till it's resumed.

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
			_, err = io.WriteString(cmd.OutOrStdout(), poolEditedSays(edited)+"\n")
			return err
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
// now, then as it was, or that it changed nothing.
func poolEditedSays(e accounts.EditedPool) string {
	if !e.Changed() {
		return "pool " + e.Now.ID + " has every value given already, so nothing changed"
	}
	var changed []string
	if e.Was.Label != e.Now.Label {
		changed = append(changed, nowAndWas("label", prose.Quoted(prose.Clean(e.Now.Label)), prose.Quoted(prose.Clean(e.Was.Label))))
	}
	if !slices.Equal(e.Was.WrittenDirs, e.Now.WrittenDirs) {
		changed = append(changed, fenceSaid(e.Now)+" (was "+fenceSaid(e.Was)+")")
	}
	return "edited pool " + e.Now.ID + ": " + strings.Join(changed, ", ")
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
