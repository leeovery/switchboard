//go:build !unix

package service

import "syscall"

// groupOfItsOwn starts a process as it would be: process groups are Unix's.
func groupOfItsOwn() *syscall.SysProcAttr {
	return nil
}
