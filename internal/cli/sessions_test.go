package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// The sessions of sessions' tests.
const (
	// asking started yesterday, and asks now, on work.
	asking = "1a2b3c4d-0001-4a00-8000-000000000001"
	// moving started today on work, and moved to side at 11:00, as work was
	// at its reserve.
	moving = "5e6f7a8b-0002-4a00-8000-000000000002"
	// ended ended at 08:30, its last request of a model the price table
	// doesn't know.
	ended = "9c0d1e2f-0003-4a00-8000-000000000003"
)

// hourUsage writes the cache for an hour: worth $0.03804 at Claude Opus
// 5.5's prices, its prompt of 101,010 tokens $0.80808 to write again, its
// own write $0.008.
const hourUsage = `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
	`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":1000},"output_tokens":500}`

// sessionLines are sessions' tests' lines, by the day each arrived on.
var sessionLines = map[string][]ledger.Line{
	"2026-10-06": {
		{At: october(6, 22, 0, 0), Request: "s1", Kind: ledger.KindMessage, Session: asking, Dir: "~/Code/api", Model: "claude-opus-5-5", Account: "work",
			Reason: "new", Status: 200, Attempts: 1, TotalMS: 9000, Usage: json.RawMessage(hourUsage)},
	},
	"2026-10-07": {
		{At: october(7, 8, 0, 0), Request: "s2", Kind: ledger.KindMessage, Session: ended, Dir: "~/Code/web", Model: "claude-opus-5-5", Account: "work",
			Reason: "new", Status: 200, Attempts: 1, TotalMS: 9000, Usage: json.RawMessage(hourUsage)},
		{At: october(7, 8, 30, 0), Request: "s3", Kind: ledger.KindMessage, Session: ended, Model: "claude-opus-9", Account: "work",
			Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 900, Usage: json.RawMessage(unknownUsage)},
		{At: october(7, 9, 0, 0), Request: "s4", Kind: ledger.KindMessage, Session: asking, Dir: "~/Code/api", Model: "claude-opus-5-5", Account: "work",
			Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 9000, Usage: json.RawMessage(hourUsage)},
		{At: october(7, 10, 15, 0), Request: "s5", Kind: ledger.KindMessage, Session: moving, Dir: "~/Code/cli", Model: "claude-opus-5-5", Account: "work",
			Reason: "new", Status: 200, Attempts: 1, TotalMS: 9000, Usage: json.RawMessage(hourUsage)},
		{At: october(7, 11, 0, 0), Request: "s6", Kind: ledger.KindMessage, Session: moving, Dir: "~/Code/cli", Model: "claude-opus-5-5", Account: "side",
			Reason: "moved: work is at its reserve", From: "work", Status: 200, Attempts: 1, TotalMS: 9000, Usage: json.RawMessage(hourUsage)},
		{At: october(7, 11, 30, 0), Request: "s7", Kind: ledger.KindCheck, Session: moving, Model: "claude-haiku-4-5", Account: "side",
			Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 600, Usage: json.RawMessage(checkUsage)},
	},
}

// routedSessions are the sessions the router lists in sessions' tests.
var routedSessions = []status.Session{
	{ID: asking, Assignments: []status.Assignment{{Model: "claude-opus-5-5", Family: "opus", Account: "work", InFlight: status.Asking, Reason: "sticky",
		AssignedAt: october(6, 22, 0, 0).UTC(), LastSeen: october(7, 13, 10, 0).UTC()}}},
	{ID: moving, Assignments: []status.Assignment{{Model: "claude-opus-5-5", Family: "opus", Account: "side", Dir: "~/Code/cli",
		Reason: "moved: work is at its reserve", AssignedAt: october(7, 11, 0, 0).UTC(), LastSeen: october(7, 12, 50, 0).UTC()}}},
}

// sessionsDeps are deps whose state directory, short enough to hold the
// router's control socket, holds sessions' tests' lines, by a clock stopped
// at ledgerNow, configured as ledgerConfig says.
func sessionsDeps(t *testing.T) cli.Deps {
	t.Helper()
	return sessionsDepsOf(t, sessionLines)
}

