//go:build unix

package service

import "syscall"

// groupOfItsOwn starts a process in a process group of its own.
func groupOfItsOwn() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
