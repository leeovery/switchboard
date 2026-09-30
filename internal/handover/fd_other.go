//go:build !unix

package handover

import (
	"fmt"
	"os"
	"runtime"
)

// dupForExec fails: only a Unix system hands a descriptor over across exec.
func dupForExec(*os.File) (int, error) {
	return 0, fmt.Errorf("can't hand a listener over on %s", runtime.GOOS)
}

// listening fails: only a Unix system hands a descriptor over across exec.
func listening(int) error {
	return fmt.Errorf("can't take a listener up on %s", runtime.GOOS)
}

func closeFD(int) {}
