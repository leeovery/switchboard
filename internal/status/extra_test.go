package status_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
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

// The moment gives each account's extra usage as its document gives it,
// probed or routed, and says how it stands for it.
func TestTheMomentGivesEachAccountsExtraUsageAsItsDocumentDoes(t *testing.T) {
	on := quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.4), ResetsAt: built.Add(3 * day)}
	off := quota.ExtraUsage{Status: quota.StatusRejected}
	work := with(readAt("work", windowIn("5h", 0.3, 3*time.Hour)), func(a *status.Account) { a.Extra = on })
	personal := with(readAt("personal", windowIn("5h", 0.5, 2*time.Hour)), func(a *status.Account) { a.Extra = off })
	side := readAt("side", windowIn("5h", 0.1, 4*time.Hour))
	want := map[string]struct {
		extra quota.ExtraUsage
		said  status.ExtraState
	}{
		"work":     {extra: on, said: status.ExtraOn},
		"personal": {extra: off, said: status.ExtraOff},
		"side":     {said: status.ExtraNotRead},
	}
	tests := []struct {
		name string
		doc  status.Document
	}{
		{name: "probed", doc: statustest.Probed(policy, work, personal, side)},
		{name: "routed", doc: statustest.Router(policy, nil, statustest.Routed{Account: work}, statustest.Routed{Account: personal}, statustest.Routed{Account: side})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, a := range tt.doc.At(built.Add(time.Minute), policy).Accounts {
				if w := want[a.ID]; !reflect.DeepEqual(a.Extra, w.extra) || a.ExtraSaid() != w.said {
					t.Errorf("%s gives extra usage %+v, said %d; want %+v, said %d", a.ID, a.Extra, a.ExtraSaid(), w.extra, w.said)
				}
			}
		})
	}
}

// An account's extra usage is on while its status isn't rejected, off where
// it is, and not read where nothing has given it.
func TestAnAccountsExtraUsageIsOnWhileItsStatusIsntRejected(t *testing.T) {
	tests := []struct {
		name  string
		extra quota.ExtraUsage
		want  status.ExtraState
	}{
		{name: "not given", want: status.ExtraNotRead},
		{name: "rejected", extra: quota.ExtraUsage{Status: quota.StatusRejected}, want: status.ExtraOff},
		{name: "rejected, its use given", extra: quota.ExtraUsage{Status: quota.StatusRejected, Utilization: new(0.0)}, want: status.ExtraOff},
		{name: "allowed", extra: quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.2)}, want: status.ExtraOn},
		{name: "allowed, with a warning", extra: quota.ExtraUsage{Status: quota.StatusAllowedWarning, Utilization: new(0.9)}, want: status.ExtraOn},
		{name: "its use given, its status not", extra: quota.ExtraUsage{Utilization: new(0.2)}, want: status.ExtraOn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (status.AccountNow{Extra: tt.extra}).ExtraSaid(); got != tt.want {
				t.Errorf("ExtraSaid() of %+v = %d, want %d", tt.extra, got, tt.want)
			}
		})
	}
}

// Every account's extra usage, taken together, counts those read on, of
// every account: on where any is, off where those read are all off, and not
// read where none is.
func TestEveryAccountsExtraUsageTakenTogether(t *testing.T) {
	on, off, unread := quota.ExtraUsage{Status: quota.StatusAllowed}, quota.ExtraUsage{Status: quota.StatusRejected}, quota.ExtraUsage{}
	tests := []struct {
		name  string
		extra []quota.ExtraUsage
		want  status.AllExtra
	}{
		{name: "none read", extra: []quota.ExtraUsage{unread, unread, unread}, want: status.AllExtra{State: status.ExtraNotRead, Of: 3}},
		{name: "off on all", extra: []quota.ExtraUsage{off, off, off}, want: status.AllExtra{State: status.ExtraOff, Of: 3}},
		{name: "on on all", extra: []quota.ExtraUsage{on, on, on}, want: status.AllExtra{State: status.ExtraOn, On: 3, Of: 3}},
		{name: "one on, one off, one not read", extra: []quota.ExtraUsage{off, on, unread}, want: status.AllExtra{State: status.ExtraOn, On: 1, Of: 3}},
		{name: "those read off, one not read", extra: []quota.ExtraUsage{off, unread, off}, want: status.AllExtra{State: status.ExtraOff, Of: 3}},
		{name: "the one read on, two not read", extra: []quota.ExtraUsage{unread, unread, on}, want: status.AllExtra{State: status.ExtraOn, On: 1, Of: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := make([]status.AccountNow, len(tt.extra))
			for i, e := range tt.extra {
				accounts[i].Extra = e
			}
			if got := status.ExtraOfAll(accounts); got != tt.want {
				t.Errorf("ExtraOfAll() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
