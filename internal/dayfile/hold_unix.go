//go:build unix

package dayfile

import (
	"errors"
	"os"
	"syscall"
)

// holdAcross holds file, one of a day's plain files, open, against other
// processes, as Files says: exclusive, to rename a file rewritten into its
// place, else shared, to append to it. It waits while another process holds
// it otherwise, trying again when a signal cuts the wait short, and lets go
// as file closes. Where it can't hold it, as on a file system without locks,
// it carries on without.
func holdAcross(file *os.File, exclusive bool) {
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	for {
		err := syscall.Flock(int(file.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			return
		}
	}
}
