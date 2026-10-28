//go:build !unix

package shell

import (
	"context"
	"fmt"
	"runtime"
)

// Run fails: the login shell is asked on a Unix system alone.
func Run(context.Context, string, []string, []string) ([]byte, error) {
	return nil, fmt.Errorf("can't ask a login shell on %s", runtime.GOOS)
}
