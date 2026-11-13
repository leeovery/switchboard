package cli_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// An event naming a pool since removed is no pool's; and where the accounts
// record notes no first pool, work naming none is none's either.
func TestWorkNamingAPoolRemovedOrNoneWithTheFirstUnknownIsNoPools(t *testing.T) {
	deps := pooledDeps(t, false)
	fileEvent(t, deps, "2026-10-07", eventLine(runOne, 6, status.Event{At: october(7, 12, 30, 0), Kind: status.EventStarted, Session: sessionC, Account: "work", Pool: "x9z9", Reason: status.ReasonNew}))
	for _, args := range [][]string{{"events"}, {"events", "--pool", "q7mt"}} {
		got := run(t, deps, append(args, "--json")...)
		if pools := fieldsOf(t, got, "pool"); got.code != 0 || slices.Contains(pools, "x9z9") {
			t.Errorf("switchboard %s --json printed events of the pools %q (%+v), want none of x9z9, removed", strings.Join(args, " "), pools, got)
		}
	}

	err := identity.NewFile(stateDir(t, deps)).Update(func(r *identity.Record) error {
		r.Pools = []identity.PoolEntry{{ID: "b2n8", Added: testNow, Here: true}, {ID: "q7mt", Added: testNow, Here: true}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args  []string
		field string
		want  []string
	}{
		{args: []string{"requests"}, field: "request", want: []string{"p3"}},
		{args: []string{"events"}, field: "kind", want: []string{"restart"}},
	}
	for _, tt := range tests {
		got := run(t, deps, append(tt.args, "--json")...)
		if values := fieldsOf(t, got, tt.field); got.code != 0 || !slices.Equal(values, tt.want) {
			t.Errorf("switchboard %s --json, the first pool unknown, printed %s %q (%+v), want %q", strings.Join(tt.args, " "), tt.field, values, got, tt.want)
		}
	}
}

// An account that worked in two pools on one day is read in each with its
// work there alone, its requests and its day's sessions alike.
func TestAnAccountsDayInTwoPoolsIsReadInEachByItsOwnWork(t *testing.T) {
	deps := pooledDeps(t, false)
	var lines bytes.Buffer
	for _, l := range []ledger.Line{
		{At: october(6, 9, 0, 0), Request: "p7", Session: sessionA, Account: "work", Pool: "b2n8"},
		{At: october(6, 10, 0, 0), Request: "p8", Session: sessionB, Account: "work", Pool: "q7mt"},
		{At: october(6, 11, 0, 0), Request: "p9", Session: sessionB, Account: "work", Pool: "q7mt"},
	} {
		l.At, l.Kind, l.Model, l.Reason, l.Status, l.Attempts, l.TotalMS, l.Usage = l.At.UTC(), ledger.KindMessage, "claude-opus-5-5", "new", 200, 1, 100, json.RawMessage(unknownUsage)
		data, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(append(data, '\n'))
	}
	writeStateFile(t, ledger.Dir(stateDir(t, deps)), "requests-2026-10-06.jsonl", lines.Bytes())

	requests := []struct {
		args []string
		want []string
	}{
		{args: []string{"requests", "--since", "2026-10-06"}, want: []string{"p7", "p1", "p3"}},
		{args: []string{"requests", "--since", "2026-10-06", "--pool", "q7mt"}, want: []string{"p8", "p9", "p2"}},
	}
	for _, tt := range requests {
		got := run(t, deps, append(tt.args, "--json")...)
		if values := fieldsOf(t, got, "request"); got.code != 0 || !slices.Equal(values, tt.want) {
			t.Errorf("switchboard %s --json printed the requests %q (%+v), want %q", strings.Join(tt.args, " "), values, got, tt.want)
		}
	}
	history := []struct {
		args []string
		want []string
	}{
		{args: nil, want: []string{"work b2n8 Default s", "side b2n8 Default ss"}},
		{args: []string{"--pool", "q7mt"}, want: []string{"work q7mt Acme s", "work q7mt Acme s"}},
	}
	for _, tt := range history {
		got := run(t, deps, append([]string{"history", "--json", "--since", "2026-10-06"}, tt.args...)...)
		if accounts := historyAccounts(t, got); !slices.Equal(accounts, tt.want) {
			t.Errorf("switchboard history --json --since 2026-10-06 %s gave the accounts %q, want %q", strings.Join(tt.args, " "), accounts, tt.want)
		}
	}
}

// With two pools, the text of the verbs reading the files names the pool it
// reads first, as status and usage do, and tells of its accounts alone.
func TestTheVerbsTextNamesThePoolOnceThereIsASecond(t *testing.T) {
	deps := pooledDeps(t, false)
	writeReadings(t, readings.Dir(stateDir(t, deps)),
		readings.Reading{At: october(7, 9, 30, 0), Account: "work", Key: "5h", Utilization: 0.4, ResetsAt: october(7, 14, 0, 0), Status: quota.StatusAllowed},
		readings.Reading{At: october(7, 10, 30, 0), Account: "side", Key: "5h", Utilization: 0.2, ResetsAt: october(7, 15, 0, 0), Status: quota.StatusAllowed})
	for _, args := range [][]string{{"requests"}, {"events"}, {"sessions"}, {"history"}, {"history", "--windows"}} {
		got := run(t, deps, append(args, "--pretty", "--pool", "q7mt")...)
		if got.code != 0 || !strings.HasPrefix(got.stdout, "pool Acme\n\n") || strings.Contains(got.stdout, "Side") {
			t.Errorf("switchboard %s --pretty --pool q7mt = %+v, want it to start naming pool Acme, and tell nothing of Side, in Default", strings.Join(args, " "), got)
		}
	}
	tests := []struct {
		args []string
		want []string
	}{
		{args: nil, want: []string{"side"}},
		{args: []string{"--pool", "q7mt"}, want: []string{"work"}},
	}
	for _, tt := range tests {
		got := run(t, deps, append([]string{"history", "--windows", "--json"}, tt.args...)...)
		var doc struct {
			Windows []struct {
				Account string `json:"account"`
			} `json:"windows"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
			t.Fatalf("switchboard history --windows --json %s = %+v: %v", strings.Join(tt.args, " "), got, err)
		}
		var accounts []string
		for _, w := range doc.Windows {
			accounts = append(accounts, w.Account)
		}
		if !slices.Equal(accounts, tt.want) {
			t.Errorf("switchboard history --windows --json %s gave the windows of %q, want %q: the pool's accounts' alone", strings.Join(tt.args, " "), accounts, tt.want)
		}
	}
}
