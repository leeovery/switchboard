package config_test

import (
	"errors"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestPath(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "SWITCHBOARD_CONFIG wins",
			env:  map[string]string{"SWITCHBOARD_CONFIG": "/etc/switchboard.toml", "XDG_CONFIG_HOME": "/xdg"},
			want: "/etc/switchboard.toml",
		},
		{
			name: "XDG_CONFIG_HOME",
			env:  map[string]string{"XDG_CONFIG_HOME": "/xdg"},
			want: "/xdg/switchboard/config.toml",
		},
		{
			name: "home directory",
			env:  nil,
			want: "/home/tester/.config/switchboard/config.toml",
		},
		{
			name: "empty variables count as unset",
			env:  map[string]string{"SWITCHBOARD_CONFIG": "", "XDG_CONFIG_HOME": ""},
			want: "/home/tester/.config/switchboard/config.toml",
		},
		{
			name: "relative XDG_CONFIG_HOME is ignored",
			env:  map[string]string{"XDG_CONFIG_HOME": "relative/config"},
			want: "/home/tester/.config/switchboard/config.toml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Path(envFrom(tt.env), home)
			if err != nil {
				t.Fatalf("Path() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPathWithoutHomeDirectory(t *testing.T) {
	errNoHome := errors.New("no home directory")
	noHome := func() (string, error) { return "", errNoHome }

	if _, err := config.Path(envFrom(map[string]string{"SWITCHBOARD_CONFIG": "/etc/switchboard.toml"}), noHome); err != nil {
		t.Errorf("Path() with SWITCHBOARD_CONFIG set: error = %v, want none", err)
	}
	if _, err := config.Path(envFrom(map[string]string{"XDG_CONFIG_HOME": "/xdg"}), noHome); err != nil {
		t.Errorf("Path() with XDG_CONFIG_HOME set: error = %v, want none", err)
	}
	if _, err := config.Path(envFrom(nil), noHome); !errors.Is(err, errNoHome) {
		t.Errorf("Path() with nothing set: error = %v, want %v", err, errNoHome)
	}
}

func TestThemesDir(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "SWITCHBOARD_THEMES_DIR wins",
			env:  map[string]string{"SWITCHBOARD_THEMES_DIR": "/dotfiles/themes", "XDG_CONFIG_HOME": "/xdg"},
			want: "/dotfiles/themes",
		},
		{name: "XDG_CONFIG_HOME", env: map[string]string{"XDG_CONFIG_HOME": "/xdg"}, want: "/xdg/switchboard/themes"},
		{name: "home directory", want: "/home/tester/.config/switchboard/themes"},
		{
			name: "empty variables count as unset",
			env:  map[string]string{"SWITCHBOARD_THEMES_DIR": "", "XDG_CONFIG_HOME": ""},
			want: "/home/tester/.config/switchboard/themes",
		},
		{
			name: "relative XDG_CONFIG_HOME is ignored",
			env:  map[string]string{"XDG_CONFIG_HOME": "relative/config"},
			want: "/home/tester/.config/switchboard/themes",
		},
		{
			name: "the config file named elsewhere doesn't move it",
			env:  map[string]string{"SWITCHBOARD_CONFIG": "/etc/switchboard.toml"},
			want: "/home/tester/.config/switchboard/themes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ThemesDir(envFrom(tt.env), home)
			if err != nil || got != tt.want {
				t.Errorf("ThemesDir() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestThemesDirWithoutHomeDirectory(t *testing.T) {
	errNoHome := errors.New("no home directory")
	noHome := func() (string, error) { return "", errNoHome }

	if _, err := config.ThemesDir(envFrom(map[string]string{"SWITCHBOARD_THEMES_DIR": "/dotfiles/themes"}), noHome); err != nil {
		t.Errorf("ThemesDir() with SWITCHBOARD_THEMES_DIR set: error = %v, want none", err)
	}
	if _, err := config.ThemesDir(envFrom(nil), noHome); !errors.Is(err, errNoHome) {
		t.Errorf("ThemesDir() with nothing set: error = %v, want %v", err, errNoHome)
	}
}

func TestStateDir(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "XDG_STATE_HOME",
			env:  map[string]string{"XDG_STATE_HOME": "/xdg/state"},
			want: "/xdg/state/switchboard",
		},
		{
			name: "home directory",
			env:  nil,
			want: "/home/tester/.local/state/switchboard",
		},
		{
			name: "empty XDG_STATE_HOME counts as unset",
			env:  map[string]string{"XDG_STATE_HOME": ""},
			want: "/home/tester/.local/state/switchboard",
		},
		{
			name: "relative XDG_STATE_HOME is ignored",
			env:  map[string]string{"XDG_STATE_HOME": "relative/state"},
			want: "/home/tester/.local/state/switchboard",
		},
		{
			name: "the config's variables don't move it",
			env:  map[string]string{"SWITCHBOARD_CONFIG": "/etc/switchboard.toml", "XDG_CONFIG_HOME": "/xdg/config"},
			want: "/home/tester/.local/state/switchboard",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.StateDir(envFrom(tt.env), home)
			if err != nil {
				t.Fatalf("StateDir() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("StateDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateDirWithoutHomeDirectory(t *testing.T) {
	errNoHome := errors.New("no home directory")
	noHome := func() (string, error) { return "", errNoHome }

	if _, err := config.StateDir(envFrom(map[string]string{"XDG_STATE_HOME": "/xdg/state"}), noHome); err != nil {
		t.Errorf("StateDir() with XDG_STATE_HOME set: error = %v, want none", err)
	}
	if _, err := config.StateDir(envFrom(nil), noHome); !errors.Is(err, errNoHome) {
		t.Errorf("StateDir() with nothing set: error = %v, want %v", err, errNoHome)
	}
}

func TestBinDir(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "XDG_DATA_HOME", env: map[string]string{"XDG_DATA_HOME": "/xdg/data"}, want: "/xdg/data/switchboard/bin"},
		{name: "home directory", want: "/home/tester/.local/share/switchboard/bin"},
		{name: "empty XDG_DATA_HOME counts as unset", env: map[string]string{"XDG_DATA_HOME": ""}, want: "/home/tester/.local/share/switchboard/bin"},
		{name: "relative XDG_DATA_HOME is ignored", env: map[string]string{"XDG_DATA_HOME": "relative/data"}, want: "/home/tester/.local/share/switchboard/bin"},
		{
			name: "the other XDG directories don't move it",
			env:  map[string]string{"XDG_CONFIG_HOME": "/xdg/config", "XDG_STATE_HOME": "/xdg/state"},
			want: "/home/tester/.local/share/switchboard/bin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.BinDir(envFrom(tt.env), home)
			if err != nil || got != tt.want {
				t.Errorf("BinDir() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestBinDirWithoutHomeDirectory(t *testing.T) {
	errNoHome := errors.New("no home directory")
	noHome := func() (string, error) { return "", errNoHome }

	if _, err := config.BinDir(envFrom(map[string]string{"XDG_DATA_HOME": "/xdg/data"}), noHome); err != nil {
		t.Errorf("BinDir() with XDG_DATA_HOME set: error = %v, want none", err)
	}
	if _, err := config.BinDir(envFrom(nil), noHome); !errors.Is(err, errNoHome) {
		t.Errorf("BinDir() with nothing set: error = %v, want %v", err, errNoHome)
	}
}

// envFrom returns a getenv backed by vars, so tests never read the real environment.
func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}
