package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// routerDown is what a command that needs the router says when it isn't
// running.
const routerDown = "Error: the router isn't running: start it with switchboard service install (or switchboard serve)\n"

// The sessions the tests route: two whose ids start alike, and another.
const (
	sessionOne   = "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e"
	sessionTwo   = "0b5c9a01-2b3c-4d5e-8f90-a1b2c3d4e5f6"
	sessionThree = "18bb978f-3c2d-4e5f-8a9b-0c1d2e3f4a5b"
)

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

func TestPinForce(t *testing.T) {
	tests := []struct {
		args    []string
		want    string
		wantPin status.Pin
	}{
		{
			args:    []string{"pin", "side", "--force"},
			want:    "new sessions go to side · Side, every session's own pin cleared\n",
			wantPin: status.Pin{Account: "side", Since: testNow},
		},
		{
			args:    []string{"pin", "side", "--move", "--force"},
			want:    "new sessions go to side · Side, and running sessions move on their next request, every session's own pin cleared\n",
			wantPin: status.Pin{Account: "side", Since: testNow, Move: true},
		},
		{
			args: []string{"pin", "auto", "--force"},
			want: "routing automatically, every session's own pin cleared\n",
		},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), nil)
			srv.start(t)
			srv.routePinned(t, sessionOne, "claude-haiku-4-5-20251001", "work")
			if session := srv.session(t, sessionOne); session.Pin != "work" {
				t.Fatalf("the session launched pinned to work has pin %q, want work", session.Pin)
			}

			got := run(t, srv.deps, tt.args...)
			if want := (result{stdout: tt.want}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
			}
			if doc := srv.status(t); doc.Pin != tt.wantPin {
				t.Errorf("the router's pin = %+v, want %+v", doc.Pin, tt.wantPin)
			}
			if session := srv.session(t, sessionOne); session.Pin != "" {
				t.Errorf("the session launched pinned to work has pin %q, want none", session.Pin)
			}
		})
	}
}

func TestPinRefusesAnAccountNothingCanGoOutOn(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	srv.route(t, sessionThree, "claude-haiku-4-5-20251001")
	nope := `Error: there's no account "nope": pin work or side` + "\n"
	personal := "Error: account personal has no usable token, so nothing can go out on it: token missing: write it to " + tokenPath(t, srv.deps, "personal") + "\n"
	tests := [][]string{
		{"pin", "nope"},
		{"pin", "personal"},
		{"pin", "nope", "--session", "18bb"},
		{"pin", "personal", "--session", "18bb"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			want := result{stderr: map[string]string{"nope": nope, "personal": personal}[args[1]], code: 1}
			if got := run(t, srv.deps, args...); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(args, " "), got, want)
			}
		})
	}
	if session := srv.session(t, sessionThree); session.Pin != "" {
		t.Errorf("the session has pin %q, want none", session.Pin)
	}
}

func TestPinSession(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	srv.route(t, sessionOne, "claude-haiku-4-5-20251001")
	srv.route(t, sessionTwo, "claude-haiku-4-5-20251001")
	srv.routePinned(t, sessionThree, "claude-haiku-4-5-20251001", "work")
	tests := []struct {
		args    []string
		want    string
		wantPin string
	}{
		{
			args:    []string{"pin", "side", "--session", "18bb"},
			want:    "session 18bb978f goes to side from its next request\n",
			wantPin: "side",
		},
		{
			args:    []string{"pin", "auto", "--session", sessionThree},
			want:    "session 18bb978f is routed automatically from its next request\n",
			wantPin: "",
		},
		{
			args:    []string{"pin", "work", "--session", "18bb978f"},
			want:    "session 18bb978f goes to work from its next request\n",
			wantPin: "work",
		},
		{
			args:    []string{"pin", "Auto", "--session", "1"},
			want:    "session 18bb978f is routed automatically from its next request\n",
			wantPin: "",
		},
	}
	for _, tt := range tests {
		got := run(t, srv.deps, tt.args...)
		if want := (result{stdout: tt.want}); got != want {
			t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
		}
		if session := srv.session(t, sessionThree); session.Pin != tt.wantPin {
			t.Errorf("after switchboard %s, the session's pin = %q, want %q", strings.Join(tt.args, " "), session.Pin, tt.wantPin)
		}
	}
	if doc := srv.status(t); doc.Pin != (status.Pin{}) {
		t.Errorf("the router's pin = %+v, want none: a session's pin is its own", doc.Pin)
	}
}

