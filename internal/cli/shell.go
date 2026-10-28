package cli

import (
	"bytes"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/shell"
)

// shellFiles are switchboard's files for the shells, where the CLI finds its
// shell and bin directories, each loading the completions of root, the
// command tree.
func (a *app) shellFiles(root *cobra.Command) (shell.Files, error) {
	dir, err := config.ShellDir(a.Getenv, a.HomeDir)
	if err != nil {
		return shell.Files{}, err
	}
	bin, err := config.BinDir(a.Getenv, a.HomeDir)
	if err != nil {
		return shell.Files{}, err
	}
	return shell.Files{Dir: dir, Bin: bin, Completion: completions(root)}, nil
}

// completions returns what makes the completion script of root, the command
// tree, for a shell: its commands and flags, completed by cobra's
// __complete, which serves them with the completion command left out.
func completions(root *cobra.Command) func(shell.Shell) ([]byte, error) {
	return func(sh shell.Shell) ([]byte, error) {
		var gen func(io.Writer) error
		switch sh {
		case shell.Zsh:
			gen = root.GenZshCompletion
		case shell.Bash:
			gen = func(w io.Writer) error { return root.GenBashCompletionV2(w, true) }
		case shell.Fish:
			gen = func(w io.Writer) error { return root.GenFishCompletion(w, true) }
		default:
			return nil, fmt.Errorf("switchboard has no completions for %s", sh)
		}
		var b bytes.Buffer
		if err := gen(&b); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}
}

// refreshShellFiles brings switchboard's files for the shells up to date, as
// the router starts, and logs what it did: it writes none that setup didn't.
func (a *app) refreshShellFiles(root *cobra.Command) {
	files, err := a.shellFiles(root)
	if err != nil {
		logger.Warn("can't find switchboard's files for the shells to bring them up to date", "error", err)
		return
	}
	refreshed, err := files.Refresh()
	for _, f := range refreshed {
		switch f.State {
		case shell.Absent:
			logger.Info("no file of switchboard's for the shell to bring up to date; setup writes it", "path", f.Path)
		case shell.Updated:
			logger.Info("brought switchboard's file for the shell up to date", "path", f.Path)
		default:
			logger.Info("switchboard's file for the shell is up to date", "path", f.Path)
		}
	}
	if err != nil {
		logger.Warn("can't bring switchboard's files for the shells up to date", "path", files.Dir, "error", err)
	}
}
