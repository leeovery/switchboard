package shell

import (
	"regexp"
	"slices"
	"strings"
)

// quoting is how a byte of a word was quoted, which decides what the shell
// expands in it.
type quoting uint8

const (
	// bare is unquoted: ~, $, ` and a NAME= all count.
	bare quoting = iota
	// double is within double quotes: $ and ` count, ~ and NAME= don't.
	double
	// literal is single quoted, or escaped by a backslash: nothing counts.
	literal
)

// word is a word of a command as the shell splits it before it expands it,
// its quotes and backslashes removed, and how each of its bytes was quoted.
type word struct {
	text string
	how  []quoting
}

// assignment is a word that sets a variable, or adds to it.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?=`)

// assigns reports whether w sets a variable for the command after it, its
// NAME= unquoted.
func (w word) assigns() bool {
	m := assignment.FindStringIndex(w.text)
	return m != nil && w.bare(0, m[1])
}

// equals reports whether w is zsh's =name, the path PATH finds name at.
func (w word) equals(name string) bool {
	return w.text == "="+name && w.how[0] == bare
}

// expands reports whether w has a $ or ` the shell expands, from its byte
// from on.
func (w word) expands(from int) bool {
	for i := from; i < len(w.text); i++ {
		if strings.IndexByte("$`", w.text[i]) >= 0 && w.how[i] != literal {
			return true
		}
	}
	return false
}

// home returns what's left of w past the home it starts from, as an unquoted
// ~, or a $HOME or ${HOME} the shell expands, starts it; whether it starts
// from one; and whether that can be read, as ~ and a name, another user's
// home, can't.
func (w word) home() (string, bool, bool) {
	if strings.HasPrefix(w.text, "~") && w.how[0] == bare {
		end := len(w.text)
		for i := range len(w.text) {
			if w.text[i] == '/' && w.how[i] == bare {
				end = i
				break
			}
		}
		switch {
		case !w.bare(0, end):
			return w.text, false, true
		case end > 1:
			return "", false, false
		}
		return w.text[1:], true, true
	}
	for _, home := range []string{"$HOME", "${HOME}"} {
		rest, ok := strings.CutPrefix(w.text, home)
		if ok && !w.has(literal, 0, len(home)) && (rest == "" || rest[0] == '/') {
			return rest, true, true
		}
	}
	return w.text, false, true
}

// bare reports whether w's bytes from from up to to are all unquoted.
func (w word) bare(from, to int) bool {
	return !slices.ContainsFunc(w.how[from:to], func(q quoting) bool { return q != bare })
}

// has reports whether one of w's bytes from from up to to was quoted as q.
func (w word) has(q quoting, from, to int) bool {
	return slices.Contains(w.how[from:to], q)
}

// namesClaude reports whether w is claude, or a path to one.
func namesClaude(w word) bool {
	return w.text == "claude" || strings.HasSuffix(w.text, "/claude")
}

// unquote returns s, one word as the shell quotes it, its quotes removed.
func unquote(s string) (string, bool) {
	cmds, ok := commands(s)
	switch {
	case !ok || len(cmds) > 1 || len(cmds[0]) > 1:
		return "", false
	case len(cmds[0]) == 0:
		return "", true
	}
	return cmds[0][0].text, true
}

// commands splits s into its commands, at an unquoted ;, &, |, <, >, ) or
// line's end, or a ( that starts a word, opening a subshell; and each command
// into its words. $(…), ${…}, `…` and a ( within a word, as zsh's path=(…),
// stay whole within the word. It fails at a quote, or one of those, that
// isn't closed.
func commands(s string) ([][]word, bool) {
	p := splitter{cmds: [][]word{nil}}
	for i := 0; i < len(s); {
		n, ok := p.read(s[i:])
		if !ok {
			return nil, false
		}
		i += n
	}
	p.end()
	return p.cmds, true
}

// splitter splits a command line into commands and words, as commands says.
type splitter struct {
	cmds [][]word
	w    word
	in   bool
}

