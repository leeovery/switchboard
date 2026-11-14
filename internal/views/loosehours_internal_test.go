package views

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// looseAsk is a request of no session, on work, its id the one given,
// arrived at 10:00 on 7 October.
func looseAsk(id string) ledger.Held {
	return ledger.Held{
		At: time.Date(2026, 10, 7, 10, 0, 0, 0, time.Local).UTC(), Request: id, Kind: ledger.KindMessage, Account: "work",
		Model: "claude-opus-5-5", Status: 200, Class: ledger.ClassMain, Usage: json.RawMessage(`{"input_tokens":1000,"output_tokens":100}`),
	}
}

// A fold taking a read in leaves the fold it was taken from as it was: its
// hours of requests of no session among what it shares with none.
func TestWithLeavesAnEarlierFoldsLooseHoursAsTheyWere(t *testing.T) {
	earlier := todaysSessions().with(RecordsOf([]ledger.Held{looseAsk("first")}).records)
	was := *earlier.loose["work"]
	later := earlier.with(RecordsOf([]ledger.Held{looseAsk("second")}).records)
	if !reflect.DeepEqual(*earlier.loose["work"], was) {
		t.Errorf("a later read took in, the earlier fold's hours are %+v, want them as they were, %+v", *earlier.loose["work"], was)
	}
	if reflect.DeepEqual(*later.loose["work"], was) {
		t.Errorf("the later fold's hours are %+v, want the second request taken in", *later.loose["work"])
	}
}
