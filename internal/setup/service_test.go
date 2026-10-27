package setup_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/leeovery/switchboard/internal/service"
)

// restarted are the launchctl subcommands a run of setup runs that restarts
// the router: it asks whether the service is loaded, then restarts it.
var restarted = []string{"print", "print", "kickstart"}

func TestTheServiceStep(t *testing.T) {
	const installed = "Installed <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist: " +
		"launchd starts the router now, at every login, and whenever it stops.\n"
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay     func(t *testing.T, w *world)
		answers []string
		want    string
		wantRan []string
	}{
		{
			name: "not installed: installed",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.plist()); err != nil {
					t.Fatal(err)
				}
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			answers: []string{""},
			want:    installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "bootstrap"},
		},
		{
			name:    "installed, but launchd hasn't loaded it: installed again",
			lay:     func(_ *testing.T, w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			answers: []string{""},
			want:    installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "bootstrap"},
		},
		{
			name:    "installed and up, nothing changed: left as it is",
			answers: []string{""},
			want:    "The service is installed, and the router is up: healthy, pid 4242.\n",
			wantRan: untouched,
		},
		{
			name:    "installed and up, a token changed: left to the router, which takes it up",
			lay:     func(t *testing.T, w *world) { w.removeToken(t, "side") },
			answers: []string{"test-token-side", ""},
			want:    "The service is installed, and the router is up: healthy, pid 4242. It takes up setup's changes on its own.\n",
			wantRan: untouched,
		},
		{
			name: "installed and up, the config changed: left to the router, which takes it up",
			lay: func(t *testing.T, w *world) {
				w.writeConfig(t, strings.TrimSuffix(doneConfig, "\n[prime]\nday = \"08:00-23:00\"\n"))
			},
			answers: []string{"", "07:00-22:00"},
			want:    "The service is installed, and the router is up: healthy, pid 4242. It takes up setup's changes on its own.\n",
			wantRan: untouched,
		},
		{
			name:    "installed from another switchboard: installed afresh, to run this one",
			lay:     func(t *testing.T, w *world) { w.installService(t, w.goInstalled(t)) },
			answers: []string{""},
			want: "The service runs <root>/go/bin/switchboard, not this switchboard, <root>/brew/bin/switchboard, so it's installed afresh.\n" +
				installed + "The router is up: healthy, pid 4244.\n",
			wantRan: []string{"print", "print", "bootout", "print", "bootstrap"},
		},
		{
			name:    "installed with a plist naming no program: installed afresh, as setup installs it",
			lay:     func(t *testing.T, w *world) { writeFile(t, w.plist(), "<plist/>\n", 0o644) },
			answers: []string{""},
			want: "The service's plist can't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program), so it's installed afresh.\n" +
				installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "bootout", "print", "bootstrap"},
		},
		{
			name:    "installed from before ProcessType: installed afresh",
			lay:     func(t *testing.T, w *world) { w.editPlist(t, withoutProcessType) },
			answers: []string{""},
			want: "The service's plist differs from the one this switchboard writes, with no ProcessType, so it's installed afresh.\n" +
				installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "bootout", "print", "bootstrap"},
		},
		{
			name: "installed, its plist's keys in another order and with other whitespace: left as it is",
			lay: func(t *testing.T, w *world) {
				w.editPlist(t, func(plist string) string {
					plist = withoutProcessType(plist)
					plist = strings.Replace(plist, "<dict>\n", "<dict>\n"+processType, 1)
					return strings.NewReplacer("\t", "  ", "</key>\n", "</key> ").Replace(plist)
				})
			},
			answers: []string{""},
			want:    "The service is installed, and the router is up: healthy, pid 4242.\n",
			wantRan: untouched,
		},
		{
			name:    "installed with a plist that isn't XML, as a binary one isn't: installed afresh",
			lay:     func(t *testing.T, w *world) { writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644) },
			answers: []string{""},
			want: "The service's plist can't be read back (parse the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist: XML syntax error on line 1: invalid UTF-8), so it's installed afresh.\n" +
				installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "bootout", "print", "bootstrap"},
		},
		{
			name:    "installed, its router not answering: restarted",
			lay:     func(_ *testing.T, w *world) { w.launchd.pid = 0 },
			answers: []string{""},
			want:    "Restarted the router, as it didn't answer.\nThe router is up: healthy, pid 4243.\n",
			wantRan: restarted,
		},
		{
			name:    "installed, its router answering only once setup restarts it: restarted in place as it finishes its requests",
			lay:     func(_ *testing.T, w *world) { w.launchd.unanswered = 1 },
			answers: []string{""},
			want: "The router is finishing its requests in flight, then it restarts in place.\n" +
				"Restarted the router, as it didn't answer.\nThe router is up: healthy, pid 4242.\n",
			wantRan: []string{"print", "print"},
		},
		{
			name: "installed, a router from before routers restarted when asked answering only once setup restarts it: restarted by launchd",
			lay: func(_ *testing.T, w *world) {
				w.launchd.unanswered, w.launchd.old = 1, true
			},
			answers: []string{""},
			want: "The router is finishing its requests in flight, then launchd starts it again.\n" +
				"Restarted the router, as it didn't answer.\nThe router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "kill"},
		},
		{
			name: "not installed, no account with a usable token: installed, with a warning",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.plist()); err != nil {
					t.Fatal(err)
				}
				w.launchd.loaded, w.launchd.pid = false, 0
				w.removeToken(t, "work")
				w.removeToken(t, "side")
			},
			answers: []string{"", "", ""},
			want: installed + "Warning: no account has a usable token, so the router will have nothing to route to: switchboard accounts says why.\n" +
				"The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "bootstrap"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			if tt.lay != nil {
				tt.lay(t, w)
			}

			shown := w.runs(t, tt.answers...)
			if got := section(t, shown, "3. The service"); got != tt.want {
				t.Errorf("the service step showed\n%s\nwant\n%s", got, tt.want)
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran launchctl %q, want %q", ran, tt.wantRan)
			}
		})
	}
}

