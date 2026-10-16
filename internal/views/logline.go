package views

import (
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// loggedLine is a line of the Log's, as its read holds it in place of the
// line, decoded once as it's taken in: its row, told once of its session's
// story and told again only as the store says the story changed, its words
// sharing the strings the store's requests name; when it was taken in among
// the read's lines, and where its row sits within its day, as placedRow has
// it; its request's flight, and the sum of what its line says, as sumOf has
// it; how the router routed it, as its routing is told again; and what its
// details give beyond its row. The table's lines point at it, so a row is
// held once.
type loggedLine struct {
	row    RequestRow
	seq    int
	place  time.Time
	flight flight
	sum    uint64
	routed routed
	shown  *lineDetails
}

// routed is how the router routed a request, as its line tells it, beside
// the account it went to, which its row names: the accounts it went out on
// that couldn't serve it, the account it moved its session from, and why.
type routed struct {
	tried        []ledger.Tried
	from, reason string
}

// routedOf is how the router routed the request of the line l.
func routedOf(l *ledger.Line) routed {
	return routed{tried: l.Tried, from: interned(l.From), reason: interned(l.Reason)}
}

// lineDetails are what a request's details give beyond its row, as its line
// gives them, as RequestDetails has them, each decoded once: its answer's id
// and the model that served it; its betas, each undated; its shape's system
// blocks, max_tokens, effort and whether it streamed; its answer's blocks,
// counted by kind, in answerOrder; how long its writes to the cache last;
// and the windows its answer read, as claude.KeptWindows keeps them, and
// its status across them, as its usage headers gave them.
type lineDetails struct {
	answer, served    string
	betas             []string
	system, maxTokens int
	effort            string
	stream            *bool
	blocks            []DetailBlock
	life              time.Duration
	windows           []claude.KeptWindow
	status            quota.Status
}

// detailsOf are what the details of the request of the line l give beyond
// its row.
func detailsOf(l *ledger.Line) *lineDetails {
	d := &lineDetails{
		answer: l.Answer.ID, served: interned(l.Answer.Model), system: l.Shape.System, effort: interned(l.Shape.OutputConfig.Effort),
		stream: l.Shape.Stream, blocks: blocksOf(l.Answer.Blocks), status: claude.LimitStatus(l.Limits),
	}
	for _, beta := range l.Betas {
		d.betas = append(d.betas, interned(undated(beta)))
	}
	if most := l.Shape.MaxTokens; most != nil {
		d.maxTokens = int(*most)
	}
	d.life, _ = l.CacheLife()
	d.windows = claude.KeptWindows(l.Limits)
	return d
}

// windowsRead are the windows d's answer read, as claude.WindowsReadOf has
// them, the account's reserve the one given.
func (d *lineDetails) windowsRead(reserve float64) []claude.WindowRead {
	return claude.WindowsReadOf(d.windows, reserve)
}

// loggedLineOf is the Log's line of l, taken in as the read's seqth,
// placed at place, its request worth what's given, and its row told of the
// story s of its session, nil for none.
func loggedLineOf(l *ledger.Line, seq int, place time.Time, worth Worth, s *story) *loggedLine {
	logged := &loggedLine{row: lineFacts(l), seq: seq, place: place, flight: flightOf(l), sum: sumOf(l), routed: routedOf(l), shown: detailsOf(l)}
	logged.row.Worth = worth
	logged.row.toldBy(logged.routed, s)
	return logged
}

// toldBy is l with its row told of the story s of its session, afresh.
func (l *loggedLine) toldBy(s *story) *loggedLine {
	told := *l
	told.row.toldBy(l.routed, s)
	return &told
}

// isRequest reports whether l is a session's request, as isRequest has it
// of its line.
func (l *loggedLine) isRequest() bool {
	return l.row.Kind == RowRequest && l.row.Session != ""
}

// interned is s, the one copy of it every string interned the same holds.
func interned(s string) string {
	return unique.Make(s).Value()
}
