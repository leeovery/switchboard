package service

import (
	"context"
	"os"
	"os/exec"

	"github.com/leeovery/switchboard/internal/childenv"
)

// plutilPath is macOS's plutil, by its path, as no other program of that
// name is to read a plist.
const plutilPath = "/usr/bin/plutil"

// Plutil runs macOS's plutil with args, and returns what it printed on its
// standard output, as a Runner does. It runs in no more of this process's
// environment than it needs, as Launchctl does.
func Plutil(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, plutilPath, args...)
	cmd.Env = childenv.Minimal(os.Getenv)
	return cmd.Output()
}
