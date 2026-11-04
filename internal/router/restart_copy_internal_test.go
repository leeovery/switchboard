package router

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/routercopy"
)

func TestARouterStartedFromACopyThatDiffersHasAnUpgradeDue(t *testing.T) {
	tests := []struct {
		name string
		// starting changes what the router running its copy, at own, is
		// started from, before it's found as the router starts.
		starting func(t *testing.T, s startedFrom, own string)
		wantDue  string
	}{
		{
			name:     "a copy of the binary its link leads to",
			starting: func(*testing.T, startedFrom, string) {},
		},
		{
			name:     "a copy of another version, as after an upgrade made while launchd started it again",
			starting: func(t *testing.T, s startedFrom, _ string) { relink(t, s.binary, s.version("1.1")) },
			wantDue:  "upgraded",
		},
		{
			name:     "no copy there, as one removed by hand",
			starting: func(t *testing.T, _ startedFrom, own string) { remove(t, own) },
			wantDue:  "upgraded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, own := newRunningCopy(t)

			tt.starting(t, s, own)
			r := copying(s, own, unheeded)
			r.look()
			if got := r.due(); got != tt.wantDue {
				t.Errorf("due() = %q, want %q", got, tt.wantDue)
			}
		})
	}
}

func TestARouterStartedAsItsLinkLeadsNowhereHasAnUpgradeDueOnceItLeadsToAFile(t *testing.T) {
	s, own := newRunningCopy(t)
	remove(t, s.binary)
	r := copying(s, own, unheeded)

	r.look()
	if got := r.due(); got != "" || !r.gone {
		t.Errorf("with its link leading nowhere, due() = %q, and gone: %v, want none due, the binary gone: a link that leads nowhere isn't an upgrade", got, r.gone)
	}
	link(t, s.version("1.0"), s.binary)
	r.look()
	if got := r.due(); got != "upgraded" {
		t.Errorf("once it leads to a file, due() = %q, want upgraded: the copy couldn't be compared with it as the router started", got)
	}
}

func TestTheBinaryARouterRunningACopyUpgradesFromIsWatched(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router running its copy, at own, was
		// started from.
		change      func(t *testing.T, s startedFrom, own string)
		wantUpgrade bool
		wantGone    bool
	}{
		{
			name:   "left alone",
			change: func(*testing.T, startedFrom, string) {},
		},
		{
			name:        "its link moved on to another version",
			change:      func(t *testing.T, s startedFrom, _ string) { relink(t, s.binary, s.version("1.1")) },
			wantUpgrade: true,
		},
		{
			name: "the file its link leads to rewritten where it is",
			change: func(t *testing.T, s startedFrom, _ string) {
				writeFile(t, s.version("1.0"), "switchboard 1.0, rebuilt")
			},
			wantUpgrade: true,
		},
		{
			name:     "its link leading nowhere for a moment, as an upgrade moves it on",
			change:   func(t *testing.T, s startedFrom, _ string) { remove(t, s.binary) },
			wantGone: true,
		},
		{
			name:   "the copy rewritten, which isn't what it upgrades from",
			change: func(t *testing.T, _ startedFrom, own string) { writeFile(t, own, "switchboard 1.0, edited") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s, own := newRunningCopy(t)
			r := copying(s, own, unheeded)

			tt.change(t, s, own)
			r.look()
			want := ""
			if tt.wantUpgrade {
				want = "upgraded"
			}
			if got := r.due(); got != want {
				t.Errorf("due() = %q, want %q", got, want)
			}
			if r.gone != tt.wantGone {
				t.Errorf("gone = %v, want %v", r.gone, tt.wantGone)
			}
			upgraded := log.Has("level=INFO", `msg="the binary leads to another file, as after an upgrade"`, "path="+s.binary)
			if upgraded != tt.wantUpgrade {
				t.Errorf("log reads\n%s\nwant the upgrade logged, of the binary it upgrades from: %v", log, tt.wantUpgrade)
			}
		})
	}
}