func TestTheServiceStepInstallsTheServiceToRunThisSwitchboard(t *testing.T) {
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay func(t *testing.T, w *world)
	}{
		{
			name: "not installed",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.plist()); err != nil {
					t.Fatal(err)
				}
				w.launchd.loaded, w.launchd.pid = false, 0
			},
		},
		{
			name: "installed from another switchboard",
			lay:  func(t *testing.T, w *world) { w.installService(t, w.goInstalled(t)) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.lay(t, w)

			w.runs(t, "")
			plist, err := os.ReadFile(w.plist())
			if err != nil || !strings.Contains(string(plist), "<array>\n\t\t<string>"+w.switchboard+"</string>\n\t\t<string>serve</string>\n\t</array>") {
				t.Errorf("the plist reads\n%s(%v)\nwant it to run switchboard serve by the path switchboard was run by, %s", plist, err, w.switchboard)
			}
		})
	}
}

// processType is the ProcessType key, with its value, as install writes it.
const processType = "\t<key>ProcessType</key>\n\t<string>Interactive</string>\n"

// withoutProcessType is a plist as switchboards from before ProcessType
// wrote it.
func withoutProcessType(plist string) string {
	return strings.Replace(plist, processType, "", 1)
}

func TestTheServiceStepKeepsTheOptionsTheServiceWasInstalledWith(t *testing.T) {
	tests := []struct {
		name string
		// install installs the service in the world, which is done but for
		// the service.
		install func(t *testing.T, w *world)
		// configFlag is the config file setup is given, "" for none.
		configFlag func(w *world) string
		// want are what the plist setup leaves holds, the world's root
		// written <root>.
		want []string
	}{
		{
			name: "a log level, installed from before ProcessType",
			install: func(t *testing.T, w *world) {
				w.installServiceWith(t, service.InstallOptions{Executable: w.switchboard, LogLevel: "debug"})
				w.editPlist(t, withoutProcessType)
			},
			want: []string{"<string>serve</string>\n\t\t<string>--log-level</string>\n\t\t<string>debug</string>\n\t</array>"},
		},
		{
			name: "a config file, installed from before ProcessType",
			install: func(t *testing.T, w *world) {
				w.installServiceWith(t, service.InstallOptions{Executable: w.switchboard, Config: w.config})
				w.editPlist(t, withoutProcessType)
			},
			want: []string{"<string>serve</string>\n\t\t<string>--config</string>\n\t\t<string><root>/config/switchboard/config.toml</string>\n\t</array>"},
		},
		{
			name: "a log level, installed from another switchboard",
			install: func(t *testing.T, w *world) {
				w.installServiceWith(t, service.InstallOptions{Executable: w.goInstalled(t), LogLevel: "warn"})
			},
			want: []string{"<string><root>/brew/bin/switchboard</string>\n\t\t<string>serve</string>\n\t\t<string>--log-level</string>\n\t\t<string>warn</string>\n\t</array>"},
		},
		{
			name: "a log level, but another config file than the one setup is given",
			install: func(t *testing.T, w *world) {
				w.installServiceWith(t, service.InstallOptions{Executable: w.switchboard, Config: filepath.Join(w.root, "elsewhere.toml"), LogLevel: "debug"})
			},
			configFlag: func(w *world) string { return w.config },
			want:       []string{"<string>--config</string>\n\t\t<string><root>/config/switchboard/config.toml</string>\n\t\t<string>--log-level</string>\n\t\t<string>debug</string>\n\t</array>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.install(t, w)
			if tt.configFlag != nil {
				w.configFlag = tt.configFlag(w)
			}

			shown := w.runs(t, "")
			if !strings.Contains(section(t, shown, "3. The service"), "so it's installed afresh.") {
				t.Errorf("the terminal showed\n%s\nwant the service installed afresh", shown)
			}
			plist, err := os.ReadFile(w.plist())
			got := strings.ReplaceAll(string(plist), w.root, "<root>")
			for _, want := range append(tt.want, processType) {
				if err != nil || !strings.Contains(got, want) {
					t.Errorf("the plist reads\n%s(%v)\nwant it to hold\n%s", got, err, want)
				}
			}
		})
	}
}

