package router_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

func TestRun(t *testing.T) {
	log := logstest.Capture(t)
	up := newUpstream(t, answerOK)
	cfg := runConfig(t, up.URL)
	fableDown := quota.Failure{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}
	prober := &fakeProber{readings: map[string]probeResult{
		workToken: {usage: quota.Usage{Windows: []quota.Window{session, week}, Failures: []quota.Failure{fableDown}}},
		sideToken: {err: errors.New("HTTP 401 · Invalid bearer token")},
	}}
	cfg.Prober = prober
	// A state directory others can read, which the router makes private.
	if err := os.Mkdir(cfg.StateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stop := runRouter(t, cfg)
	socket := router.SocketPath(cfg.StateDir)

	for path, want := range map[string]fs.FileMode{cfg.StateDir: fs.ModeDir | 0o700, socket: fs.ModeSocket | 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode() != want {
			t.Errorf("%s has mode %v, want %v", path, info.Mode(), want)
		}
	}
	resp := send(t, http.MethodPost, "http://"+cfg.Listen+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("a request through the proxy answered %d, want 200", resp.StatusCode)
	}
	probed := waitForStatus(t, socket, func(doc status.Document) bool {
		work, _ := doc.Account("work")
		side, _ := doc.Account("side")
		return len(work.Windows) > 0 && side.Error != ""
	})
	want := []status.Account{
		{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now, Windows: []quota.Window{session, week}, Failures: []quota.Failure{fableDown}, Sessions: 1},
		{ID: "personal", Label: "Personal", Error: personalMissing},
		{ID: "side", Label: "Side", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
	}
	if !reflect.DeepEqual(probed.Accounts, want) {
		t.Errorf("once probed, the accounts read\n%+v\nwant\n%+v", probed.Accounts, want)
	}
	if got := prober.probed(); !reflect.DeepEqual(got, []string{sideToken, workToken}) {
		t.Errorf("probed tokens %q, want each account's with one, once", got)
	}

	if err := stop(); err != nil {
		t.Errorf("Run() = %v once its context ended, want nil", err)
	}
	if _, err := os.Stat(socket); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control socket after stopping: %v, want it gone", err)
	}
	for _, want := range [][]string{
		{"level=WARN", `msg="account has no usable token; nothing will go out on it"`, "account=personal", `error="` + personalMissing + `"`},
		{"level=INFO", "msg=listening", "address=" + cfg.Listen, "control=" + socket, "upstream=" + up.URL, "token_set.work=true token_set.personal=false token_set.side=true"},
		{"level=DEBUG", `msg="probed account"`, "account=work", "windows=2"},
		{"level=WARN", `msg="window unread"`, "account=work", "window=7d_oi"},
		{"level=WARN", `msg="probe failed"`, "account=side", `error="HTTP 401 · Invalid bearer token"`},
		{"level=INFO", "msg=stopping"},
		{"level=INFO", "msg=stopped"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
}

func TestNewWithoutAUsableToken(t *testing.T) {
	cfg := testConfig("http://127.0.0.1:1")
	cfg.Token = tokenstest.Files{"work": " "}.Read

	_, err := router.New(cfg)
	want := "no account has a usable token, so there's nothing to route to:\n" +
		"work: token missing\n" +
		"personal: " + personalMissing + "\n" +
		"side: " + tokenstest.Missing("side").Error()
	if err == nil || err.Error() != want {
		t.Errorf("New() error = %v, want\n%s", err, want)
	}
}

func TestRunSaysWhereItListens(t *testing.T) {
	up := newUpstream(t, answerOK)
	cfg := runConfig(t, up.URL)
	cfg.Listen = "127.0.0.1:0"
	runRouter(t, cfg)

	h, err := router.NewClient(router.SocketPath(cfg.StateDir)).Health(t.Context())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if host, port, err := net.SplitHostPort(h.Listen); err != nil || host != "127.0.0.1" || port == "0" {
		t.Fatalf("Health() says the proxy listens on %q, want the address it took for 127.0.0.1:0", h.Listen)
	}
	if got := post("http://" + h.Listen + "/v1/messages"); !strings.HasPrefix(got, "200 ") {
		t.Errorf("a request where the router says it listens answered %s, want 200", got)
	}
}

func TestRunLetsRequestsInFlightFinish(t *testing.T) {
	arrived, release := make(chan struct{}), make(chan struct{})
	up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			t.Error("the request was never released")
		}
		answerOK(w, r)
	})
	cfg := runConfig(t, up.URL)
	stop := runRouter(t, cfg)
	answered := make(chan string, 1)
	go func() { answered <- post("http://" + cfg.Listen + "/v1/messages") }()
	<-arrived

	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	waitUntil(t, "the proxy stops taking requests", func() bool {
		conn, err := net.Dial("tcp", cfg.Listen)
		if err == nil {
			_ = conn.Close()
		}
		return err != nil
	})
	select {
	case err := <-stopped:
		t.Fatalf("Run() = %v with a request still in flight", err)
	default:
	}
	close(release)
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
	if err := <-stopped; err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
}

