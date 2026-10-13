package views

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// RequestDetails are a request's details, as the Log's card holds them,
// typed, each printed form left to its one owner. They start from the
// request's row, so the card and the row can't disagree: the card's id, its
// agent, to, upstream, model, size, messages, tools, thinking, status, first
// byte, ended, stopped, in, from cache, cached, out and worth are its Row's,
// and the move its Row's Routing's Cost, where its routing names the account
// it left. The rest are its line's, each blank until it comes, as its Row's
// Stage says: a request in flight's card holds what the stream told. Their
// text, but Left's and the windows' names, is as the line gives it, for the
// drawer to clean, as it cleans the Row's Dir.
type RequestDetails struct {
	Row RequestRow
	// Answer is Anthropic's id for its answer, and Served the model that
	// served it, as its answer names them.
	Answer, Served string
	// Betas are the features it asked the API for beyond its version, each
	// without its date: context-1m of context-1m-2025-08-07.
	Betas []string
	// Left is, of a request that moved its session, the account it left and
	// why, as RoutingWords says them but for their leading "from ": "work, at
	// its cap". None where it moved none.
	Left status.Words
	// Windows are the windows of the account it went out on as its answer
	// left them, in quota's order, as an event's card has an account's, and
	// LimitStatus is its answer's status across them, both as its usage
	// headers gave them: none where they gave none.
	Windows     []WindowThen
	LimitStatus quota.Status
	// System counts its system prompt's blocks, MaxTokens is its max_tokens
	// and Effort its output_config's effort, each zero where its shape gives
	// none; Stream is whether it asked to stream, nil where it didn't say.
	System, MaxTokens int
	Effort            string
	Stream            *bool
	// Blocks are its answer's blocks, counted by their kinds, in the order an
	// answer streams them: thinking, text, then calls of tools; then any
	// other kind, by its name.
	Blocks []DetailBlock
	// CacheLife is how long its writes to the cache last, as its usage says:
	// zero where it wrote none.
	CacheLife time.Duration
}

// DetailBlock is a kind of block an answer held, as tool_use, and how many.
type DetailBlock struct {
	Kind  string
	Count int
}

// Details are the details of the request the line's row is of, built from
// its row and, once it has come, its line: false for a day's rule or the
// oldest day's line.
func (l LogLine) Details() (RequestDetails, bool) {
	if !l.IsRow() {
		return RequestDetails{}, false
	}
	d := RequestDetails{Row: l.Row}
	if l.line != nil {
		d.lined(l.line)
	}
	return d, true
}

// lined has d hold what the request's line l gives beyond its row.
func (d *RequestDetails) lined(l *ledger.Line) {
	d.Answer, d.Served = l.Answer.ID, l.Answer.Model
	for _, beta := range l.Betas {
		d.Betas = append(d.Betas, undated(beta))
	}
	if l.From != "" {
		d.Left = leftWords(status.RoutingWords(l.Reason, status.Facts{From: l.From, To: l.Account}), l.From)
	}
	d.Windows, d.LimitStatus = windowsThen(claude.WindowsRead(l.Limits, 0), nil), claude.LimitStatus(l.Limits)
	d.System, d.Effort, d.Stream = l.Shape.System, l.Shape.OutputConfig.Effort, l.Shape.Stream
	if most := l.Shape.MaxTokens; most != nil {
		d.MaxTokens = int(*most)
	}
	d.Blocks = blocksOf(l.Answer.Blocks)
	d.CacheLife, _ = l.CacheLife()
}

// undated is a beta's name without the date it ends in, where it ends in
// one.
func undated(beta string) string {
	at := len(beta) - len(time.DateOnly) - 1
	if at < 1 || beta[at] != '-' {
		return beta
	}
	if _, err := time.Parse(time.DateOnly, beta[at+1:]); err != nil {
		return beta
	}
	return beta[:at]
}

// leftWords are a move's routing words without their leading "from ", as
// FromWords starts them, keeping the account it left: whole where they don't
// start so, as a session's first's don't.
func leftWords(routed status.Words, from string) status.Words {
	lead := status.FromWords(from)
	if !slices.Equal(routed[:min(len(lead), len(routed))], lead) {
		return routed
	}
	return routed[len(lead)-1:]
}

// answerOrder is the order an answer streams its blocks' kinds in: a kind
// not named comes after them.
var answerOrder = []string{"thinking", "redacted_thinking", "text", "tool_use", "server_tool_use", "mcp_tool_use"}

// blocksOf are an answer's blocks, counted by their kinds, in answerOrder,
// the others by their names after them.
func blocksOf(counted map[string]int) []DetailBlock {
	kinds := slices.SortedFunc(maps.Keys(counted), func(a, b string) int {
		return cmp.Or(cmp.Compare(answerPlace(a), answerPlace(b)), strings.Compare(a, b))
	})
	var blocks []DetailBlock
	for _, kind := range kinds {
		blocks = append(blocks, DetailBlock{Kind: kind, Count: counted[kind]})
	}
	return blocks
}

// answerPlace is the place of a kind of block in answerOrder: after them all
// where it isn't there.
func answerPlace(kind string) int {
	if i := slices.Index(answerOrder, kind); i >= 0 {
		return i
	}
	return len(answerOrder)
}
