package router_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
)

func TestASessionsDirectoryReachesItsAssignmentTheStreamTheStateFileAndTheLedger(t *testing.T) {
	long := "~/" + strings.Repeat("a", 300) + "/project/sk-ant-oat01-fake_dir-token-shaped"
	tests := []struct {
		name string
		// sent is the directory run tells the router, "" for none.
		sent string
		want string
	}{
		{name: "none"},
		{name: "in the home, shown as ~", sent: "~/Code/my project", want: "~/Code/my project"},
		{
			name: "too long, cut from its front, a token in it hidden", sent: long,
			want: "…" + strings.Repeat("a", 200-len("…/project/")-len(redact.Placeholder)) + "/project/" + redact.Placeholder,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream, arrived, release := holdingUpstream(t)
			cfg := runConfig(t, upstream)
			stop := runRouter(t, cfg)
			client := router.NewClient(router.SocketPath(cfg.StateDir))
			answered := make(chan string, 1)
			header := with(claudeCode(workToken), router.DirHeader, router.EncodeDir(tt.sent))
			go func() { answered <- postAsking("http://"+cfg.Listen+"/v1/messages", messages, header) }()
			<-arrived

			told := toldOfARequest(t, client, release)
			<-answered
			if len(told) == 0 || told[0].Kind != router.StreamInFlight {
				t.Fatalf("the stream told of %+v, want it to open with the request in flight", told)
			}
			for _, e := range told {
				if e.Dir != tt.want {
					t.Errorf("the stream told of %s with the directory %q, want %q", e.Kind, e.Dir, tt.want)
				}
			}
			if got := sessionDir(t, client); got != tt.want {
				t.Errorf("the session's assignment holds the directory %q, want %q", got, tt.want)
			}
			listed, err := client.Sessions(t.Context())
			if err != nil || len(listed) != 1 || listed[0].Assignments[0].Dir != tt.want {
				t.Errorf("Sessions() = %+v, %v, want the session, its assignment's directory %q", listed, err, tt.want)
			}
			if err := stop(); err != nil {
				t.Fatalf("Run() = %v", err)
			}
			if got, kept := savedDir(t, filepath.Join(cfg.StateDir, "state.json")); got != tt.want || kept != (tt.want != "") {
				t.Errorf("the state file holds the directory %q (given: %v), want %q, left out for none", got, kept, tt.want)
			}
			lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
			if len(lines) != 1 || lines[0].Dir != tt.want {
				t.Errorf("the ledger holds %+v, want the request's line, its directory %q, as the session's", lines, tt.want)
			}
		})
	}
}

func TestAnAssignmentHoldsTheDirectoryItsSessionsLastRequestOfItsModelNamed(t *testing.T) {
	up := newUpstream(t, answerOK)
	rt := newRouter(t, up.URL)
	proxy, client := serveProxy(t, rt)+"/v1/messages", router.NewClient(serveControl(t, rt))
	ask := func(dir, model string) {
		header := with(with(claudeCode(workToken), claude.SessionHeader, sessionID), router.DirHeader, router.EncodeDir(dir))
		body := `{"model":"` + model + `","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`
		readAll(t, send(t, http.MethodPost, proxy, header, strings.NewReader(body)))
	}
	steps := []struct {
		name, dir, model string
		// want is the directory of each of the session's models' assignments.
		want map[string]string
	}{
		{name: "naming none", model: opus, want: map[string]string{opus: ""}},
		{name: "naming one", dir: "~/Code/api", model: opus, want: map[string]string{opus: "~/Code/api"}},
		{name: "naming another", dir: "~/Code/web", model: opus, want: map[string]string{opus: "~/Code/web"}},
		{name: "naming none after", model: opus, want: map[string]string{opus: "~/Code/web"}},
		{name: "another model's, naming one", dir: "~/Code/api", model: haiku, want: map[string]string{opus: "~/Code/web", haiku: "~/Code/api"}},
	}
	for _, step := range steps {
		ask(step.dir, step.model)
		s, err := client.Session(t.Context(), sessionID)
		if err != nil {
			t.Fatalf("%s: Session() error = %v", step.name, err)
		}
		got := make(map[string]string)
		for _, a := range s.Assignments {
			got[a.Model] = a.Dir
		}
		if !maps.Equal(got, step.want) {
			t.Errorf("%s: the session's assignments hold the directories %q, want %q", step.name, got, step.want)
		}
	}
}

// toldOfARequest reads the request stream of the router client reaches, as
// it opens with a request in flight, releasing it once it has, until it tells
// of the request done, and returns what it told of it.
func toldOfARequest(t *testing.T, client *router.Client, release func()) []router.StreamEvent {
	t.Helper()
	events, err := client.Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var told []router.StreamEvent
	for e := range events {
		told = append(told, e)
		release()
		if e.Kind == router.StreamDone {
			break
		}
	}
	return told
}

// sessionDir is the directory of the assignment of the session sessionID, as
// the router client reaches gives it.
func sessionDir(t *testing.T, client *router.Client) string {
	t.Helper()
	s, err := client.Session(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("Session() error = %v", err)
	}
	return s.Assignments[0].Dir
}

// savedDir is the directory the state file at path holds of the one
// assignment it keeps, and whether it gives one at all.
func savedDir(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Sessions []map[string]json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal(data, &saved); err != nil || len(saved.Sessions) != 1 {
		t.Fatalf("the state file holds %s (%v), want one assignment", data, err)
	}
	raw, kept := saved.Sessions[0]["dir"]
	if !kept {
		return "", false
	}
	var dir string
	if err := json.Unmarshal(raw, &dir); err != nil {
		t.Fatal(err)
	}
	return dir, true
}
