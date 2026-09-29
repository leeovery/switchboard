package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// kind is what a line of the config file is, as TOML reads it, as far as
// editing the accounts needs to tell.
type kind int

const (
	// blank is a line of whitespace alone.
	blank kind = iota
	// comment is a line holding a comment alone.
	comment
	// accountHeader opens an [[account]] table.
	accountHeader
	// header opens any other table.
	header
	// primaryKey sets primary.
	primaryKey
	// content is any other line: a key and its value, or part of a string
	// that goes on over lines.
	content
)

var (
	accountHeaderLine = regexp.MustCompile(`^\[\[\s*(?:account|"account"|'account')\s*\]\]\s*(?:#.*)?$`)
	primaryKeyLine    = regexp.MustCompile(`^(?:primary|"primary"|'primary')\s*=`)
	// multilineOpening matches a key whose value opens a string that can go
	// on over lines, capturing the quotes that open it and what follows them.
	multilineOpening = regexp.MustCompile(`^[^=]*=\s*("""|''')(.*)$`)
)

// document is the config file's text, a line at a time, and what TOML makes
// of each line.
type document struct {
	lines []string
	kinds []kind
}

// newDocument reads text as a document.
func newDocument(text string) document {
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return documentOf(lines)
}

func documentOf(lines []string) document {
	return document{lines: lines, kinds: scan(lines)}
}

// text is the document's text, each line ending in a newline.
func (d document) text() string {
	if len(d.lines) == 0 {
		return ""
	}
	return strings.Join(d.lines, "\n") + "\n"
}

// scan says what each of lines is. A line within a string that goes on over
// lines is content, whatever it looks like.
func scan(lines []string) []kind {
	kinds := make([]kind, len(lines))
	// open is the quotes that opened a string that goes on past the lines
	// scanned, when there's one.
	var open string
	for i, line := range lines {
		text := strings.TrimSpace(line)
		if open != "" {
			kinds[i] = content
			if closes(open, text) {
				open = ""
			}
			continue
		}
		kinds[i] = classify(text)
		if kinds[i] == content || kinds[i] == primaryKey {
			open = opens(text)
		}
	}
	return kinds
}

// classify says what a line is, its whitespace trimmed, when it isn't within
// a string that goes on over lines.
func classify(text string) kind {
	switch {
	case text == "":
		return blank
	case strings.HasPrefix(text, "#"):
		return comment
	case accountHeaderLine.MatchString(text):
		return accountHeader
	case strings.HasPrefix(text, "["):
		return header
	case primaryKeyLine.MatchString(text):
		return primaryKey
	}
	return content
}

// opens returns the quotes that open a string a key's value starts on the
// line and that goes on past it, or "" when it doesn't.
func opens(text string) string {
	m := multilineOpening.FindStringSubmatch(text)
	if m == nil || closes(m[1], m[2]) {
		return ""
	}
	return m[1]
}

// closes reports whether text holds the quotes that close a string opened
// with quote: the same three again, which a backslash escapes in a basic
// string but not in a literal one.
func closes(quote, text string) bool {
	if quote == `'''` {
		return strings.Contains(text, quote)
	}
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '\\':
			i++
		case strings.HasPrefix(text[i:], quote):
			return true
		}
	}
	return false
}

// span is the lines of a document from start up to end.
type span struct {
	start, end int
}

// accounts returns the span of each [[account]] table, in the order they
// come: from the comments directly above its header to its last key, and
// the comments directly below that, but for those directly above the next
// table's header, which are that table's.
func (d document) accounts() []span {
	var tables []span
	for i, k := range d.kinds {
		if k == accountHeader {
			tables = append(tables, d.table(i))
		}
	}
	return tables
}

// table returns the span of the table whose header is line h.
func (d document) table(h int) span {
	next := d.nextTable(h)
	end := h + 1
	for i := h + 1; i < next; i++ {
		if d.kinds[i] != blank && d.kinds[i] != comment {
			end = i + 1
		}
	}
	for end < next && d.kinds[end] == comment {
		end++
	}
	return span{start: d.commentsAbove(h), end: end}
}

// nextTable returns where the table after line h starts, with the comments
// directly above its header, or the end of the document.
func (d document) nextTable(h int) int {
	for i := h + 1; i < len(d.kinds); i++ {
		if d.kinds[i] == header || d.kinds[i] == accountHeader {
			return d.commentsAbove(i)
		}
	}
	return len(d.kinds)
}

// commentsAbove returns where the comments directly above line i start: i
// itself when there are none.
func (d document) commentsAbove(i int) int {
	for i > 0 && d.kinds[i-1] == comment {
		i--
	}
	return i
}

// linesOf returns the lines of the kind given within s.
func (d document) linesOf(k kind, s span) []int {
	var found []int
	for i := s.start; i < s.end; i++ {
		if d.kinds[i] == k {
			found = append(found, i)
		}
	}
	return found
}

// newAccountAt returns where a new [[account]] table goes: after the last
// account's, else before the first table, with the comments directly above
// its header, else at the end.
func (d document) newAccountAt() int {
	if tables := d.accounts(); len(tables) > 0 {
		return tables[len(tables)-1].end
	}
	if i := slices.Index(d.kinds, header); i >= 0 {
		return d.commentsAbove(i)
	}
	return len(d.kinds)
}

// insert returns the document with lines put in at line at, a blank line
// setting them apart from the lines before and after them, unless one does
// already.
func (d document) insert(at int, lines []string) document {
	var inserted []string
	if at > 0 && d.kinds[at-1] != blank {
		inserted = append(inserted, "")
	}
	inserted = append(inserted, lines...)
	if at < len(d.kinds) && d.kinds[at] != blank {
		inserted = append(inserted, "")
	}
	return documentOf(slices.Concat(d.lines[:at], inserted, d.lines[at:]))
}

// without returns the document without the lines given.
func (d document) without(drop []int) document {
	var kept []string
	for i, line := range d.lines {
		if !slices.Contains(drop, i) {
			kept = append(kept, line)
		}
	}
	return documentOf(kept)
}

// cut returns the document without the lines in s, which are whole tables,
// and without the blank lines that set them apart and no longer set anything
// apart: those left leading or trailing the document, and those after where
// s was when there are some before it too.
func (d document) cut(s span) document {
	left := documentOf(slices.Concat(d.lines[:s.start], d.lines[s.end:]))
	before, after := s.start, s.start
	for before > 0 && left.kinds[before-1] == blank {
		before--
	}
	for after < len(left.kinds) && left.kinds[after] == blank {
		after++
	}
	switch {
	case after == len(left.lines):
		return documentOf(left.lines[:before])
	case before == 0:
		return documentOf(left.lines[after:])
	case before < s.start:
		return documentOf(slices.Delete(left.lines, s.start, after))
	}
	return left
}

// lines returns the lines of the account's [[account]] table, its keys lined
// up as Example lines them up.
func (a NewAccount) lines() []string {
	keys := [][2]string{{"id", quote(a.ID)}}
	if a.Label != "" {
		keys = append(keys, [2]string{"label", quote(a.Label)})
	}
	if a.Primary {
		keys = append(keys, [2]string{"primary", "true"})
	}
	width := 0
	for _, kv := range keys {
		width = max(width, len(kv[0]))
	}
	lines := []string{"[[account]]"}
	for _, kv := range keys {
		lines = append(lines, fmt.Sprintf("%-*s = %s", width, kv[0], kv[1]))
	}
	return lines
}

// quote returns s as a TOML basic string, escaping what TOML asks to be:
// quotes, backslashes and control characters.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
