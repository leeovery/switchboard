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

			drift, err := newService(t, cfg).Drift(tt.opts)
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

			got, err := s.svc.Options()
			if err != nil || got.Executable != tt.opts.Executable || got.Config != tt.opts.Config || got.LogLevel != tt.opts.LogLevel {
				t.Errorf("Options() = %+v, %v; want %+v", got, err, tt.opts)
			}
		})
	}
}

func TestInstallKeepsTheEnvironmentAndTheLogOptionsReadBack(t *testing.T) {
	carried := map[string]string{"XDG_CONFIG_HOME": "/Users/tester/.config", "XDG_STATE_HOME": "/Users/tester/.local/state", "SWITCHBOARD_LOG_LEVEL": "warn"}
	s := newSetup(t, carried, func(asked int) (router.Health, error) { return up(4242 + asked) })
	if _, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary, LogLevel: "debug"}); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(s.plist)
	if err != nil {
		t.Fatal(err)
	}
	// Installing from another shell, its state elsewhere.
	cfg := s.cfg
	cfg.StateDir = filepath.Join(s.root, "elsewhere", "switchboard")
	cfg.Getenv = func(key string) string {
		return map[string]string{"TMPDIR": s.tmp, "XDG_STATE_HOME": filepath.Join(s.root, "elsewhere")}[key]
	}
	svc := newService(t, cfg)

	opts, err := svc.Options()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Install(t.Context(), opts); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got, err := os.ReadFile(s.plist); err != nil || string(got) != string(installed) {
		t.Errorf("the plist reads\n%s(%v)\nwant it as it was installed, its variables and its log kept\n%s", got, err, installed)
	}
}

func TestOtherEnvironment(t *testing.T) {
	carried := map[string]string{"XDG_CONFIG_HOME": "/Users/tester/.config", "XDG_STATE_HOME": "/Users/tester/.local/state"}
	tests := []struct {
		name string
		// installedEnv is the environment the plist in place was installed
		// from, and env the one OtherEnvironment is asked in.
		installedEnv, env map[string]string
		want              []string
	}{
		{name: "none set either side"},
		{name: "the same", installedEnv: carried, env: carried},
		{
			name:         "one set otherwise, one unset here, and one set here alone",
			installedEnv: carried,
			env:          map[string]string{"XDG_STATE_HOME": "/Users/tester/elsewhere", "CLAUDE_CONFIG_DIR": "/Users/tester/.claude"},
			want:         []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "CLAUDE_CONFIG_DIR"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, tt.installedEnv, nil)
			written, err := s.svc.PlistOf(service.InstallOptions{Executable: s.binary})
			if err != nil {
				t.Fatal(err)
			}
			s.putPlist(t, string(written))
			cfg := s.cfg
			cfg.Getenv = func(key string) string { return tt.env[key] }
			svc := newService(t, cfg)
			opts, err := svc.Options()
			if err != nil {
				t.Fatal(err)
			}

			other, err := svc.OtherEnvironment(opts)
			if err != nil || !slices.Equal(other, tt.want) {
				t.Errorf("OtherEnvironment() = %q, %v; want %q", other, err, tt.want)
			}
		})
	}
}

func TestOtherEnvironmentOfOptionsNotReadBackIsNone(t *testing.T) {
	s := newSetup(t, map[string]string{"XDG_STATE_HOME": "/Users/tester/.local/state"}, nil)

	other, err := s.svc.OtherEnvironment(service.InstallOptions{Executable: s.binary})
	if err != nil || other != nil {
		t.Errorf("OtherEnvironment() = %q, %v; want none: Install writes these options with the environment it's run in", other, err)
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

			if opts, err := s.svc.Options(); err == nil || !strings.Contains(err.Error(), "the service's plist") {
				t.Errorf("Options() = %+v, %v; want it failing to read the plist", opts, err)
			}
			if drift, err := s.svc.Drift(service.InstallOptions{Executable: s.binary}); err == nil || !strings.Contains(err.Error(), "the service's plist") {
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
