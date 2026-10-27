package service_test

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

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
			name:         "a variable no longer carried, and another one's value",
			installedEnv: carried,
			env:          map[string]string{"XDG_CONFIG_HOME": "/Users/tester/elsewhere"},
			want:         []string{"another XDG_CONFIG_HOME in its EnvironmentVariables", "an extra XDG_STATE_HOME in its EnvironmentVariables"},
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
			if err != nil || !reflect.DeepEqual(got, tt.opts) {
				t.Errorf("Options() = %+v, %v; want %+v", got, err, tt.opts)
			}
		})
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
