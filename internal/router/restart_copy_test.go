package router_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/routercopy"
)

func TestARouterRunningItsOwnCopyReplacesItselfWithTheCopy(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router was started from.
		change     func(t *testing.T, s *selfWatching)
		wantReason string
		// wantStaged is whether the binary asked is the new version copied in
		// beside the copy, and wantCopy the version the copy then holds.
		wantStaged bool
		wantCopy   func(s *selfWatching) string
	}{
		{
			name:       "once the binary it upgrades from leads to another, copying the upgrade in",
			change:     func(t *testing.T, s *selfWatching) { s.upgrade(t) },
			wantReason: "reason=upgraded",
			wantStaged: true,
			wantCopy:   func(s *selfWatching) string { return s.next },
		},
		{
			name:       "once its config file makes another valid config, as it stands",
			change:     func(t *testing.T, s *selfWatching) { writeFile(t, s.config, twoAccounts) },
			wantReason: `reason="config changed"`,
			wantCopy:   func(s *selfWatching) string { return s.current },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newSelfWatching(t, true)
			execs := replacing(&s.cfg)
			own := s.runOwnCopy(t)
			asks := checkingHeld(&s.cfg)
			r := startRouter(t, s.cfg)

			tt.change(t, s)
			r.waitForExit(t)
			made := execs.only(t)
			if made.path != own || !slices.Equal(made.argv, s.cfg.Args) {
				t.Errorf("replaced itself with %s run as %q, want its copy, by the path it was started as, %s, run as it was, %q", made.path, made.argv, own, s.cfg.Args)
			}
			asked := asks.only(t)
			staged := asked.path != own && filepath.Dir(asked.path) == filepath.Dir(own) && strings.HasPrefix(filepath.Base(asked.path), ".switchboard.")
			if staged != tt.wantStaged || asked.held != holding(t, tt.wantCopy(s)) {
				t.Errorf("asked %s, holding %q, want the version it restarts into, %s, copied in beside the copy: %v", asked.path, asked.held, tt.wantCopy(s), tt.wantStaged)
			}
			checkOwnCopy(t, own, tt.wantCopy(s))
			if !log.Has("level=INFO", `msg="replacing itself"`, "path="+own, tt.wantReason) {
				t.Errorf("log reads\n%s\nwant the router replacing itself with its copy", log)
			}
		})
	}
}

func TestAServiceRestartCopiesInTheUpgradeDueBeforeItAnswers(t *testing.T) {
	log := logstest.Capture(t)
	s := newSelfWatching(t, true)
	var arrived <-chan struct{}
	var release func()
	s.cfg.Upstream, arrived, release = holdingUpstream(t)
	execs := replacing(&s.cfg)
	own := s.runOwnCopy(t)
	r := startRouter(t, s.cfg)
	answered := make(chan string, 1)
	go func() { answered <- post("http://" + s.cfg.Listen + "/v1/messages") }()
	<-arrived
	s.upgrade(t)
	waitForLine(t, log, "level=INFO", `msg="restart due; restarting once no request is in flight"`, "reason=upgraded")

	if _, err := router.NewClient(router.SocketPath(s.cfg.StateDir)).Restart(t.Context()); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	checkOwnCopy(t, own, s.next)
	if made := execs.count(); made != 0 {
		t.Fatalf("replaced itself %d times with a request still in flight, want none", made)
	}
	release()
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
	r.waitForExit(t)
	if made := execs.only(t); made.path != own {
		t.Errorf("replaced itself with %s, want its copy, %s", made.path, own)
	}
}