func TestPinSessionThatIsntOneAlone(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	srv.route(t, sessionOne, "claude-haiku-4-5-20251001")
	srv.routePinned(t, sessionTwo, "claude-haiku-4-5-20251001", "work")
	ambiguous := `Error: session 0b5c could be any of these, so give more of its id:
  0b5c6f2e  haiku on work  ·  seen just now
  0b5c9a01  haiku on work  ·  pinned to work  ·  seen just now
`
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"pin", "side", "--session", "0b5c"}, want: ambiguous},
		{args: []string{"pin", "auto", "--session", "0b5c"}, want: ambiguous},
		{args: []string{"status", "--session", "0b5c"}, want: ambiguous},
		{args: []string{"pin", "side", "--session", "nope"}, want: "Error: the router hasn't seen session nope\n"},
		{args: []string{"pin", "auto", "--session", "nope"}, want: "Error: the router hasn't seen session nope\n"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			if got, want := run(t, srv.deps, tt.args...), (result{stderr: tt.want, code: 1}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
			}
		})
	}
	for id, want := range map[string]string{sessionOne: "", sessionTwo: "work"} {
		if session := srv.session(t, id); session.Pin != want {
			t.Errorf("session %s has pin %q, want %q, as it had", id, session.Pin, want)
		}
	}
}

func TestPinSessionIdleForAnHourByItsWholeID(t *testing.T) {
	var later atomic.Int64
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.deps.Now = func() time.Time { return testNow.Add(time.Duration(later.Load())) }
	srv.start(t)
	srv.route(t, sessionOne, "claude-haiku-4-5-20251001")
	later.Store(int64(2 * time.Hour))

	if got, want := run(t, srv.deps, "pin", "side", "--session", sessionOne), (result{stdout: "session 0b5c6f2e goes to side from its next request\n"}); got != want {
		t.Errorf("switchboard pin side --session <its whole id> = %+v, want %+v", got, want)
	}
	if got, want := run(t, srv.deps, "pin", "side", "--session", "0b5c"), (result{stderr: "Error: the router hasn't seen session 0b5c\n", code: 1}); got != want {
		t.Errorf("switchboard pin side --session <part of its id> = %+v, want %+v: parts match the sessions of the last hour", got, want)
	}
}

func TestCommandsThatNeedTheRouterWithoutIt(t *testing.T) {
	for _, args := range [][]string{
		{"pin", "side"},
		{"pin", "side", "--move"},
		{"pin", "side", "--force"},
		{"pin", "auto"},
		{"pin", "auto", "--force"},
		{"pin", "side", "--session", "0b5c"},
		{"pin", "auto", "--session", "0b5c"},
		{"status", "--session", sessionOne},
		{"status", "--session", "0b5c"},
		{"status", "--session", sessionOne, "--json"},
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
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	srv.route(t, sessionOne, "claude-haiku-4-5-20251001")
	srv.route(t, sessionThree, "claude-haiku-4-5-20251001")

	for _, given := range []string{sessionOne, "0b5c6", "0"} {
		if got, want := run(t, srv.deps, "status", "--session", given), (result{stdout: "work\n"}); got != want {
			t.Errorf("switchboard status --session %s = %+v, want %+v", given, got, want)
		}
	}

	got := run(t, srv.deps, "status", "--session", "0b5c", "--json")
	var session status.Session
	if got.code != 0 || got.stderr != "" || json.Unmarshal([]byte(got.stdout), &session) != nil {
		t.Fatalf("switchboard status --session --json = %+v, want exit status 0 and JSON alone", got)
	}
	wantAssignments := []status.Assignment{
		{Model: "claude-haiku-4-5-20251001", Family: "haiku", Account: "work", Reason: "new", AssignedAt: testNow, LastSeen: testNow},
	}
	if session.ID != sessionOne || !reflect.DeepEqual(session.Assignments, wantAssignments) || session.Account.ID != "work" || session.Account.Sessions != 2 {
		t.Errorf("switchboard status --session --json printed\n%+v\nwant the session, its assignment %+v, and work's status with its sessions", session, wantAssignments)
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

// session returns what the router says of the session with the given id.
func (s *serveSetup) session(t *testing.T, id string) status.Session {
	t.Helper()
	session, err := router.NewClient(s.socket()).Session(t.Context(), id)
	if err != nil {
		t.Fatalf("Session() error = %v", err)
	}
	return session
}

// route sends a messages request of the session given, for model, through
// the router on work's token, as Claude Code does.
func (s *serveSetup) route(t *testing.T, session, model string) {
	t.Helper()
	s.routePinned(t, session, model, "")
}

// routePinned sends a messages request of the session given, for model,
// through the router on work's token, as Claude Code launched pinned to pin
// does, or as one launched without a pin does when pin is "".
func (s *serveSetup) routePinned(t *testing.T, session, model, pin string) {
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
	if pin != "" {
		req.Header.Set(router.PinHeader, pin)
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
