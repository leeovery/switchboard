package claude

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"
)

// fallbackVersion is the version requests claim when no CLI answers. A version
// older than a model's minimum loses that model's window, so keep it current.
const fallbackVersion = "2.1.283"

// versionTimeout bounds `claude --version`, which normally answers in milliseconds.
const versionTimeout = 5 * time.Second

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

var installedVersion = sync.OnceValue(func() string {
	home, _ := os.UserHomeDir()
	return systemCLI(home).version(context.Background())
})

// InstalledVersion returns the version of the Claude Code CLI installed here,
// such as "2.1.283", for a Prober to claim. The API rejects a version older
// than a model's minimum ("Claude Code X does not support this model"), so a
// pinned version would lose each new model's window the day it ships. It asks
// the CLI once per process, and returns a floor version when none answers
// within five seconds.
func InstalledVersion() string {
	return installedVersion()
}

// installedCLI finds and runs the claude command. Tests give it stand-ins, so
// they never run the real one.
type installedCLI struct {
	// paths are where installers put the binary, tried in order before PATH.
	paths    []string
	lookPath func(file string) (string, error)
	output   func(ctx context.Context, path string, args ...string) ([]byte, error)
}

func systemCLI(home string) installedCLI {
	return installedCLI{paths: installPaths(home), lookPath: exec.LookPath, output: commandOutput}
}

// installPaths lists where Claude Code's installers put the CLI. They come
// before PATH because a LaunchAgent runs with a minimal one.
func installPaths(home string) []string {
	paths := []string{
		filepath.Join(home, ".local", "bin", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
		filepath.Join(home, ".claude", "local", "claude"),
	}
	// Without a home directory, the home-relative paths would resolve against
	// the working directory.
	return slices.DeleteFunc(paths, func(path string) bool { return !filepath.IsAbs(path) })
}

func commandOutput(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).Output()
}

// version returns the first "x.y.z" that `claude --version` prints, or
// fallbackVersion when there's no CLI or it doesn't answer in time.
func (c installedCLI) version(ctx context.Context) string {
	out, err := c.versionOutput(ctx)
	if err != nil {
		return fallbackVersion
	}
	if v := versionPattern.Find(out); v != nil {
		return string(v)
	}
	return fallbackVersion
}

func (c installedCLI) versionOutput(ctx context.Context) ([]byte, error) {
	path, err := c.find()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	return c.output(ctx, path, "--version")
}

// find returns the first install path that holds a file, else the claude on PATH.
func (c installedCLI) find() (string, error) {
	for _, path := range c.paths {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return c.lookPath("claude")
}
