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

// unreadBinary is why a binary plist plutil can't read can't be read back.
const unreadBinary = "read the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, through plutil: plutil: not a binary plist the world knows"

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
			name: "installed with a plist naming no program, not loaded: installed afresh, as setup installs it",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "<plist/>\n", 0o644)
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			answers: []string{""},
			want: "The service's plist can't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program), so it's installed afresh.\n" +
				installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "bootstrap"},
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
			name:    "installed with a plist naming no program, loaded: left as it is",
			lay:     func(t *testing.T, w *world) { writeFile(t, w.plist(), "<plist/>\n", 0o644) },
			answers: []string{""},
			want: "The service's plist can't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program), " +
				"but launchd has it loaded, so the service is left as it is: switchboard service install, run from the shell the router should use, installs it afresh.\n",
			wantRan: untouched,
		},
		{
			name: "installed with a plist naming no program, not loaded, a router run otherwise answering: left as it is",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "<plist/>\n", 0o644)
				w.launchd.loaded = false
			},
			answers: []string{""},
			want: "The service's plist can't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program), " +
				"but a router answers, so the service is left as it is: switchboard service install, run from the shell the router should use, installs it afresh.\n",
			wantRan: untouched,
		},
		{
			name:    "installed with a binary plist plutil can't read, loaded: left as it is",
			lay:     func(t *testing.T, w *world) { writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644) },
			answers: []string{""},
			want: "The service's plist can't be read back (" + unreadBinary + "), " +
				"but launchd has it loaded, so the service is left as it is: switchboard service install, run from the shell the router should use, installs it afresh.\n",
			wantRan: untouched,
		},
		{
			name: "installed with a binary plist plutil can't read, not loaded: installed afresh",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644)
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			answers: []string{""},
			want: "The service's plist can't be read back (" + unreadBinary + "), so it's installed afresh.\n" +
				installed + "The router is up: healthy, pid 4243.\n",
			wantRan: []string{"print", "print", "bootstrap"},
		},
		{
			name:    "installed, its router not answering: restarted",
			lay:     func(_ *testing.T, w *world) { w.launchd.pid = 0 },
			answers: []string{""},
			want:    "Restarted the router, as it didn't answer.\nThe router is up: healthy, pid 4243.\n",
			wantRan: restarted,
		},
		{
			name:    "installed, launchd running its router, which answers nowhere setup looks: left as it is",
			lay:     func(_ *testing.T, w *world) { w.launchd.unanswered = 1 },
			answers: []string{""},
			want: "Launchd runs the service's router, pid 4242, but it answers neither where the service's plist keeps its state nor where this shell keeps it, " +
				"so setup leaves it as it is: switchboard service status says how it stands.\n",
			wantRan: untouched,
		},
		{
			name:    "not loaded, a router run otherwise answering: left as it is",
			lay:     func(_ *testing.T, w *world) { w.launchd.loaded = false },
			answers: []string{""},
			want: "Launchd hasn't loaded the service, but a router run some other way answers, and another would clash with it, so setup leaves it as it is: " +
				"once that one's stopped, switchboard service install, run from the shell the router should use, installs it.\n",
			wantRan: untouched,
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
			wantShown: installedAt + tokenless + apart + "The router is up: healthy, pid 4244.\n",
			wantRan:   []string{"print", "print", "bootstrap"},
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
			name:       "CLAUDE_CONFIG_DIR naming setup's by another path",
			serviceEnv: map[string]string{"CLAUDE_CONFIG_DIR": "<root>/config/../claude"},
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
			w.launchd.loaded, w.launchd.pid = false, 0
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
	if checked := []string{filepath.Join(w.root, "custom", "switchboard") + " " + w.config + " debug"}; !slices.Equal(w.checked, checked) {
		t.Errorf("checked the router could start from %q, want %q: the service's state directory, config file and log level", w.checked, checked)
	}
}

func TestTheServiceStepRedactsWhyThePlistCantBeRead(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644)
	w.plutilSays = "plutil: not a plist, but " + tokenShaped

	shown := section(t, w.runs(t, ""), "3. The service")
	if strings.Contains(shown, tokenShaped) || !strings.Contains(shown, "The service's plist can't be read back (") {
		t.Errorf("the service step showed\n%s\nwant why the plist can't be read back, redacted", shown)
	}
}

func TestTheServiceStepRedactsWhyTheRouterCouldntStart(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.editPlist(t, withoutProcessType)
	w.unstartable = errors.New("the config holds " + tokenShaped)

	shown := section(t, w.runs(t, ""), "3. The service")
	if strings.Contains(shown, tokenShaped) || !strings.Contains(shown, "It couldn't start as:\nthe config holds ") {
		t.Errorf("the service step showed\n%s\nwant why the router couldn't start, redacted", shown)
	}
}

