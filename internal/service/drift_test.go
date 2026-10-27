package service_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

func TestDrift(t *testing.T) {
	const processType = "\t<key>ProcessType</key>\n\t<string>Interactive</string>\n"
	carried := map[string]string{"XDG_CONFIG_HOME": "/Users/tester/.config", "XDG_STATE_HOME": "/Users/tester/.local/state"}
	tests := []struct {
		name string
		// installedEnv is the environment the plist in place was installed
		// from, and installed what it was installed with, its binary the
		// setup's.
		installedEnv map[string]string
		installed    service.InstallOptions
		// edit changes the plist install wrote, nil leaving it as it is.
		edit func(plist string) string
		// env is the environment Drift is asked in, and opts what it's
		// given, its binary the setup's.
		env  map[string]string
		opts service.InstallOptions
		want []string
	}{
		{name: "as install writes it"},
		{
			name:      "as install writes it, with a config file and a log level",
			installed: service.InstallOptions{Config: "/Users/tester/work.toml", LogLevel: "debug"},
			opts:      service.InstallOptions{Config: "/Users/tester/work.toml", LogLevel: "debug"},
		},
		{
			name:         "its keys in another order, and other whitespace",
			installedEnv: carried,
			edit: func(plist string) string {
				plist = strings.Replace(plist, processType, "", 1)
				plist = strings.Replace(plist, "<dict>\n", "<dict>\n"+processType, 1)
				plist = strings.Replace(plist,
					"\t\t<key>XDG_CONFIG_HOME</key>\n\t\t<string>/Users/tester/.config</string>\n\t\t<key>XDG_STATE_HOME</key>\n\t\t<string>/Users/tester/.local/state</string>\n",
					"\t\t<key>XDG_STATE_HOME</key>\n\t\t<string>/Users/tester/.local/state</string>\n\t\t<key>XDG_CONFIG_HOME</key>\n\t\t<string>/Users/tester/.config</string>\n", 1)
				plist = strings.Replace(plist, "<integer>", "<integer>\n  ", 1)
				return strings.NewReplacer("\t", "    ", "</key>\n", "</key>  ").Replace(plist)
			},
			env: carried,
		},
		{
			name: "from before ProcessType",
			edit: func(plist string) string { return strings.Replace(plist, processType, "", 1) },
			want: []string{"no ProcessType"},
		},
		{
			name: "a key switchboard doesn't write",
			edit: func(plist string) string {
				return strings.Replace(plist, "</dict>\n</plist>", "\t<key>Nice</key>\n\t<integer>1</integer>\n</dict>\n</plist>", 1)
			},
			want: []string{"an extra Nice"},
		},
		{
			name: "another value for a key",
			edit: func(plist string) string {
				return strings.Replace(plist, "<key>KeepAlive</key>\n\t<true/>", "<key>KeepAlive</key>\n\t<false/>", 1)
			},
			want: []string{"another KeepAlive"},
		},
		{
			name:      "another log level",
			installed: service.InstallOptions{LogLevel: "debug"},
			opts:      service.InstallOptions{LogLevel: "warn"},
			want:      []string{"another --log-level in its ProgramArguments"},
		},
		{
			name:      "a log level not given now, and a config file given",
			installed: service.InstallOptions{LogLevel: "debug"},
			opts:      service.InstallOptions{Config: "/Users/tester/work.toml"},
			want:      []string{"no --config in its ProgramArguments", "an extra --log-level in its ProgramArguments"},
		},
		{
			// The variables carried differ as the shells installing from do.
			name:         "a variable no longer carried, another one's value, and one switchboard doesn't carry: never counted",
			installedEnv: carried,
			edit: func(plist string) string {
				return strings.Replace(plist, "<key>EnvironmentVariables</key>\n\t<dict>", "<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>PATH</key>\n\t\t<string>/usr/bin</string>", 1)
			},
			env: map[string]string{"XDG_CONFIG_HOME": "/Users/tester/elsewhere"},
		},
		{
			name:      "a flag's value given after =",
			installed: service.InstallOptions{LogLevel: "debug"},
			edit: func(plist string) string {
				return strings.Replace(plist, "<string>--log-level</string>\n\t\t<string>debug</string>", "<string>--log-level=debug</string>", 1)
			},
			opts: service.InstallOptions{LogLevel: "debug"},
		},
		{
			name: "an extra flag, its value given after =, named alone",
			edit: func(plist string) string {
				return strings.Replace(plist, "<string>serve</string>", "<string>serve</string>\n\t\t<string>--listen=127.0.0.1:9999</string>", 1)
			},
			want: []string{"an extra --listen in its ProgramArguments"},
		},
		{
			name:      "a flag given twice",
			installed: service.InstallOptions{LogLevel: "debug"},
			edit: func(plist string) string {
				return strings.Replace(plist, "<string>debug</string>", "<string>debug</string>\n\t\t<string>--log-level=debug</string>", 1)
			},
			opts: service.InstallOptions{LogLevel: "debug"},
			want: []string{"a repeated --log-level in its ProgramArguments"},
		},
		{
			// switchboard owns the log's path, in the state directory.
			name: "launchd's log elsewhere",
			edit: func(plist string) string {
				return strings.Replace(plist, "/logs/launchd.log</string>", "/logs/other.log</string>", 1)
			},
			want: []string{"another StandardOutPath"},
		},
		{
			name: "another program",
			edit: func(plist string) string {
				return strings.Replace(plist, "<string>serve</string>", "<string>serve</string>\n\t\t<string>now</string>", 1)
			},
			want: []string{"another program in its ProgramArguments"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, tt.installedEnv, nil)
			tt.installed.Executable, tt.opts.Executable = s.binary, s.binary
			written, err := s.svc.PlistOf(tt.installed)
			if err != nil {
				t.Fatal(err)
			}
			plist := string(written)
			if tt.edit != nil {
				plist = tt.edit(plist)
			}
			s.putPlist(t, plist)
			cfg := s.cfg
			cfg.Getenv = func(key string) string {
				if key == "TMPDIR" {
					return s.tmp
				}
				return tt.env[key]
			}

			drift, err := newService(t, cfg).Drift(t.Context(), tt.opts)
			if err != nil || !slices.Equal(drift, tt.want) {
				t.Errorf("Drift() = %q, %v; want %q, for the plist\n%s", drift, err, tt.want, plist)
			}
		})
	}
}

