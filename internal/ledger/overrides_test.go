package ledger_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

// changing is a table of a model whose prices changed the day before today,
// and of a plan.
var changing = ledger.Table{AsOf: today, Models: []ledger.Model{{IDs: []string{"claude-test-1-20260901", "claude-test-1"}, Prices: []ledger.Prices{
	{From: "2026-09-01", Input: dollars(4e-6), Output: dollars(20e-6), CacheRead: dollars(0.4e-6), CacheWrite5m: dollars(5e-6), CacheWrite1h: dollars(8e-6),
		WebSearch: dollars(0.01), USOnlyPercent: 110},
	{From: "2026-10-06", Input: dollars(3e-6), Output: dollars(15e-6), CacheRead: dollars(0.3e-6), CacheWrite5m: dollars(3.75e-6), CacheWrite1h: dollars(6e-6),
		WebSearch: dollars(0.01), USOnlyPercent: 110},
}}}, Plans: []ledger.Plan{{ID: "max5x", Name: "Max 5x", Size: 5, Prices: []ledger.PlanPrice{{From: "2025-04-09", Monthly: dollars(100)}}}}}

// everyKind is a usage of a million tokens of each kind, and a thousand web
// searches.
const everyKind = `{"input_tokens":1000000,"output_tokens":1000000,"cache_read_input_tokens":1000000,"cache_creation_input_tokens":2000000,` +
	`"cache_creation":{"ephemeral_5m_input_tokens":1000000,"ephemeral_1h_input_tokens":1000000},"server_tool_use":{"web_search_requests":1000}}`

// overridden is table with the config's prices in place of its own, failing
// the test where it passes over any of them.
func overridden(t *testing.T, table ledger.Table, prices config.Prices) ledger.Table {
	t.Helper()
	table, passed := table.With(prices)
	if len(passed) > 0 {
		t.Fatalf("With() passed over %q, want every price the config gives taken", passed)
	}
	return table
}

func TestTheConfigsPricesOfAModelPriceEveryDayAlike(t *testing.T) {
	table := overridden(t, changing, config.Prices{Models: map[string]config.ModelPrices{"claude-test-1": {Input: new(3.2), CacheWrite1h: new(6.4)}}})
	tests := []struct {
		name  string
		model string
		date  string
		geo   string
		want  ledger.Picodollars
	}{
		{name: "before the change, the rest at the table's", model: "claude-test-1", date: date, want: dollars(3.2 + 20 + 0.4 + 5 + 6.4 + 10)},
		{name: "after it, the rest at the table's", model: "claude-test-1", date: today, want: dollars(3.2 + 15 + 0.3 + 3.75 + 6.4 + 10)},
		{name: "by the model's other id", model: "claude-test-1-20260901", date: today, want: dollars(3.2 + 15 + 0.3 + 3.75 + 6.4 + 10)},
		{name: "in the US, a tenth more on each token", model: "claude-test-1", geo: "us", date: today, want: dollars((3.2+15+0.3+3.75+6.4)*1.1 + 10)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := worthOf(t, table, tt.model, tt.geo, everyKind, tt.date)
			if got.Cost != tt.want || len(got.Unpriced) > 0 {
				t.Errorf("Worth() = %+v, want %s, all of it priced", got, tt.want.Cents())
			}
		})
	}
	if !table.Overridden {
		t.Error("the table isn't overridden, want it to be, as the config prices a model")
	}
}

func TestTheConfigsPricesOfAModelAreConvertedExactly(t *testing.T) {
	table := overridden(t, ledger.Pricing, config.Prices{Models: map[string]config.ModelPrices{"claude-opus-5-5": {Input: new(3.2), Output: new(16.0), CacheRead: new(0.16)}}})
	got := worthOf(t, table, "claude-opus-5-5", "", `{"input_tokens":3,"output_tokens":7,"cache_read_input_tokens":11}`, today)
	if want := ledger.Picodollars(3*3_200_000 + 7*16_000_000 + 11*160_000); got.Cost != want {
		t.Errorf("Worth() = %d picodollars, want %d, each token priced whole", got.Cost, want)
	}
}

