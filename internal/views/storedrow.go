package views

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
)

// storedRow is a line's record as a day's rows store it, a line of its
// chunk: all it decodes of its line as the store's request does, in
// storedRequest's own form; what it asked, by its kind's code, as
// storedKinds has it; and the Log's facts beyond, each as the record holds
// it, its answer's blocks as storedBlock has them and the windows its answer
// read in claude.KeptWindow's own stored form. It holds facts alone: never a
// price, as what its request was worth is priced of what it spent at the
// reader's day; nor what its session's story tells of it, whether it's the
// session's first, wrote its context again, how it was routed and so its
// mark, told afresh as it's read. So it restores to the record it stores
// exactly. Its keys are short and its zeros left out, a pointer's nil apart
// from what it points at.
type storedRow struct {
	storedRequest
	Kind           string              `json:"k,omitempty"`
	Session        string              `json:"s,omitempty"`
	CountedSession string              `json:"cs,omitempty"`
	Status         int                 `json:"sc,omitzero"`
	Canceled       bool                `json:"cn,omitzero"`
	CutOff         bool                `json:"co,omitzero"`
	Attempts       int                 `json:"at,omitzero"`
	FirstMS        *int64              `json:"fm,omitempty"`
	TotalMS        int64               `json:"tm,omitzero"`
	Size           int                 `json:"sz,omitzero"`
	Messages       int                 `json:"ms,omitzero"`
	System         int                 `json:"sy,omitzero"`
	ToolDefs       int                 `json:"td,omitzero"`
	MaxTokens      int                 `json:"mx,omitzero"`
	Thinking       int                 `json:"th,omitzero"`
	Effort         string              `json:"ef,omitempty"`
	Stream         *bool               `json:"sm,omitempty"`
	Agent          string              `json:"ua,omitempty"`
	Betas          []string            `json:"b,omitempty"`
	Answer         string              `json:"id,omitempty"`
	Served         string              `json:"sv,omitempty"`
	Blocks         []storedBlock       `json:"bk,omitempty"`
	Windows        []claude.KeptWindow `json:"w,omitempty"`
	Limit          quota.Status        `json:"l,omitempty"`
}

// storedKinds are the codes a stored row gives each RowKind by, what its
// request asked: a request's none, as most rows are one's. A kind's own
// code, never its number, so the kinds' order can't change what a row
// stored says.
var storedKinds = [...]string{RowRequest: "", RowCheck: "check", RowCount: "count"}

// A kind added to rowKinds fails to compile here until storedKinds codes it.
var _ = [1]int{}[len(storedKinds)-len(rowKinds)]

// storedBlock is an answer's blocks of a kind, as a stored row holds them:
// the kind, and how many.
type storedBlock struct {
	Kind  string `json:"k,omitempty"`
	Count int    `json:"n,omitzero"`
}

// stored is r as a day's rows store it.
func (r *lineRecord) stored() storedRow {
	return storedRow{
		storedRequest: r.decodedRequest.stored(), Kind: storedKinds[r.kind], Session: r.session, CountedSession: r.countedSession, Status: r.status,
		Canceled: r.canceled, CutOff: r.cutOff, Attempts: r.attempts, FirstMS: r.firstMS, TotalMS: r.totalMS, Size: r.size, Messages: r.messages,
		System: r.system, ToolDefs: r.toolDefs, MaxTokens: r.maxTokens, Thinking: r.thinking, Effort: r.effort, Stream: r.stream, Agent: r.agent,
		Betas: r.betas, Answer: r.answer, Served: r.served, Blocks: eachAs(r.blocks, func(b DetailBlock) storedBlock { return storedBlock(b) }),
		Windows: r.windows, Limit: r.limit,
	}
}

// restored is the record s stores, exactly as it was decoded of its line:
// refused where s gives what its request asked by a code storedKinds hasn't.
func (s *storedRow) restored() (lineRecord, error) {
	kind := slices.Index(storedKinds[:], s.Kind)
	if kind < 0 {
		return lineRecord{}, fmt.Errorf("restore a row: no kind is stored as %q", s.Kind)
	}
	return lineRecord{
		decodedRequest: s.decoded(), kind: RowKind(kind), session: s.Session, countedSession: s.CountedSession, status: s.Status,
		canceled: s.Canceled, cutOff: s.CutOff, attempts: s.Attempts, firstMS: s.FirstMS, totalMS: s.TotalMS, size: s.Size, messages: s.Messages,
		system: s.System, toolDefs: s.ToolDefs, maxTokens: s.MaxTokens, thinking: s.Thinking, effort: s.Effort, stream: s.Stream, agent: s.Agent,
		betas: s.Betas, answer: s.Answer, served: s.Served, blocks: eachAs(s.Blocks, func(b storedBlock) DetailBlock { return DetailBlock(b) }),
		windows: s.Windows, limit: s.Limit,
	}, nil
}

