package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// pooledConfig has work in Acme, fenced, and side in Default, the first
// pool, unfenced: every pool fenced where fenced is set.
func pooledConfig(fenced bool) string {
	dflt := ""
	if fenced {
		dflt = "dirs  = [\"/work/default\"]\n"
	}
	return `[[account]]
id    = "work"
label = "Work"
pool  = "q7mt"

[[account]]
id    = "side"
label = "Side"
pool  = "b2n8"

[[pool]]
id    = "b2n8"
label = "Default"
` + dflt + `
[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["/work/acme"]
`
}

// pooledLines are today's lines in the pools' tests: side's, before pools,
// so the first pool's; work's in Acme; and side's in Default.
var pooledLines = []ledger.Line{
	{At: october(7, 9, 0, 0), Request: "p1", Kind: ledger.KindMessage, Session: sessionA, Model: "claude-opus-5-5", Account: "side", Reason: "new",
		Status: 200, Attempts: 1, TotalMS: 100, Usage: json.RawMessage(unknownUsage)},
	{At: october(7, 10, 0, 0), Request: "p2", Kind: ledger.KindMessage, Session: sessionB, Model: "claude-opus-5-5", Account: "work", Pool: "q7mt", Reason: "new",
		Status: 200, Attempts: 1, TotalMS: 100, Usage: json.RawMessage(unknownUsage)},
	{At: october(7, 11, 0, 0), Request: "p3", Kind: ledger.KindMessage, Session: sessionC, Model: "claude-opus-5-5", Account: "side", Pool: "b2n8", Reason: "new",
		Status: 200, Attempts: 1, TotalMS: 100, Usage: json.RawMessage(unknownUsage)},
}

// pooledEvents are today's events in the pools' tests: a session's start in
// Acme, and one before pools, so the first pool's; a limit of work, in Acme
// now; a pin from before pins named their pool, so the first pool's; and
// the router's restart, every pool's.
var pooledEvents = []filed{
	{date: "2026-10-07", line: eventLine(runOne, 1, status.Event{At: october(7, 9, 0, 0), Kind: status.EventStarted, Session: sessionA, Account: "side", Reason: status.ReasonNew})},
	{date: "2026-10-07", line: eventLine(runOne, 2, status.Event{At: october(7, 10, 0, 0), Kind: status.EventStarted, Session: sessionB, Account: "work", Pool: "q7mt", Reason: status.ReasonNew})},
	{date: "2026-10-07", line: eventLine(runOne, 3, status.Event{At: october(7, 10, 30, 0), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: october(7, 14, 0, 0)})},
	{date: "2026-10-07", line: eventLine(runOne, 4, status.Event{At: october(7, 11, 0, 0), Kind: status.EventPin, Account: "side", Accounts: []string{"side"}, By: "cli"})},
	{date: "2026-10-07", line: eventLine(runOne, 5, status.Event{At: october(7, 12, 0, 0), Kind: status.EventRestart, Reason: status.RestartForConfig})},
}

