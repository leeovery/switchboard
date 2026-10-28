//go:build !darwin

package setuptoken

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// openTerminal fails: setup-token is run on macOS's pseudo-terminals alone.
func openTerminal(uint16, uint16) (*os.File, *os.File, error) {
	return nil, nil, fmt.Errorf("can't open a pseudo-terminal on %s", runtime.GOOS)
}

// start fails, as openTerminal does.
func start(context.Context, string, []string, []string, *os.File) (*exec.Cmd, error) {
	return nil, fmt.Errorf("can't start a program on a pseudo-terminal on %s", runtime.GOOS)
}

// killGroup kills nothing, as start starts nothing.
func killGroup(int) error {
	return os.ErrProcessDone
}
