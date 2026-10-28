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

// runs returns what body runs, and the other claude it runs by its path.
// Only a first command word read whole, in a plain form, decides it, as
// first says. Otherwise a PATH set before a later claude, in any of its
// commands, runs claude where only that PATH tells; and a later claude, or
// what isn't read whole, makes it a wrapper setup doesn't read. With
// neither, it runs something else.
func (a alias) runs(body string) (Runs, string) {
	cmds, ok := commands(body)
	if !ok {
		return Unknown, ""
	}
	runs, path, decided := a.first(cmds[0])
	switch {
	case decided:
		return runs, path
	case a.pathBeforeClaude(cmds):
		return SetsPath, ""
	case slices.ContainsFunc(later(cmds), mayRunClaude):
		return Unknown, ""
	}
	return Else, ""
}

// first returns what ws, an alias's first command, runs, the other claude it
// runs by its path, and whether that decides what the alias runs.
// Variables set first are passed over, and command with no option but --.
// Then its command word decides where it's claude, or zsh's =claude, by
// name, but with a variable before it that sets PATH, on that PATH; or a
// path by itself; or one only its expansion tells, setup can't read.
func (a alias) first(ws []word) (Runs, string, bool) {
	n := leading(ws)
	cmd := ws[n:]
	if len(cmd) > 1 && cmd[0].text == "command" {
		rest := cmd[1:]
		if rest[0].text == "--" {
			rest = rest[1:]
		}
		if len(rest) > 0 && !strings.HasPrefix(rest[0].text, "-") {
			cmd = rest
		}
	}
	if len(cmd) == 0 {
		return Else, "", false
	}
	runs, path, decided := a.command(cmd[0])
	if decided && runs == ByName && slices.ContainsFunc(ws[:n], a.setsPath) {
		return SetsPath, "", true
	}
	return runs, path, decided
}

// leading returns how many of ws, a command, are variables set for it.
func leading(ws []word) int {
	n := 0
	for n < len(ws) && ws[n].assigns() {
		n++
	}
	return n
}

// later returns the words of cmds, an alias's commands, after its first
// command word.
func later(cmds [][]word) []word {
	first := cmds[0]
	return slices.Concat(first[min(leading(first)+1, len(first)):], slices.Concat(cmds[1:]...))
}

// pathBeforeClaude reports whether, in cmds, a word that sets PATH comes
// before a later claude: a variable set for a command, or an argument after
// it, as env's and export's.
func (a alias) pathBeforeClaude(cmds [][]word) bool {
	set := false
	for _, ws := range cmds {
		n := leading(ws)
		for i, w := range ws {
			if set && namesClaude(w) {
				return true
			}
			set = set || i < n && a.setsPath(w) || i > n && w.setsPATH()
		}
	}
	return false
}

// setsPath reports whether w, a variable set for a command, sets PATH, or
// adds to it, in sh, as zsh's path does too.
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
	}
	return 0, "", false
}

// path returns the path w names, absolute or from the home, and whether it
// names one; and whether it's plain, as word.plain says, but for the home
// it starts from.
func (a alias) path(w word) (string, bool, bool) {
	rest, fromHome, readable := w.home()
	switch {
	case !readable || !w.plain(len(w.text)-len(rest)) || fromHome && !filepath.IsAbs(a.home):
		return "", false, false
	case fromHome:
		return filepath.Join(a.home, rest), true, true
	}
	return w.text, filepath.IsAbs(w.text), true
}
