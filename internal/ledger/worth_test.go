package ledger_test

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// today is the date of the day the worth's tests price at.
const today = "2026-10-07"

// dollars is an amount of dollars, as picodollars.
func dollars(d float64) ledger.Picodollars {
	return ledger.Picodollars(math.Round(d * 1e12))
}

// worthOf returns what usage, of a request for model that asked for the geo
// given, is worth at table's prices on the day with the given date, failing
// the test where it's unpriced.
func worthOf(t *testing.T, table ledger.Table, model, geo, usage, date string) ledger.Worth {
	t.Helper()
	w, ok := table.Worth(model, geo, json.RawMessage(usage), date)
	if !ok {
		t.Fatalf("Worth(%s, %q, %s, %s) is unpriced, want it priced", model, geo, usage, date)
	}
	return w
}

func TestEachKindOfCountIsWorthItsPrice(t *testing.T) {
	tests := []struct {
		name  string
		usage string
		want  ledger.Picodollars
	}{
		{name: "input", usage: `{"input_tokens":12}`, want: dollars(12 * 4e-6)},
		{name: "writes to the cache for five minutes", usage: `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":3120}}`,
			want: dollars(3120 * 5e-6)},
		{name: "writes to the cache for an hour", usage: `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_1h_input_tokens":3120}}`,
			want: dollars(3120 * 8e-6)},
		{name: "reads from the cache", usage: `{"cache_read_input_tokens":182340}`, want: dollars(182340 * 0.2e-6)},
		{name: "output", usage: `{"output_tokens":845}`, want: dollars(845 * 20e-6)},
		{name: "web searches, beyond their tokens", usage: `{"server_tool_use":{"web_search_requests":3}}`, want: dollars(0.03)},
		{name: "web fetches, which cost nothing beyond their tokens", usage: `{"server_tool_use":{"web_fetch_requests":2}}`},
		{name: "the thinking within the output, which costs nothing more", usage: `{"output_tokens":845,"output_tokens_details":{"thinking_tokens":600}}`,
			want: dollars(845 * 20e-6)},
		{name: "the model's own turns, which its usage sums", usage: `{"input_tokens":12,"iterations":[{"type":"message","input_tokens":5},{"type":"message","input_tokens":7}]}`,
			want: dollars(12 * 4e-6)},
		{name: "what isn't a count, as the service tier", usage: `{"output_tokens":845,"service_tier":"standard","inference_geo":"not_available"}`,
			want: dollars(845 * 20e-6)},
		{name: "every kind at once", usage: `{"input_tokens":12,"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,` +
			`"cache_creation":{"ephemeral_5m_input_tokens":120,"ephemeral_1h_input_tokens":3000},"output_tokens":845,"server_tool_use":{"web_search_requests":1}}`,
			want: dollars(12*4e-6 + 120*5e-6 + 3000*8e-6 + 182340*0.2e-6 + 845*20e-6 + 0.01)},
		{name: "none", usage: ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := worthOf(t, ledger.Pricing, "claude-opus-5-5", "", tt.usage, today)
			if got.Cost != tt.want || len(got.Unpriced) > 0 {
				t.Errorf("on Claude Opus 5.5, %s is worth %+v, want %v, all of it priced", tt.usage, got, tt.want)
			}
		})
	}
}

