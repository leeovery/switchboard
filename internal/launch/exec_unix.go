//go:build unix

package launch

import "syscall"

// Exec replaces this process with the program at path, run as argv with env,
// as a shell's exec does: the program keeps the process's id, terminal and
// signals, so it behaves as if it had been started directly. Exec returns
// only when it fails.
func Exec(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
