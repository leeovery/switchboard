package claude

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The flags that have Claude Code make a worktree of the repository it
// starts in, or open one it made before, and start in it: -w or --worktree,
// with a name or without, as claude --worktree feature-auth.
const (
	worktreeFlag      = "--worktree"
	worktreeFlagShort = "-w"
)

// A worktree's name as Claude Code takes one, as 2.1.327 does: segments
// worktreeSegment matches, "/" apart, none of them reserved, as "." and ".."
// would leave the folder and ".git" is git's own, worktreeNameMost
// characters at the most in all. Claude Code makes its folder of the
// segments joined by worktreeJoin, so feature/auth is made in feature+auth.
var (
	worktreeSegment  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	worktreeReserved = []string{".", "..", ".git"}
)

const (
	worktreeNameMost = 64
	worktreeJoin     = "+"
)

// WorktreeArg returns the folder, within WorktreesDir in the repository's
// root, of the worktree Claude Code, run with args, makes and starts in, as
// its docs say: of the name the last -w or --worktree before "--" gives, as
// the next argument, one that doesn't start with "-", as Claude Code's
// options take an optional value, or after "=", the folder made of it as
// worktreeFolder has it. It reports none where no such flag is given, where
// one is given without a name, as Claude Code then makes up one of its own,
// and where the name isn't one worktreeFolder takes, as a prompt after the
// flag, or a pull request's number or address, which Claude Code makes a
// worktree named pr-<number> of, isn't.
func WorktreeArg(args []string) (folder string, ok bool) {
	var name string
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
	if !ok {
		return "", false
	}
	return worktreeFolder(name)
}

// worktreeFolder is the folder Claude Code makes the worktree with the given
// name in, as a worktree's name is taken: its segments joined by
// worktreeJoin; false for a name it doesn't take so.
func worktreeFolder(name string) (string, bool) {
	if name == "" || len(name) > worktreeNameMost {
		return "", false
	}
	segments := strings.Split(name, "/")
	for _, s := range segments {
		if !worktreeSegment.MatchString(s) || slices.Contains(worktreeReserved, s) {
			return "", false
		}
	}
	return strings.Join(segments, worktreeJoin), true
}

// WorktreePath is where Claude Code makes the worktree in the given folder,
// as WorktreeArg gives it, of the repository whose root is given.
func WorktreePath(root, folder string) string {
	return filepath.Join(root, WorktreesDir, folder)
}
