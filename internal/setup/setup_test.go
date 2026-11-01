package setup_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/setup"
)

// firstRun takes each step of a first run as setup's screen does, from
// nothing: work and side added with their tokens pasted, priming over
// 08:00-23:00, the service, the claude link, switchboard's line added to
// .zshrc, the session hook, and the skill.
func firstRun(t *testing.T, s *setup.Setup) {
	t.Helper()
	for _, a := range []config.NewAccount{{ID: "work", Label: "Work"}, {ID: "side", Label: "Side"}} {
		got, err := s.PasteToken(t.Context(), setup.TokenFor{Account: a, Add: true}, "test-token-"+a.ID)
		if err != nil || got.Result != setup.TokenSaved {
			t.Fatalf("PasteToken(%s) = %+v, %v; want it saved", a.ID, got, err)
		}
	}
	if _, err := s.SetDay("08:00-23:00"); err != nil {
		t.Fatalf("SetDay() error = %v", err)
	}
	if _, err := s.Service(t.Context()); err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	found, err := s.Link(t.Context())
	if err != nil || found.Shell.Situation != setup.OfferLine {
		t.Fatalf("Link() = %+v, %v; want switchboard's line offered", found.Shell, err)
	}
	if _, err := s.AddLine(found); err != nil {
		t.Fatalf("AddLine() error = %v", err)
	}
	s.SessionHook()
	if _, err := s.Skill(); err != nil {
		t.Fatalf("Skill() error = %v", err)
	}
}

func TestTheStepsLogWhatTheyChangeButNoToken(t *testing.T) {
	w := newWorld(t)
	log := logstest.Capture(t)

	firstRun(t, w.setup(t))

	for _, want := range [][]string{
		{"level=INFO", `msg="added an account"`, "account=work"},
		{"level=INFO", `msg="added an account"`, "account=side"},
		{"level=INFO", `msg="set the priming day"`, "component=setup", "day=08:00-23:00"},
		{"level=INFO", `msg="installed the service"`},
		{"level=INFO", `msg="linked claude to switchboard"`, "component=setup", "link=" + filepath.Join(w.bin, "claude"), "switchboard=" + w.switchboard},
		{"level=INFO", `msg="wrote switchboard's file for a shell"`, "component=setup", "path=" + filepath.Join(w.shellDir, "init.zsh")},
		{"level=INFO", `msg="added switchboard's line to the shell's startup file"`, "component=setup", "path=" + filepath.Join(w.home, ".zshrc")},
		{"level=INFO", `msg="installed the session hook"`, "component=setup", "path=" + w.settings},
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

// Each step taken again on what setup has set up changes nothing, and asks
// launchd only whether the service is loaded.
func TestTakingEachStepAgainChangesNothing(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	before := w.snapshot(t)
	s := w.setup(t)

	if _, err := s.SetDay("08:00-23:00"); err != nil {
		t.Fatalf("SetDay() error = %v", err)
	}
	if _, err := s.Service(t.Context()); err != nil {
		t.Fatalf("Service() error = %v", err)
	}
	if found, err := s.Link(t.Context()); err != nil || found.Shell.Situation != setup.Through {
		t.Fatalf("Link() = %+v, %v; want claude going through switchboard", found.Shell, err)
	}
	if said := s.SessionHook(); said.Mark != setup.Done {
		t.Errorf("SessionHook() = %+v, want it in place", said)
	}
	if _, err := s.Skill(); err != nil {
		t.Fatalf("Skill() error = %v", err)
	}

	w.checkUnchanged(t, before)
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q: asked whether the service is loaded, and nothing done", ran, untouched)
	}
}

func TestATemporaryBuildIsRefusedBeforeAnyStepChangesAnything(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	built := filepath.Join(w.tmp, "go-build", "switchboard")
	writeFile(t, built, "#!/bin/sh\n", 0o700)
	w.switchboard = built
	before := w.snapshot(t)
	s := w.setup(t)
	want := "this switchboard is a temporary build, " + built + ", which won't last: use one that does, such as Homebrew's or one go install built"

	if _, err := s.Binary(); err == nil || err.Error() != want {
		t.Errorf("Binary() error = %v, want %q", err, want)
	}
	if _, err := s.Service(t.Context()); err == nil || err.Error() != want {
		t.Errorf("Service() error = %v, want %q", err, want)
	}
	if _, err := s.Link(t.Context()); err == nil || err.Error() != want {
		t.Errorf("Link() error = %v, want %q", err, want)
	}
	if said := s.SessionHook(); said.Mark != setup.Warning || !strings.HasSuffix(said.Detail, want) {
		t.Errorf("SessionHook() = %+v, want a warning saying %q", said, want)
	}
	w.checkUnchanged(t, before)
}