func TestAModelTheTableDoesntKnowIsPricedOnlyWhereTheConfigGivesEveryPrice(t *testing.T) {
	every := config.ModelPrices{Input: new(1.0), Output: new(5.0), CacheRead: new(0.1), CacheWrite5m: new(1.25), CacheWrite1h: new(2.0)}
	some := every
	some.CacheWrite1h = nil
	table, passed := ledger.Pricing.With(config.Prices{Models: map[string]config.ModelPrices{
		"claude-test-2": every, "claude-test-3": some, "claude-test-4": {}, "claude-opus-5-5": {},
	}})
	if want := []string{"claude-test-3", "claude-test-4"}; !slices.Equal(passed, want) {
		t.Errorf("With() passed over %q, want %q, the models it doesn't know whose every price the config doesn't give", passed, want)
	}

	got := worthOf(t, table, "claude-test-2", "", everyKind, today)
	if want := dollars(1 + 5 + 0.1 + 1.25 + 2 + 10); got.Cost != want || len(got.Unpriced) > 0 {
		t.Errorf("Worth() of the model given every price = %+v, want %s, its web searches at the table's price", got, want.Cents())
	}
	if got, ok := table.Worth("claude-test-2", "us", json.RawMessage(everyKind), today); ok {
		t.Errorf("Worth() of the model given every price, in the US = %+v, want it unpriced", got)
	}
	if got, ok := table.Worth("claude-test-3", "", json.RawMessage(everyKind), today); ok {
		t.Errorf("Worth() of the model given some prices = %+v, want it unpriced", got)
	}
	if !table.Overridden {
		t.Error("the table isn't overridden, want it to be, as the config prices a model")
	}
}

func TestTheConfigsPriceOfAPlanPricesEveryDayAlike(t *testing.T) {
	table := overridden(t, changing, config.Prices{Plans: map[string]float64{"max5x": 90.5}})
	plan, ok := table.Plan("max5x")
	if !ok {
		t.Fatal("Plan(max5x) isn't priced")
	}
	for _, day := range []string{"2020-01-01", "2025-04-09", today} {
		if got := plan.Price(day); got != dollars(90.5) {
			t.Errorf("Price(%s) = %s, want $90.50, the config's", day, got.Cents())
		}
	}
	if plan.Name != "Max 5x" || plan.Size != 5 || !table.Overridden {
		t.Errorf("Plan(max5x) = %+v, overridden: %v; want Max 5x, of 5 Pros, and the table overridden", plan, table.Overridden)
	}
}

func TestATableIsOverriddenOnlyWhereTheConfigsPricesStandInPlaceOfItsOwn(t *testing.T) {
	tests := []struct {
		name   string
		prices config.Prices
		want   bool
		passed []string
	}{
		{name: "none"},
		{name: "a model's table, empty", prices: config.Prices{Models: map[string]config.ModelPrices{"claude-test-1": {}}}},
		{name: "some prices of a model the table doesn't know", prices: config.Prices{Models: map[string]config.ModelPrices{"claude-test-9": {Input: new(1.0)}}},
			passed: []string{"claude-test-9"}},
		{name: "a price of a model it knows", prices: config.Prices{Models: map[string]config.ModelPrices{"claude-test-1": {Output: new(0.0)}}}, want: true},
		{name: "a plan's", prices: config.Prices{Plans: map[string]float64{"max5x": 100}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, passed := changing.With(tt.prices); got.Overridden != tt.want || !slices.Equal(passed, tt.passed) {
				t.Errorf("With() is overridden: %v, passing over %q; want %v, passing over %q", got.Overridden, passed, tt.want, tt.passed)
			}
		})
	}
}

func TestTheConfigsPricesLeaveTheTableTheyreGivenAsItIs(t *testing.T) {
	before := ledger.Table{AsOf: changing.AsOf, Models: slices.Clone(changing.Models), Plans: slices.Clone(changing.Plans)}
	for i := range before.Models {
		before.Models[i].Prices = slices.Clone(before.Models[i].Prices)
	}
	overridden(t, changing, config.Prices{
		Plans:  map[string]float64{"max5x": 1},
		Models: map[string]config.ModelPrices{"claude-test-1": {Input: new(1.0)}, "claude-test-2": {Input: new(1.0), Output: new(1.0), CacheRead: new(1.0), CacheWrite5m: new(1.0), CacheWrite1h: new(1.0)}},
	})
	if !reflect.DeepEqual(changing, before) {
		t.Errorf("once priced by the config, the table is\n%+v\nwant it as it was\n%+v", changing, before)
	}
}