// installedAt is how the service step says it installed the service.
const installedAt = "Installed <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist: " +
	"launchd starts the router now, at every login, and whenever it stops.\n"

// installedFrom installs the service in the world, which is done, as opts
// say, from a shell that sets env over the world's own, and returns its
// plist as install wrote it.
func (w *world) installedFrom(t *testing.T, env map[string]string, opts service.InstallOptions) string {
	t.Helper()
	w.env = env
	w.installServiceWith(t, opts)
	w.env = nil
	plist, err := os.ReadFile(w.plist())
	if err != nil {
		t.Fatal(err)
	}
	return string(plist)
}

// tokenless is the warning installing the service gives for a router with
// no account's token to serve with.
const tokenless = "Warning: no account has a usable token, so the router will have nothing to route to: switchboard accounts says why.\n"

func TestTheServiceStepKeepsTheServicesEnvironment(t *testing.T) {
	// The router's state directory, which holds no tokens, is the one the
	// router it starts is checked for them in.
	const apart = "The router uses another state directory and Claude Code config directory than setup: " +
		"switchboard service install, run from the shell the router should use, lines them up.\n"
	tests := []struct {
		name string
		// lay changes the world, the service installed from serviceEnv,
		// before setup runs, given what was installed.
		lay func(t *testing.T, w *world)
		// executable is the switchboard the service was installed to run,
		// "" for this one.
		executable func(t *testing.T, w *world) string
		wantShown  string
		wantRan    []string
	}{
		{
			name:      "up, as it was installed: left as it is, its router never restarted",
			wantShown: apart + "The service is installed, and the router is up: healthy, pid 4243.\n",
			wantRan:   untouched,
		},
		{
			name:      "from before ProcessType: installed afresh, keeping it",
			lay:       func(t *testing.T, w *world) { w.editPlist(t, withoutProcessType) },
			wantShown: apart + "The service's plist differs from the one this switchboard writes, with no ProcessType, so it's installed afresh.\n" + installedAt + tokenless + "The router is up: healthy, pid 4244.\n",
			wantRan:   []string{"print", "print", "bootout", "print", "bootstrap"},
		},
		{
			name:       "from another switchboard: installed afresh, keeping it",
			executable: func(t *testing.T, w *world) string { return w.goInstalled(t) },
			wantShown:  apart + "The service runs <root>/go/bin/switchboard, not this switchboard, <root>/brew/bin/switchboard, so it's installed afresh.\n" + installedAt + tokenless + "The router is up: healthy, pid 4244.\n",
			wantRan:    []string{"print", "print", "bootout", "print", "bootstrap"},
		},
		{
			name:      "not loaded: installed again, keeping it",
			lay:       func(_ *testing.T, w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			wantShown: apart + installedAt + tokenless + "The router is up: healthy, pid 4244.\n",
			wantRan:   []string{"print", "print", "bootstrap"},
		},
		{
			// As when launchctl setenv gave launchd another XDG_STATE_HOME.
			name: "its router running at the shell's state directory, not the plist's: left as it is, never restarted",
			lay: func(_ *testing.T, w *world) {
				w.launchd.stateDir = filepath.Join(w.home, ".local", "state", "switchboard")
			},
			wantShown: apart + "The service is installed, and the router is up: healthy, pid 4243.\n",
			wantRan:   untouched,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			executable := w.switchboard
			if tt.executable != nil {
				executable = tt.executable(t, w)
			}
			serviceEnv := map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom"), "CLAUDE_CONFIG_DIR": filepath.Join(w.root, "claude-service")}
			written := w.installedFrom(t, serviceEnv, service.InstallOptions{Executable: executable, LogLevel: "debug"})
			if tt.lay != nil {
				tt.lay(t, w)
			}
			// The shell sets no XDG_STATE_HOME.
			w.env = map[string]string{"XDG_STATE_HOME": ""}

			shown := section(t, w.runs(t, ""), "3. The service")
			if shown != tt.wantShown {
				t.Errorf("the service step showed\n%s\nwant\n%s", shown, tt.wantShown)
			}
			for _, value := range []string{"<root>/custom", "<root>/claude-service", "<root>/home/.local/state"} {
				if strings.Contains(shown, value) {
					t.Errorf("the service step showed\n%s\nwant the places named, never where they are, such as %s", shown, value)
				}
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran launchctl %q, want %q", ran, tt.wantRan)
			}
			want := strings.Replace(written, executable, w.switchboard, 1)
			if got, err := os.ReadFile(w.plist()); err != nil || string(got) != want {
				t.Errorf("the plist reads\n%s(%v)\nwant the service's options and environment kept\n%s", got, err, want)
			}
		})
	}
}

