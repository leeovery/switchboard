package cli

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/leeovery/switchboard/internal/status"
)

// The commands read a scratchpad's project against the filesystem, home,
// temporary directory and user they're given; with no filesystem, as
// status.ProjectOf reads it.
func TestTheCommandsReadProjectsAgainstTheMachineTheyreGiven(t *testing.T) {
	pad := "/scratch/tmp/claude-501/-Users-you-Code-api/0b5c6f2e-6a1d-4c3e-9f7a-2b8d4e6f1a3c/scratchpad"
	dir := &fstest.MapFile{Mode: fs.ModeDir}
	machine := fstest.MapFS{"Users/you/Code/api/.git": dir, "Users/you/Code/api/.claude/worktrees": dir, "scratch/tmp/claude-501": dir}
	getenv := func(key string) string {
		if key == "TMPDIR" {
			return "/var/run/tmp/"
		}
		return ""
	}
	given := Deps{
		Root: machine, HomeDir: func() (string, error) { return "/Users/you", nil }, Getenv: getenv, UID: 501,
		FindProject: func(dir string) (string, bool) { return dir, false },
		RepositoryOf: func(dir string) (string, bool) {
			return "/Users/you/Code/api", dir == "/Users/you/Code/api/.claude/worktrees"
		},
	}
	removed := "/scratch/tmp/claude-501/-Users-you-Code-api--claude-worktrees-fix/0b5c6f2e-6a1d-4c3e-9f7a-2b8d4e6f1a3c/scratchpad"
	tests := []struct {
		name string
		deps Deps
		dir  string
		want string
	}{
		{name: "a scratchpad, its project", deps: given, dir: pad, want: "~/Code/api"},
		{name: "a scratchpad of a worktree since removed, its repository", deps: given, dir: removed, want: "~/Code/api"},
		{name: "a folder in the temporary directory, temporary folders", deps: given, dir: "/var/run/tmp/call-1", want: status.TemporaryFolders},
		{name: "a folder beside the scratch root, of the user's id, temporary folders", deps: given, dir: "/scratch/tmp/call-1", want: status.TemporaryFolders},
		{name: "no filesystem, a scratchpad as itself", deps: Deps{}, dir: pad, want: pad},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &app{Deps: tt.deps}
			if got := a.projects().Of("", tt.dir); got != tt.want {
				t.Errorf("projects().Of(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}
