package setup_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/setup"
)

// unreadBinary is why a binary plist plutil can't read can't be read back.
const unreadBinary = "read the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, through plutil: plutil: not a binary plist the world knows"

// leftRunning and errLeft are what the service step says, and fails with,
// of launchd running a router, pid 4242 as fmt puts it in, that doesn't
// answer.
const (
	leftRunning = "✗ Router: launchd runs it, %s, but it doesn't answer here: it may be starting, or stopping, which can take over half a minute after a bootout, " +
		"or keeping its state somewhere setup doesn't look; setup leaves it as it is: if it doesn't answer soon, as switchboard service status shows, " +
		"switchboard service install installs it again: give it the --config and --log-level it ran with, if any\n"
	errLeft = "launchd's router, %s, doesn't answer: if it doesn't soon, run switchboard service install"
)

// restarted are the launchctl subcommands a service step runs that
// restarts the router: it asks whether the service is loaded, then restarts
// it.
var restarted = []string{"print", "print", "kickstart"}

// What the service step says of the service installed afresh, and why, of
// a router that takes up setup's changes on its own, and of the router left
// with no account's token to serve with.
const (
	updatedRow = "! Updated: reinstalled: "
	takesUp    = "✓ Router: healthy · 127.0.0.1:4747 · takes up setup's changes on its own\n"
	tokenless  = "! Warning: no account has a usable token, so the router will have nothing to route to: switchboard accounts says why\n"
)

// dayless is doneConfig without priming, as a config from before setup.
var dayless = strings.TrimSuffix(doneConfig, "\n[prime]\nday = \"08:00-23:00\"\n")

