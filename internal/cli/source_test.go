package cli_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// workAtStatus is work's status, as status prints it from fakeClaudeAPI's
// probes.
const workAtStatus = `work · Work (primary)
  5-hour      23%  resets in 4h 58m · Mon 18:10
  Week        93%  resets in 4d 7h · Fri 21:00 · runs out ~Mon 18:01
  Fable week 100%  resets in 5d 11h · Sun 01:10 · exhausted
`

// othersAtStatus is how status prints personal and side from fakeClaudeAPI's
// probes, commands run with deps finding personal's token file missing.
func othersAtStatus(t *testing.T, deps cli.Deps) string {
	t.Helper()
	return `personal · Personal
  token missing: write it to ` + tokenPath(t, deps, "personal") + `

side · Side
  HTTP 401 · Invalid bearer token
`
}

func TestStatusReadsTheRouterWhileItRuns(t *testing.T) {
	srv := routingSetup(t)

	got := run(t, srv.deps, "status", "--pretty")
	want := result{stdout: workAtStatus + "  1 session\n\n" + othersAtStatus(t, srv.deps) + `
sessions
  0b5c6f2e  haiku on work  ·  seen just now

best next: side · Side
from the router: healthy  ·  1 session  ·  pinned to side · Side
`}
	if got != want {
		t.Errorf("switchboard status =\n%+v\nwant\n%+v", got, want)
	}

	doc := statusJSON(t, srv.deps)
	work, _ := doc.Account("work")
	if doc.Source != status.SourceRouter || !slices.Equal(doc.Pin.Accounts, []string{"side"}) || !doc.Router.Healthy || doc.Router.Requests != 1 || doc.Sessions != 1 || work.Sessions != 1 {
		t.Errorf("switchboard status --json printed\n%+v\nwant the router's document: pinned to side, healthy, with work's session", doc)
	}
}

func TestStatusReadsAnUnhealthyRoutersDocument(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	srv := newServeSetup(t, gone.URL, nil)
	srv.start(t)
	for range 5 {
		if code := srv.ask(t); code != http.StatusBadGateway {
			t.Fatalf("a request the router can't send on was answered %d, want 502", code)
		}
	}
	srv.waitForStatus(t, func(doc status.Document) bool { return doc.Router.Requests == 5 })

	got := run(t, srv.deps, "status", "--pretty")
	const reason = "5 of the 5 requests in the last 5 minutes failed"
	wantLast := "from the router: unhealthy, " + reason + "  ·  no sessions  ·  routing automatically\n"
	if got.code != 0 || !strings.HasSuffix(got.stdout, "\n"+wantLast) {
		t.Errorf("switchboard status = %+v, want it to end %q", got, wantLast)
	}
	doc := statusJSON(t, srv.deps)
	if want := (status.Health{Requests: 5, Failures: 5, Reason: reason}); doc.Source != status.SourceRouter || doc.Router != want {
		t.Errorf("switchboard status --json printed source %q and router %+v, want the router's, %+v", doc.Source, doc.Router, want)
	}
}

func TestStatusProbesWithoutTheRouter(t *testing.T) {
	got := run(t, statusDeps(t, fakeClaudeAPI(t), nil), "status", "--pretty")
	if want := "\nprobed directly: the router isn't running\n"; !strings.HasSuffix(got.stdout, want) {
		t.Errorf("switchboard status = %+v, want it to end %q", got, want)
	}
	doc := statusJSON(t, statusDeps(t, fakeClaudeAPI(t), nil))
	if want := (status.Fallback{Router: status.RouterNotRunning}); doc.Source != status.SourceProbe || doc.Fallback != want {
		t.Errorf("switchboard status --json printed source %q and fallback %+v, want probe, %+v", doc.Source, doc.Fallback, want)
	}
}