func TestARestartedRouterKeepsSessionsWhereTheyWere(t *testing.T) {
	up := newAccountsAPI(t)
	cfg := runConfig(t, up.URL)
	prober := &fakeProber{}
	cfg.Prober = prober
	prober.answer(workToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, weekOf(0.5, 24*time.Hour)}}})
	prober.answer(sideToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, weekOf(0.5, 5*24*time.Hour)}}})
	stop := runRouter(t, cfg)
	proxy := "http://" + cfg.Listen
	readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
	if _, err := router.NewClient(router.SocketPath(cfg.StateDir)).Pin(t.Context(), router.PinRequest{Account: "side"}); err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	statePath := filepath.Join(cfg.StateDir, "state.json")
	if info, err := os.Stat(statePath); err != nil || info.Mode() != 0o600 {
		t.Fatalf("state file: %v, %v, want it saved on the way out, readable by its owner alone", info, err)
	}

	// Side is pinned, and its quota now needs using first, but the session's
	// cache is on work.
	prober.answer(workToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, weekOf(0.5, 5*24*time.Hour)}}})
	prober.answer(sideToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, weekOf(0, 24*time.Hour)}}})
	log := logstest.Capture(t)
	runRouter(t, cfg)
	readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(sideToken), strings.NewReader(messages)))
	if got := up.lastAccount(); got != "work" {
		t.Errorf("after the restart, the session's request went out on %s, want work", got)
	}
	waitForLine(t, log, "msg=routed", "session=0b5c6f2e", "account=work", "reason=sticky")
	doc := waitForStatus(t, router.SocketPath(cfg.StateDir), func(status.Document) bool { return true })
	if doc.Pin.Account != "side" {
		t.Errorf("after the restart, the pin is %+v, want side's, as it was", doc.Pin)
	}
	if !log.Has("level=INFO", `msg="loaded state"`, "path="+statePath, "assignments=1", "pin=side") {
		t.Errorf("log reads\n%s\nwant the state loaded", log)
	}
}

func TestARestartedRouterProbesOnlyTheAccountsItHasNoReadingOf(t *testing.T) {
	clock := newFakeClock(now)
	cfg := runConfig(t, "http://127.0.0.1:1")
	cfg.Now = clock.read
	// Work's session runs past the restart, and side's resets before it.
	sideSession := session
	sideSession.ResetsAt = now.Add(time.Hour)
	first := &fakeProber{}
	first.answer(workToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, week}}})
	first.answer(sideToken, probeResult{usage: quota.Usage{Windows: []quota.Window{sideSession, week}}})
	cfg.Prober = first
	stop := runRouter(t, cfg)
	socket := router.SocketPath(cfg.StateDir)
	waitForStatus(t, socket, func(doc status.Document) bool {
		work, _ := doc.Account("work")
		side, _ := doc.Account("side")
		return len(work.Windows) > 0 && len(side.Windows) > 0
	})
	if err := stop(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	// Two hours on, with nothing read of either since, personal has a token
	// too.
	clock.advance(2 * time.Hour)
	withPersonalToken(&cfg)
	second := readingEvery(session, week)
	cfg.Prober = second
	runRouter(t, cfg)
	doc := waitForStatus(t, socket, func(doc status.Document) bool {
		personal, _ := doc.Account("personal")
		return len(personal.Windows) > 0
	})
	if got := second.probed(); !reflect.DeepEqual(got, []string{personalToken}) {
		t.Errorf("after the restart, probed %q, want personal's alone: the router has readings of work and side", got)
	}
	want := []status.Account{
		{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now, Windows: []quota.Window{session, week}},
		{ID: "personal", Label: "Personal", TokenSet: true, FetchedAt: clock.read(), Windows: []quota.Window{session, week}},
		{ID: "side", Label: "Side", TokenSet: true, FetchedAt: now, Windows: []quota.Window{{Key: "5h", Label: "Session"}, week}, Lapsed: []string{"5h"}},
	}
	if !reflect.DeepEqual(doc.Accounts, want) {
		t.Errorf("after the restart, the accounts read\n%+v\nwant\n%+v: side's session lapsed, its week as read", doc.Accounts, want)
	}
}

