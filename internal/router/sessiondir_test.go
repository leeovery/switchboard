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

// place is where a session ran, as the router holds it: its directory and
// its project.
type place struct {
	dir, project string
}

func TestASessionsDirectoryAndProjectReachItsAssignmentTheStreamTheStateFileAndTheLedger(t *testing.T) {
	long := "~/" + strings.Repeat("a", 300) + "/project/sk-ant-oat01-fake_dir-token-shaped"
	cut := "…" + strings.Repeat("a", 200-len("…/project/")-len(redact.Placeholder)) + "/project/" + redact.Placeholder
	tests := []struct {
		name string
		// dir and project are the headers' values run sends, "" for none.
		dir, project string
		want         place
	}{
		{name: "none"},
		{
			name: "in the home, shown as ~", dir: router.EncodeDir("~/Code/my project/web"), project: router.EncodeDir("~/Code/my project"),
			want: place{dir: "~/Code/my project/web", project: "~/Code/my project"},
		},
		{
			name: "a worktree's, its repository's", dir: "~/Code/api/.claude/worktrees/fix-login", project: "~/Code/api",
			want: place{dir: "~/Code/api/.claude/worktrees/fix-login", project: "~/Code/api"},
		},
		{
			name: "too long, the directory cut from its front, the project none, a token in each hidden", dir: router.EncodeDir(long), project: router.EncodeDir(long),
			want: place{dir: cut},
		},
		{name: "a project that doesn't decode, none", dir: "~/Code/api", project: "~/Code/%zz", want: place{dir: "~/Code/api"}},
		{name: "a project without a directory", project: "~/Code/api", want: place{project: "~/Code/api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream, arrived, release := holdingUpstream(t)
			cfg := runConfig(t, upstream)
			stop := runRouter(t, cfg)
			client := router.NewClient(router.SocketPath(cfg.StateDir))
			answered := make(chan string, 1)
			header := with(with(claudeCode(workToken), router.DirHeader, tt.dir), router.ProjectHeader, tt.project)
			go func() { answered <- postAsking("http://"+cfg.Listen+"/v1/messages", messages, header) }()
			<-arrived

			told := toldOfARequest(t, client, release)
			if answer := <-answered; !strings.Contains(answer, `"type":"message"`) {
				t.Errorf("the request was answered %q, want the upstream's answer: no header refuses it", answer)
			}
			if len(told) == 0 || told[0].Kind != router.StreamInFlight {
				t.Fatalf("the stream told of %+v, want it to open with the request in flight", told)
			}
			for _, e := range told {
				if got := (place{e.Dir, e.Project}); got != tt.want {
					t.Errorf("the stream told of %s in %+v, want %+v", e.Kind, got, tt.want)
				}
			}
			if got := sessionPlace(t, client); got != tt.want {
				t.Errorf("the session's assignment holds %+v, want %+v", got, tt.want)
			}
			listed, err := client.Sessions(t.Context())
			if err != nil || len(listed) != 1 || (place{listed[0].Assignments[0].Dir, listed[0].Assignments[0].Project}) != tt.want {
				t.Errorf("Sessions() = %+v, %v, want the session, its assignment in %+v", listed, err, tt.want)
			}
			if err := stop(); err != nil {
				t.Fatalf("Run() = %v", err)
			}
			saved := savedAssignment(t, filepath.Join(cfg.StateDir, "state.json"))
			for key, want := range map[string]string{"dir": tt.want.dir, "project": tt.want.project} {
				if got, kept := saved[key]; got != want || kept != (want != "") {
					t.Errorf("the state file holds the %s %q (given: %v), want %q, left out for none", key, got, kept, want)
				}
			}
			lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
			if len(lines) != 1 || (place{lines[0].Dir, lines[0].Project}) != tt.want {
				t.Errorf("the ledger holds %+v, want the request's line, in %+v, as the session is", lines, tt.want)
			}
		})
	}
}

func TestAnAssignmentHoldsThePlaceItsSessionsLastRequestOfItsModelNamed(t *testing.T) {
	up := newUpstream(t, answerOK)
	rt := newRouter(t, up.URL)
	proxy, client := serveProxy(t, rt)+"/v1/messages", router.NewClient(serveControl(t, rt))
	ask := func(at place, model string) {
		header := with(claudeCode(workToken), claude.SessionHeader, sessionID)
		header = with(with(header, router.DirHeader, router.EncodeDir(at.dir)), router.ProjectHeader, router.EncodeDir(at.project))
		body := `{"model":"` + model + `","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`
		readAll(t, send(t, http.MethodPost, proxy, header, strings.NewReader(body)))
	}
	api, web := place{dir: "~/Code/api/cmd", project: "~/Code/api"}, place{dir: "~/Code/web", project: "~/Code/web"}
	steps := []struct {
		name  string
		at    place
		model string
		// want is where each of the session's models' assignments holds it
		// ran.
		want map[string]place
	}{
		{name: "naming none", model: opus, want: map[string]place{opus: {}}},
		{name: "naming one", at: api, model: opus, want: map[string]place{opus: api}},
		{name: "naming another", at: web, model: opus, want: map[string]place{opus: web}},
		{name: "naming none after", model: opus, want: map[string]place{opus: web}},
		{
			name: "a directory alone, as a run from before told, the project with it none", at: place{dir: "~/Code/site"}, model: opus,
			want: map[string]place{opus: {dir: "~/Code/site"}},
		},
		{
			name: "a project alone, the directory kept", at: place{project: "~/Code/api"}, model: opus,
			want: map[string]place{opus: {dir: "~/Code/site", project: "~/Code/api"}},
		},
		{
			name: "another model's, naming one", at: api, model: haiku,
			want: map[string]place{opus: {dir: "~/Code/site", project: "~/Code/api"}, haiku: api},
		},
	}
	for _, step := range steps {
		ask(step.at, step.model)
		s, err := client.Session(t.Context(), sessionID)
		if err != nil {
			t.Fatalf("%s: Session() error = %v", step.name, err)
		}
		got := make(map[string]place)
		for _, a := range s.Assignments {
			got[a.Model] = place{a.Dir, a.Project}
		}
		if !maps.Equal(got, step.want) {
			t.Errorf("%s: the session's assignments hold %+v, want %+v", step.name, got, step.want)
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

// sessionPlace is where the assignment of the session sessionID holds it
// ran, as the router client reaches gives it.
func sessionPlace(t *testing.T, client *router.Client) place {
	t.Helper()
	s, err := client.Session(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("Session() error = %v", err)
	}
	return place{s.Assignments[0].Dir, s.Assignments[0].Project}
}

// savedAssignment is what the state file at path holds of the one
// assignment it keeps, each of its texts by its key.
func savedAssignment(t *testing.T, path string) map[string]string {
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
	texts := make(map[string]string)
	for key, raw := range saved.Sessions[0] {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			texts[key] = text
		}
	}
	return texts
}
