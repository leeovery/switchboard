package setup_test

import (
	"cmp"
	"errors"
	"fmt"
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

// leftRunning and errLeft are what the service step says, and fails with,
// of launchd running a router, pid 4242 as fmt puts it in, that doesn't
// answer.
const (
	leftRunning = "Launchd runs the service's router, %s, but it doesn't answer here: it may be starting, or stopping, which can take over half a minute after a bootout, " +
		"or keeping its state somewhere setup doesn't look. Setup leaves it as it is: if it doesn't answer soon, as switchboard service status shows, " +
		"switchboard service install installs it again: give it the --config and --log-level it ran with, if any.\n"
	errLeft = "launchd's router, %s, doesn't answer: if it doesn't soon, run switchboard service install"
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
			wantRan: []string{"print", "print", "bootstrap"},
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
			name:    "installed, launchd running its router, which answers once asked again, as one starting does: up",
			lay:     func(_ *testing.T, w *world) { w.launchd.unanswered = 1 },
			answers: []string{""},
			want:    "The service is installed, and the router is up: healthy, pid 4242.\n",
			wantRan: []string{"print", "print"},
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
			wantRan: []string{"print", "print", "bootstrap"},
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
	want := "The service's plist differs from the one this switchboard writes, with no ProcessType, " +
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
	for name, loaded := range map[string]bool{"loaded: left": true, "not loaded, none running: installed afresh": false} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644)
			w.plutilSays = "plutil: not a plist, but " + tokenShaped
			if !loaded {
				w.launchd.loaded, w.launchd.pid = false, 0
			}

			all, err := w.run(t, "")
			shown := section(t, all, "3. The service")
			if strings.Contains(shown, tokenShaped) || !strings.Contains(shown, "The service's plist can't be read back (") {
				t.Errorf("the service step showed\n%s\nwant why the plist can't be read back, redacted", shown)
			}
			if installed := strings.Contains(shown, "so it's installed afresh"); installed == loaded || (err == nil) == loaded {
				t.Errorf("the service step showed\n%s\nand Run() error = %v; want it installed afresh only where launchd hasn't loaded it", shown, err)
			}
		})
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
			name: "the plist sets a state directory of its own, launchd's router saying no pid",
			lay: func(t *testing.T, w *world) {
				w.installedFrom(t, map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom")}, service.InstallOptions{Executable: w.switchboard})
				w.launchd.stateDir = filepath.Join(w.root, "state", "switchboard")
				w.launchd.anonymous = true
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
	for _, tt := range []struct {
		name             string
		drifted, pidless bool
		pid              string
	}{
		{name: "as written", pid: "pid 4242"},
		{name: "its plist differing", drifted: true, pid: "pid 4242"},
		{name: "launchd naming no pid", pidless: true, pid: "its pid unknown"},
		{name: "launchd naming no pid, its plist differing", drifted: true, pidless: true, pid: "its pid unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			// launchctl setenv gave launchd a third state home, where the
			// router runs.
			w.launchd.setenv = map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "third")}
			w.editPlist(t, func(plist string) string {
				plist = strings.Replace(plist, "\t\t<key>XDG_STATE_HOME</key>\n\t\t<string>"+filepath.Join(w.root, "state")+"</string>\n", "", 1)
				// launchd's log where the plist's state directory has it.
				return strings.ReplaceAll(plist, service.LogIn(filepath.Join(w.root, "state", "switchboard")), service.LogIn(filepath.Join(w.home, ".local", "state", "switchboard")))
			})
			w.launchd.stateDir = filepath.Join(w.root, "third", "switchboard")
			w.launchd.pidless = tt.pidless
			if tt.drifted {
				w.editPlist(t, withoutProcessType)
			}
			before, err := os.ReadFile(w.plist())
			if err != nil {
				t.Fatal(err)
			}
			w.launchd.calls = nil
			w.unstartable = errors.New("the config names no account")

			synctest.Test(t, func(t *testing.T) {
				shown, err := w.run(t, "")
				if want := fmt.Sprintf(errLeft, tt.pid); err == nil || err.Error() != want {
					t.Errorf("Run() error = %v, want %q", err, want)
				}
				if got, want := section(t, shown, "3. The service"), fmt.Sprintf(leftRunning, tt.pid); got != want {
					t.Errorf("the service step showed\n%s\nwant\n%s", got, want)
				}
			})
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

func TestTheServiceStepLeavesARouterRunByHand(t *testing.T) {
	const left = "A router run some other way answers, pid 5000, while %s: setup leaves them as they are. Stop the router run by hand, then run switchboard service install.\n"
	const errHand = "a router run by hand answers: stop it, then run switchboard service install"
	tests := []struct {
		name string
		lay  func(w *world)
		want string
	}{
		{
			name: "launchd not loaded",
			lay:  func(w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			want: fmt.Sprintf(left, "launchd hasn't loaded the service"),
		},
		{
			name: "launchd loaded, running none",
			lay:  func(w *world) { w.launchd.pid = 0 },
			want: fmt.Sprintf(left, "launchd has the service loaded, but runs no router"),
		},
		{
			name: "launchd running another, at a third state directory",
			lay: func(w *world) {
				w.launchd.stateDir, w.launchd.byHandAt = filepath.Join(w.root, "third", "switchboard"), filepath.Join(w.root, "state", "switchboard")
			},
			want: fmt.Sprintf(left, "launchd runs a router, pid 4242, which doesn't answer where setup looks"),
		},
		{
			// Credited as launchd's only where launchd runs one.
			name: "launchd loaded, running none, the router saying no pid",
			lay:  func(w *world) { w.launchd.pid, w.launchd.anonymous = 0, true },
			want: strings.Replace(fmt.Sprintf(left, "launchd has the service loaded, but runs no router"), "pid 5000", "its pid unknown", 1),
		},
	}
	for _, tt := range tests {
		for name, drifted := range map[string]bool{"as written": false, "its plist differing": true} {
			t.Run(tt.name+", "+name, func(t *testing.T) {
				w := newWorld(t)
				w.done(t)
				if drifted {
					w.editPlist(t, withoutProcessType)
				}
				w.launchd.byHand = 5000
				tt.lay(w)
				before, err := os.ReadFile(w.plist())
				if err != nil {
					t.Fatal(err)
				}
				w.launchd.calls = nil

				shown, err := w.run(t, "")
				if err == nil || err.Error() != errHand {
					t.Errorf("Run() error = %v, want %q", err, errHand)
				}
				if got := section(t, shown, "3. The service"); got != tt.want {
					t.Errorf("the service step showed\n%s\nwant\n%s", got, tt.want)
				}
				if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
					t.Errorf("ran launchctl %q, want %q: nothing changed", ran, untouched)
				}
				if after, err := os.ReadFile(w.plist()); err != nil || string(after) != string(before) {
					t.Errorf("the plist reads\n%s(%v)\nwant it as it was", after, err)
				}
			})
		}
	}
}

func TestTheServiceStepTakesARouterWithoutAPidForLaunchds(t *testing.T) {
	tests := []struct {
		name string
		// lay leaves the router or launchd naming no pid.
		lay func(w *world)
	}{
		{name: "the router saying no pid", lay: func(w *world) { w.launchd.anonymous = true }},
		{name: "launchd naming no pid", lay: func(w *world) { w.launchd.pidless = true }},
	}
	for _, tt := range tests {
		for name, drifted := range map[string]bool{"as written": false, "its plist differing": true} {
			t.Run(tt.name+", "+name, func(t *testing.T) {
				w := newWorld(t)
				w.done(t)
				if drifted {
					w.editPlist(t, withoutProcessType)
				}
				tt.lay(w)
				w.launchd.calls = nil

				shown := section(t, w.runs(t, ""), "3. The service")
				want, ran := "The service is installed, and the router is up: ", untouched
				if drifted {
					want = "The service's plist differs from the one this switchboard writes, with no ProcessType, so it's installed afresh.\n" + installedAt + "The router is up: "
					ran = []string{"print", "print", "bootout", "print", "bootstrap"}
				}
				if !strings.HasPrefix(shown, want) {
					t.Errorf("the service step showed\n%s\nwant it to start\n%s", shown, want)
				}
				if got := w.launchd.ran(); !slices.Equal(got, ran) {
					t.Errorf("ran launchctl %q, want %q", got, ran)
				}
			})
		}
	}
}

func TestTheServiceStepLeavesARouterRunByHandWhereThisShellKeepsItsState(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	// The plist keeps its state elsewhere; launchd's router answers at a
	// third state directory, and one run by hand where this shell keeps it.
	w.installedFrom(t, map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom")}, service.InstallOptions{Executable: w.switchboard})
	w.launchd.stateDir = filepath.Join(w.root, "third", "switchboard")
	w.launchd.byHand, w.launchd.byHandAt = 5000, filepath.Join(w.root, "state", "switchboard")
	w.launchd.calls = nil

	shown, err := w.run(t, "")
	if want := "a router run by hand answers: stop it, then run switchboard service install"; err == nil || err.Error() != want {
		t.Errorf("Run() error = %v, want %q", err, want)
	}
	want := "A router run some other way answers, pid 5000, while launchd runs a router, pid 4243, which doesn't answer where setup looks: setup leaves them as they are. " +
		"Stop the router run by hand, then run switchboard service install.\n"
	if got := section(t, shown, "3. The service"); got != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", got, want)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q: nothing changed", ran, untouched)
	}
}

func TestTheServiceStepFailsWithNoPlistItCanUse(t *testing.T) {
	const unread = "The service's plist can't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program)"
	plists := []struct {
		name string
		// lay leaves the service with no plist setup can use.
		lay func(t *testing.T, w *world)
		// said and short are how the service step says so where launchd
		// has the service loaded, and in its error; unloaded where it
		// hasn't, if otherwise.
		said, short, unloaded, shortUnloaded string
	}{
		{
			name:     "no plist",
			lay:      removePlist,
			said:     "The service's plist is gone",
			short:    "the service's plist is gone",
			unloaded: "No service is installed", shortUnloaded: "no service is installed",
		},
		{
			name:  "a plist it can't read",
			lay:   func(t *testing.T, w *world) { writeFile(t, w.plist(), "<plist/>\n", 0o644) },
			said:  unread,
			short: "the service's plist can't be read back",
		},
	}
	const (
		leftLaunchds = ", though launchd runs its router, pid 4242, which answers: setup leaves it as it is, and switchboard service install writes the plist afresh.\n"
		byHand       = ", and a router run some other way answers, pid 5000, while %s: setup leaves them as they are. Stop the router run by hand, then run switchboard service install.\n"
		noneAnswers  = ", and no router answers, while %s: setup leaves it as it is, and switchboard service install writes the plist afresh.\n"
		notLoaded    = "launchd hasn't loaded the service"
		notRunning   = "launchd has the service loaded, but runs no router"
		running      = "launchd runs a router, pid 4242, which doesn't answer where setup looks"
	)
	thirdDir := func(w *world) string { return filepath.Join(w.root, "third", "switchboard") }
	shellsDir := func(w *world) string { return filepath.Join(w.root, "state", "switchboard") }
	tests := []struct {
		name string
		// lay sets how launchd and the routers stand.
		lay      func(w *world)
		unloaded bool
		// line and err are what the step says after the plist's words, and
		// fails with after its short words; but with no plist, where
		// installsWithNoPlist is set, setup installs the service.
		line, err           string
		installsWithNoPlist bool
		// lineNoService is line where the plist's words say no service is
		// installed, if otherwise.
		lineNoService string
	}{
		{
			name: "launchd's router answering",
			lay:  func(*world) {},
			line: leftLaunchds, err: ": run switchboard service install",
		},
		{
			name: "launchd's router answering, saying no pid",
			lay:  func(w *world) { w.launchd.anonymous = true },
			line: leftLaunchds, err: ": run switchboard service install",
		},
		{
			name:     "a router run by hand answering, launchd not loaded",
			lay:      func(w *world) { w.launchd.loaded, w.launchd.pid, w.launchd.byHand = false, 0, 5000 },
			unloaded: true,
			line:     fmt.Sprintf(byHand, notLoaded), err: ", and a router run by hand answers: stop it, then run switchboard service install",
			// No service is installed says launchd hasn't loaded it.
			lineNoService: ", and a router run some other way answers, pid 5000: setup leaves it as it is. Stop the router run by hand, then run switchboard service install.\n",
		},
		{
			name: "a router run by hand answering, launchd loaded but running none",
			lay:  func(w *world) { w.launchd.pid, w.launchd.byHand = 0, 5000 },
			line: fmt.Sprintf(byHand, notRunning), err: ", and a router run by hand answers: stop it, then run switchboard service install",
		},
		{
			name: "a router run by hand answering where this shell keeps its state, launchd's at a third",
			lay: func(w *world) {
				w.launchd.stateDir, w.launchd.byHand, w.launchd.byHandAt = thirdDir(w), 5000, shellsDir(w)
			},
			line: fmt.Sprintf(byHand, running), err: ", and a router run by hand answers: stop it, then run switchboard service install",
		},
		{
			name: "none answering, launchd loaded but running none",
			lay:  func(w *world) { w.launchd.pid = 0 },
			line: fmt.Sprintf(noneAnswers, notRunning), err: ", and no router answers: run switchboard service install",
			// Nothing to swap out, and no options it can't keep.
			installsWithNoPlist: true,
		},
		{
			name: "none answering, launchd running one at a third state directory",
			lay:  func(w *world) { w.launchd.stateDir = thirdDir(w) },
			line: fmt.Sprintf(noneAnswers, running), err: ", and no router answers: run switchboard service install",
		},
		{
			name: "none answering, launchd running one at a third state directory, naming no pid",
			lay:  func(w *world) { w.launchd.stateDir, w.launchd.pidless = thirdDir(w), true },
			line: fmt.Sprintf(noneAnswers, "launchd runs a router, its pid unknown, which doesn't answer where setup looks"), err: ", and no router answers: run switchboard service install",
		},
	}
	for _, plist := range plists {
		for _, tt := range tests {
			t.Run(plist.name+", "+tt.name, func(t *testing.T) {
				w := newWorld(t)
				w.done(t)
				plist.lay(t, w)
				tt.lay(w)
				w.launchd.calls = nil
				said, short, line := plist.said, plist.short, tt.line
				if tt.unloaded && plist.unloaded != "" {
					said, short, line = plist.unloaded, plist.shortUnloaded, cmp.Or(tt.lineNoService, tt.line)
				}

				if tt.installsWithNoPlist && plist.name == "no plist" {
					shown := section(t, w.runs(t, ""), "3. The service")
					if want := installedAt + "The router is up: healthy, pid 4243.\n"; shown != want {
						t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
					}
					if ran, want := w.launchd.ran(), []string{"print", "print", "bootout", "print", "bootstrap"}; !slices.Equal(ran, want) {
						t.Errorf("ran launchctl %q, want %q", ran, want)
					}
					return
				}
				synctest.Test(t, func(t *testing.T) {
					shown, err := w.run(t, "")
					if want := short + tt.err; err == nil || err.Error() != want {
						t.Errorf("Run() error = %v, want %q", err, want)
					}
					if got, want := section(t, shown, "3. The service"), said+line; got != want {
						t.Errorf("the service step showed\n%s\nwant\n%s", got, want)
					}
				})
				if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
					t.Errorf("ran launchctl %q, want %q: the service left alone", ran, untouched)
				}
			})
		}
	}
}

