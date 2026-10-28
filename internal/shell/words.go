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
	// bare is unquoted: ~, $, `, a glob and a NAME= all count.
	bare quoting = iota
	// double is within double quotes: $ and ` count, the rest don't.
	double
	// literal is single quoted, escaped by a backslash, or within $'…':
	// nothing counts.
	literal
)

// word is a word of a command as the shell splits it before it expands it.
type word struct {
	// text is the word, its quotes and backslashes removed, $'…' decoded,
	// and $(…), ${…} and `…` kept as written.
	text string
	// how is how each byte of text was quoted.
	how []quoting
	// span numbers each byte of text by the stretch, quoted or not, it was
	// written in; start is the word's first, even where an empty quote
	// gave it no byte.
	span  []int
	start int
	// raw is the word as written.
	raw string
	// odd is set where $'…' holds an escape this doesn't decode, so text
	// isn't what the shell reads.
	odd bool
}

// assignment is a word that sets a variable, or adds to it.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?=`)

// assigns reports whether w sets a variable for the command after it, its
// NAME= unquoted.
func (w word) assigns() bool {
	m := assignment.FindStringIndex(w.text)
	return m != nil && w.bare(0, m[1])
}

// setsPATH reports whether w, an argument, sets PATH as env and export take
// it: PATH= with its name unquoted.
func (w word) setsPATH() bool {
	return strings.HasPrefix(w.text, "PATH=") && w.bare(0, len("PATH="))
}

// equals reports whether w is zsh's =name, unquoted, the path PATH finds
// name at.
func (w word) equals(name string) bool {
	return w.text == "="+name && w.bare(0, len(w.text))
}

// plain reports whether, from its byte from on, w is what the shell runs as
// it's written: nothing in it expanded, nor a glob or a brace, and nothing
// left undecoded.
func (w word) plain(from int) bool {
	for i := from; i < len(w.text); i++ {
		if strings.IndexByte("$`", w.text[i]) >= 0 && w.how[i] != literal ||
			strings.IndexByte("*?[{(", w.text[i]) >= 0 && w.how[i] == bare {
			return false
		}
	}
	return !w.odd
}

// home returns what's left of w past the home it starts from, and whether
// it starts from one: a ~/ unquoted, or $HOME or ${HOME} followed by /,
// unquoted or within the one double quoted stretch. And it returns whether
// w can be read, as a word starting with an unquoted ~ in any other way,
// another user's home or one quoted after the ~, which the shells read
// apart, can't.
func (w word) home() (string, bool, bool) {
	if strings.HasPrefix(w.text, "~") && w.how[0] == bare {
		if strings.HasPrefix(w.text, "~/") && w.leads(2) && w.how[1] == bare {
			return w.text[1:], true, true
		}
		return "", false, false
	}
	for _, home := range []string{"$HOME", "${HOME}"} {
		rest, ok := strings.CutPrefix(w.text, home)
		if ok && strings.HasPrefix(rest, "/") && w.leads(len(home)) && w.how[0] != literal {
			return rest, true, true
		}
	}
	return w.text, false, true
}

// leads reports whether w's bytes up to to were all written in its first
// stretch.
func (w word) leads(to int) bool {
	return !slices.ContainsFunc(w.span[:to], func(s int) bool { return s != w.start })
}

// bare reports whether w's bytes from from up to to are all unquoted.
func (w word) bare(from, to int) bool {
	return !slices.ContainsFunc(w.how[from:to], func(q quoting) bool { return q != bare })
}

// claudeToken is claude as a word of its own within some text: not
// claude-code, say, nor .claude.
var claudeToken = regexp.MustCompile(`(^|[^A-Za-z0-9_.])claude($|[^A-Za-z0-9_.-])`)

