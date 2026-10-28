package shell

import (
	"path/filepath"
	"regexp"
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
	// shell doesn't give, or an alias that runs what only its expansion
	// tells.
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
	if body, ok := words(value); ok {
		f.Runs, f.Path = aliasRuns(sh, strings.Join(body, " "), home, link)
	}
	return f
}

// precommand is a word that runs the command after it, in the shells it
// names, every shell where it names none. It takes options in the shells
// options names, the letters of those that take a word of their own after
// them being withArg; and variables set for the command after it where
// assignments is set, as only a reserved word does.
type precommand struct {
	shells      []Shell
	options     []Shell
	withArg     string
	assignments bool
}

// precommands run the command after them. builtin isn't one: it runs only
// the shell's own commands, which claude isn't.
var precommands = map[string]precommand{
	"command":    {options: Shells},
	"exec":       {options: Shells, withArg: "a"},
	"time":       {options: []Shell{Bash}, assignments: true},
	"nohup":      {},
	"caffeinate": {options: Shells, withArg: "tw"},
	"noglob":     {shells: []Shell{Zsh}},
	"nocorrect":  {shells: []Shell{Zsh}, assignments: true},
}

// in reports whether p is a precommand in sh.
func (p precommand) in(sh Shell) bool {
	return p.shells == nil || slices.Contains(p.shells, sh)
}

// assignment is a word that sets a variable, or adds to it, for the command
// after it.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?=`)

// setsPath reports whether w, an assignment, sets PATH in sh, as zsh's path
// does too.
func setsPath(sh Shell, w string) bool {
	name, _, _ := strings.Cut(strings.Replace(w, "+=", "=", 1), "=")
	return name == "PATH" || sh == Zsh && name == "path"
}

// aliasRuns returns what body, an alias's in sh, runs, and the other claude
// it runs by its path: its command, its first word, past any variables set
// for it, and any precommand sh has, with its options. One that sets PATH
// runs claude where only that PATH tells; command -p looks on the system's
// PATH, and -v and -V print rather than run; and after a precommand that
// takes none, an assignment is the command.
func aliasRuns(sh Shell, body, home, link string) (Runs, string) {
	ws, ok := words(body)
	if !ok {
		return Unknown, ""
	}
	pre, takesArg := "", false
	for _, w := range ws {
		p, after := precommands[pre]
		switch {
		case takesArg:
			takesArg = false
		case assignment.MatchString(w) && (!after || p.assignments):
			if setsPath(sh, w) {
				return SetsPath, ""
			}
		case assignment.MatchString(w):
			return Else, ""
		case after && slices.Contains(p.options, sh) && len(w) > 1 && strings.HasPrefix(w, "-"):
			if pre == "command" && strings.ContainsAny(w, "pvV") {
				return Else, ""
			}
			takesArg = strings.ContainsRune(p.withArg, rune(w[len(w)-1]))
		default:
			if q, ok := precommands[w]; ok && q.in(sh) {
				pre = w
				continue
			}
			return command(w, home, link)
		}
	}
	return Unknown, ""
}

// command returns what word, the command an alias runs, is, and the other
// claude it names by its path.
func command(word, home, link string) (Runs, string) {
	path, isPath := fromHome(word, home)
	switch {
	case word == "claude":
		return ByName, ""
	case isPath && sameFile(path, link):
		return TheLink, ""
	case isPath:
		return APath, redact.Text(path)
	case strings.ContainsAny(word, "$`") || word == "env":
		// env's options can take words of their own, so what it runs is
		// left undecided.
		return Unknown, ""
	}
	return Else, ""
}

// words splits s, a simple command, into its words, as a shell does before
// it expands them, quotes and backslashes removed: up to the end of the
// command, at an unquoted ;, &, |, <, >, ( or ). It fails at a quote that
// isn't closed.
func words(s string) ([]string, bool) {
	var ws []string
	var w strings.Builder
	inWord := false
	end := func() {
		if inWord {
			ws = append(ws, w.String())
		}
		w.Reset()
		inWord = false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case strings.IndexByte(" \t\n", c) >= 0:
			end()
			continue
		case strings.IndexByte(";&|<>()", c) >= 0:
			end()
			return ws, true
		case c == '\\' && i+1 < len(s):
			i++
			w.WriteByte(s[i])
		case c == '\'':
			closing := strings.IndexByte(s[i+1:], '\'')
			if closing < 0 {
				return nil, false
			}
			w.WriteString(s[i+1 : i+1+closing])
			i += closing + 1
		case c == '"':
			n, ok := doubleQuotedWord(s[i+1:], &w)
			if !ok {
				return nil, false
			}
			i += n + 1
		default:
			w.WriteByte(c)
		}
		inWord = true
	}
	end()
	return ws, true
}

// doubleQuotedWord writes to w what s holds up to the double quote that
// closes it, as a shell reads it, a backslash escaping only $, `, " and \;
// and returns how far into s that quote is, and whether there's one.
func doubleQuotedWord(s string, w *strings.Builder) (int, bool) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			return i, true
		case c == '\\' && i+1 < len(s) && strings.IndexByte("$`\"\\", s[i+1]) >= 0:
			i++
			w.WriteByte(s[i])
		default:
			w.WriteByte(c)
		}
	}
	return 0, false
}

// fromHome returns the path word names, written from the home, as with ~ or
// $HOME, or absolute; and whether it names one.
func fromHome(word, home string) (string, bool) {
	if filepath.IsAbs(word) {
		return word, true
	}
	for _, prefix := range []string{"~/", "$HOME/", "${HOME}/"} {
		if rest, ok := strings.CutPrefix(word, prefix); ok && filepath.IsAbs(home) {
			return filepath.Join(home, rest), true
		}
	}
	return "", false
}
