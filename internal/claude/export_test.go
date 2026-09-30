package claude

import "time"

// standInTimeout is how long a test gives a stand-in for the CLI to answer,
// output and all, in place of versionTimeout: a machine hard at work can take
// seconds to start a program, which is no failure of what's tested.
const standInTimeout = time.Minute

// InstalledVersionOfStandIn is InstalledVersion, the CLI, a stand-in, given
// standInTimeout to answer.
func InstalledVersionOfStandIn(getenv func(key string) string, homeDir, executable func() (string, error)) func() string {
	return installedVersion(getenv, homeDir, executable, standInTimeout)
}
