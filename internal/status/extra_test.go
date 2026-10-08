package status_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

func TestTheProbedDocumentGivesEachAccountsExtraUsageAsItsProbeLeftIt(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: now.Add(4 * time.Hour)}
	extra := quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.4), ResetsAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	prober := &fakeProber{results: map[string]probeResult{
		"test-token-work": {usage: quota.Usage{Windows: []quota.Window{session}, Extra: extra}},
		"test-token-side": withWindows(session),
	}}
	collector := status.Collector{
		Prober: prober,
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
		Now:    func() time.Time { return now },
	}

	doc := collector.Collect(t.Context(), accounts)
	work, _ := doc.Account("work")
	if !reflect.DeepEqual(work.Extra, extra) || len(work.Windows) != 1 {
		t.Errorf("work reads extra usage %+v beside windows %+v, want %+v beside its one window", work.Extra, work.Windows, extra)
	}
	for _, id := range []string{"personal", "side"} {
		if a, _ := doc.Account(id); a.Extra.Given() {
			t.Errorf("%s reads extra usage %+v, want none, as none was given", id, a.Extra)
		}
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var printed struct {
		Accounts []map[string]json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(data, &printed); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"work": `{"status":"allowed","utilization":0.4,"resets_at":"2026-10-01T00:00:00Z"}`}
	for _, a := range printed.Accounts {
		var id string
		if err := json.Unmarshal(a["id"], &id); err != nil {
			t.Fatal(err)
		}
		if got := string(a["extra_usage"]); got != want[id] {
			t.Errorf("%s's extra_usage is %q, want %q", id, got, want[id])
		}
	}
}
