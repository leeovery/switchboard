//go:build !unix

package notify

import "syscall"

// detached starts a program as any other is started where there are no
// process groups to start it in.
func detached() *syscall.SysProcAttr {
	return nil
}
