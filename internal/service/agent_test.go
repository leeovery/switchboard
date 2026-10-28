package service_test

import (
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/service"
)

var update = flag.Bool("update", false, "rewrite the golden files with what the tests write")

func TestPlist(t *testing.T) {
	// The link Homebrew puts on PATH, which the plist names as it is.
	const binary = "/opt/homebrew/bin/switchboard"
	tests := []struct {
		name string
		env  map[string]string
		opts service.InstallOptions
	}{
		{name: "agent", opts: service.InstallOptions{Executable: binary}},
		{
			name: "agent-xdg",
			env: map[string]string{
				"XDG_CONFIG_HOME":       "/Users/tester/.config",
				"XDG_STATE_HOME":        "/Users/tester/.local/state",
				"XDG_DATA_HOME":         "/Users/tester/.local/share",
				"SWITCHBOARD_CONFIG":    "/Users/tester/.config/switchboard/work.toml",
				"SWITCHBOARD_LOG_LEVEL": "warn",
				"CLAUDE_CONFIG_DIR":     "/Users/tester/.config/claude",
			},
			opts: service.InstallOptions{
				Executable: binary,
				Config:     "/Users/tester/switchboard & co/config.toml",
				LogLevel:   "debug",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(t, service.Config{
				Home:     "/Users/tester",
				StateDir: "/Users/tester/.local/state/switchboard",
				Getenv:   func(key string) string { return tt.env[key] },
			})

			got, err := svc.PlistOf(tt.opts)
			if err != nil {
				t.Fatalf("PlistOf() error = %v", err)
			}
			path := filepath.Join("testdata", tt.name+".golden")
			if *update {
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden file (run with -update to create it): %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("plist reads\n%s\nwant (%s; run with -update to accept it)\n%s", got, path, want)
			}
		})
	}
}

func TestLaunchdRunsSwitchboardItself(t *testing.T) {
	const binary = "/opt/homebrew/bin/switchboard"
	tests := []struct {
		name string
		opts service.InstallOptions
		want []string
	}{
		{name: "serving", opts: service.InstallOptions{Executable: binary}, want: []string{binary, "serve"}},
		{
			name: "serving the config given, logging at the level given",
			opts: service.InstallOptions{Executable: binary, Config: "/Users/tester/it's my/config.toml", LogLevel: "debug"},
			want: []string{binary, "serve", "--config", "/Users/tester/it's my/config.toml", "--log-level", "debug"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(t, service.Config{Home: "/Users/tester", StateDir: "/Users/tester/.local/state/switchboard", Getenv: func(string) string { return "" }})

			program, err := svc.ProgramOf(tt.opts)
			if err != nil || !slices.Equal(program, tt.want) {
				t.Errorf("ProgramOf() = %q, %v, want %q: switchboard itself, needing none of the user's environment", program, err, tt.want)
			}
		})
	}
}

func TestLaunchdRunsTheRouterUnthrottled(t *testing.T) {
	svc := newService(t, service.Config{Home: "/Users/tester", StateDir: "/Users/tester/.local/state/switchboard", Getenv: func(string) string { return "" }})

	plist, err := svc.PlistOf(service.InstallOptions{Executable: "/opt/homebrew/bin/switchboard"})
	if err != nil {
		t.Fatalf("PlistOf() error = %v", err)
	}
	if got := stringOf(t, plist, "ProcessType"); got != "Interactive" {
		t.Errorf("ProcessType = %q, want %q: left unset, launchd throttles the CPU and I/O of the router every request waits on", got, "Interactive")
	}
}

// stringOf returns the string a plist's top dictionary holds under key, ""
// when it holds none.
func stringOf(t *testing.T, plist []byte, key string) string {
	t.Helper()
	var doc struct {
		Dict struct {
			Entries []struct {
				XMLName xml.Name
				Text    string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"dict"`
	}
	if err := xml.Unmarshal(plist, &doc); err != nil {
		t.Fatalf("plist isn't XML: %v", err)
	}
	entries := doc.Dict.Entries
	for i := 1; i < len(entries); i++ {
		if entries[i-1].XMLName.Local == "key" && entries[i-1].Text == key && entries[i].XMLName.Local == "string" {
			return entries[i].Text
		}
	}
	return ""
}

// newService returns the service as cfg configures it, on macOS, with
// launchctl and the router refusing to be asked unless cfg says otherwise.
func newService(t *testing.T, cfg service.Config) *service.Service {
	t.Helper()
	cfg.GOOS = "darwin"
	if cfg.Launchctl == nil {
		cfg.Launchctl = (&fakeLaunchctl{t: t, refuse: true}).run
	}
	if cfg.Router == nil {
		cfg.Router = &fakeRouter{t: t, refuse: true}
	}
	svc, err := service.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return svc
}