func TestAnUpgradeIsCopiedOverTheRoutersCopyAsTheRestartBegins(t *testing.T) {
	restarts := []struct {
		name string
		// restart restarts r as the router would, and reports whether it can.
		restart func(t *testing.T, r *restarts) bool
	}{
		{name: "falling due", restart: func(t *testing.T, r *restarts) bool { return r.restart(t.Context()) }},
		{name: "asked for, as service restart asks", restart: func(t *testing.T, r *restarts) bool { return askedFor(t, r) == nil }},
	}
	tests := []struct {
		name string
		// answer is how the upgrade copied in answers whether it could start.
		answer binaryAnswer
	}{
		{name: "able to start"},
		{name: "unable to answer, as one from before the check", answer: binaryAnswer{err: errors.New("exit status 1")}},
	}
	for _, how := range restarts {
		for _, tt := range tests {
			t.Run(how.name+", "+tt.name, func(t *testing.T) {
				logstest.Capture(t)
				s, own := newRunningCopy(t)
				r := copying(s, own, unheeded)
				asks := askedAt(t, r, tt.answer)
				relink(t, s.binary, s.version("1.1"))
				r.look()

				if !how.restart(t, r) {
					t.Fatal("the restart didn't go ahead, want it to")
				}
				if len(*asks) != 1 {
					t.Fatalf("the binary was asked %d times, want once", len(*asks))
				}
				asked := (*asks)[0]
				if filepath.Dir(asked.path) != filepath.Dir(own) || !strings.HasPrefix(filepath.Base(asked.path), ".switchboard.") || asked.held != "switchboard 1.1" {
					t.Errorf("asked %s, holding %q, want the new version, switchboard 1.1, copied in hidden beside the copy, %s", asked.path, asked.held, own)
				}
				checkCopy(t, own, s.version("1.1"))
				checkAlone(t, own)
			})
		}
	}
}

func TestARestartThatDoesntGoAheadLeavesTheCopyAsItWas(t *testing.T) {
	falling := func(ctx context.Context, r *restarts) bool { return r.restart(ctx) }
	asked := func(ctx context.Context, r *restarts) bool {
		cleared, err := r.restartable(ctx)
		if err == nil {
			r.atOnce(cleared)
		}
		return err == nil
	}
	movedOn := func(t *testing.T, s startedFrom, _ *restarts, _ context.CancelFunc) {
		writeFile(t, s.version("1.2"), "switchboard 1.2")
		relink(t, s.binary, s.version("1.2"))
	}
	tests := []struct {
		name string
		// restart restarts r as the router would, until ctx ends, and reports
		// whether it can; asking happens as the upgrade copied in is asked,
		// stop ending ctx; and answer is how it answers.
		restart func(ctx context.Context, r *restarts) bool
		asking  func(t *testing.T, s startedFrom, r *restarts, stop context.CancelFunc)
		answer  binaryAnswer
	}{
		{
			name:    "refusing to start",
			restart: falling,
			answer:  binaryAnswer{refusal: "invalid config"},
		},
		{
			name:    "a request begun as it's asked",
			restart: falling,
			asking:  func(_ *testing.T, _ startedFrom, r *restarts, _ context.CancelFunc) { r.inFlight.begin() },
		},
		{
			name:    "its link moved on to another version as it's asked",
			restart: falling,
			asking:  movedOn,
		},
		{
			name:    "the config file saved as it's asked",
			restart: falling,
			asking: func(t *testing.T, s startedFrom, _ *restarts, _ context.CancelFunc) {
				writeFile(t, s.kept, twoAccounts)
			},
		},
		{
			name:    "told to stop as it's asked",
			restart: falling,
			asking:  func(_ *testing.T, _ startedFrom, _ *restarts, stop context.CancelFunc) { stop() },
		},
		{
			name:    "asked for, refusing to start",
			restart: asked,
			answer:  binaryAnswer{refusal: "invalid config"},
		},
		{
			name:    "asked for, its link moved on to another version as it's asked",
			restart: asked,
			asking:  movedOn,
		},
		{
			name:    "asked for, told to stop as it's asked",
			restart: asked,
			asking:  func(_ *testing.T, _ startedFrom, _ *restarts, stop context.CancelFunc) { stop() },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			s, own := newRunningCopy(t)
			r := copying(s, own, unheeded)
			stopped, stop := context.WithCancel(t.Context())
			defer stop()
			asks := askedAt(t, r, tt.answer)
			answer := r.ask
			r.ask = func(ctx context.Context, path string) (string, error) {
				if tt.asking != nil && len(*asks) == 0 {
					tt.asking(t, s, r, stop)
				}
				return answer(ctx, path)
			}
			relink(t, s.binary, s.version("1.1"))
			r.look()

			if tt.restart(stopped, r) || r.restarting() {
				t.Fatal("the restart went ahead, want it not to")
			}
			if len(*asks) != 1 {
				t.Errorf("the binary was asked %d times, want once", len(*asks))
			}
			checkCopy(t, own, s.version("1.0"))
			checkAlone(t, own)
		})
	}
}

