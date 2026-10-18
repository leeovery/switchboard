package ledger_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// spendUsages are the usages the Spend's tests decode, of every shape an
// answer gives, and of shapes it never gives, which a line read from a file
// could hold all the same.
var spendUsages = []struct{ name, usage string }{
	{name: "absent", usage: ""},
	{name: "null", usage: `null`},
	{name: "empty", usage: `{}`},
	{name: "writes for an hour", usage: `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":1000},"output_tokens":500}`},
	{name: "writes for five minutes", usage: `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":1000,"ephemeral_1h_input_tokens":0},"output_tokens":500}`},
	{name: "writes for each", usage: `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":120,"ephemeral_1h_input_tokens":3000}}`},
	{name: "writes not broken down", usage: `{"input_tokens":12,"cache_creation_input_tokens":3120}`},
	{name: "writes broken down in part", usage: `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`},
	{name: "writes of a last the prices don't know", usage: `{"cache_creation_input_tokens":3120,` +
		`"cache_creation":{"ephemeral_1h_input_tokens":3000,"ephemeral_1d_input_tokens":120}}`},
	{name: "reads alone", usage: `{"cache_read_input_tokens":3120,"cache_creation_input_tokens":0}`},
	{name: "web searches and fetches", usage: `{"input_tokens":12,"server_tool_use":{"web_search_requests":2,"web_fetch_requests":1}}`},
	{name: "a server tool the prices don't know", usage: `{"output_tokens":5,"server_tool_use":{"code_execution_requests":1}}`},
	{name: "thinking and the model's own turns", usage: `{"output_tokens":845,"output_tokens_details":{"thinking_tokens":600},` +
		`"iterations":[{"type":"message","input_tokens":5},{"type":"compaction","input_tokens":7}]}`},
	{name: "the service tier and the geo, which aren't counts", usage: `{"output_tokens":845,"service_tier":"standard","inference_geo":"us"}`},
	{name: "nothing that costs anything", usage: `{"input_tokens":0,"server_tool_use":{"web_fetch_requests":1}}`},
	{name: "a count given as a fraction", usage: `{"input_tokens":1.5,"output_tokens":10}`},
	{name: "a count given as text", usage: `{"input_tokens":"12","output_tokens":10}`},
	{name: "a count's name in capitals", usage: `{"Input_Tokens":12,"output_tokens":10}`},
	{name: "a count past what a token count holds", usage: `{"input_tokens":99999999999999999999}`},
	{name: "writes named beside cache_creation", usage: `{"cache_creation_input_tokens":100,"cache_creation.ephemeral_5m_input_tokens":40,` +
		`"cache_creation":{"ephemeral_1h_input_tokens":60},"cache_creation.unknown":3}`},
	{name: "writes within writes", usage: `{"cache_creation":{"ephemeral_5m_input_tokens":{"x":3}},"cache_creation_input_tokens":{"y":2}}`},
	{name: "cache_creation a count", usage: `{"cache_creation":5,"cache_creation_input_tokens":5}`},
	{name: "a list", usage: `[1,2]`},
	{name: "text", usage: `"ok"`},
	{name: "a number", usage: `5`},
	{name: "more after the usage", usage: `{"input_tokens":5} {"output_tokens":7}`},
	{name: "cut short", usage: `{"input_tokens":5,"output`},
}

// spendModels are the models the Spend's tests price a usage of, each in
// the geo given: one the table prices; one whose price changes on the day
// spendDates gives after the first; one in the US; one in a geo the table
// doesn't price it in; one the table doesn't know; and none.
var spendModels = []struct{ model, geo string }{
	{model: opus}, {model: "claude-sonnet-5-5"}, {model: opus, geo: "us"}, {model: haiku, geo: "us"},
	{model: "claude-opus-9"}, {model: ""},
}

// spendDates are the dates of the days the Spend's tests price on: the day
// before Claude Sonnet 5.5's cache reads fell in price, and the day they did.
var spendDates = []string{"2026-10-06", "2026-10-07"}