// removePlist removes the service's plist, which launchd keeps loaded.
func removePlist(t *testing.T, w *world) {
	t.Helper()
	if err := os.Remove(w.plist()); err != nil {
		t.Fatal(err)
	}
}

// setsHomeEmpty has the service's plist set HOME empty in place of the
// variable key, which held the directory dir under the world's root, so the
// place it named is under HOME; or beside it, for "".
func setsHomeEmpty(t *testing.T, w *world, key, dir string) {
	t.Helper()
	w.editPlist(t, func(plist string) string {
		empty := "\t\t<key>HOME</key>\n\t\t<string></string>\n"
		if key == "" {
			return strings.Replace(plist, "\t<dict>\n\t\t<key>", "\t<dict>\n"+empty+"\t\t<key>", 1)
		}
		return strings.Replace(plist, "\t\t<key>"+key+"</key>\n\t\t<string>"+filepath.Join(w.root, dir)+"</string>\n", empty, 1)
	})
	if plist, err := os.ReadFile(w.plist()); err != nil || !strings.Contains(string(plist), "<key>HOME</key>") {
		t.Fatalf("the plist reads\n%s(%v)\nwant HOME set empty", plist, err)
	}
}

func TestTheServiceStepCantWorkOutThePlacesOfAnEmptyHome(t *testing.T) {
	const (
		why = "Where the service's router finds its state or its config can't be worked out from its plist (%s: the service's plist sets HOME empty, where the router finds no home directory)"
		// short is how the step's error names the plist, as fmt puts in
		// the place it can't work out.
		short    = "the service's plist: %s: the service's plist sets HOME empty, where the router finds no home directory"
		installs = "switchboard service install, run from the shell the router should use, installs it afresh.\n"
		state    = "locate state directory"
	)
	tests := []struct {
		name string
		// key and dir are the variable the plist sets HOME empty in place
		// of, and the directory it held; place what can't be worked out.
		key, dir, place string
		lay             func(w *world)
		want, wantErr   string
	}{
		{
			name: "the state directory, launchd's router answering", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay:     func(*world) {},
			want:    ", though launchd runs its router, pid 4242, which answers: setup leaves it as it is, but the plist can't start a router again, so " + installs,
			wantErr: ": run switchboard service install",
		},
		{
			name: "the state directory, launchd's router answering, saying no pid", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay:     func(w *world) { w.launchd.anonymous = true },
			want:    ", though launchd runs its router, pid 4242, which answers: setup leaves it as it is, but the plist can't start a router again, so " + installs,
			wantErr: ": run switchboard service install",
		},
		{
			name: "the state directory, a router run by hand answering", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay: func(w *world) { w.launchd.pid, w.launchd.byHand = 0, 5000 },
			want: ", and a router run some other way answers, pid 5000, while launchd has the service loaded, but runs no router: setup leaves them as they are. " +
				"Stop the router run by hand, then run switchboard service install.\n",
			wantErr: ", and a router run by hand answers: stop it, then run switchboard service install",
		},
		{
			name: "the state directory, none answering", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay:     func(w *world) { w.launchd.pid = 0 },
			want:    ", and no router answers: " + installs,
			wantErr: ": run switchboard service install",
		},
		{
			name: "the config file, none answering", key: "XDG_CONFIG_HOME", dir: "config", place: "locate config",
			lay:     func(w *world) { w.launchd.pid = 0 },
			want:    ", and no router answers: " + installs,
			wantErr: ": run switchboard service install",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			setsHomeEmpty(t, w, tt.key, tt.dir)
			tt.lay(w)
			w.launchd.calls = nil

			shown, err := w.run(t, "")
			if want := fmt.Sprintf(short, tt.place) + tt.wantErr; err == nil || err.Error() != want {
				t.Errorf("Run() error = %v, want %q", err, want)
			}
			if got, want := section(t, shown, "3. The service"), fmt.Sprintf(why, tt.place)+tt.want; got != want {
				t.Errorf("the service step showed\n%s\nwant\n%s", got, want)
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
				t.Errorf("ran launchctl %q, want %q: the service left alone", ran, untouched)
			}
		})
	}
}

