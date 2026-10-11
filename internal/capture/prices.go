package capture

import "github.com/leeovery/switchboard/internal/ledger"

// drawnPrices is the price table the frames were drawn at, which their
// fixtures inject in place of the built-in table: every model priced alike,
// at a sample of Opus 5's rates, from the day each launched, the table dated 1
// October 2026 as the frames were drawn; its plans and versions the built-in
// table's.
var drawnPrices = func() ledger.Table {
	t := ledger.Pricing
	t.AsOf = "2026-10-01"
	t.Models = make([]ledger.Model, len(ledger.Pricing.Models))
	for i, m := range ledger.Pricing.Models {
		first := m.Prices[0]
		t.Models[i] = ledger.Model{IDs: m.IDs, Prices: []ledger.Prices{{
			From: first.From, Input: 5_000_000, CacheWrite5m: 6_250_000, CacheWrite1h: 10_000_000, CacheRead: 500_000,
			Output: 25_000_000, WebSearch: first.WebSearch, USOnlyPercent: first.USOnlyPercent,
		}}}
	}
	return t
}()
