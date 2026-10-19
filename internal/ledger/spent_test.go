package ledger_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

// spentSeeds are how many generated sets of requests the Spent's properties
// hold over.
const spentSeeds = 150

// spentModels are the models the Spent's generated requests ask for, each in
// the geo given: one the table prices, in no geo, in the US and in a geo it
// doesn't price it in; one whose price changes on a day spentDates gives;
// one in the US, which doesn't offer it; one that launched on a day
// spentDates gives; one the table doesn't know, unless the config prices it,
// in no geo and in the US; one a table of its own prices, whose prices
// change; and none.
var spentModels = []struct{ model, geo string }{
	{model: opus}, {model: opus, geo: "us"}, {model: opus, geo: "eu"},
	{model: "claude-sonnet-5-5"}, {model: haiku, geo: "us"}, {model: "claude-haiku-5-5"},
	{model: "claude-opus-9"}, {model: "claude-opus-9", geo: "us"},
	{model: "claude-test-1"}, {model: "claude-test-1", geo: "us"},
	{model: ""},
}

// spentDates are the dates of the days the Spent's tests price on: before
// most models launched, so each is priced as it launched; the day the
// changing table's model fell in price; the day Claude Sonnet 5.5's cache
// reads did and Claude Haiku 5.5 launched; and a day after.
var spentDates = []string{"2026-09-01", "2026-10-06", "2026-10-07", "2026-10-12"}

// spentTable is a table the Spent's tests price by, by what it is.
type spentTable struct {
	name  string
	table ledger.Table
}

// spentTables are the tables the Spent's tests price by: the one switchboard
// carries; it with the config's price of a token of a model it knows, and
// every price of one it doesn't, which it then prices in no geo but the
// global; a table of a model whose prices changed; and a table that prices
// nothing.
func spentTables(t *testing.T) []spentTable {
	t.Helper()
	unknown := config.ModelPrices{Input: new(6.0), Output: new(30.0), CacheRead: new(0.6), CacheWrite5m: new(7.5), CacheWrite1h: new(12.0)}
	return []spentTable{
		{name: "the table switchboard carries", table: ledger.Pricing},
		{name: "the config's prices", table: overridden(t, ledger.Pricing, config.Prices{Models: map[string]config.ModelPrices{opus: {Input: new(3.3)}, "claude-opus-9": unknown}})},
		{name: "a table whose prices changed", table: changing},
		{name: "a table that prices nothing", table: ledger.Table{}},
	}
}

// spentLines are the lines of seed's requests: as many as forty, each of
// one of spendKinds, asking for one of spentModels, its usage as spentUsage
// draws it.
func spentLines(seed uint64) []ledger.Line {
	r := rand.New(rand.NewPCG(seed, seed))
	lines := make([]ledger.Line, r.IntN(41))
	for i := range lines {
		m := spentModels[r.IntN(len(spentModels))]
		lines[i] = spentLine(m.model, m.geo, spentUsage(r), spendKinds[r.IntN(len(spendKinds))].edit)
	}
	return lines
}

// spentUsage is a usage r draws: now and then one of spendUsages, of every
// shape a line could hold; else some of each count an answer gives, the
// cache's writes broken down by how long they last or not, web searches and
// fetches, a server tool the prices don't know, and thinking, each as
// spentCount draws it.
func spentUsage(r *rand.Rand) string {
	if r.IntN(5) == 0 {
		return spendUsages[r.IntN(len(spendUsages))].usage
	}
	drawn := func(names ...string) map[string]any {
		counts := make(map[string]any)
		for _, name := range names {
			if r.IntN(2) == 0 {
				counts[name] = spentCount(r)
			}
		}
		return counts
	}
	usage := drawn("input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens")
	for _, within := range []struct {
		name  string
		names []string
	}{
		{name: "cache_creation", names: []string{"ephemeral_5m_input_tokens", "ephemeral_1h_input_tokens"}},
		{name: "server_tool_use", names: []string{"web_search_requests", "web_fetch_requests", "code_execution_requests"}},
		{name: "output_tokens_details", names: []string{"thinking_tokens"}},
	} {
		if counts := drawn(within.names...); len(counts) > 0 {
			usage[within.name] = counts
		}
	}
	// A map of counts always marshals.
	b, _ := json.Marshal(usage)
	return string(b)
}

