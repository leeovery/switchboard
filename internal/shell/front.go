package shell

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/redact"
)

// Runs is what an alias or function named claude, in front of PATH's claude,
// runs, as far as its text tells.
type Runs int

const (
	// ByName runs claude by name, so PATH's claude decides: an alias that
	// does, or nothing in front at all.
	ByName Runs = iota
	// Unknown may run claude by name or not: a function, whose body the
	// shell doesn't give, a wrapper setup doesn't read, or an alias that
	// runs what only its expansion tells.
	Unknown
	// TheLink runs switchboard's claude link by its path.
	TheLink
	// APath runs another claude by its path.
	APath
	// Else runs something else than claude.
	Else
	// SetsPath runs claude by name on a PATH of its own, which the alias
	// sets, so which claude it runs can't be told.
	SetsPath
)

// Front is an alias or function named claude, standing in front of PATH's
// claude.
type Front struct {
	// Text is how the shell gives it, any token in it hidden: "" where
	// nothing's in front.
	Text string
	// Runs is what it runs.
	Runs Runs
	// Path is the other claude an alias runs by its path, any token in it
	// hidden.
	Path string
}

// readFront reads is, what sh says claude is, home being the user's home and
// link switchboard's claude link. An alias is text, so it's decided by the
// command it runs; a function isn't.
func readFront(sh Shell, is, home, link string) Front {
	switch {
	case is == "" || filepath.IsAbs(is):
		return Front{}
	case is == "claude" || is == "function claude":
		// command -v gives a function by its name alone, as fish is asked
		// to give it.
		return Front{Text: "function claude", Runs: Unknown}
	}
	f := Front{Text: redact.Text(is), Runs: Unknown}
	value, isAlias := strings.CutPrefix(is, "alias claude=")
	if !isAlias {
		return f
	}
	if body, ok := unquote(value); ok {
		f.Runs, f.Path = alias{sh: sh, home: home, link: link}.runs(body)
	}
	return f
}

// alias reads an alias's body in sh, home being the user's home and link
// switchboard's claude link.
type alias struct {
	sh         Shell
	home, link string
}

// runs returns what body runs, and the other claude it runs by its path. Its
// first command decides, as first says; but where that runs something else
// without deciding it, a later word of the alias, in any of its commands,
// that's claude or a path to one makes it a wrapper setup doesn't read.
func (a alias) runs(body string) (Runs, string) {
	cmds, ok := commands(body)
	if !ok {
		return Unknown, ""
	}
	runs, path, after, decided := a.first(cmds[0])
	switch {
	case decided:
		return runs, path
	case slices.ContainsFunc(after, namesClaude) || slices.ContainsFunc(slices.Concat(cmds[1:]...), namesClaude):
		return Unknown, ""
	}
	return Else, ""
}

// first returns what ws, an alias's first command, runs, the other claude it
// runs by its path, the words after its command word, and whether that
// decides what the alias runs. Variables set first are passed over. Then its
// command word decides: claude, or command claude without an option, by
// name; a path by itself; builtin, or command with an option or alone, runs
// no claude, or not the one PATH finds; and one only its expansion tells,
// setup can't read. A PATH set before claude, by a variable or a word after
// another command, as env's, runs claude where only that PATH tells.
func (a alias) first(ws []word) (runs Runs, path string, after []word, decided bool) {
	n := 0
	for n < len(ws) && ws[n].assigns() {
		n++
	}
	pathSet := slices.ContainsFunc(ws[:n], a.setsPath)
	cmd := ws[n:]
	if len(cmd) > 0 && cmd[0].text == "command" {
		cmd = cmd[1:]
		if len(cmd) > 0 && cmd[0].text == "--" {
			cmd = cmd[1:]
		}
		if len(cmd) == 0 || strings.HasPrefix(cmd[0].text, "-") {
			return Else, "", nil, true
		}
	}
	if len(cmd) == 0 {
		return Else, "", nil, false
	}
	runs, path, decided = a.command(cmd[0])
	if decided && runs == ByName && pathSet || !decided && a.pathBeforeClaude(pathSet, cmd[1:]) {
		return SetsPath, "", nil, true
	}
	return runs, path, cmd[1:], decided
}

// pathBeforeClaude reports whether claude is among ws after a word that sets
// PATH, or after any where set.
func (a alias) pathBeforeClaude(set bool, ws []word) bool {
	for _, w := range ws {
		if set && w.text == "claude" {
			return true
		}
		set = set || a.setsPath(w)
	}
	return false
}

// setsPath reports whether w sets PATH, or adds to it, in sh, as zsh's path
// does too.
func (a alias) setsPath(w word) bool {
	name := strings.TrimSuffix(strings.TrimSuffix(assignment.FindString(w.text), "="), "+")
	return name == "PATH" || a.sh == Zsh && name == "path"
}

// command returns what w, the command word an alias runs, is, and the other
// claude it names by its path, and whether that decides what the alias runs.
// zsh's =claude is PATH's claude by its path.
func (a alias) command(w word) (Runs, string, bool) {
	path, isPath, plain := a.path(w)
	switch {
	case !plain:
		return Unknown, "", true
	case w.text == "claude" || a.sh == Zsh && w.equals("claude"):
		return ByName, "", true
	case isPath && sameFile(path, a.link):
		return TheLink, "", true
	case isPath:
		return APath, redact.Text(path), true
	case w.text == "builtin":
		return Else, "", true
	}
	return 0, "", false
}

// path returns the path w names, absolute or from the home, and whether it
// names one; and whether it can be read, with no expansion in it but the
// home it starts from.
func (a alias) path(w word) (string, bool, bool) {
	rest, fromHome, plain := w.home()
	switch {
	case !plain || w.expands(len(w.text)-len(rest)) || fromHome && !filepath.IsAbs(a.home):
		return "", false, false
	case fromHome:
		return filepath.Join(a.home, rest), true, true
	}
	return w.text, filepath.IsAbs(w.text), true
}