// read reads what s starts with, and returns how much of s that is, and
// whether it's closed.
func (p *splitter) read(s string) (int, bool) {
	c := s[0]
	switch {
	case c == ' ' || c == '\t':
		p.end()
	case strings.IndexByte(";&|<>)\n", c) >= 0 || c == '(' && !p.in:
		p.end()
		p.cmds = append(p.cmds, nil)
	case strings.HasPrefix(s, "\\\n"):
		return 2, true
	case c == '\\' && len(s) > 1:
		p.add(s[1:2], literal)
		return 2, true
	case c == '\'':
		n, ok := singleQuote(s)
		if ok {
			p.add(s[1:n-1], literal)
		}
		return n, ok
	case c == '"':
		p.add("", double)
		return doubleQuote(s, p.add)
	case c == '(' || opens(s):
		n, ok := group(s)
		if ok {
			p.add(s[:n], bare)
		}
		return n, ok
	default:
		p.add(s[:1], bare)
	}
	return 1, true
}

// add adds s, quoted as q, to the word being read.
func (p *splitter) add(s string, q quoting) {
	p.w.text += s
	for range len(s) {
		p.w.how = append(p.w.how, q)
	}
	p.in = true
}

// end ends the word being read, if there's one.
func (p *splitter) end() {
	if p.in {
		last := len(p.cmds) - 1
		p.cmds[last] = append(p.cmds[last], p.w)
	}
	p.w, p.in = word{}, false
}

// opens reports whether s opens a group the shell keeps within a word: $(…),
// as $((…)) does too, ${…} or `…`.
func opens(s string) bool {
	return s[0] == '`' || strings.HasPrefix(s, "$(") || strings.HasPrefix(s, "${")
}

// group returns how long the group s starts with is, as opens says or a (,
// and whether it's closed.
func group(s string) (int, bool) {
	if s[0] == '$' {
		n, ok := closes(s[1:])
		return n + 1, ok
	}
	return closes(s)
}

// closes returns how long the group s opens is, up to the ), } or ` that
// closes its (, { or `, quotes and groups within read as the shell reads
// them; and whether one does.
func closes(s string) (int, bool) {
	open := s[0]
	shut := map[byte]byte{'(': ')', '{': '}', '`': '`'}[open]
	depth := 1
	for i := 1; i < len(s); {
		n, ok := 1, true
		switch c := s[i]; {
		case c == '\\':
			n = 2
		case c == shut:
			if depth--; depth == 0 {
				return i + 1, true
			}
		case open == '`':
		case c == open:
			depth++
		case c == '\'':
			n, ok = singleQuote(s[i:])
		case c == '"':
			n, ok = doubleQuote(s[i:], func(string, quoting) {})
		case opens(s[i:]):
			n, ok = group(s[i:])
		}
		if !ok {
			return 0, false
		}
		i += n
	}
	return 0, false
}

// singleQuote returns how long the single quoted string s starts with is,
// and whether it's closed.
func singleQuote(s string) (int, bool) {
	n := strings.IndexByte(s[1:], '\'')
	return n + 2, n >= 0
}

// doubleQuote returns how long the double quoted string s starts with is,
// and whether it's closed, giving add what it holds as the shell reads it: a
// backslash escapes only $, `, ", \ and a line's end, and groups within stay
// whole.
func doubleQuote(s string, add func(string, quoting)) (int, bool) {
	for i := 1; i < len(s); {
		n, ok := 1, true
		switch {
		case s[i] == '"':
			return i + 1, true
		case strings.HasPrefix(s[i:], "\\\n"):
			n = 2
		case s[i] == '\\' && i+1 < len(s) && strings.IndexByte("$`\"\\", s[i+1]) >= 0:
			add(s[i+1:i+2], literal)
			n = 2
		case opens(s[i:]):
			if n, ok = group(s[i:]); ok {
				add(s[i:i+n], double)
			}
		default:
			add(s[i:i+1], double)
		}
		if !ok {
			return 0, false
		}
		i += n
	}
	return 0, false
}
