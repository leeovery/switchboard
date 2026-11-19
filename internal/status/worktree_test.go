package status_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

func TestAWorktreeClaudeCodeMadeIsNamedByItsFolder(t *testing.T) {
	tests := []struct {
		name, dir, want string
		in              bool
	}{
		{name: "its root", dir: "~/Code/api/.claude/worktrees/fix-login", want: "fix-login", in: true},
		{name: "a folder within it", dir: "~/Code/api/.claude/worktrees/fix-login/web/src", want: "fix-login", in: true},
		{name: "written two ways, cleaned", dir: "~/Code/api//.claude/worktrees/./fix-login/", want: "fix-login", in: true},
		{name: "the innermost, of one made in another's", dir: "/code/api/.claude/worktrees/a/.claude/worktrees/b", want: "b", in: true},
		{name: "the worktrees' folder itself, none", dir: "~/Code/api/.claude/worktrees"},
		{name: "a repository, none", dir: "~/Code/api"},
		{name: "a folder named worktrees elsewhere, none", dir: "~/Code/worktrees/fix-login"},
		{name: "the worktrees' folder at the root, of no repository, none", dir: "/.claude/worktrees/fix-login"},
		{name: "none for none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, in := status.WorktreeName(tt.dir)
			if got != tt.want || in != tt.in {
				t.Errorf("WorktreeName(%q) = %q, %v, want %q, %v", tt.dir, got, in, tt.want, tt.in)
			}
		})
	}
}