// rowsChunk is a chunk of a day's rows as it's encoded of their records,
// taken in the order the day gives them, by arrival: each record's stored
// row, a JSON line, in the order taken; and its meta, of every row taken.
type rowsChunk struct {
	rows []byte
	meta chunkMeta
}

// take encodes the stored row of r as c's next, and counts r in c's meta.
func (c *rowsChunk) take(r *lineRecord) error {
	row, err := json.Marshal(r.stored())
	if err != nil {
		return fmt.Errorf("encode a row: %w", err)
	}
	c.rows = append(append(c.rows, row...), '\n')
	c.meta.take(r)
	return nil
}

// taken is how many rows c holds.
func (c *rowsChunk) taken() int {
	return c.meta.lines.Lines()
}

// builtChunk is a chunk of a day's rows as it's built, as the ledger keeps
// it: its rows, the stored row of each record it took, a JSON line each, in
// the order taken; how many; and its meta, in its stored form.
type builtChunk struct {
	rows  []byte
	count int
	meta  json.RawMessage
}

// built is c as it's built of the records it took, as builtChunk has it.
func (c *rowsChunk) built() (builtChunk, error) {
	meta, err := json.Marshal(c.meta.stored())
	if err != nil {
		return builtChunk{}, fmt.Errorf("encode a chunk's meta: %w", err)
	}
	return builtChunk{rows: c.rows, count: c.taken(), meta: meta}, nil
}

// restoredRows are records, with the records of a chunk's rows appended,
// rows as builtChunk has them, each restored exactly as its stored row has
// it, in order: refused where a line of rows isn't a row stored.
func restoredRows(records []lineRecord, rows []byte) ([]lineRecord, error) {
	records = slices.Grow(records, bytes.Count(rows, []byte{'\n'}))
	for line := range bytes.Lines(rows) {
		var s storedRow
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("decode a row: %w", err)
		}
		r, err := s.restored()
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

// chunkMeta is what a chunk of a day's rows says of them without a row
// read: when the first and the last of them arrived; and what they count, as
// the Log counts a day's lines whatever its filter, as LogCount's count has
// it: those of each kind, those that failed, and by the models they asked
// for, the accounts they're on and the sessions they count toward. So a
// reader tells which chunks may hold a session's rows, or rows of a time,
// and sums a day's count, reading none.
type chunkMeta struct {
	first, last instant
	lines       LogCount
}

// take counts the record r, the chunk's next, in m: when it arrived, and its
// row as its line alone tells it, as lineFacts has it.
func (m *chunkMeta) take(r *lineRecord) {
	none := m.lines.Lines() == 0
	if none || r.at.compare(m.first) < 0 {
		m.first = r.at
	}
	if none || r.at.compare(m.last) > 0 {
		m.last = r.at
	}
	row := lineFacts(r, nil)
	m.lines.count(&row)
}

// storedChunkMeta is a chunk's meta as its stored form holds it: when the
// first and the last of its rows arrived, exactly; how many of them asked
// what each kind asks; how many failed; and how many are of each model,
// account and session, by its whole id. Its keys are short and its zeros
// left out.
type storedChunkMeta struct {
	First    storedInstant  `json:"f,omitzero"`
	Last     storedInstant  `json:"l,omitzero"`
	Requests int            `json:"rq,omitzero"`
	Checks   int            `json:"ck,omitzero"`
	Counts   int            `json:"cn,omitzero"`
	Failed   int            `json:"x,omitzero"`
	Models   map[string]int `json:"m,omitempty"`
	Accounts map[string]int `json:"a,omitempty"`
	Sessions map[string]int `json:"s,omitempty"`
}

// stored is m as its stored form holds it.
func (m *chunkMeta) stored() storedChunkMeta {
	c := m.lines
	return storedChunkMeta{
		First: m.first.stored(), Last: m.last.stored(), Requests: c.Requests, Checks: c.Checks, Counts: c.Counts, Failed: c.Failed,
		Models: c.Models, Accounts: c.Accounts, Sessions: c.Sessions,
	}
}

// chunkMetaOf is the meta of a chunk whose stored form is meta, as
// builtChunk has it, exactly as it was built.
func chunkMetaOf(meta []byte) (chunkMeta, error) {
	var s storedChunkMeta
	if err := json.Unmarshal(meta, &s); err != nil {
		return chunkMeta{}, fmt.Errorf("decode a chunk's meta: %w", err)
	}
	return chunkMeta{
		first: s.First.instant(), last: s.Last.instant(),
		lines: LogCount{
			Requests: s.Requests, Checks: s.Checks, Counts: s.Counts, Failed: s.Failed, Models: s.Models, Accounts: s.Accounts, Sessions: s.Sessions,
		},
	}, nil
}
