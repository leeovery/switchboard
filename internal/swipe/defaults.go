package swipe

import (
	"context"
	"os"
	"os/exec"

	"github.com/leeovery/switchboard/internal/childenv"
)

// defaultsPath is macOS's defaults, by its path, as no other program of that
// name is to read the setting.
const defaultsPath = "/usr/bin/defaults"

// Defaults reads the Mac's scroll setting, com.apple.swipescrolldirection in
// the global domain, one setting for the trackpad and the mouse alike, and
// returns what defaults printed. It runs in no more of this process's
// environment than it needs.
func Defaults(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, defaultsPath, "read", "-g", "com.apple.swipescrolldirection")
	cmd.Env = childenv.Minimal(os.Getenv)
	return cmd.Output()
}
