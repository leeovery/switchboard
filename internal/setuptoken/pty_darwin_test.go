//go:build darwin

package setuptoken

import (
	"io"
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
