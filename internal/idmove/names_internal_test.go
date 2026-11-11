package idmove

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The router writes its lines with json.Marshal, which escapes <, > and &:
// a reason renamed is written so too, so every byte but the ids is as the
// router would write the line renamed.
func TestALineRenamedIsWrittenAsTheRouterWritesIt(t *testing.T) {
	line := func(account, reason string) []byte {
		data, err := json.Marshal(ledger.Line{At: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC), Request: "r1", Kind: ledger.KindMessage,
			Account: account, Reason: reason, Status: 200, Attempts: 1})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	rename := func(id string) string {
		if id == "work" {
			return "wk7q"
		}
		return id
	}
	was := line("work", status.ReasonPinYields+"work <has> no room & more")
	got, changed := lineNames.renamed(was, rename)
	if want := line("wk7q", status.ReasonPinYields+"wk7q <has> no room & more"); !changed || string(got) != string(want) {
		t.Errorf("renamed, the line reads\n%s\nwant\n%s", got, want)
	}
}
