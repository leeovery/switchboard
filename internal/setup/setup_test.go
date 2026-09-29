package setup_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/skill"
)

// firstRun are the answers of a user setting switchboard up from nothing:
// work and side, with their tokens, work the primary, and priming over
// 08:00-23:00.
var firstRun = []string{
	"work", "Work", "test-token-work",
	"y", "side", "Side", "test-token-side",
	"",            // no more accounts
	"",            // work, the first, the primary
	"08:00-23:00", // the day
}

func TestAFirstRunSetsEverythingUp(t *testing.T) {
	w := newWorld(t)

	shown := w.runs(t, firstRun...)

	checkGolden(t, "first-run", shown)
	if got := w.readConfig(t); got != doneConfig {
		t.Errorf("the config reads\n%s\nwant\n%s", got, doneConfig)
	}
	w.checkToken(t, "work", "test-token-work")
	w.checkToken(t, "side", "test-token-side")
	if want := []string{"test-token-work", "test-token-side"}; !slices.Equal(w.api.asked, want) {
		t.Errorf("the API was asked about %q, want %q, each token checked before it's saved", w.api.asked, want)
	}
	if ran, want := w.launchd.ran(), []string{"print", "print", "bootstrap"}; !slices.Equal(ran, want) {
		t.Errorf("ran launchctl %q, want %q: the service installed", ran, want)
	}
	if plist, err := os.ReadFile(w.plist()); err != nil || !strings.Contains(string(plist), "<string>"+w.switchboard+"</string>\n\t\t<string>serve</string>") {
		t.Errorf("the plist reads\n%s(%v)\nwant it to serve with switchboard by the path it was run by, %s", plist, err, w.switchboard)
	}
	link := filepath.Join(w.bin, "claude")
	if target, err := os.Readlink(link); err != nil || target != w.switchboard {
		t.Errorf("%s leads to %q (%v), want %s, the path switchboard was run by, which an upgrade moves on", link, target, err, w.switchboard)
	}
	if found, err := skill.Refresh(w.skill); err != nil || !found.Installed || found.Rewritten || found.Was != skill.Version() {
		t.Errorf("the skill at %s: %+v, %v; want this switchboard's installed", w.skill, found, err)
	}
}

func TestSetupLogsWhatItChangesButNoToken(t *testing.T) {
	w := newWorld(t)
	log := logstest.Capture(t)

	w.runs(t, firstRun...)

	for _, want := range [][]string{
		{"level=INFO", `msg="saved a token"`, "account=work"},
		{"level=INFO", `msg="added an account"`, "account=side"},
		{"level=INFO", `msg="marked the primary"`, "component=setup", "account=work"},
		{"level=INFO", `msg="set the priming day"`, "component=setup", "day=08:00-23:00"},
		{"level=INFO", `msg="installed the service"`},
		{"level=INFO", `msg="linked claude to switchboard"`, "component=setup", "link=" + filepath.Join(w.bin, "claude"), "switchboard=" + w.switchboard},
		{"level=INFO", `msg="installed the skill"`, "component=setup", "path=" + w.skill},
	} {
		if !log.Has(want...) {
			t.Errorf("the log reads\n%s\nwant a line with %q", log, want)
		}
	}
	for _, token := range []string{"test-token-work", "test-token-side"} {
		if strings.Contains(log.String(), token) {
			t.Errorf("the log shows a token:\n%s", log)
		}
	}
}

func TestRunningSetupAgainDoesNothingNew(t *testing.T) {
	w := newWorld(t)
	w.runs(t, firstRun...)
	// The user adds the line the first run said to add.
	w.putBinOnPath()
	before := w.snapshot(t)
	w.launchd.calls, w.api.asked = nil, nil

	shown := w.runs(t, "") // no more accounts

	checkGolden(t, "run-again", shown)
	w.checkUnchanged(t, before)
	if ran, want := w.launchd.ran(), []string{"print"}; !slices.Equal(ran, want) {
		t.Errorf("ran launchctl %q, want %q: asked whether the service is loaded, and nothing done", ran, want)
	}
	if len(w.api.asked) > 0 {
		t.Errorf("the API was asked about %q, want nothing", w.api.asked)
	}
}

