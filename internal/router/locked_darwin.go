//go:build darwin

package router

import "golang.org/x/sys/unix"

// locked reports whether the file or folder at path is locked, by the user,
// as the Finder locks it, or by the system: either refuses a change to it,
// or a file made in it, with EPERM.
func locked(path string) bool {
	var st unix.Stat_t
	return unix.Stat(path, &st) == nil && st.Flags&(unix.UF_IMMUTABLE|unix.SF_IMMUTABLE) != 0
}