func TestTheServiceStepAsksTheRouterAtThePlistsStateDirectoryWhereItsConfigCantBeWorkedOut(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	// The plist keeps its state elsewhere, where launchd's router answers;
	// its config file is under the HOME it sets empty.
	w.installedFrom(t, map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom")}, service.InstallOptions{Executable: w.switchboard})
	setsHomeEmpty(t, w, "XDG_CONFIG_HOME", "config")
	w.launchd.calls = nil

	shown, err := w.run(t, "")
	if err == nil || !strings.Contains(err.Error(), "run switchboard service install") {
		t.Errorf("Run() error = %v, want one naming switchboard service install, the plist's places unworked out", err)
	}
	if got := section(t, shown, "3. The service"); !strings.Contains(got, "though launchd runs its router, pid 4243, which answers: setup leaves it as it is") {
		t.Errorf("the service step showed\n%s\nwant the service left, its router answering where the plist keeps its state", got)
	}
}

func TestTheServiceStepTakesTheConfigGivenForOneUnderAnEmptyHome(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	setsHomeEmpty(t, w, "XDG_CONFIG_HOME", "config")
	w.configFlag = w.config
	w.launchd.calls = nil

	shown := section(t, w.runs(t, ""), "3. The service")
	if !strings.HasPrefix(shown, "Setup is given another config file than the one the router serves, so it's installed afresh.\n") {
		t.Errorf("the service step showed\n%s\nwant the service installed afresh with the config given", shown)
	}
	if plist, err := os.ReadFile(w.plist()); err != nil || !strings.Contains(string(plist), "<string>--config</string>\n\t\t<string>"+w.config+"</string>") {
		t.Errorf("the plist reads\n%s(%v)\nwant --config %s", plist, err, w.config)
	}
}

func TestTheServiceStepLeavesARouterBeforeItsPlacesWhereTheConfigGivenCouldntStart(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	// The plist's config is under the HOME it sets empty; the config given
	// couldn't start a router; the router answers.
	setsHomeEmpty(t, w, "XDG_CONFIG_HOME", "config")
	w.configFlag = w.config
	w.unstartable = errors.New("the config names no account")
	w.launchd.calls = nil

	shown := section(t, w.runs(t, ""), "3. The service")
	want := "Setup is given another config file than the one the router serves, but the router couldn't start installed afresh, so the service is left as it is, its router running: " +
		"switchboard service install, run from the shell the router should use, installs it afresh once what stops it is put right. It couldn't start as:\n" +
		"the config names no account\n"
	if shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q", ran, untouched)
	}
}