// spentCount is a count r draws: mostly of a size an answer gives; now and
// then none; less than none, as a file edited by hand could hold; or so
// near the most an int64 holds that two summed, or one priced, go past it.
func spentCount(r *rand.Rand) int64 {
	switch r.IntN(10) {
	case 0:
		return 0
	case 1:
		return -r.Int64N(1_000)
	case 2:
		return math.MaxInt64 - r.Int64N(1_000)
	}
	return r.Int64N(200_000)
}

// spentLine is a request's line of the kind edit makes it, asking for the
// model given in the geo given, its usage the one given, none for "".
func spentLine(model, geo, usage string, edit func(l *ledger.Line)) ledger.Line {
	l := spendLine(usage, edit)
	l.Model, l.Shape.InferenceGeo = model, geo
	return l
}

// The kinds of request the Spent's tests make of a line: answered; a quota
// check; cut off, which went upstream without usage; and answered by the
// router itself, which never went upstream.
func answered(*ledger.Line)  {}
func checked(l *ledger.Line) { l.Kind = ledger.KindCheck }
func cutOff(l *ledger.Line)  { l.CutOff, l.Status = true, 0 }
func neverUp(l *ledger.Line) { l.Attempts, l.Status = 0, 429 }

// spentOfLines is the Spent of the lines' requests, each taken in as Add
// takes it.
func spentOfLines(lines []ledger.Line) ledger.Spent {
	var spent ledger.Spent
	for i := range lines {
		spent.Add(lines[i].Model, lines[i].Shape.InferenceGeo, ledger.SpendOf(&lines[i]))
	}
	return spent
}

// requestsSummed is what a sum of requests' worth, as the views sum it,
// makes of the lines' requests, each priced on its own as Summed prices its
// line, at table's prices on the day with the given date: the cost of those
// priced, summed as an int64 sums them; whether any was; and each name any
// leaves unpriced, once, in order.
func requestsSummed(table ledger.Table, lines []ledger.Line, date string) (ledger.Worth, bool) {
	var sum ledger.Worth
	priced := false
	for i := range lines {
		w, ok := table.Summed(&lines[i], date)
		if ok {
			sum.Cost += w.Cost
			priced = true
		}
		for _, name := range w.Unpriced {
			if !slices.Contains(sum.Unpriced, name) {
				sum.Unpriced = append(sum.Unpriced, name)
			}
		}
	}
	slices.Sort(sum.Unpriced)
	return sum, priced
}

// pricedOtherwise are where spent, of the lines' requests, prices otherwise
// than those requests each priced on its own and summed, as requestsSummed
// sums them, by each of tables on each of spentDates: none where it prices
// alike.
func pricedOtherwise(tables []spentTable, spent ledger.Spent, lines []ledger.Line) []string {
	var problems []string
	for _, tt := range tables {
		for _, date := range spentDates {
			got, gotPriced := tt.table.SummedSpent(spent, date)
			if want, wantPriced := requestsSummed(tt.table, lines, date); !reflect.DeepEqual([]any{got, gotPriced}, []any{want, wantPriced}) {
				problems = append(problems, fmt.Sprintf("by %s on %s, SummedSpent(%s) = %+v, priced: %t; want %+v, priced: %t, as its %d requests each priced and summed",
					tt.name, date, spentJSON(spent), got, gotPriced, want, wantPriced, len(lines)))
			}
		}
	}
	return problems
}

// spentJSON is spent's stored form, as a failure shows it.
func spentJSON(spent ledger.Spent) string {
	b, err := json.Marshal(spent)
	if err != nil {
		return fmt.Sprintf("(unwritten: %v)", err)
	}
	return string(b)
}

// A Spent priced is exactly its requests each priced on its own and summed,
// by every table on every day: of every kind of request, of every model and
// geo, priced or not, every usage, counts past what an int64 holds among
// them. So a page that tallies tokens and prices them as it's shown shows
// what pricing each request did.
func TestASpentPricesAsItsRequestsEachPricedAndSummed(t *testing.T) {
	tables := spentTables(t)
	for seed := range uint64(spentSeeds) {
		lines := spentLines(seed)
		for _, problem := range pricedOtherwise(tables, spentOfLines(lines), lines) {
			t.Errorf("seed %d: %s", seed, problem)
		}
	}
}

