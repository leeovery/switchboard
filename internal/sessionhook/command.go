package sessionhook

import (
	"path/filepath"
	"strings"
)

// subcommand is what the hook runs switchboard with.
const subcommand = "hook session-start"

// binaryName is the name switchboard's binary goes by, which the path in
// one of its hooks ends with.
const binaryName = "switchboard"

// Command returns the command the hook runs: switchboard at path, unless
// nothing there can be run, as when switchboard is removed, which leaves the
// hook doing nothing, and succeeding.
func Command(switchboard string) string {
	quoted := shellQuoted(switchboard)
	return "[ -x " + quoted + " ] && " + quoted + " " + subcommand + " || true"
}

// shellQuoted quotes s as one word for sh: between single quotes, each
// single quote in it ended, escaped and begun again.
func shellQuoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// isOurCommand reports whether command is one Command gives, for a binary
// named switchboard at any path: the path the same in both places it's
// named. Anything else, a hook of the user's that merely mentions
// switchboard included, isn't switchboard's.
func isOurCommand(command string) bool {
	rest, ok := strings.CutPrefix(command, "[ -x ")
	if !ok {
		return false
	}
	guarded, rest, ok := cutQuoted(rest)
	if !ok {
		return false
	}
	if rest, ok = strings.CutPrefix(rest, " ] && "); !ok {
		return false
	}
	run, rest, ok := cutQuoted(rest)
	return ok && rest == " "+subcommand+" || true" && run == guarded && filepath.Base(run) == binaryName
}

// cutQuoted cuts a word quoted as shellQuoted quotes it from the start of s,
// returning the word and what follows it.
func cutQuoted(s string) (word, rest string, ok bool) {
	var b strings.Builder
	for {
		inside, ok := strings.CutPrefix(s, "'")
		if !ok {
			return "", "", false
		}
		part, after, ok := strings.Cut(inside, "'")
		if !ok {
			return "", "", false
		}
		b.WriteString(part)
		if s, ok = strings.CutPrefix(after, `\'`); !ok {
			return b.String(), after, true
		}
		b.WriteByte('\'')
	}
}
