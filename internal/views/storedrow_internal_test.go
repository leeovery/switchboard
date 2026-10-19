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
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// unpricedModel is the id of a model no price knows.
const unpricedModel = "claude-unknown-1"

// oddUsages are usages the ledger reads otherwise than their counts derive,
// or with a count no price knows: a count named in capitals; writes named
// beside cache_creation; writes of a last the prices don't know; and a
// server tool the prices don't know.
var oddUsages = []string{
	`{"Input_Tokens":12,"output_tokens":10}`,
	`{"cache_creation_input_tokens":100,"cache_creation.ephemeral_5m_input_tokens":40,"cache_creation":{"ephemeral_1h_input_tokens":60},"cache_creation.unknown":3}`,
	`{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_1h_input_tokens":3000,"ephemeral_1d_input_tokens":120}}`,
	`{"output_tokens":5,"server_tool_use":{"code_execution_requests":1}}`,
}

// oddKinds, oddStatuses and limitStatuses are what a line may say it asked,
// as Line.Kind has it, of every kind a row tells, a kind a later router may
// add and none among them; what it was answered, nothing among it; and its
// answer's status across its windows, none among them.
var (
	oddKinds      = []string{ledger.KindMessage, ledger.KindCheck, ledger.KindCount, "batch", ""}
	oddStatuses   = []int{0, 200, 400, 429, 500, 529}
	limitStatuses = []quota.Status{"", quota.StatusAllowed, quota.StatusAllowedWarning, quota.StatusRejected}
)

// oddly has l say, now and then as r draws it, what a router's line seldom
// says, or one edited by hand may: no id; a usage as oddUsages has them; a
// model no price knows; no usage, though it went upstream; what it asked, as
// oddKinds has it; what it was answered, as oddStatuses has it; and its
// answer's status across its windows, as limitStatuses has it.
func oddly(r *rand.Rand, l *ledger.Line) {
	switch r.IntN(10) {
	case 0:
		l.Request = ""
	case 1:
		l.Model = unpricedModel
	}
	switch r.IntN(8) {
	case 0:
		l.Usage = json.RawMessage(oddUsages[r.IntN(len(oddUsages))])
	case 1:
		l.Usage, l.Attempts = nil, 1+r.IntN(3)
	}
	if r.IntN(4) == 0 {
		l.Kind = oddKinds[r.IntN(len(oddKinds))]
	}
	if r.IntN(4) == 0 {
		l.Status = oddStatuses[r.IntN(len(oddStatuses))]
	}
	limits := maps.Clone(l.Limits)
	if limits == nil {
		limits = make(map[string]string)
	}
	if status := limitStatuses[r.IntN(len(limitStatuses))]; status != "" {
		limits["status"] = string(status)
	}
	l.Limits = limits
}

// eastward and westward are zones a line's arrival may be given in, as a
// line read back gives it in UTC and a line made in memory needn't.
var (
	eastward = time.FixedZone("eastward", 5*60*60+30*60)
	westward = time.FixedZone("westward", -7*60*60)
)

// oddlyTimed has l arrive, now and then as r draws it, otherwise than a
// router's line does: to the nanosecond, in another zone than UTC; or, as a
// line edited by hand may, at no time at all, or beyond the years
// nanoseconds since the epoch hold.
func oddlyTimed(r *rand.Rand, l *ledger.Line) {
	switch p := r.IntN(20); {
	case p == 0:
		l.At = time.Time{}
	case p == 1:
		l.At = time.Date(2300, 1, 1, 0, 0, 0, r.IntN(1e9), time.UTC)
	case p < 12:
		l.At = l.At.Add(time.Duration(1 + r.IntN(999_999))).In([]*time.Location{eastward, westward}[r.IntN(2)])
	}
}

