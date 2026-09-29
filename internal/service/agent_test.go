package service_test

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/service"
)

var update = flag.Bool("update", false, "rewrite the golden files with what the tests write")

func TestPlist(t *testing.T) {
	// The link Homebrew puts on PATH, which the plist names as it is.
	const binary = "/opt/homebrew/bin/switchboard"
	tests := []struct {
		name    string
		env     map[string]string
		envFile string
		config  string
	}{
		{name: "agent"},
		{name: "agent-env-file", envFile: "/Users/tester/.config/tokens.env"},
		{
			name: "agent-xdg",
			env: map[string]string{
				"XDG_CONFIG_HOME":    "/Users/tester/.config",
				"XDG_STATE_HOME":     "/Users/tester/.local/state",
				"SWITCHBOARD_CONFIG": "/Users/tester/.config/switchboard/work.toml",
			},
			envFile: "/Users/tester/.config/tokens.env",
			config:  "/Users/tester/switchboard & co/config.toml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(t, service.Config{
				Home:     "/Users/tester",
				StateDir: "/Users/tester/.local/state/switchboard",
				Getenv:   func(key string) string { return tt.env[key] },
			})

			got, err := svc.PlistOf(binary, tt.envFile, tt.config)
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

func TestTheEnvFileLauncherServesWithTheTokens(t *testing.T) {
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("no zsh at /bin/zsh")
	}
	// Paths zsh would read as script, were they pasted into it.
	dir := filepath.Join(t.TempDir(), `it's "$(here)"`)
	envFile := filepath.Join(dir, "tokens; env")
	binary := filepath.Join(dir, "switch board")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte("export CLAUDE_TOKEN_WORK=test-token-work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" \"token=${CLAUDE_TOKEN_WORK-unset}\"\n"
	if err := os.WriteFile(binary, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	svc := newService(t, service.Config{Home: t.TempDir(), StateDir: t.TempDir(), Getenv: func(string) string { return "" }})
	config := filepath.Join(dir, "my config.toml")

	program := svc.ProgramOf(binary, envFile, config)
	cmd := exec.CommandContext(t.Context(), program[0], program[1:]...)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}

	out, err := cmd.CombinedOutput()
	if want := "serve\n--config\n" + config + "\ntoken=test-token-work\n"; err != nil || string(out) != want {
		t.Errorf("launchd's program printed\n%s(%v)\nwant\n%s", out, err, want)
	}
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
