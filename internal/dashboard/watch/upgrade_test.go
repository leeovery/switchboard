package watch

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
)

// The version the tests' dashboards run, an upgrade's, and one older than
// both.
const (
	ownVersion   = "1.3.1"
	newVersion   = "1.4.0"
	olderVersion = "1.2.0"
)

// upgradeRig is an Upgrade of a binary in a directory of the test's own, its
// other seams faked: the router answers Health with health; the binary
// answers its version with answer, or fails with refusal, once hold, where
// it's set, closes, each ask counted, with how long it was given; and the
// process is replaced in vain, each restart noting the pool it shows, and
// calling atExec, where it's set, then failing with execErr.
type upgradeRig struct {
	mu      sync.Mutex
	path    string
	watched router.Watched
	health  router.Health
	answer  string
	refusal error
	hold    chan struct{}
	asks    int
	given   time.Duration
	execs   []string
	atExec  func()
	execErr error
}

// newUpgradeRig is a rig of a binary as the dashboard was started as,
// answering the new version, its router answering none.
func newUpgradeRig(t *testing.T) *upgradeRig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "switchboard")
	writeBinary(t, path)
	return &upgradeRig{path: path, watched: router.Watch(path), answer: newVersion, execErr: errors.New("exec format error")}
}

// writeBinary writes a binary at path: another file than any there before.
func writeBinary(t *testing.T, path string) {
	t.Helper()
	next := path + ".next"
	if err := os.WriteFile(next, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, path); err != nil {
		t.Fatal(err)
	}
}

// upgrade has the binary's path lead to another file, as brew upgrade
// moves its link on to a new version's.
func (r *upgradeRig) upgrade(t *testing.T) {
	t.Helper()
	writeBinary(t, r.path)
}

// vanish has the binary's path lead nowhere, as for a moment while brew
// upgrade moves its link on.
func (r *upgradeRig) vanish(t *testing.T) {
	t.Helper()
	if err := os.Remove(r.path); err != nil {
		t.Fatal(err)
	}
}

// routerOn has the router run version, as both a read and Health find it.
func (r *upgradeRig) routerOn(h *harness, version string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.health = router.Health{PID: 4242, Version: version}
	h.source.health = r.health
}

// config is the Upgrade the rig fakes, of a dashboard of ownVersion.
func (r *upgradeRig) config() Upgrade {
	return Upgrade{
		Version: ownVersion,
		Binary:  r.watched,
		Look:    func() filestate.State { return filestate.Of(r.path) },
		Health: func(context.Context) router.Health {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.health
		},
		Ask: func(ctx context.Context) (string, error) {
			if r.hold != nil {
				<-r.hold
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			r.asks++
			if deadline, ok := ctx.Deadline(); ok {
				r.given = time.Until(deadline)
			}
			return r.answer, r.refusal
		},
		Exec: func(pool string) error {
			r.mu.Lock()
			r.execs = append(r.execs, pool)
			atExec := r.atExec
			r.mu.Unlock()
			if atExec != nil {
				atExec()
			}
			return r.execErr
		},
	}
}

// counts are how many times the binary was asked its version, and the
// process replaced.
func (r *upgradeRig) counts() (asks, execs int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.asks, len(r.execs)
}

// upgradedBy has h's model restart onto an upgrade as rig fakes it, the
// terminal handed over in place.
func (h *harness) upgradedBy(rig *upgradeRig) {
	h.model.cfg.Upgrade = rig.config()
	h.model.cfg.handOver = func(c tea.ExecCommand, fn tea.ExecCallback) tea.Cmd {
		c.SetStdout(io.Discard)
		return func() tea.Msg { return fn(c.Run()) }
	}
}

// reread has h's model read its source again, as R does, and delivers all
// it sends back.
func (h *harness) reread() {
	h.t.Helper()
	h.deliver(h.press("R")...)
}

// A dashboard restarts onto an upgrade of its binary once a read finds the
// binary changed, and the router running another version than its own, or
// none answering, and the binary says its version, another than its own.
func TestADashboardRestartsOntoAnUpgradeOnceTheRouterRunsAnotherVersion(t *testing.T) {
	tests := []struct {
		name     string
		upgraded bool
		// router is the version the router runs, "" for none answering;
		// probing has the dashboard probe as asked, as --probe does, the
		// router answering only when asked after.
		router  string
		probing bool
		// answer is the version the binary says, and refusal how it fails
		// to say one.
		answer              string
		refusal             error
		wantAsks, wantExecs int
	}{
		{name: "the binary unchanged", router: newVersion, answer: newVersion},
		{name: "the binary unchanged, the dashboard newer than the router", router: olderVersion, answer: newVersion},
		{name: "upgraded, the router running the upgrade", upgraded: true, router: newVersion, answer: newVersion, wantAsks: 1, wantExecs: 1},
		{name: "upgraded, the router still running the dashboard's version", upgraded: true, router: ownVersion, answer: newVersion},
		{name: "upgraded, no router answering", upgraded: true, answer: newVersion, wantAsks: 1, wantExecs: 1},
		{name: "upgraded, probing, the router running the upgrade", upgraded: true, router: newVersion, probing: true, answer: newVersion, wantAsks: 1, wantExecs: 1},
		{name: "upgraded, probing, the router still running the dashboard's version", upgraded: true, router: ownVersion, probing: true, answer: newVersion},
		{name: "upgraded, the binary failing to say its version", upgraded: true, refusal: errors.New("exit status 1"), wantAsks: 1},
		{name: "upgraded, the binary not answering in time", upgraded: true, refusal: context.DeadlineExceeded, wantAsks: 1},
		{name: "upgraded, the binary saying the dashboard's own version, as a dev build does", upgraded: true, answer: ownVersion, wantAsks: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rig := newUpgradeRig(t)
			rig.answer, rig.refusal = tt.answer, tt.refusal
			h := newHarness(t, calm())
			if tt.router != "" {
				h = routedHarness(t, routerDocument(three()...))
				h.source.probing = tt.probing
				rig.routerOn(h, tt.router)
			}
			h.upgradedBy(rig)
			if tt.upgraded {
				rig.upgrade(t)
			}

			h.start()
			if asks, execs := rig.counts(); asks != tt.wantAsks || execs != tt.wantExecs {
				t.Errorf("the binary asked its version %d times, the process replaced %d; want %d and %d", asks, execs, tt.wantAsks, tt.wantExecs)
			}
		})
	}
}