func TestStatusProbesPastARouterThatDoesntAnswer(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	serveSilently(t, srv.socket())

	got := run(t, srv.deps, "status", "--pretty")
	want := result{stdout: workAtStatus + "\n" + othersAtStatus(t, srv.deps) + `
probed directly: the router is unhealthy, no answer within 500ms
`}
	if got != want {
		t.Errorf("switchboard status =\n%+v\nwant\n%+v", got, want)
	}
	doc := statusJSON(t, srv.deps)
	if want := (status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"}); doc.Source != status.SourceProbe || doc.Fallback != want {
		t.Errorf("switchboard status --json printed source %q and fallback %+v, want probe, %+v", doc.Source, doc.Fallback, want)
	}
}

func TestStatusProbesAsAsked(t *testing.T) {
	srv := routingSetup(t)

	got := run(t, srv.deps, "status", "--pretty", "--probe")
	want := result{stdout: workAtStatus + "\n" + othersAtStatus(t, srv.deps) + `
probed directly
`}
	if got != want {
		t.Errorf("switchboard status --probe =\n%+v\nwant\n%+v", got, want)
	}
	doc := statusJSON(t, srv.deps, "--probe")
	if doc.Source != status.SourceProbe || doc.Fallback != (status.Fallback{}) {
		t.Errorf("switchboard status --json --probe printed source %q and fallback %+v, want probe, and no fallback", doc.Source, doc.Fallback)
	}
}

func TestUsageReadsTheRouterWhileItRuns(t *testing.T) {
	api := newClaudeAPI(t)
	srv := newServeSetup(t, api.URL, nil)
	onATerminal.on(&srv.deps)
	// A token for personal, as goldenDeps gives it.
	writeToken(t, srv.deps, "personal", "test-token-personal")
	routing(t, srv)

	tests := []struct {
		args   []string
		probes bool
		// status is the printout's status line, and next the account new
		// sessions go to, which carries ▲: none probed, as where they go is
		// the router's to say, and it's pinned to Side.
		status, next string
	}{
		{status: "● healthy  ·  pinned to Side  ·  1 session  ·  new sessions → ▲ Side", next: "Side"},
		{args: []string{"--probe"}, probes: true, status: "○ probed, as asked"},
	}
	for _, tt := range tests {
		before := len(api.questions())
		got := run(t, srv.deps, append([]string{"usage"}, tt.args...)...)
		printed := ansi.Strip(got.stdout)
		if got.code != 0 || got.stderr != "" || !strings.HasPrefix(printed, "\n SWITCHBOARD ") || !strings.Contains(printed, "\n"+tt.status+"\n") {
			t.Errorf("switchboard usage %s = %+v, want exit status 0, nothing on stderr, and the printout after a blank line, its status %q",
				strings.Join(tt.args, " "), got, tt.status)
		}
		if probed := len(api.questions()) > before; probed != tt.probes {
			t.Errorf("switchboard usage %s probed the accounts: %v, want %v", strings.Join(tt.args, " "), probed, tt.probes)
		}
		for _, label := range []string{"Work", "Personal", "Side"} {
			row := printedRow(got.stdout, "5-HOUR", label)
			if marked := strings.Contains(row, "▲"); marked != (label == tt.next) {
				t.Errorf("switchboard usage %s printed %s's row as %q, marked ▲: %v, want ▲ on %q alone", strings.Join(tt.args, " "), label, row, marked, tt.next)
			}
		}
	}
}

func TestUsageWithoutATerminalPrintsTheRoutersDocument(t *testing.T) {
	srv := routingSetup(t)

	for _, args := range [][]string{nil, {"--refresh"}} {
		got := run(t, srv.deps, append([]string{"usage"}, args...)...)
		var doc status.Document
		if got.code != 0 || json.Unmarshal([]byte(got.stdout), &doc) != nil || doc.Source != status.SourceRouter {
			t.Fatalf("switchboard usage %s, without a terminal, = %+v, want exit status 0 and the router's document", strings.Join(args, " "), got)
		}
		if want := run(t, srv.deps, append([]string{"status", "--json"}, args...)...); got != want {
			t.Errorf("switchboard usage %s, without a terminal, =\n%+v\nwant what status --json prints\n%+v", strings.Join(args, " "), got, want)
		}
	}
}

