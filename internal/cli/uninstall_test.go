package cli_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/routercopy"
	"github.com/leeovery/switchboard/internal/sessionhook"
	"github.com/leeovery/switchboard/internal/shell"
	"github.com/leeovery/switchboard/internal/skill"
)

// uninstallWorld is a home with everything setup puts in place laid out in
// it, as commands run with deps find it, launchd faked as never having
// loaded the service, and bash the login shell, which uninstall needn't ask
// where it reads its startup file.
type uninstallWorld struct {
	deps cli.Deps
	home string
	// The places setup puts things, as the commands find them.
	plist, routerCopy, settings, skill, link, shellDir, dataDir, configDir, stateDir string
	// startupFiles are the shells' startup files, each holding switchboard's
	// line after theirs.
	startupFiles []string
}

// theirs is what each startup file and Claude Code's settings hold of the
// user's own.
const (
	theirs         = "export EDITOR=vi\n"
	theirSettings  = "{\n  \"model\": \"opus\"\n}\n"
	homebrewBinary = "/opt/homebrew/bin/switchboard"
)

func newUninstallWorld(t *testing.T) *uninstallWorld {
	t.Helper()
	home := t.TempDir()
	deps := testDeps(map[string]string{"SHELL": "/bin/bash"}, home)
	deps.Launchctl = func(context.Context, ...string) ([]byte, error) {
		// launchctl print's exit status for a service it hasn't loaded.
		return nil, exitStatus(113)
	}
	data := filepath.Join(home, ".local", "share", "switchboard")
	w := &uninstallWorld{
		deps:       deps,
		home:       home,
		plist:      filepath.Join(home, "Library", "LaunchAgents", "io.github.leeovery.switchboard.plist"),
		settings:   filepath.Join(home, ".claude", "settings.json"),
		skill:      filepath.Join(home, ".claude", "skills", "switchboard", "SKILL.md"),
		link:       filepath.Join(data, "bin", "claude"),
		shellDir:   filepath.Join(data, "shell"),
		dataDir:    data,
		configDir:  filepath.Join(home, ".config", "switchboard"),
		stateDir:   filepath.Join(home, ".local", "state", "switchboard"),
		routerCopy: routerCopyPath(t, deps),
		startupFiles: []string{
			filepath.Join(home, ".zshrc"),
			filepath.Join(home, ".bash_profile"),
			filepath.Join(home, ".config", "fish", "config.fish"),
		},
	}
	w.lay(t)
	return w
}