func TestOptions(t *testing.T) {
	tests := []struct {
		name string
		opts service.InstallOptions
	}{
		{name: "installed with none"},
		{name: "installed with a config file", opts: service.InstallOptions{Config: "/Users/tester/it's -my- config.toml"}},
		{name: "installed with a log level", opts: service.InstallOptions{LogLevel: "debug"}},
		{name: "installed with both", opts: service.InstallOptions{Config: "/Users/tester/work.toml", LogLevel: "warn"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, nil)
			tt.opts.Executable = s.binary
			written, err := s.svc.PlistOf(tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			s.putPlist(t, string(written))

			got, err := s.svc.Options(t.Context())
			if err != nil || got.Executable != tt.opts.Executable || got.Config != tt.opts.Config || got.LogLevel != tt.opts.LogLevel {
				t.Errorf("Options() = %+v, %v; want %+v", got, err, tt.opts)
			}
		})
	}
}

func TestOptionsReadsFlagsAsServeParsesThem(t *testing.T) {
	s := newSetup(t, nil, nil)
	written, err := s.svc.PlistOf(service.InstallOptions{Executable: s.binary})
	if err != nil {
		t.Fatal(err)
	}
	s.putPlist(t, strings.Replace(string(written), "<string>serve</string>",
		"<string>serve</string>\n\t\t<string>--config=/Users/tester/work.toml</string>\n\t\t<string>--log-level</string>\n\t\t<string>debug</string>\n\t\t<string>--log-level</string>\n\t\t<string>warn</string>", 1))

	got, err := s.svc.Options(t.Context())
	if err != nil || got.Config != "/Users/tester/work.toml" || got.LogLevel != "warn" {
		t.Errorf("Options() = %+v, %v; want the config given after =, and the log level given last", got, err)
	}
}

