//go:build unix

package handover

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// dupForExec duplicates f's descriptor as one exec leaves open, as dup's
// copies are. It's read through SyscallConn, as Fd would put the socket in
// blocking mode.
func dupForExec(f *os.File) (int, error) {
	raw, err := f.SyscallConn()
	if err != nil {
		return 0, err
	}
	fd, dupErr := -1, error(nil)
	if err := raw.Control(func(s uintptr) { fd, dupErr = syscall.Dup(int(s)) }); err != nil {
		return 0, err
	}
	return fd, dupErr
}

// listening fails unless the descriptor fd is a stream socket that isn't
// connected, as a listener is: macOS's getsockopt doesn't say whether a
// socket listens.
func listening(fd int) error {
	kind, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_TYPE)
	switch {
	case err != nil:
		return fmt.Errorf("not a socket: %w", err)
	case kind != syscall.SOCK_STREAM:
		return errors.New("not a stream socket")
	}
	if _, err := syscall.Getpeername(fd); !errors.Is(err, syscall.ENOTCONN) {
		return errors.New("a connected socket, not a listener")
	}
	return nil
}

func closeFD(fd int) {
	_ = syscall.Close(fd)
}
