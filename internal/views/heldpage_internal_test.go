package views

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Past midnight, a held page is built afresh once, as the first look takes
// in the new day's lines, none here, and rolls it on to the new day: no look
// after builds it again, each its story as held.
func TestAHeldPagePastMidnightIsBuiltOnceForAllItsLooks(t *testing.T) {
	before := time.Date(2026, 10, 7, 23, 50, 0, 0, time.Local)
	after := before.Add(20 * time.Minute)
	lines := []ledger.Line{{
		Request: "r1", At: before, Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work",
		Status: 200, Attempts: 1, TotalMS: 4000, Usage: []byte(`{"input_tokens":1000}`),
	}}
	h := HoldPage("s1", SessionsRead{day: Today(before), lines: map[string][]ledger.Line{"s1": lines}}, ledger.Pricing, 0, before)
	built := map[*story]bool{}
	for range 2 {
		h = h.Take(Today(after).Date, nil)
		built[h.story] = true
		built[h.at(after).story] = true
	}
	if len(built) != 1 || h.day != Today(after) {
		t.Errorf("two looks past midnight built the page %d times, held at %s; want it built once, held at %s", len(built), h.day.Date, Today(after).Date)
	}
	if lags := h.Take(Today(before).Date, nil); lags.story != h.story || lags.day != h.day {
		t.Errorf("a look of the day before, as a ledger yet to turn gives, held the page at %s, built afresh %t; want it as held", lags.day.Date, lags.story != h.story)
	}
}