func TestARouterStoppedWithAnUpgradeCopiedInKeptLeavesNothingBesideItsCopy(t *testing.T) {
	s := newSelfWatching(t, true)
	var arrived <-chan struct{}
	var release func()
	s.cfg.Upstream, arrived, release = holdingUpstream(t)
	execs := replacing(&s.cfg)
	own := s.runOwnCopy(t)
	answered := make(chan string, 1)
	var asking sync.Once
	// A request begins as the upgrade copied in is asked, which keeps it for
	// the next quiet moment.
	s.cfg.Check = func(context.Context, string, []string, []string) ([]byte, error) {
		asking.Do(func() {
			go func() { answered <- post("http://" + s.cfg.Listen + "/v1/messages") }()
			<-arrived
		})
		return nil, nil
	}
	stop, finished := runStoppable(t, s.cfg)

	s.upgrade(t)
	waitUntil(t, "the upgrade copied in is kept beside the copy", func() bool {
		entries, err := os.ReadDir(filepath.Dir(own))
		return err == nil && len(entries) == 2
	})
	stop()
	release()
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
	if err := <-finished; err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
	if made := execs.count(); made != 0 {
		t.Errorf("replaced itself %d times, want none: it was told to stop", made)
	}
	checkOwnCopy(t, own, s.current)
}

func TestARouterRunningItsOwnCopyRemovesWhatCopiesCutShortLeftBesideItAsItStarts(t *testing.T) {
	s := newSelfWatching(t, true)
	own := s.runOwnCopy(t)
	leftover := filepath.Join(filepath.Dir(own), ".switchboard.4074419316")
	writeFile(t, leftover, "switchboard 1.1, cut short")

	if err := router.Startable(s.cfg); err != nil {
		t.Fatalf("Startable() = %v", err)
	}
	if _, err := os.Stat(leftover); err != nil {
		t.Errorf("what a copy left once the router is checked: %v, want it left, as the check is of an upgrade copied in there", err)
	}
	startRouter(t, s.cfg)
	if _, err := os.Stat(leftover); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("what a copy left once the router started: %v, want it gone", err)
	}
	checkOwnCopy(t, own, s.current)
}

// runOwnCopy has the router run a copy of switchboard of its own, made of its
// binary as it stands, as service install makes it, upgrading from the
// binary's link, as the service runs it, and returns the copy's path. The
// binary's next version is built an hour on, as releases are, so the copy
// tells the two apart.
func (s *selfWatching) runOwnCopy(t *testing.T) string {
	t.Helper()
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(s.next, later, later); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(t.TempDir(), "switchboard", "router", "switchboard")
	if err := routercopy.At(own).Make(s.binary); err != nil {
		t.Fatal(err)
	}
	s.cfg.Binary, s.cfg.From = router.Watch(own), router.WatchFrom(s.binary, own)
	return own
}

// heldAsked is a binary run to ask it whether it could start: by the path it
// was run by, and what the file there held as it was.
type heldAsked struct {
	path, held string
}

// heldChecks stands in for the binaries a router asks whether they could
// start, each able to, noting each asked, and what it held as it was.
type heldChecks struct {
	mu    sync.Mutex
	asked []heldAsked
}

// checkingHeld has a router run with cfg ask its binaries through heldChecks.
func checkingHeld(cfg *router.Config) *heldChecks {
	c := &heldChecks{}
	cfg.Check = func(_ context.Context, path string, _, _ []string) ([]byte, error) {
		held, err := os.ReadFile(path)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.asked = append(c.asked, heldAsked{path: path, held: string(held)})
		return nil, err
	}
	return c
}

// only returns the one binary asked.
func (c *heldChecks) only(t *testing.T) heldAsked {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.asked) != 1 {
		t.Fatalf("asked %d binaries whether they could start, want one", len(c.asked))
	}
	return c.asked[0]
}

// checkOwnCopy checks the router's copy, at own, is one of the binary at
// want, as copying an upgrade in keeps it, what it holds and when it was
// modified, alone in its directory.
func checkOwnCopy(t *testing.T, own, want string) {
	t.Helper()
	if got := holding(t, own); got != holding(t, want) {
		t.Errorf("the copy holds %q, want %q", got, holding(t, want))
	}
	have, err := os.Stat(own)
	if err != nil {
		t.Fatal(err)
	}
	if wanted, err := os.Stat(want); err != nil || !have.ModTime().Equal(wanted.ModTime()) {
		t.Errorf("the copy was modified at %v, want when %s was", have.ModTime(), want)
	}
	if entries, err := os.ReadDir(filepath.Dir(own)); err != nil || len(entries) != 1 {
		t.Errorf("the copy's directory holds %v (%v), want the copy alone", entries, err)
	}
}

// holding returns what the file at path holds.
func holding(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