// A dashboard that stays on its own version, as the binary doesn't say
// another or can't be run, asks again only once the binary changes again,
// never at every read: no loop.
func TestADashboardThatStaysAsksAgainOnlyOnceTheBinaryChangesAgain(t *testing.T) {
	tests := []struct {
		name    string
		answer  string
		refusal error
		// execs is how many times a restart replaces the process in vain at
		// each ask.
		execs int
		want  string
	}{
		{name: "failing to say its version", refusal: errors.New("exit status 1"), want: `msg="the upgraded binary didn't say its version; staying on this one until the binary changes again"`},
		{name: "not answering in time", refusal: context.DeadlineExceeded, want: `error="context deadline exceeded"`},
		{name: "saying the dashboard's own version", answer: ownVersion, want: `msg="the upgraded binary is this version; staying on it until the binary changes again"`},
		{name: "the process not replaced", answer: newVersion, execs: 1, want: `msg="can't restart onto the upgrade; carrying on as this version until the binary changes again"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			rig := newUpgradeRig(t)
			rig.answer, rig.refusal = tt.answer, tt.refusal
			h := newHarness(t, calm())
			h.upgradedBy(rig)
			rig.upgrade(t)
			h.start()

			h.reread()
			h.reread()
			if asks, execs := rig.counts(); asks != 1 || execs != tt.execs {
				t.Errorf("after two more reads, the binary asked %d times, the process replaced %d; want once, and %d", asks, execs, tt.execs)
			}
			if !log.Has("version="+ownVersion, tt.want) || strings.Count(log.String(), tt.want) != 1 {
				t.Errorf("the log reads\n%s\nwant a line with %q once", log, tt.want)
			}
			rig.upgrade(t)
			h.reread()
			if asks, execs := rig.counts(); asks != 2 || execs != 2*tt.execs {
				t.Errorf("once the binary changed again, asked %d times, replaced %d; want twice, and %d", asks, execs, 2*tt.execs)
			}
		})
	}
}

// A path leading nowhere for a moment, as while brew upgrade moves the link
// on, isn't an upgrade: the dashboard waits for a read that finds it there.
func TestABinaryGoneForAMomentIsNoUpgrade(t *testing.T) {
	rig := newUpgradeRig(t)
	h := newHarness(t, calm())
	h.upgradedBy(rig)
	rig.vanish(t)
	h.start()
	h.reread()
	if asks, _ := rig.counts(); asks != 0 {
		t.Fatalf("with the binary gone, it was asked its version %d times, want none", asks)
	}

	rig.upgrade(t)
	h.reread()
	if asks, execs := rig.counts(); asks != 1 || execs != 1 {
		t.Errorf("back as another file, asked %d times, replaced %d; want once each", asks, execs)
	}
}

// Upgraded while the router still runs the dashboard's own version, the
// dashboard waits for the router to hand over, saying so once, then
// restarts at the first read that finds it running another.
func TestADashboardWaitsForTheRouterToHandOver(t *testing.T) {
	log := logstest.Capture(t)
	rig := newUpgradeRig(t)
	h := routedHarness(t, routerDocument(three()...))
	rig.routerOn(h, ownVersion)
	h.upgradedBy(rig)
	rig.upgrade(t)
	h.start()
	h.reread()

	const waits = `msg="the binary was upgraded; restarting onto it once the router runs another version"`
	if asks, _ := rig.counts(); asks != 0 || strings.Count(log.String(), waits) != 1 {
		t.Fatalf("with the router on the dashboard's version, the binary asked %d times, and the log reads\n%s\nwant none, and the wait noted once", asks, log)
	}
	rig.routerOn(h, newVersion)
	h.reread()
	if asks, execs := rig.counts(); asks != 1 || execs != 1 {
		t.Errorf("once the router runs the upgrade, asked %d times, replaced %d; want once each", asks, execs)
	}
	if !log.Has("level=INFO", `msg="restarting onto the upgrade"`, "from="+ownVersion, "to="+newVersion) {
		t.Errorf("the log reads\n%s\nwant the restart noted, from and to which version", log)
	}
}

// The binary is given 10 seconds to say its version.
func TestTheBinaryIsGivenTenSecondsToAnswer(t *testing.T) {
	rig := newUpgradeRig(t)
	h := newHarness(t, calm())
	h.upgradedBy(rig)
	rig.upgrade(t)
	h.start()

	if rig.given <= 9*time.Second || rig.given > 10*time.Second {
		t.Errorf("the binary was given %s to answer, want 10s", rig.given)
	}
}

// The binary is asked about one ask at a time: a read landing while one is
// under way asks nothing more.
func TestTheBinaryIsAskedOneAskAtATime(t *testing.T) {
	rig := newUpgradeRig(t)
	h := newHarness(t, calm())
	h.upgradedBy(rig)
	rig.upgrade(t)
	held := holdingBack[upgradeAskedMsg](h, h.init()...)
	holdingBack[upgradeAskedMsg](h, h.press("R")...)
	if asks, _ := rig.counts(); asks != 1 || len(held) != 1 {
		t.Fatalf("with an ask under way, a read asked the binary %d times in all, %d answers held; want once", asks, len(held))
	}

	h.deliver(held...)
	if asks, execs := rig.counts(); asks != 1 || execs != 1 {
		t.Errorf("once the ask landed, asked %d times, replaced %d; want once each", asks, execs)
	}
}

// A restart shows the pool shown, one P turned to among them, and keeps
// what's shown, the changes not yet kept and those of a write under way
// among them, before the process is replaced.
func TestARestartShowsThePoolShownAndKeepsWhatsShownFirst(t *testing.T) {
	rig := newUpgradeRig(t)
	h, acme, _ := pooledHarness(t, acmeAccounts)
	prefs := newCountedPrefs(t)
	h.model.cfg.Prefs = prefs
	h.upgradedBy(rig)
	h.start()
	h.deliver(h.press("P")...)
	if h.model.cfg.Pool.ID != acmeShown.ID {
		t.Fatalf("P showed %q, want Acme's pool", h.model.cfg.Pool.ID)
	}
	next, _ := h.model.Update(tabKey)
	h.model = next.(Model)
	next, _ = h.model.Update(tabKey)
	h.model = next.(Model)
	var kept string
	rig.atExec = func() { kept = prefs.file.Read().Tab }

	rig.upgrade(t)
	acme.health = router.Health{PID: 4242, Version: newVersion}
	h.reread()
	if len(rig.execs) != 1 || rig.execs[0] != acmeShown.ID {
		t.Fatalf("the process replaced showing the pools %q, want once, Acme's", rig.execs)
	}
	if kept != string(dashboard.Projects) {
		t.Errorf("as the process was replaced, the tab kept was %q, want projects, the one shown", kept)
	}
}

// An ask, or a restart, under way as P turns to another pool lands all the
// same, so the watch asks again once the binary moves on.
func TestAnUpgradeAskedAboutLandsAfterATurn(t *testing.T) {
	h, _, _ := pooledHarness(t, acmeAccounts)
	h.start()
	before := h.model.poolTurns
	h.model.upgrade.asking = true

	tm := turns{Model: h.model}
	next, _ := tm.Update(ofTurn{turn: before, msg: turnedMsg{pooled: Pooled{Pool: acmeShown, Source: h.source, Grounds: h.model.cfg.LedgerGrounds}}})
	tm = next.(turns)
	if tm.poolTurns == before || !tm.upgrade.asking {
		t.Fatalf("after the turn, %d turns, asking %v; want it counted, still asking", tm.poolTurns, tm.upgrade.asking)
	}
	for _, msg := range []tea.Msg{upgradeAskedMsg{err: errors.New("exit status 1")}, restartFailedMsg{err: errors.New("exec format error")}} {
		tm.upgrade.asking = true
		next, _ = tm.Update(ofTurn{turn: before, msg: msg})
		if tm = next.(turns); tm.upgrade.asking {
			t.Errorf("%T of before the turn didn't land after it, the watch still asking; want it to", msg)
		}
	}
}

// A restart gives the terminal back as a quit would: out of the alternate
// screen and the mouse's modes, the background put back, and what was typed
// unread thrown away, before the process is replaced. Where it isn't, the
// terminal is taken back.
func TestARestartGivesTheTerminalBackAsAQuitWouldAndTakesItBackInVain(t *testing.T) {
	t.Setenv("TEA_TRACE", "")
	t.Setenv("TEA_DEBUG", "")
	rig := newUpgradeRig(t)
	rig.upgrade(t)
	cfg := themedConfig()
	cfg.Upgrade = rig.config()
	var out lockedBuffer
	var unread []string
	var atExec string
	rig.atExec = func() { atExec = out.String() }
	// The binary answers once the screen is drawn in the terminal's
	// background, which it then has to put back.
	rig.hold = make(chan struct{})
	var settle sync.Once
	filter := func(_ tea.Model, msg tea.Msg) tea.Msg {
		switch msg.(type) {
		case tea.BackgroundColorMsg:
			settle.Do(func() { close(rig.hold) })
		case restartFailedMsg:
			return tea.QuitMsg{}
		}
		return msg
	}
	in, typing := io.Pipe()
	t.Cleanup(func() { _ = typing.Close() })
	go func() { _, _ = io.WriteString(typing, "\x1b]11;rgb:1a1a/1b1b/2626\x07") }()

	err := run(t.Context(), cfg, &out, func() { unread = append(unread, out.String()) }, tea.WithInput(in), tea.WithoutSignals(), tea.WithFilter(filter))
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if _, execs := rig.counts(); execs != 1 {
		t.Fatalf("the process replaced %d times, want once", execs)
	}
	if !strings.HasSuffix(atExec, ansi.SetBackgroundColor("#1a1b26")) {
		t.Errorf("as the process was replaced, the output ended %q, want the background put back", tail(atExec))
	}
	for _, mode := range []struct{ set, reset string }{
		{ansi.SetModeAltScreenSaveCursor, ansi.ResetModeAltScreenSaveCursor},
		{ansi.SetModeMouseButtonEvent, ansi.ResetModeMouseButtonEvent},
	} {
		if strings.LastIndex(atExec, mode.reset) < strings.LastIndex(atExec, mode.set) {
			t.Errorf("as the process was replaced, %q was left set, want it reset", mode.set)
		}
	}
	if len(unread) == 0 || unread[0] != atExec {
		t.Errorf("what was typed unread was thrown away %d times, first as the output ended %q; want before the process was replaced", len(unread), tail(strings.Join(unread, "|")))
	}
	if after := strings.TrimPrefix(out.String(), atExec); !strings.Contains(after, ansi.SetModeAltScreenSaveCursor) {
		t.Errorf("once the process wasn't replaced, the output went on %q, want the terminal taken back", tail(after))
	}
}

// The zero Upgrade, as the capture harness's fixtures have it, never asks
// about a binary, whatever a read finds.
func TestTheZeroUpgradeNeverRestarts(t *testing.T) {
	rig := newUpgradeRig(t)
	rig.upgrade(t)
	h := newHarness(t, calm())
	if _, cmd := h.model.upgradeLooked(filestate.Of(rig.path), router.Health{}); cmd != nil || h.model.cfg.Upgrade.Look != nil {
		t.Error("the zero Upgrade asked about a binary there, or looks at one; want nothing asked, nothing looked at")
	}
}
