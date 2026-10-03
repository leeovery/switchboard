//go:build unix

package cli

import (
	"errors"
	"fmt"
	"io"
	"syscall"
	"unsafe"

	"github.com/charmbracelet/x/term"
)

// tty is the process's terminal, as askBackground asks it, opened afresh, so
// reading it without waiting leaves the descriptor stdin shares with the
// shell as it was.
type tty struct {
	fd int
}

// openConsole opens the process's terminal, and returns what closes it.
func openConsole() (console, func(), error) {
	fd, err := syscall.Open("/dev/tty", syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open the terminal: %w", err)
	}
	return tty{fd: fd}, func() { _ = syscall.Close(fd) }, nil
}

// Foreground reports whether the process is in the terminal's foreground
// process group: one in the background is stopped as it reads the terminal,
// or changes its mode.
func (t tty) Foreground() bool {
	var group int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(t.fd), uintptr(syscall.TIOCGPGRP), uintptr(unsafe.Pointer(&group))); errno != 0 {
		return false
	}
	return int(group) == syscall.Getpgrp()
}

// Raw puts the terminal in raw mode, and returns what puts it back.
func (t tty) Raw() (func(), error) {
	state, err := term.MakeRaw(uintptr(t.fd))
	if err != nil {
		return nil, fmt.Errorf("put the terminal in raw mode: %w", err)
	}
	return func() { _ = term.Restore(uintptr(t.fd), state) }, nil
}

func (t tty) Write(p []byte) (int, error) {
	n, err := syscall.Write(t.fd, p)
	if err != nil {
		return 0, fmt.Errorf("write to the terminal: %w", err)
	}
	return n, nil
}

// ReadNow reads what the terminal has sent, without waiting for it.
func (t tty) ReadNow(p []byte) (int, error) {
	n, err := syscall.Read(t.fd, p)
	switch {
	case errors.Is(err, syscall.EAGAIN), errors.Is(err, syscall.EINTR):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("read the terminal: %w", err)
	case n == 0:
		return 0, io.EOF
	}
	return n, nil
}