func TestARestartWithNoUpgradeDueGoesIntoTheCopyAsItStands(t *testing.T) {
	tests := []struct {
		name string
		// restart has r, started from s, restart as the router would, and
		// reports whether it can.
		restart func(t *testing.T, s startedFrom, r *restarts) bool
	}{
		{
			name: "once the config file makes another valid config",
			restart: func(t *testing.T, s startedFrom, r *restarts) bool {
				writeFile(t, s.kept, twoAccounts)
				return r.restart(t.Context())
			},
		},
		{
			name: "once the time zone's file leads to another",
			restart: func(t *testing.T, s startedFrom, r *restarts) bool {
				relink(t, s.zone, s.zoneInfo("Etc/GMT-9"))
				return r.restart(t.Context())
			},
		},
		{
			name:    "asked for, as service restart asks",
			restart: func(t *testing.T, _ startedFrom, r *restarts) bool { return askedFor(t, r) == nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, own := newRunningCopy(t)
			r := copying(s, own, unheeded)
			asks := askedAt(t, r, binaryAnswer{})

			if !tt.restart(t, s, r) {
				t.Fatal("the restart didn't go ahead, want it to")
			}
			if len(*asks) != 1 || (*asks)[0].path != own {
				t.Errorf("asked %+v, want the copy alone, by the path the router was started as, %s", *asks, own)
			}
			checkCopy(t, own, s.version("1.0"))
			checkAlone(t, own)
		})
	}
}

func TestAnUpgradeAskedForIsCopiedInOnce(t *testing.T) {
	s, own := newRunningCopy(t)
	r := copying(s, own, unheeded)
	asks := askedAt(t, r, binaryAnswer{})
	relink(t, s.binary, s.version("1.1"))
	r.look()
	if err := askedFor(t, r); err != nil {
		t.Fatalf("restartable() = %v, want nil", err)
	}

	if !r.restart(t.Context()) {
		t.Error("restart() = false at the quiet moment after, want true: the router is restarting")
	}
	if len(*asks) != 1 {
		t.Errorf("the binary was asked %d times, want once: the restart had begun", len(*asks))
	}
	checkCopy(t, own, s.version("1.1"))
	checkAlone(t, own)
}

func TestACopyGoneIsMadeAfreshAsTheRouterRestarts(t *testing.T) {
	s, own := newRunningCopy(t)
	r := copying(s, own, unheeded)
	asks := askedAt(t, r, binaryAnswer{})
	remove(t, own)
	writeFile(t, s.kept, twoAccounts)

	if !r.restart(t.Context()) {
		t.Fatal("restart() = false, want the router restarted into a copy made afresh")
	}
	if len(*asks) != 1 || (*asks)[0].path == own || (*asks)[0].held != "switchboard 1.0" {
		t.Errorf("asked %+v, want a copy of the binary it upgrades from, made beside where the copy was", *asks)
	}
	checkCopy(t, own, s.version("1.0"))
	checkAlone(t, own)
}

func TestARefusedUpgradeIsHeldTillTheBinaryItUpgradesFromMovesOn(t *testing.T) {
	log := logstest.Capture(t)
	s, own := newRunningCopy(t)
	r := copying(s, own, unheeded)
	asks := askedAt(t, r, binaryAnswer{refusal: "invalid config"}, binaryAnswer{})
	relink(t, s.binary, s.version("1.1"))
	r.look()
	r.restart(t.Context())

	for range 3 {
		r.look()
		if ready := r.ready(); ready != nil {
			t.Fatalf("ready() = %v while the upgrade refuses to start, with nothing changed, want none", ready)
		}
	}
	if got := r.report(); got.Held != "invalid config" || got.Reason != "upgraded" {
		t.Errorf("report() = %+v, want the upgrade held, saying why", got)
	}
	if n := strings.Count(log.String(), `msg="restart held`); n != 1 || !log.Has("level=WARN", "path="+s.binary) {
		t.Errorf("log reads\n%s\nwant the restart held said once, of the binary it upgrades from", log)
	}

	writeFile(t, s.version("1.2"), "switchboard 1.2")
	relink(t, s.binary, s.version("1.2"))
	r.look()
	if got := r.report(); got.Held != "" || !quiet(r.ready()) {
		t.Fatalf("with the binary moved on, report() = %+v, want the hold lifted, and the restart ready", got)
	}
	if !r.restart(t.Context()) {
		t.Fatal("restart() = false, want the router restarted: the version it moved on to starts")
	}
	if len(*asks) != 2 || (*asks)[1].held != "switchboard 1.2" {
		t.Errorf("asked %+v, want the version it moved on to asked second", *asks)
	}
	checkCopy(t, own, s.version("1.2"))
	checkAlone(t, own)
}

