//go:build darwin

package setuptoken

import (
	"io"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenTerminalOpensAPseudoTerminalOfTheSizeAsked(t *testing.T) {
	master, tty, err := openTerminal(1000, 50)
	if err != nil {
		t.Fatalf("openTerminal() failed: %v", err)
	}
	defer func() { _, _ = master.Close(), tty.Close() }()

	size, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ)
	if err != nil || size.Col != 1000 || size.Row != 50 {
		t.Errorf("the terminal is %+v (%v), want 1000 columns by 50 rows", size, err)
	}
	if _, err := tty.WriteString("drawn"); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len("drawn"))
	if _, err := io.ReadFull(master, got); err != nil || string(got) != "drawn" {
		t.Errorf("the master read %q (%v), want what was drawn on its terminal", got, err)
	}
}

// As the terminal's unlocked, it's its owner's alone already, and it was
// locked till then, so nothing opened it before it was made so.
func TestOpenTerminalMakesItsTerminalItsOwnersAloneWhileItsLocked(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = master.Close() }()
	var mode os.FileMode
	var opened error
	checking := func(master *os.File) error {
		name, _, err := terminalName(master)
		if err != nil {
			return err
		}
		info, err := os.Stat(name)
		if err != nil {
			return err
		}
		mode = info.Mode().Perm()
		var early *os.File
		if early, opened = os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0); opened == nil {
			_ = early.Close()
		}
		return unlock(master)
	}

	tty, err := openItsTerminal(master, checking)
	if err != nil {
		t.Fatalf("openItsTerminal() failed: %v", err)
	}
	defer func() { _ = tty.Close() }()

	if mode != 0o600 {
		t.Errorf("as it's unlocked, the terminal's mode is %v, want %v: its owner's alone", mode, os.FileMode(0o600))
	}
	if opened == nil {
		t.Error("the terminal opened before it was unlocked, want it locked till then")
	}
	info, err := tty.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("the terminal opened is %v, want it its owner's alone", got)
	}
}
