package views

import (
	"iter"

	"github.com/leeovery/switchboard/internal/ledger"
)

// RequestLine is a request's row as requests prints it, a line of the
// ledger each: its row, the Log's, as its line alone tells it, its session's
// story untold, so whether it's its session's first, and what a move it made
// cost, aren't known; and the tokens it took in. Its Worth is never priced,
// and left zero, as requests prints none.
type RequestLine struct {
	RequestRow
	// In counts the tokens it took in, as its answer's usage counts them:
	// its input, and those it read from the cache and wrote to it, where
	// Metered says its answer gave usage.
	In int
}

// RequestLines are the rows of the requests lines hold, in their order.
func RequestLines(lines iter.Seq[ledger.Held]) iter.Seq[RequestLine] {
	return func(yield func(RequestLine) bool) {
		for h := range lines {
			rec := lineRecordOf(&h.Line)
			r := RequestLine{RequestRow: lineRow(&rec, nil)}
			if tokens, ok := rec.spend.Tokens(); ok {
				r.In = tokens.Input + tokens.CacheRead + tokens.CacheWrite
			}
			if !yield(r) {
				return
			}
		}
	}
}

// OnAccount are those of lines of the account with the given id, every one
// for "".
func OnAccount(lines iter.Seq[ledger.Held], account string) iter.Seq[ledger.Held] {
	if account == "" {
		return lines
	}
	return func(yield func(ledger.Held) bool) {
		for h := range lines {
			if h.Account == account && !yield(h) {
				return
			}
		}
	}
}
