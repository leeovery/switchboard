package views

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
// ledger and digested day by day, as goldenChain has it, its session o's last
// request on the day arriving to the nanosecond, as only a line edited by
// hand does, so a side kept to the millisecond fails it, and the day given
// the lines headChecked adds: its file's JSON, inflated, indented, ending its
// last line. The digest's golden keeps the world as it is.
func goldenHead(t *testing.T) []byte {
	t.Helper()
	lines, zoned := goldenWorld()
	last := slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Request == "o4" })
	lines[last].At = lines[last].At.Add(123_456 * time.Nanosecond)
	c := goldenChain(t, append(lines, headChecked()...), zoned)
	data, err := os.ReadFile(filepath.Join(ledger.Dir(c.state), "logday-"+goldenDay+".json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return indented(t, gunzipped(t, data))
}

// headChecked are the lines goldenHead adds to goldenDay, so its rows fill
// a chunk and begin another, and their meta holds a value of every key: a
// quota check of session o's every five minutes from just past five past midnight,
// enough of them that the day's lines are one more than a chunk holds, one
// refused as overloaded; and a count of tokens of session s's in the
// afternoon.
func headChecked() []ledger.Line {
	from := time.Date(2026, 10, 8, 0, 5, 0, 987_654_321, time.Local).UTC()
	checks := rowsAChunk - 7
	lines := make([]ledger.Line, 0, checks+1)
	for i := range checks {
		check := ledger.Line{
			At: from.Add(time.Duration(i) * 5 * time.Minute), Request: fmt.Sprintf("q%d", i), Kind: ledger.KindCheck, Session: "o", Model: foldHaiku,
			Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 300, Usage: json.RawMessage(`{"input_tokens":8,"output_tokens":1}`),
		}
		if i == 100 {
			check.Status, check.Usage = 529, nil
		}
		lines = append(lines, check)
	}
	return append(lines, ledger.Line{
		At: time.Date(2026, 10, 8, 16, 0, 0, 0, time.Local).UTC(), Request: "n1", Kind: ledger.KindCount, Session: "s", Model: foldOpus,
		Account: "side", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 90,
	})
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