func TestStatusRefreshHasTheRouterReadWhatItHasntInAMinute(t *testing.T) {
	var later atomic.Int64
	api := newClaudeAPI(t)
	srv := newServeSetup(t, api.URL, nil)
	srv.deps.Now = func() time.Time { return testNow.Add(time.Duration(later.Load())) }
	srv.start(t)
	srv.waitForProbes(t)

	for i, args := range [][]string{nil, {"--refresh"}, {"-r"}} {
		// Two minutes on each time, every account last read over a minute
		// ago, and last probed, work's session reads as used a point more.
		later.Store(int64(i+1) * int64(2*time.Minute))
		used := fmt.Sprintf("0.%d", 31+i)
		api.readSessionAs(used)
		asked := len(api.questions())
		doc := statusJSON(t, srv.deps, args...)
		refreshed := len(api.questions()) > asked
		if want := len(args) > 0; refreshed != want {
			t.Errorf("after switchboard status --json %s, the router probed = %v, want %v", strings.Join(args, " "), refreshed, want)
		}
		shown := "0.23"
		if refreshed {
			shown = used
		}
		work, _ := doc.Account("work")
		if doc.Source != status.SourceRouter || len(work.Windows) == 0 || fmt.Sprint(work.Windows[0].Utilization) != shown {
			t.Errorf("switchboard status --json %s printed\n%+v\nwant the router's document, work's session at %s, as the router last read it", strings.Join(args, " "), doc, shown)
		}
	}
}

func TestUsageRefreshHasTheRouterReadWhatItHasntInAMinute(t *testing.T) {
	var later atomic.Int64
	api := newClaudeAPI(t)
	srv := newServeSetup(t, api.URL, nil)
	srv.deps.Now = func() time.Time { return testNow.Add(time.Duration(later.Load())) }
	onATerminal.on(&srv.deps)
	srv.start(t)
	srv.waitForProbes(t)

	for i, args := range [][]string{{"usage"}, {"usage", "--refresh"}, {"usage", "-r"}} {
		// Two minutes on each time, every account last read over a minute
		// ago, and last probed, work's session reads as used a point more.
		later.Store(int64(i+1) * int64(2*time.Minute))
		used := fmt.Sprintf("0.%d", 31+i)
		api.readSessionAs(used)
		asked := len(api.questions())
		got := run(t, srv.deps, args...)
		if got.code != 0 || got.stderr != "" {
			t.Fatalf("switchboard %s = %+v, want exit status 0 and nothing on stderr", strings.Join(args, " "), got)
		}
		refreshed := len(api.questions()) > asked
		if want := len(args) > 1; refreshed != want {
			t.Errorf("after switchboard %s, the router probed = %v, want %v", strings.Join(args, " "), refreshed, want)
		}
		shown := " 23% "
		if refreshed {
			shown = fmt.Sprintf(" %d%% ", 31+i)
		}
		if row := printedRow(got.stdout, "5-HOUR", "Work"); !strings.Contains(row, shown) {
			t.Errorf("switchboard %s printed work's 5-hour window as %q, want it at%s as the router last read it", strings.Join(args, " "), row, shown)
		}
	}
}

func TestUsageRefreshSeesALimitResetByHand(t *testing.T) {
	var later atomic.Int64
	api := newClaudeAPI(t)
	srv := newServeSetup(t, api.URL, nil)
	srv.deps.Now = func() time.Time { return testNow.Add(time.Duration(later.Load())) }
	onATerminal.on(&srv.deps)
	srv.start(t)
	srv.waitForProbes(t)
	api.limitWeek(true)
	if code := srv.ask(t); code != http.StatusTooManyRequests {
		t.Fatalf("a request on work, at the limit of its week, was answered %d, want 429", code)
	}
	// Work's session lapses, and then its week is reset by hand.
	later.Store(int64(5 * time.Hour))
	api.limitWeek(false)

	// Work's limit, of its week, holds it back from every request: its Fable
	// week's, the fake API's, holds back that model's alone.
	const limited = "limit · back Fri 21:00"
	got := run(t, srv.deps, "usage")
	if row := printedRow(got.stdout, "WEEK", "Work"); got.code != 0 || !strings.HasSuffix(row, limited) {
		t.Fatalf("switchboard usage = %+v, printing work's week as %q; want it to end %q, the limit the router hasn't read since", got, row, limited)
	}
	got = run(t, srv.deps, "usage", "--refresh")
	if row := printedRow(got.stdout, "WEEK", "Work"); got.code != 0 || strings.Contains(row, "limit") || !strings.Contains(row, " 93% ") {
		t.Errorf("switchboard usage --refresh = %+v, printing work's week as %q; want it at 93%%, its limit lifted: its probe, though its session has lapsed, read the reset",
			got, row)
	}
}

