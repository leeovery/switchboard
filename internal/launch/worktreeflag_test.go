package launch_test

import (
	"slices"
	"testing"
)

// claude run to make a worktree, or open one, by its name, tells the router
// the worktree Claude Code then starts in, in the repository the directory
// it's run in is in, the project that repository, as Claude Code makes it
// only once it starts; run with the flag but no name, or outside a
// repository, the directory it's run in. Claude Code is given its
// arguments as they were.
func TestRunTellsTheRouterTheWorktreeClaudeCodeMakes(t *testing.T) {
	api := homeDir + "/Code/api"
	tests := []struct {
		name string
		args []string
		// dir is the directory claude is run in, and found the project
		// Project finds it in, inRepository whether it's in one.
		dir, found   string
		inRepository bool
		// want are the headers Claude Code starts with.
		want string
	}{
		{
			name: "named, in its repository", args: []string{"--worktree", "fix-login"}, dir: api, found: api, inRepository: true,
			want: "X-Switchboard-Dir: ~/Code/api/.claude/worktrees/fix-login\nX-Switchboard-Project: ~/Code/api",
		},
		{
			name: "named, in a folder of its repository", args: []string{"-w", "fix-login"}, dir: api + "/web/src", found: api, inRepository: true,
			want: "X-Switchboard-Dir: ~/Code/api/.claude/worktrees/fix-login\nX-Switchboard-Project: ~/Code/api",
		},
		{
			name: "named, in another worktree, its repository's", args: []string{"--worktree=review"}, dir: api + "/.claude/worktrees/fix-login", found: api, inRepository: true,
			want: "X-Switchboard-Dir: ~/Code/api/.claude/worktrees/review\nX-Switchboard-Project: ~/Code/api",
		},
		{
			name: "named none, the directory", args: []string{"-w", "--resume"}, dir: api, found: api, inRepository: true,
			want: "X-Switchboard-Dir: ~/Code/api\nX-Switchboard-Project: ~/Code/api",
		},
		{
			name: "outside a repository, the directory", args: []string{"-w", "fix-login"}, dir: homeDir + "/notes", found: homeDir + "/notes",
			want: "X-Switchboard-Dir: ~/notes\nX-Switchboard-Project: ~/notes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.Dir = tt.dir
			h.launcher.Project = func(string) (string, bool) { return tt.found, tt.inRepository }

			if err := h.launcher.Run(t.Context(), route(healthy(), ""), tt.args); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.environment(t)["ANTHROPIC_CUSTOM_HEADERS"]; got != tt.want {
				t.Errorf("started with ANTHROPIC_CUSTOM_HEADERS %q, want %q", got, tt.want)
			}
			if got, want := h.only(t).argv, append([]string{"claude"}, tt.args...); !slices.Equal(got, want) {
				t.Errorf("started claude as %q, want %q, its arguments as given", got, want)
			}
		})
	}
}