func TestTheServiceStepSaysNothingOfWhatFindsTheSame(t *testing.T) {
	tests := []struct {
		name string
		// serviceEnv is what the service was installed from, and shellEnv
		// what setup runs in, over the world's own; every <root> the
		// world's.
		serviceEnv, shellEnv map[string]string
		installed            service.InstallOptions
	}{
		{
			name:       "SWITCHBOARD_CONFIG set otherwise, naming the same file",
			serviceEnv: map[string]string{"SWITCHBOARD_CONFIG": "<root>/config/switchboard/config.toml"},
			shellEnv:   map[string]string{"SWITCHBOARD_CONFIG": "<root>/config/switchboard/../switchboard/config.toml"},
		},
		{
			name:       "the config given, XDG_CONFIG_HOME set otherwise",
			serviceEnv: map[string]string{"XDG_CONFIG_HOME": "<root>/elsewhere"},
			installed:  service.InstallOptions{Config: "<root>/config/switchboard/config.toml"},
		},
		{
			name:       "SWITCHBOARD_LOG_LEVEL set to info, and unset",
			serviceEnv: map[string]string{"SWITCHBOARD_LOG_LEVEL": "info"},
		},
		{
			name:       "SWITCHBOARD_LOG_LEVEL set to DEBUG, and to debug",
			serviceEnv: map[string]string{"SWITCHBOARD_LOG_LEVEL": "DEBUG"},
			shellEnv:   map[string]string{"SWITCHBOARD_LOG_LEVEL": "debug"},
		},
		{
			name:       "CLAUDE_CONFIG_DIR set to the one its absence finds",
			serviceEnv: map[string]string{"CLAUDE_CONFIG_DIR": "<root>/home/.claude"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.installed.Executable = w.switchboard
			tt.installed.Config = strings.ReplaceAll(tt.installed.Config, "<root>", w.root)
			w.installedFrom(t, w.rooted(tt.serviceEnv), tt.installed)
			w.env = w.rooted(tt.shellEnv)
			w.writeConfig(t, strings.TrimSuffix(doneConfig, "\n[prime]\nday = \"08:00-23:00\"\n"))

			shown := section(t, w.runs(t, "", "07:00-22:00"), "3. The service")
			if want := "The service is installed, and the router is up: healthy, pid 4243. It takes up setup's changes on its own.\n"; shown != want {
				t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
			}
		})
	}
}

