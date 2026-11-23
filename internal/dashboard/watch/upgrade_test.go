package watch

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
	"github.com/leeovery/switchboard/internal/theme"
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
// process is replaced in vain, each restart noting what it carries, and
// calling atExec, where it's set, then failing with execErr. Holding has a
// harness hold each restart, for held to run.
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
	execs   []Shown
	atExec  func()
	execErr error
	holding bool
	held    []func() tea.Msg
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

// routerGone has the router answer neither a read nor Health, as one under
// load whose health check times out.
func (r *upgradeRig) routerGone(h *harness) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.health = router.Health{}
	h.stopRouter()
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
		Exec: func(shown Shown) error {
			r.mu.Lock()
			r.execs = append(r.execs, shown)
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
// terminal handed over in place, at once, or where rig is holding, once the
// test runs the restart held.
func (h *harness) upgradedBy(rig *upgradeRig) {
	h.model.cfg.Upgrade = rig.config()
	h.model.cfg.handOver = func(c tea.ExecCommand, fn tea.ExecCallback) tea.Cmd {
		c.SetStdout(io.Discard)
		restart := func() tea.Msg { return fn(c.Run()) }
		if !rig.holding {
			return restart
		}
		rig.held = append(rig.held, restart)
		return nil
	}
}

// reread has h's model read its source again, as R does, and delivers all
// it sends back.
func (h *harness) reread() {
	h.t.Helper()
	h.deliver(h.press("R")...)
}

// A dashboard restarts onto an upgrade of its binary once a read finds the
// binary changed, and the router running another version than its own, at
// the first read, a probing one's among them, or none answering two reads in
// a row, and the binary says its version, another than its own.
func TestADashboardRestartsOntoAnUpgradeOnceTheRouterHasMovedOver(t *testing.T) {
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
		{name: "upgraded, probing, no router answering", upgraded: true, probing: true, answer: newVersion, wantAsks: 1, wantExecs: 1},
		{name: "upgraded, the binary failing to say its version", upgraded: true, refusal: errors.New("exit status 1"), wantAsks: 1},
		{name: "upgraded, the binary not answering in time", upgraded: true, refusal: context.DeadlineExceeded, wantAsks: 1},
		{name: "upgraded, the binary saying the dashboard's own version, as a dev build does", upgraded: true, answer: ownVersion, wantAsks: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rig := newUpgradeRig(t)
			rig.answer, rig.refusal = tt.answer, tt.refusal
			h := newHarness(t, calm())
			h.source.probing = tt.probing
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
			reads := 1
			if tt.router == "" {
				h.reread()
				reads++
			}
			if asks, execs := rig.counts(); asks != tt.wantAsks || execs != tt.wantExecs {
				t.Errorf("after %d reads, the binary asked its version %d times, the process replaced %d; want %d and %d", reads, asks, execs, tt.wantAsks, tt.wantExecs)
			}
		})
	}
}

// With no router answering, a dashboard moves over only at the second read
// in a row that finds none: the first is no router gone.
func TestWithNoRouterADashboardMovesOverAtTheSecondReadThatFindsNone(t *testing.T) {
	rig := newUpgradeRig(t)
	h := newHarness(t, calm())
	h.upgradedBy(rig)
	rig.upgrade(t)
	h.start()
	if asks, _ := rig.counts(); asks != 0 {
		t.Fatalf("at the first read finding no router, the binary asked %d times, want none", asks)
	}
	h.reread()
	if asks, execs := rig.counts(); asks != 1 || execs != 1 {
		t.Errorf("at the second, asked %d times, replaced %d; want once each", asks, execs)
	}
}

