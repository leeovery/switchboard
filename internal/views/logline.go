package views

import (
	"maps"
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// loggedLine is a line of the Log's, as its read holds it in place of the
// line, of its record, as it's taken in: its row, told once of its session's
// story and told again only as the store says the story changed, its words
// sharing the strings the store's requests name; when it was taken in among
// the read's lines, and where its row sits within its day, as placedRow has
// it; its request's flight, and, of one without an id, the sum of its
// record, as its summed has it; how the router routed it, as its routing is
// told again; and what its details give beyond its row. The table's lines
// point at it, so a row is held once. Its day holds the handles of its
// words, as interning has them.
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

// routedOf is how the router routed the request of the record r, its words
// interned by n.
func routedOf(r *rowRecord, n interning) routed {
	return routed{tried: r.tried, from: n.of(r.from), reason: n.of(r.reason)}
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

// detailsOf are what the details of the request of the record r give
// beyond its row, their words interned by n.
func detailsOf(r *lineRecord, n interning) *lineDetails {
	d := &lineDetails{
		answer: r.answer, served: n.of(r.served), system: r.system, maxTokens: r.maxTokens, effort: n.of(r.effort), stream: r.stream,
		blocks: r.blocks, windows: r.windows, status: r.limit,
	}
	for _, beta := range r.betas {
		d.betas = append(d.betas, n.of(beta))
	}
	d.life, _ = r.spend.CacheLife()
	return d
}

// windowsRead are the windows d's answer read, as claude.WindowsReadOf has
// them, the account's reserve the one given.
func (d *lineDetails) windowsRead(reserve float64) []claude.WindowRead {
	return claude.WindowsReadOf(d.windows, reserve)
}

// loggedLineOf is the Log's line of the record r, taken in as the read's
// seqth, placed at place, its request worth what's given, its row told of
// the story s of its session, nil for none, and its words interned by n.
func loggedLineOf(r *lineRecord, seq int, place time.Time, worth Worth, s *story, n interning) *loggedLine {
	logged := &loggedLine{row: lineFacts(&r.rowRecord, n), seq: seq, place: place, flight: r.flight, routed: routedOf(&r.rowRecord, n), shown: detailsOf(r, n)}
	if r.request == "" {
		logged.sum = r.summed()
	}
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

// interning interns the words of lines, each the one copy every word
// interned the same holds, holding the handle of each, so that copy stays
// the one shared for as long as the holder is held: a word alone doesn't
// keep its copy interned, so one interned once that's collected is copied
// afresh. A nil one holds none, for words held no longer than a look.
type interning map[unique.Handle[string]]struct{}

// of is s interned, its handle held by n.
func (n interning) of(s string) string {
	h := unique.Make(s)
	if n != nil {
		n[h] = struct{}{}
	}
	return h.Value()
}

// mergedNames are the handles under and over, afresh.
func mergedNames(under, over interning) interning {
	merged := maps.Clone(under)
	maps.Copy(merged, over)
	return merged
}

// heldName reports whether the handles held hold h.
func heldName(held *stacked[interning], h unique.Handle[string]) bool {
	for layer := held; layer != nil; layer = layer.under {
		if _, ok := layer.top[h]; ok {
			return true
		}
	}
	return false
}