func TestTheServiceStepTakesTheConfigGivenOnlyWhereTheRouterServesAnother(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	// The service serves setup's config file, through its environment.
	w.installedFrom(t, map[string]string{"XDG_CONFIG_HOME": "", "SWITCHBOARD_CONFIG": w.config}, service.InstallOptions{Executable: w.switchboard})
	w.env = map[string]string{"XDG_CONFIG_HOME": "", "SWITCHBOARD_CONFIG": w.config}
	w.configFlag = w.config

	shown := section(t, w.runs(t, ""), "3. The service")
	if want := "The service is installed, and the router is up: healthy, pid 4243.\n"; shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q: the router serving the config given left alone", ran, untouched)
	}
}

func TestTheServiceStepInstallsAPlistItCantReadBackAsSetupWould(t *testing.T) {
	for _, plist := range []string{"<plist/>\n", "<plist><dict/></plist>\n", "<plist><dict><key>Label</key><string>io.github.leeovery.switchboard</string></dict></plist>\n", "bplist00\xd1\x01\x02"} {
		t.Run(plist, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			writeFile(t, w.plist(), plist, 0o644)
			w.configFlag = w.config

			shown := section(t, w.runs(t, ""), "3. The service")
			if !strings.HasPrefix(shown, "The service's plist can't be read back (") || !strings.Contains(shown, "so it's installed afresh.\n"+installedAt) {
				t.Errorf("the service step showed\n%s\nwant the plist said unreadable, and the service installed afresh", shown)
			}
			got, err := os.ReadFile(w.plist())
			for _, want := range []string{
				"<array>\n\t\t<string>" + w.switchboard + "</string>\n\t\t<string>serve</string>\n\t\t<string>--config</string>\n\t\t<string>" + w.config + "</string>\n\t</array>",
				// The shell's, which the router's state is found by.
				"<key>XDG_STATE_HOME</key>\n\t\t<string>" + filepath.Join(w.root, "state") + "</string>",
			} {
				if err != nil || !strings.Contains(string(got), want) {
					t.Errorf("the plist reads\n%s(%v)\nwant it as setup's options and its shell's environment have it, holding\n%s", got, err, want)
				}
			}
		})
	}
}

func TestTheServiceStepLeavesAServiceWhoseRouterCouldntStartInstalledAfresh(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	serviceEnv := map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom")}
	w.installedFrom(t, serviceEnv, service.InstallOptions{Executable: w.switchboard, LogLevel: "debug"})
	w.editPlist(t, withoutProcessType)
	written, err := os.ReadFile(w.plist())
	if err != nil {
		t.Fatal(err)
	}
	w.unstartable = errors.New("no config file at <the config>\n\nCreate one like this:\n\n[[account]]")

	shown := section(t, w.runs(t, ""), "3. The service")
	want := "The router uses another state directory than setup: switchboard service install, run from the shell the router should use, lines them up.\n" +
		"The service's plist differs from the one this switchboard writes, with no ProcessType, " +
		"but the router couldn't start installed afresh, so the service is left as it is, its router running: " +
		"switchboard service install, run from the shell the router should use, installs it afresh once what stops it is put right. It couldn't start as:\n" +
		"no config file at <the config>\n\nCreate one like this:\n\n[[account]]\n"
	if shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q: the running router left alone", ran, untouched)
	}
	if got, err := os.ReadFile(w.plist()); err != nil || string(got) != string(written) {
		t.Errorf("the plist reads\n%s(%v)\nwant it left as it was\n%s", got, err, written)
	}
	if checked := []string{filepath.Join(w.root, "custom") + " " + w.config + " debug"}; !slices.Equal(w.checked, checked) {
		t.Errorf("checked the router could start from %q, want %q: the service's state directory, config file and log level", w.checked, checked)
	}
}