// A Spent prices each model and geo its requests asked for as they're
// priced each on its own and summed: its counts at their prices, what it
// can't price named once, and priced, at nothing, where it holds a request
// the table prices however little it spent; or, where the table prices
// none, naming why, as wholly does.
func TestASpentPricesEachModelAndGeoAsItsRequestsDo(t *testing.T) {
	opusInput, opusOutput := dollars(4e-6), dollars(20e-6)
	used := `{"input_tokens":10,"output_tokens":20}`
	tests := []struct {
		name   string
		lines  []ledger.Line
		want   ledger.Worth
		priced bool
	}{
		{name: "none"},
		{name: "a request the table prices that spent nothing, priced at nothing", lines: []ledger.Line{spentLine(opus, "", "", neverUp)}, priced: true},
		{name: "a quota check, priced at nothing whatever it spent", lines: []ledger.Line{spentLine(opus, "", used, checked)}, priced: true},
		{name: "a quota check of a model the table doesn't know, none", lines: []ledger.Line{spentLine("claude-opus-9", "", used, checked)}},
		{name: "requests of a model, their counts summed", lines: []ledger.Line{spentLine(opus, "", used, answered), spentLine(opus, "", used, answered)},
			want: ledger.Worth{Cost: 2 * (10*opusInput + 20*opusOutput)}, priced: true},
		{name: "of two models, each at its own prices", lines: []ledger.Line{
			spentLine(opus, "", used, answered), spentLine("claude-sonnet-5-5", "", `{"cache_read_input_tokens":1000000}`, answered),
		}, want: ledger.Worth{Cost: 10*opusInput + 20*opusOutput + dollars(0.10)}, priced: true},
		{name: "in the US, a tenth more", lines: []ledger.Line{spentLine(opus, "us", used, answered)},
			want: ledger.Worth{Cost: (10*opusInput + 20*opusOutput) * 110 / 100}, priced: true},
		{name: "web searches at their price", lines: []ledger.Line{spentLine(opus, "", `{"server_tool_use":{"web_search_requests":2}}`, answered)},
			want: ledger.Worth{Cost: dollars(0.02)}, priced: true},
		{name: "one cut off, the rest priced, naming no_usage", lines: []ledger.Line{spentLine(opus, "", "", cutOff), spentLine(opus, "", used, answered)},
			want: ledger.Worth{Cost: 10*opusInput + 20*opusOutput, Unpriced: []string{"no_usage"}}, priced: true},
		{name: "a count the prices don't know, named once of two requests", lines: []ledger.Line{
			spentLine(opus, "", `{"output_tokens":5,"server_tool_use":{"code_execution_requests":1}}`, answered),
			spentLine(opus, "", `{"output_tokens":5,"server_tool_use":{"code_execution_requests":2}}`, answered),
		}, want: ledger.Worth{Cost: 10 * opusOutput, Unpriced: []string{"server_tool_use.code_execution_requests"}}, priced: true},
		{name: "a model the table doesn't know, by its id", lines: []ledger.Line{
			spentLine("claude-opus-9", "", used, answered), spentLine("claude-opus-9", "", "", neverUp),
		}, want: ledger.Worth{Unpriced: []string{"claude-opus-9"}}},
		{name: "a model the table doesn't know, one of its requests cut off", lines: []ledger.Line{
			spentLine("claude-opus-9", "", "", cutOff), spentLine("claude-opus-9", "", used, answered),
		}, want: ledger.Worth{Unpriced: []string{"claude-opus-9", "no_usage"}}},
		{name: "a model the table doesn't know whose requests spent nothing, none", lines: []ledger.Line{
			spentLine("claude-opus-9", "", `{"input_tokens":0,"server_tool_use":{"web_fetch_requests":1}}`, answered), spentLine("claude-opus-9", "", "", neverUp),
		}},
		{name: "a geo the table doesn't price its model in, by the geo", lines: []ledger.Line{spentLine(haiku, "us", used, answered)},
			want: ledger.Worth{Unpriced: []string{"inference_geo:us"}}},
		{name: "no model, by no_model", lines: []ledger.Line{spentLine("", "", used, answered)}, want: ledger.Worth{Unpriced: []string{"no_model"}}},
		{name: "priced and unpriced together", lines: []ledger.Line{
			spentLine(opus, "", used, answered), spentLine("claude-opus-9", "", used, answered), spentLine(haiku, "us", "", cutOff),
		}, want: ledger.Worth{Cost: 10*opusInput + 20*opusOutput, Unpriced: []string{"claude-opus-9", "inference_geo:us", "no_usage"}}, priced: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spent := spentOfLines(tt.lines)
			if got, priced := ledger.Pricing.SummedSpent(spent, today); !reflect.DeepEqual([]any{got, priced}, []any{tt.want, tt.priced}) {
				t.Errorf("SummedSpent(%s) = %+v, priced: %t; want %+v, priced: %t", spentJSON(spent), got, priced, tt.want, tt.priced)
			}
			if want, priced := requestsSummed(ledger.Pricing, tt.lines, today); !reflect.DeepEqual([]any{want, priced}, []any{tt.want, tt.priced}) {
				t.Errorf("each request priced and summed gives %+v, priced: %t; want %+v, priced: %t, as SummedSpent gives it", want, priced, tt.want, tt.priced)
			}
		})
	}
}

