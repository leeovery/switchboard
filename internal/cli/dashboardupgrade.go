package cli

import (
	"context"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/router"
)

// dashboardUpgrade is how the dashboard restarts itself onto an upgrade of
// this binary, as watch.Upgrade says: by the path it was run by, as it
// stands now; the router's health asked as the verbs ask it; the binary
// asked its version as --version prints it, run as the dashboard was, as
// versionIn reads it; and the process replaced with it, run by the command
// line and in the environment the dashboard was, but for the pool shown, as
// showingPool has it. None where the binary, or the command line it was run
// by, can't be found.
func (a *app) dashboardUpgrade() watch.Upgrade {
	exe, err := a.Executable()
	if err != nil {
		logger.Warn("can't find this switchboard binary, so the dashboard won't restart onto an upgrade", "error", err)
		return watch.Upgrade{}
	}
	if len(a.Args) == 0 {
		logger.Warn("can't tell how the dashboard was started, so it won't restart onto an upgrade")
		return watch.Upgrade{}
	}
	argv, env := slices.Clone(a.Args), a.Environ()
	return watch.Upgrade{
		Version: a.Version,
		Binary:  router.Watch(exe),
		Look:    func() filestate.State { return filestate.Of(exe) },
		Health:  a.routersHealth,
		Ask: func(ctx context.Context) (string, error) {
			out, err := a.Check(ctx, exe, []string{argv[0], "--version"}, env)
			if err != nil {
				return "", err
			}
			return versionIn(out)
		},
		Exec: func(pool string) error {
			return a.Exec(exe, showingPool(argv, pool), env)
		},
	}
}

// showingPool is argv, the command line a dashboard was run by, as os.Args
// gives it, showing the pool with the given id: --pool naming it in place of
// any it was run with, or none for "", before any -- ending the flags.
func showingPool(argv []string, pool string) []string {
	flags, rest := argv, []string(nil)
	if end := slices.Index(argv, "--"); end >= 0 {
		flags, rest = argv[:end], argv[end:]
	}
	named := "--" + poolFlag
	shown := make([]string, 0, len(argv)+1)
	for i := 0; i < len(flags); i++ {
		switch {
		case flags[i] == named:
			i++
		case strings.HasPrefix(flags[i], named+"="):
		default:
			shown = append(shown, flags[i])
		}
	}
	if pool != "" {
		shown = append(shown, named+"="+pool)
	}
	return append(shown, rest...)
}
