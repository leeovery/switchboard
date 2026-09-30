package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

// Configs the tests' config file holds: two valid, and one that isn't.
const (
	oneAccount  = "[[account]]\nid = \"work\"\n"
	twoAccounts = "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n"
	notAConfig  = "[[account]]\nid = \"work\"\ncolour = \"red\"\n"
)

func TestTheConfigFileIsLookedAtForAChange(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router was started from.
		change func(t *testing.T, s startedFrom)
		// wantDue is why a restart is due, if one is, and wantLog the line
		// the change is logged with, if any.
		wantDue string
		wantLog []string
	}{
		{
			name:   "left alone",
			change: func(*testing.T, startedFrom) {},
		},
		{
			name:    "edited through its link, where it's kept, into another valid config",
			change:  func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) },
			wantDue: "config changed",
			wantLog: []string{"level=INFO", `msg="config changed"`},
		},
		{
			name: "replaced whole where it's kept, as an editor saves it",
			change: func(t *testing.T, s startedFrom) {
				saved := filepath.Join(filepath.Dir(s.kept), ".config.toml.new")
				writeFile(t, saved, twoAccounts)
				rename(t, saved, s.kept)
			},
			wantDue: "config changed",
			wantLog: []string{"level=INFO", `msg="config changed"`},
		},
		{
			name: "its link leading to another valid config",
			change: func(t *testing.T, s startedFrom) {
				other := filepath.Join(filepath.Dir(s.kept), "other.toml")
				writeFile(t, other, twoAccounts)
				relink(t, s.config, other)
			},
			wantDue: "config changed",
			wantLog: []string{"level=INFO", `msg="config changed"`},
		},
		{
			name:    "made invalid",
			change:  func(t *testing.T, s startedFrom) { writeFile(t, s.kept, notAConfig) },
			wantLog: []string{"level=WARN", `msg="config change refused; carrying on with the config as it was"`, `error="invalid config `},
		},
		{
			name:    "removed",
			change:  func(t *testing.T, s startedFrom) { remove(t, s.kept) },
			wantLog: []string{"level=WARN", `msg="config change refused; carrying on with the config as it was"`, "no such file or directory"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start))

			tt.change(t, s)
			r.look()
			if got := r.due(); got != tt.wantDue {
				t.Errorf("due() = %q, want %q", got, tt.wantDue)
			}
			if tt.wantLog != nil && !log.Has(append(tt.wantLog, "path="+s.config)...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
			if tt.wantLog == nil && log.Has("component=router") {
				t.Errorf("log reads\n%s\nwant nothing of the router's", log)
			}
		})
	}
}

func TestTheBinaryIsUpgradedOnceItLeadsToAnotherFile(t *testing.T) {
	tests := []struct {
		name string
		// change changes where the binary's path leads.
		change      func(t *testing.T, s startedFrom)
		wantUpgrade bool
	}{
		{
			name:   "left alone",
			change: func(*testing.T, startedFrom) {},
		},
		{
			name:        "moved on to another version",
			change:      func(t *testing.T, s startedFrom) { relink(t, s.binary, s.version("1.1")) },
			wantUpgrade: true,
		},
		{
			name:        "rewritten where it is",
			change:      func(t *testing.T, s startedFrom) { writeFile(t, s.version("1.0"), "switchboard 1.0, rebuilt") },
			wantUpgrade: true,
		},
		{
			name:   "leading nowhere for a moment, as an upgrade moves it on",
			change: func(t *testing.T, s startedFrom) { remove(t, s.binary) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start))

			tt.change(t, s)
			r.look()
			want := ""
			if tt.wantUpgrade {
				want = "upgraded"
			}
			if got := r.due(); got != want {
				t.Errorf("due() = %q, want %q", got, want)
			}
			upgraded := log.Has("level=INFO", `msg="the binary leads to another file, as after an upgrade"`, "path="+s.binary)
			if upgraded != tt.wantUpgrade {
				t.Errorf("log reads\n%s\nwant the upgrade logged: %v", log, tt.wantUpgrade)
			}
		})
	}
}