func TestOptionsFailsForAPlistNamingNoProgram(t *testing.T) {
	for _, plist := range []string{
		"<plist/>\n",
		"<plist><dict/></plist>\n",
		"<plist><dict><key>EnvironmentVariables</key><dict><key>XDG_STATE_HOME</key><string>/Users/tester/state</string></dict></dict></plist>\n",
		"<plist><dict><key>ProgramArguments</key><array/></dict></plist>\n",
	} {
		t.Run(plist, func(t *testing.T) {
			s := newSetup(t, nil, nil)
			s.putPlist(t, plist)

			if opts, err := s.svc.Options(t.Context()); err == nil || !strings.HasSuffix(err.Error(), "names no program") {
				t.Errorf("Options() = %+v, %v; want it failing, as there's no program to keep the options of", opts, err)
			}
		})
	}
}

func TestDifferencesSetNestedDictionariesApartKeyByKey(t *testing.T) {
	const want = `<plist><dict><key>SoftResourceLimits</key><dict><key>NumberOfFiles</key><integer>4096</integer><key>Core</key><integer>0</integer></dict></dict></plist>`
	const have = `<plist><dict><key>SoftResourceLimits</key><dict><key>NumberOfFiles</key><integer>256</integer><key>Stack</key><integer>1</integer></dict></dict></plist>`

	got, err := service.Differences([]byte(want), []byte(have))
	if wantGot := []string{"another NumberOfFiles in its SoftResourceLimits", "no Core in its SoftResourceLimits", "an extra Stack in its SoftResourceLimits"}; err != nil || !slices.Equal(got, wantGot) {
		t.Errorf("Differences() = %q, %v; want %q", got, err, wantGot)
	}
}

