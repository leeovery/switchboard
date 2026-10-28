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
	// shell doesn't give, or an alias setup doesn't read.
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

// unquote returns value, an alias's as zsh and bash print it, unquoted: bare,
// or single quoted strings and escaped quotes, one after another, as bash
// writes a quote within the value, closing the string, escaping one and
// opening another, and zsh does too, leaving out a string that'd be empty.
// Any other form it isn't read in.
func unquote(value string) (string, bool) {
	if !strings.ContainsAny(value, `'"\`) {
		return value, true
	}
	var body strings.Builder
	for value != "" {
		if rest, ok := strings.CutPrefix(value, `\'`); ok {
			body.WriteByte('\'')
			value = rest
			continue
		}
		quoted, rest, ok := strings.Cut(strings.TrimPrefix(value, "'"), "'")
		if !ok || !strings.HasPrefix(value, "'") {
			return "", false
		}
		body.WriteString(quoted)
		value = rest
	}
	return body.String(), true
}

const (
	// plainly is what a plain word is made of.
	plainly = `[A-Za-z0-9_./:@%+,=-]`
	// fromHome is a path from the home, as the shells expand it.
	fromHome = `(~|\$HOME|\$\{HOME\})/`
	// valueExpansion is what a variable's value may hold beside plain
	// characters: PATH or HOME, braced, or bare and ending where a name
	// can't go on.
	valueExpansion = `\$\{(?:PATH|HOME)\}|\$(?:PATH|HOME)(?:[./:@%+,=-]|$)`
)

var (
	// plain is a word the shells run as it's written, or a path from the
	// home.
	plain = regexp.MustCompile(`^(` + plainly + `+|` + fromHome + plainly + `*)$`)
	// homePath is a path from the home, and what follows the home.
	homePath = regexp.MustCompile(`^` + fromHome + `(` + plainly + `*)$`)
	// assignment is a word that sets a variable, or adds to it, to plain
	// characters, PATH and HOME among them, and the variable's name.
	assignment = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\+?=(?:~/)?(?:` + plainly + `|` + valueExpansion + `)*$`)
)

// alias reads an alias's body in sh, home being the user's home and link
// switchboard's claude link.
type alias struct {
	sh         Shell
	home, link string
}

// runs returns what body runs, and the other claude it runs by its path, by
// a plain rule. Its words, split at spaces and tabs, are read only in their
// plain forms. Leading variables are passed over, and command. Then the
// command word decides where it's claude, by name, though on a PATH of its
// own where a variable set PATH; or a clean path to a claude, by itself, as
// command says. Whatever follows either, claude runs first. Anything else
// runs something else where every word after the variables is plain and
// none names claude; otherwise setup doesn't read it.
func (a alias) runs(body string) (Runs, string) {
	ws := strings.FieldsFunc(body, func(r rune) bool { return r == ' ' || r == '\t' })
	setsPath := false
	n := 0
	for ; n < len(ws); n++ {
		m := assignment.FindStringSubmatch(ws[n])
		if m == nil {
			break
		}
		setsPath = setsPath || m[1] == "PATH" || a.sh == Zsh && m[1] == "path"
	}
	cmd := ws[n:]
	if len(cmd) > 1 && cmd[0] == "command" {
		cmd = cmd[1:]
	}
	if len(cmd) > 0 {
		if runs, path, ok := a.command(cmd[0], setsPath); ok {
			return runs, path
		}
	}
	if slices.ContainsFunc(ws, claudeToken) || slices.ContainsFunc(ws[n:], func(w string) bool { return !plain.MatchString(w) }) {
		return Unknown, ""
	}
	return Else, ""
}

// command returns what w, an alias's command word, runs, setsPath being
// whether a variable before it sets PATH; and whether it decides that. The
// link runs on that PATH too, as it starts claude by it.
func (a alias) command(w string, setsPath bool) (Runs, string, bool) {
	path, isPath, clean := a.path(w)
	switch {
	case (w == "claude" || w == `\claude`) && setsPath:
		return SetsPath, "", true
	case w == "claude" || w == `\claude`:
		return ByName, "", true
	case isPath && !clean:
		return Unknown, "", true
	case !isPath || filepath.Base(path) != "claude":
		return 0, "", false
	case !sameFile(path, a.link):
		return APath, redact.Text(path), true
	case setsPath:
		return SetsPath, "", true
	}
	return TheLink, "", true
}

// path returns the path w, a plain word, names, absolute or from the home;
// whether it names one; and whether it's written clean, with no trailing /
// nor . or .. in it, as the shell runs only what it names.
func (a alias) path(w string) (string, bool, bool) {
	if m := homePath.FindStringSubmatch(w); m != nil {
		return filepath.Join(a.home, m[2]), filepath.IsAbs(a.home), filepath.Clean(m[2]) == m[2]
	}
	return w, filepath.IsAbs(w) && plain.MatchString(w), filepath.Clean(w) == w
}

// claudeToken reports whether w names claude, as one of its parts between
// a /, =, : or ,.
func claudeToken(w string) bool {
	return slices.Contains(strings.FieldsFunc(w, func(r rune) bool { return strings.ContainsRune("/=:,", r) }), "claude")
}