// odds are what a line may say that its record, restored, must hold
// exactly, each by its name, judged of the line and its record.
var odds = []struct {
	name string
	says func(l *ledger.Line, r *lineRecord) bool
}{
	{"no id", func(_ *ledger.Line, r *lineRecord) bool { return r.request == "" }},
	{"an odd usage", func(l *ledger.Line, _ *lineRecord) bool { return slices.Contains(oddUsages, string(l.Usage)) }},
	{"an unpriced model", func(_ *ledger.Line, r *lineRecord) bool { return r.model == unpricedModel }},
	{"unmetered", func(l *ledger.Line, _ *lineRecord) bool { return l.Upstream() && len(l.Usage) == 0 }},
	{"windows", func(_ *ledger.Line, r *lineRecord) bool { return len(r.windows) > 0 }},
	{"tried accounts", func(_ *ledger.Line, r *lineRecord) bool { return len(r.tried) > 0 }},
	{"betas", func(_ *ledger.Line, r *lineRecord) bool { return len(r.betas) > 0 }},
	{"a geo", func(_ *ledger.Line, r *lineRecord) bool { return r.geo != "" }},
	{"blocks", func(_ *ledger.Line, r *lineRecord) bool { return len(r.blocks) > 0 }},
	{"nanoseconds", func(_ *ledger.Line, r *lineRecord) bool { return r.at.nsec%1e6 != 0 }},
	{"another zone", func(l *ledger.Line, _ *lineRecord) bool { return l.At.Location() != time.UTC }},
	{"no time at all", func(l *ledger.Line, _ *lineRecord) bool { return l.At.IsZero() }},
	{"beyond nanoseconds", func(l *ledger.Line, _ *lineRecord) bool { return l.At.Year() > 2262 }},
	{"a first byte", func(_ *ledger.Line, r *lineRecord) bool { return r.firstMS != nil }},
	{"no answer streamed", func(_ *ledger.Line, r *lineRecord) bool { return r.stream != nil && !*r.stream }},
	{"a subagent's", func(_ *ledger.Line, r *lineRecord) bool { return r.agentID != "" }},
	{"counted toward no one", func(_ *ledger.Line, r *lineRecord) bool { return r.session != "" && r.countedSession == "" }},
	{"a client gone", func(_ *ledger.Line, r *lineRecord) bool { return r.canceled }},
	{"cut off by its router", func(_ *ledger.Line, r *lineRecord) bool { return r.cutOff }},
	{"its answer's own model", func(_ *ledger.Line, r *lineRecord) bool { return r.served != "" }},
}

// oddsOf are the names of what the line l, of the record rec, says, as
// odds judge it, and of its kind, its status and its answer's status across
// its windows.
func oddsOf(l *ledger.Line, rec *lineRecord) []string {
	said := []string{fmt.Sprintf("kind %d", rec.kind), fmt.Sprintf("status %d", rec.status), fmt.Sprintf("limit %q", rec.limit)}
	for _, odd := range odds {
		if odd.says(l, rec) {
			said = append(said, odd.name)
		}
	}
	return said
}

// oddsWanted are the names of what the lines a property's runs are of must
// say between them, as oddsOf names them: each of odds, every kind, every
// status oddStatuses has and every status limitStatuses has.
func oddsWanted() []string {
	var wanted []string
	for _, odd := range odds {
		wanted = append(wanted, odd.name)
	}
	for _, k := range rowKinds {
		wanted = append(wanted, fmt.Sprintf("kind %d", k))
	}
	for _, s := range oddStatuses {
		wanted = append(wanted, fmt.Sprintf("status %d", s))
	}
	for _, s := range limitStatuses {
		wanted = append(wanted, fmt.Sprintf("limit %q", s))
	}
	return wanted
}

// oddRun is the lines of the run the seed given makes, as pagedRun makes
// them, and of the spread world's, as spreadWorld makes it, each dressed in
// all a router's line says, as dressed has it, and said oddly, as oddly and
// oddlyTimed have it.
func oddRun(seed uint64) []ledger.Line {
	paged, _ := pagedRun(seed)
	spread, early, late := spreadWorld(seed)
	lines := slices.Concat(paged, spread, []ledger.Line{early, late})
	r := rand.New(rand.NewPCG(seed, 13))
	for i := range lines {
		dressed(r, &lines[i])
		oddly(r, &lines[i])
		oddlyTimed(r, &lines[i])
	}
	return lines
}

// encodedChunk is the chunk of records, as rowsChunk encodes them, taken in
// order: as it's built, and its meta as it took them.
func encodedChunk(t *testing.T, records []lineRecord) (builtChunk, chunkMeta) {
	t.Helper()
	var c rowsChunk
	for i := range records {
		if err := c.take(&records[i]); err != nil {
			t.Fatal(err)
		}
	}
	built, err := c.built()
	if err != nil {
		t.Fatal(err)
	}
	return built, c.meta
}

