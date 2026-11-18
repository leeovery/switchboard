package claude

import (
	"path/filepath"
	"regexp"
	"strings"
)

// The flags that have Claude Code make a worktree of the repository it
// starts in, or open one it made before, and start in it: -w or --worktree,
// with a name or without, as claude --worktree feature-auth.
const (
	worktreeFlag      = "--worktree"
	worktreeFlagShort = "-w"
)

// worktreeNamed matches the worktree names WorktreeArg takes: letters, digits,
// dots, dashes and underscores. Claude Code's docs give no rule for the
// name, so one of any other character, which it may write otherwise, isn't
// guessed at.
var worktreeNamed = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// WorktreeArg returns the name of the worktree Claude Code, run with args,
// makes and starts in, at WorktreesDir/<name> within the repository's root,
// as its docs say: the value of the last -w or --worktree before "--", given
// as the next argument, one that doesn't start with "-", as Claude Code's
// options take an optional value, or after "=". It reports none where no
// such flag is given, where one is given without a name, as Claude Code
// then makes up one of its own, and where the name isn't plainly one
// worktreeNamed matches, "." or "..", as where it's a prompt that follows
// the flag, which the path can't be told of.
func WorktreeArg(args []string) (name string, ok bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if value, given := strings.CutPrefix(arg, worktreeFlag+"="); given {
			name, ok = value, true
			continue
		}
		if arg != worktreeFlag && arg != worktreeFlagShort {
			continue
		}
		name, ok = "", true
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			name = args[i]
		}
	}
	if !ok || !worktreeNamed.MatchString(name) || name == "." || name == ".." {
		return "", false
	}
	return name, true
}

// WorktreePath is where Claude Code makes the worktree with the given name
// of the repository whose root is given, as WorktreeArg's name has it.
func WorktreePath(root, name string) string {
	return filepath.Join(root, WorktreesDir, name)
}
