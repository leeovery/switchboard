package views

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
)

// recordsOfLines are the records of lines, each decoded once, in order.
func recordsOfLines(lines []ledger.Line) []lineRecord {
	records := make([]lineRecord, len(lines))
	for i := range lines {
		records[i] = lineRecordOf(&lines[i])
	}
	return records
}

// dressed has l say what a router's line says beyond what the generated
// runs give, each now and then, as the seed's r draws it: the client that
// sent it, its betas, its shape, its answer's id, model and blocks, and its
// client going away.
func dressed(r *rand.Rand, l *ledger.Line) {
	l.Agent = []string{"", "claude-cli/2.1.300 (external, cli)", "claude-cli/2.2.0"}[r.IntN(3)]
	for _, beta := range []string{"claude-code-20250219", "context-1m-2025-08-07", "interleaved-thinking-2025-05-14"} {
		if r.IntN(2) == 0 {
			l.Betas = append(l.Betas, beta)
		}
	}
	l.Shape.Bytes, l.Shape.Messages, l.Shape.System, l.Shape.Tools = r.IntN(900_000), r.IntN(400), r.IntN(4), r.IntN(40)
	l.Shape.OutputConfig.Effort = []string{"", "high", "low"}[r.IntN(3)]
	if r.IntN(2) == 0 {
		l.Shape.MaxTokens = new(int64(32_000))
		l.Shape.Thinking.BudgetTokens = new(int64(r.IntN(32_000)))
	}
	if r.IntN(3) > 0 {
		l.Shape.Stream = new(r.IntN(2) == 0)
	}
	if r.IntN(4) > 0 {
		l.Answer.ID, l.Answer.Model = fmt.Sprintf("msg_%016x", r.Uint64()), l.Model
		l.Answer.Blocks = map[string]int{"thinking": 1, "text": 1, "tool_use": len(l.Answer.Tools), "mcp_tool_use": r.IntN(2)}
	}
	l.Canceled = l.Canceled || r.IntN(20) == 0
}

// twinOf is l but for what no record of it holds, as the seed's r draws it:
// what its headers hint, the shape's settings no row reads, the rest of its
// user agent after its name and version, its betas' dates, its kind among
// those a row counts among the requests, its usage's bytes, and a usage
// header that's no window's nor its status. Its record is l's.
func twinOf(r *rand.Rand, l ledger.Line) ledger.Line {
	twin := l
	twin.ParentAgentID, twin.AgentType, twin.Compaction, twin.Compacted = "parent", "Explore", "auto", "manual"
	twin.ToolMS = []ledger.ToolTime{{Tool: "Bash", MS: r.Int64N(9000)}}
	twin.Shape.ToolChoice.Type, twin.Shape.Temperature, twin.Shape.TopK = "auto", new(r.Float64()), new(int64(r.IntN(40)))
	twin.Shape.TopP, twin.Shape.ServiceTier, twin.Shape.Speed = new(r.Float64()), "priority", "fast"
	twin.Shape.Thinking.Type, twin.Shape.Thinking.Display = "enabled", "summarized"
	twin.Shape.ContextManagement.Edits = []string{"clear_tool_uses_20250919"}
	if name, _, _ := strings.Cut(strings.TrimSpace(l.Agent), " "); name != "" {
		twin.Agent = name + " (external, sdk-ts)"
	}
	twin.Betas = nil
	for _, beta := range l.Betas {
		if undated := undated(beta); undated != beta {
			beta = undated + "-2027-01-31"
		}
		twin.Betas = append(twin.Betas, beta)
	}
	if l.Kind == ledger.KindMessage {
		twin.Kind = []string{"", "batch"}[r.IntN(2)]
	}
	twin.Usage = rewritten(l.Usage)
	twin.Limits = map[string]string{"representative-claim": "five_hour"}
	maps.Copy(twin.Limits, l.Limits)
	if l.FirstMS != nil {
		twin.FirstMS = new(*l.FirstMS)
	}
	return twin
}

// rewritten is usage written again, its counts as they were, its keys in
// another order and spaced otherwise: usage itself where it's none, or
// isn't an object.
func rewritten(usage json.RawMessage) json.RawMessage {
	var counts map[string]json.RawMessage
	if json.Unmarshal(usage, &counts) != nil || counts == nil {
		return usage
	}
	var b bytes.Buffer
	if err := json.Indent(&b, mustJSON(counts), "", "\t"); err != nil {
		return usage
	}
	return b.Bytes()
}