func routerCopyPath(t *testing.T, deps cli.Deps) string {
	t.Helper()
	own, err := routercopy.Find(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	return own.Path()
}

// lay puts in place what setup does, and the config and the state.
func (w *uninstallWorld) lay(t *testing.T) {
	t.Helper()
	writeAt(t, w.plist, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<plist version=\"1.0\">\n<dict>\n</dict>\n</plist>\n")
	writeAt(t, w.routerCopy, "#!/bin/sh\n")
	files := shell.Files{Dir: w.shellDir, Bin: filepath.Dir(w.link), Completion: func(shell.Shell) ([]byte, error) { return []byte("# completions\n"), nil }}
	if _, err := files.Write(); err != nil {
		t.Fatal(err)
	}
	for i, sh := range shell.Shells {
		writeAt(t, w.startupFiles[i], theirs+shell.Line(sh, shell.InitFile(w.shellDir, sh), w.home)+"\n")
	}
	writeAt(t, w.settings, theirSettings)
	if _, err := sessionhook.Install(w.settings, homebrewBinary); err != nil {
		t.Fatal(err)
	}
	if err := skill.Install(w.skill); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(w.link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(homebrewBinary, w.link); err != nil {
		t.Fatal(err)
	}
	writeAt(t, filepath.Join(w.configDir, "config.toml"), "[[account]]\nid = \"work\"\n")
	writeAt(t, filepath.Join(w.configDir, "themes", "lake.theme"), "")
	writeAt(t, filepath.Join(w.stateDir, "tokens", "work"), "test-token-work\n")
	writeAt(t, filepath.Join(w.stateDir, "ledger", "day-2026-11-08.json"), "{}\n")
}

// writeAt writes text to a file at path, making its directory.
func writeAt(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// uninstallRow is a line of uninstall's, as it prints it.
func uninstallRow(mark, label, detail string) string {
	return fmt.Sprintf("%s  %-12s  %s\n", mark, label, detail)
}

// uninstallEnd is what uninstall says after its pieces' lines.
const uninstallEnd = "\nThe accounts' setup tokens stay valid on Anthropic's side until their year is out, their files removed here or not.\n" +
	"Last, brew uninstall switchboard removes switchboard itself.\n"

// keptRow is the line uninstall without --all says of what it keeps.
var keptRow = uninstallRow("✓", "Kept", "the config, the accounts' token files and the history: switchboard uninstall --all removes them too")

// removedRows are uninstall's lines for w's pieces, each removed.
func (w *uninstallWorld) removedRows() string {
	return w.rowsBut(nil)
}

// rowsBut are uninstall's lines for w's pieces, each removed but those
// instead names, by their labels, which say what it gives in place of all
// that label's lines.
func (w *uninstallWorld) rowsBut(instead map[string]string) string {
	var rows strings.Builder
	said := make(map[string]bool)
	for _, row := range [][2]string{
		{"Service", "removed " + w.plist},
		{"Startup line", "removed from " + w.startupFiles[0]},
		{"Startup line", "removed from " + w.startupFiles[1]},
		{"Startup line", "removed from " + w.startupFiles[2]},
		{"Shell files", "removed from " + w.shellDir},
		{"Session hook", "removed from " + w.settings},
		{"Skill", "removed " + w.skill},
		{"claude link", "removed " + w.link},
		{"Data", "removed " + w.dataDir},
	} {
		if in, ok := instead[row[0]]; ok {
			if !said[row[0]] {
				rows.WriteString(in)
			}
			said[row[0]] = true
			continue
		}
		rows.WriteString(uninstallRow("✓", row[0], row[1]))
	}
	return rows.String()
}

// absentRows are uninstall's lines for pieces none of which is there.
var absentRows = uninstallRow("✓", "Service", "not there") +
	uninstallRow("✓", "Startup line", "not there") +
	uninstallRow("✓", "Shell files", "not there") +
	uninstallRow("✓", "Session hook", "not there") +
	uninstallRow("✓", "Skill", "not there") +
	uninstallRow("✓", "claude link", "not there")

// checkPiecesGone checks nothing setup put in place is left, the user's
// own lines and settings kept, and the notifications' app removed each time
// uninstall ran.
func (w *uninstallWorld) checkPiecesGone(t *testing.T, runs int) {
	t.Helper()
	for _, path := range []string{w.plist, w.routerCopy, w.dataDir, filepath.Dir(w.skill)} {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: %v, want it gone", path, err)
		}
	}
	for _, file := range w.startupFiles {
		checkText(t, file, theirs)
	}
	checkText(t, w.settings, theirSettings)
	if _, uninstalled := w.deps.Notifier.(*recordingNotifier).counts(); uninstalled != runs {
		t.Errorf("the notifications' app was removed %d times, want once each uninstall", uninstalled)
	}
}

// checkText checks the file at path reads want.
func checkText(t *testing.T, path, want string) {
	t.Helper()
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Errorf("%s reads %q (%v), want %q", path, got, err, want)
	}
}

// checkThere checks there's something at each of paths.
func checkThere(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s: %v, want it kept", path, err)
		}
	}
}

