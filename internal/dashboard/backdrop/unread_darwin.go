//go:build darwin

package backdrop

import "golang.org/x/sys/unix"

// fread is FREAD of <sys/fcntl.h>, which x/sys doesn't name: TIOCFLUSH given
// it flushes the terminal's input alone, as tcflush's TCIFLUSH does.
const fread = 0x1

// dropTyped throws away what the terminal fd holds of what was typed at it,
// unread.
func dropTyped(fd uintptr) error {
	return unix.IoctlSetPointerInt(int(fd), unix.TIOCFLUSH, fread)
}