func TestTheTimeZoneChangesOnceItsFileLeadsToAnother(t *testing.T) {
	tests := []struct {
		name string
		// change changes where the time zone's file leads.
		change   func(t *testing.T, s startedFrom)
		wantDue  string
		wantNews bool
	}{
		{
			name:   "left alone",
			change: func(*testing.T, startedFrom) {},
		},
		{
			name:     "leading to another time zone's, as the Mac is taken there",
			change:   func(t *testing.T, s startedFrom) { relink(t, s.zone, s.zoneInfo("Etc/GMT-9")) },
			wantDue:  "time zone changed",
			wantNews: true,
		},
		{
			name:   "leading nowhere for a moment, as it's set",
			change: func(t *testing.T, s startedFrom) { remove(t, s.zone) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start))

			tt.change(t, s)
			r.look()
			if got := r.due(); got != tt.wantDue {
				t.Errorf("due() = %q, want %q", got, tt.wantDue)
			}
			news := log.Has("level=INFO", `msg="the time zone's file leads to another, as in another time zone"`, "path="+s.zone)
			if news != tt.wantNews {
				t.Errorf("log reads\n%s\nwant the change logged: %v", log, tt.wantNews)
			}
		})
	}
}

func TestAChangeMadeOnceWhatTheRouterStartsFromWasFoundIsSeen(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router is started from, once it's found,
		// as the router starts.
		change  func(t *testing.T, s startedFrom)
		wantDue string
	}{
		{
			name:    "its config file making another valid config",
			change:  func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) },
			wantDue: "config changed",
		},
		{
			name:    "its binary leading to another file",
			change:  func(t *testing.T, s startedFrom) { relink(t, s.binary, s.version("1.1")) },
			wantDue: "upgraded",
		},
		{
			name:    "the time zone's file leading to another",
			change:  func(t *testing.T, s startedFrom) { relink(t, s.zone, s.zoneInfo("Etc/GMT-9")) },
			wantDue: "time zone changed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStartedFrom(t)
			config, binary, zone := Watch(s.config), Watch(s.binary), Watch(s.zone)

			tt.change(t, s)
			r := newRestarts(config, binary, zone, true, newInFlight(), at(start))
			r.look()
			if got := r.due(); got != tt.wantDue {
				t.Errorf("due() = %q, want %q", got, tt.wantDue)
			}
		})
	}
}

func TestABinaryThatLedNowhereAsTheRouterStartedIsNeverUpgraded(t *testing.T) {
	s := newStartedFrom(t)
	missing := filepath.Join(t.TempDir(), "switchboard")
	r := newRestarts(Watch(s.config), Watch(missing), Watch(s.zone), true, newInFlight(), at(start))

	link(t, s.version("1.1"), missing)
	r.look()
	if got := r.due(); got != "" {
		t.Errorf("due() = %q, want none: nothing's known of the binary running", got)
	}
}

func TestARestartIsHeldBackWhileTheConfigFileIsInvalid(t *testing.T) {
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start))

	writeFile(t, s.kept, notAConfig)
	relink(t, s.binary, s.version("1.1"))
	r.look()
	if got, ready := r.due(), r.ready(); got != "" || ready != nil {
		t.Errorf("upgraded, with the config file invalid, due() = %q, and ready() = %v, want none: a router started again couldn't start", got, ready)
	}

	writeFile(t, s.kept, twoAccounts)
	r.look()
	if got := r.due(); got != "config changed" {
		t.Errorf("with the config file put right, due() = %q, want config changed", got)
	}
}

func TestARestartDueIsSaidOnceByHand(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), false, newInFlight(), at(start))

	writeFile(t, s.kept, twoAccounts)
	r.look()
	relink(t, s.binary, s.version("1.1"))
	writeFile(t, s.kept, oneAccount)
	r.look()
	if ready := r.ready(); ready != nil {
		t.Errorf("ready() = %v, want nil: a router run by hand doesn't restart", ready)
	}
	due := []string{"level=INFO", `msg="restart due; run switchboard serve again to take it up"`, `reason="config changed"`}
	if !log.Has(due...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, due)
	}
	if n := strings.Count(log.String(), `msg="restart due`); n != 1 {
		t.Errorf("log reads\n%s\nwant the restart due said once, not %d times", log, n)
	}
}

