package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// routerDown is what a command that needs the router says when it isn't
// running.
const routerDown = "Error: the router isn't running: start it with switchboard service install (or switchboard serve)\n"

func TestPin(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	tests := []struct {
		args    []string
		want    string
		wantPin status.Pin
	}{
		{
			args:    []string{"pin", "side"},
			want:    "new sessions go to side · Side\n",
			wantPin: status.Pin{Account: "side", Since: testNow},
		},
		{
			args:    []string{"pin", "work", "--move"},
			want:    "new sessions go to work · Work, and running sessions move on their next request\n",
			wantPin: status.Pin{Account: "work", Since: testNow, Move: true},
		},
		{
			args: []string{"pin", "auto"},
			want: "routing automatically\n",
		},
		{
			args:    []string{"pin", "side"},
			want:    "new sessions go to side · Side\n",
			wantPin: status.Pin{Account: "side", Since: testNow},
		},
		{
			args: []string{"pin", "AUTO"},
			want: "routing automatically\n",
		},
	}
	for _, tt := range tests {
		got := run(t, srv.deps, tt.args...)
		if want := (result{stdout: tt.want}); got != want {
			t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
		}
		if doc := srv.status(t); doc.Pin != tt.wantPin {
			t.Errorf("after switchboard %s, the router's pin = %+v, want %+v", strings.Join(tt.args, " "), doc.Pin, tt.wantPin)
		}
	}
}

func TestPinRefusesAnAccountNothingCanGoOutOn(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	tests := []struct {
		account string
		want    string
	}{
		{account: "nope", want: `Error: there's no account "nope": pin work or side` + "\n"},
		{account: "personal", want: "Error: account personal has no token, so nothing can go out on it: set CLAUDE_TOKEN_PERSONAL\n"},
	}
	for _, tt := range tests {
		t.Run(tt.account, func(t *testing.T) {
			got := run(t, srv.deps, "pin", tt.account)
			if want := (result{stderr: tt.want, code: 1}); got != want {
				t.Errorf("switchboard pin %s = %+v, want %+v", tt.account, got, want)
			}
		})
	}
}

func TestCommandsThatNeedTheRouterWithoutIt(t *testing.T) {
	for _, args := range [][]string{
		{"pin", "side"},
		{"pin", "side", "--move"},
		{"pin", "auto"},
		{"status", "--session", "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e"},
		{"status", "--session", "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), nil)

			got := run(t, srv.deps, args...)
			if want := (result{stderr: routerDown, code: 1}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(args, " "), got, want)
			}
		})
	}
}

func TestStatusSession(t *testing.T) {
	const id = "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e"
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	srv.route(t, id, "claude-haiku-4-5-20251001")

	got := run(t, srv.deps, "status", "--session", id)
	want := result{stdout: `work · Work
  Session     23%  resets in 4h 58m · Mon 18:10
  Week        93%  resets in 4d 7h · Fri 21:00 · runs out ~Mon 18:01
  Fable week 100%  resets in 5d 11h · Sun 01:10 · exhausted
`}
	if got != want {
		t.Errorf("switchboard status --session =\n%+v\nwant\n%+v", got, want)
	}

	got = run(t, srv.deps, "status", "--session", id, "--json")
	var session router.Session
	if got.code != 0 || got.stderr != "" || json.Unmarshal([]byte(got.stdout), &session) != nil {
		t.Fatalf("switchboard status --session --json = %+v, want exit status 0 and JSON alone", got)
	}
	wantAssignments := []router.Assignment{{Model: "claude-haiku-4-5-20251001", Account: "work", Reason: "new", AssignedAt: testNow, LastSeen: testNow}}
	if session.ID != id || !reflect.DeepEqual(session.Assignments, wantAssignments) || session.Account.ID != "work" || session.Account.Sessions != 1 {
		t.Errorf("switchboard status --session --json printed\n%+v\nwant the session, its assignment %+v, and work's status with its session", session, wantAssignments)
	}
}

func TestStatusSessionTheRouterHasntSeen(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)

	got := run(t, srv.deps, "status", "--session", "nope")
	if want := (result{stderr: "Error: the router hasn't seen session nope\n", code: 1}); got != want {
		t.Errorf("switchboard status --session nope = %+v, want %+v", got, want)
	}
}

// status returns the router's status document.
func (s *serveSetup) status(t *testing.T) status.Document {
	t.Helper()
	doc, err := router.NewClient(s.socket()).Status(t.Context())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	return doc
}

// route sends a messages request of the session given, for model, through
// the router on work's token, as Claude Code does.
func (s *serveSetup) route(t *testing.T, session, model string) {
	t.Helper()
	body := `{"model":"` + model + `","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+s.listen+"/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header = http.Header{
		"Authorization":            {"Bearer test-token-work"},
		"Content-Type":             {"application/json"},
		"User-Agent":               {"claude-code/" + testClaudeVersion},
		"X-Claude-Code-Session-Id": {session},
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/messages through the router: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/messages through the router answered %d (%v), want 200", resp.StatusCode, err)
	}
}
