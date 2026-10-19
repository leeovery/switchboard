package views

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// The head of a day of the golden world, as the ledger writes it, its side
// as the views' Digests build it, is what the golden of the versions it's
// written in holds: testdata/rows-v<RowsVersion>.json, inflated. A change to
// how a day's side is stored, or to the working behind it, or to the
// ledger's envelope of a day's head, that changes it without a new version
// fails, as heads already written would be read as they no longer are. A new
// version has its golden written with -update, in place of the one before,
// which is removed; one already kept of the envelope written isn't written
// again. Its world's times are UTC's, so it encodes alike in every zone the
// tests run in.
func TestADaysHeadIsEncodedAsTheGoldenOfItsVersionsHoldsIt(t *testing.T) {
	zone := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = zone })
	keptGolden{
		file: "rows", what: "a day's head", version: RowsVersion, test: "TestADaysHeadIsEncodedAsTheGoldenOfItsVersionsHoldsIt", envelope: headEnvelopeOf,
		bump: "RowsVersion where how a day's side is stored, or the working behind it, changed, or the ledger's logdayVersion where its envelope did",
	}.check(t, goldenHead(t))
}

// goldenHead is the head of goldenDay of the golden world, written as a real
// ledger and digested day by day, as goldenChain has it: its file's JSON,
// inflated, indented, ending its last line.
func goldenHead(t *testing.T) []byte {
	t.Helper()
	c := goldenChain(t)
	data, err := os.ReadFile(filepath.Join(ledger.Dir(c.state), "logday-"+goldenDay+".json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return indented(t, gunzipped(t, data))
}

// headEnvelopeOf is the version of the ledger's envelope a golden's day head
// names.
func headEnvelopeOf(t *testing.T, kept []byte) int {
	t.Helper()
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(kept, &head); err != nil {
		t.Fatal(err)
	}
	return head.Version
}