func TestEachModelIsPricedAsAnthropicsPricingPageGivesIt(t *testing.T) {
	// The prices Anthropic's pricing page gave on 7 October 2026, as dollars a
	// million tokens: of input, writing the cache for five minutes and for an
	// hour, reading it, and output.
	page := []struct {
		ids                                   []string
		input, write5m, write1h, read, output float64
	}{
		{ids: []string{"claude-fable-5-1"}, input: 10, write5m: 12.50, write1h: 20, read: 0.25, output: 50},
		{ids: []string{"claude-mythos-5-1"}, input: 10, write5m: 12.50, write1h: 20, read: 0.25, output: 50},
		{ids: []string{"claude-fable-5"}, input: 10, write5m: 12.50, write1h: 20, read: 1, output: 50},
		{ids: []string{"claude-mythos-5"}, input: 10, write5m: 12.50, write1h: 20, read: 1, output: 50},
		{ids: []string{"claude-opus-5-5"}, input: 4, write5m: 5, write1h: 8, read: 0.20, output: 20},
		{ids: []string{"claude-opus-5"}, input: 5, write5m: 6.25, write1h: 10, read: 0.50, output: 25},
		{ids: []string{"claude-opus-4-8"}, input: 5, write5m: 6.25, write1h: 10, read: 0.50, output: 25},
		{ids: []string{"claude-opus-4-7"}, input: 5, write5m: 6.25, write1h: 10, read: 0.50, output: 25},
		{ids: []string{"claude-opus-4-6"}, input: 5, write5m: 6.25, write1h: 10, read: 0.50, output: 25},
		{ids: []string{"claude-opus-4-5-20251101", "claude-opus-4-5"}, input: 5, write5m: 6.25, write1h: 10, read: 0.50, output: 25},
		{ids: []string{"claude-sonnet-5-5"}, input: 2, write5m: 2.50, write1h: 4, read: 0.20, output: 10},
		{ids: []string{"claude-sonnet-5"}, input: 2, write5m: 2.50, write1h: 4, read: 0.20, output: 10},
		{ids: []string{"claude-sonnet-4-6"}, input: 3, write5m: 3.75, write1h: 6, read: 0.30, output: 15},
		{ids: []string{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5"}, input: 3, write5m: 3.75, write1h: 6, read: 0.30, output: 15},
		{ids: []string{"claude-haiku-4-5-20251001", "claude-haiku-4-5"}, input: 1, write5m: 1.25, write1h: 2, read: 0.10, output: 5},
	}
	// cacheWrite is a usage of a million tokens written to the cache to last
	// as long as ttl says.
	cacheWrite := func(ttl string) string {
		return fmt.Sprintf(`{"cache_creation_input_tokens":1000000,"cache_creation":{"ephemeral_%s_input_tokens":1000000}}`, ttl)
	}
	for _, m := range page {
		for _, id := range m.ids {
			t.Run(id, func(t *testing.T) {
				for _, priced := range []struct {
					usage   string
					dollars float64
				}{
					{usage: `{"input_tokens":1000000}`, dollars: m.input},
					{usage: cacheWrite("5m"), dollars: m.write5m},
					{usage: cacheWrite("1h"), dollars: m.write1h},
					{usage: `{"cache_read_input_tokens":1000000}`, dollars: m.read},
					{usage: `{"output_tokens":1000000}`, dollars: m.output},
					{usage: `{"server_tool_use":{"web_search_requests":1000}}`, dollars: 10},
				} {
					if got := worthOf(t, ledger.Pricing, id, "", priced.usage, today); got.Cost != dollars(priced.dollars) {
						t.Errorf("%s is worth %s, want $%v, as the pricing page gives it", priced.usage, got.Cost.Cents(), priced.dollars)
					}
				}
			})
		}
	}
	if got := len(ledger.Pricing.Models); got != len(page) {
		t.Errorf("the table prices %d models, want the %d on the pricing page the ledger can hold", got, len(page))
	}
}

func TestARequestIsPricedAtTheDayAskedFor(t *testing.T) {
	// A model whose output price fell the day after the request.
	changed := ledger.Table{AsOf: today, Models: []ledger.Model{{IDs: []string{"claude-test-1"}, Prices: []ledger.Prices{
		{From: "2026-09-01", Output: dollars(20e-6)},
		{From: "2026-10-06", Output: dollars(15e-6)},
	}}}}
	line := ledger.Line{At: on(0, 9, 0), Model: "claude-test-1", Usage: usage(`{"output_tokens":1000}`)}
	tests := []struct {
		name string
		date string
		want ledger.Picodollars
	}{
		{name: "its own day, before the change", date: date, want: dollars(1000 * 20e-6)},
		{name: "today's, since the change", date: today, want: dollars(1000 * 15e-6)},
		{name: "the day the change took effect", date: "2026-10-06", want: dollars(1000 * 15e-6)},
		{name: "a day before the model's first prices, at those", date: "2026-08-01", want: dollars(1000 * 20e-6)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := changed.Request(&line, tt.date); !ok || got.Cost != tt.want {
				t.Errorf("Request() at %s = %+v, %v, want %v", tt.date, got, ok, tt.want)
			}
		})
	}
}

func TestUSOnlyInferenceCostsATenthMoreOnTheModelsThatOfferIt(t *testing.T) {
	tokens := `{"input_tokens":1000,"cache_creation_input_tokens":2000,"cache_creation":{"ephemeral_5m_input_tokens":1000,"ephemeral_1h_input_tokens":1000},` +
		`"cache_read_input_tokens":10000,"output_tokens":500,"server_tool_use":{"web_search_requests":2}}`
	// Claude Opus 5.5's prices of these tokens, and of the web searches.
	opusTokens, searches := 1000*4e-6+1000*5e-6+1000*8e-6+10000*0.2e-6+500*20e-6, 0.02
	tests := []struct {
		name   string
		model  string
		geo    string
		want   ledger.Picodollars
		priced bool
	}{
		{name: "asked for nowhere", model: "claude-opus-5-5", want: dollars(opusTokens + searches), priced: true},
		{name: "asked for globally", model: "claude-opus-5-5", geo: "global", want: dollars(opusTokens + searches), priced: true},
		{name: "asked for in the US, each token a tenth more, the searches as they were", model: "claude-opus-5-5", geo: "us",
			want: dollars(opusTokens*1.1 + searches), priced: true},
		{name: "asked for in the US on a model that doesn't offer it", model: "claude-haiku-4-5", geo: "us"},
		{name: "asked for in a geo the table doesn't know", model: "claude-opus-5-5", geo: "eu"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ledger.Pricing.Worth(tt.model, tt.geo, json.RawMessage(tokens), today)
			if ok != tt.priced || got.Cost != tt.want {
				t.Errorf("Worth() = %+v, %v, want %v, %v", got, ok, tt.want, tt.priced)
			}
		})
	}
}