// A record stored as a chunk's row is restored exactly as it was decoded of
// its line, of every line a run gives, among them lines without ids, odd
// usages, a model no price knows, lines gone upstream without usage, checks
// and counts, windows read, accounts tried, betas, geos, and arrivals to
// the nanosecond, in other zones, at no time at all and beyond the years
// nanoseconds hold: and of every kind and status, each of those met by some
// run. And the chunk's meta is restored exactly as it was built.
func TestARecordIsRestoredFromItsStoredRowExactly(t *testing.T) {
	met := make(map[string]bool)
	for seed := range uint64(30) {
		lines := oddRun(seed)
		records := recordsOfLines(lines)
		for i := range lines {
			for _, odd := range oddsOf(&lines[i], &records[i]) {
				met[odd] = true
			}
		}
		built, meta := encodedChunk(t, records)
		restored, err := restoredRows(nil, built.rows)
		if err != nil {
			t.Fatalf("run %d: restoring its rows: %v", seed, err)
		}
		if len(restored) != len(records) || built.count != len(records) {
			t.Fatalf("run %d: %d records restored of %d built, want %d", seed, len(restored), built.count, len(records))
		}
		for i := range records {
			if !reflect.DeepEqual(restored[i], records[i]) {
				row, _ := json.Marshal(records[i].stored())
				t.Errorf("run %d, line %d, stored as %s: restored as\n%+v\nwant\n%+v", seed, i, row, restored[i], records[i])
			}
		}
		if got, err := chunkMetaOf(built.meta); err != nil || !reflect.DeepEqual(got, meta) {
			t.Errorf("run %d: its meta, stored as %s, is restored as %+v, %v, want %+v", seed, built.meta, got, err, meta)
		}
	}
	for _, odd := range oddsWanted() {
		if !met[odd] {
			t.Errorf("no run's line says %s, want one that does", odd)
		}
	}
}

// fieldStored is a fact of a record, set alone on the zero record: the
// record's field it's held in, and its row's JSON, as storedRow gives it.
type fieldStored struct {
	field string
	set   func(r *lineRecord)
	want  string
}

