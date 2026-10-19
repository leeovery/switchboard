package router

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// handedDays is a ledger.Digester that gives each session it's handed an
// empty state, and no part, noting the sessions of each day, by its date;
// and each day an empty side, and a chunk of rows of its own.
type handedDays map[string][]string

// Version is the version of the states it gives.
func (handedDays) Version() int {
	return 1
}

// Builder gives itself, as the builder of a day's digest.
func (h handedDays) Builder() ledger.DigestBuilder {
	return h
}

// RowsVersion is the version of the sides it gives.
func (handedDays) RowsVersion() int {
	return 1
}

// Day gives a builder of a day's side and rows, as emptySide has it.
func (handedDays) Day() ledger.DayBuilder {
	return emptySide{}
}

// emptySide is a builder of a day's side and rows that gives an empty side,
// and a chunk of one row, whatever it's handed.
type emptySide struct{}

// Take takes in no line.
func (emptySide) Take(ledger.Line) {}

// Built gives an empty side, and a chunk of one row.
func (emptySide) Built() (ledger.DayBuilt, error) {
	return ledger.DayBuilt{Side: json.RawMessage(`[]`), Chunks: []ledger.BuiltChunk{{Rows: []byte("{}\n"), Count: 1}}}, nil
}

// Take takes in no line.
func (handedDays) Take(string, ledger.Line) {}

// Digest notes the sessions of day, giving each an empty state.
func (h handedDays) Digest(_ context.Context, day ledger.DigestDay) (map[string]ledger.Folded, error) {
	folded := make(map[string]ledger.Folded, len(day.Sessions))
	for id := range day.Sessions {
		h[day.Date] = append(h[day.Date], id)
		folded[id] = ledger.Folded{State: json.RawMessage(`{}`)}
	}
	return folded, nil
}

// A router given a Digester has its ledger, as it runs, digest each day that
// has ended with it, once the day is summarised, and build its head and its
// rows, and keep them beside the day's lines.
func TestTheRoutersLedgerDigestsEachEndedDayWithItsDigester(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		day := time.Date(2026, 9, 8, 10, 0, 0, 0, time.Local)
		date := day.Format(time.DateOnly)
		handed := make(handedDays)
		cfg := testConfig((&testClock{now: day.AddDate(0, 0, 2)}).read, &stubProber{}, testTokens.Read)
		cfg.Digester = handed
		r := newTestRouterOf(t, cfg)
		state := t.TempDir()
		ledgerDir, historyDir := ledger.Dir(state), readings.Dir(state)
		for _, dir := range []string{ledgerDir, historyDir} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		line, err := json.Marshal(ledger.Line{At: day.UTC(), Request: "3f2a91c4", Kind: ledger.KindMessage, Session: "one", Model: opus,
			Account: "work", Reason: "sticky", Status: 200, Attempts: 1, Usage: json.RawMessage(`{"input_tokens":10}`)})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ledgerDir, "requests-"+date+".jsonl"), append(line, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		r.history.open(historyDir)
		r.ledger.open(ledgerDir, ledger.HistoryIn(r.history.files))

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			r.ledger.run(ctx)
			close(done)
		}()
		synctest.Wait()
		cancel()
		<-done
		if !slices.Equal(handed[date], []string{"one"}) {
			t.Errorf("the digester was handed sessions %v, want of %s its session one", handed, date)
		}
		for _, name := range []string{"digest-" + date + ".json.gz", "logday-" + date + ".json.gz", "rows-" + date + ".gz"} {
			if _, err := os.Stat(filepath.Join(ledgerDir, name)); err != nil {
				t.Errorf("the ledger holds no %s: %v", name, err)
			}
		}
	})
}