func TestALongRequestIsPricedAtTheStandardRates(t *testing.T) {
	// 900,000 tokens of input, asking for the 1M-token context window's beta,
	// which charged more on Claude Sonnet 4.5 until it was retired, before the
	// ledger began recording.
	for _, model := range []string{"claude-sonnet-4-5", "claude-sonnet-4-6", "claude-opus-5-5"} {
		t.Run(model, func(t *testing.T) {
			long := ledger.Line{At: on(0, 9, 0), Model: model, Betas: []string{"context-1m-2025-08-07"},
				Usage: usage(`{"input_tokens":900000,"output_tokens":1000}`)}
			short := ledger.Line{At: on(0, 9, 0), Model: model, Usage: usage(`{"input_tokens":9000,"output_tokens":10}`)}
			longWorth, longOK := ledger.Pricing.Request(&long, today)
			shortWorth, shortOK := ledger.Pricing.Request(&short, today)
			if !longOK || !shortOK || longWorth.Cost != 100*shortWorth.Cost {
				t.Errorf("the long request is worth %v, the short %v, want the long a hundred times the short, at the same rates", longWorth.Cost, shortWorth.Cost)
			}
		})
	}
}

func TestAModelTheTableDoesntKnowIsUnpricedNeverFree(t *testing.T) {
	for _, model := range []string{"claude-opus-9", "", "claude-opus-5-5-20270101"} {
		if got, ok := ledger.Pricing.Worth(model, "", json.RawMessage(`{"input_tokens":12}`), today); ok {
			t.Errorf("Worth() of %q = %+v, want it unpriced", model, got)
		}
	}
}