// installAdvice is how the service step says to install the service once
// the router could start.
const installAdvice = "switchboard service install, run from the shell the router should use, installs it once what stops it is put right.\n"

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
			want: "Launchd hasn't loaded the service, but the router couldn't start installed afresh, and none is running: " + installAdvice,
		},
		{
			name: "drifted, its router down",
			lay: func(t *testing.T, w *world) {
				w.editPlist(t, withoutProcessType)
				w.launchd.pid = 0
			},
			want: "The service's plist differs from the one this switchboard writes, with no ProcessType, but the router couldn't start installed afresh, and none is running: " + installAdvice,
		},
		{
			name: "loaded, its router down",
			lay:  func(_ *testing.T, w *world) { w.launchd.pid = 0 },
			want: "Launchd has the service loaded, but no router runs, and one couldn't start as the service runs it: " + installAdvice,
		},
		{
			name: "a plist it can't read, not loaded",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "<plist/>\n", 0o644)
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			want: "The service's plist can't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program), but the router couldn't start installed afresh, and none is running: " + installAdvice,
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
			if ran := w.launchd.ran(); slices.Contains(ran, "bootstrap") || slices.Contains(ran, "kickstart") {
				t.Errorf("ran launchctl %q, want nothing loaded or started", ran)
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

func TestTheServiceStepPutsLaunchdsLogBackInTheStateDirectory(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	log := filepath.Join(w.state, "logs", "launchd.log")
	w.editPlist(t, func(plist string) string {
		return strings.ReplaceAll(plist, log, filepath.Join(w.root, "elsewhere", "router.out"))
	})

	shown := section(t, w.runs(t, ""), "3. The service")
	want := "The service's plist differs from the one this switchboard writes, with another StandardOutPath and another StandardErrorPath, so it's installed afresh.\n" +
		installedAt + "The router is up: healthy, pid 4243.\n"
	if shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}
	if plist, err := os.ReadFile(w.plist()); err != nil || strings.Count(string(plist), "<string>"+log+"</string>") != 2 {
		t.Errorf("the plist reads\n%s(%v)\nwant launchd's log back at %s", plist, err, log)
	}
}

func TestTheServiceStepLeavesAServiceWhoseRouterCantBeTold(t *testing.T) {
	const left = "No router answers where the service's plist keeps its state, but one answers where this shell keeps it, " +
		"so which the service runs can't be told, and setup leaves it as it is: switchboard service install, run from the shell the router should use, lines them up.\n"
	tests := []struct {
		name string
		// lay changes the world, which is done, its router answering at the
		// shell's state directory, before setup runs.
		lay func(t *testing.T, w *world)
	}{
		{
			name: "the plist sets a state directory of its own",
			lay: func(t *testing.T, w *world) {
				w.installedFrom(t, map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom")}, service.InstallOptions{Executable: w.switchboard})
				w.launchd.stateDir = filepath.Join(w.root, "state", "switchboard")
			},
		},
		{
			name: "the plist sets none, launchctl setenv giving launchd the shell's",
			lay: func(t *testing.T, w *world) {
				w.launchd.setenv = map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "state")}
				w.editPlist(t, func(plist string) string {
					return strings.Replace(plist, "\t\t<key>XDG_STATE_HOME</key>\n\t\t<string>"+filepath.Join(w.root, "state")+"</string>\n", "", 1)
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.lay(t, w)
			// Differing from the plist this switchboard writes, too.
			w.editPlist(t, withoutProcessType)
			before, err := os.ReadFile(w.plist())
			if err != nil {
				t.Fatal(err)
			}
			w.launchd.calls = nil

			shown := section(t, w.runs(t, ""), "3. The service")
			if shown != left {
				t.Errorf("the service step showed\n%s\nwant\n%s", shown, left)
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
				t.Errorf("ran launchctl %q, want %q: the service left as it is", ran, untouched)
			}
			if after, err := os.ReadFile(w.plist()); err != nil || string(after) != string(before) {
				t.Errorf("the plist reads\n%s(%v)\nwant it as it was\n%s", after, err, before)
			}
			if len(w.checked) != 0 {
				t.Errorf("checked the router could start from %q, want it left unchecked", w.checked)
			}
		})
	}
}

func TestTheServiceStepLeavesARouterLaunchdRunsOutOfReach(t *testing.T) {
	const left = "Launchd runs the service's router, pid 4242, but it answers neither where the service's plist keeps its state nor where this shell keeps it, " +
		"so setup leaves it as it is: switchboard service status says how it stands.\n"
	for name, drifted := range map[string]bool{"as written": false, "its plist differing": true} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			// launchctl setenv gave launchd a third state home, where the
			// router runs.
			w.launchd.setenv = map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "third")}
			w.editPlist(t, func(plist string) string {
				return strings.Replace(plist, "\t\t<key>XDG_STATE_HOME</key>\n\t\t<string>"+filepath.Join(w.root, "state")+"</string>\n", "", 1)
			})
			w.launchd.stateDir = filepath.Join(w.root, "third", "switchboard")
			if drifted {
				w.editPlist(t, withoutProcessType)
			}
			before, err := os.ReadFile(w.plist())
			if err != nil {
				t.Fatal(err)
			}
			w.launchd.calls = nil
			w.unstartable = errors.New("the config names no account")

			shown, err := w.run(t, "")
			if err != nil {
				t.Errorf("Run() error = %v, want none", err)
			}
			if got := section(t, shown, "3. The service"); got != left {
				t.Errorf("the service step showed\n%s\nwant\n%s", got, left)
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
				t.Errorf("ran launchctl %q, want %q: the router launchd runs left alone", ran, untouched)
			}
			if after, err := os.ReadFile(w.plist()); err != nil || string(after) != string(before) {
				t.Errorf("the plist reads\n%s(%v)\nwant it as it was", after, err)
			}
			if len(w.checked) != 0 {
				t.Errorf("checked the router could start from %q, want nothing checked", w.checked)
			}
		})
	}
}

