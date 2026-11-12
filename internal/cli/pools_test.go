package cli_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

// fencedAccounts are threeAccounts in two pools, each fenced: work in
// Default, personal and side in Acme.
const fencedAccounts = `
[[account]]
id    = "work"
label = "Work"
pool  = "b2n8"

[[account]]
id    = "personal"
label = "Personal"
pool  = "q7mt"

[[account]]
id    = "side"
label = "Side"
pool  = "q7mt"

[[pool]]
id    = "b2n8"
label = "Default"
dirs  = ["/work/default"]

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["/work/acme"]
`

// routeInAcme sends a messages request of the session given through the
// router, as one started in Acme does.
func (s *serveSetup) routeInAcme(t *testing.T, session string) {
	t.Helper()
	req := s.messages(t, session, "claude-haiku-4-5-20251001", "")
	req.Header.Set(router.PoolHeader, "q7mt")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST /v1/messages through the router: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/messages through the router answered %d, want 200", resp.StatusCode)
	}
}

func TestASessionIsFoundByAPrefixInAFencedPoolWithEveryPoolFenced(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.accounts = fencedAccounts
	srv.writeConfig(t)
	srv.start(t)
	srv.routeInAcme(t, sessionThree)

	if got, want := run(t, srv.deps, "status", "--session", "18bb"), (result{stdout: "side\n"}); got != want {
		t.Errorf("switchboard status --session 18bb = %+v, want %+v", got, want)
	}
	want := result{stdout: "session 18bb978f goes to side from its next request\n"}
	if got := run(t, srv.deps, "pin", "side", "--session", "18bb"); got != want {
		t.Errorf("switchboard pin side --session 18bb = %+v, want %+v", got, want)
	}
}

func TestPinningAFencedPoolsAccountSaysWhereItsNewSessionsGo(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.accounts = fencedAccounts
	srv.writeConfig(t)
	srv.start(t)

	if got, want := run(t, srv.deps, "pin", "side"), (result{stdout: "new sessions go to side\n"}); got != want {
		t.Errorf("switchboard pin side = %+v, want %+v", got, want)
	}
}

func TestReadingTheRouterWithEveryPoolFencedNeedsAPoolAndProbesNothing(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.accounts = fencedAccounts
	srv.writeConfig(t)
	srv.start(t)

	got := run(t, srv.deps, "usage", "--json")
	if got.code == 0 || !strings.Contains(got.stderr, "every pool is fenced") || got.stdout != "" {
		t.Errorf("switchboard usage --json = %+v, want it to need a pool, and print nothing", got)
	}
}