func TestUninstall(t *testing.T) {
	w := newUninstallWorld(t)

	if got, want := run(t, w.deps, "uninstall"), (result{stdout: w.removedRows() + keptRow + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall = %+v\nwant %+v", got, want)
	}
	w.checkPiecesGone(t, 1)
	checkThere(t, filepath.Join(w.configDir, "config.toml"), filepath.Join(w.configDir, "themes", "lake.theme"),
		filepath.Join(w.stateDir, "tokens", "work"), filepath.Join(w.stateDir, "ledger", "day-2026-11-08.json"))

	if got, want := run(t, w.deps, "uninstall"), (result{stdout: absentRows + keptRow + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall, again = %+v\nwant %+v", got, want)
	}
	w.checkPiecesGone(t, 2)
}

func TestUninstallAll(t *testing.T) {
	w := newUninstallWorld(t)

	allRows := uninstallRow("✓", "Config", "removed "+w.configDir) + uninstallRow("✓", "State", "removed "+w.stateDir)
	if got, want := run(t, w.deps, "uninstall", "--all"), (result{stdout: w.removedRows() + allRows + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall --all = %+v\nwant %+v", got, want)
	}
	w.checkPiecesGone(t, 1)
	for _, dir := range []string{w.configDir, w.stateDir} {
		if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: %v, want it gone", dir, err)
		}
	}

	againRows := uninstallRow("✓", "Config", "not there: "+w.configDir) + uninstallRow("✓", "State", "not there: "+w.stateDir)
	if got, want := run(t, w.deps, "uninstall", "--all"), (result{stdout: absentRows + againRows + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall --all, again = %+v\nwant %+v", got, want)
	}
}

func TestUninstallAllHonoursTheEnvironmentsPlaces(t *testing.T) {
	w := newUninstallWorld(t)
	elsewhere := t.TempDir()
	configFile := filepath.Join(elsewhere, "work.toml")
	themes := filepath.Join(elsewhere, "themes")
	state := filepath.Join(elsewhere, "state")
	writeAt(t, configFile, "[[account]]\nid = \"work\"\n")
	writeAt(t, filepath.Join(themes, "lake.theme"), "")
	// Another tool's file, beside the themes.
	writeAt(t, filepath.Join(themes, "starship.toml"), "")
	writeAt(t, filepath.Join(state, "switchboard", "tokens", "work"), "test-token-work\n")
	writeAt(t, filepath.Join(elsewhere, "kept"), "the user's")
	env := map[string]string{"SWITCHBOARD_CONFIG": configFile, "SWITCHBOARD_THEMES_DIR": themes, "XDG_STATE_HOME": state}
	w.deps.Getenv = func(key string) string { return env[key] }

	got := run(t, w.deps, "uninstall", "--all")
	for _, want := range []string{
		uninstallRow("✓", "Config", "removed "+w.configDir),
		uninstallRow("✓", "Config", "removed "+configFile),
		uninstallRow("✓", "Config", "removed the theme files from "+themes),
		uninstallRow("✓", "State", "removed "+filepath.Join(state, "switchboard")),
	} {
		if got.code != 0 || !strings.Contains(got.stdout, want) {
			t.Errorf("switchboard uninstall --all = %+v, want exit status 0, and a line\n%s", got, want)
		}
	}
	if left := entries(t, elsewhere); !slices.Equal(left, []string{"kept", "state", "themes"}) {
		t.Errorf("%s holds %q, want what the user keeps there alone, the state directory's parent, and the themes directory", elsewhere, left)
	}
	if left := entries(t, themes); !slices.Equal(left, []string{"starship.toml"}) {
		t.Errorf("the themes directory holds %q, want the other tool's file alone", left)
	}
	// The state directory XDG_STATE_HOME names is the one removed: the
	// home's is left.
	checkThere(t, filepath.Join(w.stateDir, "tokens", "work"))
}

func TestUninstallAllKeepsTheExportsFolder(t *testing.T) {
	w := newUninstallWorld(t)
	export := filepath.Join(w.stateDir, "exports")
	writeAt(t, filepath.Join(w.configDir, "config.toml"), "[[account]]\nid = \"work\"\n[export]\ndir = \"~/.local/state/switchboard/exports\"\n")
	writeAt(t, filepath.Join(export, "mac-1", "export.json"), "{}\n")

	got := run(t, w.deps, "uninstall", "--all")
	want := uninstallRow("✓", "Config", "removed "+w.configDir) +
		uninstallRow("✓", "State", "removed all but the export's folder from "+w.stateDir) +
		uninstallRow("✓", "Export", "kept "+export+": the export's folder, which uninstall never removes") +
		uninstallEnd
	if got.code != 0 || !strings.HasSuffix(got.stdout, want) {
		t.Errorf("switchboard uninstall --all = %+v, want exit status 0, ending\n%s", got, want)
	}
	checkText(t, filepath.Join(export, "mac-1", "export.json"), "{}\n")
	if left := entries(t, w.stateDir); !slices.Equal(left, []string{"exports"}) {
		t.Errorf("the state directory holds %q, want the export's folder alone", left)
	}
}

func TestUninstallAllKeepsTheStateDirectoryWhereItsTheExportsFolder(t *testing.T) {
	w := newUninstallWorld(t)
	writeAt(t, filepath.Join(w.configDir, "config.toml"), "[[account]]\nid = \"work\"\n[export]\ndir = \"~/.local/state/switchboard\"\n")

	got := run(t, w.deps, "uninstall", "--all")
	want := uninstallRow("✓", "Config", "removed "+w.configDir) +
		uninstallRow("✓", "State", "kept "+w.stateDir+": it's the export's folder") +
		uninstallEnd
	if got.code != 0 || !strings.HasSuffix(got.stdout, want) {
		t.Errorf("switchboard uninstall --all = %+v, want exit status 0, ending\n%s", got, want)
	}
	checkThere(t, filepath.Join(w.stateDir, "tokens", "work"))
}

func TestUninstallAllKeepsTheConfigAndStateWhereItCantTellWhereTheExportGoes(t *testing.T) {
	w := newUninstallWorld(t)
	writeAt(t, filepath.Join(w.configDir, "config.toml"), "[export\n")

	got := run(t, w.deps, "uninstall", "--all")
	if got.code != 1 || !strings.Contains(got.stdout, w.removedRows()) {
		t.Errorf("switchboard uninstall --all = %+v, want exit status 1, every piece removed all the same", got)
	}
	for _, label := range []string{"Config", "State"} {
		if want := uninstallRow("✗", label, "failed: kept, with the state directory, as where the export goes can't be told: parse config "); !strings.Contains(got.stdout, strings.TrimSuffix(want, "\n")) {
			t.Errorf("switchboard uninstall --all printed\n%s\nwant a line starting\n%s", got.stdout, want)
		}
	}
	checkThere(t, filepath.Join(w.configDir, "config.toml"), filepath.Join(w.stateDir, "tokens", "work"))
}

func TestUninstallGoesOnPastAPieceThatFails(t *testing.T) {
	tests := []struct {
		name string
		// breaks breaks a piece of w's.
		breaks func(t *testing.T, w *uninstallWorld)
		// instead are the lines uninstall says in place of the removed ones,
		// by their labels, as rowsBut has them.
		instead func(w *uninstallWorld) map[string]string
		// kept is what's left of the piece.
		kept func(w *uninstallWorld) []string
	}{
		{
			name: "the service, as launchd can't be asked: the data directory kept for the router's copy",
			breaks: func(_ *testing.T, w *uninstallWorld) {
				w.deps.Launchctl = func(context.Context, ...string) ([]byte, error) { return nil, errors.New("launchctl isn't there") }
			},
			instead: func(w *uninstallWorld) map[string]string {
				return map[string]string{
					"Service": uninstallRow("✗", "Service", "failed: launchctl print "+target+": launchctl isn't there"),
					"Data":    uninstallRow("!", "Data", "kept "+w.dataDir+": something's still in it"),
				}
			},
			kept: func(w *uninstallWorld) []string { return []string{w.plist, w.routerCopy} },
		},
		{
			name: "the notifications' app",
			breaks: func(_ *testing.T, w *uninstallWorld) {
				w.deps.Notifier.(*recordingNotifier).failUninstall = errors.New("remove the notifications' app: permission denied")
			},
			instead: func(w *uninstallWorld) map[string]string {
				return map[string]string{"Service": uninstallRow("✗", "Service", "removed "+w.plist+", but the notifications' app: remove the notifications' app: permission denied")}
			},
		},
		{
			name: "two startup files that can't be read: both said on the one line",
			breaks: func(t *testing.T, w *uninstallWorld) {
				for _, file := range w.startupFiles[:2] {
					if err := os.Remove(file); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(file, 0o700); err != nil {
						t.Fatal(err)
					}
				}
			},
			instead: func(w *uninstallWorld) map[string]string {
				return map[string]string{"Startup line": uninstallRow("✓", "Startup line", "removed from "+w.startupFiles[2]) +
					uninstallRow("✗", "Startup line", "failed: read "+w.startupFiles[0]+": is a directory; read "+w.startupFiles[1]+": is a directory")}
			},
		},
		{
			name:   "the session hook, in settings Claude Code can't read: left as they are",
			breaks: func(t *testing.T, w *uninstallWorld) { writeAt(t, w.settings, "not json") },
			instead: func(w *uninstallWorld) map[string]string {
				return map[string]string{"Session hook": uninstallRow("✗", "Session hook", "failed: "+w.settings+": line 1, column 2: invalid character 'o' in literal null (expecting 'u')")}
			},
			kept: func(w *uninstallWorld) []string { return []string{w.settings} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newUninstallWorld(t)
			tt.breaks(t, w)
			settings, _ := os.ReadFile(w.settings)

			got := run(t, w.deps, "uninstall")
			want := result{
				stdout: w.rowsBut(tt.instead(w)) + keptRow + uninstallEnd,
				stderr: "Error: some of switchboard couldn't be taken away, as said above: put it right, then run switchboard uninstall again\n",
				code:   1,
			}
			if got != want {
				t.Errorf("switchboard uninstall = %+v\nwant %+v", got, want)
			}
			if tt.kept != nil {
				checkThere(t, tt.kept(w)...)
			}
			if _, broken := tt.instead(w)["Session hook"]; broken {
				checkText(t, w.settings, string(settings))
			}
		})
	}
}

func TestUninstallLeavesAClaudeThatIsntSwitchboardsAlone(t *testing.T) {
	w := newUninstallWorld(t)
	if err := os.Remove(w.link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr/local/bin/claude", w.link); err != nil {
		t.Fatal(err)
	}

	got := run(t, w.deps, "uninstall")
	want := uninstallRow("!", "claude link", "left alone: "+w.link+" isn't switchboard's link, so it's someone else's")
	want += uninstallRow("!", "Data", "kept "+w.dataDir+": something's still in it")
	if got.code != 0 || !strings.Contains(got.stdout, want) {
		t.Errorf("switchboard uninstall = %+v, want exit status 0, the link left alone, and the data directory kept for it, as\n%s", got, want)
	}
	if target, err := os.Readlink(w.link); err != nil || target != "/usr/local/bin/claude" {
		t.Errorf("the claude in switchboard's bin directory leads to %q (%v), want it left alone", target, err)
	}
}

func TestUninstallLeavesAnIndentedLineWhereItIs(t *testing.T) {
	w := newUninstallWorld(t)
	profile := w.startupFiles[1]
	indented := "if [ -n \"$PS1\" ]; then\n  " + shell.Line(shell.Bash, shell.InitFile(w.shellDir, shell.Bash), w.home) + "\nfi\n"
	writeAt(t, profile, indented)

	got := run(t, w.deps, "uninstall")
	want := w.rowsBut(map[string]string{"Startup line": uninstallRow("✓", "Startup line", "removed from "+w.startupFiles[0]) +
		uninstallRow("✓", "Startup line", "removed from "+w.startupFiles[2]) +
		uninstallRow("!", "Startup line", "left alone: an indented copy in "+profile+", which may be all of a block of yours, so it's yours to take out: with switchboard's files gone it does nothing")})
	if want := (result{stdout: want + keptRow + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall = %+v\nwant %+v", got, want)
	}
	checkText(t, profile, indented)
}

func TestUninstallOffMacOSHasNoService(t *testing.T) {
	w := newUninstallWorld(t)
	w.deps.GOOS = "linux"

	got := run(t, w.deps, "uninstall")
	if want := uninstallRow("✓", "Service", "not there"); got.code != 0 || !strings.HasPrefix(got.stdout, want) {
		t.Errorf("switchboard uninstall = %+v, want exit status 0, starting\n%s", got, want)
	}
}

// entries are the names of what's in dir.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range des {
		names = append(names, d.Name())
	}
	return names
}

// zshIsTheLoginShell has uninstall's environment name zsh as the user's
// login shell, which run runs, and moves zsh's startup file, switchboard's
// line in it, into zdotdir, where only the login shell, not uninstall's
// environment, says zsh reads it.
func (w *uninstallWorld) zshIsTheLoginShell(t *testing.T, zdotdir string, run shell.Runner) {
	t.Helper()
	moved := filepath.Join(zdotdir, ".zshrc")
	if err := os.MkdirAll(zdotdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(w.startupFiles[0], moved); err != nil {
		t.Fatal(err)
	}
	w.startupFiles[0] = moved
	w.deps.Getenv = withEnv(w.deps.Getenv, "SHELL", "/bin/zsh")
	w.deps.Environ = func() []string { return []string{"HOME=" + w.home, "SHELL=/bin/zsh"} }
	w.deps.LoginShell = run
}

func TestUninstallTakesTheLineOutWhereTheLoginShellSaysItReadsIt(t *testing.T) {
	w := newUninstallWorld(t)
	zdotdir := filepath.Join(w.home, ".config", "zsh")
	var asked []string
	w.zshIsTheLoginShell(t, zdotdir, func(_ context.Context, path string, _, _ []string) ([]byte, error) {
		asked = append(asked, path)
		return []byte("switchboard-asks-which-claude\nis:\npath:\nconfig:" + zdotdir + "\nswitchboard-asked-which-claude\n"), nil
	})

	if got, want := run(t, w.deps, "uninstall"), (result{stdout: w.removedRows() + keptRow + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall = %+v\nwant %+v", got, want)
	}
	if want := []string{"/bin/zsh"}; !slices.Equal(asked, want) {
		t.Errorf("the login shells asked = %q, want %q", asked, want)
	}
	w.checkPiecesGone(t, 1)
}

func TestUninstallSaysItCouldntLookWhereTheLoginShellCantBeAsked(t *testing.T) {
	w := newUninstallWorld(t)
	w.zshIsTheLoginShell(t, filepath.Join(w.home, ".config", "zsh"), func(context.Context, string, []string, []string) ([]byte, error) {
		return nil, errors.New("zsh isn't there")
	})
	zshrc, _ := os.ReadFile(w.startupFiles[0])

	got := run(t, w.deps, "uninstall")
	want := w.rowsBut(map[string]string{"Startup line": uninstallRow("✓", "Startup line", "removed from "+w.startupFiles[1]) +
		uninstallRow("✓", "Startup line", "removed from "+w.startupFiles[2]) +
		uninstallRow("!", "Startup line", "couldn't look where zsh reads its startup file, as it couldn't be asked: zsh isn't there")})
	if want := (result{stdout: want + keptRow + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall = %+v\nwant %+v", got, want)
	}
	checkText(t, w.startupFiles[0], string(zshrc))

	got = run(t, w.deps, "uninstall")
	if want := uninstallRow("!", "Startup line", "couldn't look where zsh reads its startup file, as it couldn't be asked: zsh isn't there"); !strings.Contains(got.stdout, want) || strings.Contains(got.stdout, "Startup line  not there") {
		t.Errorf("switchboard uninstall, again = %+v\nwant it to say, never that the line's not there\n%s", got, want)
	}
}

func TestUninstallDoesntAskBash(t *testing.T) {
	w := newUninstallWorld(t)
	asked := 0
	w.deps.LoginShell = func(context.Context, string, []string, []string) ([]byte, error) {
		asked++
		return nil, errors.New("bash is never asked")
	}

	if got, want := run(t, w.deps, "uninstall"), (result{stdout: w.removedRows() + keptRow + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall = %+v\nwant %+v", got, want)
	}
	if asked != 0 {
		t.Errorf("the login shell was asked %d times, want bash never asked: its startup files are in the home", asked)
	}
}

func TestUninstallWithoutAShellSaysItCouldntLook(t *testing.T) {
	w := newUninstallWorld(t)
	w.deps.Getenv = withEnv(w.deps.Getenv, "SHELL", "")
	couldnt := uninstallRow("!", "Startup line", "couldn't look where your login shell reads its startup file, as it couldn't be asked: $SHELL isn't set")

	got := run(t, w.deps, "uninstall")
	want := w.rowsBut(map[string]string{"Startup line": uninstallRow("✓", "Startup line", "removed from "+w.startupFiles[0]) +
		uninstallRow("✓", "Startup line", "removed from "+w.startupFiles[1]) +
		uninstallRow("✓", "Startup line", "removed from "+w.startupFiles[2]) + couldnt})
	if want := (result{stdout: want + keptRow + uninstallEnd}); got != want {
		t.Errorf("switchboard uninstall = %+v\nwant %+v", got, want)
	}

	got = run(t, w.deps, "uninstall")
	if !strings.Contains(got.stdout, couldnt) || strings.Contains(got.stdout, "Startup line  not there") {
		t.Errorf("switchboard uninstall, again = %+v\nwant it to say, never that the line's not there\n%s", got, couldnt)
	}
}

func TestUninstallInterruptedAsItAsksTheLoginShellStops(t *testing.T) {
	w := newUninstallWorld(t)
	w.zshIsTheLoginShell(t, filepath.Join(w.home, ".config", "zsh"), func(ctx context.Context, _ string, _, _ []string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	w.deps.Interrupt = func(ctx context.Context) (context.Context, context.CancelFunc) {
		ctx, interrupt := context.WithCancel(ctx)
		interrupt()
		return ctx, interrupt
	}
	zshrc, _ := os.ReadFile(w.startupFiles[0])

	got := run(t, w.deps, "uninstall")
	want := result{stdout: uninstallRow("✓", "Service", "removed "+w.plist), stderr: "Error: interrupted\n", code: 130}
	if got != want {
		t.Errorf("switchboard uninstall = %+v\nwant %+v", got, want)
	}
	checkText(t, w.startupFiles[0], string(zshrc))
	checkThere(t, w.shellDir, w.link, w.skill)
}
