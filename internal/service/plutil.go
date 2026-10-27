package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/leeovery/switchboard/internal/childenv"
)

// plutilPath is macOS's plutil, by its path, as no other program of that
// name is to read a plist.
const plutilPath = "/usr/bin/plutil"

// Plutil runs macOS's plutil with args, and returns what it printed on its
// standard output, as a Runner does, failing with what it printed on its
// standard error. It runs in no more of this process's environment than it
// needs, as Launchctl does.
func Plutil(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, plutilPath, args...)
	cmd.Env = childenv.Minimal(os.Getenv)
	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		if said := strings.TrimSpace(string(exit.Stderr)); said != "" {
			return out, fmt.Errorf("%s (%w)", said, err)
		}
	}
	return out, err
}
