package service

import (
	"bytes"
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
// standard output, as a Runner does, failing with what it printed, on its
// standard error and its standard output both, as which it writes a failure
// to may vary with what it's asked. It runs in no more of this process's
// environment than it needs, as Launchctl does.
func Plutil(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, plutilPath, args...)
	cmd.Env = childenv.Minimal(os.Getenv)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		if said := strings.Join(strings.Fields(stderr.String()+" "+out.String()), " "); said != "" {
			return out.Bytes(), fmt.Errorf("%s (%w)", said, err)
		}
	}
	return out.Bytes(), err
}
