package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// The router's ledger gives a day's work in no pool the first pool as its
// table has it: the config's first, once the one the accounts record notes
// is no longer configured, as after pool remove of an emptied Default.
func TestTheRoutersLedgerGivesWorkInNoPoolTheConfigsFirstOnceTheOneNotedIsRemoved(t *testing.T) {
	logstest.Capture(t)
	day := start.AddDate(0, 0, -1)
	date := dayfile.DateOf(day)
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.StateDir = t.TempDir()
	cfg.Accounts = config.Accounts{{ID: "work", Label: "Work", Pool: acmePool, Primary: true}}
	cfg.Pools = config.Pools{{ID: acmePool, Label: "Acme"}}
	writeFile(t, filepath.Join(cfg.StateDir, "pools.moved"), "")
	noteFirstPool(t, cfg.StateDir, defaultPool)
	line, err := json.Marshal(ledger.Line{At: day.UTC(), Request: "r1", Kind: ledger.KindMessage, Session: "one", Model: opus,
		Account: "work", Reason: "sticky", Status: 200, Attempts: 1, Usage: json.RawMessage(`{"input_tokens":10}`)})
	if err != nil {
		t.Fatal(err)
	}
	dir := ledger.Dir(cfg.StateDir)
	writeFile(t, filepath.Join(dir, "requests-"+date+".jsonl"), string(line)+"\n")

	r := newTestRouterOf(t, cfg)
	r.openHistory()
	r.openLedger(dir)
	r.ledger.opened.Load().SummariseEnded(start)

	data, err := os.ReadFile(ledger.SummaryFile(dir, date))
	if err != nil {
		t.Fatal(err)
	}
	var s ledger.Summary
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Accounts {
		if a.Account == "work" && a.InPool(acmePool, "").Sessions != 1 {
			t.Errorf("the day's summary has work's day %+v, want its session in Acme, the first now, %s being removed", a, defaultPool)
		}
	}
	if len(s.Accounts) == 0 {
		t.Errorf("the day's summary holds no account, want work's")
	}
}
