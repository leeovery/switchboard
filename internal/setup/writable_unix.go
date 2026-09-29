//go:build unix

package setup

import "syscall"

// writeOK asks access(2) whether the user can write to a file: W_OK, which
// the syscall package doesn't name.
const writeOK = 0x2

// canWrite reports whether the user can write to the file at path, as the
// system judges it, owner, group, mode and ACLs alike.
func canWrite(path string) bool {
	return syscall.Access(path, writeOK) == nil
}
