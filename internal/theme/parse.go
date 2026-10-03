package theme

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"unicode"
)

// Why a theme doesn't load, each in a word or two, as the theme picker shows
// it beside the theme.
const (
	badName       = "bad name"
	reservedName  = "reserved name"
	unreadable    = "unreadable"
	badSyntax     = "bad syntax"
	badColour     = "bad colour"
	missingTokens = "missing tokens"
	notFound      = "not found"
)

// Problem is why a theme doesn't load: its Reason, a word or two, which the
// theme picker shows beside the theme, and, as the log has it, its Detail.
type Problem struct {
	Reason string
	// Detail says more, where there's more to say: which line, which tokens,
	// what the system said.
	Detail string
	// Err is the system's error, for a file that couldn't be read.
	Err error
}

// Error is the reason, and the detail after it where there's one.
func (p *Problem) Error() string {
	if p.Detail == "" {
		return p.Reason
	}
	return p.Reason + ": " + p.Detail
}

// Unwrap is the system's error, for a file that couldn't be read.
func (p *Problem) Unwrap() error {
	return p.Err
}

// byName finds a token by its key in a .theme file.
var byName = func() map[string]Token {
	m := make(map[string]Token, tokens)
	for tok := Token(1); tok < tokens; tok++ {
		m[tok.String()] = tok
	}
	return m
}()

// Parse reads a theme, under the slug given, from a .theme file's text:
// flat `key = #RRGGBB` lines, a # starting a comment only at the start of a
// line, a value unquoted, and a key once. A key it doesn't know, such as a
// newer switchboard's, is passed over. A file without one of the 19 base
// tokens doesn't load; one without one of the charts' has it worked out from
// them.
func Parse(slug string, data []byte) (Theme, error) {
	t, p := parse(slug, data)
	if p != nil {
		return Theme{}, p
	}
	return t, nil
}

// parse is Parse, its problem as a Problem.
func parse(slug string, data []byte) (Theme, *Problem) {
	pairs, p := lex(data)
	if p != nil {
		return Theme{}, p
	}
	t := Theme{Slug: slug}
	var bad []string
	for _, p := range pairs {
		tok, known := byName[p.key]
		if !known {
			continue
		}
		c, ok := hexColour(p.value)
		if !ok {
			bad = append(bad, p.key+" = "+p.value)
			continue
		}
		t.colours[tok] = c
	}
	if len(bad) > 0 {
		return Theme{}, &Problem{Reason: badColour, Detail: strings.Join(bad, ", ")}
	}
	var missing []string
	for tok := firstBase; tok <= lastBase; tok++ {
		if t.colours[tok] == nil {
			missing = append(missing, tok.String())
		}
	}
	if len(missing) > 0 {
		return Theme{}, &Problem{Reason: missingTokens, Detail: "missing " + strings.Join(missing, ", ")}
	}
	t.derive()
	return t, nil
}

// byteOrderMark may open a file an editor saved, before its first line.
const byteOrderMark = string(rune(0xFEFF))

// pair is a line of a .theme file: its key and its value, as written.
type pair struct {
	key, value string
}

// lex reads a .theme file's lines as key = value pairs, passing over blank
// lines and comments, and failing at the first line that isn't a pair, has
// a quoted value, or gives a key again.
func lex(data []byte) ([]pair, *Problem) {
	seen := make(map[string]bool)
	var pairs []pair
	for i, raw := range strings.Split(strings.TrimPrefix(string(data), byteOrderMark), "\n") {
		text := strings.TrimSpace(raw)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, isPair := strings.Cut(text, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch {
		case !isPair || key == "" || strings.ContainsFunc(key, unicode.IsSpace):
			return nil, syntaxProblem(i, "not a key = value pair")
		case strings.HasPrefix(value, `"`) || strings.HasPrefix(value, "'"):
			return nil, syntaxProblem(i, "quoted value")
		case seen[key]:
			return nil, syntaxProblem(i, "duplicate key "+key)
		}
		seen[key] = true
		pairs = append(pairs, pair{key: key, value: value})
	}
	return pairs, nil
}

// syntaxProblem is the problem with a file's line, counting from 0, as it's
// told counting from 1.
func syntaxProblem(line int, what string) *Problem {
	return &Problem{Reason: badSyntax, Detail: fmt.Sprintf("line %d: %s", line+1, what)}
}

// hexColour reads a colour written #RRGGBB, in either case: neither the
// short #RGB nor an alpha.
func hexColour(value string) (color.Color, bool) {
	if len(value) != len("#RRGGBB") || value[0] != '#' {
		return nil, false
	}
	n, err := strconv.ParseUint(value[1:], 16, 32)
	if err != nil {
		return nil, false
	}
	return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff}, true
}

// ValidSlug reports whether s can name a theme: a lower-case letter or a
// digit, then those or hyphens. Nothing else is ever made a path of.
func ValidSlug(s string) bool {
	for i, r := range s {
		lower, digit := 'a' <= r && r <= 'z', '0' <= r && r <= '9'
		if !lower && !digit && (i == 0 || r != '-') {
			return false
		}
	}
	return s != ""
}