// sessionsDepsOf are deps as sessionsDeps says, their state directory
// holding days' lines, by the date each arrived on, in place of sessions'
// tests'.
func sessionsDepsOf(t *testing.T, days map[string][]ledger.Line) cli.Deps {
	t.Helper()
	stateHome, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateHome) })
	deps := testDeps(map[string]string{"XDG_STATE_HOME": stateHome}, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	configure(t, deps)
	dir := ledger.Dir(stateDir(t, deps))
	for date, lines := range days {
		var day bytes.Buffer
		for _, line := range lines {
			line.At = line.At.UTC()
			data, err := json.Marshal(line)
			if err != nil {
				t.Fatal(err)
			}
			day.Write(append(data, '\n'))
		}
		writeStateFile(t, dir, "requests-"+date+".jsonl", day.Bytes())
	}
	return deps
}

// serveSessions answers GET /sessions with sessions on the control socket of
// the router commands run with deps find, until the test ends.
func serveSessions(t *testing.T, deps cli.Deps, sessions []status.Session) {
	t.Helper()
	path := router.SocketPath(stateDir(t, deps))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(sessions)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

// sessionsText is what sessions prints of its tests' sessions, the router
// listing those running: asking, then moving, which started after it, then
// ended.
const sessionsText = `   session              on    model     state        started          requests  worth   routing
↑  1a2b3c4d ~/Code/api  work  opus-5-5  asking       yesterday 22:00  1         $0.04
▸  5e6f7a8b ~/Code/cli  side  opus-5-5  idle 22m     10:15            2         $0.08   from work, at its cap  $0.01
   9c0d1e2f ~/Code/web  work  opus-9    ended 08:30  08:00            2         $0.04+

today  3 sessions  ·  5 requests  ·  $0.15+ at API prices
`

func TestSessionsListsTodaysSessions(t *testing.T) {
	deps := sessionsDeps(t)
	serveSessions(t, deps, routedSessions)
	for _, form := range prettyForms() {
		t.Run(form.name, func(t *testing.T) {
			if got := form.run(t, deps, "sessions"); got != (result{stdout: sessionsText}) {
				t.Errorf("switchboard sessions %s = %+v, want it to print\n%s", strings.Join(form.args, " "), got, sessionsText)
			}
		})
	}
}

func TestSessionsSaysASessionItsOwnPinPutOnItsAccountIsPinnedThere(t *testing.T) {
	deps := sessionsDeps(t)
	pinned := status.Session{ID: asking, Pin: "work", Assignments: []status.Assignment{{Model: "claude-opus-5-5", Account: "work", Pinned: true,
		Reason: status.ReasonPinned, LastSeen: october(7, 13, 11, 30).UTC()}}}
	serveSessions(t, deps, []status.Session{pinned})

	got := run(t, deps, "sessions", "--pretty")
	const want = "   1a2b3c4d ~/Code/api  work  opus-5-5  idle 30s     yesterday 22:00  1         $0.04   pinned here\n"
	if got.code != 0 || !strings.Contains(got.stdout, want) {
		t.Errorf("switchboard sessions --pretty = %+v, want the row\n%s", got, want)
	}
}

func TestSessionsJSONHoldsItsShape(t *testing.T) {
	deps := sessionsDeps(t)
	serveSessions(t, deps, routedSessions)
	// at is t as the JSON gives it.
	at := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }
	want := `{"generated_at":"` + at(ledgerNow) + `","prices_as_of":"2026-10-07","sessions":[` +
		`{"session":"` + asking + `","dir":"~/Code/api","account":"work","model":"claude-opus-5-5","running":true,"last_seen":"` + at(october(7, 13, 10, 0)) + `",` +
		`"models":[{"model":"claude-opus-5-5","account":"work","reason":"sticky"}],"state":"asking","move_cost":0.80808,` +
		`"started":"` + at(october(6, 22, 0, 0)) + `","requests":1,"worth":0.03804},` +
		`{"session":"` + moving + `","dir":"~/Code/cli","account":"side","model":"claude-opus-5-5","running":true,"last_seen":"` + at(october(7, 12, 50, 0)) + `",` +
		`"models":[{"model":"claude-opus-5-5","account":"side","reason":"moved: work is at its reserve"}],` +
		`"started":"` + at(october(7, 10, 15, 0)) + `","requests":2,"worth":0.07608,` +
		`"moved":{"at":"` + at(october(7, 11, 0, 0)) + `","from":"work","reason":"moved: work is at its reserve","cost":0.008}},` +
		`{"session":"` + ended + `","dir":"~/Code/web","account":"work","model":"claude-opus-9","ended":"` + at(october(7, 8, 30, 0)) + `",` +
		`"started":"` + at(october(7, 8, 0, 0)) + `","requests":2,"worth":0.03804,"unpriced":["input_tokens","output_tokens"]}],` +
		`"today":{"sessions":3,"requests":5,"worth":0.15216,"unpriced":["input_tokens","output_tokens"]}}`
	for _, form := range jsonForms() {
		t.Run(form.name, func(t *testing.T) {
			got := form.run(t, deps, "sessions")
			var compact bytes.Buffer
			if err := json.Compact(&compact, []byte(got.stdout)); err != nil || got.code != 0 || got.stderr != "" ||
				!strings.HasPrefix(got.stdout, "{\n  \"generated_at\"") {
				t.Fatalf("switchboard sessions %s = %+v (%v), want indented JSON", strings.Join(form.args, " "), got, err)
			}
			if compact.String() != want {
				t.Errorf("switchboard sessions %s printed\n%s\nwant\n%s", strings.Join(form.args, " "), compact.String(), want)
			}
		})
	}
}