// pooledDeps are deps whose state directory holds the pools' tests' lines
// and events, the move to pools done with Default the first pool, by a
// clock stopped at ledgerNow, configured as pooledConfig says.
func pooledDeps(t *testing.T, fenced bool) cli.Deps {
	t.Helper()
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(pooledConfig(fenced)))
	state := stateDir(t, deps)
	markMoved(t, state)
	err = identity.NewFile(state).Update(func(r *identity.Record) error {
		r.Pools = []identity.PoolEntry{{ID: "b2n8", Added: testNow, First: true, Here: true}, {ID: "q7mt", Added: testNow, Here: true}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var day bytes.Buffer
	for _, line := range pooledLines {
		line.At = line.At.UTC()
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		day.Write(append(data, '\n'))
	}
	writeStateFile(t, ledger.Dir(state), "requests-2026-10-07.jsonl", day.Bytes())
	for _, f := range pooledEvents {
		fileEvent(t, deps, f.date, f.line)
	}
	return deps
}

// fieldsOf returns the given field of each JSON object a line printed, as a
// string.
func fieldsOf(t *testing.T, got result, field string) []string {
	t.Helper()
	var values []string
	for line := range strings.Lines(got.stdout) {
		var object map[string]any
		if err := json.Unmarshal([]byte(line), &object); err != nil {
			t.Fatalf("%+v printed a line that isn't JSON: %v", got, err)
		}
		value, _ := object[field].(string)
		values = append(values, value)
	}
	return values
}

func TestTheVerbsReadingTheFilesReadThePoolEachRecordedOneFromBeforePoolsTheFirst(t *testing.T) {
	deps := pooledDeps(t, false)
	tests := []struct {
		name  string
		args  []string
		field string
		want  []string
	}{
		{name: "requests, none named: the unfenced pool's, one from before pools among them", args: []string{"requests"}, field: "request", want: []string{"p1", "p3"}},
		{name: "requests in Acme", args: []string{"requests", "--pool", "q7mt"}, field: "request", want: []string{"p2"}},
		{name: "events, none named: a start and a pin from before pools, and the router's own", args: []string{"events"}, field: "kind", want: []string{"started", "pin", "restart"}},
		{name: "events in Acme: a start in it, a limit of an account in it now, and the router's own", args: []string{"events", "--pool", "q7mt"}, field: "kind", want: []string{"started", "limit", "restart"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, deps, append(tt.args, "--json")...)
			if values := fieldsOf(t, got, tt.field); got.code != 0 || !slices.Equal(values, tt.want) {
				t.Errorf("switchboard %s --json printed %s %q (%+v), want %q", strings.Join(tt.args, " "), tt.field, values, got, tt.want)
			}
		})
	}
}

// Following the events of a pool prints those filed after that are of it
// alone, as listing them does.
func TestEventsFollowedInAPoolAreThePoolsAlone(t *testing.T) {
	deps := pooledDeps(t, false)
	stdout, interrupt := follow(t, deps, offATerminal, "events", "-f", "--pool", "q7mt")
	waitForLines(t, stdout, 3)
	for i, e := range []status.Event{
		{At: october(7, 12, 10, 0), Kind: status.EventLimit, Account: "side", Windows: []string{"5h"}, Until: october(7, 14, 0, 0)},
		{At: october(7, 12, 20, 0), Kind: status.EventStarted, Session: sessionC, Account: "work", Pool: "x9z9", Reason: status.ReasonNew},
		{At: october(7, 12, 30, 0), Kind: status.EventCap, Account: "work", Windows: []string{"7d"}, Until: october(9, 14, 0, 0)},
	} {
		fileEvent(t, deps, "2026-10-07", eventLine(runOne, 6+i, e))
	}
	waitForLines(t, stdout, 4)
	if code, stderr := interrupt(); code != 0 || stderr != "" {
		t.Errorf("switchboard events -f --pool q7mt exited %d, printing %q on stderr, once interrupted; want 0 and nothing", code, stderr)
	}
	got := result{stdout: stdout.String()}
	if kinds, want := fieldsOf(t, got, "kind"), []string{"started", "limit", "restart", "cap"}; !slices.Equal(kinds, want) {
		t.Errorf("switchboard events -f --pool q7mt printed the kinds %q, want %q: side's limit is Default's, and a start in a pool since removed none's", kinds, want)
	}
}

// waitForLines waits a few seconds at most for the buffer to hold lines of
// its own, at least as many as given.
func waitForLines(t *testing.T, b *syncBuffer, lines int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(b.String(), "\n") < lines {
		if time.Now().After(deadline) {
			t.Fatalf("printed %q, want %d lines", b.String(), lines)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTheStoredLinesJSONLabelsThePoolEachNames(t *testing.T) {
	deps := pooledDeps(t, false)
	tests := []struct {
		args  []string
		pools []string
	}{
		{args: []string{"requests"}, pools: []string{" ", "b2n8 Default"}},
		{args: []string{"requests", "--pool", "q7mt"}, pools: []string{"q7mt Acme"}},
		{args: []string{"events", "--pool", "q7mt"}, pools: []string{"q7mt Acme", " ", " "}},
	}
	for _, tt := range tests {
		got := run(t, deps, append(tt.args, "--json")...)
		ids, labels := fieldsOf(t, got, "pool"), fieldsOf(t, got, "pool_label")
		var pools []string
		for i := range ids {
			pools = append(pools, ids[i]+" "+labels[i])
		}
		if !slices.Equal(pools, tt.pools) {
			t.Errorf("switchboard %s --json gave the pools %q, want %q: a line naming one labels it, one naming none labels none", strings.Join(tt.args, " "), pools, tt.pools)
		}
		if !strings.HasPrefix(got.stdout, `{"schema":1,`) {
			t.Errorf("switchboard %s --json = %+v, want each line's schema first", strings.Join(tt.args, " "), got)
		}
	}
}

// historyAccounts are the accounts of each day history --json printed, each
// with the pool its work names, and its label.
func historyAccounts(t *testing.T, got result) []string {
	t.Helper()
	var doc struct {
		Days []struct {
			Accounts []struct {
				Account   string `json:"account"`
				Pool      string `json:"pool"`
				PoolLabel string `json:"pool_label"`
				Sessions  int    `json:"sessions"`
			} `json:"accounts"`
		} `json:"days"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("switchboard history --json = %+v: %v", got, err)
	}
	var accounts []string
	for _, d := range doc.Days {
		for _, a := range d.Accounts {
			accounts = append(accounts, strings.Join([]string{a.Account, a.Pool, a.PoolLabel, strings.Repeat("s", a.Sessions)}, " "))
		}
	}
	return accounts
}

func TestHistoryShowsThePoolsWorkAsRecordedAndNamesItsPoolByIDAndLabel(t *testing.T) {
	deps := pooledDeps(t, false)
	tests := []struct {
		args []string
		want []string
	}{
		{args: nil, want: []string{"side b2n8 Default ss"}},
		{args: []string{"--pool", "q7mt"}, want: []string{"work q7mt Acme s"}},
	}
	for _, tt := range tests {
		got := run(t, deps, append([]string{"history", "--json"}, tt.args...)...)
		if accounts := historyAccounts(t, got); !slices.Equal(accounts, tt.want) {
			t.Errorf("switchboard history --json %s gave the accounts %q, want %q", strings.Join(tt.args, " "), accounts, tt.want)
		}
	}
}

// A day of one pool of several leaves out its files' lines and bytes, the
// whole day's, never the pool's.
func TestHistoryOfOnePoolOfSeveralLeavesOutTheDaysFiles(t *testing.T) {
	deps := pooledDeps(t, false)
	for _, args := range [][]string{nil, {"--pool", "q7mt"}} {
		got := run(t, deps, append([]string{"history", "--json"}, args...)...)
		var doc struct {
			Days []map[string]json.RawMessage `json:"days"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || len(doc.Days) == 0 {
			t.Fatalf("switchboard history --json %s = %+v (%v), want its days", strings.Join(args, " "), got, err)
		}
		for _, d := range doc.Days {
			if _, lines := d["lines"]; lines {
				t.Errorf("switchboard history --json %s gave a day its lines, %s, want them left out", strings.Join(args, " "), d["lines"])
			}
			if _, bytes := d["bytes"]; bytes {
				t.Errorf("switchboard history --json %s gave a day its bytes, %s, want them left out", strings.Join(args, " "), d["bytes"])
			}
			if _, day := d["day"]; !day {
				t.Errorf("switchboard history --json %s gave a day without its date: %v", strings.Join(args, " "), d)
			}
		}
	}

	// With one pool, its days are the whole day's, and keep them.
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	onePool := strings.Replace(pooledConfig(false), `pool  = "q7mt"`, `pool  = "b2n8"`, 1)
	onePool = onePool[:strings.LastIndex(onePool, "[[pool]]")]
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(onePool))
	got := run(t, deps, "history", "--json")
	if !strings.Contains(got.stdout, `"lines": 3`) {
		t.Errorf("switchboard history --json with one pool = %+v, want its day's lines", got)
	}
}

// sessionsListed are the ids of the sessions sessions --json listed.
func sessionsListed(t *testing.T, got result) []string {
	t.Helper()
	var list struct {
		Sessions []struct {
			Session string `json:"session"`
		} `json:"sessions"`
		Today struct {
			Sessions int `json:"sessions"`
			Requests int `json:"requests"`
		} `json:"today"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &list); err != nil {
		t.Fatalf("switchboard sessions --json = %+v: %v", got, err)
	}
	var ids []string
	for _, s := range list.Sessions {
		ids = append(ids, status.ShortID(s.Session))
	}
	if list.Today.Sessions != len(ids) || list.Today.Requests != len(ids) {
		t.Errorf("sessions --json summed %+v, want %d sessions of a request each", list.Today, len(ids))
	}
	return ids
}

func TestSessionsListsThePoolsSessionsAlone(t *testing.T) {
	deps := pooledDeps(t, false)
	tests := []struct {
		args []string
		want []string
	}{
		{args: nil, want: []string{status.ShortID(sessionC), status.ShortID(sessionA)}},
		{args: []string{"--pool", "q7mt"}, want: []string{status.ShortID(sessionB)}},
	}
	for _, tt := range tests {
		got := run(t, deps, append([]string{"sessions", "--json"}, tt.args...)...)
		if ids := sessionsListed(t, got); !slices.Equal(ids, tt.want) {
			t.Errorf("switchboard sessions --json %s listed %q, want %q", strings.Join(tt.args, " "), ids, tt.want)
		}
	}
}

// A session with requests in two pools is listed in each with its requests
// there alone, and their worth, and each list sums its own sessions once.
func TestASessionInTwoPoolsIsCountedInEachByItsOwnRequests(t *testing.T) {
	const twoPools = "7c1d2e3f-0004-4a00-8000-000000000004"
	deps := pooledDeps(t, false)
	var lines bytes.Buffer
	for _, l := range []ledger.Line{
		{At: october(7, 11, 30, 0), Request: "p4", Session: twoPools, Account: "side", Pool: "b2n8"},
		{At: october(7, 11, 40, 0), Request: "p5", Session: twoPools, Account: "work", Pool: "q7mt"},
		{At: october(7, 11, 50, 0), Request: "p6", Session: twoPools, Account: "side", Pool: "b2n8"},
	} {
		l.At, l.Kind, l.Model, l.Reason, l.Status, l.Attempts, l.TotalMS, l.Usage = l.At.UTC(), ledger.KindMessage, "claude-opus-5-5", "new", 200, 1, 100, json.RawMessage(hourUsage)
		data, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(append(data, '\n'))
	}
	appendTo(t, filepath.Join(ledger.Dir(stateDir(t, deps)), "requests-2026-10-07.jsonl"), lines.String())

	type listed struct {
		Session  string      `json:"session"`
		Requests int         `json:"requests"`
		Worth    json.Number `json:"worth"`
	}
	tests := []struct {
		args             []string
		requests         int
		worth            string
		sessions, inPool int
	}{
		{args: nil, requests: 2, worth: "0.07608", sessions: 3, inPool: 4},
		{args: []string{"--pool", "q7mt"}, requests: 1, worth: "0.03804", sessions: 2, inPool: 2},
	}
	for _, tt := range tests {
		got := run(t, deps, append([]string{"sessions", "--json"}, tt.args...)...)
		var list struct {
			Sessions []listed `json:"sessions"`
			Today    struct {
				Sessions int `json:"sessions"`
				Requests int `json:"requests"`
			} `json:"today"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &list); err != nil {
			t.Fatalf("switchboard sessions --json %s = %+v: %v", strings.Join(tt.args, " "), got, err)
		}
		i := slices.IndexFunc(list.Sessions, func(s listed) bool { return s.Session == twoPools })
		if i < 0 || list.Sessions[i].Requests != tt.requests || list.Sessions[i].Worth.String() != tt.worth {
			t.Errorf("switchboard sessions --json %s listed %+v, want the session in both pools with %d requests worth %s", strings.Join(tt.args, " "), list.Sessions, tt.requests, tt.worth)
		}
		if list.Today.Sessions != tt.sessions || list.Today.Requests != tt.inPool {
			t.Errorf("switchboard sessions --json %s summed %+v, want %d sessions of %d requests", strings.Join(tt.args, " "), list.Today, tt.sessions, tt.inPool)
		}
	}
}

func TestTheVerbsReadingTheFilesAskForAPoolWhereEveryPoolIsFenced(t *testing.T) {
	deps := pooledDeps(t, true)
	for _, verb := range []string{"requests", "history", "events", "sessions"} {
		t.Run(verb, func(t *testing.T) {
			got := run(t, deps, verb, "--json")
			if got.code == 0 || got.stdout != "" || !strings.Contains(got.stderr, "every pool is fenced: name a pool, b2n8 (Default) or q7mt (Acme), with --pool") {
				t.Errorf("switchboard %s --json = %+v, want it to ask for --pool, naming the pools, and print nothing", verb, got)
			}
			if got := run(t, deps, verb, "--json", "--pool", "q7mt"); got.code != 0 {
				t.Errorf("switchboard %s --json --pool q7mt = %+v, want it to read Acme", verb, got)
			}
		})
	}
}

// --pool "" is --pool not given, on every verb reading the files, as the
// skill passes $SWITCHBOARD_POOL, empty outside a pool.
func TestAPoolGivenEmptyIsNoneGiven(t *testing.T) {
	deps := pooledDeps(t, false)
	for _, args := range [][]string{{"requests"}, {"events"}, {"history"}, {"sessions"}, {"sessions", sessionB}, {"requests", "--session", sessionB}} {
		without, with := run(t, deps, append(args, "--json")...), run(t, deps, append(args, "--json", "--pool", "")...)
		if with.code != 0 || with != without {
			t.Errorf("switchboard %s --json --pool \"\" = %+v, want it as without --pool, %+v", strings.Join(args, " "), with, without)
		}
	}
}

// A session given to requests --session is found in whatever pool it's in,
// or with --pool, in that pool alone, its requests those routed there; one
// given to sessions takes --pool, a pool configured, for the router's
// sessions it's found among, and without the router, shows from the ledger.
func TestASessionGivenIsFoundInWhateverPoolElseInThePoolNamed(t *testing.T) {
	deps := pooledDeps(t, false)
	tests := []struct {
		args []string
		want []string
	}{
		{args: []string{"requests", "--session", sessionB}, want: []string{"p2"}},
		{args: []string{"requests", "--session", sessionB, "--pool", "q7mt"}, want: []string{"p2"}},
		{args: []string{"requests", "--session", sessionB, "--pool", "b2n8"}},
		{args: []string{"requests", "--session", sessionA, "--pool", "b2n8"}, want: []string{"p1"}},
	}
	for _, tt := range tests {
		got := run(t, deps, append(tt.args, "--json")...)
		if requests := fieldsOf(t, got, "request"); got.code != 0 || !slices.Equal(requests, tt.want) {
			t.Errorf("switchboard %s --json printed the requests %q (%+v), want %q", strings.Join(tt.args, " "), requests, got, tt.want)
		}
	}
	for _, pool := range []string{"q7mt", "b2n8"} {
		if got := run(t, deps, "sessions", sessionB, "--pool", pool, "--json"); got.code != 0 || !strings.Contains(got.stdout, sessionB) {
			t.Errorf("switchboard sessions %s --pool %s --json = %+v, want the session's page", status.ShortID(sessionB), pool, got)
		}
	}
	if got := run(t, deps, "sessions", sessionB, "--pool", "x9z9"); got.code == 0 || !strings.Contains(got.stderr, `there's no pool "x9z9": name b2n8 (Default) or q7mt (Acme), with --pool`) {
		t.Errorf("switchboard sessions %s --pool x9z9 = %+v, want it refused, naming the pools", status.ShortID(sessionB), got)
	}
}
