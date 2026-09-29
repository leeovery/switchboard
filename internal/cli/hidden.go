package cli

import (
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/charmbracelet/x/term"

	"github.com/leeovery/switchboard/internal/accounts"
)

// HiddenInput returns what reads a line typed at the terminal stdin is,
// without showing it, as a password is typed, or false when stdin isn't a
// terminal: Deps.Hidden, for the process's own stdin. An interrupt while it
// reads puts the terminal back as it was, showing what's typed again, and
// fails the read with accounts.ErrInterrupted, which Execute exits 130 for.
func HiddenInput(stdin io.Reader) (func() ([]byte, error), bool) {
	f, ok := stdin.(term.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		return nil, false
	}
	return func() ([]byte, error) {
		fd := f.Fd()
		state, err := term.GetState(fd)
		if err != nil {
			return nil, fmt.Errorf("read the terminal's state: %w", err)
		}
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
		return readUnseen(func() ([]byte, error) { return term.ReadPassword(fd) }, func() error { return term.Restore(fd, state) }, interrupts)
	}, true
}

// readUnseen reads a line with read, which has the terminal show nothing
// typed while it reads, unless an interrupt comes first: then restore puts
// the terminal back as it was, as read would have once it finished, and the
// read fails with accounts.ErrInterrupted. A read an interrupt cuts short is
// left to end with the process.
func readUnseen(read func() ([]byte, error), restore func() error, interrupts <-chan os.Signal) ([]byte, error) {
	type result struct {
		line []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := read()
		done <- result{line: line, err: err}
	}()
	select {
	case r := <-done:
		return r.line, r.err
	case <-interrupts:
		if err := restore(); err != nil {
			return nil, fmt.Errorf("%w, and the terminal couldn't be put back: %w", accounts.ErrInterrupted, err)
		}
		return nil, accounts.ErrInterrupted
	}
}