func TestAnUpgradeThatCantBeCopiedInIsCopiedAgainAtTheNextLook(t *testing.T) {
	tests := []struct {
		name string
		// fail has copying the upgrade in fail, and fixed puts that right.
		fail, fixed func(t *testing.T, s startedFrom, own string)
		// wantErr is how the failure ends what the log says of it.
		wantErr string
	}{
		{
			name:    "copying it, the new version unreadable",
			fail:    func(t *testing.T, s startedFrom, _ string) { chmod(t, s.version("1.1"), 0o000) },
			fixed:   func(t *testing.T, s startedFrom, _ string) { chmod(t, s.version("1.1"), 0o600) },
			wantErr: `permission denied"`,
		},
		{
			name: "putting it in place of the copy, where a directory is",
			fail: func(t *testing.T, _ startedFrom, own string) {
				remove(t, own)
				writeFile(t, filepath.Join(own, "in the way"), "")
			},
			fixed:   func(t *testing.T, _ startedFrom, own string) { removeAll(t, own) },
			wantErr: `put the copy of switchboard in place of the router's, `,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s, own := newRunningCopy(t)
			r := copying(s, own, unheeded)
			asks := askedAt(t, r, binaryAnswer{})
			relink(t, s.binary, s.version("1.1"))
			tt.fail(t, s, own)
			r.look()

			for look := range 3 {
				if r.restart(t.Context()) || r.restarting() {
					t.Fatalf("at look %d, the restart went ahead, want it held off: the upgrade can't be copied in", look)
				}
				if ready := r.ready(); ready != nil {
					t.Fatalf("at look %d, ready() = %v once the copy failed, want none until the next look, so it's not copied again and again", look, ready)
				}
				if got := r.report(); got.Reason != "upgraded" || got.Held != "" {
					t.Errorf("at look %d, report() = %+v, want the upgrade due still, held by nothing", look, got)
				}
				r.look()
				if !quiet(r.ready()) {
					t.Fatalf("at look %d, ready() isn't closed once looked again, want the upgrade copied again", look)
				}
			}
			warning := []string{"level=WARN", `msg="can't copy the upgrade in; carrying on as it is, and copying it again at the next moment with no request in flight"`, "path=" + s.binary, tt.wantErr}
			if n := strings.Count(log.String(), `msg="can't copy the upgrade in`); n != 1 || !log.Has(warning...) {
				t.Errorf("log reads\n%s\nwant a line with %q, once", log, warning)
			}
			if n := strings.Count(log.String(), `msg="the upgrade still can't be copied in"`); n != 2 {
				t.Errorf("log reads\n%s\nwant the copies after the first noted at debug, twice, not %d times", log, n)
			}

			tt.fixed(t, s, own)
			r.look()
			if !r.restart(t.Context()) {
				t.Fatal("restart() = false once the copy can be made, want the router restarted")
			}
			if len(*asks) > 1 {
				t.Errorf("the binary was asked %d times, want once at most: nothing it was asked of has changed since", len(*asks))
			}
			checkCopy(t, own, s.version("1.1"))
			checkAlone(t, own)
		})
	}
}

func TestARestartAskedForFailsWhereTheUpgradeCantBeCopiedIn(t *testing.T) {
	log := logstest.Capture(t)
	s, own := newRunningCopy(t)
	r := copying(s, own, unheeded)
	asks := askedAt(t, r, binaryAnswer{})
	relink(t, s.binary, s.version("1.1"))
	chmod(t, s.version("1.1"), 0o000)

	err := askedFor(t, r)
	if want := "the router can't copy in the upgrade it would restart into: copy " + s.binary + " beside the router's copy, " + own + ": "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("restartable() = %v, want it refused, starting %q", err, want)
	}
	if r.restarting() || len(*asks) != 0 {
		t.Errorf("restarting: %v, and the binary asked %d times, want neither", r.restarting(), len(*asks))
	}
	if log.Has(`msg="can't copy the upgrade in`) {
		t.Errorf("log reads\n%s\nwant the refusal told only to whoever asked", log)
	}
	checkCopy(t, own, s.version("1.0"))
	checkAlone(t, own)
}

