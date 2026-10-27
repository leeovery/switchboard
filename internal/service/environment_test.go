package service_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// installedFrom puts the plist install would write for opts, from an
// environment env sets, in the service's place, every value's <root> the
// setup's root, and returns the service as a shell setting here sees it,
// asking routers at another state directory as at gives them.
func (s *setup) installedFrom(t *testing.T, env map[string]string, opts service.InstallOptions, here map[string]string, at func(dir string) service.Router) *service.Service {
	t.Helper()
	rooted := func(m map[string]string) func(string) string {
		return func(key string) string {
			if key == "TMPDIR" {
				return s.tmp
			}
			return strings.ReplaceAll(m[key], "<root>", s.root)
		}
	}
	cfg := s.cfg
	cfg.Getenv = rooted(env)
	opts.Executable = s.binary
	written, err := newService(t, cfg).PlistOf(opts)
	if err != nil {
		t.Fatal(err)
	}
	s.putPlist(t, string(written))
	cfg.Getenv, cfg.RouterAt = rooted(here), at
	return newService(t, cfg)
}

// down is a router that doesn't answer.
func down(t *testing.T) service.Router {
	return &fakeRouter{t: t, answer: notRunning}
}

// notRunning answers as no router does.
func notRunning(int) (router.Health, error) {
	return router.Health{}, errNotRunning
}