// mustJSON is v as JSON: v is one that always encodes.
func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// shownOfLines are all the Log and a turn show of the lines filed by now,
// read alike: the Log's table over the days they span, what each day's
// lines count, and each row's details; each turn of the session "s", opened,
// its requests' rows and their details; the rows requests prints; each day's
// sessions, as the find holds them; and the sessions the Log wants the
// store to hold.
func shownOfLines(lines []ledger.Line, prices ledger.Table, now time.Time) []any {
	var shown []any
	read := (&logGrowing{lines: lines, prices: prices}).readAtOnce(now)
	table := LogRows(LogSources{From: lines[arrivalOrder(lines)[0]].At, Read: read, Now: now})
	shown = append(shown, shownLines(table)...)
	for _, l := range table.All() {
		d, _ := l.Details()
		shown = append(shown, d)
	}
	l, live := filedLedger{lines: lines, now: now}, PageLive{Now: now}
	page := ReadPage(l, "s", Stream{}, prices, 0, now)
	for n := 1; n <= len(page.worked(live).tally.turns); n++ {
		open := page.Opening(n, live)
		open = open.Turned(open.ReadTurn(l, live))
		for _, row := range open.TurnRequests(n, live) {
			d, _ := row.Details()
			shown = append(shown, row.Row, d)
		}
	}
	held := make([]ledger.Held, len(lines))
	byDay := make(map[string][]ledger.Held)
	for i := range lines {
		held[i] = ledger.Held{Line: lines[i]}
		byDay[Today(lines[i].At).Date] = append(byDay[Today(lines[i].At).Date], held[i])
	}
	for row := range RequestLines(slices.Values(held)) {
		shown = append(shown, row)
	}
	for _, date := range slices.Sorted(maps.Keys(byDay)) {
		for _, s := range DaySessions(RecordsOf(byDay[date])) {
			shown = append(shown, s)
		}
	}
	return append(shown, ReadLog(prices).Wanted(Stream{}, RecordsOf(held)))
}

// Two lines alike in all their records hold, as a line is to its twin,
// written otherwise only where no record reads it, give the Log, a turn,
// requests and the find the same: so whatever a row, its details or its
// count shows is its record's, read raw or restored alike. The runs' lines
// are dressed in all a router's line says, among them lines without ids,
// and the twins drawn afresh for each line.
func TestTwoLinesWhoseRecordsAreEqualGiveEqualRows(t *testing.T) {
	for seed := range uint64(40) {
		lines, _ := pagedRun(seed)
		r := rand.New(rand.NewPCG(seed, 12))
		for i := range lines {
			dressed(r, &lines[i])
		}
		twins := make([]ledger.Line, len(lines))
		for i := range lines {
			twins[i] = twinOf(r, lines[i])
			if a, b := lineRecordOf(&lines[i]), lineRecordOf(&twins[i]); !reflect.DeepEqual(a, b) {
				t.Fatalf("run %d: the twin of line %d holds the record\n%+v\nwant its line's\n%+v", seed, i, b, a)
			}
		}
		now := ended(&lines[len(lines)-1]).Add(time.Minute)
		got, want := shownOfLines(twins, changingPrices(), now), shownOfLines(lines, changingPrices(), now)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("run %d: the twins show otherwise than their lines\n%s", seed, unlike(got, want))
		}
	}
}

// The store's request of a line is the one its record decodes, as a turn's
// rows count a request of it the store hasn't been fed in its turn: one
// decoding of a line for both.
func TestALinesRecordDecodesTheStoresRequestOfIt(t *testing.T) {
	for seed := range uint64(20) {
		lines, _ := pagedRun(seed)
		for i := range lines {
			if got, want := newHeldRequest(lineRecordOf(&lines[i]).decodedRequest), heldRequestOf(&lines[i]); !reflect.DeepEqual(got, want) {
				t.Errorf("run %d, line %d: its record's request is\n%+v\nwant the store's\n%+v", seed, i, got, want)
			}
		}
	}
}

