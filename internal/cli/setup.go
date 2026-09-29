package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/skill"
)

func newSetupCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Walk through setting switchboard up, or what's left of it",
		Long: `Walk through setting switchboard up: the accounts, each one's token, and which
is the primary; priming; the service; the claude link; and the skill; then
show usage. Each step says what's done already, and does only what isn't, so
setup is safe to run again. It asks as it goes, so it needs a terminal:
without one, it says which command takes each step alone.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.setup(cmd)
		},
	}
}

// setup walks the user at the terminal stdin is through setting switchboard
// up.
func (a *app) setup(cmd *cobra.Command) error {
	hidden, ok := a.Hidden(cmd.InOrStdin())
	if !ok {
		return setup.ErrNoTerminal
	}
	s, err := a.newSetup(setup.Terminal{In: cmd.InOrStdin(), Out: cmd.OutOrStdout(), Hidden: hidden})
	if err != nil {
		return err
	}
	return s.Run(cmd.Context())
}

// newSetup returns setup at term, for the config, the state and the Claude
// Code config directory the CLI finds, this switchboard binary, and PATH.
func (a *app) newSetup(term setup.Terminal) (*setup.Setup, error) {
	path, err := a.configFile()
	if err != nil {
		return nil, err
	}
	files, err := a.tokens()
	if err != nil {
		return nil, err
	}
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	exe, err := a.Executable()
	if err != nil {
		return nil, fmt.Errorf("find this switchboard binary: %w", err)
	}
	skillPath, err := skill.Path(a.Getenv, a.HomeDir)
	if err != nil {
		return nil, err
	}
	bin, err := config.BinDir(a.Getenv, a.HomeDir)
	if err != nil {
		return nil, err
	}
	// Without a home directory, only the install paths outside it are tried.
	home, _ := a.HomeDir()
	return &setup.Setup{
		Terminal:     term,
		Accounts:     accounts.Registry{ConfigPath: path, Tokens: files, Prober: a.prober},
		Service:      svc,
		Install:      service.InstallOptions{Executable: exe, Config: a.configPath},
		Path:         a.Getenv("PATH"),
		InstallPaths: claude.InstallPaths(home),
		Bin:          bin,
		Home:         home,
		Skill:        skillPath,
		Usage: func(ctx context.Context, out io.Writer) error {
			return a.printUsage(ctx, out, usageOptions{})
		},
	}, nil
}
