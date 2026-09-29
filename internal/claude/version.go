package claude

import (
	"cmp"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/childenv"
	"github.com/leeovery/switchboard/internal/logs"
)

var logger = logs.For("claude")

// fallbackVersion is the version requests claim when no CLI answers. A version
// older than a model's minimum loses that model's window, so keep it current.
const fallbackVersion = "2.1.283"

const (
	// versionTimeout bounds `claude --version`, which normally answers in
	// milliseconds.
	versionTimeout = 5 * time.Second
	// versionLifetime is how long a version the CLI gave stands before the
	// CLI is asked again.
	versionLifetime = time.Hour
)

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// InstalledVersion returns what gives the version of the Claude Code CLI
// installed here, such as "2.1.283", for a Prober to claim: the claude Find
// finds, on the PATH getenv gives, else where its installers put it for the
// home directory homeDir gives, passing over switchboard's own executable, as
// executable gives it. The API rejects a version older than a model's minimum
// ("Claude Code X does not support this model"), so a pinned version would
// lose each new model's window the day it ships. What it returns asks the CLI
// at most once an hour, so a process that runs for days follows the CLI's
// updates. When the CLI doesn't answer within five seconds, the version it
// last gave stands, or a floor version before it has given one.
func InstalledVersion(getenv func(key string) string, homeDir, executable func() (string, error)) func() string {
	installed := &versionCache{
		ask: func() (string, error) {
			// Without a home directory, only the install paths outside it are
			// tried.
			home, _ := homeDir()
			return systemCLI(getenv, home, executable).version(context.Background())
		},
		now: time.Now,
	}
	return installed.get
}

// versionCache keeps the version the CLI last gave for an hour.
type versionCache struct {
	// ask asks the CLI for its version.
	ask func() (string, error)
	now func() time.Time

	mu sync.Mutex
	// version is what the CLI last gave: empty until it has given one.
	version string
	// asked is when the CLI was last asked: zero until it has been.
	asked time.Time
}

// get returns the version, asking the CLI first when it hasn't been asked in
// the last hour. An ask that fails counts as one.
func (c *versionCache) get() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now := c.now(); c.asked.IsZero() || now.Sub(c.asked) >= versionLifetime {
		c.refresh()
		c.asked = now
	}
	return cmp.Or(c.version, fallbackVersion)
}

// refresh asks the CLI for its version. When it doesn't give one, the version
// it last gave stands, since a CLI that answered before and doesn't now is
// more likely mid-update than gone; before it has given one, the floor does,
// and probes may lose a model newer than the floor.
func (c *versionCache) refresh() {
	version, err := c.ask()
	switch {
	case err == nil:
		logger.Debug("read the claude CLI's version", "version", version)
		c.version = version
	case c.version != "":
		logger.Debug("claude CLI gave no version; keeping the last", "version", c.version, "error", err)
	default:
		logger.Warn("claude CLI gave no version; claiming the floor", "version", fallbackVersion, "error", err)
	}
}

// installedCLI finds and runs the claude command. Tests give it stand-ins, so
// they never run the real one.
type installedCLI struct {
	// pathList is PATH's value, paths are where installers put the binary,
	// and executable gives switchboard's own, as Find takes them.
	pathList   string
	paths      []string
	executable func() (string, error)
	// env is the environment the command runs in, but for its own
	// directories, which go first on PATH.
	env    []string
	output func(ctx context.Context, env []string, path string, args ...string) ([]byte, error)
}

// systemCLI is the claude command installed here, found on the PATH getenv
// gives, else where its installers put it for the home directory home,
// passing over switchboard's own executable, as executable gives it. It runs
// in no more of the environment getenv gives than it needs, which can hold a
// token, as a Claude Code session's does, its own directories first on PATH,
// as childenv.Beside puts them.
func systemCLI(getenv func(key string) string, home string, executable func() (string, error)) installedCLI {
	return installedCLI{
		pathList:   getenv("PATH"),
		paths:      InstallPaths(home),
		executable: executable,
		env:        childenv.Minimal(getenv),
		output:     commandOutput,
	}
}

func commandOutput(ctx context.Context, env []string, path string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	return cmd.Output()
}

// version returns the first "x.y.z" that `claude --version` prints. It fails
// when there's no CLI, it doesn't answer in time, or it prints no version.
func (c installedCLI) version(ctx context.Context) (string, error) {
	out, err := c.versionOutput(ctx)
	if err != nil {
		return "", err
	}
	v := versionPattern.Find(out)
	if v == nil {
		return "", errors.New("claude --version printed no version")
	}
	return string(v), nil
}

func (c installedCLI) versionOutput(ctx context.Context) ([]byte, error) {
	path, err := Find(c.pathList, c.paths, c.executable)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	return c.output(ctx, childenv.Beside(c.env, path), path, "--version")
}