func TestDashboardReadsTheRouterWhileItRuns(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	stop := srv.start(t)
	srv.waitForProbes(t)
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "dashboard")

	if cfg.ProbeOnly {
		t.Error("the watch is told it probes as asked, want it reading the router")
	}
	for _, r := range []status.Read{{Refresh: 30 * time.Minute, Probe: true}, {Probe: true}, {Refresh: 30 * time.Minute}} {
		doc, health, err := cfg.Source.Read(t.Context(), r)
		if err != nil || doc.Source != status.SourceRouter {
			t.Errorf("Read(%+v) = %+v, %v, want the router's document", r, doc, err)
		}
		if health.PID != os.Getpid() || health.StartedAt.IsZero() {
			t.Errorf("Read(%+v) says the router is %+v, want the one running: its health check's answer", r, health)
		}
	}
	if err := cfg.Source.Pin(t.Context(), []string{"side", "work"}, true); err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if got, want := srv.status(t).Pin, (status.Pin{Accounts: []string{"work", "side"}, Since: testNow, Move: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("once pinned, the router's pin = %+v, want %+v", got, want)
	}
	if e := srv.told(t); e.Kind != status.EventPin || e.By != "dashboard" {
		t.Errorf("once pinned, the router told of %+v, want a pin by the dashboard", e)
	}
	if err := cfg.Source.Unpin(t.Context()); err != nil {
		t.Fatalf("Unpin() error = %v", err)
	}
	if got := srv.status(t).Pin; !got.IsZero() {
		t.Errorf("once unpinned, the router's pin = %+v, want none", got)
	}
	if e := srv.told(t); e.Kind != status.EventAuto || e.By != "dashboard" {
		t.Errorf("once unpinned, the router told of %+v, want an auto by the dashboard", e)
	}
	history, err := cfg.Source.History(t.Context(), "7d", 30*time.Minute)
	if err != nil || history.Window != "7d" || history.Step != "30m0s" || len(history.Accounts) != 3 {
		t.Errorf("History() = %+v, %v, want the router's history of every account's week", history, err)
	}
	if sessions, err := cfg.Source.Sessions(t.Context()); err != nil || sessions == nil {
		t.Errorf("Sessions() = %+v, %v, want the router's list of its sessions", sessions, err)
	}

	stop()
	if _, err := cfg.Source.History(t.Context(), "7d", 30*time.Minute); !errors.Is(err, router.ErrNotRunning) {
		t.Errorf("once the router stopped, History() error = %v, want it not running", err)
	}
	if _, err := cfg.Source.Sessions(t.Context()); !errors.Is(err, router.ErrNotRunning) {
		t.Errorf("once the router stopped, Sessions() error = %v, want it not running", err)
	}
	if _, _, err := cfg.Source.Read(t.Context(), status.Read{Refresh: 30 * time.Minute}); !errors.Is(err, status.ErrNoRouter) {
		t.Errorf("once the router stopped, a question after it failed with %v, want ErrNoRouter", err)
	}
	doc, health, err := cfg.Source.Read(t.Context(), status.Read{Probe: true})
	if err != nil || doc.Source != status.SourceProbe || doc.Fallback != (status.Fallback{Router: status.RouterNotRunning}) {
		t.Errorf("once the router stopped, a read that may probe = %+v, %v, want one probed, as the router isn't running", doc, err)
	}
	if health != (router.Health{}) {
		t.Errorf("once the router stopped, a read that probed says the router is %+v, want none", health)
	}
	if err := cfg.Source.Pin(t.Context(), []string{"side"}, false); err == nil || err.Error() != "the router isn't running: start it with switchboard service install (or switchboard serve)" {
		t.Errorf("once the router stopped, Pin() error = %v, want it to say so", err)
	}
}