func TestFor(t *testing.T) {
	s := newSetup(t, map[string]string{"XDG_STATE_HOME": "/Users/tester/custom"}, nil)
	written, err := s.svc.PlistOf(service.InstallOptions{Executable: s.binary})
	if err != nil {
		t.Fatal(err)
	}
	s.putPlist(t, string(written))
	// Asked from a shell setting another config, and no state home.
	cfg := s.cfg
	cfg.StateDir = filepath.Join(s.root, "home", ".local", "state", "switchboard")
	cfg.Getenv = func(key string) string {
		return map[string]string{"TMPDIR": s.tmp, "SWITCHBOARD_CONFIG": "/Users/tester/other.toml"}[key]
	}
	var asked []string
	cfg.RouterAt = func(dir string) service.Router {
		asked = append(asked, dir)
		return &fakeRouter{t: t, answer: func(int) (router.Health, error) { return up(4300) }}
	}
	opts, err := newService(t, cfg).Options(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	svc, err := newService(t, cfg).For(t.Context(), opts)
	if err != nil {
		t.Fatalf("For() error = %v", err)
	}
	if want := []string{"/Users/tester/custom/switchboard"}; !slices.Equal(asked, want) {
		t.Errorf("asked for the routers at %q, want %q", asked, want)
	}
	if got, want := svc.Log(), "/Users/tester/custom/switchboard/logs/launchd.log"; got != want {
		t.Errorf("Log() = %s, want %s, in the state directory the plist keeps", got, want)
	}
	for key, want := range map[string]string{"XDG_STATE_HOME": "/Users/tester/custom", "SWITCHBOARD_CONFIG": "", "TMPDIR": s.tmp} {
		if got := svc.Getenv(key); got != want {
			t.Errorf("Getenv(%s) = %q, want %q: the variables carried as installed, the rest as asked", key, got, want)
		}
	}
	if file, err := svc.ConfigFile(opts); err != nil || file != filepath.Join(s.root, "home", ".config", "switchboard", "config.toml") {
		t.Errorf("ConfigFile() = %s, %v; want the one the environment installed finds", file, err)
	}
	st, err := svc.Status(t.Context())
	if err != nil || st.Router == nil || st.Router.PID != 4300 {
		t.Errorf("Status() = %+v, %v; want the router at the state directory installed answering", st, err)
	}
}

func TestForWithoutARouterAtFails(t *testing.T) {
	s := newSetup(t, map[string]string{"XDG_STATE_HOME": "/Users/tester/custom"}, nil)
	written, err := s.svc.PlistOf(service.InstallOptions{Executable: s.binary})
	if err != nil {
		t.Fatal(err)
	}
	s.putPlist(t, string(written))
	cfg := s.cfg
	cfg.StateDir, cfg.RouterAt = filepath.Join(s.root, "home", ".local", "state", "switchboard"), nil
	cfg.Getenv = func(key string) string { return map[string]string{"TMPDIR": s.tmp}[key] }
	svc := newService(t, cfg)
	opts, err := svc.Options(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.For(t.Context(), opts); err == nil {
		t.Error("For() succeeded, want it failing, with no router to ask at the state directory installed")
	}
	if same, err := svc.For(t.Context(), service.InstallOptions{Executable: s.binary}); err != nil || same != svc {
		t.Errorf("For() = %p, %v; want the service itself for options not read back", same, err)
	}
}

func TestConfigFile(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		opts service.InstallOptions
		want string
	}{
		{name: "given", env: map[string]string{"SWITCHBOARD_CONFIG": "/Users/tester/other.toml"}, opts: service.InstallOptions{Config: "/Users/tester/work.toml"}, want: "/Users/tester/work.toml"},
		{name: "named by SWITCHBOARD_CONFIG", env: map[string]string{"SWITCHBOARD_CONFIG": "/Users/tester/other.toml"}, want: "/Users/tester/other.toml"},
		{name: "under XDG_CONFIG_HOME", env: map[string]string{"XDG_CONFIG_HOME": "/Users/tester/config"}, want: "/Users/tester/config/switchboard/config.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, tt.env, nil)

			if got, err := s.svc.ConfigFile(tt.opts); err != nil || got != tt.want {
				t.Errorf("ConfigFile() = %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestInstallKeepsTheEnvironmentAndTheLogOptionsReadBack(t *testing.T) {
	carried := map[string]string{"XDG_CONFIG_HOME": "/Users/tester/.config", "XDG_STATE_HOME": "<root>/state", "SWITCHBOARD_LOG_LEVEL": "warn"}
	s := newSetup(t, carried, func(asked int) (router.Health, error) { return up(4242 + asked) })
	if _, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary, LogLevel: "debug"}); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(s.plist)
	if err != nil {
		t.Fatal(err)
	}
	// A variable added by hand, which switchboard doesn't carry.
	installed := strings.Replace(string(written), "\t</dict>\n\t<key>RunAtLoad</key>", "\t\t<key>HTTPS_PROXY</key>\n\t\t<string>http://proxy.example:8080</string>\n\t</dict>\n\t<key>RunAtLoad</key>", 1)
	s.putPlist(t, installed)
	// Installing from another shell, its state elsewhere.
	cfg := s.cfg
	cfg.StateDir = filepath.Join(s.root, "elsewhere", "switchboard")
	cfg.Getenv = func(key string) string {
		return map[string]string{"TMPDIR": s.tmp, "XDG_STATE_HOME": filepath.Join(s.root, "elsewhere")}[key]
	}
	shell := newService(t, cfg)
	opts, err := shell.Options(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := shell.For(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Install(t.Context(), opts); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got, err := os.ReadFile(s.plist); err != nil || string(got) != installed || !strings.Contains(installed, "HTTPS_PROXY") {
		t.Errorf("the plist reads\n%s(%v)\nwant it as it was installed, its variables, all of them, and its log kept\n%s", got, err, installed)
	}
}

func TestDriftAndOptionsFailForAPlistTheyCantRead(t *testing.T) {
	tests := []struct {
		name string
		// plist is what's in the plist's place, "" for nothing.
		plist string
	}{
		{name: "none"},
		{name: "one that isn't XML, as a binary one isn't", plist: "bplist00\xd1\x01\x02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, nil)
			if tt.plist != "" {
				s.putPlist(t, tt.plist)
			}

			if opts, err := s.svc.Options(t.Context()); err == nil || !strings.Contains(err.Error(), "the service's plist") {
				t.Errorf("Options() = %+v, %v; want it failing to read the plist", opts, err)
			}
			if drift, err := s.svc.Drift(t.Context(), service.InstallOptions{Executable: s.binary}); err == nil || !strings.Contains(err.Error(), "the service's plist") {
				t.Errorf("Drift() = %q, %v; want it failing to read the plist", drift, err)
			}
			if tt.plist != "" {
				if got, err := os.ReadFile(s.plist); err != nil || string(got) != tt.plist {
					t.Errorf("the plist reads %q, %v; want it left as it was", got, err)
				}
			}
		})
	}
}
