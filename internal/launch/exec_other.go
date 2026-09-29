//go:build !unix

package launch

import (
	"fmt"
	"runtime"
)

// Exec fails: only a Unix system can replace a process with another.
func Exec(string, []string, []string) error {
	return fmt.Errorf("can't hand this process over to another on %s", runtime.GOOS)
}
