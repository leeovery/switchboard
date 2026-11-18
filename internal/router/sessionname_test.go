package router_test

import (
	"maps"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/router"
)

// A session's name, as run tells it, is kept with its assignment, in the
// state file and on its line, cleaned and cut as a line keeps it, and never
// sent upstream; one that's missing or doesn't decode names none, and the
// request goes as it would without it.
func TestASessionsNameReachesItsAssignmentTheStateFileAndTheLedgerAndOutlivesARestart(t *testing.T) {
	tests := []struct {
		name string
		// sent is the header's value run sends, "" for none.
		sent, want string
	}{
		{name: "none, the session named by none, as before names were told"},
		{name: "a terminal session's", sent: router.EncodeDir("deploy"), want: "deploy"},
		{name: "past ASCII, as it's encoded", sent: router.EncodeDir("café"), want: "café"},
		{name: "control characters cleaned", sent: router.EncodeDir("fix\x1b[31m\nlogin"), want: "fix [31m login"},
		{name: "longer than a line holds, cut", sent: router.EncodeDir(strings.Repeat("a", 300)), want: strings.Repeat("a", 100)},
		{name: "one that doesn't decode, none", sent: "deploy%zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpstream(t, answerOK)
			cfg := runConfig(t, up.URL)
			stop := runRouter(t, cfg)
			client := router.NewClient(router.SocketPath(cfg.StateDir))
			header := with(with(claudeCode(workToken), router.DirHeader, router.EncodeDir("~/Code/api")), router.NameHeader, tt.sent)
			if answer := postAsking("http://"+cfg.Listen+"/v1/messages", messages, header); !strings.Contains(answer, `"type":"message"`) {
				t.Errorf("the request was answered %q, want the upstream's answer: no name refuses it", answer)
			}
			if got := up.only(t).header.Get(router.NameHeader); got != "" {
				t.Errorf("the upstream was sent the name %q, want none: the header is switchboard's own", got)
			}
			if got := sessionName(t, client); got != tt.want {
				t.Errorf("the session's assignment names it %q, want %q", got, tt.want)
			}
			if err := stop(); err != nil {
				t.Fatalf("Run() = %v", err)
			}
			saved := savedAssignment(t, filepath.Join(cfg.StateDir, "state.json"))
			if got, kept := saved["name"]; got != tt.want || kept != (tt.want != "") {
				t.Errorf("the state file holds the name %q (given: %v), want %q, left out for none", got, kept, tt.want)
			}
			lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
			if len(lines) != 1 || lines[0].Name != tt.want || lines[0].Dir != "~/Code/api" {
				t.Errorf("the ledger holds %+v, want the request's line, named %q, in ~/Code/api", lines, tt.want)
			}

			runRouter(t, cfg)
			if got := sessionName(t, client); got != tt.want {
				t.Errorf("after a restart, the session's assignment names it %q, want %q, as the state file kept it", got, tt.want)
			}
		})
	}
}

// An assignment holds the name its session's last request to name a place
// gave, a directory or a name, so a session resumed outside every terminal
// session is named by none, and a request naming no place keeps the name.
func TestAnAssignmentHoldsTheNameItsSessionsLastRequestToNameAPlaceGave(t *testing.T) {
	up := newUpstream(t, answerOK)
	rt := newRouter(t, up.URL)
	proxy, client := serveProxy(t, rt)+"/v1/messages", router.NewClient(serveControl(t, rt))
	ask := func(dir, name, model string) {
		header := with(claudeCode(workToken), claude.SessionHeader, sessionID)
		header = with(with(header, router.DirHeader, router.EncodeDir(dir)), router.NameHeader, router.EncodeDir(name))
		body := `{"model":"` + model + `","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`
		readAll(t, send(t, http.MethodPost, proxy, header, strings.NewReader(body)))
	}
	steps := []struct {
		about             string
		dir, named, model string
		// want is the name each of the session's models' assignments holds.
		want map[string]string
	}{
		{about: "naming one", dir: "~/Code/api", named: "deploy", model: opus, want: map[string]string{opus: "deploy"}},
		{about: "naming no place, as a claude run didn't start, the name kept", model: opus, want: map[string]string{opus: "deploy"}},
		{about: "resumed in another terminal session", dir: "~/Code/api", named: "review", model: opus, want: map[string]string{opus: "review"}},
		{about: "resumed outside every one, named by none", dir: "~/Code/api", model: opus, want: map[string]string{opus: ""}},
		{about: "a name without a directory", named: "deploy", model: opus, want: map[string]string{opus: "deploy"}},
		{about: "another model's, naming another", dir: "~/Code/api", named: "docs", model: haiku, want: map[string]string{opus: "deploy", haiku: "docs"}},
	}
	for _, step := range steps {
		ask(step.dir, step.named, step.model)
		s, err := client.Session(t.Context(), sessionID)
		if err != nil {
			t.Fatalf("%s: Session() error = %v", step.about, err)
		}
		got := make(map[string]string)
		for _, a := range s.Assignments {
			got[a.Model] = a.SessionName
		}
		if !maps.Equal(got, step.want) {
			t.Errorf("%s: the session's assignments name it %+v, want %+v", step.about, got, step.want)
		}
	}
}

// sessionName is the name the assignment of the session sessionID holds, as
// the router answers the client given.
func sessionName(t *testing.T, client *router.Client) string {
	t.Helper()
	s, err := client.Session(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("Session() error = %v", err)
	}
	return s.Assignments[0].SessionName
}
