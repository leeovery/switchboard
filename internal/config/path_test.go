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

// envFrom returns a getenv backed by vars, so tests never read the real environment.
func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}
