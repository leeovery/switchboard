//go:build darwin

package launch_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/launch"
)

// laidOutProcesses is processes, but with the environment of each process
// args holds read from what kern.procargs2 gives of it, as ProcessTable reads
// it.
type laidOutProcesses struct {
	processes
	args map[int][]byte
}

func (p laidOutProcesses) Environ(pid int) ([]string, bool) {
	if args, ok := p.args[pid]; ok {
		return launch.SwitchboardsOf(args)
	}
	return p.processes.Environ(pid)
}

// A claude started in a scratchpad within a session whose claude Node runs,
// its title written over what's read of that claude, starts in the session's
// pool and project, through a whole launch. Where the title took the
// variables ahead of the session's project, its pool among them, the
// environment the launch inherits gives the pool.
func TestALaunchWithinASessionWhoseClaudeNodeRunsIsInItsPool(t *testing.T) {
	const node = "/usr/local/bin/node"
	argv := []string{node, "/usr/local/lib/node_modules/@anthropic-ai/claude-code/cli.js", "--resume"}
	cfg := pooledConfig()
	sessionEnv := session(t, cfg, acmeDir)
	cleaned := []string{"HOME=/home/tester"}
	laid := func() []byte { return launch.LaidOut(node, argv, sessionEnv, "\x00ptr_munge=\x00") }
	aheadOfProject := launch.TitledAhead(laid(), node, "claude", "SWITCHBOARD_PROJECT")
	tests := []struct {
		name string
		// args is the session's claude's, as kern.procargs2 gives them, and
		// environ the environment the launch within it starts from.
		args      []byte
		environ   []string
		inSession bool
	}{
		{name: "its title written over its arguments, the launch's environment cleaned", args: launch.Titled(laid(), node, argv, "claude"), environ: cleaned, inSession: true},
		{name: "its title written over all ahead of its project, the launch inheriting the session's environment", args: aheadOfProject, environ: sessionEnv, inSession: true},
		{name: "its title written over all ahead of its project, the launch's environment cleaned", args: aheadOfProject, environ: cleaned},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := within(t, tt.environ, sessionEnv, scratchpad)
			h.launcher.Processes = laidOutProcesses{processes: sessionTable(sessionEnv), args: map[int][]byte{pid: tt.args}}

			runs(t, h, cfg)
			if tt.inSession {
				placedIn(t, h.environment(t), acmePool.ID, acme1Token, acmeDir)
				return
			}
			placedIn(t, h.environment(t), defaultPool.ID, workToken, scratchpad)
		})
	}
}