// A line's record counts its request as its line does, of every kind a line
// may say, of a session or of none: a request where ledger.Line.Counted
// counts it, a session's where it's of one too, as a line's isRequest has
// it, toward the session its line's CountedSession gives, and gone upstream
// as its Upstream says. So no rule of the record's own decides what counts.
func TestALinesRecordCountsItsRequestAsItsLineDoes(t *testing.T) {
	for _, kind := range []string{ledger.KindMessage, ledger.KindCheck, ledger.KindCount, "", "batch"} {
		for _, session := range []string{"", "s"} {
			l := recordedLine()
			l.Kind, l.Session = kind, session
			r := lineRecordOf(&l)
			got := []any{r.kind == RowRequest, r.isRequest(), r.countedSession, r.upstream}
			want := []any{l.Counted(), l.Counted() && l.Session != "", l.CountedSession(), l.Upstream()}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("of kind %q, session %q: the record's request, session's request, counted session and upstream are %v, want its line's, %v",
					kind, session, got, want)
			}
		}
	}
}

// A line's record holds a list its line gives empty as none, as of a line
// that gives none: the accounts its request went out on that couldn't serve
// it, and the tools its answer called. So what's restored of it is the
// record decoded, whichever its line gave.
func TestALinesRecordHoldsAnEmptyListAsNone(t *testing.T) {
	none, empty := recordedLine(), recordedLine()
	none.Tried, none.Answer.Tools = nil, nil
	empty.Tried, empty.Answer.Tools = []ledger.Tried{}, []string{}
	if got, want := lineRecordOf(&empty), lineRecordOf(&none); !reflect.DeepEqual(got, want) {
		t.Errorf("a line whose lists are empty has the record\n%+v\nwant the record of one whose lists are none\n%+v", got, want)
	}
}

// recordedLine is a line that says something of every fact a record holds.
func recordedLine() ledger.Line {
	at := time.Date(2026, 10, 7, 9, 0, 0, 123_000_000, time.UTC)
	return ledger.Line{
		At: at, Request: "r1", Kind: ledger.KindMessage, Session: "s", Dir: "/code/app", Model: foldOpus, Account: "side", Reason: "moved: work hit its limit",
		From: "work", Tried: []ledger.Tried{{Account: "work", Why: "hit its limit"}}, Status: 200, Canceled: true, Attempts: 2, FirstMS: new(int64(800)),
		TotalMS: 11_800, Agent: "claude-cli/2.1.300 (external, cli)", Betas: []string{"context-1m-2025-08-07"}, Prompt: "p1", Class: "subagent", AgentID: "a1",
		Shape: ledger.Shape{
			Bytes: 482_000, Messages: 12, System: 3, Tools: 9, MaxTokens: new(int64(32_000)), Thinking: ledger.Thinking{BudgetTokens: new(int64(16_000))},
			Stream: new(true), OutputConfig: ledger.OutputConfig{Effort: "high"}, InferenceGeo: "us",
		},
		Answer: ledger.Answer{ID: "msg_1", Model: foldOpus, Stop: "tool_use", Tools: []string{"Bash"}, Blocks: map[string]int{"text": 1, "tool_use": 1}},
		Usage:  json.RawMessage(foldUsages[2]),
		Limits: map[string]string{"status": "allowed", "5h-utilization": "0.42", "5h-reset": "1791370800", "5h-status": "allowed"},
	}
}