func TestSetupStopsWhenTheInputEnds(t *testing.T) {
	w := newWorld(t)

	shown, err := w.run(t, "work", "Work", "test-token-work")
	want := "the input ended without an answer, so setup stops here: run it again to carry on"
	if err == nil || err.Error() != want {
		t.Fatalf("Run() error = %v, want %q", err, want)
	}
	if !strings.HasSuffix(shown, "Added work.\nAdd another account? [y/N] \n") {
		t.Errorf("the terminal showed\n%s\nwant it to end at the question left unanswered", shown)
	}
	w.checkToken(t, "work", "test-token-work")
	if w.launchd.calls != nil {
		t.Errorf("ran launchctl %q, want the service left alone", w.launchd.calls)
	}
}

func TestSetupStopsAtAnInterrupt(t *testing.T) {
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay     func(t *testing.T, w *world)
		answers []string
		// wantEnd is how what the terminal showed ends.
		wantEnd string
	}{
		{
			name:    "taking a missing token",
			lay:     func(t *testing.T, w *world) { w.removeToken(t, "side") },
			answers: []string{interrupt},
			wantEnd: "Paste side's token, from claude setup-token run while signed in to that subscription (it won't show): \n",
		},
		{
			name:    "taking an account's token as it's added",
			answers: []string{"y", "spare", "", interrupt},
			wantEnd: "Paste spare's token, from claude setup-token run while signed in to that subscription (it won't show): \n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if tt.lay != nil {
				tt.lay(t, w)
			}
			before := w.snapshot(t)

			shown, err := w.run(t, tt.answers...)
			if !errors.Is(err, accounts.ErrInterrupted) {
				t.Errorf("Run() error = %v, want one matching %v", err, accounts.ErrInterrupted)
			}
			if !strings.HasSuffix(shown, tt.wantEnd) {
				t.Errorf("the terminal showed\n%s\nwant it to end, with nothing more asked, at\n%s", shown, tt.wantEnd)
			}
			w.checkUnchanged(t, before)
			if w.launchd.calls != nil {
				t.Errorf("ran launchctl %q, want the service left alone", w.launchd.calls)
			}
		})
	}
}

func TestSetupNeedsAnAccount(t *testing.T) {
	w := newWorld(t)
	before := w.snapshot(t)

	shown, err := w.run(t, "") // no id for the first account
	want := "no account is configured, so setup stops here: run it again to add one, or switchboard accounts add <id>"
	if err == nil || err.Error() != want {
		t.Errorf("Run() error = %v, want %q; the terminal showed\n%s", err, want, shown)
	}
	w.checkUnchanged(t, before)
}

func TestSetupRefusesATemporaryBuildBeforeItAsksAnything(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	built := filepath.Join(w.tmp, "go-build", "switchboard")
	writeFile(t, built, "#!/bin/sh\n", 0o700)
	w.switchboard = built
	before := w.snapshot(t)

	shown, err := w.run(t)
	want := "this switchboard is a temporary build, " + built + ", which won't last: use one that does, such as Homebrew's or one go install built"
	if err == nil || err.Error() != want || shown != "" {
		t.Errorf("Run() error = %v, the terminal showing %q; want %q, and nothing shown", err, shown, want)
	}
	w.checkUnchanged(t, before)
}

func TestSetupStopsAtAStepThatFails(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	if err := os.Remove(w.skill); err != nil {
		t.Fatal(err)
	}
	// A config that isn't TOML, as a hand edit can leave it.
	w.writeConfig(t, "listen = \"127.0.0.1:4747\n")

	shown, err := w.run(t)
	if err == nil || !strings.HasPrefix(err.Error(), "parse config "+w.config+": toml: line 1") {
		t.Errorf("Run() error = %v, want the config's", err)
	}
	if strings.Contains(shown, "2. Priming") {
		t.Errorf("the terminal showed\n%s\nwant setup stopped at the accounts", shown)
	}
	if _, err := os.Stat(w.skill); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the skill: %v, want none written", err)
	}
}