// unquoted is a word as written with its quotes taken out, so a claude
// quoted in parts within $(…) still shows.
var unquoted = strings.NewReplacer(`'`, "", `"`, "", `\`, "")

// namesClaude reports whether claude is a word within w, as written or as
// the shell reads it, through its quotes and within its groups.
func namesClaude(w word) bool {
	return claudeToken.MatchString(w.text) || claudeToken.MatchString(unquoted.Replace(w.raw))
}

// mayRunClaude reports whether w may hold claude: it names it, or it holds
// what this doesn't decode.
func mayRunClaude(w word) bool {
	return w.odd || namesClaude(w)
}

// unquote returns s, one word as the shell quotes it, its quotes removed,
// and whether it's read whole.
func unquote(s string) (string, bool) {
	cmds, ok := commands(s)
	switch {
	case !ok || len(cmds) > 1 || len(cmds[0]) > 1:
		return "", false
	case len(cmds[0]) == 0:
		return "", true
	}
	return cmds[0][0].text, !cmds[0][0].odd
}

// commands splits s into its commands, at an unquoted ;, &, |, <, >, ) or
// line's end, or a ( that starts a word, opening a subshell; and each command
// into its words. $(…), ${…}, `…` and a ( within a word, as zsh's path=(…),
// stay whole within the word; a # that starts a word is a comment, to the
// line's end. It fails at a quote, or one of those, that isn't closed.
func commands(s string) ([][]word, bool) {
	p := splitter{cmds: [][]word{nil}}
	for i := 0; i < len(s); {
		n, ok := p.read(s[i:])
		if !ok {
			return nil, false
		}
		if p.in {
			p.w.raw += s[i : i+n]
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
	span int
}

// read reads what s starts with, and returns how much of s that is, and
// whether it's closed.
func (p *splitter) read(s string) (int, bool) {
	c := s[0]
	switch {
	case c == ' ' || c == '\t':
		p.end()
	case c == '#' && !p.in:
		if n := strings.IndexByte(s, '\n'); n >= 0 {
			return n, true
		}
		return len(s), true
	case strings.IndexByte(";&|<>)\n", c) >= 0 || c == '(' && !p.in:
		p.end()
		p.cmds = append(p.cmds, nil)
	case strings.HasPrefix(s, "\\\n"):
		return 2, true
	case c == '\\' && len(s) > 1:
		p.quoted(s[1:2], literal)
		return 2, true
	case c == '\'':
		n, ok := singleQuote(s)
		if ok {
			p.quoted(s[1:n-1], literal)
		}
		return n, ok
	case strings.HasPrefix(s, "$'"):
		text, odd, n, ok := ansiC(s)
		if ok {
			p.quoted(text, literal)
			p.w.odd = p.w.odd || odd
		}
		return n, ok
	case c == '"':
		p.span++
		p.add("", double)
		n, ok := doubleQuote(s, p.add)
		p.span++
		return n, ok
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

// quoted adds s, quoted as q, to the word being read, as a stretch of its
// own.
func (p *splitter) quoted(s string, q quoting) {
	p.span++
	p.add(s, q)
	p.span++
}

// add adds s, quoted as q, to the word being read.
func (p *splitter) add(s string, q quoting) {
	if !p.in {
		p.w.start = p.span
	}
	p.w.text += s
	for range len(s) {
		p.w.how = append(p.w.how, q)
		p.w.span = append(p.w.span, p.span)
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
		case strings.HasPrefix(s[i:], "$'"):
			_, _, n, ok = ansiC(s[i:])
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

// ansiEscapes are the escapes of $'…' read here, each the text it stands
// for.
var ansiEscapes = map[byte]string{
	'a': "\a", 'b': "\b", 'e': "\x1b", 'E': "\x1b", 'f': "\f", 'n': "\n", 'r': "\r", 't': "\t", 'v': "\v",
	'\\': `\`, '\'': `'`, '"': `"`, '?': "?",
}

// ansiC returns what the $'…' s starts with holds, its escapes decoded, and
// whether it holds one not decoded here, as \x41; and how long it is, and
// whether it's closed.
func ansiC(s string) (string, bool, int, bool) {
	var text strings.Builder
	odd := false
	for i := 2; i < len(s); i++ {
		switch {
		case s[i] == '\'':
			return text.String(), odd, i + 1, true
		case s[i] == '\\' && i+1 < len(s):
			i++
			e, ok := ansiEscapes[s[i]]
			if !ok {
				e, odd = s[i-1:i+1], true
			}
			text.WriteString(e)
		default:
			text.WriteByte(s[i])
		}
	}
	return "", false, 0, false
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