func TestASupervisedRouterRestartsOnceNoRequestIsInFlight(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	inFlight := newInFlight()
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, inFlight, at(start))
	if r.ready() != nil {
		t.Fatal("ready() isn't nil, with no restart due")
	}

	inFlight.begin()
	relink(t, s.binary, s.version("1.1"))
	r.look()
	if !log.Has("level=INFO", `msg="restart due; restarting once no request is in flight"`, "reason=upgraded") {
		t.Errorf("log reads\n%s\nwant the restart due", log)
	}
	ready := r.ready()
	select {
	case <-ready:
		t.Fatal("ready() is closed with a request in flight")
	default:
	}
	inFlight.end()
	select {
	case <-ready:
	default:
		t.Fatal("ready() isn't closed once no request is in flight")
	}

	if !r.restart() {
		t.Fatal("restart() = false, want the router restarted")
	}
	select {
	case <-r.restarted:
	default:
		t.Error("restarted isn't closed once the router restarts")
	}
	if !log.Has("level=INFO", "msg=restarting", "reason=upgraded") {
		t.Errorf("log reads\n%s\nwant the restart", log)
	}
}

func TestARestartAskedForIsTheOneTheRouterMakes(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start))
	relink(t, s.binary, s.version("1.1"))
	r.look()

	r.atOnce()
	if !r.restart() {
		t.Error("restart() = false, want true: a restart is due")
	}
	select {
	case <-r.restarted:
	default:
		t.Fatal("restarted isn't closed once the router is asked to restart")
	}
	if r.reason != "asked to" {
		t.Errorf("the router restarts because %q, want asked to", r.reason)
	}
	if n := strings.Count(log.String(), "msg=restarting"); n != 1 {
		t.Errorf("log reads\n%s\nwant the restart said once, not %d times", log, n)
	}
}

func TestARestartDueIsReported(t *testing.T) {
	tests := []struct {
		name       string
		supervised bool
		want       status.Restart
	}{
		{name: "by the service's router", supervised: true, want: status.Restart{Reason: "upgraded", Since: start, InFlight: 2}},
		{name: "by a router run by hand", want: status.Restart{Reason: "upgraded", Since: start, InFlight: 2, ByHand: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStartedFrom(t)
			inFlight := newInFlight()
			clock := &testClock{now: start}
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), tt.supervised, inFlight, clock.read)
			r.look()
			if got := r.report(); got != (status.Restart{}) {
				t.Fatalf("with nothing changed, report() = %+v, want none due", got)
			}

			inFlight.begin()
			inFlight.begin()
			relink(t, s.binary, s.version("1.1"))
			r.look()
			clock.now = start.Add(time.Hour)
			r.look()
			if got := r.report(); got != tt.want {
				t.Errorf("upgraded an hour ago, with two requests in flight, report() = %+v, want %+v", got, tt.want)
			}

			writeFile(t, s.kept, notAConfig)
			r.look()
			if got := r.report(); got != (status.Restart{}) {
				t.Errorf("with the config file invalid, report() = %+v, want none due: a router started again couldn't start", got)
			}
		})
	}
}

func TestARestartLooksAgainBeforeItGoes(t *testing.T) {
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start))
	relink(t, s.binary, s.version("1.1"))
	r.look()

	writeFile(t, s.kept, notAConfig)
	if r.restart() {
		t.Error("restart() = true, want the router kept running: its config file is invalid since it last looked")
	}
	select {
	case <-r.restarted:
		t.Error("restarted is closed, want the router kept running")
	default:
	}
}

