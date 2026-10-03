//go:build !unix

package cli

import (
	"fmt"
	"runtime"
)

// openConsole fails: only a Unix system's terminal is asked its background.
func openConsole() (console, func(), error) {
	return nil, nil, fmt.Errorf("no terminal to ask on %s", runtime.GOOS)
}