func TestWhatThePricesCantPriceIsLeftUnpricedNotGuessed(t *testing.T) {
	tests := []struct {
		name         string
		usage        string
		want         ledger.Picodollars
		wantUnpriced []string
	}{
		{
			name:         "a server tool's requests the table doesn't know",
			usage:        `{"input_tokens":12,"server_tool_use":{"web_search_requests":1,"code_execution_requests":2,"web_fetch_requests":1}}`,
			want:         dollars(12*4e-6 + 0.01),
			wantUnpriced: []string{"server_tool_use.code_execution_requests"},
		},
		{
			name:         "writes to the cache it doesn't say the lasts of",
			usage:        `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`,
			want:         dollars(3000 * 8e-6),
			wantUnpriced: []string{"cache_creation_input_tokens"},
		},
		{
			name:         "writes to the cache of a last the table doesn't know",
			usage:        `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_1h_input_tokens":3000,"ephemeral_1d_input_tokens":120}}`,
			want:         dollars(3000 * 8e-6),
			wantUnpriced: []string{"cache_creation.ephemeral_1d_input_tokens"},
		},
		{
			name: "another model's turns, as an advisor's, which the API charges at that model's prices",
			usage: `{"input_tokens":12,"iterations":[{"type":"message","input_tokens":12},` +
				`{"type":"advisor_message","model":"claude-opus-5","input_tokens":823,"output_tokens":1612,"cache_read_input_tokens":0}]}`,
			want:         dollars(12 * 4e-6),
			wantUnpriced: []string{"iterations.advisor_message.input_tokens", "iterations.advisor_message.output_tokens"},
		},
		{
			name:  "a count the table doesn't know, of none",
			usage: `{"input_tokens":12,"server_tool_use":{"code_execution_requests":0}}`,
			want:  dollars(12 * 4e-6),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := worthOf(t, ledger.Pricing, "claude-opus-5-5", "", tt.usage, today)
			if got.Cost != tt.want || !slices.Equal(got.Unpriced, tt.wantUnpriced) {
				t.Errorf("%s is worth %+v, want %v, with %q unpriced", tt.usage, got, tt.want, tt.wantUnpriced)
			}
		})
	}
}

func TestAnAmountIsShownToTheCentAndGivenInDollarsExactly(t *testing.T) {
	tests := []struct {
		amount ledger.Picodollars
		cents  string
		json   string
	}{
		{amount: 0, cents: "$0.00", json: "0"},
		{amount: 1, cents: "$0.00", json: "0.000000000001"},
		{amount: dollars(0.004999), cents: "$0.00", json: "0.004999"},
		{amount: dollars(0.005), cents: "$0.01", json: "0.005"},
		{amount: dollars(412.532118), cents: "$412.53", json: "412.532118"},
		{amount: dollars(12), cents: "$12.00", json: "12"},
		{amount: -dollars(1.25), cents: "-$1.25", json: "-1.25"},
	}
	for _, tt := range tests {
		got, err := json.Marshal(tt.amount)
		if tt.amount.Cents() != tt.cents || err != nil || string(got) != tt.json {
			t.Errorf("%d picodollars shows as %s and is given as %s (%v), want %s and %s", tt.amount, tt.amount.Cents(), got, err, tt.cents, tt.json)
		}
	}
}

