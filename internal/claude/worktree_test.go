package claude_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

// Claude Code's worktree flag names the worktree it starts in by the value
// that follows it, or after =, the last flag's where it's given twice;
// none where it has no value, as Claude Code then makes a name up, nor
// where the value isn't plainly a name, nor past "--".
func TestTheWorktreeFlagNamesTheWorktreeClaudeCodeStartsIn(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "the long flag", args: []string{"--worktree", "feature-auth"}, want: "feature-auth"},
		{name: "the short flag", args: []string{"-w", "feature-auth"}, want: "feature-auth"},
		{name: "after =", args: []string{"--worktree=feature-auth"}, want: "feature-auth"},
		{name: "among others", args: []string{"--model", "opus", "-w", "fix_2.1", "--resume"}, want: "fix_2.1"},
		{name: "the last of two", args: []string{"-w", "first", "--worktree", "second"}, want: "second"},
		{name: "none given"},
		{name: "none past --", args: []string{"--", "-w", "feature-auth"}},
		{name: "the flag without a name, at the end", args: []string{"-w"}},
		{name: "the flag without a name, before another flag", args: []string{"--worktree", "--resume"}},
		{name: "a prompt after the flag, of more than a name", args: []string{"-w", "fix the login bug"}},
		{name: "a path", args: []string{"-w", "../elsewhere"}},
		{name: "a folder of a folder", args: []string{"--worktree=feature/auth"}},
		{name: "up a folder", args: []string{"-w", ".."}},
		{name: "empty after =", args: []string{"--worktree="}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := claude.WorktreeArg(tt.args)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("WorktreeArg(%q) = %q, %v; want %q, %v", tt.args, got, ok, tt.want, tt.want != "")
			}
		})
	}
}

func TestAWorktreeIsMadeInTheRepositorysWorktreesFolder(t *testing.T) {
	if got, want := claude.WorktreePath("/code/api", "feature-auth"), "/code/api/.claude/worktrees/feature-auth"; got != want {
		t.Errorf("WorktreePath() = %q, want %q", got, want)
	}
}