func TestTheServiceStepTakesTheRouterLaunchdStartsWhereverItAnswers(t *testing.T) {
	tests := []struct {
		name string
		// drifted is whether the plist differs from the one this switchboard
		// writes, so setup installs it again, rather than restarting it.
		drifted bool
		wantRan []string
	}{
		{name: "restarted", wantRan: []string{"print", "print", "kickstart"}},
		{name: "installed afresh", drifted: true, wantRan: []string{"print", "print", "bootout", "print", "bootstrap"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			// launchctl setenv gives launchd the shell's state home, which the
			// plist doesn't set; no router runs.
			w.launchd.setenv = map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "state")}
			w.editPlist(t, func(plist string) string {
				return strings.Replace(plist, "\t\t<key>XDG_STATE_HOME</key>\n\t\t<string>"+filepath.Join(w.root, "state")+"</string>\n", "", 1)
			})
			if tt.drifted {
				w.editPlist(t, withoutProcessType)
			} else {
				// launchd's log where the plist's state directory has it.
				w.editPlist(t, func(plist string) string {
					return strings.ReplaceAll(plist, service.LogIn(filepath.Join(w.root, "state", "switchboard")), service.LogIn(filepath.Join(w.home, ".local", "state", "switchboard")))
				})
			}
			w.launchd.pid = 0
			w.launchd.calls = nil

			shown, err := w.run(t, "")
			if err != nil {
				t.Errorf("Run() error = %v, want the router launchd started, at the shell's state directory, taken", err)
			}
			// Its places judged where it answered: the shell's state
			// directory, which holds setup's tokens.
			const elsewhere = "Launchd started the router where this shell keeps its state, not where the service's plist does: " +
				"switchboard service install, run from the shell the router should use, lines them up.\n"
			got := section(t, shown, "3. The service")
			if !strings.HasSuffix(got, elsewhere+"The router is up: healthy, pid 4243.\n") {
				t.Errorf("the service step showed\n%s\nwant it to end\n%sThe router is up: healthy, pid 4243.", got, elsewhere)
			}
			for _, never := range []string{"uses another", "Warning"} {
				if strings.Contains(got, never) {
					t.Errorf("the service step showed\n%s\nwant nothing holding %q: the router's state directory and tokens are setup's", got, never)
				}
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran launchctl %q, want %q", ran, tt.wantRan)
			}
		})
	}
}

