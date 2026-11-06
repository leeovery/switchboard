//go:build unix

package notify

import "syscall"

// detached starts a program in a process group of its own, so an interrupt
// at the terminal that started it doesn't end it.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