func TestRequestsInFlight(t *testing.T) {
	inFlight := newInFlight()
	quiet := func() bool {
		select {
		case <-inFlight.quiet():
			return true
		default:
			return false
		}
	}
	if !quiet() {
		t.Fatal("quiet() isn't closed before any request")
	}
	inFlight.begin()
	inFlight.begin()
	inFlight.end()
	if quiet() {
		t.Error("quiet() is closed with a request still in flight")
	}
	inFlight.end()
	if !quiet() {
		t.Error("quiet() isn't closed once every request has ended")
	}
}

func TestTheRequestsCountedAreThoseInFlightButForUpgradedConnections(t *testing.T) {
	tests := []struct {
		name string
		// header is the request's.
		header    http.Header
		wantQuiet bool
	}{
		{name: "a request", header: http.Header{"Authorization": {"Bearer " + workToken}}},
		{name: "a connection upgraded, which can stay open as long as its session runs", header: http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}}, wantQuiet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inFlight := newInFlight()
			arrived, release := make(chan struct{}), make(chan struct{})
			h := inFlight.count(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				close(arrived)
				<-release
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/sessions", nil)
			req.Header = tt.header
			served := make(chan struct{})
			go func() {
				h.ServeHTTP(httptest.NewRecorder(), req)
				close(served)
			}()
			<-arrived

			select {
			case <-inFlight.quiet():
				if !tt.wantQuiet {
					t.Error("quiet() is closed while the request is in flight")
				}
			default:
				if tt.wantQuiet {
					t.Error("quiet() isn't closed, want the connection passed over")
				}
			}
			close(release)
			<-served
			select {
			case <-inFlight.quiet():
			default:
				t.Error("quiet() isn't closed once the request has been served")
			}
		})
	}
}

// startedFrom is what a router was started from, in a directory of the
// test's own: its config file, a link to one kept in a dotfiles directory;
// its binary, a link to one of two versions, as Homebrew's is; and the time
// zone's file, a link to one of two zones' files, as macOS's is.
type startedFrom struct {
	root string
	// config is the config file's path, and kept where it's kept.
	config, kept string
	binary       string
	zone         string
}

// newStartedFrom returns what a router was started from: a valid config, its
// binary at version 1.0, and the time zone Etc/GMT, each last changed an
// hour ago.
func newStartedFrom(t *testing.T) startedFrom {
	t.Helper()
	root := t.TempDir()
	s := startedFrom{
		root:   root,
		config: filepath.Join(root, "config", "config.toml"),
		kept:   filepath.Join(root, "dotfiles", "config.toml"),
		binary: filepath.Join(root, "bin", "switchboard"),
		zone:   filepath.Join(root, "etc", "localtime"),
	}
	writeFile(t, s.kept, oneAccount)
	link(t, s.kept, s.config)
	for _, v := range []string{"1.0", "1.1"} {
		writeFile(t, s.version(v), "switchboard "+v)
	}
	link(t, filepath.Join("..", "Cellar", "switchboard", "1.0", "bin", "switchboard"), s.binary)
	for _, name := range []string{"Etc/GMT", "Etc/GMT-9"} {
		writeFile(t, s.zoneInfo(name), "TZif "+name)
	}
	link(t, s.zoneInfo("Etc/GMT"), s.zone)
	anHourAgo := time.Now().Add(-time.Hour)
	for _, path := range []string{s.kept, s.version("1.0"), s.version("1.1"), s.zoneInfo("Etc/GMT"), s.zoneInfo("Etc/GMT-9")} {
		if err := os.Chtimes(path, anHourAgo, anHourAgo); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// version is where the binary of the version given is.
func (s startedFrom) version(v string) string {
	return filepath.Join(s.root, "Cellar", "switchboard", v, "bin", "switchboard")
}

// zoneInfo is where the file of the time zone named is.
func (s startedFrom) zoneInfo(name string) string {
	return filepath.Join(s.root, "zoneinfo", filepath.FromSlash(name))
}

// writeFile writes content to the file at path, in place of what it held,
// making its directory first.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// link puts a link at path leading to target, making its directory first.
func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// relink has the link at path lead to target from now on, as ln -sf does.
func relink(t *testing.T, path, target string) {
	t.Helper()
	remove(t, path)
	link(t, target, path)
}

func rename(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
