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
	dir, err := baseDir(getenv, homeDir, "XDG_CONFIG_HOME", ".config")
	if err != nil {
		return "", fmt.Errorf("locate config: %w", err)
	}
	return filepath.Join(dir, "switchboard", "config.toml"), nil
}

// ThemesDir returns where the dashboard's theme files are:
// $SWITCHBOARD_THEMES_DIR, else $XDG_CONFIG_HOME/switchboard/themes, else
// ~/.config/switchboard/themes.
func ThemesDir(getenv func(string) string, homeDir func() (string, error)) (string, error) {
	if dir := getenv("SWITCHBOARD_THEMES_DIR"); dir != "" {
		return dir, nil
	}
	dir, err := baseDir(getenv, homeDir, "XDG_CONFIG_HOME", ".config")
	if err != nil {
		return "", fmt.Errorf("locate the themes directory: %w", err)
	}
	return filepath.Join(dir, "switchboard", "themes"), nil
}

// StateDir returns where switchboard keeps what it writes as it runs, such as
// its logs: $XDG_STATE_HOME/switchboard, else ~/.local/state/switchboard.
func StateDir(getenv func(string) string, homeDir func() (string, error)) (string, error) {
	dir, err := baseDir(getenv, homeDir, "XDG_STATE_HOME", filepath.Join(".local", "state"))
	if err != nil {
		return "", fmt.Errorf("locate state directory: %w", err)
	}
	return filepath.Join(dir, "switchboard"), nil
}

// BinDir returns switchboard's own directory for PATH, which holds the claude
// link alone: $XDG_DATA_HOME/switchboard/bin, else
// ~/.local/share/switchboard/bin.
func BinDir(getenv func(string) string, homeDir func() (string, error)) (string, error) {
	return dataDir(getenv, homeDir, "bin", "switchboard's bin directory")
}

// ShellDir returns where switchboard keeps its file for each shell, which the
// line setup adds to the shell's startup file sources:
// $XDG_DATA_HOME/switchboard/shell, else ~/.local/share/switchboard/shell.
func ShellDir(getenv func(string) string, homeDir func() (string, error)) (string, error) {
	return dataDir(getenv, homeDir, "shell", "switchboard's shell directory")
}

// dataDir returns switchboard's directory named name among its data, what it
// is saying which it is: $XDG_DATA_HOME/switchboard/<name>, else
// ~/.local/share/switchboard/<name>.
func dataDir(getenv func(string) string, homeDir func() (string, error), name, what string) (string, error) {
	dir, err := baseDir(getenv, homeDir, "XDG_DATA_HOME", filepath.Join(".local", "share"))
	if err != nil {
		return "", fmt.Errorf("locate %s: %w", what, err)
	}
	return filepath.Join(dir, "switchboard", name), nil
}

// baseDir returns the XDG base directory the variable names, else its default
// under the home directory. The XDG spec says to ignore a relative value,
// which would make the location depend on the working directory.
func baseDir(getenv func(string) string, homeDir func() (string, error), variable, fallback string) (string, error) {
	if dir := getenv(variable); filepath.IsAbs(dir) {
		return dir, nil
	}
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallback), nil
}