// A router last seen running the dashboard's own version that stops
// answering, as one under load whose health check times out, or one handing
// over, is waited on, however many reads it misses, till a read finds it
// answering.
func TestARouterLastSeenOnTheDashboardsVersionIsWaitedOnThroughReadsItMisses(t *testing.T) {
	rig := newUpgradeRig(t)
	h := routedHarness(t, routerDocument(three()...))
	rig.routerOn(h, ownVersion)
	h.upgradedBy(rig)
	h.start()

	rig.upgrade(t)
	rig.routerGone(h)
	for range 3 {
		h.reread()
	}
	if asks, _ := rig.counts(); asks != 0 {
		t.Fatalf("with the router last seen on the dashboard's version missing three reads, the binary asked %d times, want none", asks)
	}
	h.startRouter(routerDocument(three()...))
	rig.routerOn(h, newVersion)
	h.reread()
	if asks, execs := rig.counts(); asks != 1 || execs != 1 {
		t.Errorf("once a read finds the router on the upgrade, asked %d times, replaced %d; want once each", asks, execs)
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
			h := routedHarness(t, routerDocument(three()...))
			rig.routerOn(h, newVersion)
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
	h := routedHarness(t, routerDocument(three()...))
	rig.routerOn(h, newVersion)
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

	const waits = `msg="the binary was upgraded; restarting onto it once the router has moved over, or none runs"`
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
	h := routedHarness(t, routerDocument(three()...))
	rig.routerOn(h, newVersion)
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
	h := routedHarness(t, routerDocument(three()...))
	rig.routerOn(h, newVersion)
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

// No restart begins while the pool shown is being read, as P turns: a read
// asks nothing, and an answer landing meanwhile restarts nothing, the
// dashboard asking again once the turn is done.
func TestNoRestartBeginsWhileThePoolShownIsRead(t *testing.T) {
	rig := newUpgradeRig(t)
	h, acme, _ := pooledHarness(t, acmeAccounts)
	rig.routerOn(h, newVersion)
	acme.health = rig.health
	h.upgradedBy(rig)
	h.start()

	turn := h.press("P")
	rig.upgrade(t)
	h.reread()
	if asks, _ := rig.counts(); asks != 0 || !h.model.turning {
		t.Fatalf("with a turn under way, %v, a read asked the binary %d times, want none", h.model.turning, asks)
	}

	answer := holdingBack[upgradeAskedMsg](h, turn...)
	turn = h.press("P")
	h.deliver(answer...)
	if asks, execs := rig.counts(); asks != 1 || execs != 0 {
		t.Fatalf("the binary asked %d times, its answer landing during a turn replacing the process %d times; want once, and none", asks, execs)
	}
	h.deliver(turn...)
	if asks, execs := rig.counts(); asks != 2 || execs != 1 {
		t.Errorf("once the turn was done, asked %d times in all, replaced %d; want twice, and once", asks, execs)
	}
}

// A restart carries what this dashboard shows, the pool P turned to among
// it, whatever another dashboard has kept in the preferences meanwhile, and
// keeps the changes it hadn't, a write under way's among them, before the
// process is replaced.
func TestARestartCarriesWhatTheDashboardShowsAndKeepsWhatItHadnt(t *testing.T) {
	rig := newUpgradeRig(t)
	h, acme, _ := pooledHarness(t, acmeAccounts)
	prefs := newCountedPrefs(t)
	h.model.cfg.Prefs = prefs
	h.upgradedBy(rig)
	h.start()
	h.deliver(h.press("P")...)
	next, _ := h.model.Update(tabKey)
	h.model = next.(Model)
	next, _ = h.model.Update(tabKey)
	h.model = next.(Model)
	if err := prefs.file.Update(func(p *theme.Prefs) { p.Tab, p.By = string(dashboard.Log), "account" }); err != nil {
		t.Fatal(err)
	}
	var kept string
	rig.atExec = func() { kept = prefs.file.Read().Tab }

	rig.upgrade(t)
	acme.health = router.Health{PID: 4242, Version: newVersion}
	h.reread()
	if len(rig.execs) != 1 {
		t.Fatalf("the process replaced %d times, want once", len(rig.execs))
	}
	if want := (Shown{Prefs: theme.Prefs{Tab: string(dashboard.Projects)}, Pool: acmeShown.ID}); !reflect.DeepEqual(rig.execs[0], want) {
		t.Errorf("the restart carried %+v, want %+v: what this dashboard shows, Acme's pool, never what another kept", rig.execs[0], want)
	}
	if kept != string(dashboard.Projects) {
		t.Errorf("as the process was replaced, the preferences kept the tab %q, want projects, this dashboard's last change", kept)
	}
}

// What a restart carries is the preferences the watch opened with, each
// change it made since made in them, drawn in the theme chosen.
func TestARestartCarriesThePreferencesOpenedWithAsChangedSince(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.Choice{})
	opened := theme.Prefs{By: "account", Log: theme.LogPrefs{Columns: []string{"model"}}}
	h.model.keeping.shown = opened
	h.model.cfg.Prefs = newCountedPrefs(t)
	h.typed("t", "down", "enter", "esc")
	h.deliver(h.update(tabKey)...)

	want := opened
	want.Tab, want.Choice = string(dashboard.Accounts), theme.One("nord-wave")
	if got := h.model.shownNow().Prefs; !reflect.DeepEqual(got, want) {
		t.Errorf("shown %+v, want %+v", got, want)
	}
	cfg := h.config(Size{Width: 80, Height: 24})
	cfg.Opened = opened
	if got := New(t.Context(), cfg).shownNow().Prefs; !reflect.DeepEqual(got, opened) {
		t.Errorf("a watch opened on %+v shows %+v, want them", opened, got)
	}
}

// A theme picked and not yet kept is carried as chosen, as the restart keeps
// it, though the watch draws in it only once it's kept.
func TestARestartCarriesAThemePickedAndNotYetKept(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.Choice{})
	h.model.cfg.Prefs = newCountedPrefs(t)
	h.typed("t", "down")
	holdingBack[prefsKeptMsg](h, h.press("enter")...)

	want := theme.One("nord-wave")
	if h.model.choice == want {
		t.Fatalf("the watch draws in %+v, want the pick not yet kept", h.model.choice)
	}
	if got := h.model.shownNow().Prefs.Choice; got != want {
		t.Errorf("shown in %+v, want %+v, the theme picked", got, want)
	}
}