func TestARestartedRouterStillTakesAReplacedTokenForItsAccounts(t *testing.T) {
	const renewed = "test-token-work-renewed"
	up := newAccountsAPI(t)
	up.script(workToken, refuseWith(http.StatusUnauthorized, "Invalid bearer token"))
	store, files := withTokenFiles(t)
	cfg := runConfig(t, up.URL)
	files(&cfg)
	proxy := "http://" + cfg.Listen
	stop := runRouter(t, cfg)
	writeToken(t, store, "work", renewed)
	readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
	if err := stop(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	state, err := os.ReadFile(filepath.Join(cfg.StateDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{workToken, renewed, sideToken} {
		if strings.Contains(string(state), token) {
			t.Errorf("the state file holds\n%s\nwant no token in it", state)
		}
	}

	log := logstest.Capture(t)
	runRouter(t, cfg)
	readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", with(claudeCode(workToken), claude.SessionHeader, "before"), strings.NewReader(messages)))
	if got := up.bearers(); len(got) != 3 || got[2] != renewed {
		t.Errorf("after the restart, the requests went out with %q, want the last routed as work's, on its new token", got)
	}
	waitForLine(t, log, "msg=routed", "session=before", "account=work", "status=200")
}

// post posts a messages request to url on work's token, and returns the
// status and body it's answered with, or what went wrong.
func post(url string) string {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(messages))
	if err != nil {
		return err.Error()
	}
	req.Header = claudeCode(workToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	return strconv.Itoa(resp.StatusCode) + " " + string(body)
}

func TestRunStopsProbesInFlight(t *testing.T) {
	log := logstest.Capture(t)
	cfg := runConfig(t, "http://127.0.0.1:1")
	prober := &hangingProber{started: make(chan struct{}, 2)}
	cfg.Prober = prober
	stop := runRouter(t, cfg)
	<-prober.started
	<-prober.started

	if err := stop(); err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
	if log.Has(`msg="probe failed"`) {
		t.Errorf("log reads\n%s\nwant no word of probes cut short by stopping", log)
	}
}

// hangingProber's probes run until they're stopped.
type hangingProber struct {
	started chan struct{}
}

func (p *hangingProber) Probe(ctx context.Context, _ string) (quota.Probe, error) {
	p.started <- struct{}{}
	<-ctx.Done()
	return quota.Probe{}, ctx.Err()
}

func TestRunStoppedAsItStarts(t *testing.T) {
	log := logstest.Capture(t)
	cfg := runConfig(t, "http://127.0.0.1:1")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := router.Run(ctx, cfg); err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
	if _, err := os.Stat(router.SocketPath(cfg.StateDir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control socket: %v, want it gone", err)
	}
	if !log.Has("msg=stopped") {
		t.Errorf("log reads\n%s\nwant the router stopped", log)
	}
}

func TestRunRefusesAControlSocketPathTooLong(t *testing.T) {
	cfg := runConfig(t, "http://127.0.0.1:1")
	cfg.StateDir = filepath.Join(shortTempDir(t), strings.Repeat("d", 100))

	err := router.Run(t.Context(), cfg)
	if err == nil || !strings.Contains(err.Error(), "control.sock") || !strings.Contains(err.Error(), "set XDG_STATE_HOME to a shorter directory") {
		t.Errorf("Run() = %v, want an error naming the socket and how to shorten its path", err)
	}
	if _, err := os.Stat(cfg.StateDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("state directory: %v, want none made", err)
	}
}

// runConfig is testConfig with a free address to listen on and a state
// directory short enough for the control socket.
func runConfig(t *testing.T, upstream string) router.Config {
	t.Helper()
	cfg := testConfig(upstream)
	cfg.Listen = freeAddress(t)
	cfg.StateDir = filepath.Join(shortTempDir(t), "state")
	return cfg
}

// freeAddress returns a loopback address whose port nothing listens on.
func freeAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

// runRouter runs a router with cfg, returning once its control socket answers.
// It runs until the test calls the stop it returns, which returns what Run
// did, or until the test ends.
func runRouter(t *testing.T, cfg router.Config) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	var err error
	finished := make(chan struct{})
	go func() {
		err = router.Run(ctx, cfg)
		close(finished)
	}()
	stop = func() error {
		cancel()
		<-finished
		return err
	}
	t.Cleanup(func() { _ = stop() })
	client := router.NewClient(router.SocketPath(cfg.StateDir))
	waitUntil(t, "the router answers", func() bool {
		select {
		case <-finished:
			t.Fatalf("Run() = %v before it answered", err)
		default:
		}
		_, err := client.Health(ctx)
		return err == nil
	})
	return stop
}

// waitForStatus waits a few seconds at most for the router whose control
// socket is at path to report a document that done accepts, and returns it.
func waitForStatus(t *testing.T, path string, done func(status.Document) bool) status.Document {
	t.Helper()
	var doc status.Document
	waitUntil(t, "the router reports the status wanted", func() bool {
		var err error
		doc, err = router.NewClient(path).Status(t.Context())
		return err == nil && done(doc)
	})
	return doc
}

// waitUntil waits a few seconds at most for ready to report true.
func waitUntil(t *testing.T, what string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatalf("gave up waiting until %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
