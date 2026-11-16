package swipe

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/leeovery/switchboard/internal/childenv"
)

// defaultsPath is macOS's defaults, by its path, as no other program of that
// name is to read the setting.
const defaultsPath = "/usr/bin/defaults"

// Defaults reads the Mac's scroll setting, com.apple.swipescrolldirection in
// the global domain, one setting for the trackpad and the mouse alike, and
// returns what defaults printed, as readSetting reads it. It runs in no more
// of this process's environment than it needs.
func Defaults(ctx context.Context) ([]byte, error) {
	return readSetting(ctx, defaultsPath)
}

// readSetting runs the defaults at path, and returns what it printed,
// waiting waitDelay at most for it once the program has exited or ctx has
// killed it. A program that exited as it should has answered, though what it
// left running held its output open.
func readSetting(ctx context.Context, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, "read", "-g", "com.apple.swipescrolldirection")
	cmd.Env = childenv.Minimal(os.Getenv)
	cmd.WaitDelay = waitDelay
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrWaitDelay) {
		return out, nil
	}
	return out, err
}