// Once a restart is decided on, P turns to no pool, so the pool it carries
// is the one shown; where the process isn't replaced, P turns again.
func TestPTurnsToNoPoolOnceARestartIsDecidedOn(t *testing.T) {
	rig := newUpgradeRig(t)
	rig.holding = true
	h, acme, _ := pooledHarness(t, acmeAccounts)
	rig.routerOn(h, newVersion)
	acme.health = rig.health
	h.upgradedBy(rig)
	rig.upgrade(t)
	h.start()
	shown := h.model.cfg.Pool.ID
	if len(rig.held) != 1 {
		t.Fatalf("%d restarts decided on, want one", len(rig.held))
	}

	if turn := h.press("P"); len(turn) != 0 || h.model.turning {
		t.Fatalf("P turned, turning %v, sending %v; want nothing asked", h.model.turning, turn)
	}
	h.deliver(rig.held[0]())
	if len(rig.execs) != 1 || rig.execs[0].Pool != shown {
		t.Fatalf("the restart carried %+v, want the pool shown, %q", rig.execs, shown)
	}
	h.deliver(h.press("P")...)
	if h.model.cfg.Pool.ID == shown {
		t.Errorf("once the process wasn't replaced, P left %q shown, want it turned", shown)
	}
}

// A restart takes what the watch shows as it runs, not as it was decided
// on: a key taken in between is carried, and kept.
func TestARestartTakesWhatTheWatchShowsAsItRuns(t *testing.T) {
	rig := newUpgradeRig(t)
	rig.holding = true
	h := routedHarness(t, routerDocument(three()...))
	prefs := newCountedPrefs(t)
	h.model.cfg.Prefs = prefs
	rig.routerOn(h, newVersion)
	h.upgradedBy(rig)
	rig.upgrade(t)
	h.start()
	if len(rig.held) != 1 {
		t.Fatalf("%d restarts decided on, want one", len(rig.held))
	}

	next, _ := h.model.Update(tabKey)
	h.model = next.(Model)
	var kept string
	rig.atExec = func() { kept = prefs.file.Read().Tab }
	h.deliver(rig.held[0]())
	if len(rig.execs) != 1 || rig.execs[0].Prefs.Tab != string(dashboard.Accounts) || kept != string(dashboard.Accounts) {
		t.Errorf("the restart carried %+v, the preferences keeping the tab %q; want accounts, the key's, carried and kept", rig.execs, kept)
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
	rig.health = router.Health{PID: 4242, Version: newVersion}
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
