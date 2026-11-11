package idmove

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
	"slices"

	"github.com/leeovery/switchboard/internal/status"
)

// names say where a kind of JSON names accounts, each by a pattern: values,
// the strings that are an account's id; texts, the reasons that name them, as
// status.RenamedReason reads them; and keys, the objects whose names are
// accounts' ids.
type names struct {
	values, texts, keys []pattern
}

// pattern is a JSON pointer's tokens, each a name or an index, "*" matching
// any.
type pattern []string

// patterns are the patterns of the JSON pointers given.
func patterns(pointers ...string) []pattern {
	made := make([]pattern, len(pointers))
	for i, p := range pointers {
		made[i] = pointerTokens(jsontext.Pointer(p))
	}
	return made
}

// Where each kind of file names accounts.
var (
	// readingNames are a line of the readings history's.
	readingNames = names{values: patterns("/account")}
	// lineNames are a line of the request ledger's.
	lineNames = names{values: patterns("/account", "/from", "/tried/*/account"), texts: patterns("/reason")}
	// eventNames are a line of the router's events'.
	eventNames = names{values: patterns("/account", "/accounts/*", "/from", "/to"), texts: patterns("/reason")}
	// summaryNames are a summary of a day of the request ledger's.
	summaryNames = names{values: patterns("/accounts/*/account")}
	// stateNames are the router's state file's: its global pin, as each
	// form of it gives its accounts, its sessions' assignments and their own
	// pins, what it keeps by account of the tokens, readings and tokens'
	// years, and the account a former token was taken up from.
	stateNames = names{
		values: patterns("/pin/accounts/*", "/pin/account", "/sessions/*/account", "/sessions/*/pin", "/session_pins/*/account", "/tokens/*/former/*/from"),
		texts:  patterns("/sessions/*/reason"),
		keys:   patterns("/tokens/*", "/readings/*", "/token_years/*"),
	}
)

// renamed returns data, a JSON value, with each account n says it names
// renamed as rename gives its id, every other byte as it was, and reports
// whether any was renamed. Data that doesn't read as one JSON value is
// returned as it is, as a reader passes it over.
func (n names) renamed(data []byte, rename func(id string) string) ([]byte, bool) {
	dec := jsontext.NewDecoder(bytes.NewReader(data), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	var out []byte
	copied := int64(0)
	for {
		before := dec.InputOffset()
		tok, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return data, false
		}
		if tok.Kind() != '"' {
			continue
		}
		was := tok.String()
		now := n.rename(dec, was, rename)
		if now == was {
			continue
		}
		end := dec.InputOffset()
		start := before + int64(bytes.IndexByte(data[before:end], '"'))
		// A string always marshals.
		quoted, _ := json.Marshal(now)
		out = append(append(out, data[copied:start]...), quoted...)
		copied = end
	}
	if out == nil {
		return data, false
	}
	return append(out, data[copied:]...), true
}

// visit hands seen each account n says data, a JSON value, names, by its
// id: none where data doesn't read as one JSON value.
func (n names) visit(data []byte, seen func(id string)) {
	n.renamed(data, func(id string) string {
		seen(id)
		return id
	})
}

// rename returns the string just read from dec, was, renamed as rename
// gives an account's id, where n says it names one there: itself where it
// doesn't.
func (n names) rename(dec *jsontext.Decoder, was string, rename func(string) string) string {
	at := pointerTokens(dec.StackPointer())
	kind, length := dec.StackIndex(dec.StackDepth())
	if kind == '{' && length%2 == 1 {
		if matchesAny(n.keys, at) {
			return rename(was)
		}
		return was
	}
	switch {
	case matchesAny(n.values, at):
		return rename(was)
	case matchesAny(n.texts, at):
		return status.RenamedReason(was, rename)
	}
	return was
}

// pointerTokens are the tokens of p, each a name or an index.
func pointerTokens(p jsontext.Pointer) []string {
	return slices.Collect(p.Tokens())
}

// matchesAny reports whether at, the tokens of a pointer, match any of
// patterns, token for token.
func matchesAny(patterns []pattern, at []string) bool {
	return slices.ContainsFunc(patterns, func(p pattern) bool {
		return slices.EqualFunc(p, at, func(want, got string) bool { return want == "*" || want == got })
	})
}