func TestTheServiceStep(t *testing.T) {
	tests := []struct {
		name string
		// lay changes the world, which is done, before the step; change is
		// what setup changes before it, where anything.
		lay     func(t *testing.T, w *world)
		change  func(t *testing.T, s *setup.Setup)
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
			want:    installedRow + up(4243),
			wantRan: []string{"print", "print", "bootstrap"},
		},
		{
			name:    "installed, but launchd hasn't loaded it: installed again",
			lay:     func(_ *testing.T, w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			want:    installedRow + up(4243),
			wantRan: []string{"print", "print", "bootstrap"},
		},
		{
			name:    "installed and up, nothing changed: left as it is",
			want:    up(4242),
			wantRan: untouched,
		},
		{
			name: "installed and up, a token changed: left to the router, which takes it up",
			change: func(t *testing.T, s *setup.Setup) {
				got, err := s.PasteToken(t.Context(), setup.TokenFor{Account: config.NewAccount{ID: "side"}}, "test-token-side")
				if err != nil || got.Result != setup.TokenSaved {
					t.Fatalf("PasteToken() = %+v, %v; want it saved", got, err)
				}
			},
			want:    takesUp + routerPID(4242),
			wantRan: untouched,
		},
		{
			name: "installed and up, the config changed: left to the router, which takes it up",
			lay:  func(t *testing.T, w *world) { w.writeConfig(t, dayless) },
			change: func(t *testing.T, s *setup.Setup) {
				if _, err := s.SetDay("07:00-22:00"); err != nil {
					t.Fatal(err)
				}
			},
			want:    takesUp + routerPID(4242),
			wantRan: untouched,
		},
		{
			name:    "installed from another switchboard: installed afresh, to run this one",
			lay:     func(t *testing.T, w *world) { w.installService(t, w.goInstalled(t)) },
			want:    installedRow + updatedRow + "it ran <root>/go/bin/switchboard, not this switchboard, <root>/brew/bin/switchboard\n" + up(4244),
			wantRan: []string{"print", "print", "bootout", "print", "bootstrap"},
		},
		{
			name: "installed with a plist naming no program, not loaded: installed afresh, as setup installs it",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "<plist/>\n", 0o644)
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			want: installedRow + updatedRow + "its plist couldn't be read back " +
				"(the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program)\n" + up(4243),
			wantRan: []string{"print", "print", "bootstrap"},
		},
		{
			name:    "installed from before ProcessType: installed afresh",
			lay:     func(t *testing.T, w *world) { w.editPlist(t, withoutProcessType) },
			want:    installedRow + updatedRow + "its plist had no ProcessType\n" + up(4243),
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
			want:    up(4242),
			wantRan: untouched,
		},
		{
			name: "installed with a binary plist plutil can't read, not loaded: installed afresh",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "bplist00\xd1\x01\x02", 0o644)
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			want:    installedRow + updatedRow + "its plist couldn't be read back (" + unreadBinary + ")\n" + up(4243),
			wantRan: []string{"print", "print", "bootstrap"},
		},
		{
			name:    "installed, its router not answering: restarted",
			lay:     func(_ *testing.T, w *world) { w.launchd.pid = 0 },
			want:    "! Restarted: the router didn't answer\n" + up(4243),
			wantRan: restarted,
		},
		{
			name:    "installed, launchd running its router, which answers once asked again, as one starting does: up",
			lay:     func(_ *testing.T, w *world) { w.launchd.unanswered = 1 },
			want:    up(4242),
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
			want:    installedRow + tokenless + up(4243),
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
			s := w.setup(t)
			if tt.change != nil {
				tt.change(t, s)
			}

			got, err := w.serviced(t, s)
			if err != nil || got != tt.want {
				t.Errorf("the service step said\n%s(%v)\nwant\n%s", got, err, tt.want)
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
		// lay changes the world, which is done, before the step.
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

			w.service(t)
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

			if told := w.service(t); !strings.Contains(told, updatedRow) {
				t.Errorf("the service step said\n%s\nwant the service installed afresh", told)
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

func TestTheServiceStepKeepsTheServicesEnvironment(t *testing.T) {
	// The router's state directory, which holds no tokens, is the one the
	// router it starts is checked for them in.
	const apart = "! Elsewhere: the router uses another state directory and Claude Code config directory than setup: " +
		"switchboard service install, run from the shell the router should use, lines them up\n"
	tests := []struct {
		name string
		// lay changes the world, the service installed from serviceEnv,
		// before the step, given what was installed.
		lay func(t *testing.T, w *world)
		// executable is the switchboard the service was installed to run,
		// "" for this one.
		executable func(t *testing.T, w *world) string
		want       string
		wantRan    []string
	}{
		{
			name:    "up, as it was installed: left as it is, its router never restarted",
			want:    apart + up(4243),
			wantRan: untouched,
		},
		{
			name:    "from before ProcessType: installed afresh, keeping it",
			lay:     func(t *testing.T, w *world) { w.editPlist(t, withoutProcessType) },
			want:    apart + installedRow + updatedRow + "its plist had no ProcessType · kept its --log-level\n" + tokenless + up(4244),
			wantRan: []string{"print", "print", "bootout", "print", "bootstrap"},
		},
		{
			name:       "from another switchboard: installed afresh, keeping it",
			executable: func(t *testing.T, w *world) string { return w.goInstalled(t) },
			want: apart + installedRow + updatedRow + "it ran <root>/go/bin/switchboard, not this switchboard, <root>/brew/bin/switchboard · kept its --log-level\n" +
				tokenless + up(4244),
			wantRan: []string{"print", "print", "bootout", "print", "bootstrap"},
		},
		{
			name:    "not loaded: installed again, keeping it",
			lay:     func(_ *testing.T, w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			want:    installedRow + tokenless + apart + up(4244),
			wantRan: []string{"print", "print", "bootstrap"},
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

			told := w.service(t)
			if told != tt.want {
				t.Errorf("the service step said\n%s\nwant\n%s", told, tt.want)
			}
			for _, value := range []string{"<root>/custom", "<root>/claude-service", "<root>/home/.local/state"} {
				if strings.Contains(told, value) {
					t.Errorf("the service step said\n%s\nwant the places named, never where they are, such as %s", told, value)
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

// changedDay has s set priming's day afresh, as the priming page does, so
// the service step finds setup has changed the config.
func changedDay(t *testing.T, s *setup.Setup) *setup.Setup {
	t.Helper()
	if _, err := s.SetDay("07:00-22:00"); err != nil {
		t.Fatalf("SetDay() error = %v", err)
	}
	return s
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
			w.writeConfig(t, dayless)

			told, err := w.serviced(t, changedDay(t, w.setup(t)))
			if want := takesUp + routerPID(4243); err != nil || told != want {
				t.Errorf("the service step said\n%s(%v)\nwant\n%s", told, err, want)
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

	if told, want := w.service(t), up(4243); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
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

			if told := w.service(t); !strings.HasPrefix(told, installedRow+updatedRow+"its plist couldn't be read back (") {
				t.Errorf("the service step said\n%s\nwant the service installed afresh, its plist said unreadable", told)
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

// leftAsItIs is what the service step says, after why the service was to be
// installed afresh, where the router couldn't start so, its own running.
const leftAsItIs = ", but the router couldn't start installed afresh, so the service is left as it is, its router running: " +
	"switchboard service install, run from the shell the router should use, installs it afresh once what stops it is put right · it couldn't start as: "

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

	want := "! Left as it is: its plist had no ProcessType" + leftAsItIs + "no config file at <the config>\n\nCreate one like this:\n\n[[account]]\n" + routerPID(4243)
	if told := w.service(t); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
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

			told, err := w.serviced(t, w.setup(t))
			if strings.Contains(told, tokenShaped) || !strings.Contains(told, "plist can't be read back (") && !strings.Contains(told, "plist couldn't be read back (") {
				t.Errorf("the service step said\n%s\nwant why the plist can't be read back, redacted", told)
			}
			if installed := strings.Contains(told, updatedRow); installed == loaded || (err == nil) == loaded {
				t.Errorf("the service step said\n%s\nand Service() error = %v; want it installed afresh only where launchd hasn't loaded it", told, err)
			}
		})
	}
}

func TestTheServiceStepRedactsWhyTheRouterCouldntStart(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.editPlist(t, withoutProcessType)
	w.unstartable = errors.New("the config holds " + tokenShaped)

	if told := w.service(t); strings.Contains(told, tokenShaped) || !strings.Contains(told, "it couldn't start as: the config holds ") {
		t.Errorf("the service step said\n%s\nwant why the router couldn't start, redacted", told)
	}
}

// installAdvice is how the service step says to install the service once
// the router could start.
const installAdvice = "switchboard service install, run from the shell the router should use, installs it once what stops it is put right\n"

func TestTheServiceStepFailsWhereTheRouterCouldntStartAndNoneRuns(t *testing.T) {
	const couldntStart = ", but the router couldn't start installed afresh, and none is running: " + installAdvice
	tests := []struct {
		name string
		// lay changes the world, which is done, before the step.
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
			want: "✗ Not set up: the router couldn't start: the config names no account\n",
		},
		{
			name: "not loaded",
			lay:  func(_ *testing.T, w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			want: "✗ Router: launchd hasn't loaded it" + couldntStart,
		},
		{
			name: "drifted, its router down",
			lay: func(t *testing.T, w *world) {
				w.editPlist(t, withoutProcessType)
				w.launchd.pid = 0
			},
			want: "✗ Router: its plist had no ProcessType" + couldntStart,
		},
		{
			name: "loaded, its router down",
			lay:  func(_ *testing.T, w *world) { w.launchd.pid = 0 },
			want: "✗ Router: not running, and one couldn't start as the service runs it: " + installAdvice,
		},
		{
			name: "a plist it can't read, not loaded",
			lay: func(t *testing.T, w *world) {
				writeFile(t, w.plist(), "<plist/>\n", 0o644)
				w.launchd.loaded, w.launchd.pid = false, 0
			},
			want: "✗ Router: its plist couldn't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program)" + couldntStart,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			tt.lay(t, w)
			w.unstartable = errors.New("the config names no account")

			told, err := w.serviced(t, w.setup(t))
			if want := "the router couldn't start: the config names no account"; err == nil || err.Error() != want {
				t.Errorf("Service() error = %v, want %q", err, want)
			}
			if told != tt.want {
				t.Errorf("the service step said\n%s\nwant\n%s", told, tt.want)
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
	w.writeConfig(t, dayless)

	told, err := w.serviced(t, changedDay(t, w.setup(t)))
	// No warning of tokens for setup's accounts, which aren't the router's.
	want := "! Elsewhere: the router uses another state directory and config file than setup: switchboard service install, run from the shell the router should use, lines them up · " +
		"for the config file, switchboard setup --config, naming the router's, does too\n" +
		installedRow + updatedRow + "its plist had no ProcessType · kept its --config\n" + up(4244)
	if err != nil || told != want {
		t.Errorf("the service step said\n%s(%v)\nwant\n%s", told, err, want)
	}
}

func TestTheServiceStepPutsLaunchdsLogBackInTheStateDirectory(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	log := filepath.Join(w.state, "logs", "launchd.log")
	w.editPlist(t, func(plist string) string {
		return strings.ReplaceAll(plist, log, filepath.Join(w.root, "elsewhere", "router.out"))
	})

	want := installedRow + updatedRow + "its plist had another StandardOutPath and another StandardErrorPath\n" + up(4243)
	if told := w.service(t); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
	}
	if plist, err := os.ReadFile(w.plist()); err != nil || strings.Count(string(plist), "<string>"+log+"</string>") != 2 {
		t.Errorf("the plist reads\n%s(%v)\nwant launchd's log back at %s", plist, err, log)
	}
}

func TestTheServiceStepLeavesAServiceWhoseRouterCantBeTold(t *testing.T) {
	const left = "! Left as it is: no router answers where the service's plist keeps its state, but one answers where this shell keeps it, " +
		"so which the service runs can't be told: switchboard service install, run from the shell the router should use, lines them up\n"
	tests := []struct {
		name string
		// lay changes the world, which is done, its router answering at the
		// shell's state directory, before the step.
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

			if told := w.service(t); told != left {
				t.Errorf("the service step said\n%s\nwant\n%s", told, left)
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
				told, err := w.serviced(t, w.setup(t))
				if want := fmt.Sprintf(errLeft, tt.pid); err == nil || err.Error() != want {
					t.Errorf("Service() error = %v, want %q", err, want)
				}
				if want := fmt.Sprintf(leftRunning, tt.pid); told != want {
					t.Errorf("the service step said\n%s\nwant\n%s", told, want)
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

// runByHand is what the service step says of a router run some other way
// that answers, pid 5000, while launchd has what fmt puts in.
const runByHand = "✗ Router run by hand: a router run some other way answers, pid 5000, while %s: setup leaves them as they are; " +
	"stop the router run by hand, then run switchboard service install\n"

func TestTheServiceStepLeavesARouterRunByHand(t *testing.T) {
	const errHand = "a router run by hand answers: stop it, then run switchboard service install"
	tests := []struct {
		name string
		lay  func(w *world)
		want string
	}{
		{
			name: "launchd not loaded",
			lay:  func(w *world) { w.launchd.loaded, w.launchd.pid = false, 0 },
			want: fmt.Sprintf(runByHand, "launchd hasn't loaded the service"),
		},
		{
			name: "launchd loaded, running none",
			lay:  func(w *world) { w.launchd.pid = 0 },
			want: fmt.Sprintf(runByHand, "launchd has the service loaded, but runs no router"),
		},
		{
			name: "launchd running another, at a third state directory",
			lay: func(w *world) {
				w.launchd.stateDir, w.launchd.byHandAt = filepath.Join(w.root, "third", "switchboard"), filepath.Join(w.root, "state", "switchboard")
			},
			want: fmt.Sprintf(runByHand, "launchd runs a router, pid 4242, which doesn't answer where setup looks"),
		},
		{
			// Credited as launchd's only where launchd runs one.
			name: "launchd loaded, running none, the router saying no pid",
			lay:  func(w *world) { w.launchd.pid, w.launchd.anonymous = 0, true },
			want: strings.Replace(fmt.Sprintf(runByHand, "launchd has the service loaded, but runs no router"), "pid 5000", "its pid unknown", 1),
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

				told, err := w.serviced(t, w.setup(t))
				if err == nil || err.Error() != errHand {
					t.Errorf("Service() error = %v, want %q", err, errHand)
				}
				if told != tt.want {
					t.Errorf("the service step said\n%s\nwant\n%s", told, tt.want)
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
		{name: "launchd saying neither its state nor its pid", lay: func(w *world) { w.launchd.stateUnread = true }},
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

				told := w.service(t)
				want, ran := "✓ Router: healthy", untouched
				if drifted {
					want = installedRow + updatedRow + "its plist had no ProcessType\n✓ Router: healthy"
					ran = []string{"print", "print", "bootout", "print", "bootstrap"}
				}
				if !strings.HasPrefix(told, want) {
					t.Errorf("the service step said\n%s\nwant it to start\n%s", told, want)
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

	told, err := w.serviced(t, w.setup(t))
	if want := "a router run by hand answers: stop it, then run switchboard service install"; err == nil || err.Error() != want {
		t.Errorf("Service() error = %v, want %q", err, want)
	}
	if want := fmt.Sprintf(runByHand, "launchd runs a router, pid 4243, which doesn't answer where setup looks"); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q: nothing changed", ran, untouched)
	}
}

func TestTheServiceStepFailsWithNoPlistItCanUse(t *testing.T) {
	plists := []struct {
		name string
		// lay leaves the service with no plist setup can use.
		lay func(t *testing.T, w *world)
		// said and short are how the service step says so where launchd
		// has the service loaded, and in its error; unloaded where it
		// hasn't, if otherwise.
		said, short, unloaded string
	}{
		{
			name:     "no plist",
			lay:      removePlist,
			said:     "the service's plist is gone",
			short:    "the service's plist is gone",
			unloaded: "no service is installed",
		},
		{
			name:  "a plist it can't read",
			lay:   func(t *testing.T, w *world) { writeFile(t, w.plist(), "<plist/>\n", 0o644) },
			said:  "the service's plist can't be read back (the service's plist, <root>/home/Library/LaunchAgents/io.github.leeovery.switchboard.plist, names no program)",
			short: "the service's plist can't be read back",
		},
	}
	// What the step says after the plist's words, as fmt puts them in.
	const (
		leaves       = ": setup leaves it as it is, and switchboard service install writes the plist afresh\n"
		leftLaunchds = "✗ Plist: %s, though launchd runs its router, pid 4242, which answers" + leaves
		byHand       = "✗ Router run by hand: %s, and a router run some other way answers, pid 5000, while %s: setup leaves them as they are; " +
			"stop the router run by hand, then run switchboard service install\n"
		noneAnswers = "✗ Plist: %s, and no router answers, while %s" + leaves
		notLoaded   = "launchd hasn't loaded the service"
		notRunning  = "launchd has the service loaded, but runs no router"
		running     = "launchd runs a router, pid 4242, which doesn't answer where setup looks"
	)
	thirdDir := func(w *world) string { return filepath.Join(w.root, "third", "switchboard") }
	shellsDir := func(w *world) string { return filepath.Join(w.root, "state", "switchboard") }
	tests := []struct {
		name string
		// lay sets how launchd and the routers stand.
		lay      func(w *world)
		unloaded bool
		// rows says what the step says, given the plist's words, and err what
		// it fails with after its short words; but with no plist, where
		// installsWithNoPlist is set, setup installs the service.
		rows                func(said string) string
		err                 string
		installsWithNoPlist bool
		// rowsNoService is rows where the plist's words say no service is
		// installed, if otherwise.
		rowsNoService func(said string) string
	}{
		{
			name: "launchd's router answering",
			lay:  func(*world) {},
			rows: func(said string) string { return fmt.Sprintf(leftLaunchds, said) + routerPID(4242) },
			err:  ": run switchboard service install",
		},
		{
			name: "launchd's router answering, saying no pid",
			lay:  func(w *world) { w.launchd.anonymous = true },
			rows: func(said string) string { return fmt.Sprintf(leftLaunchds, said) + routerPID(0) },
			err:  ": run switchboard service install",
		},
		{
			name:     "a router run by hand answering, launchd not loaded",
			lay:      func(w *world) { w.launchd.loaded, w.launchd.pid, w.launchd.byHand = false, 0, 5000 },
			unloaded: true,
			rows:     func(said string) string { return fmt.Sprintf(byHand, said, notLoaded) },
			err:      ", and a router run by hand answers: stop it, then run switchboard service install",
			// No service is installed says launchd hasn't loaded it.
			rowsNoService: func(said string) string {
				return "✗ Router run by hand: " + said + ", and a router run some other way answers, pid 5000: setup leaves it as it is; " +
					"stop the router run by hand, then run switchboard service install\n"
			},
		},
		{
			name: "a router run by hand answering, launchd loaded but running none",
			lay:  func(w *world) { w.launchd.pid, w.launchd.byHand = 0, 5000 },
			rows: func(said string) string { return fmt.Sprintf(byHand, said, notRunning) },
			err:  ", and a router run by hand answers: stop it, then run switchboard service install",
		},
		{
			name: "a router run by hand answering where this shell keeps its state, launchd's at a third",
			lay: func(w *world) {
				w.launchd.stateDir, w.launchd.byHand, w.launchd.byHandAt = thirdDir(w), 5000, shellsDir(w)
			},
			rows: func(said string) string { return fmt.Sprintf(byHand, said, running) },
			err:  ", and a router run by hand answers: stop it, then run switchboard service install",
		},
		{
			name: "none answering, launchd loaded but running none",
			lay:  func(w *world) { w.launchd.pid = 0 },
			rows: func(said string) string { return fmt.Sprintf(noneAnswers, said, notRunning) },
			err:  ", and no router answers: run switchboard service install",
			// Nothing to swap out, and no options it can't keep.
			installsWithNoPlist: true,
		},
		{
			name: "none answering, launchd running one at a third state directory",
			lay:  func(w *world) { w.launchd.stateDir = thirdDir(w) },
			rows: func(said string) string { return fmt.Sprintf(noneAnswers, said, running) },
			err:  ", and no router answers: run switchboard service install",
		},
		{
			name: "none answering, launchd running one at a third state directory, naming no pid",
			lay:  func(w *world) { w.launchd.stateDir, w.launchd.pidless = thirdDir(w), true },
			rows: func(said string) string {
				return fmt.Sprintf(noneAnswers, said, "launchd runs a router, its pid unknown, which doesn't answer where setup looks")
			},
			err: ", and no router answers: run switchboard service install",
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
				said, short, rows := plist.said, plist.short, tt.rows
				if tt.unloaded && plist.unloaded != "" {
					said, short = plist.unloaded, plist.unloaded
					if tt.rowsNoService != nil {
						rows = tt.rowsNoService
					}
				}

				if tt.installsWithNoPlist && plist.name == "no plist" {
					if told, want := w.service(t), installedRow+up(4243); told != want {
						t.Errorf("the service step said\n%s\nwant\n%s", told, want)
					}
					if ran, want := w.launchd.ran(), []string{"print", "print", "bootout", "print", "bootstrap"}; !slices.Equal(ran, want) {
						t.Errorf("ran launchctl %q, want %q", ran, want)
					}
					return
				}
				synctest.Test(t, func(t *testing.T) {
					told, err := w.serviced(t, w.setup(t))
					if want := short + tt.err; err == nil || err.Error() != want {
						t.Errorf("Service() error = %v, want %q", err, want)
					}
					if want := rows(said); told != want {
						t.Errorf("the service step said\n%s\nwant\n%s", told, want)
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
		why = "where the service's router finds its state or its config can't be worked out from its plist (%s: the service's plist sets HOME empty, where the router finds no home directory)"
		// short is how the step's error names the plist, as fmt puts in
		// the place it can't work out.
		short    = "the service's plist: %s: the service's plist sets HOME empty, where the router finds no home directory"
		installs = "switchboard service install, run from the shell the router should use, installs it afresh\n"
		state    = "locate state directory"
		// plistRow and byHandRow are the rows the step says the plist's
		// state in, as fmt puts in why.
		plistRow  = "✗ Plist: %s"
		byHandRow = "✗ Router run by hand: %s"
	)
	unprinted := "launchctl print gui/" + strconv.Itoa(os.Getuid()) + "/io.github.leeovery.switchboard: Could not print: Input/output error (exit status 5)"
	tests := []struct {
		name string
		// key and dir are the variable the plist sets HOME empty in place
		// of, and the directory it held; place what can't be worked out.
		key, dir, place string
		lay             func(w *world)
		// row is the row the step says it in, and want what follows why.
		row, want, wantErr string
	}{
		{
			name: "the state directory, launchd's router answering", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay:     func(*world) {},
			row:     plistRow,
			want:    ", though launchd runs its router, pid 4242, which answers: setup leaves it as it is, but the plist can't start a router again, so " + installs + routerPID(4242),
			wantErr: ": run switchboard service install",
		},
		{
			name: "the state directory, launchd's router answering, saying no pid", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay:     func(w *world) { w.launchd.anonymous = true },
			row:     plistRow,
			want:    ", though launchd runs its router, pid 4242, which answers: setup leaves it as it is, but the plist can't start a router again, so " + installs + routerPID(0),
			wantErr: ": run switchboard service install",
		},
		{
			name: "the state directory, a router run by hand answering", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay: func(w *world) { w.launchd.pid, w.launchd.byHand = 0, 5000 },
			row: byHandRow,
			want: ", and a router run some other way answers, pid 5000, while launchd has the service loaded, but runs no router: setup leaves them as they are; " +
				"stop the router run by hand, then run switchboard service install\n",
			wantErr: ", and a router run by hand answers: stop it, then run switchboard service install",
		},
		{
			// Launchd's router at a third state directory.
			name: "the state directory, a router run by hand answering, launchd running another", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay: func(w *world) {
				w.launchd.stateDir, w.launchd.byHand, w.launchd.byHandAt = filepath.Join(w.root, "third", "switchboard"), 5000, filepath.Join(w.root, "state", "switchboard")
			},
			row: byHandRow,
			want: ", and a router run some other way answers, pid 5000, while launchd runs a router, pid 4242, which doesn't answer where setup looks: setup leaves them as they are; " +
				"stop the router run by hand, then run switchboard service install\n",
			wantErr: ", and a router run by hand answers: stop it, then run switchboard service install",
		},
		{
			name: "the state directory, none answering, launchd running one at a third state directory", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay:     func(w *world) { w.launchd.stateDir = filepath.Join(w.root, "third", "switchboard") },
			row:     plistRow,
			want:    ", and no router answers, while launchd runs a router, pid 4242, which doesn't answer where setup looks: " + installs,
			wantErr: ": run switchboard service install",
		},
		{
			name: "the state directory, launchd not to be asked", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay:     func(w *world) { w.launchd.printFails = true },
			row:     plistRow,
			want:    ": " + strings.TrimSuffix(installs, "\n") + " · launchd couldn't be asked about the service: " + unprinted + "\n",
			wantErr: ": run switchboard service install: " + unprinted,
		},
		{
			name: "the state directory, none answering", key: "XDG_STATE_HOME", dir: "state", place: state,
			lay:     func(w *world) { w.launchd.pid = 0 },
			row:     plistRow,
			want:    ", and no router answers: " + installs,
			wantErr: ": run switchboard service install",
		},
		{
			name: "the config file, none answering", key: "XDG_CONFIG_HOME", dir: "config", place: "locate config",
			lay:     func(w *world) { w.launchd.pid = 0 },
			row:     plistRow,
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

			synctest.Test(t, func(t *testing.T) {
				told, err := w.serviced(t, w.setup(t))
				if want := fmt.Sprintf(short, tt.place) + tt.wantErr; err == nil || err.Error() != want {
					t.Errorf("Service() error = %v, want %q", err, want)
				}
				if want := fmt.Sprintf(tt.row, fmt.Sprintf(why, tt.place)) + tt.want; told != want {
					t.Errorf("the service step said\n%s\nwant\n%s", told, want)
				}
			})
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

	told, err := w.serviced(t, w.setup(t))
	if err == nil || !strings.Contains(err.Error(), "run switchboard service install") {
		t.Errorf("Service() error = %v, want one naming switchboard service install, the plist's places unworked out", err)
	}
	if !strings.Contains(told, "though launchd runs its router, pid 4243, which answers: setup leaves it as it is") {
		t.Errorf("the service step said\n%s\nwant the service left, its router answering where the plist keeps its state", told)
	}
}

func TestTheServiceStepTakesTheConfigGivenForOneUnderAnEmptyHome(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	setsHomeEmpty(t, w, "XDG_CONFIG_HOME", "config")
	w.configFlag = w.config
	w.launchd.calls = nil

	if told := w.service(t); !strings.HasPrefix(told, installedRow+updatedRow+"setup is given another config file than the one the router served\n") {
		t.Errorf("the service step said\n%s\nwant the service installed afresh with the config given", told)
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

	want := "! Left as it is: setup is given another config file than the one the router served" + leftAsItIs + "the config names no account\n" + routerPID(4242)
	if told := w.service(t); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
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

	want := "! Skill: the router finds no Claude Code config directory, as its plist sets HOME empty, so it won't keep the skill up to date: " +
		"switchboard service install, run from the shell the router should use, lines that up\n" + up(4242)
	if told := w.service(t); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
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

			told, err := w.serviced(t, w.setup(t))
			if err == nil {
				t.Errorf("Service() error = nil, want the router said unable to start")
			}
			if strings.Contains(told, "uses another") {
				t.Errorf("the service step said\n%s\nwant nothing said of where a router finds things, with none answering", told)
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

			// Its places judged where it answered: the shell's state
			// directory, which holds setup's tokens.
			told := w.service(t)
			const elsewhere = "! Elsewhere: launchd started the router where this shell keeps its state, not where the service's plist does: " +
				"switchboard service install, run from the shell the router should use, lines them up\n"
			if want := elsewhere + up(4243); !strings.HasSuffix(told, want) {
				t.Errorf("the service step said\n%s\nwant it to end\n%s", told, want)
			}
			for _, never := range []string{"uses another", "Warning"} {
				if strings.Contains(told, never) {
					t.Errorf("the service step said\n%s\nwant nothing holding %q: the router's state directory and tokens are setup's", told, never)
				}
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran launchctl %q, want %q", ran, tt.wantRan)
			}
		})
	}
}

// stateApart is what the service step says of a router whose state
// directory is apart from setup's.
const stateApart = "! Elsewhere: the router uses another state directory than setup: switchboard service install, run from the shell the router should use, lines them up\n"

func TestTheServiceStepSettlesAServiceStartedAgain(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	custom := filepath.Join(w.root, "custom")
	w.installedFrom(t, map[string]string{"XDG_STATE_HOME": custom}, service.InstallOptions{Executable: w.switchboard})
	w.editPlist(t, withoutProcessType)
	w.launchd.pid = 0
	w.launchd.calls = nil

	// The router's state directory, apart from setup's, is said each time.
	if first := w.service(t); !strings.HasPrefix(first, installedRow+updatedRow+"its plist had no ProcessType\n") || !strings.Contains(first, stateApart) {
		t.Errorf("the service step said\n%s\nwant the service installed afresh, its state directory said apart", first)
	}
	if checked := []string{filepath.Join(custom, "switchboard") + " " + filepath.Join(w.root, "config", "switchboard", "config.toml")}; !slices.Equal(w.checked, checked) {
		t.Errorf("checked the router could start from %q, want %q: the plist's state directory", w.checked, checked)
	}
	installed, err := os.ReadFile(w.plist())
	if err != nil || !strings.Contains(string(installed), "<key>XDG_STATE_HOME</key>\n\t\t<string>"+custom+"</string>") {
		t.Fatalf("the plist reads\n%s(%v)\nwant the state home it kept, %s", installed, err, custom)
	}

	// Taken again, the router the plist runs answering: nothing flips back.
	w.launchd.calls = nil
	if second := w.service(t); !strings.HasPrefix(second, stateApart+"✓ Router: healthy") {
		t.Errorf("taken again, the service step said\n%s\nwant the service left as it is", second)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("taken again, ran launchctl %q, want %q", ran, untouched)
	}
	if again, err := os.ReadFile(w.plist()); err != nil || string(again) != string(installed) {
		t.Errorf("taken again, the plist reads\n%s(%v)\nwant it as the first left it\n%s", again, err, installed)
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

	// Its keys as plutil gives them, launchd's log among them, as written.
	if told, want := w.service(t), stateApart+up(4243); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
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

	want := stateApart + installedRow + updatedRow + "its plist had no ProcessType, another StandardOutPath and another StandardErrorPath\n" + tokenless + up(4244)
	if told := w.service(t); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
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

	// Refused, the router serves the other still.
	const given = "setup is given another config file than the one the router served"
	want := "! Left as it is: " + given + leftAsItIs + "the config names no account\n" + routerPID(4243)
	if told := w.service(t); told != want {
		t.Errorf("the service step said\n%s\nwant\n%s", told, want)
	}

	w.unstartable = nil
	w.launchd.calls = nil
	want = installedRow + updatedRow + given + "\n" + up(4244)
	if told := w.service(t); told != want {
		t.Errorf("once it could start, the service step said\n%s\nwant\n%s", told, want)
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

func TestTheServiceStepFailsWhenTheRouterDoesntAnswer(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	if err := os.Remove(w.plist()); err != nil {
		t.Fatal(err)
	}
	w.launchd.loaded, w.launchd.pid, w.launchd.dead = false, 0, true
	synctest.Test(t, func(t *testing.T) {
		_, err := w.serviced(t, w.setup(t))
		want := "the router didn't answer within 5s of starting: see why with switchboard logs router, and in " + w.state + "/logs/launchd.log"
		if err == nil || err.Error() != want {
			t.Errorf("Service() error = %v, want %q", err, want)
		}
	})
}
