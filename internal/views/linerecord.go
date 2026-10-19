package views

import (
	"hash/maphash"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// lineRecord is a line as the Log and a turn's rows read it, decoded once:
// all a held request decodes of it, as decodedRequest has it; and the Log's
// facts beyond: what it asked; the session it's of; what it was answered,
// and whether its client went away or the router cut it off; how many
// times it went upstream; how long after it arrived its answer's first byte
// came, nil where none did, and its end; its shape's size, messages, system
// blocks, definitions of tools, max_tokens, thinking budget, effort and
// whether it asked to stream, nil where it didn't say; the client that sent
// it, cut to its name and version; its betas, each undated; its answer's id,
// the model that served it and its blocks, counted by kind, in answerOrder;
// and the windows its answer read, as claude.KeptWindows keeps them, and its
// status across them. Every row, details card and sum of a line is of its
// record alone, so a record restored tells its line as the one decoded of it
// did.
type lineRecord struct {
	decodedRequest
	kind                             RowKind
	session                          string
	status                           int
	canceled, cutOff                 bool
	attempts                         int
	firstMS                          *int64
	totalMS                          int64
	size, messages, system, toolDefs int
	maxTokens, thinking              int
	effort                           string
	stream                           *bool
	agent                            string
	betas                            []string
	answer, served                   string
	blocks                           []DetailBlock
	windows                          []claude.KeptWindow
	limit                            quota.Status
}

// lineRecordOf returns the record of the line l, decoded once.
func lineRecordOf(l *ledger.Line) lineRecord {
	r := lineRecord{
		decodedRequest: decodedOf(l), kind: kindOf(l), session: l.Session, status: l.Status, canceled: l.Canceled, cutOff: l.CutOff,
		attempts: l.Attempts, firstMS: l.FirstMS, totalMS: l.TotalMS, size: l.Shape.Bytes, messages: l.Shape.Messages, system: l.Shape.System,
		toolDefs: l.Shape.Tools, effort: l.Shape.OutputConfig.Effort, stream: l.Shape.Stream, agent: agentOf(l.Agent), answer: l.Answer.ID,
		served: l.Answer.Model, blocks: blocksOf(l.Answer.Blocks), windows: claude.KeptWindows(l.Limits), limit: claude.LimitStatus(l.Limits),
	}
	if most := l.Shape.MaxTokens; most != nil {
		r.maxTokens = int(*most)
	}
	if budget := l.Shape.Thinking.BudgetTokens; budget != nil {
		r.thinking = int(*budget)
	}
	for _, beta := range l.Betas {
		r.betas = append(r.betas, undated(beta))
	}
	return r
}

// decodedOf returns all a held request decodes of the line l: the one
// decoding of it, both the store's request and the line's record put
// together of it.
func decodedOf(l *ledger.Line) decodedRequest {
	return decodedRequest{
		flight: flightOf(l), dir: l.Dir, model: l.Model, account: l.Account, reason: l.Reason, from: l.From, class: l.Class, agentID: l.AgentID,
		geo: l.Shape.InferenceGeo, prompt: l.Prompt, stop: l.Answer.Stop, tools: l.Answer.Tools, upstream: l.Upstream(), tried: l.Tried, spend: ledger.SpendOf(l),
	}
}

// LogDay is lines as the Log takes them in, each decoded once into its
// record, in their order: a day's every one, as Ledger's Day gives them, or
// today's gained since a mark, as its Today gives them.
type LogDay struct {
	records []lineRecord
}

// RecordsOf is the LogDay of lines, each its record, in their order.
func RecordsOf(lines []ledger.Held) LogDay {
	day := LogDay{records: make([]lineRecord, len(lines))}
	for i := range lines {
		day.records[i] = lineRecordOf(&lines[i].Line)
	}
	return day
}

// isRequest reports whether r is of a session's request, as isRequest has
// it of its line.
func (r *lineRecord) isRequest() bool {
	return r.kind == RowRequest && r.session != ""
}

// countedSession is the session r's request counts toward, as
// ledger.Line.CountedSession has it of its line: none for a quota check.
func (r *lineRecord) countedSession() string {
	if r.kind == RowCheck {
		return ""
	}
	return r.session
}

// worth is what r's request was worth, at the prices in effect on the local
// day with the given date, as ledger.Table.SummedSpend gives it of what it
// spent: no Amount where the table priced none of it.
func (r *lineRecord) worth(prices ledger.Table, date string) Worth {
	var w Worth
	w.addSummed(prices.SummedSpend(r.model, r.geo, r.spend, date))
	return w
}

// summed is a sum of all r holds, its spend as ledger.Spend's Sum has it:
// two records whose sums differ hold otherwise, and two whose sums are the
// same hold the same, but by a chance of one in 2^64. So a record restored
// sums as the one decoded of its line did, and two lines that say otherwise
// only of what no record holds, as their usages' bytes, sum alike. It's
// never compared with a held request's sum, as heldRequest's summed has it.
func (r *lineRecord) summed() uint64 {
	var h maphash.Hash
	h.SetSeed(sumsSeed)
	firstMS, answered := valueOf(r.firstMS)
	stream, streamed := valueOf(r.stream)
	maphash.WriteComparable(&h, struct {
		flight
		dir, model, account, reason, from, class, agentID, geo, prompt, stop string
		session, effort, agent, answer, served                               string
		kind                                                                 RowKind
		limit                                                                quota.Status
		status, attempts, size, messages, system, toolDefs                   int
		maxTokens, thinking                                                  int
		firstMS, totalMS                                                     int64
		upstream, canceled, cutOff, answered, stream, streamed               bool
		tools, tried, betas, blocks, windows                                 int
		spend                                                                uint64
	}{
		r.flight, r.dir, r.model, r.account, r.reason, r.from, r.class, r.agentID, r.geo, r.prompt, r.stop,
		r.session, r.effort, r.agent, r.answer, r.served, r.kind, r.limit, r.status, r.attempts, r.size, r.messages, r.system, r.toolDefs,
		r.maxTokens, r.thinking, firstMS, r.totalMS, r.upstream, r.canceled, r.cutOff, answered, stream, streamed,
		len(r.tools), len(r.tried), len(r.betas), len(r.blocks), len(r.windows), r.spend.Sum(sumsSeed),
	})
	writeEach(&h, r.tools)
	writeEach(&h, r.tried)
	writeEach(&h, r.betas)
	writeEach(&h, r.blocks)
	writeEach(&h, r.windows)
	return h.Sum64()
}

// writeEach writes each of items into h, in order.
func writeEach[T comparable](h *maphash.Hash, items []T) {
	for _, item := range items {
		maphash.WriteComparable(h, item)
	}
}

// valueOf is what p points at, reporting false where it's nil: the zero
// value then.
func valueOf[T any](p *T) (T, bool) {
	if p == nil {
		var zero T
		return zero, false
	}
	return *p, true
}