// fieldsStored are a case of each fact a record holds, by its field, the
// kinds of what a request asked each a case of their own: so each key a
// row stores by, and how it stores what it holds, is pinned.
var fieldsStored = []fieldStored{
	{field: "request", set: func(r *lineRecord) { r.request = "r1" }, want: `{"r":"r1"}`},
	{field: "at", set: func(r *lineRecord) { r.at = instant{sec: 1791450000, nsec: 123_456_789} }, want: `{"a":{"s":1791450000,"n":123456789}}`},
	{field: "end", set: func(r *lineRecord) { r.end = instantOf(time.Time{}) }, want: `{"e":{"s":-62135596800}}`},
	{field: "dir", set: func(r *lineRecord) { r.dir = "/code/app" }, want: `{"d":"/code/app"}`},
	{field: "model", set: func(r *lineRecord) { r.model = foldOpus }, want: `{"m":"claude-opus-5-5"}`},
	{field: "account", set: func(r *lineRecord) { r.account = "work" }, want: `{"ac":"work"}`},
	{field: "reason", set: func(r *lineRecord) { r.reason = "sticky" }, want: `{"rs":"sticky"}`},
	{field: "from", set: func(r *lineRecord) { r.from = "side" }, want: `{"f":"side"}`},
	{field: "class", set: func(r *lineRecord) { r.class = ledger.ClassMain }, want: `{"c":"main"}`},
	{field: "agentID", set: func(r *lineRecord) { r.agentID = "a1" }, want: `{"ag":"a1"}`},
	{field: "geo", set: func(r *lineRecord) { r.geo = "us" }, want: `{"g":"us"}`},
	{field: "prompt", set: func(r *lineRecord) { r.prompt = "p1" }, want: `{"p":"p1"}`},
	{field: "stop", set: func(r *lineRecord) { r.stop = "tool_use" }, want: `{"st":"tool_use"}`},
	{field: "tools", set: func(r *lineRecord) { r.tools = []string{"Read", "Edit"} }, want: `{"tl":["Read","Edit"]}`},
	{field: "upstream", set: func(r *lineRecord) { r.upstream = true }, want: `{"u":true}`},
	{
		field: "tried", set: func(r *lineRecord) { r.tried = []ledger.Tried{{Account: "work", Why: "hit its limit"}} },
		want: `{"t":[{"a":"work","w":"hit its limit"}]}`,
	},
	{
		field: "spend",
		set: func(r *lineRecord) {
			r.spend = ledger.SpendOf(&ledger.Line{Kind: ledger.KindMessage, Usage: json.RawMessage(`{"input_tokens":10,"output_tokens":300}`)})
		},
		want: `{"sp":{"i":10,"o":300,"d":true,"c":true}}`,
	},
	{field: "kind", set: func(r *lineRecord) { r.kind = RowCheck }, want: `{"k":"check"}`},
	{field: "kind", set: func(r *lineRecord) { r.kind = RowCount }, want: `{"k":"count"}`},
	{field: "session", set: func(r *lineRecord) { r.session = "s" }, want: `{"s":"s"}`},
	{field: "countedSession", set: func(r *lineRecord) { r.countedSession = "s" }, want: `{"cs":"s"}`},
	{field: "status", set: func(r *lineRecord) { r.status = 529 }, want: `{"sc":529}`},
	{field: "canceled", set: func(r *lineRecord) { r.canceled = true }, want: `{"cn":true}`},
	{field: "cutOff", set: func(r *lineRecord) { r.cutOff = true }, want: `{"co":true}`},
	{field: "attempts", set: func(r *lineRecord) { r.attempts = 2 }, want: `{"at":2}`},
	{field: "firstMS", set: func(r *lineRecord) { r.firstMS = new(int64(0)) }, want: `{"fm":0}`},
	{field: "totalMS", set: func(r *lineRecord) { r.totalMS = 11_800 }, want: `{"tm":11800}`},
	{field: "size", set: func(r *lineRecord) { r.size = 482_000 }, want: `{"sz":482000}`},
	{field: "messages", set: func(r *lineRecord) { r.messages = 12 }, want: `{"ms":12}`},
	{field: "system", set: func(r *lineRecord) { r.system = 3 }, want: `{"sy":3}`},
	{field: "toolDefs", set: func(r *lineRecord) { r.toolDefs = 9 }, want: `{"td":9}`},
	{field: "maxTokens", set: func(r *lineRecord) { r.maxTokens = 32_000 }, want: `{"mx":32000}`},
	{field: "thinking", set: func(r *lineRecord) { r.thinking = 16_000 }, want: `{"th":16000}`},
	{field: "effort", set: func(r *lineRecord) { r.effort = "high" }, want: `{"ef":"high"}`},
	{field: "stream", set: func(r *lineRecord) { r.stream = new(false) }, want: `{"sm":false}`},
	{field: "agent", set: func(r *lineRecord) { r.agent = "claude-cli/2.1.300" }, want: `{"ua":"claude-cli/2.1.300"}`},
	{field: "betas", set: func(r *lineRecord) { r.betas = []string{"context-1m"} }, want: `{"b":["context-1m"]}`},
	{field: "answer", set: func(r *lineRecord) { r.answer = "msg_01" }, want: `{"id":"msg_01"}`},
	{field: "served", set: func(r *lineRecord) { r.served = foldOpus }, want: `{"sv":"claude-opus-5-5"}`},
	{
		field: "blocks", set: func(r *lineRecord) { r.blocks = blocksOf(map[string]int{"text": 2, "thinking": 1, "image": 0}) },
		want: `{"bk":[{"k":"thinking","n":1},{"k":"text","n":2},{"k":"image"}]}`,
	},
	{
		field: "windows",
		set: func(r *lineRecord) {
			r.windows = claude.KeptWindows(map[string]string{"5h-utilization": "0.42", "5h-reset": "1791378000", "5h-status": "allowed", "7d-utilization": "0"})
		},
		want: `{"w":[{"k":"5h","s":"allowed","u":0.42,"r":1791378000},{"k":"7d"}]}`,
	},
	{field: "limit", set: func(r *lineRecord) { r.limit = quota.StatusRejected }, want: `{"l":"rejected"}`},
}

