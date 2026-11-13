package cli_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
)

// A session run starts in a worktree Claude Code made tells the router its
// directory, and its project, the repository the worktree's .git leads to,
// each from home: the router's stream tells of its request in them, its
// session's assignment holds them, as GET /sessions gives it, and its line in
// the request ledger keeps them.
func TestRunTellsTheProjectThroughTheRouterToTheStreamTheSessionsAndTheLedger(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	stop := srv.start(t)
	home, err := srv.deps.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(home, "Code", "api")
	gitDir, worktree := filepath.Join(repo, ".git", "worktrees", "fix-login"), filepath.Join(repo, ".claude", "worktrees", "fix-login")
	for _, dir := range []string{gitDir, worktree} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, text := range map[string]string{filepath.Join(worktree, ".git"): "gitdir: " + gitDir + "\n", filepath.Join(gitDir, "commondir"): "../..\n"} {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv.deps.Getwd = func() (string, error) { return worktree, nil }
	handed := recordHandOffs(t, &srv.deps)

	if got := run(t, srv.deps, "run"); got != (result{}) {
		t.Fatalf("switchboard run = %+v, want exit status 0 and no output", got)
	}
	headers := handed.only(t).env["ANTHROPIC_CUSTOM_HEADERS"]
	if want := "X-Switchboard-Dir: ~/Code/api/.claude/worktrees/fix-login\nX-Switchboard-Project: ~/Code/api"; headers != want {
		t.Fatalf("handed over with ANTHROPIC_CUSTOM_HEADERS %q, want %q", headers, want)
	}
	want := place{dir: "~/Code/api/.claude/worktrees/fix-login", project: "~/Code/api"}

	stream, err := router.NewClient(srv.socket()).Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	req := srv.messages(t, "worktree-session", "claude-haiku-4-5-20251001", "")
	for line := range strings.Lines(headers) {
		name, value, _ := strings.Cut(strings.TrimSpace(line), ": ")
		req.Header.Set(name, value)
	}
	routed(t, req)
	for e := range stream {
		if got := (place{e.Dir, e.Project}); got != want {
			t.Errorf("the stream told of %s in %+v, want %+v", e.Kind, got, want)
		}
		if e.Kind == router.StreamDone {
			break
		}
	}
	if a := srv.session(t, "worktree-session").Assignments; len(a) != 1 || (place{a[0].Dir, a[0].Project}) != want {
		t.Errorf("the router's session holds %+v, want its assignment in %+v", a, want)
	}
	if got := stop(); got.code != 0 {
		t.Fatalf("switchboard serve = %+v, want exit status 0", got)
	}
	if lines := ledgerLines(t, ledger.Dir(srv.state)); len(lines) != 1 || (place{lines[0].Dir, lines[0].Project}) != want {
		t.Errorf("the ledger holds %+v, want the request's line in %+v", lines, want)
	}
}

// place is where a session ran, as the router holds it: its directory and
// its project.
type place struct {
	dir, project string
}

// routed sends req through the router, failing the test unless it's
// answered 200.
func routed(t *testing.T, req *http.Request) {
	t.Helper()
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

// ledgerLines are the lines of every day's file of the request ledger in
// dir, as written.
func ledgerLines(t *testing.T, dir string) []ledger.Line {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "requests-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []ledger.Line
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			var l ledger.Line
			if err := json.Unmarshal(scanner.Bytes(), &l); err != nil {
				t.Fatalf("%s holds a line that isn't one: %v", file, err)
			}
			lines = append(lines, l)
		}
	}
	return lines
}