func TestASummaryIsPricedModelByModel(t *testing.T) {
	summary := ledger.Summary{Version: 1, Day: date, Lines: 9, Accounts: []ledger.AccountDay{
		{Account: "work", Sessions: 2, Models: []ledger.ModelDay{
			{Model: "claude-opus-5-5", Upstream: 2, Sessions: 1, Usage: usage(`{"input_tokens":1000,"output_tokens":100}`)},
			{Model: "claude-opus-5-5", InferenceGeo: "us", Upstream: 1, Sessions: 1, Usage: usage(`{"input_tokens":1000,"output_tokens":100}`)},
			{Model: "claude-opus-9", Upstream: 1, Sessions: 1, Usage: usage(`{"input_tokens":1000}`)},
			{Model: "claude-haiku-4-5", Checks: 1, Usage: usage(`{"input_tokens":8,"output_tokens":1,"server_tool_use":{"code_execution_requests":1}}`)},
			{Model: "claude-sonnet-5-5", Upstream: 2, NoUsage: 1, Sessions: 1, Usage: usage(`{"input_tokens":1000,"output_tokens":100}`)},
			{Model: "claude-fable-5", Upstream: 1, NoUsage: 1, Sessions: 1},
		}},
		{Models: []ledger.ModelDay{{Unsent: 1, Sessions: 1}}, Sessions: 1},
	}}

	got, err := json.Marshal(ledger.Pricing.Priced(summary, today))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"day":"2026-10-05","lines":9,"accounts":[` +
		`{"account":"work","sessions":2,"moved_on":0,"moved_off":0,"models":[` +
		`{"model":"claude-opus-5-5","upstream":2,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":1000,"output_tokens":100},"worth":0.006},` +
		`{"model":"claude-opus-5-5","inference_geo":"us","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,` +
		`"usage":{"input_tokens":1000,"output_tokens":100},"worth":0.0066},` +
		`{"model":"claude-opus-9","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":1000}},` +
		`{"model":"claude-haiku-4-5","upstream":0,"no_usage":0,"unsent":0,"checks":1,"counts":0,"sessions":0,` +
		`"usage":{"input_tokens":8,"output_tokens":1,"server_tool_use":{"code_execution_requests":1}},"worth":0.000013,"unpriced":["server_tool_use.code_execution_requests"]},` +
		`{"model":"claude-sonnet-5-5","upstream":2,"no_usage":1,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":1000,"output_tokens":100},` +
		`"worth":0.003,"unpriced":["no_usage"]},` +
		`{"model":"claude-fable-5","upstream":1,"no_usage":1,"unsent":0,"checks":0,"counts":0,"sessions":1,"worth":0,"unpriced":["no_usage"]}]},` +
		`{"sessions":1,"moved_on":0,"moved_off":0,"models":[{"upstream":0,"no_usage":0,"unsent":1,"checks":0,"counts":0,"sessions":1}]}]}`
	if string(got) != want {
		t.Errorf("the priced summary is\n%s\nwant\n%s: each model's worth beside it, none where the model is unpriced, and the requests that went "+
			"upstream without usage named unpriced, never free", got, want)
	}
}

func TestARequestThatWentUpstreamWithoutUsageIsUnpricedNeverFree(t *testing.T) {
	answered := asked("1", on(0, 9, 0))
	tests := []struct {
		name string
		edit func(l *ledger.Line)
		want ledger.Worth
	}{
		{name: "one whose answer gave its usage", edit: func(*ledger.Line) {}, want: ledger.Worth{Cost: dollars(10*4e-6 + 20*20e-6)}},
		{name: "one cut short", edit: func(l *ledger.Line) { l.Usage, l.CutOff = nil, true }, want: ledger.Worth{Unpriced: []string{"no_usage"}}},
		{name: "one the API answered with an error", edit: func(l *ledger.Line) { l.Usage, l.Status = nil, 529 }, want: ledger.Worth{Unpriced: []string{"no_usage"}}},
		{name: "one the router answered itself", edit: func(l *ledger.Line) { l.Usage, l.Attempts, l.Status = nil, 0, 429 }},
		{name: "a count of tokens, which spends nothing", edit: func(l *ledger.Line) { l.Usage, l.Kind = nil, ledger.KindCount }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := answered
			tt.edit(&line)
			if got, ok := ledger.Pricing.Request(&line, today); !ok || got.Cost != tt.want.Cost || !slices.Equal(got.Unpriced, tt.want.Unpriced) {
				t.Errorf("Request() = %+v, %v, want %+v", got, ok, tt.want)
			}
		})
	}
}

func TestAMovesCostIsItsRequestsCacheWriteAlone(t *testing.T) {
	// The rest of a request's usage, which its move's cost leaves out: input,
	// reads from the cache, output, a web search, and a count the table
	// doesn't know.
	const rest = `"input_tokens":12,"cache_read_input_tokens":182340,"output_tokens":845,"server_tool_use":{"web_search_requests":1,"code_execution_requests":1}`
	tests := []struct {
		name  string
		edit  func(l *ledger.Line)
		usage string
		want  ledger.Worth
	}{
		{name: "writes for five minutes, at the price of one", usage: `{` + rest + `,"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":3120}}`,
			want: ledger.Worth{Cost: dollars(3120 * 5e-6)}},
		{name: "writes for an hour, at the price of one", usage: `{` + rest + `,"cache_creation_input_tokens":182000,"cache_creation":{"ephemeral_1h_input_tokens":182000}}`,
			want: ledger.Worth{Cost: dollars(182000 * 8e-6)}},
		{name: "writes for each, each at its own", usage: `{` + rest + `,"cache_creation_input_tokens":3120,` +
			`"cache_creation":{"ephemeral_5m_input_tokens":120,"ephemeral_1h_input_tokens":3000}}`,
			want: ledger.Worth{Cost: dollars(120*5e-6 + 3000*8e-6)}},
		{name: "writes it doesn't say the lasts of, unpriced", usage: `{` + rest + `,"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`,
			want: ledger.Worth{Cost: dollars(3000 * 8e-6), Unpriced: []string{"cache_creation_input_tokens"}}},
		{name: "writes it doesn't break down at all, unpriced", usage: `{` + rest + `,"cache_creation_input_tokens":3120}`,
			want: ledger.Worth{Unpriced: []string{"cache_creation_input_tokens"}}},
		{name: "writes of a last the table doesn't know, unpriced", usage: `{` + rest + `,"cache_creation_input_tokens":3120,` +
			`"cache_creation":{"ephemeral_1h_input_tokens":3000,"ephemeral_1d_input_tokens":120}}`,
			want: ledger.Worth{Cost: dollars(3000 * 8e-6), Unpriced: []string{"cache_creation.ephemeral_1d_input_tokens"}}},
		{name: "no writes, nothing", usage: `{` + rest + `}`},
		{name: "writes in the US, a tenth more", edit: func(l *ledger.Line) { l.Shape.InferenceGeo = "us" },
			usage: `{` + rest + `,"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`,
			want:  ledger.Worth{Cost: dollars(3000 * 8e-6 * 1.1)}},
		{name: "an answer without usage, unpriced, never free", edit: func(l *ledger.Line) { l.CutOff = true },
			want: ledger.Worth{Unpriced: []string{"no_usage"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := asked("1", on(0, 9, 0))
			line.Usage = usage(tt.usage)
			if tt.edit != nil {
				tt.edit(&line)
			}
			if got, ok := ledger.Pricing.CacheWrite(&line, today); !ok || got.Cost != tt.want.Cost || !slices.Equal(got.Unpriced, tt.want.Unpriced) {
				t.Errorf("CacheWrite() of %s = %+v, %v, want %+v", tt.usage, got, ok, tt.want)
			}
		})
	}
}

func TestAMovesCostIsUnpricedWhereItsRequestIs(t *testing.T) {
	write := usage(`{"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`)
	for _, line := range []ledger.Line{
		{Model: "claude-opus-9", Kind: ledger.KindMessage, Attempts: 1, Usage: write},
		{Model: "claude-haiku-4-5", Kind: ledger.KindMessage, Attempts: 1, Usage: write, Shape: ledger.Shape{InferenceGeo: "us"}},
	} {
		if got, ok := ledger.Pricing.CacheWrite(&line, today); ok {
			t.Errorf("CacheWrite() of %s in %q = %+v, want it unpriced, as its request is", line.Model, line.Shape.InferenceGeo, got)
		}
	}
}

func TestASumOfRequestsNamesWhatItCantPrice(t *testing.T) {
	tests := []struct {
		name string
		edit func(l *ledger.Line)
		want ledger.Worth
	}{
		{name: "one the table prices, at its worth", edit: func(*ledger.Line) {}, want: ledger.Worth{Cost: dollars(10*4e-6 + 20*20e-6)}},
		{name: "one the table prices, but without usage", edit: func(l *ledger.Line) { l.Usage, l.CutOff = nil, true },
			want: ledger.Worth{Unpriced: []string{"no_usage"}}},
		{name: "one of a model the table doesn't know, its every count that costs anything", edit: func(l *ledger.Line) {
			l.Model = "claude-opus-9"
			l.Usage = usage(`{"input_tokens":10,"cache_read_input_tokens":0,"output_tokens":20,"output_tokens_details":{"thinking_tokens":5},` +
				`"server_tool_use":{"web_fetch_requests":1,"web_search_requests":1}}`)
		}, want: ledger.Worth{Unpriced: []string{"input_tokens", "output_tokens", "server_tool_use.web_search_requests"}}},
		{name: "one of a model the table doesn't know, without usage", edit: func(l *ledger.Line) { l.Model, l.Usage = "claude-opus-9", nil },
			want: ledger.Worth{Unpriced: []string{"no_usage"}}},
		{name: "one of a model the table doesn't know, the router answered itself", edit: func(l *ledger.Line) {
			l.Model, l.Usage, l.Attempts, l.Status = "claude-opus-9", nil, 0, 429
		}},
		{name: "one in a geo the table doesn't price its model in", edit: func(l *ledger.Line) { l.Model, l.Shape.InferenceGeo = "claude-haiku-4-5", "us" },
			want: ledger.Worth{Unpriced: []string{"input_tokens", "output_tokens"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := asked("1", on(0, 9, 0))
			tt.edit(&line)
			if got := ledger.Pricing.Summed(&line, today); got.Cost != tt.want.Cost || !slices.Equal(got.Unpriced, tt.want.Unpriced) {
				t.Errorf("Summed() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestACachesWritesLastFiveMinutesOnlyWhereEachDoes(t *testing.T) {
	tests := []struct {
		usage string
		want  time.Duration
	}{
		{usage: `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":3120,"ephemeral_1h_input_tokens":0}}`, want: 5 * time.Minute},
		{usage: `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":3120}}`, want: time.Hour},
		{usage: `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":120,"ephemeral_1h_input_tokens":3000}}`, want: time.Hour},
		{usage: `{"cache_creation_input_tokens":3120}`, want: time.Hour},
		// Of these, which wrote nothing, nothing is said.
		{usage: `{"cache_read_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}}`},
		{usage: `{"cache_read_input_tokens":3120,"cache_creation_input_tokens":0}`},
		{usage: ``},
	}
	for _, tt := range tests {
		if got, ok := (ledger.Reply{Usage: usage(tt.usage)}).CacheLife(); got != tt.want || ok != (tt.want != 0) {
			t.Errorf("CacheLife() of %s = %v, %v, want %v, as it wrote to the cache", tt.usage, got, ok, tt.want)
		}
	}
}

func TestAMoveWouldWriteTheWholePromptAgainAtTheCachesOwnPrice(t *testing.T) {
	// The prompt: 12 input, 182,340 read from the cache and 3,120 written to
	// it; the output and a web search are left out.
	const prompt = `"input_tokens":12,"cache_read_input_tokens":182340,"output_tokens":845,"server_tool_use":{"web_search_requests":1},` +
		`"cache_creation_input_tokens":3120`
	const tokens = 12 + 182340 + 3120
	tests := []struct {
		name string
		geo  string
		life time.Duration
		want ledger.Picodollars
	}{
		{name: "writes for an hour, at the price of one", life: ledger.LongCache, want: dollars(tokens * 8e-6)},
		{name: "writes for five minutes, at the price of one", life: ledger.ShortCache, want: dollars(tokens * 5e-6)},
		{name: "in the US, a tenth more", geo: "us", life: ledger.LongCache, want: dollars(tokens * 8e-6 * 1.1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := asked("1", on(0, 9, 0))
			line.Usage, line.Shape.InferenceGeo = usage(`{`+prompt+`,"cache_creation":{"ephemeral_1h_input_tokens":3120}}`), tt.geo
			if got, ok := ledger.Pricing.Rewrite(&line, tt.life, today); !ok || got != tt.want {
				t.Errorf("Rewrite() for %v = %v, %v, want %v", tt.life, got, ok, tt.want)
			}
		})
	}
}

func TestAMoveWhosePromptIsntKnownIsntPriced(t *testing.T) {
	write := usage(`{"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_1h_input_tokens":3000}}`)
	for _, line := range []ledger.Line{
		{Model: "claude-opus-9", Kind: ledger.KindMessage, Attempts: 1, Usage: write},
		{Model: "claude-haiku-4-5", Kind: ledger.KindMessage, Attempts: 1, Usage: write, Shape: ledger.Shape{InferenceGeo: "us"}},
		{Model: "claude-opus-5-5", Kind: ledger.KindMessage, Attempts: 1, CutOff: true},
	} {
		if got, ok := ledger.Pricing.Rewrite(&line, ledger.LongCache, today); ok {
			t.Errorf("Rewrite() of %s in %q, usage %s, = %v, want it unpriced", line.Model, line.Shape.InferenceGeo, line.Usage, got)
		}
	}
}
