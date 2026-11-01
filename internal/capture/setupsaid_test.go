package capture

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/shell"
)

// saidHome is a home of the test's own, in which setup's own steps say what
// the fixtures of setup's screen have them say: switchboard, a build that
// lasts, outside the temporary directory; Claude Code's own claude, in
// ~/.local/bin, on PATH; zsh the login shell, answering as answer has it;
// and switchboard's bin directory, the skill and Claude Code's settings
// where setup puts them.
type saidHome struct {
	home   string
	answer shell.Answer
	setup  *setup.Setup
}

// newSaidHome is a saidHome whose login shell runs Claude Code's own
// claude, reading its startup file from the home.
func newSaidHome(t *testing.T) *saidHome {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := &saidHome{home: filepath.Join(root, "home")}
	tmp := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		switch key {
		case "TMPDIR":
			return tmp
		case "SHELL":
			return "/bin/zsh"
		}
		return ""
	}
	switchboard := claudetest.Program(t, filepath.Join(root, "brew", "bin", "switchboard"))
	claudeCode := claudetest.Program(t, filepath.Join(h.home, ".local", "bin", "claude"))
	h.answer = shell.Answer{Kind: shell.Other, Claude: claudeCode, ConfigDir: h.home}
	svc, err := service.New(service.Config{GOOS: "darwin", Home: h.home, Getenv: getenv})
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(h.home, ".local", "share", "switchboard", "bin")
	h.setup = &setup.Setup{
		LaunchAgent: svc,
		Install:     service.InstallOptions{Executable: switchboard},
		Path:        filepath.Dir(claudeCode),
		Bin:         bin,
		Home:        h.home,
		Shells: shell.Files{Dir: filepath.Join(h.home, ".local", "share", "switchboard", "shell"), Bin: bin, Completion: func(sh shell.Shell) ([]byte, error) {
			return []byte("# switchboard's completions for " + string(sh) + "\n"), nil
		}},
		Login:     h,
		Getenv:    getenv,
		Interrupt: func(ctx context.Context) (context.Context, context.CancelFunc) { return context.WithCancel(ctx) },
		SkillFile: filepath.Join(h.home, ".claude", "skills", "switchboard", "SKILL.md"),
		Settings:  filepath.Join(h.home, ".claude", "settings.json"),
	}
	return h
}

// Which answers as the login shell does, as answer has it.
func (h *saidHome) Which(context.Context, string) shell.Answer {
	return h.answer
}

// checkSaid checks the fixture's row reads as setup's own step said it.
func checkSaid(t *testing.T, row string, fixture, said setup.Said) {
	t.Helper()
	if fixture != said {
		t.Errorf("the fixtures' %s reads %+v, want it as setup says it, %+v", row, fixture, said)
	}
}

// Where setup's screen's fixtures have setup say what its claude link step,
// its session hook and its skill step say, they say it in setup's own
// words, as setup's own steps say it of the same outcome: the claude link
// made; zsh running Claude Code's own claude, then switchboard's; the
// session hook installed; and the skill installed, then up to date.
func TestSetupsFixturesSayWhatSetupsStepsSay(t *testing.T) {
	now := setupMoment()
	left, through, passed := linkLeft(now), linkThrough(now), everyStepPassed(now)
	h := newSaidHome(t)

	found, err := h.setup.Link(t.Context())
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if len(found.Said) != 1 || found.Shell.Situation != left.link.Shell.Situation {
		t.Fatalf("Link() = %+v, want the link made, and the line offered, as the fixtures have it", found)
	}
	checkSaid(t, "claude link", left.link.Said[0], found.Said[0])
	checkSaid(t, "shell running Claude Code's own claude", left.link.Shell.Said, found.Shell.Said)

	h.answer = shell.Answer{Kind: shell.Link}
	again, err := h.setup.AskAgain(t.Context(), found)
	if err != nil {
		t.Fatalf("AskAgain() error = %v", err)
	}
	checkSaid(t, "shell running switchboard's claude", through.link.Shell.Said, again.Shell.Said)

	checkSaid(t, "session hook", left.hook, h.setup.SessionHook())

	installed, err := h.setup.Skill()
	if err != nil {
		t.Fatalf("Skill() error = %v", err)
	}
	checkSaid(t, "skill installed", passed.skill, installed)
	upToDate, err := h.setup.Skill()
	if err != nil {
		t.Fatalf("Skill() again error = %v", err)
	}
	checkSaid(t, "skill up to date", left.skill, upToDate)
}