func TestTheServiceStepCarriesOnWithoutClaudeCodesConfigUnderAnEmptyHome(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	setsHomeEmpty(t, w, "CLAUDE_CONFIG_DIR", "claude")
	w.launchd.calls = nil

	shown := section(t, w.runs(t, ""), "3. The service")
	want := "The router finds no Claude Code config directory, as its plist sets HOME empty, so it won't keep the skill up to date: " +
		"switchboard service install, run from the shell the router should use, lines that up.\n" +
		"The service is installed, and the router is up: healthy, pid 4242.\n"
	if shown != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", shown, want)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q", ran, untouched)
	}
}

func TestTheServiceStepSaysNothingOfPlacesWithNoRouterAnswering(t *testing.T) {
	for name, drifted := range map[string]bool{"restarted": false, "installed afresh": true} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			// The plist keeps its state apart from setup's; no router runs,
			// and none could start.
			w.installedFrom(t, map[string]string{"XDG_STATE_HOME": filepath.Join(w.root, "custom")}, service.InstallOptions{Executable: w.switchboard})
			if drifted {
				w.editPlist(t, withoutProcessType)
			}
			w.launchd.pid = 0
			w.unstartable = errors.New("the config names no account")

			shown, err := w.run(t, "")
			if err == nil {
				t.Errorf("Run() error = nil, want the router said unable to start")
			}
			if got := section(t, shown, "3. The service"); strings.Contains(got, "uses another") {
				t.Errorf("the service step showed\n%s\nwant nothing said of where a router finds things, with none answering", got)
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
	want := "Setup is given another config file than the one the router serves, but the router couldn't start installed afresh, so the service is left as it is, its router running: " +
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
