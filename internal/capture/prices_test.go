package capture

import (
	"encoding/json"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

func TestTheFramesPricesAreASampleEveryModelAlike(t *testing.T) {
	if drawnPrices.AsOf != "2026-10-01" {
		t.Errorf("the frames' table is as of %s, want 2026-10-01, as the frames were drawn", drawnPrices.AsOf)
	}
	// Work's week as History's Tokens draws it: 1.2M input, 0.8M output, 11.0M
	// written to the cache for five minutes and 178M read from it, $184.
	week := json.RawMessage(`{"input_tokens":1200000,"output_tokens":800000,"cache_creation_input_tokens":11000000,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":11000000},"cache_read_input_tokens":178000000}`)
	// And a request of a session's, written to the cache for an hour.
	hour := json.RawMessage(`{"input_tokens":3,"output_tokens":900,"cache_creation_input_tokens":3000,` +
		`"cache_creation":{"ephemeral_1h_input_tokens":3000},"cache_read_input_tokens":40000}`)
	for _, m := range ledger.Pricing.Models {
		for _, id := range m.IDs {
			date := m.Prices[0].From
			if got, ok := drawnPrices.Worth(id, "", week, date); !ok || got.Cost != 183_750_000_000_000 {
				t.Errorf("%s's worth of work's week is %v (%t), want $183.75", id, got, ok)
			}
			if got, ok := drawnPrices.Worth(id, "", hour, date); !ok || got.Cost != 72_515_000_000 {
				t.Errorf("%s's worth of a session's request is %v (%t), want $0.072515", id, got, ok)
			}
		}
	}
}