func TestSessionsWithoutTheRouterListsTheLedgersAloneNoneRunning(t *testing.T) {
	deps := sessionsDeps(t)
	const notice = "switchboard: the router isn't running — listing today's sessions from the ledger alone, none as running\n"
	const want = `   session              on    model     state        started          requests  worth   routing
▸  5e6f7a8b ~/Code/cli  side  opus-5-5  ended 11:00  10:15            2         $0.08   from work, at its cap  $0.01
   1a2b3c4d ~/Code/api  work  opus-5-5  ended 09:00  yesterday 22:00  1         $0.04
   9c0d1e2f ~/Code/web  work  opus-9    ended 08:30  08:00            2         $0.04+

today  3 sessions  ·  5 requests  ·  $0.15+ at API prices
`
	if got := run(t, deps, "sessions", "--pretty"); got != (result{stdout: want, stderr: notice}) {
		t.Errorf("switchboard sessions --pretty = %+v, want it to print\n%s\nand say on stderr\n%s", got, want, notice)
	}

	got := run(t, deps, "sessions", "--json")
	var list struct {
		Sessions []struct {
			Session  string          `json:"session"`
			Running  json.RawMessage `json:"running"`
			LastSeen json.RawMessage `json:"last_seen"`
			Models   json.RawMessage `json:"models"`
			MoveCost json.RawMessage `json:"move_cost"`
			Ended    string          `json:"ended"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &list); err != nil || got.code != 0 || got.stderr != notice || len(list.Sessions) != 3 {
		t.Fatalf("switchboard sessions --json = %+v (%v), want three sessions, and the notice", got, err)
	}
	for _, s := range list.Sessions {
		if s.Running != nil || s.LastSeen != nil || s.Models != nil || s.MoveCost != nil || s.Ended == "" {
			t.Errorf("without the router, session %s is %+v, want it ended, with nothing the router says", s.Session, s)
		}
	}
}

func TestSessionsNeedsNeitherTheRouterNorTheLedger(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	configure(t, deps)
	const notice = "switchboard: the router isn't running — listing today's sessions from the ledger alone, none as running\n"
	if got := run(t, deps, "sessions", "--pretty"); got != (result{stdout: "no sessions today\n", stderr: notice}) {
		t.Errorf("switchboard sessions --pretty = %+v, want no sessions, and the notice", got)
	}
	got := run(t, deps, "sessions", "--json")
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(got.stdout)); err != nil || got.code != 0 || got.stderr != notice {
		t.Fatalf("switchboard sessions --json = %+v (%v), want JSON, and the notice", got, err)
	}
	want := `{"generated_at":"` + ledgerNow.UTC().Format(time.RFC3339) + `","prices_as_of":"2026-10-07","sessions":[],"today":{"sessions":0,"requests":0,"worth":0}}`
	if compact.String() != want {
		t.Errorf("switchboard sessions --json printed\n%s\nwant\n%s", compact.String(), want)
	}
}

func TestSessionsFailsWithoutAStateDirectory(t *testing.T) {
	path := writeConfig(t, ledgerConfig)
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir())
	deps.HomeDir = func() (string, error) { return "", errors.New("no home directory") }
	for _, form := range append(prettyForms(), jsonForms()...) {
		t.Run(form.name, func(t *testing.T) {
			got := form.run(t, deps, "sessions")
			if want := (result{stderr: "Error: locate state directory: no home directory\n", code: 1}); got != want {
				t.Errorf("switchboard sessions %s = %+v, want %+v", strings.Join(form.args, " "), got, want)
			}
		})
	}
}