func TestTheServiceStepFailsWhereTheRouterCouldntStartAndNoneRuns(t *testing.T) {
	tests := []struct {
		name string
		// lay changes the world, which is done, before setup runs.
		lay  func(t *testing.T, w *world)
		want string
	}{
		{
			name: "not installed",
			lay: func(t *testing.T, w *world) {
				if err := os.Remove(w.plist()); err != nil {
					t.Fatal(err)
				}
				w.launchd.loaded, w.launchd.pid = false, 0
			},
		},
		{
			name: "not loaded",
			lay:  func(_ *testing.T, w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			want: "Launchd hasn't loaded the service, but the router couldn't start installed afresh, and none is running.\n",
		},
		{
			name: "drifted, its router down",
			lay: func(t *testing.T, w *world) {
				w.editPlist(t, withoutProcessType)
				w.launchd.pid = 0
			},
			want: "The service's plist differs from the one this switchboard writes, with no ProcessType, but the router couldn't start installed afresh, and none is running.\n",
		},
		{
			name: "a plist it can't read, its router down",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "<plist/>\n", 0o644)
				w.launchd.pid = 0
			},
			want: "The service's plist can't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program), but the router couldn't start installed afresh, and none is running.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.lay(t, w)
			w.unstartable = errors.New("the config names no account")

			shown, err := w.run(t, "")
			if want := "the router couldn't start: the config names no account"; err == nil || err.Error() != want {
				t.Errorf("Run() error = %v, want %q", err, want)
			}
			if got := section(t, shown, "3. The service"); got != tt.want {
				t.Errorf("the service step showed\n%s\nwant\n%s", got, tt.want)
			}
			if slices.Contains(w.launchd.ran(), "bootstrap") {
				t.Errorf("ran launchctl %q, want nothing loaded", w.launchd.ran())
			}
		})
	}
}

func TestTheServiceStepSaysWhenTheRouterServesAnotherConfigFile(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	// The service's state elsewhere, without the tokens.
	w.installedFrom(t, map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom")}, service.InstallOptions{Executable: w.switchboard, Config: filepath.Join(w.root, "other.toml")})
	w.editPlist(t, withoutProcessType)
	w.writeConfig(t, strings.TrimSuffix(doneConfig, "\n[prime]\nday = \"08:00-23:00\"\n"))

	shown := section(t, w.runs(t, "", "07:00-22:00"), "3. The service")
	// No warning of tokens for setup's accounts, which aren't the router's.
	want := "The router uses another state directory and config file than setup: switchboard service install, run from the shell the router should use, lines them up. " +
		"For the config file, switchboard setup --config, naming the router's, does too.\n" +
		"The service's plist differs from the one this switchboard writes, with no ProcessType, so it's installed afresh.\n" +
		installedAt + "The router is up: healthy, pid 4244.\n"
	if shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}
}

func TestTheServiceStepNamesTheLogThePlistHasLaunchdWrite(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	log := filepath.Join(w.root, "logs", "router.out")
	w.editPlist(t, func(plist string) string {
		return strings.ReplaceAll(withoutProcessType(plist), filepath.Join(w.state, "logs", "launchd.log"), log)
	})
	w.launchd.dead = true
	synctest.Test(t, func(t *testing.T) {
		_, err := w.run(t, "")
		if err == nil || !strings.HasSuffix(err.Error(), "and in "+log) {
			t.Errorf("Run() error = %v, want it naming %s, where the plist has launchd write", err, log)
		}
	})
}

// rooted is env with every value's <root> the world's root.
func (w *world) rooted(env map[string]string) map[string]string {
	rooted := map[string]string{}
	for key, value := range env {
		rooted[key] = strings.ReplaceAll(value, "<root>", w.root)
	}
	return rooted
}

func TestSetupStopsWhenTheRouterDoesntAnswer(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	if err := os.Remove(w.plist()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(w.skill); err != nil {
		t.Fatal(err)
	}
	w.launchd.loaded, w.launchd.pid, w.launchd.dead = false, 0, true
	synctest.Test(t, func(t *testing.T) {
		shown, err := w.run(t, "")
		want := "the router didn't answer within 5s of starting: see why with switchboard logs router, and in " + w.state + "/logs/launchd.log"
		if err == nil || err.Error() != want {
			t.Errorf("Run() error = %v, want %q", err, want)
		}
		if strings.Contains(shown, "4. The claude link") {
			t.Errorf("the terminal showed\n%s\nwant setup stopped at the service", shown)
		}
	})
	if _, err := os.Stat(w.skill); err == nil {
		t.Error("the skill is installed, want setup stopped before it")
	}
}