func TestDashboardPinsOneSessionAsPinSessionDoes(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	stop := srv.start(t)
	srv.route(t, sessionOne, "claude-haiku-4-5-20251001")
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "dashboard")

	if err := cfg.Source.PinSession(t.Context(), sessionOne, "side"); err != nil {
		t.Fatalf("PinSession() error = %v", err)
	}
	if got := srv.session(t, sessionOne).Pin; got != "side" {
		t.Errorf("once pinned, the session's own pin = %q, want side", got)
	}
	if e := srv.told(t); e.Kind != status.EventPin || e.Session != sessionOne || e.By != "dashboard" {
		t.Errorf("once pinned, the router told of %+v, want the session's pin by the dashboard", e)
	}
	if err := cfg.Source.PinSession(t.Context(), sessionOne, "personal"); err == nil || !strings.Contains(err.Error(), "personal") {
		t.Errorf("PinSession() to an account without a usable token error = %v, want the router to say why", err)
	}
	if err := cfg.Source.UnpinSession(t.Context(), sessionOne); err != nil {
		t.Fatalf("UnpinSession() error = %v", err)
	}
	if got := srv.session(t, sessionOne).Pin; got != "" {
		t.Errorf("once unpinned, the session's own pin = %q, want none", got)
	}
	if e := srv.told(t); e.Kind != status.EventAuto || e.Session != sessionOne || e.By != "dashboard" {
		t.Errorf("once unpinned, the router told of %+v, want the session's auto by the dashboard", e)
	}
	if !srv.status(t).Pin.IsZero() {
		t.Errorf("the router's pin = %+v, want none: a session's pin is its own", srv.status(t).Pin)
	}

	stop()
	notRunning := "the router isn't running: start it with switchboard service install (or switchboard serve)"
	if err := cfg.Source.PinSession(t.Context(), sessionOne, "side"); err == nil || err.Error() != notRunning {
		t.Errorf("once the router stopped, PinSession() error = %v, want it to say so", err)
	}
	if err := cfg.Source.UnpinSession(t.Context(), sessionOne); err == nil || err.Error() != notRunning {
		t.Errorf("once the router stopped, UnpinSession() error = %v, want it to say so", err)
	}
}

func TestTheRoutingCardsOrdersReachTheRouter(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	srv.waitForProbes(t)
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "dashboard")
	m := watching(t, *cfg)

	// Personal, the second, has no token, so its number picks nothing.
	m = pressed(m, "p", "2", "3", "m", "enter")
	if got, want := srv.status(t).Pin, (status.Pin{Accounts: []string{"side"}, Since: testNow, Move: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("once p 2 3 m enter were pressed, the router's pin = %+v, want %+v", got, want)
	}
	if e := srv.told(t); e.Kind != status.EventPin || !e.Move || e.By != "dashboard" {
		t.Errorf("once pinned, the router told of %+v, want a pin moving the sessions running, by the dashboard", e)
	}

	pressed(m, "p", "0", "enter")
	if got := srv.status(t).Pin; !got.IsZero() {
		t.Errorf("once p 0 enter were pressed, the router's pin = %+v, want none", got)
	}
	if e := srv.told(t); e.Kind != status.EventAuto || e.By != "dashboard" {
		t.Errorf("once unpinned, the router told of %+v, want an auto by the dashboard", e)
	}
}

// watching is the watch dashboard runs, as Watch was given cfg, started and
// its first read taken: its timers, and its waits on the request stream,
// held, so nothing it does waits.
func watching(t *testing.T, cfg watch.Config) watch.Model {
	t.Helper()
	cfg.After = func(time.Duration, tea.Msg) tea.Cmd { return nil }
	cfg.Await = func(tea.Cmd) tea.Cmd { return nil }
	m := watch.New(t.Context(), cfg)
	return delivered(m, sent(m.Init())...)
}

// pressed presses the keys given, in turn, as the terminal sends them.
func pressed(m watch.Model, keys ...string) watch.Model {
	for _, k := range keys {
		key := tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
		if k == "enter" {
			key = tea.KeyPressMsg{Code: tea.KeyEnter}
		}
		m = delivered(m, key)
	}
	return m
}