// recordEdits change each fact a record holds, by its field's name, from
// what recordedLine's record holds.
var recordEdits = map[string]func(r *lineRecord){
	"request":  func(r *lineRecord) { r.request = "r2" },
	"at":       func(r *lineRecord) { r.at.nsec++ },
	"end":      func(r *lineRecord) { r.end.sec++ },
	"dir":      func(r *lineRecord) { r.dir = "/code/other" },
	"model":    func(r *lineRecord) { r.model = foldHaiku },
	"account":  func(r *lineRecord) { r.account = "personal" },
	"reason":   func(r *lineRecord) { r.reason = "sticky" },
	"from":     func(r *lineRecord) { r.from = "personal" },
	"class":    func(r *lineRecord) { r.class = ledger.ClassMain },
	"agentID":  func(r *lineRecord) { r.agentID = "a2" },
	"geo":      func(r *lineRecord) { r.geo = "eu" },
	"prompt":   func(r *lineRecord) { r.prompt = "p2" },
	"stop":     func(r *lineRecord) { r.stop = "end_turn" },
	"tools":    func(r *lineRecord) { r.tools = append(slices.Clone(r.tools), "Read") },
	"upstream": func(r *lineRecord) { r.upstream = !r.upstream },
	"tried": func(r *lineRecord) {
		r.tried = append(slices.Clone(r.tried), ledger.Tried{Account: "side", Why: "refused"})
	},
	"spend": func(r *lineRecord) {
		r.spend = ledger.SpendOf(&ledger.Line{Kind: ledger.KindMessage, Usage: json.RawMessage(foldUsages[6])})
	},
	"kind":           func(r *lineRecord) { r.kind = RowCount },
	"session":        func(r *lineRecord) { r.session = "s2" },
	"countedSession": func(r *lineRecord) { r.countedSession = "" },
	"status":         func(r *lineRecord) { r.status = 529 },
	"canceled":       func(r *lineRecord) { r.canceled = !r.canceled },
	"cutOff":         func(r *lineRecord) { r.cutOff = !r.cutOff },
	"attempts":       func(r *lineRecord) { r.attempts++ },
	"firstMS":        func(r *lineRecord) { r.firstMS = nil },
	"totalMS":        func(r *lineRecord) { r.totalMS++ },
	"size":           func(r *lineRecord) { r.size++ },
	"messages":       func(r *lineRecord) { r.messages++ },
	"system":         func(r *lineRecord) { r.system++ },
	"toolDefs":       func(r *lineRecord) { r.toolDefs++ },
	"maxTokens":      func(r *lineRecord) { r.maxTokens++ },
	"thinking":       func(r *lineRecord) { r.thinking++ },
	"effort":         func(r *lineRecord) { r.effort = "low" },
	"stream":         func(r *lineRecord) { r.stream = new(false) },
	"agent":          func(r *lineRecord) { r.agent = "claude-cli/2.2.0" },
	"betas":          func(r *lineRecord) { r.betas = append(slices.Clone(r.betas), "claude-code") },
	"answer":         func(r *lineRecord) { r.answer = "msg_2" },
	"served":         func(r *lineRecord) { r.served = foldSonnet },
	"blocks":         func(r *lineRecord) { r.blocks = blocksOf(map[string]int{"text": 2}) },
	"windows": func(r *lineRecord) {
		r.windows = claude.KeptWindows(map[string]string{"5h-utilization": "0.43", "5h-reset": "1791370800"})
	},
	"limit": func(r *lineRecord) { r.limit = "allowed_warning" },
}

// fieldNames are the names of the fields of the struct type t, those of the
// structs it embeds in place of them.
func fieldNames(t reflect.Type) []string {
	var names []string
	for f := range t.Fields() {
		if f.Anonymous {
			names = append(names, fieldNames(f.Type)...)
		} else {
			names = append(names, f.Name)
		}
	}
	return names
}

// A record's sum is of every fact it holds, and of nothing else: two
// records alike, each of its own line, sum alike, and one that differs in
// any fact from another sums otherwise. Every field a record holds has its
// edit here, so a fact added to it is summed.
func TestALineRecordsSumIsOfEveryFactItHolds(t *testing.T) {
	a, b := recordedLine(), recordedLine()
	base := lineRecordOf(&a)
	if alike := lineRecordOf(&b); alike.summed() != base.summed() {
		t.Errorf("two records of lines alike sum %x and %x, want alike", alike.summed(), base.summed())
	}
	if got, want := slices.Sorted(maps.Keys(recordEdits)), slices.Sorted(slices.Values(fieldNames(reflect.TypeFor[lineRecord]()))); !slices.Equal(got, want) {
		t.Fatalf("the edits change the fields %v, want every field a record holds, %v", got, want)
	}
	for name, edit := range recordEdits {
		edited := lineRecordOf(&a)
		edit(&edited)
		switch {
		case reflect.DeepEqual(edited, base):
			t.Errorf("editing %s leaves the record as it was, want an edit that changes it", name)
		case edited.summed() == base.summed():
			t.Errorf("a record differing in its %s sums as the record it differs from, want otherwise", name)
		}
	}
}
