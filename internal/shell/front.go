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

// readFront reads is, what the shell says claude is, home being the user's
// home and link switchboard's claude link. An alias is text, so it's
// decided by the command it runs; a function isn't.
func readFront(is, home, link string) Front {
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
	body, ok := words(value)
	if !ok {
		return f
	}
	word, ok := commandWord(strings.Join(body, " "))
	if !ok {
		return f
	}
	path, isPath := fromHome(word, home)
	switch {
	case word == "claude":
		f.Runs = ByName
	case isPath && sameFile(path, link):
		f.Runs = TheLink
	case isPath:
		f.Runs, f.Path = APath, redact.Text(path)
	case !strings.ContainsAny(word, "$`") && word != "env":
		// env's options can take words of their own, so what it runs is
		// left undecided.
		f.Runs = Else
	}
	return f
}

// precommands run the command after them.
var precommands = []string{"command", "builtin", "noglob", "nocorrect"}

// assignment is a word that sets a variable for the command after it.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// commandWord returns the command body runs, its first word, quotes removed,
// past any variables set for it and any precommand; and whether it has one.
func commandWord(body string) (string, bool) {
	ws, ok := words(body)
	if !ok {
		return "", false
	}
	pre := false
	for _, w := range ws {
		switch {
		case assignment.MatchString(w), pre && strings.HasPrefix(w, "-"):
		case slices.Contains(precommands, w):
			pre = true
		default:
			return w, true
		}
	}
	return "", false
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