func TestARouterUpgradesFromTheBinaryItsGiven(t *testing.T) {
	const binary, from = "/test/data/switchboard/router/switchboard", "/test/opt/bin/switchboard"
	tests := []struct {
		name         string
		cfg          Config
		wantUpgrades string
		wantCopy     string
	}{
		{
			name:         "given one, running a copy of its own",
			cfg:          Config{Binary: Watched{path: binary}, From: Watched{path: from}},
			wantUpgrades: from,
			wantCopy:     binary,
		},
		{
			name:         "given none, as run by hand",
			cfg:          Config{Binary: Watched{path: from}},
			wantUpgrades: from,
		},
		{
			name: "given one, not knowing the binary it runs",
			cfg:  Config{From: Watched{path: from}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upgrades, own := tt.cfg.upgradesFrom()
			copied := ""
			if own != nil {
				copied = own.Path()
			}
			if upgrades.path != tt.wantUpgrades || copied != tt.wantCopy {
				t.Errorf("upgradesFrom() = %q, a copy at %q, want %q, a copy at %q", upgrades.path, copied, tt.wantUpgrades, tt.wantCopy)
			}
		})
	}
}

// newRunningCopy returns what a router running its own copy of switchboard
// was started from, as newStartedFrom gives it, each version of its binary
// built at a time of its own, as releases are, so its copy tells them apart;
// and the copy's path, in a data directory of the test's own, made of the
// binary its link leads to, as service install makes it.
func newRunningCopy(t *testing.T) (startedFrom, string) {
	t.Helper()
	s := newStartedFrom(t)
	built := time.Now().Add(-time.Hour)
	for i, v := range []string{"1.0", "1.1"} {
		at := built.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(s.version(v), at, at); err != nil {
			t.Fatal(err)
		}
	}
	own := filepath.Join(s.root, "data", "switchboard", "router", "switchboard")
	if err := routercopy.At(own).Make(s.binary); err != nil {
		t.Fatal(err)
	}
	return s, own
}

// copying returns the restarts of a supervised router running its own copy,
// at own, started from s, upgrading from its binary's link, as serve finds
// them, telling emit of its restarts.
func copying(s startedFrom, own string, emit func(Event)) *restarts {
	return restartsFor(Config{
		ConfigFile: Watch(s.config),
		Binary:     Watch(own),
		From:       WatchFrom(s.binary, own),
		Zone:       Watch(s.zone),
		Supervised: true,
		Now:        at(start),
	}, newInFlight(), emit)
}

// binaryAsked is a binary the router restarts into, asked whether it could
// start: by the path it was asked by, and what the file there held as it was.
type binaryAsked struct {
	path, held string
}

// askedAt has r ask the binary it restarts into whether it could start,
// answered as answers says, in turn, as askedOf does, and returns what was
// asked, as it was.
func askedAt(t *testing.T, r *restarts, answers ...binaryAnswer) *[]binaryAsked {
	t.Helper()
	asked := new([]binaryAsked)
	r.ask = func(_ context.Context, path string) (string, error) {
		held, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("asked %s: %v, want a binary there", path, err)
		}
		a := answers[min(len(*asked), len(answers)-1)]
		*asked = append(*asked, binaryAsked{path: path, held: string(held)})
		return a.refusal, a.err
	}
	return asked
}

// checkCopy checks the router's copy, at own, is one of the binary at want,
// as an upgrade copied in keeps it: what it holds, and when it was modified.
func checkCopy(t *testing.T, own, want string) {
	t.Helper()
	held, err := os.ReadFile(own)
	wanted, _ := os.ReadFile(want)
	if err != nil || string(held) != string(wanted) {
		t.Errorf("the copy holds %q (%v), want %q", held, err, wanted)
	}
	have, err := os.Stat(own)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := os.Stat(want); !have.ModTime().Equal(want.ModTime()) {
		t.Errorf("the copy was modified at %v, want %v, as the binary it's a copy of was", have.ModTime(), want.ModTime())
	}
}

// checkAlone checks the copy's directory holds the copy alone: nothing
// copied in left beside it.
func checkAlone(t *testing.T, own string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(own))
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(own) {
		t.Errorf("the copy's directory holds %v (%v), want the copy alone", entries, err)
	}
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func removeAll(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}