func TestTheServiceStepSettlesAServiceStartedAgain(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	custom := filepath.Join(w.root, "custom")
	w.installedFrom(t, map[string]string{"XDG_STATE_HOME": custom}, service.InstallOptions{Executable: w.switchboard})
	w.editPlist(t, withoutProcessType)
	w.launchd.pid = 0
	w.launchd.calls = nil

	first := section(t, w.runs(t, ""), "3. The service")
	// The router's state directory, apart from setup's, is said each run.
	const apart = "The router uses another state directory than setup: switchboard service install, run from the shell the router should use, lines them up.\n"
	if !strings.HasPrefix(first, "The service's plist differs from the one this switchboard writes, with no ProcessType, so it's installed afresh.\n") || !strings.Contains(first, apart) {
		t.Errorf("the service step showed\n%s\nwant the service installed afresh, its state directory said apart", first)
	}
	if checked := []string{filepath.Join(custom, "switchboard") + " " + filepath.Join(w.root, "config", "switchboard", "config.toml")}; !slices.Equal(w.checked, checked) {
		t.Errorf("checked the router could start from %q, want %q: the plist's state directory", w.checked, checked)
	}
	installed, err := os.ReadFile(w.plist())
	if err != nil || !strings.Contains(string(installed), "<key>XDG_STATE_HOME</key>\n\t\t<string>"+custom+"</string>") {
		t.Fatalf("the plist reads\n%s(%v)\nwant the state home it kept, %s", installed, err, custom)
	}

	// Run again, the router the plist runs answering: nothing flips back.
	w.launchd.calls = nil
	second := section(t, w.runs(t, ""), "3. The service")
	if !strings.HasPrefix(second, apart+"The service is installed, and the router is up: healthy, pid ") {
		t.Errorf("run again, the service step showed\n%s\nwant the service left as it is", second)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("run again, ran launchctl %q, want %q", ran, untouched)
	}
	if again, err := os.ReadFile(w.plist()); err != nil || string(again) != string(installed) {
		t.Errorf("run again, the plist reads\n%s(%v)\nwant it as the first run left it\n%s", again, err, installed)
	}
}

func TestTheServiceStepReadsABinaryPlistBack(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	serviceEnv := map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom")}
	written := w.installedFrom(t, serviceEnv, service.InstallOptions{Executable: w.switchboard, LogLevel: "debug"})
	// As defaults writes it, adding a variable.
	w.binary = strings.Replace(written, "\t</dict>\n\t<key>RunAtLoad</key>", "\t\t<key>HTTPS_PROXY</key>\n\t\t<string>http://proxy.example:8080</string>\n\t</dict>\n\t<key>RunAtLoad</key>", 1)
	writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644)

	shown := section(t, w.runs(t, ""), "3. The service")
	// Its keys as plutil gives them, launchd's log among them, as written.
	want := "The router uses another state directory than setup: switchboard service install, run from the shell the router should use, lines them up.\n" +
		"The service is installed, and the router is up: healthy, pid 4243.\n"
	if shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q: the service read back, and left as it is", ran, untouched)
	}
}

func TestTheServiceStepSeesTheRouterInTheHomeThePlistSets(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	home := filepath.Join(w.root, "other")
	// The state home unset, and a HOME added by hand, the token files there.
	w.installedFrom(t, map[string]string{"XDG_STATE_HOME": ""}, service.InstallOptions{Executable: w.switchboard})
	w.editPlist(t, func(plist string) string {
		return strings.Replace(withoutProcessType(plist), "\t\t<key>CLAUDE_CONFIG_DIR</key>", "\t\t<key>HOME</key>\n\t\t<string>"+home+"</string>\n\t\t<key>CLAUDE_CONFIG_DIR</key>", 1)
	})
	w.launchd.stateDir = filepath.Join(home, ".local", "state", "switchboard")

	shown := section(t, w.runs(t, ""), "3. The service")
	want := "The router uses another state directory than setup: switchboard service install, run from the shell the router should use, lines them up.\n" +
		"The service's plist differs from the one this switchboard writes, with no ProcessType, another StandardOutPath and another StandardErrorPath, so it's installed afresh.\n" +
		installedAt + tokenless + "The router is up: healthy, pid 4244.\n"
	if shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}
	if checked := []string{filepath.Join(home, ".local", "state", "switchboard") + " " + w.config}; !slices.Equal(w.checked, checked) {
		t.Errorf("checked the router could start from %q, want %q: the state directory the home decides", w.checked, checked)
	}
}

func TestTheServiceStepSaysSetupIsGivenAnotherConfigFile(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	other := filepath.Join(w.root, "other.toml")
	w.installedFrom(t, nil, service.InstallOptions{Executable: w.switchboard, Config: other})
	w.configFlag = w.config
	w.unstartable = errors.New("the config names no account")

	shown := section(t, w.runs(t, ""), "3. The service")
	// Refused, the router serves the other still.
	want := "The router uses another config file than setup: switchboard service install, run from the shell the router should use, lines them up. " +
		"For the config file, switchboard setup --config, naming the router's, does too.\n" +
		"Setup is given another config file than the one the router serves, but the router couldn't start installed afresh, so the service is left as it is, its router running: " +
		"switchboard service install, run from the shell the router should use, installs it afresh once what stops it is put right. It couldn't start as:\n" +
		"the config names no account\n"
	if shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}

	w.unstartable = nil
	w.launchd.calls = nil
	shown = section(t, w.runs(t, ""), "3. The service")
	want = "Setup is given another config file than the one the router serves, so it's installed afresh.\n" + installedAt + "The router is up: healthy, pid 4244.\n"
	if shown != want {
		t.Errorf("once it could start, the service step showed\n%s\nwant\n%s", shown, want)
	}
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