// spendKinds are the lines the Spend's tests give a usage on: a request
// answered; one cut off, which went upstream without usage where it gives
// none; one the router answered itself; a quota check; a count of tokens;
// and one of a kind a later router adds.
var spendKinds = []struct {
	name string
	edit func(l *ledger.Line)
}{
	{name: "answered", edit: func(*ledger.Line) {}},
	{name: "cut off", edit: func(l *ledger.Line) { l.CutOff, l.Status = true, 0 }},
	{name: "answered by the router", edit: func(l *ledger.Line) { l.Attempts, l.Status = 0, 429 }},
	{name: "a quota check", edit: func(l *ledger.Line) { l.Kind = ledger.KindCheck }},
	{name: "a count of tokens", edit: func(l *ledger.Line) { l.Kind = ledger.KindCount }},
	{name: "of a later kind", edit: func(l *ledger.Line) { l.Kind = "batch" }},
}

// spendLine is a request's line of the kind given, its usage the one given,
// none for "", as a line read back from its file leaves it out.
func spendLine(usage string, edit func(l *ledger.Line)) ledger.Line {
	l := asked("1", on(0, 9, 0))
	l.Usage = nil
	if usage != "" {
		l.Usage = json.RawMessage(usage)
	}
	edit(&l)
	return l
}

// Every call priced over a line's Spend gives exactly what the line's own
// call gives, every usage on every kind of line, of every model, in every
// geo, on each day: so a view that holds a request's Spend in place of its
// line tells it alike.
func TestASpendGivesWhatItsLineGives(t *testing.T) {
	for _, u := range spendUsages {
		for _, k := range spendKinds {
			t.Run(u.name+", "+k.name, func(t *testing.T) {
				l := spendLine(u.usage, k.edit)
				for _, problem := range spendDisagreements(&l, ledger.SpendOf(&l)) {
					t.Error(problem)
				}
			})
		}
	}
}

// Fuzzed, a Spend gives what its line gives whatever the usage, on each
// kind of line.
func FuzzASpendGivesWhatItsLineGives(f *testing.F) {
	for _, u := range spendUsages {
		for i := range spendKinds {
			f.Add([]byte(u.usage), uint8(i))
		}
	}
	f.Fuzz(func(t *testing.T, usage []byte, kind uint8) {
		l := spendLine(string(usage), spendKinds[int(kind)%len(spendKinds)].edit)
		for _, problem := range spendDisagreements(&l, ledger.SpendOf(&l)) {
			t.Error(problem)
		}
	})
}

// spendDisagreements are where spend, the line l's, gives otherwise than the
// line's own calls, priced at ledger.Pricing, of each of spendModels, on
// each of spendDates: none where it agrees.
func spendDisagreements(l *ledger.Line, spend ledger.Spend) []string {
	var problems []string
	differs := func(call string, got, want any) {
		if !reflect.DeepEqual(got, want) {
			problems = append(problems, fmt.Sprintf("of %q, %s gives %#v, want %#v, as the line's", l.Usage, call, got, want))
		}
	}
	tokens, metered := l.Tokens()
	got, gotMetered := spend.Tokens()
	differs("Tokens", []any{got, gotMetered}, []any{tokens, metered})
	lasts, wrote := l.CacheLife()
	gotLasts, gotWrote := spend.CacheLife()
	differs("CacheLife", []any{gotLasts, gotWrote}, []any{lasts, wrote})
	for _, m := range spendModels {
		priced := *l
		priced.Model, priced.Shape.InferenceGeo = m.model, m.geo
		for _, date := range spendDates {
			on := fmt.Sprintf(" of %q in %q on %s", m.model, m.geo, date)
			w, ok := ledger.Pricing.Summed(&priced, date)
			gotW, gotOK := ledger.Pricing.SummedSpend(m.model, m.geo, spend, date)
			differs("SummedSpend"+on, []any{gotW, gotOK}, []any{w, ok})
			w, ok = ledger.Pricing.CacheWrite(&priced, date)
			gotW, gotOK = ledger.Pricing.CacheWriteSpend(m.model, m.geo, spend, date)
			differs("CacheWriteSpend"+on, []any{gotW, gotOK}, []any{w, ok})
			for _, life := range []time.Duration{ledger.ShortCache, ledger.LongCache} {
				cost, ok := ledger.Pricing.Rewrite(&priced, life, date)
				gotCost, gotOK := ledger.Pricing.RewriteSpend(m.model, m.geo, spend, life, date)
				differs(fmt.Sprintf("RewriteSpend for %v%s", life, on), []any{gotCost, gotOK}, []any{cost, ok})
			}
		}
	}
	return problems
}

