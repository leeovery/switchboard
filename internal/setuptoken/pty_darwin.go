//go:build darwin

package setuptoken

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// openTerminal opens a pseudo-terminal, columns wide and rows high: its
// master, which setup-token's drawing is read from, and the terminal
// setup-token runs on.
func openTerminal(columns, rows uint16) (master, tty *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	tty, err = openItsTerminal(master)
	if err == nil {
		// Darwin sizes a pseudo-terminal by its terminal alone: its master
		// refuses the request.
		err = unix.IoctlSetWinsize(int(tty.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: columns, Row: rows})
	}
	if err != nil {
		_ = master.Close()
		if tty != nil {
			_ = tty.Close()
		}
		return nil, nil, err
	}
	return master, tty, nil
}

// openItsTerminal grants and unlocks master's terminal, as grantpt and
// unlockpt do, and opens it. Darwin names it by the minor number the two
// share, as ptsname does, by a request that only a direct system call can
// make; the terminal opened is checked to be that one, never another.
func openItsTerminal(master *os.File) (*os.File, error) {
	minor, err := deviceMinor(master)
	if err != nil {
		return nil, err
	}
	if err := unlock(master); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("/dev/ttys%03d", minor)
	tty, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	if its, err := deviceMinor(tty); err != nil || its != minor {
		_ = tty.Close()
		return nil, errors.Join(fmt.Errorf("%s isn't the pseudo-terminal opened", name), err)
	}
	return tty, nil
}

// unlock grants and unlocks master's terminal, as grantpt and unlockpt do.
func unlock(master *os.File) error {
	conn, err := master.SyscallConn()
	if err != nil {
		return err
	}
	var ioctlErr error
	err = conn.Control(func(fd uintptr) {
		ioctlErr = unix.IoctlSetInt(int(fd), unix.TIOCPTYGRANT, 0)
		if ioctlErr == nil {
			ioctlErr = unix.IoctlSetInt(int(fd), unix.TIOCPTYUNLK, 0)
		}
	})
	return errors.Join(err, ioctlErr)
}

// deviceMinor returns the minor number of the device f is.
func deviceMinor(f *os.File) (uint32, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeCharDevice == 0 {
		return 0, fmt.Errorf("%s isn't a terminal", f.Name())
	}
	return unix.Minor(uint64(stat.Rdev)), nil
}

// start starts the program at path, run as argv, with env, on tty: its
// standard streams, and its controlling terminal, in a session of its own, so
// it draws as at a terminal, and its process group, which it leads, is killed
// when ctx ends, all it started on tty with it.
func start(ctx context.Context, path string, argv, env []string, tty *os.File) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, path)
	cmd.Args = argv
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process.Pid) }
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// killGroup kills the process group leader leads.
func killGroup(leader int) error {
	err := syscall.Kill(-leader, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
