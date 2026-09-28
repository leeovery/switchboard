package config

import (
	"fmt"
	"path/filepath"
)

// Path returns where the config file lives: $SWITCHBOARD_CONFIG, else
// $XDG_CONFIG_HOME/switchboard/config.toml, else ~/.config/switchboard/config.toml.
func Path(getenv func(string) string, homeDir func() (string, error)) (string, error) {
	if path := getenv("SWITCHBOARD_CONFIG"); path != "" {
		return path, nil
	}
	// The XDG spec says to ignore a relative XDG_CONFIG_HOME, which would make
	// the config's location depend on the working directory.
	if dir := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "switchboard", "config.toml"), nil
	}
	home, err := homeDir()
	if err != nil {
		return "", fmt.Errorf("locate config: %w", err)
	}
	return filepath.Join(home, ".config", "switchboard", "config.toml"), nil
}