// Each fact a record holds is stored by its own key, as the case of its
// field says, set alone on the zero record, which is stored as nothing at
// all; and restored exactly as it was set. Every field a record holds has
// its case here, so a fact added to it is stored; and every key a row
// declares is a case's, so a row stores nothing but the facts its record
// holds: never a price, nor what a story tells.
func TestEachFactOfARecordIsStoredByItsOwnKey(t *testing.T) {
	var fields, keys []string
	for _, c := range fieldsStored {
		var stored map[string]json.RawMessage
		if err := json.Unmarshal([]byte(c.want), &stored); err != nil {
			t.Fatal(err)
		}
		fields, keys = append(fields, c.field), append(keys, slices.Collect(maps.Keys(stored))...)
	}
	if got, want := slices.Compact(slices.Sorted(slices.Values(fields))), slices.Sorted(slices.Values(fieldNames(reflect.TypeFor[lineRecord]()))); !slices.Equal(got, want) {
		t.Fatalf("the cases store the fields %v, want every field a record holds, %v", got, want)
	}
	if got, want := slices.Compact(slices.Sorted(slices.Values(keys))), slices.Sorted(slices.Values(jsonKeys(reflect.TypeFor[storedRow]()))); !slices.Equal(got, want) {
		t.Fatalf("the cases store by the keys %v, want every key a row declares, each once, %v", got, want)
	}
	cases := append([]fieldStored{{field: "none", set: func(*lineRecord) {}, want: `{}`}}, fieldsStored...)
	for _, c := range cases {
		var r lineRecord
		c.set(&r)
		row, err := json.Marshal(r.stored())
		if err != nil {
			t.Fatalf("%s: storing it: %v", c.field, err)
		}
		if string(row) != c.want {
			t.Errorf("%s is stored as %s, want %s", c.field, row, c.want)
		}
		if restored, err := restoredRows(nil, append(row, '\n')); err != nil || len(restored) != 1 || !reflect.DeepEqual(restored[0], r) {
			t.Errorf("%s, stored as %s, is restored as %+v, %v, want\n%+v", c.field, row, restored, err, r)
		}
	}
}

// Each kind of what a request asked is stored by its own code, as
// storedKinds has it, and restored as that kind; a code none is stored by,
// as a row of a later release may give, is refused, as is a line that isn't
// a row: so no row is restored as another kind than it was stored.
func TestEachKindIsStoredByItsOwnCode(t *testing.T) {
	codes := make(map[string]RowKind)
	for _, k := range RowKinds() {
		code := storedKinds[k]
		if other, ok := codes[code]; ok {
			t.Errorf("kinds %d and %d are both stored as %q, want each by its own code", other, k, code)
		}
		codes[code] = k
		r := lineRecord{kind: k}
		row, err := json.Marshal(r.stored())
		if err != nil {
			t.Fatal(err)
		}
		if restored, err := restoredRows(nil, append(row, '\n')); err != nil || len(restored) != 1 || restored[0].kind != k {
			t.Errorf("kind %d, stored as %s, is restored as %+v, %v, want of its kind", k, row, restored, err)
		}
	}
	for _, rows := range []string{`{"k":"batch"}` + "\n", `{"k":1}` + "\n", "not a row\n", `{"r":"r1"` + "\n"} {
		if restored, err := restoredRows(nil, []byte(rows)); err == nil {
			t.Errorf("%q is restored as %+v, want it refused", rows, restored)
		}
	}
}

// A chunk's rows are the stored rows of the records it took, a JSON line
// each, in the order it took them, each ending in a newline, and it counts
// them; and they're restored after the records already restored, as a day's
// chunks are, one after another.
func TestAChunksRowsAreItsRecordsStoredRowsALineEach(t *testing.T) {
	records := recordsOfLines(goldenRows())
	built, _ := encodedChunk(t, records)
	var want bytes.Buffer
	for i := range records {
		row, err := json.Marshal(records[i].stored())
		if err != nil {
			t.Fatal(err)
		}
		want.Write(append(row, '\n'))
	}
	if !bytes.Equal(built.rows, want.Bytes()) {
		t.Errorf("the chunk's rows are\n%s\nwant\n%s", built.rows, want.Bytes())
	}
	if built.count != len(records) {
		t.Errorf("the chunk counts %d rows, want %d", built.count, len(records))
	}
	half := len(records) / 2
	first, _ := encodedChunk(t, records[:half])
	second, _ := encodedChunk(t, records[half:])
	restored, err := restoredRows(nil, first.rows)
	if err == nil {
		restored, err = restoredRows(restored, second.rows)
	}
	if err != nil || !reflect.DeepEqual(restored, records) {
		t.Errorf("two chunks, one after the other, are restored as\n%+v, %v\nwant\n%+v", restored, err, records)
	}
}

