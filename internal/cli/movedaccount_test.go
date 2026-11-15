package cli_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// sessionM is a session work served in Default, before it moved to Acme.
const sessionM = "4e2f1a0b-0006-4a00-8000-000000000006"

// movedSaid is how the verbs' text names work seen from Default, which it
// left for Acme.
const movedSaid = "Work (now in Acme)"

// movedDeps are pooledDeps with work's work in Default before it moved to
// Acme, where the config has it now: a request of sessionM's, and its start.
func movedDeps(t *testing.T) cli.Deps {
	t.Helper()
	deps := pooledDeps(t, false)
	moved := ledger.Line{At: october(7, 8, 0, 0), Request: "m1", Kind: ledger.KindMessage, Session: sessionM, Model: "claude-opus-5-5",
		Account: "work", Pool: "b2n8", Reason: "new", Status: 200, Attempts: 1, TotalMS: 100, Usage: json.RawMessage(unknownUsage)}
	fileDay(t, stateDir(t, deps), append([]ledger.Line{moved}, pooledLines...)...)
	fileEvent(t, deps, "2026-10-07", eventLine(runOne, 6, status.Event{At: october(7, 8, 0, 0), Kind: status.EventStarted, Session: sessionM, Account: "work", Pool: "b2n8", Reason: status.ReasonNew}))
	return deps
}

// Seen from the pool it left, each verb's text names an account moved by
// its label and where it is now, never by its id; seen from the pool it's
// in, by its label alone.
func TestTheVerbsNameAnAccountMovedByItsLabelAndWhereItIsNow(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "history", args: []string{"history", "--pretty", "--since", "2026-10-07", "--pool", "b2n8"}, want: movedSaid},
		{name: "requests", args: []string{"requests", "--pretty", "--pool", "b2n8"}, want: " " + movedSaid + " "},
		{name: "sessions", args: []string{"sessions", "--pretty", "--pool", "b2n8"}, want: " " + movedSaid + " "},
		{name: "the session's page", args: []string{"sessions", "--pretty", "--pool", "b2n8", sessionM}, want: movedSaid},
		{name: "events", args: []string{"events", "--pretty", "--pool", "b2n8"}, want: movedSaid},
		{name: "history in Acme", args: []string{"history", "--pretty", "--since", "2026-10-07", "--pool", "q7mt"}, want: "Work"},
		{name: "requests in Acme", args: []string{"requests", "--pretty", "--pool", "q7mt"}, want: " Work "},
		{name: "events in Acme", args: []string{"events", "--pretty", "--pool", "q7mt"}, want: "Work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, movedDeps(t), tt.args...)
			if got.code != 0 || !strings.Contains(got.stdout, tt.want) {
				t.Fatalf("switchboard %s = %+v, want it naming %q", strings.Join(tt.args, " "), got, tt.want)
			}
			if acme := slices.Contains(tt.args, "q7mt"); acme == strings.Contains(got.stdout, "now in") {
				t.Errorf("switchboard %s printed\n%s\nwant work said to be in Acme in Default alone", strings.Join(tt.args, " "), got.stdout)
			}
			if strings.Contains(got.stdout, " work ") {
				t.Errorf("switchboard %s printed\n%s\nwant no account named by its id, work", strings.Join(tt.args, " "), got.stdout)
			}
		})
	}
}

// Seen from the pool it left, history's totals give an account moved a
// column of its own, so every request all accounts count is in a column;
// and its JSON lists it among the accounts moved, with the pool it's in.
func TestHistoryOfThePoolAnAccountLeftGivesItAColumn(t *testing.T) {
	deps := movedDeps(t)
	text := run(t, deps, "history", "--pretty", "--since", "2026-10-07", "--pool", "b2n8").stdout
	i := strings.Index(text, "Over these days")
	if i < 0 {
		t.Fatalf("history printed\n%s\nwant its totals", text)
	}
	head := strings.Fields(strings.SplitN(text[i:], "\n", 2)[0])
	if want := []string{"Over", "these", "days", "Side", "Work", "(now", "in", "Acme)", "all", "accounts"}; !slices.Equal(head, want) {
		t.Errorf("Default's totals are headed %q, want %q", head, want)
	}

	var doc struct {
		Totals struct {
			Accounts []struct {
				Account  string `json:"account"`
				Label    string `json:"label"`
				All      bool   `json:"all"`
				Requests int    `json:"requests"`
			} `json:"accounts"`
		} `json:"totals"`
		Moved []status.MovedAccount `json:"moved"`
	}
	got := run(t, deps, "history", "--json", "--since", "2026-10-07", "--pool", "b2n8")
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("switchboard history --json = %+v: %v", got, err)
	}
	var totals []string
	for _, a := range doc.Totals.Accounts {
		totals = append(totals, strings.Join([]string{a.Account, a.Label, strings.Repeat("r", a.Requests)}, " "))
	}
	if want := []string{"side Side rr", "work Work r", "  rrr"}; !slices.Equal(totals, want) {
		t.Errorf("Default's totals are %q, want %q", totals, want)
	}
	if want := []status.MovedAccount{{ID: "work", Label: "Work", Pool: "q7mt", PoolLabel: "Acme"}}; !slices.Equal(doc.Moved, want) {
		t.Errorf("Default's history gives the accounts moved %+v, want %+v", doc.Moved, want)
	}
}

// status names a session the router still lists on an account moved out of
// the pool read, as it does till the session's next request, by the
// account's label and where it is now, never by its id.
func TestStatusNamesASessionOnAnAccountMovedOutByItsLabelAndWhereItIsNow(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.accounts = pooledAccounts
	srv.writeConfig(t)
	health := pooledHealth(t, srv.deps)
	doc := status.Document{
		GeneratedAt: testNow, Source: status.SourceRouter, Router: status.Health{Healthy: true}, PoolID: "b2n8", PoolLabel: "Default",
		Accounts: []status.Account{{ID: "work", Label: "Work", TokenSet: true, FetchedAt: testNow}, {ID: "personal", Label: "Personal", TokenSet: true, FetchedAt: testNow}},
	}
	onSide := []status.Session{{ID: sessionOne, Assignments: []status.Assignment{
		{Model: "claude-haiku-4-5-20251001", Family: "haiku", Account: "side", Reason: "new", AssignedAt: testNow, LastSeen: testNow},
	}}}
	serveRouter(t, srv.socket(), new(atomic.Int64), map[string]http.HandlerFunc{
		"GET /health":   func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(health) },
		"GET /status":   func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(doc) },
		"GET /sessions": func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(onSide) },
	})

	got := run(t, srv.deps, "status", "--pretty", "--pool", "b2n8")
	if want := "haiku on Side (now in Acme)"; got.code != 0 || !strings.Contains(got.stdout, want) {
		t.Errorf("switchboard status --pool b2n8 = %+v, want it naming %q", got, want)
	}
}
