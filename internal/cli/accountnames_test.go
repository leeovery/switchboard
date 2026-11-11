package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// The accounts the naming tests' history is of: work, configured under an id
// switchboard made, and an account removed on 5 October, kept labelled team.
const (
	labelledID        = "c4rf"
	removedLabelledID = "h7c2"
	// removedSaid is how the verbs' text names the account removed.
	removedSaid = "team (removed 5 Oct)"
)

// namedLines are today's lines of the naming tests: a session started on
// work, then moved onto the account removed as work hit its limit.
var namedLines = []ledger.Line{
	{At: october(7, 9, 0, 0), Request: "n1", Kind: ledger.KindMessage, Session: sessionA, Prompt: "first", Class: ledger.ClassMain, Model: "claude-opus-5-5", Account: labelledID, Reason: "new",
		Status: 200, Attempts: 1, TotalMS: 900, Usage: json.RawMessage(cachedUsage)},
	{At: october(7, 10, 0, 0), Request: "n2", Kind: ledger.KindMessage, Session: sessionA, Prompt: "first", Class: ledger.ClassMain, Model: "claude-opus-5-5", Account: removedLabelledID,
		Reason: "moved: " + labelledID + " hit its limit", From: labelledID, Status: 200, Attempts: 1, TotalMS: 900, Usage: json.RawMessage(cachedUsage)},
}

// namedDeps are deps, by a clock stopped at ledgerNow, whose config
// configures work alone, and whose state directory holds namedLines, a
// reading of each account's 5-hour window, an event of each, and the
// accounts record keeping the account removed.
func namedDeps(t *testing.T) cli.Deps {
	t.Helper()
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte("[[account]]\nid = \""+labelledID+"\"\nlabel = \"work\"\n"))
	state := stateDir(t, deps)
	markMoved(t, state)
	writeStateFile(t, state, "accounts.json", []byte(`{"version":1,"accounts":[`+
		`{"id":"`+removedLabelledID+`","label":"team","added":"2026-08-01T09:00:00Z","removed":"2026-10-05T12:00:00Z"}]}`+"\n"))
	var day bytes.Buffer
	for _, l := range namedLines {
		l.At = l.At.UTC()
		data, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		day.Write(append(data, '\n'))
	}
	writeStateFile(t, ledger.Dir(state), "requests-2026-10-07.jsonl", day.Bytes())
	writeReadings(t, readings.Dir(state),
		readings.Reading{At: october(7, 9, 30, 0), Account: labelledID, Key: "5h", Utilization: 1, ResetsAt: october(7, 14, 0, 0), Status: quota.StatusRejected},
		readings.Reading{At: october(7, 10, 30, 0), Account: removedLabelledID, Key: "5h", Utilization: 0.2, ResetsAt: october(7, 15, 0, 0), Status: quota.StatusAllowed})
	fileEvent(t, deps, "2026-10-07", eventLine(runOne, 1, status.Event{At: october(7, 9, 30, 0), Kind: status.EventLimit, Account: labelledID, Windows: []string{"5h"},
		Until: october(7, 14, 0, 0)}))
	fileEvent(t, deps, "2026-10-07", eventLine(runOne, 2, status.Event{At: october(7, 10, 0, 0), Kind: status.EventMoved, Session: sessionA, Model: "claude-opus-5-5",
		From: labelledID, To: removedLabelledID, Reason: "moved: " + labelledID + " hit its limit"}))
	return deps
}

// Each verb's text names a configured account by its label, and one removed
// by the label the accounts record kept, saying when it was removed: never
// by an id.
func TestTheVerbsTextNamesAccountsByTheirLabels(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "history", args: []string{"history", "--pretty", "--since", "2026-10-07"}, want: []string{"work ", removedSaid + ": "}},
		{name: "history --windows", args: []string{"history", "--windows", "--pretty", "--since", "2026-10-07"}, want: []string{"work  5-hour", removedSaid + "  5-hour"}},
		{name: "requests", args: []string{"requests", "--pretty"}, want: []string{" work ", " " + removedSaid + " "}},
		{name: "sessions", args: []string{"sessions", "--pretty"}, want: []string{" " + removedSaid + " ", "from work"}},
		{name: "the session's page", args: []string{"sessions", "--pretty", sessionA}, want: []string{"work ", removedSaid + " ", "work → " + removedSaid}},
		{name: "events", args: []string{"events", "--pretty"}, want: []string{" work ", removedSaid}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, namedDeps(t), tt.args...)
			if got.code != 0 {
				t.Fatalf("switchboard %s = %+v, want it to print", strings.Join(tt.args, " "), got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stdout, want) {
					t.Errorf("switchboard %s printed\n%s\nwant it naming %q", strings.Join(tt.args, " "), got.stdout, want)
				}
			}
			for _, id := range []string{labelledID, removedLabelledID} {
				if strings.Contains(got.stdout, id) {
					t.Errorf("switchboard %s printed\n%s\nwant no account named by its id, %s", strings.Join(tt.args, " "), got.stdout, id)
				}
			}
		})
	}
}

// Each verb's JSON that gives an account by its id gives its label beside
// it.
func TestTheVerbsJSONGivesEachAccountsLabelBesideItsID(t *testing.T) {
	work, team := `"account":"`+labelledID+`","label":"work"`, `"account":"`+removedLabelledID+`","label":"team"`
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "history", args: []string{"history", "--json", "--since", "2026-10-07"}, want: []string{work}},
		{name: "history --windows", args: []string{"history", "--windows", "--json", "--since", "2026-10-07"}, want: []string{work, team}},
		{name: "sessions", args: []string{"sessions", "--json"}, want: []string{team, `"from":"` + labelledID + `","from_label":"work"`}},
		{name: "the session's page", args: []string{"sessions", "--json", sessionA}, want: []string{work, team,
			`"from":"` + labelledID + `","to":"` + removedLabelledID + `","from_label":"work","to_label":"team"`, `"labels":["work","team"]`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, namedDeps(t), tt.args...)
			var compact bytes.Buffer
			if err := json.Compact(&compact, []byte(got.stdout)); err != nil || got.code != 0 {
				t.Fatalf("switchboard %s = %+v (%v), want its JSON", strings.Join(tt.args, " "), got, err)
			}
			for _, want := range tt.want {
				if !strings.Contains(compact.String(), want) {
					t.Errorf("switchboard %s printed\n%s\nwant it giving %s", strings.Join(tt.args, " "), compact.String(), want)
				}
			}
		})
	}
}