// A model costs alike on two days where the prices in effect on each are the
// same, whenever each took effect, and otherwise not; a model the table
// doesn't know costs alike, as none, on any day.
func TestAModelCostsAlikeOnDaysItsPricesAreTheSame(t *testing.T) {
	restated := ledger.Table{Models: []ledger.Model{{IDs: []string{"claude-test-1"}, Prices: []ledger.Prices{
		{From: "2026-09-01", Output: dollars(20e-6)},
		{From: "2026-10-01", Output: dollars(20e-6)},
		{From: "2026-10-06", Output: dollars(15e-6)},
	}}}}
	tests := []struct {
		name  string
		table ledger.Table
		model string
		a, b  string
		want  bool
	}{
		{name: "the same day", table: ledger.Pricing, model: "claude-sonnet-5-5", a: "2026-10-07", b: "2026-10-07", want: true},
		{name: "two days of the same prices", table: ledger.Pricing, model: "claude-sonnet-5-5", a: "2026-10-07", b: "2026-10-11", want: true},
		{name: "the day before a price fell and the day it did", table: ledger.Pricing, model: "claude-sonnet-5-5", a: "2026-10-06", b: "2026-10-07"},
		{name: "the other way about", table: ledger.Pricing, model: "claude-sonnet-5-5", a: "2026-10-07", b: "2026-10-06"},
		{name: "a model whose prices never changed", table: ledger.Pricing, model: opus, a: "2026-09-22", b: "2026-10-11", want: true},
		{name: "prices given again, unchanged", table: restated, model: "claude-test-1", a: "2026-09-15", b: "2026-10-02", want: true},
		{name: "prices given again, then changed", table: restated, model: "claude-test-1", a: "2026-10-02", b: "2026-10-06"},
		{name: "a model the table doesn't know", table: ledger.Pricing, model: "claude-opus-9", a: "2026-10-06", b: "2026-10-07", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.table.SamePrices(tt.model, tt.a, tt.b); got != tt.want {
				t.Errorf("SamePrices(%s, %s, %s) = %t, want %t", tt.model, tt.a, tt.b, got, tt.want)
			}
			usage := json.RawMessage(`{"input_tokens":10,"cache_read_input_tokens":1000000,"output_tokens":20}`)
			if a, b := worthOn(tt.table, tt.model, usage, tt.a), worthOn(tt.table, tt.model, usage, tt.b); reflect.DeepEqual(a, b) != tt.want {
				t.Errorf("priced %+v on %s and %+v on %s, want them alike: %t", a, tt.a, b, tt.b, tt.want)
			}
		})
	}
}

// The zero Spend is the spend of a line without usage of a request that
// never went upstream, and of no other: not of one that went and gave no
// usage, whose spend is unknown, nor of a quota check.
func TestTheZeroSpendIsALinesThatNeverWentUpstream(t *testing.T) {
	tests := []struct {
		name string
		line ledger.Line
		zero bool
	}{
		{name: "never upstream", line: ledger.Line{Kind: ledger.KindMessage, Status: 429}, zero: true},
		{name: "upstream, no usage given", line: ledger.Line{Kind: ledger.KindMessage, Status: 200, Attempts: 1}},
		{name: "a quota check", line: ledger.Line{Kind: ledger.KindCheck, Status: 200}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if zero := reflect.DeepEqual(ledger.SpendOf(&tt.line), ledger.Spend{}); zero != tt.zero {
				t.Errorf("SpendOf(%+v) is the zero Spend: %t, want %t", tt.line, zero, tt.zero)
			}
		})
	}
}

// worthOn is what usage, of a request for the model with the given id, is
// worth at table's prices on the day with the given date, priced or not.
func worthOn(table ledger.Table, model string, usage json.RawMessage, date string) []any {
	w, ok := table.Worth(model, "", usage, date)
	return []any{w, ok}
}