// A chunk's meta counts its rows as the Log counts its day's lines whatever
// its filter, as LogCount has it: summed over the chunks a day's lines are
// cut into, of whatever size, it's the count the Log's table gives of the
// day, but those its filter shows. And it gives when the first and the last
// of its rows arrived, whatever order it took them in.
func TestAChunksMetaCountsItsRowsAsTheLogCountsTheirDay(t *testing.T) {
	for seed := range uint64(20) {
		lines, _ := pagedRun(seed)
		r := rand.New(rand.NewPCG(seed, 14))
		for i := range lines {
			dressed(r, &lines[i])
			oddly(r, &lines[i])
		}
		now := ended(&lines[len(lines)-1]).Add(time.Minute)
		read := (&logGrowing{lines: lines, prices: changingPrices()}).readAtOnce(now)
		table := LogRows(LogSources{From: lines[arrivalOrder(lines)[0]].At, Read: read, Now: now})
		byDay := make(map[string][]lineRecord)
		for i := range lines {
			date := dayfile.DateOf(lines[i].At)
			byDay[date] = append(byDay[date], lineRecordOf(&lines[i]))
		}
		size := 1 + r.IntN(20)
		for _, date := range slices.Sorted(maps.Keys(byDay)) {
			records := byDay[date]
			r.Shuffle(len(records), func(i, j int) { records[i], records[j] = records[j], records[i] })
			var counted LogCount
			for chunk := range slices.Chunk(records, size) {
				_, meta := encodedChunk(t, chunk)
				counted = summedCounts(counted, meta.lines)
				first, last := arrivalsOf(chunk)
				if meta.first != first || meta.last != last {
					t.Errorf("run %d, %s: a chunk's arrivals are %v to %v, want %v to %v", seed, date, meta.first.time(), meta.last.time(), first.time(), last.time())
				}
			}
			want := table.Count(date)
			want.Shown = 0
			if !reflect.DeepEqual(counted, want) {
				t.Errorf("run %d, %s, chunks of %d: its chunks count\n%+v\nwant the Log's count\n%+v", seed, date, size, counted, want)
			}
		}
	}
}

// summedCounts is what a and b count together, as their day's lines count
// whatever the filter.
func summedCounts(a, b LogCount) LogCount {
	return LogCount{
		Requests: a.Requests + b.Requests, Checks: a.Checks + b.Checks, Counts: a.Counts + b.Counts, Failed: a.Failed + b.Failed,
		Models: summedTallies(a.Models, b.Models), Accounts: summedTallies(a.Accounts, b.Accounts), Sessions: summedTallies(a.Sessions, b.Sessions),
	}
}

// summedTallies are what a and b tally together, by id: none where neither
// tallies any.
func summedTallies(a, b map[string]int) map[string]int {
	var sum map[string]int
	for _, tallied := range []map[string]int{a, b} {
		for id, n := range tallied {
			if sum == nil {
				sum = make(map[string]int)
			}
			sum[id] += n
		}
	}
	return sum
}

// arrivalsOf are when the first and the last of records arrived.
func arrivalsOf(records []lineRecord) (first, last instant) {
	first, last = records[0].at, records[0].at
	for i := range records {
		if records[i].at.compare(first) < 0 {
			first = records[i].at
		}
		if records[i].at.compare(last) > 0 {
			last = records[i].at
		}
	}
	return first, last
}

// jsonKeys are the keys of the JSON of the struct type t, as encoding/json
// gives them, those of the structs it embeds untagged in place of them.
func jsonKeys(t reflect.Type) []string {
	var keys []string
	for f := range t.Fields() {
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if f.Anonymous && key == "" {
			keys = append(keys, jsonKeys(f.Type)...)
			continue
		}
		keys = append(keys, key)
	}
	return keys
}