// delivered gives the model each message, and each that the commands it
// returns send back, until none is left, as Bubble Tea would, but at once.
func delivered(m watch.Model, msgs ...tea.Msg) watch.Model {
	for len(msgs) > 0 {
		next, cmd := m.Update(msgs[0])
		m = next.(watch.Model)
		msgs = append(msgs[1:], sent(cmd)...)
	}
	return m
}

// sent runs cmd, and every command it batches, returning what they send back.
func sent(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, sent(c)...)
		}
		return msgs
	default:
		return []tea.Msg{msg}
	}
}

func TestDashboardProbesAsAsked(t *testing.T) {
	srv := routingSetup(t)
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "dashboard", "--probe")

	if !cfg.ProbeOnly {
		t.Error("the watch isn't told it probes as asked, want it told: there's no router for a failed read to have lost")
	}
	doc, _, err := cfg.Source.Read(t.Context(), status.Read{Refresh: 30 * time.Minute, Probe: true})
	if err != nil || doc.Source != status.SourceProbe || doc.Fallback != (status.Fallback{}) {
		t.Errorf("Read() = %+v, %v, want one probed, as asked, while the router runs", doc, err)
	}
	if _, _, err := cfg.Source.Read(t.Context(), status.Read{Refresh: 30 * time.Minute}); !errors.Is(err, status.ErrNoRouter) {
		t.Errorf("a question after the router failed with %v, want ErrNoRouter: it's not to be read", err)
	}
}

func TestDashboardProbingAsAskedAsksAfterTheRouter(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	stop := srv.start(t)
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "dashboard", "--probe")

	if !cfg.Source.RouterAnswers(t.Context()) {
		t.Error("RouterAnswers() = false while the router runs, want true: it posts the notifications")
	}
	stop()
	if cfg.Source.RouterAnswers(t.Context()) {
		t.Error("RouterAnswers() = true once the router stopped, want false: the dashboard posts its own")
	}
}

// routingSetup starts a router of statusDeps' accounts, as routing does.
func routingSetup(t *testing.T) *serveSetup {
	t.Helper()
	return routing(t, newServeSetup(t, fakeClaudeAPI(t), nil))
}

// routing starts srv's router, once it has probed the accounts, with a
// session on work, and pinned to side, and returns srv.
func routing(t *testing.T, srv *serveSetup) *serveSetup {
	t.Helper()
	srv.start(t)
	srv.waitForProbes(t)
	srv.route(t, "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e", "claude-haiku-4-5-20251001")
	if got := run(t, srv.deps, "pin", "side"); got.code != 0 {
		t.Fatalf("switchboard pin side = %+v", got)
	}
	return srv
}

// waitForProbes waits for the router to have probed every account it has a
// token for: to have read its usage, or why it couldn't.
func (s *serveSetup) waitForProbes(t *testing.T) {
	t.Helper()
	s.waitForStatus(t, func(doc status.Document) bool {
		return !slices.ContainsFunc(doc.Accounts, func(a status.Account) bool {
			return a.TokenSet && len(a.Windows) == 0 && a.Error == ""
		})
	})
}

// ask sends the router a messages request on work's token, of no session,
// and returns the status it's answered with.
func (s *serveSetup) ask(t *testing.T) int {
	t.Helper()
	body := `{"model":"claude-opus-5-5","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+s.listen+"/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token-work")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

// statusJSON runs status --json with args, and returns the document it
// prints.
func statusJSON(t *testing.T, deps cli.Deps, args ...string) status.Document {
	t.Helper()
	got := run(t, deps, append([]string{"status", "--json"}, args...)...)
	var doc status.Document
	if got.code != 0 || json.Unmarshal([]byte(got.stdout), &doc) != nil {
		t.Fatalf("switchboard status --json = %+v, want exit status 0 and a document", got)
	}
	return doc
}

// serveSilently listens on the socket at path until the test ends, and
// answers nothing it's asked.
func serveSilently(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }),
		ReadHeaderTimeout: time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}