func TestForTakesTheHomeThePlistSets(t *testing.T) {
	s := newSetup(t, nil, notRunning)
	shell := s.installedFrom(t, map[string]string{"HOME": "<root>/elsewhere"}, service.InstallOptions{}, nil, func(string) service.Router { return down(t) })
	// A HOME launchd sets isn't one switchboard carries: it's there only
	// where it was added by hand.
	editPlist(t, s.plist, func(plist string) string {
		return strings.Replace(plist, "\t<key>RunAtLoad</key>", "\t<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>HOME</key>\n\t\t<string>"+s.root+"/elsewhere</string>\n\t</dict>\n\t<key>RunAtLoad</key>", 1)
	})
	opts, err := shell.Options(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	svc, err := shell.For(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	places, err := svc.Places(opts)
	want := service.Places{
		Tokens:     filepath.Join(s.root, "elsewhere", ".local", "state", "switchboard", "tokens"),
		ConfigFile: filepath.Join(s.root, "elsewhere", ".config", "switchboard", "config.toml"),
		ClaudeDir:  filepath.Join(s.root, "elsewhere", ".claude"),
	}
	if err != nil || places != want {
		t.Errorf("Places() = %+v, %v; want %+v, under the home the plist sets", places, err, want)
	}
	if svc.Plist() != s.plist {
		t.Errorf("Plist() = %s, want %s, in the user's home whatever the router's", svc.Plist(), s.plist)
	}
}

func TestForAsksTheRouterAtTheStateDirectoryAskedFromToo(t *testing.T) {
	s := newSetup(t, nil, func(int) (router.Health, error) { return up(4242) })
	s.launchctl.loaded = true
	// The plist names another state home, but launchd runs the router at
	// the one asked from, as when launchctl setenv set it.
	shell := s.installedFrom(t, map[string]string{"XDG_STATE_HOME": "<root>/custom"}, service.InstallOptions{}, nil, func(string) service.Router { return down(t) })
	opts, err := shell.Options(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := shell.For(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	st, err := svc.Status(t.Context())
	if err != nil || st.Router == nil || st.Router.PID != 4242 {
		t.Errorf("Status() = %+v, %v; want the router at the state directory asked from answering", st, err)
	}
	if h := svc.Answering(t.Context()); h == nil || h.PID != 4242 {
		t.Errorf("Answering() = %+v, want the router at the state directory asked from", h)
	}
}

func TestRestartNeverKickstartsWhileARouterAnswers(t *testing.T) {
	s := newSetup(t, nil, func(asked int) (router.Health, error) {
		if asked == 0 {
			return up(4242)
		}
		return upAgain(4242)
	})
	s.router.restart = tookRestart(healthOf(up(4242)), true)
	s.launchctl.loaded = true
	shell := s.installedFrom(t, map[string]string{"XDG_STATE_HOME": "<root>/custom"}, service.InstallOptions{}, nil, func(string) service.Router { return down(t) })
	opts, err := shell.Options(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := shell.For(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	h, err := svc.Restart(t.Context(), func(service.Restarting) {})
	if err != nil || h == nil || s.router.restarts != 1 {
		t.Errorf("Restart() = %+v, %v, the router asked to restart %d times; want the router answering asked, once", h, err, s.router.restarts)
	}
	if want := [][]string{{"print", target}}; !reflect.DeepEqual(s.launchctl.calls, want) {
		t.Errorf("ran launchctl %q, want %q: never a kickstart while a router answers", s.launchctl.calls, want)
	}
}

func TestPlacesApart(t *testing.T) {
	tests := []struct {
		name string
		// installedEnv and installed are what the service was installed
		// with, here the environment setup runs in, and setup what it's
		// given; every <root> the setup's root.
		installedEnv, here map[string]string
		installed, setup   service.InstallOptions
		want               []string
		wantShared         bool
	}{
		{name: "all the same", wantShared: true},
		{
			name:         "SWITCHBOARD_CONFIG set otherwise, finding the same file through a link",
			installedEnv: map[string]string{"SWITCHBOARD_CONFIG": "<root>/work.toml"},
			here:         map[string]string{"SWITCHBOARD_CONFIG": "<root>/linked.toml"},
			wantShared:   true,
		},
		{
			name:         "the config given, XDG_CONFIG_HOME set otherwise",
			installedEnv: map[string]string{"XDG_CONFIG_HOME": "<root>/config"},
			installed:    service.InstallOptions{Config: "<root>/work.toml"},
			setup:        service.InstallOptions{Config: "<root>/work.toml"},
			wantShared:   true,
		},
		{
			name:         "a state home set to the one its absence finds",
			installedEnv: map[string]string{"XDG_STATE_HOME": "<root>/home/.local/state", "CLAUDE_CONFIG_DIR": "<root>/home/.claude"},
			wantShared:   true,
		},
		{
			name:         "log levels, which find nothing",
			installedEnv: map[string]string{"SWITCHBOARD_LOG_LEVEL": "info"},
			here:         map[string]string{"SWITCHBOARD_LOG_LEVEL": "DEBUG"},
			installed:    service.InstallOptions{LogLevel: "debug"},
			wantShared:   true,
		},
		{
			name:         "every place elsewhere",
			installedEnv: map[string]string{"XDG_STATE_HOME": "<root>/custom", "SWITCHBOARD_CONFIG": "<root>/other.toml", "CLAUDE_CONFIG_DIR": "<root>/claude"},
			want:         []string{"state directory", "config file", "Claude Code config directory"},
		},
		{
			name:         "Claude Code's config alone",
			installedEnv: map[string]string{"CLAUDE_CONFIG_DIR": "<root>/claude"},
			want:         []string{"Claude Code config directory"},
			wantShared:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, notRunning)
			writeFile(t, filepath.Join(s.root, "work.toml"))
			if err := os.Symlink(filepath.Join(s.root, "work.toml"), filepath.Join(s.root, "linked.toml")); err != nil {
				t.Fatal(err)
			}
			tt.installed.Config = strings.ReplaceAll(tt.installed.Config, "<root>", s.root)
			tt.setup.Config = strings.ReplaceAll(tt.setup.Config, "<root>", s.root)
			shell := s.installedFrom(t, tt.installedEnv, tt.installed, tt.here, func(string) service.Router { return down(t) })
			opts, err := shell.Options(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			svc, err := shell.For(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			routers, err := svc.Places(opts)
			if err != nil {
				t.Fatal(err)
			}
			setups, err := shell.Places(tt.setup)
			if err != nil {
				t.Fatal(err)
			}

			if apart := routers.Apart(setups); !slices.Equal(apart, tt.want) {
				t.Errorf("Apart() = %q, want %q, for the router's %+v and setup's %+v", apart, tt.want, routers, setups)
			}
			if shared := routers.Shared(setups); shared != tt.wantShared {
				t.Errorf("Shared() = %v, want %v", shared, tt.wantShared)
			}
		})
	}
}

func TestInstallKeepsTheVariablesAddedByHand(t *testing.T) {
	s := newSetup(t, map[string]string{"XDG_STATE_HOME": "<root>/now"}, upOnceStarted(4242))
	s.putPlist(t, `<plist><dict><key>ProgramArguments</key><array><string>/elsewhere/switchboard</string><string>serve</string></array>`+
		`<key>EnvironmentVariables</key><dict><key>XDG_STATE_HOME</key><string>/Users/tester/then</string><key>XDG_CONFIG_HOME</key><string>/Users/tester/config</string>`+
		`<key>HTTPS_PROXY</key><string>http://proxy.example:8080</string></dict></dict></plist>`)

	if _, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	plist, err := os.ReadFile(s.plist)
	if err != nil {
		t.Fatal(err)
	}
	want := "\t<dict>\n\t\t<key>XDG_STATE_HOME</key>\n\t\t<string>" + s.root + "/now</string>\n\t\t<key>HTTPS_PROXY</key>\n\t\t<string>http://proxy.example:8080</string>\n\t</dict>"
	if !strings.Contains(string(plist), want) {
		t.Errorf("the plist reads\n%s\nwant the variables carried as they're set here, those added by hand kept:\n%s", plist, want)
	}
}

// editPlist rewrites the plist at path as edit says.
func editPlist(t *testing.T, path string, edit func(string) string) {
	t.Helper()
	plist, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(edit(string(plist))), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeFile writes an empty file at path.
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsReadsABinaryPlistThroughPlutil(t *testing.T) {
	s := newSetup(t, nil, nil)
	written, err := s.svc.PlistOf(service.InstallOptions{Executable: s.binary, LogLevel: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	// As defaults writes it, a variable added.
	s.putPlist(t, "bplist00\xd1\x01\x02")
	var ran [][]string
	cfg := s.cfg
	cfg.Plutil = func(_ context.Context, args ...string) ([]byte, error) {
		ran = append(ran, args)
		return []byte(strings.Replace(string(written), "\t<key>RunAtLoad</key>",
			"\t<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>HTTPS_PROXY</key>\n\t\t<string>http://proxy.example:8080</string>\n\t</dict>\n\t<key>RunAtLoad</key>", 1)), nil
	}

	opts, err := newService(t, cfg).Options(t.Context())
	if err != nil || opts.Executable != s.binary || opts.LogLevel != "debug" {
		t.Errorf("Options() = %+v, %v; want the options the binary plist holds", opts, err)
	}
	if want := [][]string{{"-convert", "xml1", "-o", "-", s.plist}}; !reflect.DeepEqual(ran, want) {
		t.Errorf("ran plutil %q, want %q", ran, want)
	}
	if _, err := newService(t, s.cfg).Options(t.Context()); err == nil || !strings.Contains(err.Error(), "plutil") {
		t.Errorf("Options() without plutil error = %v, want it saying there's no plutil to read a binary plist", err)
	}
}

func TestInstallKeepsTheVariablesABinaryPlistSets(t *testing.T) {
	s := newSetup(t, nil, upOnceStarted(4242))
	s.putPlist(t, "bplist00\xd1\x01\x02")
	cfg := s.cfg
	cfg.Plutil = func(context.Context, ...string) ([]byte, error) {
		return []byte(`<plist><dict><key>ProgramArguments</key><array><string>/elsewhere/switchboard</string><string>serve</string></array>` +
			`<key>EnvironmentVariables</key><dict><key>HTTPS_PROXY</key><string>http://proxy.example:8080</string></dict></dict></plist>`), nil
	}

	if _, err := newService(t, cfg).Install(t.Context(), service.InstallOptions{Executable: s.binary}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if plist, err := os.ReadFile(s.plist); err != nil || !strings.Contains(string(plist), "<key>HTTPS_PROXY</key>") {
		t.Errorf("the plist reads\n%s(%v)\nwant the variable the binary plist set kept", plist, err)
	}
}

func TestForTakesTheStateDirectoryOfTheRouterThatAnswers(t *testing.T) {
	s := newSetup(t, nil, func(int) (router.Health, error) { return up(4242) })
	shell := s.installedFrom(t, map[string]string{"XDG_STATE_HOME": "<root>/custom"}, service.InstallOptions{}, nil, func(string) service.Router { return down(t) })
	opts, err := shell.Options(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	svc, err := shell.For(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if want := s.cfg.StateDir; svc.StateDir() != want || svc.Log() != service.LogIn(want) {
		t.Errorf("StateDir() = %s, Log() = %s; want %s, the answering router's, and its log", svc.StateDir(), svc.Log(), want)
	}
	places, err := svc.Places(opts)
	if want := filepath.Join(s.cfg.StateDir, "tokens"); err != nil || places.Tokens != want {
		t.Errorf("Places() = %+v, %v; want the token files at %s", places, err, want)
	}
}

func TestInstallSeesTheRouterInTheHomeThePlistSets(t *testing.T) {
	s := newSetup(t, nil, nil)
	var asked []string
	s.cfg.RouterAt = func(dir string) service.Router {
		asked = append(asked, dir)
		return &fakeRouter{t: t, answer: func(asked int) (router.Health, error) { return upAfter(asked, 1, 4242) }}
	}
	s.svc = newService(t, s.cfg)
	s.putPlist(t, `<plist><dict><key>ProgramArguments</key><array><string>`+s.binary+`</string><string>serve</string></array>`+
		`<key>EnvironmentVariables</key><dict><key>HOME</key><string>`+s.root+`/other</string></dict></dict></plist>`)
	s.router.answer, s.router.refuse = notRunning, false

	installed, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary})
	if err != nil || installed.Router == nil {
		t.Fatalf("Install() = %+v, %v; want the router at the state directory the home decides answering", installed, err)
	}
	stateDir := filepath.Join(s.root, "other", ".local", "state", "switchboard")
	if !slices.Contains(asked, stateDir) {
		t.Errorf("asked for the routers at %q, want the one at %s", asked, stateDir)
	}
	if plist, err := os.ReadFile(s.plist); err != nil || !strings.Contains(string(plist), "<string>"+service.LogIn(stateDir)+"</string>") {
		t.Errorf("the plist reads\n%s(%v)\nwant launchd's log in %s", plist, err, stateDir)
	}
}

func TestOptionsReadsARelativePathAsLaunchdDoes(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.putPlist(t, `<plist><dict><key>ProgramArguments</key><array><string>`+s.binary+`</string><string>serve</string><string>--config</string><string>work.toml</string></array>`+
		`<key>EnvironmentVariables</key><dict><key>SWITCHBOARD_CONFIG</key><string>config/other.toml</string><key>CLAUDE_CONFIG_DIR</key><string>claude</string></dict></dict></plist>`)
	t.Chdir(s.root)

	opts, err := s.svc.Options(t.Context())
	if err != nil || opts.Config != "/work.toml" {
		t.Fatalf("Options() = %+v, %v; want the config at /work.toml, as launchd runs the router in /", opts, err)
	}
	svc, err := s.svc.For(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"SWITCHBOARD_CONFIG": "/config/other.toml", "CLAUDE_CONFIG_DIR": "/claude"} {
		if got := svc.Getenv(key); got != want {
			t.Errorf("Getenv(%s) = %q, want %q, as launchd runs the router in /", key, got, want)
		}
	}
}

func TestSameFileJudgesAPathNotThereByWhereItWouldBe(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		a, b string
		want bool
	}{
		{a: "real/claude", b: "link/claude", want: true},
		{a: "real/claude", b: "link/other", want: false},
		{a: "real/claude/skills", b: "link/claude/skills", want: true},
		{a: "real/claude", b: "elsewhere/claude", want: false},
	}
	for _, tt := range tests {
		if got := service.SameFile(filepath.Join(root, tt.a), filepath.Join(root, tt.b)); got != tt.want {
			t.Errorf("SameFile(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