// A Spent priced past what an int64 holds costs exactly what its requests
// each priced and summed do, as int64 arithmetic wraps alike whether a
// group's counts are summed before they're priced or after.
func TestASpentPricedPastWhatAnInt64HoldsIsItsRequestsSummed(t *testing.T) {
	type tokens struct{ input, output int64 }
	opusInput, opusOutput := dollars(4e-6), dollars(20e-6)
	tests := []struct {
		name     string
		requests []tokens
		want     ledger.Picodollars
	}{
		{name: "two requests of the most input an int64 holds", requests: []tokens{{input: math.MaxInt64}, {input: math.MaxInt64}}, want: -8_000_000},
		{name: "counts past it as they're summed, and as they're priced", requests: []tokens{{input: math.MaxInt64, output: math.MaxInt64}, {output: 3}}, want: 36_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lines []ledger.Line
			cost := new(big.Int)
			for _, r := range tt.requests {
				lines = append(lines, spentLine(opus, "", fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d}`, r.input, r.output), answered))
				cost.Add(cost, new(big.Int).Mul(big.NewInt(r.input), big.NewInt(int64(opusInput))))
				cost.Add(cost, new(big.Int).Mul(big.NewInt(r.output), big.NewInt(int64(opusOutput))))
			}
			if cost.IsInt64() {
				t.Fatalf("the requests cost %s, within what an int64 holds; want a case past it", cost)
			}
			spent := spentOfLines(lines)
			if got, priced := ledger.Pricing.SummedSpent(spent, today); got.Cost != tt.want || !priced {
				t.Errorf("SummedSpent(%s) = %+v, priced: %t; want a cost of %d, priced", spentJSON(spent), got, priced, tt.want)
			}
			if summed, _ := requestsSummed(ledger.Pricing, lines, today); summed.Cost != tt.want {
				t.Errorf("each request priced and summed costs %d; want %d, as SummedSpent gives it", summed.Cost, tt.want)
			}
		})
	}
}

// A Spent takes its requests in any order alike: it holds what it would
// had they come in another.
func TestASpentTakesItsRequestsInAnyOrderAlike(t *testing.T) {
	for seed := range uint64(spentSeeds) {
		lines := spentLines(seed)
		shuffled := slices.Clone(lines)
		rand.New(rand.NewPCG(seed, seed+1)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		if got, want := spentOfLines(shuffled), spentOfLines(lines); !reflect.DeepEqual(got, want) {
			t.Errorf("seed %d: its requests taken in another order, the Spent holds %s; want %s", seed, spentJSON(got), spentJSON(want))
		}
	}
}

// A copy of a Spent is its own: adding requests to one, of models it holds
// already and of others, never changes what a copy holds, whenever it was
// copied, as a tally told on afresh copies what it sums.
func TestACopyOfASpentIsItsOwn(t *testing.T) {
	for seed := range uint64(spentSeeds) {
		lines := spentLines(seed)
		var spent ledger.Spent
		copies := make([]ledger.Spent, len(lines))
		for i := range lines {
			copies[i] = spent
			spent.Add(lines[i].Model, lines[i].Shape.InferenceGeo, ledger.SpendOf(&lines[i]))
		}
		for i, copied := range copies {
			if want := spentOfLines(lines[:i]); !reflect.DeepEqual(copied, want) {
				t.Errorf("seed %d: copied after %d requests, then %d more taken in by what it was copied of, it holds %s; want %s, as of the %d",
					seed, i, len(lines)-i, spentJSON(copied), spentJSON(want), i)
			}
		}
	}
}
