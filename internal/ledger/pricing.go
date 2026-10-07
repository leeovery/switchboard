package ledger

// Pricing is the price table switchboard carries: every model on Anthropic's
// pricing page, https://platform.claude.com/docs/en/about-claude/pricing, as
// read on 7 October 2026, the day the ledger began recording, but those the
// API had retired by then, as no answer of theirs is in the ledger to price.
// Each model's prices are those it launched with, on the day its release
// notes give: none has changed since. A cache read is the page's own price,
// never worked out from the input's, as its share of it differs from model to
// model. US-only inference costs a tenth more on Claude 4.6 and later, and a
// web search $10 a thousand, beyond its tokens, on every model. No
// long-context rate is kept: Claude 4.6 and later price their whole context
// at their standard rates, and the 1M-token beta that charged more on Claude
// Sonnet 4 and 4.5 was retired on 30 April 2026.
var Pricing = Table{AsOf: "2026-10-07", Models: []Model{
	priced("2026-09-01", 10, 12.50, 20, 0.25, 50, usOnly, "claude-fable-5-1"),
	priced("2026-09-01", 10, 12.50, 20, 0.25, 50, usOnly, "claude-mythos-5-1"),
	priced("2026-06-09", 10, 12.50, 20, 1, 50, usOnly, "claude-fable-5"),
	priced("2026-06-09", 10, 12.50, 20, 1, 50, usOnly, "claude-mythos-5"),
	priced("2026-09-22", 4, 5, 8, 0.20, 20, usOnly, "claude-opus-5-5"),
	priced("2026-07-24", 5, 6.25, 10, 0.50, 25, usOnly, "claude-opus-5"),
	priced("2026-05-28", 5, 6.25, 10, 0.50, 25, usOnly, "claude-opus-4-8"),
	priced("2026-04-16", 5, 6.25, 10, 0.50, 25, usOnly, "claude-opus-4-7"),
	priced("2026-02-05", 5, 6.25, 10, 0.50, 25, usOnly, "claude-opus-4-6"),
	priced("2025-11-24", 5, 6.25, 10, 0.50, 25, globalOnly, "claude-opus-4-5-20251101", "claude-opus-4-5"),
	priced("2026-09-28", 2, 2.50, 4, 0.20, 10, usOnly, "claude-sonnet-5-5"),
	priced("2026-06-30", 2, 2.50, 4, 0.20, 10, usOnly, "claude-sonnet-5"),
	priced("2026-02-17", 3, 3.75, 6, 0.30, 15, usOnly, "claude-sonnet-4-6"),
	priced("2025-09-29", 3, 3.75, 6, 0.30, 15, globalOnly, "claude-sonnet-4-5-20250929", "claude-sonnet-4-5"),
	priced("2025-10-15", 1, 1.25, 2, 0.10, 5, globalOnly, "claude-haiku-4-5-20251001", "claude-haiku-4-5"),
}}

const (
	// usOnly is what a token costs where a request asks for US-only
	// inference, as a percentage of its price, on Claude 4.6 and later.
	usOnly = 110
	// globalOnly is a model that doesn't offer US-only inference, as those
	// before Claude 4.6 don't.
	globalOnly = 0
	// webSearch is a web search's price, as dollars a thousand.
	webSearch = 10
)

// priced is the model the ids given name, priced from the day with the date
// from on, each price given as dollars a million tokens: of input, of writing
// the prompt cache for five minutes and for an hour, of reading it, and of
// output, US-only inference costing usOnlyPercent of each.
func priced(from string, input, write5m, write1h, read, output float64, usOnlyPercent int64, ids ...string) Model {
	return Model{IDs: ids, Prices: []Prices{{
		From: from, Input: perMTok(input), CacheWrite5m: perMTok(write5m), CacheWrite1h: perMTok(write1h), CacheRead: perMTok(read),
		Output: perMTok(output), WebSearch: perThousand(webSearch), USOnlyPercent: usOnlyPercent,
	}}}
}
